package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"sync"
)

type Manager struct {
	service string
	root    *slog.Logger
	closers []io.Closer
}

var (
	managerMu      sync.RWMutex
	currentManager *Manager
)

func NewManager(cfg Config) (*Manager, error) {
	service := serviceName(cfg.Service)
	level := cfg.Level
	if cfg.Now == nil {
		cfg.Now = DefaultConfig(service).Now
	}
	if cfg.Dir == "" {
		cfg.Dir = DefaultConfig(service).Dir
	}
	if cfg.RetentionDays <= 0 {
		cfg.RetentionDays = defaultRetentionDays
	}
	if cfg.ConsoleWriter == nil {
		cfg.ConsoleWriter = os.Stderr
	}

	var handlers []slog.Handler
	var closers []io.Closer

	if cfg.ConsoleEnable {
		handlers = append(handlers, newConsoleHandler(cfg.ConsoleWriter, level))
	}
	if cfg.FileEnable {
		fileWriter := NewDailyWriter(DailyWriterConfig{
			Dir:           cfg.Dir,
			Service:       service,
			RetentionDays: cfg.RetentionDays,
			Now:           cfg.Now,
		})
		handlers = append(handlers, slog.NewJSONHandler(fileWriter, &slog.HandlerOptions{Level: level}))
		closers = append(closers, fileWriter)
	}
	if len(handlers) == 0 {
		handlers = append(handlers, newConsoleHandler(io.Discard, level))
	}

	root := slog.New(newTeeHandler(handlers...)).With("service", service)
	return &Manager{
		service: service,
		root:    root,
		closers: closers,
	}, nil
}

func Init(cfg Config) (*Manager, error) {
	manager, err := NewManager(cfg)
	if err != nil {
		return nil, err
	}

	managerMu.Lock()
	defer managerMu.Unlock()
	if currentManager != nil {
		_ = currentManager.Close()
	}
	currentManager = manager
	slog.SetDefault(manager.Component("app"))
	return manager, nil
}

func Default() *Manager {
	managerMu.RLock()
	manager := currentManager
	managerMu.RUnlock()
	if manager != nil {
		return manager
	}

	fallback, _ := NewManager(Config{
		Service:       defaultServiceFileName,
		Level:         slog.LevelInfo,
		ConsoleWriter: os.Stderr,
		ConsoleEnable: true,
		FileEnable:    false,
	})
	managerMu.Lock()
	if currentManager == nil {
		currentManager = fallback
	}
	manager = currentManager
	managerMu.Unlock()
	return manager
}

func Component(name string) *slog.Logger {
	return Default().Component(name)
}

func (m *Manager) Component(name string) *slog.Logger {
	if m == nil {
		return Default().Component(name)
	}
	if name == "" {
		return m.root
	}
	return m.root.With("component", name)
}

func (m *Manager) Close() error {
	if m == nil {
		return nil
	}
	var firstErr error
	for _, closer := range m.closers {
		if err := closer.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

type teeHandler struct {
	handlers []slog.Handler
}

func newTeeHandler(handlers ...slog.Handler) slog.Handler {
	filtered := make([]slog.Handler, 0, len(handlers))
	for _, handler := range handlers {
		if handler != nil {
			filtered = append(filtered, handler)
		}
	}
	return teeHandler{handlers: filtered}
}

func (h teeHandler) Enabled(ctx context.Context, level slog.Level) bool {
	for _, handler := range h.handlers {
		if handler.Enabled(ctx, level) {
			return true
		}
	}
	return false
}

func (h teeHandler) Handle(ctx context.Context, record slog.Record) error {
	var firstErr error
	for _, handler := range h.handlers {
		if !handler.Enabled(ctx, record.Level) {
			continue
		}
		if err := handler.Handle(ctx, record.Clone()); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

func (h teeHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithAttrs(attrs))
	}
	return teeHandler{handlers: handlers}
}

func (h teeHandler) WithGroup(name string) slog.Handler {
	handlers := make([]slog.Handler, 0, len(h.handlers))
	for _, handler := range h.handlers {
		handlers = append(handlers, handler.WithGroup(name))
	}
	return teeHandler{handlers: handlers}
}
