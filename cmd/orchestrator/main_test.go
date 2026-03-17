package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"agentic-core/internal/bus"
)

func TestParseAppConfigDefaultsSQLiteDSN(t *testing.T) {
	cfg, err := ParseAppConfig(nil)
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.SQLiteDSN != "agentic_core.db" {
		t.Fatalf("expected default sqlite dsn, got %s", cfg.SQLiteDSN)
	}
}

func TestParseAppConfigAcceptsCustomSQLiteDSN(t *testing.T) {
	cfg, err := ParseAppConfig([]string{"--sqlite-dsn", ":memory:"})
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.SQLiteDSN != ":memory:" {
		t.Fatalf("expected custom sqlite dsn, got %s", cfg.SQLiteDSN)
	}
}

func TestNewAppInitializesCoreComponents(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}
	if app == nil {
		t.Fatal("expected app instance")
	}
	if app.taskStore == nil {
		t.Fatal("expected task store to be initialized")
	}
	if app.pubsub == nil {
		t.Fatal("expected pubsub to be initialized")
	}
	if app.heartbeat == nil {
		t.Fatal("expected heartbeat bus to be initialized")
	}
}

func TestAppRunReturnsContextErrorWhenCanceled(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := app.Run(ctx); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestAppRunPerformsBusHealthPublishConsume(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	if err := app.Run(ctx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("expected run success or timeout, got %v", err)
	}

	msg, err := app.pubsub.Consume(context.Background(), "system.health")
	if err != nil {
		t.Fatalf("expected health message, got %v", err)
	}
	if msg.MessageID != "orchestrator.health" {
		t.Fatalf("unexpected health message id: %s", msg.MessageID)
	}

	status, ok := app.heartbeat.(*bus.FakeHeartbeatBus).LastStatus("orchestrator")
	if !ok || status != "running" {
		t.Fatalf("expected orchestrator running heartbeat, got %v %v", status, ok)
	}
}

func TestProcessOneTaskSpawnsWorkflow(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	msg := bus.Message{
		MessageID:  "task-1",
		SenderID:   "user",
		ReceiverID: "orchestrator",
		Payload:    json.RawMessage(`{"task":"demo"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	if err := app.pubsub.Publish(context.Background(), "tasks", msg); err != nil {
		t.Fatalf("publish task failed: %v", err)
	}

	// We don't want to actually spawn processes in tests
	app.proc = &fakeProcessManager{}

	// Start processing in a goroutine because ProcessOneTask might block depending on workflow
	go func() {
		_ = app.ProcessOneTask(context.Background())
	}()

	// Wait for workflow to be registered
	time.Sleep(100 * time.Millisecond)
	
	app.mu.RLock()
	_, exists := app.workflows["task-1"]
	app.mu.RUnlock()
	
	if !exists {
		t.Fatal("expected workflow to be created for task-1")
	}
}

type fakeProcessManager struct{}
func (f *fakeProcessManager) SpawnAgent(ctx context.Context, agentType string, taskID string, extraArgs ...string) (int, error) {
	return 123, nil
}
func (f *fakeProcessManager) KillAgent(pid int) error {
	return nil
}

func TestProcessOneTaskReturnsErrNoMessageWhenQueueEmpty(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	err = app.ProcessOneTask(context.Background())
	if !errors.Is(err, bus.ErrNoMessage) {
		t.Fatalf("expected ErrNoMessage, got %v", err)
	}
}

func TestProcessLoopStopsOnContextCancel(t *testing.T) {
	cfg := AppConfig{SQLiteDSN: ":memory:", RedisAddr: "skip", MQTTBroker: "skip"}
	app, err := NewApp(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new app failed: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := app.ProcessLoop(ctx); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}


func TestParseAppConfigDefaultLoopDisabled(t *testing.T) {
	cfg, err := ParseAppConfig(nil)
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if cfg.Loop {
		t.Fatal("expected loop mode disabled by default")
	}
}

func TestParseAppConfigAcceptsLoopFlag(t *testing.T) {
	cfg, err := ParseAppConfig([]string{"--loop"})
	if err != nil {
		t.Fatalf("parse app config failed: %v", err)
	}
	if !cfg.Loop {
		t.Fatal("expected loop mode enabled")
	}
}

func TestRunMainLoopReturnsCanceledOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := runMain(ctx, []string{"--sqlite-dsn", ":memory:", "--redis-addr", "skip", "--mqtt-broker", "skip", "--loop"}); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}
