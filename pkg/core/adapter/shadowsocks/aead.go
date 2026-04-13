// Package shadowsocks AEAD 加密实现
package shadowsocks

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
	"golang.org/x/crypto/hkdf"
)

// aeadCipher AEAD 加密器
type aeadCipher struct {
	key      []byte
	keySize  int
	aead     cipher.AEAD
	saltSize int
	nonceSize int
}

// newAEADCipher 创建 AEAD 加密器
func newAEADCipher(cipherName, password string) (*aeadCipher, error) {
	info, ok := supportedCiphers[cipherName]
	if !ok || !info.AEAD {
		return nil, fmt.Errorf("不支持的加密方式: %s", cipherName)
	}

	// 生成密钥
	key := evpBytesToKey(password, info.KeySize)

	var aead cipher.AEAD
	var err error

	switch cipherName {
	case "aes-128-gcm":
		aead, err = newAesGCM(key)
	case "aes-256-gcm":
		aead, err = newAesGCM(key)
	case "chacha20-ietf-poly1305":
		aead, err = chacha20poly1305.New(key)
	case "xchacha20-ietf-poly1305":
		aead, err = chacha20poly1305.NewX(key)
	case "2022-blake3-aes-128-gcm", "2022-blake3-aes-256-gcm":
		aead, err = newAesGCM(key[:32])
	case "2022-blake3-chacha20-poly1305":
		aead, err = chacha20poly1305.New(key[:32])
	default:
		return nil, fmt.Errorf("不支持的 AEAD 加密方式: %s", cipherName)
	}

	if err != nil {
		return nil, fmt.Errorf("创建 AEAD 失败: %w", err)
	}

	return &aeadCipher{
		key:       key,
		keySize:   info.KeySize,
		aead:      aead,
		saltSize:  saltSize,
		nonceSize: nonceSize,
	}, nil
}

// newAesGCM 创建 AES-GCM
func newAesGCM(key []byte) (cipher.AEAD, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	return cipher.NewGCM(block)
}

// evpBytesToKey 生成密钥 (EVP_BytesToKey)
func evpBytesToKey(password string, keyLen int) []byte {
	const iterations = 16
	var result []byte
	d := make([]byte, 0, sha256.Size)

	for len(result) < keyLen {
		h := sha256.New()
		h.Write(d)
		h.Write([]byte(password))
		d = h.Sum(d[:0])
		result = append(result, d...)

		for i := 1; i < iterations; i++ {
			h = sha256.New()
			h.Write(d)
			d = h.Sum(d[:0])
			result = append(result, d...)
		}
	}

	return result[:keyLen]
}

// deriveSubkey 派生子密钥 (HKDF)
func deriveSubkey(masterKey, salt []byte, keySize int) []byte {
	info := []byte("shadowsocks 2022 subkey")
	hkdf := hkdf.New(sha256.New, masterKey, salt, info)
	subKey := make([]byte, keySize)
	if _, err := io.ReadFull(hkdf, subKey); err != nil {
		return nil
	}
	return subKey
}

// encryptPayload 加密 payload
func (c *aeadCipher) encryptPayload(salt []byte, nonce uint64, payload []byte) ([]byte, error) {
	// 派生子密钥
	subKey := deriveSubkey(c.key, salt, c.keySize)
	if subKey == nil {
		return nil, fmt.Errorf("派生子密钥失败")
	}

	// 创建 AEAD 实例
	var aead cipher.AEAD
	var err error

	if len(subKey) == 32 || len(subKey) == 16 {
		aead, err = newAesGCM(subKey)
	} else {
		aead, err = chacha20poly1305.New(subKey[:32])
	}
	if err != nil {
		return nil, err
	}

	// 构造 nonce
	nonceBytes := make([]byte, c.nonceSize)
	binary.BigEndian.PutUint64(nonceBytes[c.nonceSize-8:], nonce)

	// 加密 payload
	// 格式: [len(2)][encrypted_len(2+16)][encrypted_payload(n+16)]
	buf := make([]byte, payloadSizeLen+c.nonceSize+aead.Overhead())
	binary.BigEndian.PutUint16(buf[:payloadSizeLen], uint16(len(payload)))

	// 加密长度
	dst := buf[payloadSizeLen:]
	aead.Seal(dst[:0], nonceBytes, buf[:payloadSizeLen], nil)
	nonceBytes[c.nonceSize-1]++

	// 加密数据
	dst = dst[c.nonceSize+aead.Overhead():]
	aead.Seal(dst[:0], nonceBytes, payload, nil)

	return buf, nil
}

