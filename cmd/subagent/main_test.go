package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/memory"
)

type providerErrorStub struct {
	err error
}

func (p *providerErrorStub) Predict(ctx context.Context, req llm.InferenceRequest) (string, error) {
	return "", p.err
}

func (p *providerErrorStub) PredictStream(ctx context.Context, req llm.InferenceRequest) (io.ReadCloser, error) {
	return nil, nil
}

type stubRuntime struct {
	result llm.FinalResult
	err    error
}

func (s *stubRuntime) Run(ctx context.Context, req llm.InferenceRequest, fanout *llm.Fanout) (llm.FinalResult, error) {
	result := s.result
	if result.TraceID == "" {
		result.TraceID = req.TraceID
	}
	if result.TaskID == "" {
		result.TaskID = req.TaskID
	}
	return result, s.err
}

func TestParseConfigParsesAgentTypeAndTaskID(t *testing.T) {
	cfg, err := ParseConfig([]string{"--agent-type", "planner", "--task-id", "task-1"})
	if err != nil {
		t.Fatalf("parse config failed: %v", err)
	}
	if cfg.AgentType != "planner" {
		t.Fatalf("expected agent type planner, got %s", cfg.AgentType)
	}
	if cfg.TaskID != "task-1" {
		t.Fatalf("expected task id task-1, got %s", cfg.TaskID)
	}
}

func TestParseConfigRejectsMissingRequiredFlags(t *testing.T) {
	if _, err := ParseConfig([]string{"--agent-type", "planner"}); err == nil {
		t.Fatal("expected parse error when task-id is missing")
	}
}

func TestParseConfigDefaultsSQLiteDSN(t *testing.T) {
	cfg, err := ParseConfig([]string{"--agent-type", "planner", "--task-id", "task-1"})
	if err != nil {
		t.Fatalf("parse config failed: %v", err)
	}
	if cfg.SQLiteDSN != filepath.Join("var", "db", "agentic_core.db") {
		t.Fatalf("expected default sqlite dsn, got %s", cfg.SQLiteDSN)
	}
}

func TestResolveSQLiteDSNForOpen_DefaultUsesRuntimeRoot(t *testing.T) {
	tmp := t.TempDir()
	dsn, err := resolveSQLiteDSNForOpen(filepath.Join("var", "db", "agentic_core.db"), tmp)
	if err != nil {
		t.Fatalf("resolveSQLiteDSNForOpen returned error: %v", err)
	}

	want := filepath.Join(tmp, "var", "db", "agentic_core.db")
	if dsn != want {
		t.Fatalf("expected runtime-root resolved dsn %q, got %q", want, dsn)
	}
}

func TestResolveSQLiteDSNForOpen_ExplicitRelativeStaysCwdBased(t *testing.T) {
	tmp := t.TempDir()
	cwd := filepath.Join(tmp, "cwd")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("os.MkdirAll returned error: %v", err)
	}

	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd returned error: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("os.Chdir returned error: %v", err)
	}
	t.Cleanup(func() {
		_ = os.Chdir(wd)
	})

	dsn, err := resolveSQLiteDSNForOpen(filepath.Join("relative", "custom.db"), tmp)
	if err != nil {
		t.Fatalf("resolveSQLiteDSNForOpen returned error: %v", err)
	}

	want := filepath.Join("relative", "custom.db")
	if dsn != want {
		t.Fatalf("expected cwd-compatible relative dsn %q, got %q", want, dsn)
	}
}

func TestPrepareSQLiteDSNDir_CreatesParentForFilePath(t *testing.T) {
	tmp := t.TempDir()
	dsn := filepath.Join(tmp, "var", "db", "agentic_core.db")
	parent := filepath.Dir(dsn)

	if _, err := os.Stat(parent); !os.IsNotExist(err) {
		t.Fatalf("expected parent directory absent before prepare, stat err=%v", err)
	}

	if err := prepareSQLiteDSNDir(dsn); err != nil {
		t.Fatalf("prepareSQLiteDSNDir returned error: %v", err)
	}

	info, err := os.Stat(parent)
	if err != nil {
		t.Fatalf("expected parent directory created, stat err=%v", err)
	}
	if !info.IsDir() {
		t.Fatalf("expected parent path to be directory, got mode=%v", info.Mode())
	}
}

func TestPrepareSQLiteDSNDir_NoOpForMemoryDSN(t *testing.T) {
	if err := prepareSQLiteDSNDir(":memory:"); err != nil {
		t.Fatalf("expected no error for memory dsn, got %v", err)
	}
}

