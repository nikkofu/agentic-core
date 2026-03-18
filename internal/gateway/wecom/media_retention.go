package wecom

import (
	"context"
	"os"
	"path/filepath"
	"time"
)

const defaultMediaSweepInterval = time.Hour

type MediaRetentionConfig struct {
	MediaDir      string
	Retention     time.Duration
	SweepInterval time.Duration
	Now           func() time.Time
}

func StartMediaRetention(ctx context.Context, cfg MediaRetentionConfig) {
	cfg = cfg.normalize()
	if cfg.Retention <= 0 || cfg.MediaDir == "" {
		return
	}

	_, _ = CleanupExpiredMediaFiles(cfg.MediaDir, cfg.Retention, cfg.Now())

	ticker := time.NewTicker(cfg.SweepInterval)
	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				_, _ = CleanupExpiredMediaFiles(cfg.MediaDir, cfg.Retention, cfg.Now())
			}
		}
	}()
}

func CleanupExpiredMediaFiles(mediaDir string, retention time.Duration, now time.Time) (int, error) {
	if retention <= 0 || mediaDir == "" {
		return 0, nil
	}

	entries, err := os.ReadDir(mediaDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, nil
		}
		return 0, err
	}

	cutoff := now.Add(-retention)
	removed := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		info, err := entry.Info()
		if err != nil {
			return removed, err
		}
		if info.ModTime().After(cutoff) {
			continue
		}
		if err := os.Remove(filepath.Join(mediaDir, entry.Name())); err != nil {
			return removed, err
		}
		removed++
	}
	return removed, nil
}

func (c MediaRetentionConfig) normalize() MediaRetentionConfig {
	cfg := c
	if cfg.SweepInterval <= 0 {
		cfg.SweepInterval = defaultMediaSweepInterval
	}
	if cfg.Now == nil {
		cfg.Now = time.Now
	}
	return cfg
}
