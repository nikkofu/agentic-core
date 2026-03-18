package skill

import (
	"agentic-core/internal/bus"
	"agentic-core/internal/llm"
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestApprovalGate(t *testing.T) {
	transport := bus.NewFakeTransport()
	gate := NewApprovalGate(transport)
	ctx := context.Background()

	req := llm.ApprovalRequest{
		TraceID:       "trace-1",
		TaskID:        "task-1",
		ToolCallID:    "call-1",
		ToolName:      "write_file",
		Arguments:     json.RawMessage(`{"path":"a.txt"}`),
		RequestedAtMs: time.Now().UnixMilli(),
	}

	t.Run("approves successfully", func(t *testing.T) {
		decision := llm.ApprovalDecision{
			TraceID:     "trace-1",
			TaskID:      "task-1",
			ToolCallID:  "call-1",
			Approved:    true,
			DecidedAtMs: time.Now().UnixMilli(),
		}
		payload, _ := json.Marshal(decision)

		// 异步发送决策
		go func() {
			time.Sleep(100 * time.Millisecond)
			_ = transport.Publish(ctx, "approvals", bus.Message{
				MessageID:  "approval.task-1",
				SenderID:   "orchestrator",
				ReceiverID: "task-1",
				Payload:    payload,
				Timestamp:  time.Now().UnixMilli(),
			})
		}()

		got, err := gate.WaitDecision(ctx, req, 1*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !got.Approved {
			t.Error("expected approved=true")
		}
	})

	t.Run("times out", func(t *testing.T) {
		_, err := gate.WaitDecision(ctx, req, 100*time.Millisecond)
		if err == nil || err.Error() != "approval timeout" {
			t.Errorf("expected timeout error, got: %v", err)
		}
	})

	t.Run("ignores mismatched tool call", func(t *testing.T) {
		go func() {
			time.Sleep(25 * time.Millisecond)
			wrongPayload, _ := json.Marshal(llm.ApprovalDecision{
				TraceID:     "trace-1",
				TaskID:      "task-1",
				ToolCallID:  "other-call",
				Approved:    true,
				DecidedAtMs: time.Now().UnixMilli(),
			})
			_ = transport.Publish(ctx, "approvals", bus.Message{
				MessageID:  "approval.task-1.other-call",
				SenderID:   "orchestrator",
				ReceiverID: "task-1",
				Payload:    wrongPayload,
				Timestamp:  time.Now().UnixMilli(),
			})

			time.Sleep(25 * time.Millisecond)
			rightPayload, _ := json.Marshal(llm.ApprovalDecision{
				TraceID:     "trace-1",
				TaskID:      "task-1",
				ToolCallID:  "call-1",
				Approved:    false,
				Reason:      "rejected",
				DecidedAtMs: time.Now().UnixMilli(),
			})
			_ = transport.Publish(ctx, "approvals", bus.Message{
				MessageID:  "approval.task-1.call-1",
				SenderID:   "orchestrator",
				ReceiverID: "task-1",
				Payload:    rightPayload,
				Timestamp:  time.Now().UnixMilli(),
			})
		}()

		got, err := gate.WaitDecision(ctx, req, 1*time.Second)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got.ToolCallID != "call-1" {
			t.Fatalf("expected matched tool call id, got %s", got.ToolCallID)
		}
		if got.Approved {
			t.Fatal("expected final matched decision to be rejected")
		}
	})

	t.Run("late decision after timeout leaves no active subscriber", func(t *testing.T) {
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			if transport.SubscriberCount() == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if got := transport.SubscriberCount(); got != 0 {
			t.Fatalf("expected no active subscribers before wait, got %d", got)
		}

		_, err := gate.WaitDecision(ctx, req, 25*time.Millisecond)
		if err == nil || err.Error() != "approval timeout" {
			t.Fatalf("expected timeout error, got %v", err)
		}

		deadline = time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			if transport.SubscriberCount() == 0 {
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if got := transport.SubscriberCount(); got != 0 {
			t.Fatalf("expected subscriber cleanup after timeout, got %d", got)
		}

		latePayload, _ := json.Marshal(llm.ApprovalDecision{
			TraceID:     "trace-1",
			TaskID:      "task-1",
			ToolCallID:  "call-1",
			Approved:    true,
			DecidedAtMs: time.Now().UnixMilli(),
		})
		if err := transport.Publish(ctx, "approvals", bus.Message{
			MessageID:  "approval.task-1.call-1.late",
			SenderID:   "orchestrator",
			ReceiverID: "task-1",
			Payload:    latePayload,
			Timestamp:  time.Now().UnixMilli(),
		}); err != nil {
			t.Fatalf("publish late decision failed: %v", err)
		}

		if got := transport.SubscriberCount(); got != 0 {
			t.Fatalf("expected late decision to have no active listener, got %d", got)
		}
	})
}
