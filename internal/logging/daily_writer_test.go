package logging

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDailyWriterSplitsByDate(t *testing.T) {
	tmpDir := t.TempDir()
	now := time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)

	writer := NewDailyWriter(DailyWriterConfig{
		Dir:           tmpDir,
		Service:       "orchestrator",
		RetentionDays: 30,
		Now: func() time.Time {
			return now
		},
	})

	if _, err := writer.Write([]byte(`{"msg":"first"}` + "\n")); err != nil {
		t.Fatalf("first write failed: %v", err)
	}

	firstPath := filepath.Join(tmpDir, "2026-03-18", "orchestrator.jsonl")
	if _, err := os.Stat(firstPath); err != nil {
		t.Fatalf("expected first daily log file: %v", err)
	}

	now = now.Add(24 * time.Hour)
	if _, err := writer.Write([]byte(`{"msg":"second"}` + "\n")); err != nil {
		t.Fatalf("second write failed: %v", err)
	}

	secondPath := filepath.Join(tmpDir, "2026-03-19", "orchestrator.jsonl")
	if _, err := os.Stat(secondPath); err != nil {
		t.Fatalf("expected second daily log file: %v", err)
	}
}

func TestDailyWriterRemovesExpiredDirectories(t *testing.T) {
	tmpDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(tmpDir, "2026-03-10"), 0o755); err != nil {
		t.Fatalf("mkdir old dir failed: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "2026-03-17"), 0o755); err != nil {
		t.Fatalf("mkdir fresh dir failed: %v", err)
	}

	writer := NewDailyWriter(DailyWriterConfig{
		Dir:           tmpDir,
		Service:       "subagent",
		RetentionDays: 3,
		Now: func() time.Time {
			return time.Date(2026, 3, 18, 9, 0, 0, 0, time.UTC)
		},
	})

	if _, err := writer.Write([]byte(`{"msg":"cleanup"}` + "\n")); err != nil {
		t.Fatalf("write failed: %v", err)
	}

	if _, err := os.Stat(filepath.Join(tmpDir, "2026-03-10")); !os.IsNotExist(err) {
		t.Fatalf("expected expired directory removed, got %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmpDir, "2026-03-17")); err != nil {
		t.Fatalf("expected retained directory to remain: %v", err)
	}
}

func TestParseLevelDefaultsToInfo(t *testing.T) {
	if got := ParseLevel(""); got != DefaultConfig("orchestrator").Level {
		t.Fatalf("expected empty level to default to info, got %v", got)
	}
	if got := ParseLevel("definitely-not-a-level"); got != DefaultConfig("orchestrator").Level {
		t.Fatalf("expected invalid level to default to info, got %v", got)
	}
}
