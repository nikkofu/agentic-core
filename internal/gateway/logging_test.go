package gateway

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/logging"
)

func TestSessionRouterHandleIncomingWritesStructuredLog(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "gateway-test",
		Dir:           tmpDir,
		Level:         logging.ParseLevel("info"),
		RetentionDays: 30,
		ConsoleEnable: false,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Init logging failed: %v", err)
	}

	router := NewSessionRouter(bus.NewFakeTransport())
	err = router.HandleIncoming(context.Background(), ChannelRequest{
		SessionID:   "session-1",
		ChannelName: "web",
		Text:        "hello",
		SenderName:  "alice",
	})
	if err != nil {
		t.Fatalf("HandleIncoming failed: %v", err)
	}

	logPath := filepath.Join(tmpDir, "2026-03-18", "gateway-test.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one gateway log line")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &payload); err != nil {
		t.Fatalf("unmarshal log line failed: %v", err)
	}
	if payload["component"] != "gateway" {
		t.Fatalf("expected gateway component, got %+v", payload["component"])
	}
	if payload["sender_name"] != "alice" {
		t.Fatalf("expected sender_name alice, got %+v", payload["sender_name"])
	}
}

func TestMockAdapterSendMessageWritesStructuredLog(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "gateway-test",
		Dir:           tmpDir,
		Level:         logging.ParseLevel("info"),
		RetentionDays: 30,
		ConsoleEnable: false,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Init logging failed: %v", err)
	}

	adapter := NewMockAdapter("mock")
	if err := adapter.SendMessage(context.Background(), "session-1", "hello"); err != nil {
		t.Fatalf("SendMessage failed: %v", err)
	}

	logPath := filepath.Join(tmpDir, "2026-03-18", "gateway-test.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) == 0 {
		t.Fatal("expected at least one adapter log line")
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &payload); err != nil {
		t.Fatalf("unmarshal log line failed: %v", err)
	}
	if payload["component"] != "gateway.mock_adapter" {
		t.Fatalf("expected gateway.mock_adapter component, got %+v", payload["component"])
	}
	if payload["session_id"] != "session-1" {
		t.Fatalf("expected session_id session-1, got %+v", payload["session_id"])
	}
}
