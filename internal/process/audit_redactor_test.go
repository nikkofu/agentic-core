package process

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentic-core/internal/logging"
)

func TestRedactorMasksAPIKeysAndSecrets(t *testing.T) {
	redacted := redactJSON(mustJSON(map[string]any{
		"api_key": "sk-secret-123",
		"nested": map[string]any{
			"authorization": "Bearer top-secret",
			"safe":          "ok",
		},
		"items": []any{
			map[string]any{"password": "pw-1"},
			"plain",
		},
	}))

	var payload map[string]any
	if err := json.Unmarshal(redacted, &payload); err != nil {
		t.Fatalf("unmarshal redacted payload failed: %v", err)
	}

	if payload["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key redacted, got %+v", payload["api_key"])
	}

	nested, ok := payload["nested"].(map[string]any)
	if !ok {
		t.Fatalf("expected nested object, got %+v", payload["nested"])
	}
	if nested["authorization"] != "[REDACTED]" {
		t.Fatalf("expected authorization redacted, got %+v", nested["authorization"])
	}
	if nested["safe"] != "ok" {
		t.Fatalf("expected safe field preserved, got %+v", nested["safe"])
	}

	items, ok := payload["items"].([]any)
	if !ok || len(items) != 2 {
		t.Fatalf("expected items array preserved, got %+v", payload["items"])
	}
	firstItem, ok := items[0].(map[string]any)
	if !ok {
		t.Fatalf("expected first item object, got %+v", items[0])
	}
	if firstItem["password"] != "[REDACTED]" {
		t.Fatalf("expected password redacted, got %+v", firstItem["password"])
	}
}

func TestLogActionIncludesStageAndStatus(t *testing.T) {
	tmpDir := t.TempDir()
	_, err := logging.Init(logging.Config{
		Service:       "process-test",
		Dir:           tmpDir,
		Level:         logging.ParseLevel("info"),
		RetentionDays: 30,
		ConsoleEnable: false,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 10, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("Init logging failed: %v", err)
	}

	LogAction("task-1", "run", "ERROR", map[string]any{
		"api_key": "sk-secret-123",
		"detail":  "boom",
	})

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

	if payload["msg"] != "legacy audit action" {
		t.Fatalf("expected legacy audit action msg, got %+v", payload["msg"])
	}
	if payload["stage"] != "run" {
		t.Fatalf("expected stage run, got %+v", payload["stage"])
	}
	if payload["status"] != "error" {
		t.Fatalf("expected status error, got %+v", payload["status"])
	}

	dataField, ok := payload["data"].(map[string]any)
	if !ok {
		t.Fatalf("expected data object, got %+v", payload["data"])
	}
	if dataField["api_key"] != "[REDACTED]" {
		t.Fatalf("expected api_key redacted, got %+v", dataField["api_key"])
	}
	if dataField["detail"] != "boom" {
		t.Fatalf("expected detail preserved, got %+v", dataField["detail"])
	}
}
