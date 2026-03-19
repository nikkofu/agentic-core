package feishu

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/gateway"
	"agentic-core/internal/logging"
	"context"
	"encoding/json"
	"errors"
	larkcard "github.com/larksuite/oapi-sdk-go/v3/card"
	larkevent "github.com/larksuite/oapi-sdk-go/v3/event"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
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
		AppID:             "cli_app_id",
		AppSecret:         "cli_secret",
		VerificationToken: "verify-token",
	})
	router := gateway.NewSessionRouter(bus.NewFakeTransport())

	req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(`{"challenge":"challenge-token","token":"verify-token","type":"url_verification"}`))
	rec := httptest.NewRecorder()

	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), `"challenge":"challenge-token"`) {
		t.Fatalf("expected challenge response, got %s", rec.Body.String())
	}
}

func TestAppHandlerMapsTextMessageEvent(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	adapter := newAppAdapterForTest(t, AppConfig{
		AppID:             "cli_app_id",
		AppSecret:         "cli_secret",
		VerificationToken: "verify-token",
	})

	body := `{
		"schema":"2.0",
		"header":{"event_id":"evt-1","token":"verify-token","event_type":"im.message.receive_v1","tenant_key":"tenant-1"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_1","user_id":"u_1","union_id":"on_1"},"sender_type":"user","tenant_key":"tenant-1"},
			"message":{"message_id":"om_1","chat_id":"oc_1","chat_type":"group","message_type":"text","content":"{\"text\":\"你好，飞书\"}"}
		}
	}`

	req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(body))
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

	if payload["channel"] != "feishu_app" {
		t.Fatalf("expected channel feishu_app, got %#v", payload["channel"])
	}
	if payload["session_id"] != "oc_1" {
		t.Fatalf("expected session_id oc_1, got %#v", payload["session_id"])
	}
	if payload["task"] != "你好，飞书" {
		t.Fatalf("expected text payload, got %#v", payload["task"])
	}
	if payload["message_type"] != "text" {
		t.Fatalf("expected text message_type, got %#v", payload["message_type"])
	}
	metadata, ok := payload["metadata"].(map[string]any)
	if !ok {
		t.Fatalf("expected metadata map, got %#v", payload["metadata"])
	}
	if metadata["chat_id"] != "oc_1" {
		t.Fatalf("expected chat_id metadata, got %#v", metadata["chat_id"])
	}
	if metadata["open_id"] != "ou_1" {
		t.Fatalf("expected open_id metadata, got %#v", metadata["open_id"])
	}
}

