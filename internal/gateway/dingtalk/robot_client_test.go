package dingtalk

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

func TestRobotClientBuildsTextPayload(t *testing.T) {
	payload, err := buildRobotPayload(gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		MessageType: gateway.MessageTypeText,
		Text:        "hello ding",
	})
	if err != nil {
		t.Fatalf("buildRobotPayload failed: %v", err)
	}

	if got := payload["msgtype"]; got != "text" {
		t.Fatalf("expected msgtype text, got %#v", got)
	}

	textBody, ok := payload["text"].(map[string]any)
	if !ok {
		t.Fatalf("expected text payload, got %#v", payload["text"])
	}
	if got := textBody["content"]; got != "hello ding" {
		t.Fatalf("expected text content, got %#v", got)
	}
}

func TestRobotClientBuildsMarkdownPayload(t *testing.T) {
	payload, err := buildRobotPayload(gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		MessageType: gateway.MessageTypeMarkdown,
		Format:      gateway.FormatMarkdown,
		Text:        "## deploy ok",
		Metadata: map[string]any{
			"title": "Deploy Status",
		},
	})
	if err != nil {
		t.Fatalf("buildRobotPayload failed: %v", err)
	}

	if got := payload["msgtype"]; got != "markdown" {
		t.Fatalf("expected msgtype markdown, got %#v", got)
	}

	markdownBody, ok := payload["markdown"].(map[string]any)
	if !ok {
		t.Fatalf("expected markdown payload, got %#v", payload["markdown"])
	}
	if got := markdownBody["title"]; got != "Deploy Status" {
		t.Fatalf("expected markdown title, got %#v", got)
	}
	if got := markdownBody["text"]; got != "## deploy ok" {
		t.Fatalf("expected markdown text, got %#v", got)
	}
}

func TestRobotClientBuildsCardPayload(t *testing.T) {
	payload, err := buildRobotPayload(gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		Card: map[string]any{
			"msgtype": "actionCard",
			"actionCard": map[string]any{
				"title": "Alert",
				"text":  "### investigate",
			},
		},
	})
	if err != nil {
		t.Fatalf("buildRobotPayload failed: %v", err)
	}

	if got := payload["msgtype"]; got != "actionCard" {
		t.Fatalf("expected msgtype actionCard, got %#v", got)
	}

	cardBody, ok := payload["actionCard"].(map[string]any)
	if !ok {
		t.Fatalf("expected actionCard payload, got %#v", payload["actionCard"])
	}
	if got := cardBody["title"]; got != "Alert" {
		t.Fatalf("expected actionCard title, got %#v", got)
	}
}

func TestRobotClientUsesMetadataWebhookOverride(t *testing.T) {
	var capturedPath string
	var capturedBody map[string]any

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.RawQuery
		defer r.Body.Close()
		if err := json.NewDecoder(r.Body).Decode(&capturedBody); err != nil {
			t.Fatalf("decode body failed: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"errcode":0,"errmsg":"ok"}`))
	}))
	defer server.Close()

	client := newRobotClient(RobotConfig{
		WebhookURL:  "https://oapi.dingtalk.com/robot/send?access_token=default-token",
		Secret:      "default-secret",
		HTTPTimeout: time.Second,
	}, server.Client())
	client.now = func() time.Time {
		return time.Unix(1700000000, 0)
	}

	err := client.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		MessageType: gateway.MessageTypeText,
		Text:        "override webhook",
		Metadata: map[string]any{
			"dingtalk_robot_webhook_url": server.URL + "?access_token=override-token",
			"dingtalk_robot_secret":      "override-secret",
		},
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if !strings.Contains(capturedPath, "timestamp=") {
		t.Fatalf("expected timestamp query, got %s", capturedPath)
	}
	if !strings.Contains(capturedPath, "sign=") {
		t.Fatalf("expected sign query, got %s", capturedPath)
	}
	if got := capturedBody["msgtype"]; got != "text" {
		t.Fatalf("expected text payload, got %#v", got)
	}
}

func TestRobotClientRejectsUnsupportedMediaMessages(t *testing.T) {
	client := newRobotClient(RobotConfig{
		WebhookURL: "https://oapi.dingtalk.com/robot/send?access_token=demo",
	}, nil)

	err := client.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		MessageType: gateway.MessageTypeImage,
		Media: []gateway.MediaItem{{
			Kind: gateway.MediaKindImage,
			URL:  "https://example.com/image.png",
		}},
	})
	if err == nil {
		t.Fatal("expected unsupported media error")
	}
	if !strings.Contains(err.Error(), "unsupported dingtalk robot message type") {
		t.Fatalf("expected unsupported media error, got %v", err)
	}
}

type stubRobotSender struct {
	messages []gateway.ChannelResponse
}

func (s *stubRobotSender) Send(ctx context.Context, msg gateway.ChannelResponse) error {
	s.messages = append(s.messages, msg)
	return nil
}

func TestRobotAdapterSendUsesWebhookClient(t *testing.T) {
	stub := &stubRobotSender{}
	adapter := &RobotAdapter{client: stub}

	err := adapter.Send(context.Background(), gateway.ChannelResponse{
		ChannelName: "dingtalk_robot",
		MessageType: gateway.MessageTypeText,
		Text:        "hello adapter",
	})
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}

	if len(stub.messages) != 1 {
		t.Fatalf("expected one message, got %d", len(stub.messages))
	}
	if got := stub.messages[0].Text; got != "hello adapter" {
		t.Fatalf("expected hello adapter, got %s", got)
	}
}
