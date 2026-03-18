package process

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"agentic-core/internal/logging"
)

type AuditLog struct {
	Timestamp time.Time       `json:"timestamp"`
	AgentID   string          `json:"agent_id"`
	Action    string          `json:"action"` // "think", "call_skill", "delegate", "final"
	Payload   json.RawMessage `json:"payload"`
	Level     string          `json:"level"` // "INFO", "WARN", "ERROR"
}

type Auditor struct {
	events   bus.EventBus
	senderID string
	logger   *slog.Logger
}

func NewAuditor(events bus.EventBus, senderID string) *Auditor {
	return &Auditor{
		events:   events,
		senderID: senderID,
		logger:   logging.Component("audit").With("actor", senderID),
	}
}

func (a *Auditor) Record(ctx context.Context, event llm.AuditEvent) error {
	if a == nil {
		return nil
	}
	if event.Actor == "" {
		event.Actor = a.senderID
	}
	if event.Level == "" {
		event.Level = "INFO"
	}
	if event.TimestampMs == 0 {
		event.TimestampMs = time.Now().UnixMilli()
	}

	payload, err := json.Marshal(event)
	if err != nil {
		return err
	}

	if a.events != nil {
		topic := "audit"
		if event.TaskID != "" {
			topic = "audit." + event.TaskID
		}
		if err := a.events.Publish(ctx, topic, bus.Message{
			MessageID:  fmt.Sprintf("audit.%s.%s.%d", auditKey(event.TaskID), event.Event, event.TimestampMs),
			SenderID:   a.senderID,
			ReceiverID: "audit",
			Payload:    payload,
			Timestamp:  event.TimestampMs,
		}); err != nil {
			return err
		}
	}

	attrs := []slog.Attr{
		slog.String("event", event.Event),
		slog.String("task_id", event.TaskID),
		slog.String("parent_task_id", event.ParentTaskID),
		slog.String("tool_call_id", event.ToolCallID),
		slog.String("status", event.Status),
		slog.String("error", event.Error),
	}
	if event.TraceID != "" {
		attrs = append(attrs, slog.String("trace_id", event.TraceID))
	}
	if event.SessionID != "" {
		attrs = append(attrs, slog.String("session_id", event.SessionID))
	}
	if value, ok := decodeJSONValue(event.Data); ok {
		attrs = append(attrs, slog.Any("data", value))
	}

	a.logger.LogAttrs(ctx, levelFromString(event.Level), "audit event", attrs...)
	return nil
}

func auditKey(taskID string) string {
	if taskID == "" {
		return "global"
	}
	return taskID
}

func LogAction(agentID, action, level string, payload interface{}) {
	raw := redactJSON(mustJSON(payload))
	attrs := []slog.Attr{
		slog.String("event", action),
		slog.String("stage", action),
		slog.String("task_id", agentID),
		slog.String("status", legacyStatus(level)),
	}
	if value, ok := decodeJSONValue(raw); ok {
		attrs = append(attrs, slog.Any("data", value))
	}
	logging.Component("audit").With("actor", agentID).LogAttrs(context.Background(), levelFromString(level), "legacy audit action", attrs...)
}

func mustJSON(payload interface{}) json.RawMessage {
	data, _ := json.Marshal(payload)
	return data
}

func decodeJSONValue(raw json.RawMessage) (interface{}, bool) {
	if len(raw) == 0 || !json.Valid(raw) {
		return nil, false
	}
	var value interface{}
	if err := json.Unmarshal(raw, &value); err != nil {
		return nil, false
	}
	return value, true
}

func levelFromString(level string) slog.Level {
	switch level {
	case "DEBUG":
		return slog.LevelDebug
	case "WARN", "WARNING":
		return slog.LevelWarn
	case "ERROR":
		return slog.LevelError
	default:
		return slog.LevelInfo
	}
}
