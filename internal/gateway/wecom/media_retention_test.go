package wecom

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestCleanupExpiredMediaFilesRemovesExpiredFiles(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	now := time.Date(2026, 3, 18, 22, 0, 0, 0, time.UTC)

	expiredPath := filepath.Join(dir, "expired.dat")
	if err := os.WriteFile(expiredPath, []byte("expired"), 0o644); err != nil {
		t.Fatalf("write expired file failed: %v", err)
	}
	if err := os.Chtimes(expiredPath, now.Add(-72*time.Hour), now.Add(-72*time.Hour)); err != nil {
		t.Fatalf("set expired file time failed: %v", err)
	}

	freshPath := filepath.Join(dir, "fresh.dat")
	if err := os.WriteFile(freshPath, []byte("fresh"), 0o644); err != nil {
		t.Fatalf("write fresh file failed: %v", err)
	}
	if err := os.Chtimes(freshPath, now.Add(-2*time.Hour), now.Add(-2*time.Hour)); err != nil {
		t.Fatalf("set fresh file time failed: %v", err)
	}

	removed, err := CleanupExpiredMediaFiles(dir, 24*time.Hour, now)
	if err != nil {
		t.Fatalf("CleanupExpiredMediaFiles failed: %v", err)
	}
	if removed != 1 {
		t.Fatalf("expected 1 expired file removed, got %d", removed)
	}
	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Fatalf("expected expired file removed, stat err=%v", err)
	}
	if _, err := os.Stat(freshPath); err != nil {
		t.Fatalf("expected fresh file kept, stat err=%v", err)
	}
}

func TestStartMediaRetentionRunsImmediateSweep(t *testing.T) {
	t.Helper()

	dir := t.TempDir()
	now := time.Date(2026, 3, 18, 22, 0, 0, 0, time.UTC)

	expiredPath := filepath.Join(dir, "expired-now.dat")
	if err := os.WriteFile(expiredPath, []byte("expired"), 0o644); err != nil {
		t.Fatalf("write expired file failed: %v", err)
	}
	if err := os.Chtimes(expiredPath, now.Add(-48*time.Hour), now.Add(-48*time.Hour)); err != nil {
		t.Fatalf("set expired file time failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	StartMediaRetention(ctx, MediaRetentionConfig{
		MediaDir:      dir,
		Retention:     24 * time.Hour,
		SweepInterval: time.Hour,
		Now:           func() time.Time { return now },
	})

	if _, err := os.Stat(expiredPath); !os.IsNotExist(err) {
		t.Fatalf("expected immediate sweep to remove expired file, stat err=%v", err)
	}
}
