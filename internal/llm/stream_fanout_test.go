package llm

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestFanoutSequence(t *testing.T) {
	f := NewFanout("trace-1", "session-1", "task-1")

	c1 := f.NewChunk("delta")
	if c1.Sequence != 1 {
		t.Errorf("expected sequence 1, got %d", c1.Sequence)
	}

	c2 := f.NewChunk("delta")
	if c2.Sequence != 2 {
		t.Errorf("expected sequence 2, got %d", c2.Sequence)
	}

	if c1.TraceID != "trace-1" || c1.TaskID != "task-1" {
		t.Errorf("chunk metadata mismatch")
	}
}

func TestFanoutPublisherEmitsTypedChunks(t *testing.T) {
	var chunks []StreamChunk

	f := NewFanout("trace-1", "session-1", "task-1")
	f.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	call := ToolCall{
		ID:        "call-1",
		Name:      "lookup",
		Arguments: json.RawMessage(`{"city":"shanghai"}`),
	}
	if err := f.EmitToolCall(context.Background(), call); err != nil {
		t.Fatalf("EmitToolCall failed: %v", err)
	}

	result := ToolResult{
		ToolCallID: "call-1",
		Name:       "lookup",
		Success:    false,
		Output:     "denied",
		Error:      "approval rejected",
	}
	if err := f.EmitToolResult(context.Background(), result); err != nil {
		t.Fatalf("EmitToolResult failed: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected 2 chunks, got %d", len(chunks))
	}
	if chunks[0].Event != "tool_call" {
		t.Fatalf("expected first chunk event tool_call, got %s", chunks[0].Event)
	}
	if chunks[0].Role != "assistant" {
		t.Fatalf("expected tool call role assistant, got %s", chunks[0].Role)
	}
	if chunks[0].ToolName != "lookup" {
		t.Fatalf("expected tool name lookup, got %s", chunks[0].ToolName)
	}

	var gotCall ToolCall
	if err := json.Unmarshal(chunks[0].Data, &gotCall); err != nil {
		t.Fatalf("unmarshal tool call failed: %v", err)
	}
	if gotCall.ID != call.ID || gotCall.Name != call.Name {
		t.Fatalf("unexpected tool call payload: %+v", gotCall)
	}

	if chunks[1].Event != "tool_result" {
		t.Fatalf("expected second chunk event tool_result, got %s", chunks[1].Event)
	}
	if chunks[1].Error != "approval rejected" {
		t.Fatalf("expected tool result error propagated, got %s", chunks[1].Error)
	}

	var gotResult ToolResult
	if err := json.Unmarshal(chunks[1].Data, &gotResult); err != nil {
		t.Fatalf("unmarshal tool result failed: %v", err)
	}
	if gotResult.ToolCallID != result.ToolCallID || gotResult.Error != result.Error {
		t.Fatalf("unexpected tool result payload: %+v", gotResult)
	}
}

func TestFanoutOnlyEmitsFirstTerminalChunk(t *testing.T) {
	var chunks []StreamChunk

	f := NewFanout("trace-1", "session-1", "task-1")
	f.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	if err := f.EmitFinal(context.Background(), "done"); err != nil {
		t.Fatalf("EmitFinal failed: %v", err)
	}
	if err := f.EmitError(context.Background(), "boom"); err != nil {
		t.Fatalf("EmitError failed: %v", err)
	}

	if len(chunks) != 1 {
		t.Fatalf("expected only one terminal chunk, got %d", len(chunks))
	}
	if chunks[0].Event != "final" {
		t.Fatalf("expected terminal event final, got %s", chunks[0].Event)
	}
	if !chunks[0].Done {
		t.Fatal("expected terminal chunk marked done")
	}
}

func TestFanoutWaitingApprovalAndTerminalUniqueness(t *testing.T) {
	var chunks []StreamChunk

	f := NewFanout("trace-1", "session-1", "task-1")
	f.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	req := ApprovalRequest{
		TraceID:       "trace-1",
		TaskID:        "task-1",
		ToolCallID:    "call-1",
		ToolName:      "write_file",
		Arguments:     json.RawMessage(`{"path":"a.txt"}`),
		RequestedAtMs: time.Now().UnixMilli(),
	}
	if err := f.EmitWaitingApproval(context.Background(), req); err != nil {
		t.Fatalf("EmitWaitingApproval failed: %v", err)
	}
	if err := f.EmitTimeout(context.Background(), "approval timeout"); err != nil {
		t.Fatalf("EmitTimeout failed: %v", err)
	}
	if err := f.EmitCancelled(context.Background(), "context canceled"); err != nil {
		t.Fatalf("EmitCancelled failed: %v", err)
	}

	if len(chunks) != 2 {
		t.Fatalf("expected waiting + first terminal only, got %d", len(chunks))
	}
	if chunks[0].Event != "waiting_approval" {
		t.Fatalf("expected waiting_approval event, got %s", chunks[0].Event)
	}
	if chunks[1].Event != "timeout" {
		t.Fatalf("expected first terminal event timeout, got %s", chunks[1].Event)
	}
	if !chunks[1].Done {
		t.Fatal("expected timeout chunk to be terminal")
	}
}
