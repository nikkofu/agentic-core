package feishu

import (
	"agentic-core/internal/gateway"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestBotClientBuildsSignedTextPayload(t *testing.T) {
	payload, err := buildBotPayload(gateway.ChannelResponse{
		ChannelName: "feishu_bot",
		MessageType: gateway.MessageTypeText,
		Text:        "hello from bot",
	}, time.Unix(1773840000, 0), "bot-secret")
	if err != nil {
		t.Fatalf("buildBotPayload failed: %v", err)
	}

	if payload["msg_type"] != "text" {
		t.Fatalf("expected msg_type text, got %#v", payload["msg_type"])
	}
	if payload["timestamp"] != "1773840000" {
		t.Fatalf("expected timestamp string, got %#v", payload["timestamp"])
	}
	if payload["sign"] == "" {
		t.Fatal("expected sign to be present")
	}

	content, ok := payload["content"].(map[string]any)
	if !ok {
		t.Fatalf("expected content map, got %#v", payload["content"])
	}
	if content["text"] != "hello from bot" {
		t.Fatalf("expected text payload, got %#v", content["text"])
	}
}

func TestBotClientBuildsCardPayload(t *testing.T) {
	payload, err := buildBotPayload(gateway.ChannelResponse{
		ChannelName: "feishu_bot",
		Card: map[string]any{
			"config": map[string]any{
				"wide_screen_mode": true,
			},
			"elements": []any{
				map[string]any{
					"tag":     "markdown",
					"content": "**done**",
				},
			},
		},
	}, time.Unix(1773840000, 0), "")
	if err != nil {
		t.Fatalf("buildBotPayload failed: %v", err)
	}

	if payload["msg_type"] != "interactive" {
		t.Fatalf("expected interactive msg_type, got %#v", payload["msg_type"])
	}
	card, ok := payload["card"].(map[string]any)
	if !ok {
		t.Fatalf("expected card map, got %#v", payload["card"])
	}
	config, ok := card["config"].(map[string]any)
	if !ok || config["wide_screen_mode"] != true {
		t.Fatalf("expected card config preserved, got %#v", card["config"])
	}
}

func TestBotAdapterSendUsesWebhookClient(t *testing.T) {
	var received map[string]any
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer r.Body.Close()
		if r.Method != http.MethodPost {
			t.Fatalf("expected POST, got %s", r.Method)
		}
		if !strings.Contains(r.Header.Get("Content-Type"), "application/json") {
			t.Fatalf("expected json content-type, got %s", r.Header.Get("Content-Type"))
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		_, _ = w.Write([]byte(`{"code":0,"msg":"success"}`))
	}))
	defer server.Close()

	adapter := NewBotAdapter(BotConfig{
		WebhookURL:  server.URL,
		Secret:      "bot-secret",
		HTTPTimeout: time.Second,
	}, server.Client())

	if err := adapter.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "feishu_bot",
		MessageType: gateway.MessageTypeText,
		Text:        "notify team",
	}); err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if received["msg_type"] != "text" {
		t.Fatalf("expected text msg_type, got %#v", received["msg_type"])
	}
	content, ok := received["content"].(map[string]any)
	if !ok || content["text"] != "notify team" {
		t.Fatalf("expected text content, got %#v", received["content"])
	}
	if received["sign"] == "" {
		t.Fatal("expected request sign field")
	}
}
