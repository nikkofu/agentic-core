package logging

import (
	"io"
	"os"
	"path/filepath"
	"sync"
	"time"
)

type DailyWriterConfig struct {
	Dir           string
	Service       string
	RetentionDays int
	Now           func() time.Time
}

type DailyWriter struct {
	mu          sync.Mutex
	dir         string
	service     string
	retention   int
	now         func() time.Time
	currentDate string
	file        *os.File
}

func NewDailyWriter(cfg DailyWriterConfig) *DailyWriter {
	retention := cfg.RetentionDays
	if retention <= 0 {
		retention = defaultRetentionDays
	}

	nowFn := cfg.Now
	if nowFn == nil {
		nowFn = time.Now
	}

	return &DailyWriter{
		dir:       cfg.Dir,
		service:   serviceName(cfg.Service),
		retention: retention,
		now:       nowFn,
	}
}

func (w *DailyWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()

	if err := w.rotateLocked(); err != nil {
		return 0, err
	}
	return w.file.Write(p)
}

func (w *DailyWriter) Close() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.file == nil {
		return nil
	}
	err := w.file.Close()
	w.file = nil
	return err
}

func (w *DailyWriter) rotateLocked() error {
	now := w.now().UTC()
	date := now.Format("2006-01-02")
	if err := w.cleanupExpiredLocked(now); err != nil {
		return err
	}
	if w.file != nil && w.currentDate == date {
		return nil
	}
	if w.file != nil {
		if err := w.file.Close(); err != nil {
			return err
		}
		w.file = nil
	}

	dir := filepath.Join(w.dir, date)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}

	file, err := os.OpenFile(filepath.Join(dir, w.service+".jsonl"), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}

	w.currentDate = date
	w.file = file
	return nil
}

func (w *DailyWriter) cleanupExpiredLocked(now time.Time) error {
	entries, err := os.ReadDir(w.dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}

	cutoff := dateOnly(now).AddDate(0, 0, -(w.retention - 1))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dirDate, err := time.Parse("2006-01-02", entry.Name())
		if err != nil {
			continue
		}
		if dirDate.Before(cutoff) {
			if err := os.RemoveAll(filepath.Join(w.dir, entry.Name())); err != nil {
				return err
			}
		}
	}

	return nil
}

func dateOnly(t time.Time) time.Time {
	year, month, day := t.UTC().Date()
	return time.Date(year, month, day, 0, 0, 0, 0, time.UTC)
}

var _ io.WriteCloser = (*DailyWriter)(nil)
