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
	ps := bus.NewFakePubSub()
	gate := NewApprovalGate(ps)
	ctx := context.Background()

	req := llm.ApprovalRequest{
		TaskID:     "task-1",
		ToolCallID: "call-1",
	}

	t.Run("approves successfully", func(t *testing.T) {
		decision := llm.ApprovalDecision{
			TaskID:     "task-1",
			ToolCallID: "call-1",
			Approved:   true,
		}
		payload, _ := json.Marshal(decision)

		// 异步发送决策
		go func() {
			time.Sleep(100 * time.Millisecond)
			ps.Publish(ctx, "approvals", bus.Message{Payload: payload})
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
}