// decryptPayload 解密 payload
func (c *aeadCipher) decryptPayload(salt []byte, nonce uint64, data []byte) ([]byte, error) {
	// 派生子密钥
	subKey := deriveSubkey(c.key, salt, c.keySize)
	if subKey == nil {
		return nil, fmt.Errorf("派生子密钥失败")
	}

	// 创建 AEAD 实例
	var aead cipher.AEAD
	var err error

	if len(subKey) == 32 || len(subKey) == 16 {
		aead, err = newAesGCM(subKey)
	} else {
		aead, err = chacha20poly1305.New(subKey[:32])
	}
	if err != nil {
		return nil, err
	}

	// 构造 nonce
	nonceBytes := make([]byte, c.nonceSize)
	binary.BigEndian.PutUint64(nonceBytes[c.nonceSize-8:], nonce)

	// 解密长度
	encLenSize := c.nonceSize + aead.Overhead()
	if len(data) < encLenSize {
		return nil, io.ErrShortBuffer
	}

 decryptedLen, err := aead.Open(nil, nonceBytes, data[:encLenSize], nil)
	if err != nil {
		return nil, fmt.Errorf("解密长度失败: %w", err)
	}

	payloadLen := binary.BigEndian.Uint16(decryptedLen)
	nonceBytes[c.nonceSize-1]++

	// 解密数据
	data = data[encLenSize:]
	encPayloadSize := int(payloadLen) + aead.Overhead()
	if len(data) < encPayloadSize {
		return nil, io.ErrShortBuffer
	}

	plaintext, err := aead.Open(nil, nonceBytes, data[:encPayloadSize], nil)
	if err != nil {
		return nil, fmt.Errorf("解密数据失败: %w", err)
	}

	return plaintext, nil
}

// aeadReader AEAD 读取器
type aeadReader struct {
	conn      io.Reader
	cipher    *aeadCipher
	salt      []byte
	nonce     uint64
	buf       []byte
	bufOffset int
}

func newAEADReader(conn io.Reader, cipher *aeadCipher) *aeadReader {
	return &aeadReader{
		conn:   conn,
		cipher: cipher,
	}
}

// Read 从 AEAD 流读取
func (r *aeadReader) Read(b []byte) (int, error) {
	if r.bufOffset < len(r.buf) {
		n := copy(b, r.buf[r.bufOffset:])
		r.bufOffset += n
		return n, nil
	}

	// 读取新的 payload
	r.bufOffset = 0
	r.buf = nil

	// 读取 salt (首次)
	if r.salt == nil {
		salt := make([]byte, r.cipher.saltSize)
		if _, err := io.ReadFull(r.conn, salt); err != nil {
			return 0, err
		}
		r.salt = salt
	}

	// 读取加密的长度
	encLenSize := r.cipher.nonceSize + r.cipher.aead.Overhead()
	encLen := make([]byte, encLenSize)
	if _, err := io.ReadFull(r.conn, encLen); err != nil {
		return 0, err
	}

	// 解密长度
	nonceBytes := make([]byte, r.cipher.nonceSize)
	binary.BigEndian.PutUint64(nonceBytes[r.cipher.nonceSize-8:], r.nonce)

	subKey := deriveSubkey(r.cipher.key, r.salt, r.cipher.keySize)
	aead, err := createAEAD(subKey)
	if err != nil {
		return 0, err
	}

	decryptedLen, err := aead.Open(nil, nonceBytes, encLen, nil)
	if err != nil {
		return 0, err
	}
	payloadLen := binary.BigEndian.Uint16(decryptedLen)
	r.nonce++
	binary.BigEndian.PutUint64(nonceBytes[r.cipher.nonceSize-8:], r.nonce)

	// 读取加密的 payload
	encPayload := make([]byte, int(payloadLen)+aead.Overhead())
	if _, err := io.ReadFull(r.conn, encPayload); err != nil {
		return 0, err
	}

	// 解密 payload
	plaintext, err := aead.Open(nil, nonceBytes, encPayload, nil)
	if err != nil {
		return 0, err
	}
	r.nonce++

	r.buf = plaintext
	n := copy(b, r.buf)
	r.bufOffset = n

	return n, nil
}

// createAEAD 根据 key 创建 AEAD
func createAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) >= 32 {
		return newAesGCM(key[:32])
	}
	return newAesGCM(key)
}

// Blake3 模拟 (使用 SHA256 替代)
func blake3Sum256(data []byte) [32]byte {
	return sha256.Sum256(data)
}
