package gateway

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/skill"
)

func TestIngressHandlerAcceptsUnifiedChannelRequest(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	handler := NewIngressHandler(router)

	body := []byte(`{
		"session_id":"session-robot-1",
		"channel_name":"wecom_robot",
		"message_type":"markdown",
		"format":"markdown",
		"text":"**deploy ok**",
		"metadata":{"wecom_robot_webhook_url":"https://qyapi.weixin.qq.com/cgi-bin/webhook/send?key=robot-key"}
	}`)

	req := httptest.NewRequest(http.MethodPost, "/v1/channels/incoming", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}

	var resp struct {
		Status string `json:"status"`
		TaskID string `json:"task_id"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response failed: %v", err)
	}
	if resp.Status != "accepted" {
		t.Fatalf("expected status accepted, got %s", resp.Status)
	}
	if resp.TaskID == "" {
		t.Fatal("expected task id returned")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	msgChan, err := transport.Dequeue(ctx, "tasks")
	if err != nil {
		t.Fatalf("Dequeue failed: %v", err)
	}

	var msg bus.Message
	select {
	case msg = <-msgChan:
	case <-ctx.Done():
		t.Fatal("timed out waiting for task")
	}
	if msg.MessageID != resp.TaskID {
		t.Fatalf("expected returned task id %s, got enqueued %s", resp.TaskID, msg.MessageID)
	}

	var payload map[string]any
	if err := json.Unmarshal(msg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal task payload failed: %v", err)
	}
	if payload["channel"] != "wecom_robot" {
		t.Fatalf("expected channel wecom_robot, got %v", payload["channel"])
	}
	if payload["message_type"] != "markdown" {
		t.Fatalf("expected markdown message_type, got %v", payload["message_type"])
	}
}

func TestIngressHandlerRejectsUnknownFields(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	handler := NewIngressHandler(router)

	req := httptest.NewRequest(http.MethodPost, "/v1/channels/incoming", bytes.NewBufferString(`{"session_id":"s1","channel_name":"web","text":"hello","extra":"bad"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestIngressHandlerRejectsMissingSignatureWhenSecretConfigured(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	handler := NewIngressHandler(router, IngressHandlerConfig{
		Secret: "ingress-secret",
		Now: func() time.Time {
			return time.Unix(1773811200, 0)
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/channels/incoming", bytes.NewBufferString(`{"session_id":"s1","channel_name":"web","text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d body=%s", rec.Code, rec.Body.String())
	}
}

func TestIngressHandlerAcceptsValidSignatureWhenSecretConfigured(t *testing.T) {
	transport := bus.NewFakeTransport()
	router := NewSessionRouter(transport)
	now := time.Unix(1773811200, 0)
	body := []byte(`{"session_id":"secure-1","channel_name":"wecom_robot","text":"secure hello"}`)

	handler := NewIngressHandler(router, IngressHandlerConfig{
		Secret:     "ingress-secret",
		NonceStore: skill.NewInMemNonceStore(),
		Now: func() time.Time {
			return now
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/v1/channels/incoming", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	timestamp := now.Unix()
	nonce := "nonce-ingress-1"
	req.Header.Set(skill.HeaderTimestamp, "1773811200")
	req.Header.Set(skill.HeaderNonce, nonce)
	req.Header.Set(skill.HeaderSignature, skill.GenerateSignature("ingress-secret", timestamp, nonce, body))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d body=%s", rec.Code, rec.Body.String())
	}
}
