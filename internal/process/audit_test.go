package process

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentic-core/internal/llm"
	"agentic-core/internal/logging"
)

func TestAuditorRecordWritesStructuredJSONLog(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "process-test",
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

	auditor := NewAuditor(nil, "worker-1")
	err = auditor.Record(context.Background(), llm.AuditEvent{
		TaskID:      "task-1",
		ToolCallID:  "call-1",
		Event:       "tool_call",
		Status:      "running",
		TimestampMs: time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC).UnixMilli(),
		Data:        json.RawMessage(`{"tool_name":"http_get"}`),
	})
	if err != nil {
		t.Fatalf("Record failed: %v", err)
	}

	logPath := filepath.Join(tmpDir, "2026-03-18", "process-test.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected one structured log line, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("unmarshal log line failed: %v", err)
	}
	if payload["component"] != "audit" {
		t.Fatalf("expected audit component, got %+v", payload["component"])
	}
	if payload["event"] != "tool_call" {
		t.Fatalf("expected event tool_call, got %+v", payload["event"])
	}
	if payload["task_id"] != "task-1" {
		t.Fatalf("expected task_id task-1, got %+v", payload["task_id"])
	}
}
