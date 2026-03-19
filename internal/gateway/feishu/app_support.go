package feishu

import (
	"encoding/json"
	"errors"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
)

var errUnauthorized = errors.New("unauthorized")

func decryptEventBody(body []byte, encryptKey string) ([]byte, error) {
	var envelope struct {
		Encrypt string `json:"encrypt"`
	}
	if err := json.Unmarshal(body, &envelope); err != nil {
		return nil, err
	}
	if envelope.Encrypt == "" {
		return body, nil
	}
	if encryptKey == "" {
		return nil, errors.New("encrypt_key not found")
	}
	return larkevent.EventDecrypt(envelope.Encrypt, encryptKey)
}
