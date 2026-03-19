package dingtalk

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
)

func extractChallenge(body []byte) (string, error) {
	var req challengeRequest
	if err := json.Unmarshal(body, &req); err != nil {
		return "", err
	}
	return req.Challenge, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func requiredString(name, value string) error {
	if value == "" {
		return fmt.Errorf("%s is required", name)
	}
	return nil
}

type encryptedCallbackEnvelope struct {
	Encrypt string `json:"encrypt,omitempty"`
}

func callbackSignature(token, timestamp, nonce, encrypt string) string {
	items := []string{token, timestamp, nonce, encrypt}
	sort.Strings(items)
	sum := sha1.Sum([]byte(strings.Join(items, "")))
	return fmt.Sprintf("%x", sum[:])
}

func decryptCallbackBody(aesKey, receiveKey, encrypt string) ([]byte, error) {
	key, err := base64.StdEncoding.DecodeString(aesKey + "=")
	if err != nil {
		return nil, err
	}
	cipherText, err := base64.StdEncoding.DecodeString(encrypt)
	if err != nil {
		return nil, err
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, err
	}
	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, key[:aes.BlockSize]).CryptBlocks(plain, cipherText)

	plain, err = pkcs7Unpad(plain, 32)
	if err != nil {
		return nil, err
	}
	if len(plain) < 20 {
		return nil, fmt.Errorf("dingtalk callback body too short")
	}

	msgLen := binary.BigEndian.Uint32(plain[16:20])
	end := 20 + int(msgLen)
	if end > len(plain) {
		return nil, fmt.Errorf("dingtalk callback body length is invalid")
	}
	msg := plain[20:end]
	if tail := string(plain[end:]); tail != receiveKey {
		return nil, fmt.Errorf("dingtalk callback receive key mismatch")
	}
	return msg, nil
}

func encryptCallbackBody(aesKey, receiveKey string, body []byte) (string, error) {
	key, err := base64.StdEncoding.DecodeString(aesKey + "=")
	if err != nil {
		return "", err
	}

	buf := bytes.NewBuffer(nil)
	buf.WriteString("0123456789abcdef")
	if err := binary.Write(buf, binary.BigEndian, uint32(len(body))); err != nil {
		return "", err
	}
	buf.Write(body)
	buf.WriteString(receiveKey)

	padded := pkcs7Pad(buf.Bytes(), 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", err
	}
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, key[:aes.BlockSize]).CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted), nil
}

func pkcs7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7Unpad(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid pkcs7 data length")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	for _, value := range data[len(data)-padding:] {
		if int(value) != padding {
			return nil, fmt.Errorf("invalid pkcs7 padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}