func TestNewSubagentReturnsErrorWhenSQLiteSchemaInitFails(t *testing.T) {
	cfg := Config{
		AgentType:  "planner",
		TaskID:     "task-1",
		RedisAddr:  "skip",
		MQTTBroker: "skip",
		SQLiteDSN:  "file:history-schema-readonly?mode=memory&cache=shared&_pragma=query_only(1)",
	}

	_, err := NewSubagent(context.Background(), cfg)
	if err == nil {
		t.Fatal("expected sqlite schema init error, got nil")
	}
}

func TestNewSubagentInitializesBuses(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}
	if agent.queue == nil {
		t.Fatal("expected queue initialized")
	}
	if agent.events == nil {
		t.Fatal("expected event bus initialized")
	}
	if agent.heartbeat == nil {
		t.Fatal("expected heartbeat initialized")
	}
	if agent.logger == nil {
		t.Fatal("expected logger initialized")
	}
}

func TestSubagentRunReturnsCanceledContextError(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if err := agent.Run(ctx); err != context.Canceled {
		t.Fatalf("expected context canceled, got %v", err)
	}
}

func TestSubagentRunPublishesHeartbeat(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()

	// Run in background and wait for heartbeats
	go func() {
		_ = agent.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)

	// Check heartbeat
	status, ok := agent.heartbeat.(*bus.FakeHeartbeatBus).LastStatus("task-1")
	if !ok || status != "running" {
		t.Fatalf("expected running heartbeat for task-1, got %v %v", status, ok)
	}
}

func TestMainParsesAndRunsSubagent(t *testing.T) {
	// Simple sanity check that it starts and can be cancelled
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	os.Args = []string{"subagent", "--agent-type", "planner", "--task-id", "task-main", "--redis-addr", "skip", "--mqtt-broker", "skip"}
	// We don't call main() because it uses os.Exit
	cfg, _ := ParseConfig(os.Args[1:])
	s, _ := NewSubagent(ctx, cfg)
	err := s.Run(ctx)

	if err != nil && err != context.DeadlineExceeded {
		t.Fatalf("expected timeout, got %v", err)
	}
}

