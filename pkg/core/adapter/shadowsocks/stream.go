// Package shadowsocks Shadowsocks 流式连接实现
package shadowsocks

import (
	"crypto/rand"
	"encoding/binary"
	"io"
	"net"
	"sync"

	"github.com/Qing060325/Hades/pkg/perf/pool"
)

// StreamConn Shadowsocks 流式加密连接
type StreamConn struct {
	net.Conn
	reader *aeadReader
	writer *aeadWriter
}

// NewStreamConn 创建流式连接
func NewStreamConn(conn net.Conn, cipher *aeadCipher, mode int) *StreamConn {
	sc := &StreamConn{
		Conn: conn,
	}

	if mode == clientMode {
		sc.writer = newAEADWriter(conn, cipher)
		sc.reader = newAEADReader(conn, cipher)
	}

	return sc
}

// Read 读取数据
func (c *StreamConn) Read(b []byte) (int, error) {
	if c.reader != nil {
		return c.reader.Read(b)
	}
	return c.Conn.Read(b)
}

// Write 写入数据
func (c *StreamConn) Write(b []byte) (int, error) {
	if c.writer != nil {
		return c.writer.Write(b)
	}
	return c.Conn.Write(b)
}

// aeadWriter AEAD 写入器
type aeadWriter struct {
	conn      io.Writer
	cipher    *aeadCipher
	salt      []byte
	nonce     uint64
	mu        sync.Mutex
}

func newAEADWriter(conn io.Writer, cipher *aeadCipher) *aeadWriter {
	return &aeadWriter{
		conn:   conn,
		cipher: cipher,
	}
}

// Write 写入加密数据
func (w *aeadWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	// 首次写入，发送 salt
	if w.salt == nil {
		salt := make([]byte, w.cipher.saltSize)
		if _, err := rand.Read(salt); err != nil {
			return 0, err
		}
		w.salt = salt
		if _, err := w.conn.Write(salt); err != nil {
			return 0, err
		}
	}

	// 分片加密
	// 分片大小: [encrypted_len(2+16)][encrypted_payload(chunk+16)]
	remaining := b
	totalWritten := 0

	for len(remaining) > 0 {
		chunkSize := len(remaining)
		if chunkSize > maxPayloadSize {
			chunkSize = maxPayloadSize
		}

		chunk := remaining[:chunkSize]
		remaining = remaining[chunkSize:]

		// 派生子密钥
		subKey := deriveSubkey(w.cipher.key, w.salt, w.cipher.keySize)
		aead, err := createAEAD(subKey)
		if err != nil {
			return totalWritten, err
		}

		// 构造 nonce
		nonceBytes := make([]byte, w.cipher.nonceSize)
		binary.BigEndian.PutUint64(nonceBytes[w.cipher.nonceSize-8:], w.nonce)

		// 加密长度
		lenBuf := make([]byte, 2)
		binary.BigEndian.PutUint16(lenBuf, uint16(chunkSize))

		encLen := make([]byte, w.cipher.nonceSize+aead.Overhead())
		aead.Seal(encLen[:0], nonceBytes, lenBuf, nil)
		w.nonce++
		binary.BigEndian.PutUint64(nonceBytes[w.cipher.nonceSize-8:], w.nonce)

		// 加密数据
		encPayload := make([]byte, chunkSize+aead.Overhead())
		aead.Seal(encPayload[:0], nonceBytes, chunk, nil)
		w.nonce++

		// 写入
		if _, err := w.conn.Write(encLen); err != nil {
			return totalWritten, err
		}
		if _, err := w.conn.Write(encPayload); err != nil {
			return totalWritten, err
		}

		totalWritten += chunkSize
	}

	return totalWritten, nil
}

// CloseWithSalt 关闭并清理
func (w *aeadWriter) CloseWithSalt() error {
	return nil
}

// Relay 双向转发 (Shadowsocks 加密)
func Relay(left, right net.Conn) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(left, right, buf)
		left.Close()
	}()

	go func() {
		defer wg.Done()
		buf := pool.GetLarge()
		defer pool.Put(buf)
		io.CopyBuffer(right, left, buf)
		right.Close()
	}()

	wg.Wait()
}
