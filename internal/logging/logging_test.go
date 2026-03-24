package logging

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestDefaultLogDirUsesRuntimeRootVarLogs(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example.com/test\n"), 0o644); err != nil {
		t.Fatalf("write go.mod failed: %v", err)
	}

	nested := filepath.Join(root, "cmd", "orchestrator")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested cwd failed: %v", err)
	}
	t.Chdir(nested)

	t.Setenv("LOG_DIR", "")

	cfg := DefaultConfig("orchestrator")
	want := filepath.Join(root, "var", "logs")
	if cfg.Dir != want {
		t.Fatalf("expected default log dir %q, got %q", want, cfg.Dir)
	}
	if !filepath.IsAbs(cfg.Dir) {
		t.Fatalf("expected absolute default log dir, got %q", cfg.Dir)
	}
}

func TestDefaultConfigRespectsExplicitLogDirOverride(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "internal", "logging")
	if err := os.MkdirAll(nested, 0o755); err != nil {
		t.Fatalf("create nested cwd failed: %v", err)
	}
	t.Chdir(nested)

	override := "custom/logs"
	t.Setenv("LOG_DIR", override)

	cfg := DefaultConfig("subagent")
	if cfg.Dir != override {
		t.Fatalf("expected LOG_DIR override %q, got %q", override, cfg.Dir)
	}
}

func TestManagerWritesJSONLineToFile(t *testing.T) {
	tmpDir := t.TempDir()
	var console bytes.Buffer

	manager, err := NewManager(Config{
		Service:       "orchestrator",
		Dir:           tmpDir,
		Level:         ParseLevel("info"),
		RetentionDays: 30,
		ConsoleWriter: &console,
		ConsoleEnable: true,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	manager.Component("gateway").Info("task routed", "task_id", "task-1")

	logPath := filepath.Join(tmpDir, "2026-03-18", "orchestrator.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}

	var payload map[string]any
	if err := json.Unmarshal([]byte(lines[0]), &payload); err != nil {
		t.Fatalf("unmarshal log line failed: %v", err)
	}
	if payload["msg"] != "task routed" {
		t.Fatalf("unexpected message: %+v", payload)
	}
	if payload["component"] != "gateway" {
		t.Fatalf("expected component gateway, got %+v", payload["component"])
	}
	if payload["task_id"] != "task-1" {
		t.Fatalf("expected task_id task-1, got %+v", payload["task_id"])
	}
}

func TestManagerFiltersDebugBelowInfo(t *testing.T) {
	tmpDir := t.TempDir()

	manager, err := NewManager(Config{
		Service:       "subagent",
		Dir:           tmpDir,
		Level:         ParseLevel("info"),
		RetentionDays: 30,
		ConsoleWriter: &bytes.Buffer{},
		ConsoleEnable: true,
		FileEnable:    true,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	logger := manager.Component("subagent")
	logger.Debug("drop me")
	logger.Info("keep me")

	logPath := filepath.Join(tmpDir, "2026-03-18", "subagent.jsonl")
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log file failed: %v", err)
	}

	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line after level filtering, got %d", len(lines))
	}
	if !strings.Contains(lines[0], "keep me") {
		t.Fatalf("expected info line retained, got %s", lines[0])
	}
}

func TestComponentLoggerAddsComponentField(t *testing.T) {
	var console bytes.Buffer

	manager, err := NewManager(Config{
		Service:       "gateway",
		Dir:           t.TempDir(),
		Level:         ParseLevel("debug"),
		RetentionDays: 30,
		ConsoleWriter: &console,
		ConsoleEnable: true,
		FileEnable:    false,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})
	if err != nil {
		t.Fatalf("NewManager failed: %v", err)
	}
	t.Cleanup(func() { _ = manager.Close() })

	manager.Component("router").Info("incoming request", "session_id", "session-1")

	output := console.String()
	if !strings.Contains(output, "incoming request") {
		t.Fatalf("expected console output to include message, got %q", output)
	}
	if !strings.Contains(output, "component=router") {
		t.Fatalf("expected console output to include component field, got %q", output)
	}
}
