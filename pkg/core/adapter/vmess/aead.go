// Package vmess VMess AEAD 加密
package vmess

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/md5"
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"io"

	"golang.org/x/crypto/chacha20poly1305"
)

const (
	// aeadKeyLen AEAD 密钥长度
	aeadKeyLen = 16
	// aeadNonceLen AEAD nonce 长度
	aeadNonceLen = 8
)

// AEADHeaderAuth AEAD 头部认证
type AEADHeaderAuth struct {
	Key       [16]byte
	Nonce     [8]byte
	Payload   []byte
	AuthTag   [16]byte
}

// CreateAEAD 创建 AEAD 加密器
func CreateAEAD(key []byte) (cipher.AEAD, error) {
	if len(key) == 16 {
		block, err := aes.NewCipher(key)
		if err != nil {
			return nil, err
		}
		return cipher.NewGCM(block)
	}
	if len(key) == 32 {
		return chacha20poly1305.New(key)
	}
	return nil, fmt.Errorf("无效的密钥长度: %d", len(key))
}

// SealVMessAEADHeader 加密 VMess AEAD 头部
func SealVMessAEADHeader(key [16]byte, payload []byte) ([]byte, error) {
	// 生成随机 nonce
	var nonce [8]byte
	if _, err := rand.Read(nonce[:]); err != nil {
		return nil, err
	}

	// 生成 AEAD Key
	aeadKey := vmessAEADKey(key)

	// 创建 AEAD
	aead, err := CreateAEAD(aeadKey)
	if err != nil {
		return nil, err
	}

	// 加密 payload
	nonceBuf := make([]byte, aead.NonceSize(), aead.NonceSize())
	copy(nonceBuf, nonce[:])

	encrypted := aead.Seal(nil, nonceBuf, payload, nil)

	// 组装结果: nonce + encrypted
	result := make([]byte, len(nonce)+len(encrypted))
	copy(result, nonce[:])
	copy(result[len(nonce):], encrypted)

	return result, nil
}

// OpenVMessAEADHeader 解密 VMess AEAD 头部
func OpenVMessAEADHeader(key [16]byte, data []byte) ([]byte, error) {
	if len(data) < 8+16 {
		return nil, io.ErrShortBuffer
	}

	// 提取 nonce
	nonce := data[:8]

	// 生成 AEAD Key
	aeadKey := vmessAEADKey(key)

	// 创建 AEAD
	aead, err := CreateAEAD(aeadKey)
	if err != nil {
		return nil, err
	}

	// 解密
	nonceBuf := make([]byte, aead.NonceSize(), aead.NonceSize())
	copy(nonceBuf, nonce)

	plaintext, err := aead.Open(nil, nonceBuf, data[8:], nil)
	if err != nil {
		return nil, err
	}

	return plaintext, nil
}

// vmessAEADKey 派生 VMess AEAD 密钥
func vmessAEADKey(key [16]byte) []byte {
	// KDF(key, "VMess Header AEAD Key\u0001")
	k := md5.New()
	k.Write(key[:])
	k.Write([]byte("VMess Header AEAD Key\x01"))
	result := k.Sum(nil)

	// 再做一次
	k = md5.New()
	k.Write(result)
	k.Write([]byte("VMess Header AEAD Key\x01"))
	return k.Sum(nil)
}

// vmessAEADNonceSeed 派生 AEAD nonce seed
func vmessAEADNonceSeed(key [16]byte) []byte {
	k := md5.New()
	k.Write(key[:])
	k.Write([]byte("VMess Header AEAD Nonce\x00"))
	return k.Sum(nil)
}

// vmessAEADLengthHeaderMask 派生长度头部掩码
func vmessAEADLengthHeaderMask(key [16]byte, nonce uint16) [6]byte {
	var maskKey [16]byte
	k := md5.New()
	k.Write(key[:])
	k.Write([]byte("VMess Header AEAD Nonce\x01"))
	copy(maskKey[:], k.Sum(nil))

	seed := make([]byte, 2+aeadKeyLen)
	binary.BigEndian.PutUint16(seed, nonce)
	copy(seed[2:], maskKey[:])

	maskHash := md5.Sum(seed)

	var result [6]byte
	copy(result[:], maskHash[:6])
	return result
}

// SealVMessPayload 加密 VMess 数据帧
func SealVMessPayload(key [16]byte, nonce uint16, data []byte) ([]byte, error) {
	// 派生数据密钥
	dataKey := vmessDataKey(key)

	aead, err := CreateAEAD(dataKey)
	if err != nil {
		return nil, err
	}

	// 构造 nonce: [2字节帧长度计数] + [4字节随机]
	nonceBuf := make([]byte, aead.NonceSize())
	binary.BigEndian.PutUint16(nonceBuf[:2], nonce)
	if _, err := rand.Read(nonceBuf[2:]); err != nil {
		return nil, err
	}

	// 加密
	encrypted := aead.Seal(nil, nonceBuf, data, nil)

	return encrypted, nil
}

// OpenVMessPayload 解密 VMess 数据帧
func OpenVMessPayload(key [16]byte, nonce uint16, data []byte) ([]byte, error) {
	dataKey := vmessDataKey(key)

	aead, err := CreateAEAD(dataKey)
	if err != nil {
		return nil, err
	}

	nonceBuf := make([]byte, aead.NonceSize())
	binary.BigEndian.PutUint16(nonceBuf[:2], nonce)

	return aead.Open(nil, nonceBuf, data, nil)
}

// vmessDataKey 派生数据传输密钥
func vmessDataKey(key [16]byte) []byte {
	k := md5.New()
	k.Write(key[:])
	k.Write([]byte("VMess Header AEAD Key_Length\x00"))
	return k.Sum(nil)
}

// vmessDataIV 派生数据传输 IV
func vmessDataIV(key [16]byte) []byte {
	k := md5.New()
	k.Write(key[:])
	k.Write([]byte("VMess Header AEAD Nonce_Length\x00"))
	return k.Sum(nil)
}