func TestSubagentPublishChunkPublishesEvent(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	transport, ok := agent.events.(*bus.FakeTransport)
	if !ok {
		t.Fatal("expected fake transport")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs, err := transport.Subscribe(ctx, "chunks.task-1")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	chunk := llm.StreamChunk{
		TraceID:     "trace-1",
		SessionID:   "session-1",
		TaskID:      "task-1",
		Sequence:    7,
		Event:       "tool_result",
		ToolName:    "http_get",
		TimestampMs: time.Now().UnixMilli(),
	}
	if err := agent.publishChunk(ctx, chunk); err != nil {
		t.Fatalf("publishChunk failed: %v", err)
	}

	select {
	case msg := <-msgs:
		if msg.MessageID != "chunk.task-1.7" {
			t.Fatalf("unexpected message id: %s", msg.MessageID)
		}
		if msg.SenderID != "task-1" {
			t.Fatalf("unexpected sender id: %s", msg.SenderID)
		}
		if msg.ReceiverID != "sender" {
			t.Fatalf("unexpected receiver id: %s", msg.ReceiverID)
		}

		var got llm.StreamChunk
		if err := json.Unmarshal(msg.Payload, &got); err != nil {
			t.Fatalf("unmarshal chunk failed: %v", err)
		}
		if got.TaskID != chunk.TaskID || got.Sequence != chunk.Sequence || got.Event != chunk.Event {
			t.Fatalf("unexpected chunk payload: %+v", got)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for chunk event")
	}
}

func TestSubagentPublishChunkPublishesAuditEvent(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	transport, ok := agent.events.(*bus.FakeTransport)
	if !ok {
		t.Fatal("expected fake transport")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	msgs, err := transport.Subscribe(ctx, "audit.task-1")
	if err != nil {
		t.Fatalf("subscribe failed: %v", err)
	}

	chunk := llm.StreamChunk{
		TraceID:     "trace-1",
		SessionID:   "session-1",
		TaskID:      "task-1",
		Sequence:    7,
		Event:       "tool_result",
		ToolName:    "http_get",
		TimestampMs: time.Now().UnixMilli(),
		Data:        json.RawMessage(`{"result":"ok"}`),
	}
	if err := agent.publishChunk(ctx, chunk); err != nil {
		t.Fatalf("publishChunk failed: %v", err)
	}

	select {
	case msg := <-msgs:
		var event llm.AuditEvent
		if err := json.Unmarshal(msg.Payload, &event); err != nil {
			t.Fatalf("unmarshal audit event failed: %v", err)
		}
		if event.TaskID != "task-1" || event.Event != "tool_result" {
			t.Fatalf("unexpected audit event: %+v", event)
		}
		if event.Actor != "task-1" {
			t.Fatalf("expected actor task-1, got %s", event.Actor)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for audit event")
	}
}

func TestSubagentRunPreservesRuntimeTimeoutStatusInTaskResult(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip", LLMProvider: "stub"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	agent.resolver.Register("stub", &providerErrorStub{err: context.DeadlineExceeded})
	provider, ok := agent.resolver.Get("stub")
	if !ok {
		t.Fatal("expected stub provider registered")
	}
	agent.runtime = llm.NewRuntime(provider, nil, nil)

	transport, ok := agent.queue.(*bus.FakeTransport)
	if !ok {
		t.Fatal("expected fake transport")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results, err := transport.Dequeue(ctx, "task_results")
	if err != nil {
		t.Fatalf("dequeue results failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task.task-1", bus.Message{
		MessageID:  "task-msg-1",
		SenderID:   "orchestrator",
		ReceiverID: "task-1",
		Payload:    json.RawMessage(`{"task":"do work"}`),
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx)
	}()

	select {
	case msg := <-results:
		var result bus.TaskResult
		if err := json.Unmarshal(msg.Payload, &result); err != nil {
			t.Fatalf("unmarshal task result failed: %v", err)
		}
		if result.Status != "timeout" {
			t.Fatalf("expected timeout status, got %s", result.Status)
		}
		if result.Error == "" {
			t.Fatal("expected timeout error message preserved")
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for task result")
	}

	cancel()
	<-done
}

func TestSubagentRunPreservesRuntimeRejectedStatusInTaskResult(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	agent.runtime = &stubRuntime{
		result: llm.FinalResult{Status: "REJECTED", Content: "denied"},
		err:    errors.New("approval rejected"),
	}

	transport, ok := agent.queue.(*bus.FakeTransport)
	if !ok {
		t.Fatal("expected fake transport")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results, err := transport.Dequeue(ctx, "task_results")
	if err != nil {
		t.Fatalf("dequeue results failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task.task-1", bus.Message{
		MessageID:  "task-msg-1",
		SenderID:   "orchestrator",
		ReceiverID: "task-1",
		Payload:    json.RawMessage(`{"task":"do work"}`),
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx)
	}()

	select {
	case msg := <-results:
		var result bus.TaskResult
		if err := json.Unmarshal(msg.Payload, &result); err != nil {
			t.Fatalf("unmarshal task result failed: %v", err)
		}
		if result.Status != memory.TaskStatusRejected {
			t.Fatalf("expected rejected status, got %s", result.Status)
		}
		if result.Error != "approval rejected" {
			t.Fatalf("expected rejection error preserved, got %q", result.Error)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for task result")
	}

	cancel()
	<-done
}

func TestSubagentRunPreservesRuntimeCancelledStatusInTaskResult(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	agent.runtime = &stubRuntime{
		result: llm.FinalResult{Status: "CaNcElLeD", Content: "stopped"},
		err:    context.Canceled,
	}

	transport, ok := agent.queue.(*bus.FakeTransport)
	if !ok {
		t.Fatal("expected fake transport")
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	results, err := transport.Dequeue(ctx, "task_results")
	if err != nil {
		t.Fatalf("dequeue results failed: %v", err)
	}

	if err := transport.Enqueue(ctx, "task.task-1", bus.Message{
		MessageID:  "task-msg-1",
		SenderID:   "orchestrator",
		ReceiverID: "task-1",
		Payload:    json.RawMessage(`{"task":"do work"}`),
		Timestamp:  time.Now().UnixMilli(),
	}); err != nil {
		t.Fatalf("enqueue task failed: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx)
	}()

	select {
	case msg := <-results:
		var result bus.TaskResult
		if err := json.Unmarshal(msg.Payload, &result); err != nil {
			t.Fatalf("unmarshal task result failed: %v", err)
		}
		if result.Status != memory.TaskStatusCancelled {
			t.Fatalf("expected cancelled status, got %s", result.Status)
		}
		if result.Error != context.Canceled.Error() {
			t.Fatalf("expected cancellation error preserved, got %q", result.Error)
		}
	case <-ctx.Done():
		t.Fatal("timed out waiting for task result")
	}

	cancel()
	<-done
}
