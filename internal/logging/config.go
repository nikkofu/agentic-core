package logging

import (
	"io"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultLogDir          = "logs"
	defaultRetentionDays   = 30
	defaultServiceFileName = "agentic-core"
)

type Config struct {
	Service       string
	Dir           string
	Level         slog.Level
	RetentionDays int
	ConsoleWriter io.Writer
	ConsoleEnable bool
	FileEnable    bool
	Now           func() time.Time
}

func DefaultConfig(service string) Config {
	retention := defaultRetentionDays
	if raw := strings.TrimSpace(os.Getenv("LOG_RETENTION_DAYS")); raw != "" {
		if parsed, err := strconv.Atoi(raw); err == nil && parsed > 0 {
			retention = parsed
		}
	}

	dir := strings.TrimSpace(os.Getenv("LOG_DIR"))
	if dir == "" {
		dir = defaultLogDir
	}

	return Config{
		Service:       service,
		Dir:           dir,
		Level:         ParseLevel(os.Getenv("LOG_LEVEL")),
		RetentionDays: retention,
		ConsoleWriter: os.Stderr,
		ConsoleEnable: true,
		FileEnable:    true,
		Now:           time.Now,
	}
}

func ParseLevel(raw string) slog.Level {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "debug":
		return slog.LevelDebug
	case "warn", "warning":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	case "info", "":
		return slog.LevelInfo
	default:
		return slog.LevelInfo
	}
}

func serviceName(service string) string {
	service = strings.TrimSpace(service)
	if service == "" {
		return defaultServiceFileName
	}
	return service
}
