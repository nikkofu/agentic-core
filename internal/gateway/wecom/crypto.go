package wecom

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"io"
	"sort"
	"strings"
)

type Codec struct {
	token     string
	receiveID string
	aesKey    []byte
	rand      io.Reader
}

func NewCodec(cfg Config) (*Codec, error) {
	key := strings.TrimSpace(cfg.EncodingAESKey)
	if key == "" {
		return nil, fmt.Errorf("encoding aes key is required")
	}

	decoded, err := base64.StdEncoding.DecodeString(key + "=")
	if err != nil {
		return nil, fmt.Errorf("decode aes key: %w", err)
	}
	if len(decoded) != 32 {
		return nil, fmt.Errorf("invalid aes key length %d", len(decoded))
	}

	return &Codec{
		token:     strings.TrimSpace(cfg.Token),
		receiveID: strings.TrimSpace(cfg.CorpID),
		aesKey:    decoded,
		rand:      rand.Reader,
	}, nil
}

func (c *Codec) Signature(timestamp, nonce, encrypted string) string {
	parts := []string{c.token, timestamp, nonce, encrypted}
	sort.Strings(parts)
	sum := sha1.Sum([]byte(strings.Join(parts, "")))
	return hex.EncodeToString(sum[:])
}

func (c *Codec) VerifyURL(signature, timestamp, nonce, echostr string) (string, error) {
	plain, err := c.Decrypt(signature, timestamp, nonce, echostr)
	if err != nil {
		return "", err
	}
	return string(plain), nil
}

func (c *Codec) Encrypt(plain []byte) (string, error) {
	randomBytes := make([]byte, 16)
	if _, err := io.ReadFull(c.rand, randomBytes); err != nil {
		return "", err
	}

	var buf bytes.Buffer
	buf.Write(randomBytes)

	msgLength := make([]byte, 4)
	binary.BigEndian.PutUint32(msgLength, uint32(len(plain)))
	buf.Write(msgLength)
	buf.Write(plain)
	buf.WriteString(c.receiveID)

	padded := pkcs7Pad(buf.Bytes(), 32)
	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return "", err
	}

	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, c.aesKey[:16]).CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func (c *Codec) Decrypt(signature, timestamp, nonce, encrypted string) ([]byte, error) {
	expected := c.Signature(timestamp, nonce, encrypted)
	if signature != expected {
		return nil, fmt.Errorf("invalid signature")
	}

	ciphertext, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		return nil, fmt.Errorf("decode ciphertext: %w", err)
	}
	if len(ciphertext) == 0 || len(ciphertext)%aes.BlockSize != 0 {
		return nil, fmt.Errorf("invalid ciphertext size")
	}

	block, err := aes.NewCipher(c.aesKey)
	if err != nil {
		return nil, err
	}

	plain := make([]byte, len(ciphertext))
	cipher.NewCBCDecrypter(block, c.aesKey[:16]).CryptBlocks(plain, ciphertext)

	unpadded, err := pkcs7Unpad(plain, 32)
	if err != nil {
		return nil, err
	}
	if len(unpadded) < 20 {
		return nil, fmt.Errorf("plaintext too short")
	}

	msgLength := binary.BigEndian.Uint32(unpadded[16:20])
	if int(20+msgLength) > len(unpadded) {
		return nil, fmt.Errorf("invalid plaintext message length")
	}

	msg := unpadded[20 : 20+msgLength]
	receiveID := string(unpadded[20+msgLength:])
	if c.receiveID != "" && receiveID != c.receiveID {
		return nil, fmt.Errorf("receive id mismatch")
	}
	return msg, nil
}

func pkcs7Pad(src []byte, blockSize int) []byte {
	padding := blockSize - len(src)%blockSize
	return append(src, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(src []byte, blockSize int) ([]byte, error) {
	if len(src) == 0 || len(src)%blockSize != 0 {
		return nil, fmt.Errorf("invalid padded data")
	}
	padding := int(src[len(src)-1])
	if padding == 0 || padding > blockSize || padding > len(src) {
		return nil, fmt.Errorf("invalid padding")
	}
	for _, b := range src[len(src)-padding:] {
		if int(b) != padding {
			return nil, fmt.Errorf("invalid padding content")
		}
	}
	return src[:len(src)-padding], nil
}
