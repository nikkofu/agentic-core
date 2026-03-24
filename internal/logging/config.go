package logging

import (
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"agentic-core/internal/runtimepaths"
)

const (
	defaultLogDir          = "var/logs"
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
		dir = resolveDefaultLogDir(defaultLogDir)
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

func resolveDefaultLogDir(defaultDir string) string {
	cwd, err := os.Getwd()
	if err != nil {
		return defaultDir
	}

	runtimeRoot, err := runtimepaths.ResolveRuntimeRoot("", cwd)
	if err != nil {
		return defaultDir
	}

	return filepath.Join(runtimeRoot, filepath.FromSlash(defaultDir))
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