func TestAppHandlerMapsImageAndFileMessageEvents(t *testing.T) {
	tests := []struct {
		name          string
		messageType   string
		content       string
		wantMediaID   string
		wantMediaKind string
	}{
		{
			name:          "image",
			messageType:   "image",
			content:       `{"image_key":"img_v2_1"}`,
			wantMediaID:   "img_v2_1",
			wantMediaKind: "image",
		},
		{
			name:          "file",
			messageType:   "file",
			content:       `{"file_key":"file_v2_1","file_name":"demo.pdf"}`,
			wantMediaID:   "file_v2_1",
			wantMediaKind: "file",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			transport := bus.NewFakeTransport()
			router := gateway.NewSessionRouter(transport)
			adapter := newAppAdapterForTest(t, AppConfig{
				AppID:             "cli_app_id",
				AppSecret:         "cli_secret",
				VerificationToken: "verify-token",
			})

			body := `{
				"schema":"2.0",
				"header":{"event_id":"evt-media","token":"verify-token","event_type":"im.message.receive_v1","tenant_key":"tenant-1"},
				"event":{
					"sender":{"sender_id":{"open_id":"ou_1","user_id":"u_1"},"sender_type":"user","tenant_key":"tenant-1"},
					"message":{"message_id":"om_media","chat_id":"oc_media","chat_type":"group","message_type":"` + tt.messageType + `","content":"` + strings.ReplaceAll(tt.content, `"`, `\"`) + `"}
				}
			}`

			req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(body))
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
			if payload.MessageType != tt.messageType {
				t.Fatalf("expected message type %s, got %s", tt.messageType, payload.MessageType)
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

func TestAppHandlerMapsCardActionEvent(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	adapter := newAppAdapterForTest(t, AppConfig{
		AppID:             "cli_app_id",
		AppSecret:         "cli_secret",
		VerificationToken: "verify-token",
	})

	body := `{"open_id":"ou_1","user_id":"u_1","open_message_id":"om_1","open_chat_id":"oc_1","tenant_key":"tenant-1","token":"verify-token","type":"block_actions","action":{"tag":"button","value":{"action":"approve"}}}`
	timestamp := "1773840000"
	nonce := "nonce-card"
	signature := larkcard.Signature(timestamp, nonce, "verify-token", body)

	req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/cards", strings.NewReader(body))
	req.Header.Set(larkevent.EventRequestTimestamp, timestamp)
	req.Header.Set(larkevent.EventRequestNonce, nonce)
	req.Header.Set(larkevent.EventSignature, signature)

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
		SessionID   string                 `json:"session_id"`
		MessageType string                 `json:"message_type"`
		Task        string                 `json:"task"`
		Metadata    map[string]any         `json:"metadata"`
		Card        map[string]interface{} `json:"card"`
	}
	if err := json.Unmarshal(enqueued.Payload, &payload); err != nil {
		t.Fatalf("unmarshal payload failed: %v", err)
	}

	if payload.SessionID != "oc_1" {
		t.Fatalf("expected session id oc_1, got %s", payload.SessionID)
	}
	if payload.MessageType != "event" {
		t.Fatalf("expected event message type, got %s", payload.MessageType)
	}
	if payload.Task != "card_action" {
		t.Fatalf("expected card_action task, got %s", payload.Task)
	}
	if payload.Metadata["open_message_id"] != "om_1" {
		t.Fatalf("expected open_message_id metadata, got %#v", payload.Metadata["open_message_id"])
	}
	if payload.Metadata["action_tag"] != "button" {
		t.Fatalf("expected action_tag metadata, got %#v", payload.Metadata["action_tag"])
	}
	if payload.Card == nil {
		t.Fatal("expected card payload preserved")
	}
}

func TestAppHandlerReturnsStatusCodesForMalformedCallbacks(t *testing.T) {
	t.Run("bad event body", func(t *testing.T) {
		adapter := newAppAdapterForTest(t, AppConfig{
			AppID:             "cli_app_id",
			AppSecret:         "cli_secret",
			VerificationToken: "verify-token",
		})
		router := gateway.NewSessionRouter(bus.NewFakeTransport())

		req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(`{bad json`))
		rec := httptest.NewRecorder()
		adapter.EventHandler(router).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadRequest {
			t.Fatalf("expected 400, got %d", rec.Code)
		}
	})

	t.Run("bad event signature", func(t *testing.T) {
		adapter := newAppAdapterForTest(t, AppConfig{
			AppID:             "cli_app_id",
			AppSecret:         "cli_secret",
			VerificationToken: "verify-token",
			EncryptKey:        "encrypt-key",
		})
		router := gateway.NewSessionRouter(bus.NewFakeTransport())
		body := `{"schema":"2.0","header":{"event_id":"evt-1","token":"verify-token","event_type":"im.message.receive_v1"},"event":{"sender":{"sender_id":{"open_id":"ou_1"}},"message":{"message_id":"om_1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hello\"}"}}}`
		req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(body))
		req.Header.Set(larkevent.EventRequestTimestamp, "1773840000")
		req.Header.Set(larkevent.EventRequestNonce, "nonce")
		req.Header.Set(larkevent.EventSignature, "bad-signature")

		rec := httptest.NewRecorder()
		adapter.EventHandler(router).ServeHTTP(rec, req)

		if rec.Code != http.StatusUnauthorized {
			t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
		}
	})

	t.Run("enqueue failed", func(t *testing.T) {
		adapter := newAppAdapterForTest(t, AppConfig{
			AppID:             "cli_app_id",
			AppSecret:         "cli_secret",
			VerificationToken: "verify-token",
		})
		router := gateway.NewSessionRouter(&failingQueue{err: errors.New("queue down")})
		body := `{
			"schema":"2.0",
			"header":{"event_id":"evt-1","token":"verify-token","event_type":"im.message.receive_v1","tenant_key":"tenant-1"},
			"event":{
				"sender":{"sender_id":{"open_id":"ou_1"},"sender_type":"user","tenant_key":"tenant-1"},
				"message":{"message_id":"om_1","chat_id":"oc_1","message_type":"text","content":"{\"text\":\"hello\"}"}
			}
		}`
		req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(body))
		rec := httptest.NewRecorder()
		adapter.EventHandler(router).ServeHTTP(rec, req)

		if rec.Code != http.StatusBadGateway {
			t.Fatalf("expected 502, got %d body=%s", rec.Code, rec.Body.String())
		}
	})
}

func TestAppHandlerLogsSafeCallbackMetadata(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "feishu-app-test",
		Dir:           tmpDir,
		Level:         logging.ParseLevel("info"),
		RetentionDays: 7,
		ConsoleEnable: false,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Init logging failed: %v", err)
	}

	transport := bus.NewFakeTransport()
	router := gateway.NewSessionRouter(transport)
	adapter := newAppAdapterForTest(t, AppConfig{
		AppID:             "cli_app_id",
		AppSecret:         "cli-secret-value",
		VerificationToken: "verify-token-value",
	})

	body := `{
		"schema":"2.0",
		"header":{"event_id":"evt-log","token":"verify-token-value","event_type":"im.message.receive_v1","tenant_key":"tenant-1"},
		"event":{
			"sender":{"sender_id":{"open_id":"ou_log","user_id":"u_log"},"sender_type":"user","tenant_key":"tenant-1"},
			"message":{"message_id":"om_log","chat_id":"oc_log","message_type":"text","content":"{\"text\":\"hello log\"}"}
		}
	}`
	req := httptest.NewRequest(http.MethodPost, "/callbacks/feishu/events", strings.NewReader(body))
	rec := httptest.NewRecorder()
	adapter.EventHandler(router).ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d body=%s", rec.Code, rec.Body.String())
	}

	logPath := filepath.Join(tmpDir, "2026-03-18", "feishu-app-test.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	raw := string(data)
	if !strings.Contains(raw, "gateway.feishu_app") {
		t.Fatalf("expected feishu app component log, got %s", raw)
	}
	if !strings.Contains(raw, "oc_log") || !strings.Contains(raw, "om_log") {
		t.Fatalf("expected session/message metadata in logs, got %s", raw)
	}
	if strings.Contains(raw, "cli-secret-value") || strings.Contains(raw, "verify-token-value") {
		t.Fatalf("expected secrets to be absent from logs, got %s", raw)
	}
}
