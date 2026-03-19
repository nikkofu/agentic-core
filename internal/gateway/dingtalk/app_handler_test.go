package dingtalk

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/gateway"
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/sha1"
	"encoding/base64"
	"encoding/binary"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"
	"time"
)

type failingQueue struct {
	err error
}

func (f *failingQueue) Enqueue(ctx context.Context, queue string, msg bus.Message) error {
	return f.err
}

func (f *failingQueue) Dequeue(ctx context.Context, queue string) (<-chan bus.Message, error) {
	ch := make(chan bus.Message)
	close(ch)
	return ch, nil
}

func newAppAdapterForTest(t *testing.T, cfg AppConfig) *AppAdapter {
	t.Helper()
	adapter, err := NewAppAdapter(cfg, nil)
	if err != nil {
		t.Fatalf("NewAppAdapter failed: %v", err)
	}
	return adapter
}

func TestAppHandlerRespondsToChallenge(t *testing.T) {
	adapter := newAppAdapterForTest(t, AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "ding-aes-key",
	})
	router := gateway.NewSessionRouter(bus.NewFakeTransport())

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(`{"challenge":"challenge-token"}`))
	rec := httptest.NewRecorder()

	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"challenge":"challenge-token"`) {
		t.Fatalf("expected challenge response, got %s", rec.Body.String())
	}
}

func TestAppHandlerMapsTextAndEventCallbacks(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	adapter := newAppAdapterForTest(t, AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "ding-aes-key",
	})

	body := `{
		"conversationId":"cid-1",
		"msgId":"mid-1",
		"chatbotUserId":"chatbot-user-1",
		"senderStaffId":"staff-1",
		"senderNick":"张三",
		"msgtype":"text",
		"text":{"content":"你好，钉钉"},
		"eventType":"chat_message"
	}`

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var enqueued bus.Message
	select {
	case enqueued = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}

	var payload map[string]any
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	if payload["channel"] != "dingtalk_app" {
		t.Fatalf("expected channel dingtalk_app, got %#v", payload["channel"])
	}
	if payload["session_id"] != "cid-1" {
		t.Fatalf("expected session_id cid-1, got %#v", payload["session_id"])
	}
	if payload["task"] != "你好，钉钉" {
		t.Fatalf("expected text payload, got %#v", payload["task"])
	}
	if payload["message_type"] != "text" {
		t.Fatalf("expected text message_type, got %#v", payload["message_type"])
	}
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", payload["metadata"])
	}
	if metadata["conversationId"] != "cid-1" {
		t.Fatalf("expected conversationId metadata, got %#v", metadata["conversationId"])
	}
	if metadata["senderStaffId"] != "staff-1" {
		t.Fatalf("expected senderStaffId metadata, got %#v", metadata["senderStaffId"])
	}
}

func TestAppHandlerDecryptsEncryptedCallbackAndReturnsEncryptedAck(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	cfg := AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
	}
	adapter := newAppAdapterForTest(t, cfg)

	plain := []byte(`{"conversationId":"cid-enc","msgId":"mid-enc","chatbotUserId":"bot-user-enc","senderStaffId":"staff-enc","senderNick":"李四","msgtype":"text","text":{"content":"加密消息"}}`)
	body, signature := encryptDingTalkCallbackForTest(t, cfg.Token, cfg.AESKey, cfg.ClientID, plain, "1700000000", "nonce-enc")

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events?signature="+signature+"&timestamp=1700000000&nonce=nonce-enc", strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	var ack map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &ack); err != nil {
		t.Fatalf("unmarshal ack failed: %v", err)
	}
	ackEncrypt := stringValue(ack["encrypt"])
	if ackEncrypt == "" {
		t.Fatalf("expected encrypted ack body, got %#v", ack)
	}
	if decryptDingTalkForTest(t, cfg.AESKey, cfg.ClientID, ackEncrypt) != "success" {
		t.Fatalf("expected decrypted ack success, got %s", decryptDingTalkForTest(t, cfg.AESKey, cfg.ClientID, ackEncrypt))
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var enqueued bus.Message
	select {
	case enqueued = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}

	var payload map[string]any
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload["session_id"] != "cid-enc" {
		t.Fatalf("expected encrypted callback to map cid-enc, got %#v", payload["session_id"])
	}
	if payload["task"] != "加密消息" {
		t.Fatalf("expected encrypted callback text, got %#v", payload["task"])
	}
}

func TestAppHandlerRejectsEncryptedCallbackWithInvalidSignature(t *testing.T) {
	router := gateway.NewSessionRouter(bus.NewFakeTransport())
	cfg := AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG",
	}
	adapter := newAppAdapterForTest(t, cfg)

	plain := []byte(`{"conversationId":"cid-enc","msgId":"mid-enc","chatbotUserId":"bot-user-enc","senderStaffId":"staff-enc","msgtype":"text","text":{"content":"加密消息"}}`)
	body, _ := encryptDingTalkCallbackForTest(t, cfg.Token, cfg.AESKey, cfg.ClientID, plain, "1700000000", "nonce-enc")

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events?signature=bad-signature&timestamp=1700000000&nonce=nonce-enc", strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestAppHandlerMapsImageAudioVideoAndFileCallbacks(t *testing.T) {
	tests := []struct {
		name          string
		body          string
		wantMediaID   string
		wantMediaKind string
	}{
		{
			name:          "image",
			body:          `{"conversationId":"cid-media","msgId":"mid-image","chatbotUserId":"chatbot-user","senderStaffId":"staff-1","msgtype":"image","image":{"mediaId":"img-1"}}`,
			wantMediaID:   "img-1",
			wantMediaKind: "image",
		},
		{
			name:          "audio",
			body:          `{"conversationId":"cid-media","msgId":"mid-audio","chatbotUserId":"chatbot-user","senderStaffId":"staff-1","msgtype":"audio","audio":{"mediaId":"aud-1"}}`,
			wantMediaID:   "aud-1",
			wantMediaKind: "audio",
		},
		{
			name:          "video",
			body:          `{"conversationId":"cid-media","msgId":"mid-video","chatbotUserId":"chatbot-user","senderStaffId":"staff-1","msgtype":"video","video":{"mediaId":"vid-1"}}`,
			wantMediaID:   "vid-1",
			wantMediaKind: "video",
		},
		{
			name:          "file",
			body:          `{"conversationId":"cid-media","msgId":"mid-file","chatbotUserId":"chatbot-user","senderStaffId":"staff-1","msgtype":"file","file":{"mediaId":"file-1","fileName":"demo.pdf"}}`,
			wantMediaID:   "file-1",
			wantMediaKind: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := bus.NewFakeTransport()
			router := gateway.NewSessionRouter(transport)
			adapter := newAppAdapterForTest(t, AppConfig{
				ClientID:     "ding-app-id",
				ClientSecret: "ding-secret",
				AgentID:      900001,
				Token:        "ding-token",
				AESKey:       "ding-aes-key",
			})

			req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(tt.body))
			rec := httptest.NewRecorder()
			adapter.EventHandler(router).ServeHTTP(rec, req)

			if rec.Code != http.StatusOK {
				t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
			}

			ctx, cancel := context.WithTimeout(context.Background(), time.Second)
			defer cancel()
			msgChan, err := transport.Dequeue(ctx, "tasks")
			if err != nil {
				t.Fatalf("Dequeue failed: %v", err)
			}

			var enqueued bus.Message
			select {
			case enqueued = <-msgChan:
			case <-ctx.Done():
				t.Fatal("timed out waiting for task")
			}

			var payload struct {
				MessageType string              `json:"message_type"`
				Media       []gateway.MediaItem `json:"media"`
			}
			if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
				t.Fatalf("unmarshal payload failed: %v", err)
			}
			if len(payload.Media) != 1 {
				t.Fatalf("expected one media item, got %#v", payload.Media)
			}
			if payload.Media[0].MediaID != tt.wantMediaID {
				t.Fatalf("expected media id %s, got %s", tt.wantMediaID, payload.Media[0].MediaID)
			}
			if string(payload.Media[0].Kind) != tt.wantMediaKind {
				t.Fatalf("expected media kind %s, got %s", tt.wantMediaKind, payload.Media[0].Kind)
			}
		})
	}
}

func TestAppHandlerMapsCardActionCallback(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	adapter := newAppAdapterForTest(t, AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "ding-aes-key",
	})

	body := `{
		"cardCallbackId":"cb-1",
		"conversationId":"cid-card",
		"msgId":"mid-card",
		"chatbotUserId":"chatbot-user-card",
		"senderStaffId":"staff-card",
		"value":{"action":"approve"},
		"cardData":{"approvalId":"ap-1"}
	}`

	req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/cards", strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.CardHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var enqueued bus.Message
	select {
	case enqueued = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}

	var payload struct {
		MessageType string                 `json:"message_type"`
		Task        string                 `json:"task"`
		Card        map[string]any         `json:"card"`
		Metadata    map[string]interface{} `json:"metadata"`
	}
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}
	if payload.MessageType != "event" {
		t.Fatalf("expected event message type, got %s", payload.MessageType)
	}
	if payload.Task != "card_action" {
		t.Fatalf("expected card_action task, got %s", payload.Task)
	}
	if payload.Card == nil {
		t.Fatal("expected card payload")
	}
	if payload.Metadata["cardCallbackId"] != "cb-1" {
		t.Fatalf("expected card callback metadata, got %#v", payload.Metadata["cardCallbackId"])
	}
}

func TestAppHandlerReturnsStatusCodesForMalformedCallbacks(t *testing.T) {
	adapter := newAppAdapterForTest(t, AppConfig{
		ClientID:     "ding-app-id",
		ClientSecret: "ding-secret",
		AgentID:      900001,
		Token:        "ding-token",
		AESKey:       "ding-aes-key",
	})

	t.Run("invalid json", func(t *testing.T) {
		router := gateway.NewSessionRouter(bus.NewFakeTransport())
		req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(`{bad json`))
		rec := httptest.NewRecorder()

		adapter.EventHandler(router).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("enqueue failure", func(t *testing.T) {
		router := gateway.NewSessionRouter(&failingQueue{err: errors.New("boom")})
		req := httptest.NewRequest(http.MethodPost, "/callbacks/dingtalk/events", strings.NewReader(`{"conversationId":"cid-1","msgId":"mid-1","chatbotUserId":"bot-user-1","senderStaffId":"staff-1","msgtype":"text","text":{"content":"hello"}}`))
		rec := httptest.NewRecorder()

		adapter.EventHandler(router).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func encryptDingTalkCallbackForTest(t *testing.T, token, aesKey, receiveKey string, plain []byte, timestamp, nonce string) (string, string) {
	t.Helper()

	encrypted := encryptDingTalkForTest(t, aesKey, receiveKey, plain)
	signature := dingTalkSignatureForTest(token, timestamp, nonce, encrypted)
	body, err := json.Marshal(map[string]string{"encrypt": encrypted})
	if err != nil {
		t.Fatalf("marshal encrypted body failed: %v", err)
	}
	return string(body), signature
}

func encryptDingTalkForTest(t *testing.T, aesKey, receiveKey string, plain []byte) string {
	t.Helper()

	key, err := base64.StdEncoding.DecodeString(aesKey + "=")
	if err != nil {
		t.Fatalf("decode aes key failed: %v", err)
	}

	msg := bytes.NewBuffer(nil)
	msg.WriteString("0123456789abcdef")
	if err := binary.Write(msg, binary.BigEndian, uint32(len(plain))); err != nil {
		t.Fatalf("write msg length failed: %v", err)
	}
	msg.Write(plain)
	msg.WriteString(receiveKey)

	padded := pkcs7PadForTest(msg.Bytes(), 32)
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher failed: %v", err)
	}
	iv := key[:aes.BlockSize]
	encrypted := make([]byte, len(padded))
	cipher.NewCBCEncrypter(block, iv).CryptBlocks(encrypted, padded)
	return base64.StdEncoding.EncodeToString(encrypted)
}

func decryptDingTalkForTest(t *testing.T, aesKey, receiveKey, encrypted string) string {
	t.Helper()

	key, err := base64.StdEncoding.DecodeString(aesKey + "=")
	if err != nil {
		t.Fatalf("decode aes key failed: %v", err)
	}
	cipherText, err := base64.StdEncoding.DecodeString(encrypted)
	if err != nil {
		t.Fatalf("decode cipher text failed: %v", err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatalf("new cipher failed: %v", err)
	}
	iv := key[:aes.BlockSize]
	plain := make([]byte, len(cipherText))
	cipher.NewCBCDecrypter(block, iv).CryptBlocks(plain, cipherText)
	plain, err = pkcs7UnpadForTest(plain, 32)
	if err != nil {
		t.Fatalf("unpad failed: %v", err)
	}
	if len(plain) < 20 {
		t.Fatalf("plain text too short: %d", len(plain))
	}
	msgLen := binary.BigEndian.Uint32(plain[16:20])
	end := 20 + int(msgLen)
	if end > len(plain) {
		t.Fatalf("invalid msg len: %d", msgLen)
	}
	msg := string(plain[20:end])
	if tail := string(plain[end:]); tail != receiveKey {
		t.Fatalf("expected receive key %s, got %s", receiveKey, tail)
	}
	return msg
}

func dingTalkSignatureForTest(token, timestamp, nonce, encrypted string) string {
	items := []string{token, timestamp, nonce, encrypted}
	sort.Strings(items)
	hash := sha1.Sum([]byte(strings.Join(items, "")))
	return fmt.Sprintf("%x", hash[:])
}

func pkcs7PadForTest(data []byte, blockSize int) []byte {
	padding := blockSize - len(data)%blockSize
	return append(data, bytes.Repeat([]byte{byte(padding)}, padding)...)
}

func pkcs7UnpadForTest(data []byte, blockSize int) ([]byte, error) {
	if len(data) == 0 || len(data)%blockSize != 0 {
		return nil, fmt.Errorf("invalid pkcs7 data length")
	}
	padding := int(data[len(data)-1])
	if padding == 0 || padding > blockSize || padding > len(data) {
		return nil, fmt.Errorf("invalid pkcs7 padding")
	}
	for _, item := range data[len(data)-padding:] {
		if int(item) != padding {
			return nil, fmt.Errorf("invalid pkcs7 padding bytes")
		}
	}
	return data[:len(data)-padding], nil
}
