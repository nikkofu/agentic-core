package main

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"agentic-core/internal/bus"
)

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

func TestNewSubagentInitializesBuses(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}
	if agent.pubsub == nil {
		t.Fatal("expected pubsub initialized")
	}
	if agent.heartbeat == nil {
		t.Fatal("expected heartbeat initialized")
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

func TestSubagentRunPublishesEventAndHeartbeat(t *testing.T) {
	cfg := Config{AgentType: "planner", TaskID: "task-1", RedisAddr: "skip", MQTTBroker: "skip"}
	agent, err := NewSubagent(context.Background(), cfg)
	if err != nil {
		t.Fatalf("new subagent failed: %v", err)
	}

	// Task channel must have a message to avoid blocking
	msg := bus.Message{
		MessageID:  "task-1",
		SenderID:   "orchestrator",
		ReceiverID: "task-1",
		Payload:    json.RawMessage(`{"cmd":"echo hello"}`),
		Timestamp:  time.Now().UnixMilli(),
	}
	_ = agent.pubsub.Publish(context.Background(), "task.task-1", msg)

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	if err := agent.Run(ctx); err != nil && err != context.DeadlineExceeded {
		t.Fatalf("expected run success, got %v", err)
	}

	// Check start event
	eventMsg, err := agent.pubsub.Consume(context.Background(), "subagent.events")
	if err != nil {
		t.Fatalf("expected start event, got %v", err)
	}
	
	var payload struct {
		Status    string `json:"status"`
		AgentType string `json:"agent_type"`
	}
	if err := json.Unmarshal(eventMsg.Payload, &payload); err != nil {
		t.Fatalf("unmarshal event payload failed: %v", err)
	}
	if payload.Status != "started" || payload.AgentType != "planner" {
		t.Fatalf("unexpected payload: %+v", payload)
	}

	// Check heartbeat
	status, ok := agent.heartbeat.(*bus.FakeHeartbeatBus).LastStatus("task-1")
	if !ok || status != "running" {
		t.Fatalf("expected running heartbeat for task-1, got %v %v", status, ok)
	}
}

func TestRunMainParsesAndRunsSubagent(t *testing.T) {
	// Use a short timeout to prevent blocking indefinitely
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := runMain(ctx, []string{"--agent-type", "planner", "--task-id", "task-main", "--redis-addr", "skip", "--mqtt-broker", "skip"})
	// We expect timeout because it will block on consuming task
	if err != nil && err != context.DeadlineExceeded && !errors.Is(err, bus.ErrNoMessage) {
		t.Fatalf("expected timeout or ErrNoMessage, got %v", err)
	}
}
