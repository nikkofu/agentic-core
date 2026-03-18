package llm

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"
)

type mockProvider struct {
	responses []string
	index     int
}

func (m *mockProvider) Predict(ctx context.Context, req InferenceRequest) (string, error) {
	if m.index >= len(m.responses) {
		return "", fmt.Errorf("no more responses")
	}
	resp := m.responses[m.index]
	m.index++
	return resp, nil
}

func (m *mockProvider) PredictStream(ctx context.Context, req InferenceRequest) (io.ReadCloser, error) {
	return nil, nil
}

type mockExecutor struct {
	lastCall ToolCall
}

func (m *mockExecutor) Execute(ctx context.Context, call ToolCall) (ToolResult, error) {
	m.lastCall = call
	return ToolResult{
		ToolCallID: call.ID,
		Name:       call.Name,
		Success:    true,
		Output:     `{"result": "success"}`,
	}, nil
}

type failingProvider struct {
	err error
}

func (f *failingProvider) Predict(ctx context.Context, req InferenceRequest) (string, error) {
	return "", f.err
}

func (f *failingProvider) PredictStream(ctx context.Context, req InferenceRequest) (io.ReadCloser, error) {
	return nil, nil
}

type mockApprovalGate struct {
	wait func(ctx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalDecision, error)
}

func (m *mockApprovalGate) WaitDecision(ctx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalDecision, error) {
	return m.wait(ctx, req, timeout)
}

func TestRuntimeLoop_ToolThenFinal(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think": "need tool", "call_skill": {"id": "c1", "name": "test_tool", "arguments": "{}"}}`,
			`{"think": "got it", "final": "the end"}`,
		},
	}
	e := &mockExecutor{}
	r := NewRuntime(p, e, nil)

	req := InferenceRequest{
		TraceID: "t1",
		TaskID:  "task-1",
	}

	res, err := r.Run(context.Background(), req, nil)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if res.Content != "the end" {
		t.Errorf("expected final content 'the end', got %s", res.Content)
	}

	if e.lastCall.Name != "test_tool" {
		t.Errorf("expected test_tool to be called")
	}

	if p.index != 2 {
		t.Errorf("expected 2 LLM calls, got %d", p.index)
	}
}

func TestRuntimeLoopPublishesToolAndFinalChunks(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think": "need tool", "call_skill": {"id": "c1", "name": "test_tool", "arguments": "{}"}}`,
			`{"think": "got it", "final": "the end"}`,
		},
	}
	e := &mockExecutor{}
	r := NewRuntime(p, e, nil)

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	req := InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}

	res, err := r.Run(context.Background(), req, fanout)
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}
	if res.Content != "the end" {
		t.Fatalf("expected final content the end, got %s", res.Content)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}

	events := []string{chunks[0].Event, chunks[1].Event, chunks[2].Event}
	expected := []string{"tool_call", "tool_result", "final"}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if chunks[2].Done != true {
		t.Fatal("expected final chunk to be terminal")
	}
	if chunks[0].Sequence != 1 || chunks[1].Sequence != 2 || chunks[2].Sequence != 3 {
		t.Fatalf("unexpected chunk sequence numbers: %d %d %d", chunks[0].Sequence, chunks[1].Sequence, chunks[2].Sequence)
	}
}

func TestRuntimeLoopPublishesErrorChunkOnProviderFailure(t *testing.T) {
	r := NewRuntime(&failingProvider{err: errors.New("provider unavailable")}, &mockExecutor{}, nil)

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	_, err := r.Run(context.Background(), InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}, fanout)
	if err == nil {
		t.Fatal("expected provider error")
	}
	if len(chunks) != 1 {
		t.Fatalf("expected 1 error chunk, got %d", len(chunks))
	}
	if chunks[0].Event != "error" {
		t.Fatalf("expected error event, got %s", chunks[0].Event)
	}
	if !chunks[0].Done {
		t.Fatal("expected error chunk marked done")
	}
	if !strings.Contains(chunks[0].Error, "provider unavailable") {
		t.Fatalf("expected error chunk to include provider error, got %s", chunks[0].Error)
	}
}

func TestRuntimeLoopPublishesWaitingApprovalAndTimeoutChunks(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think":"need approval","call_skill":{"id":"c1","name":"write_file","arguments":{},"is_write_operation":true}}`,
		},
	}
	gate := &mockApprovalGate{
		wait: func(ctx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalDecision, error) {
			return ApprovalDecision{
				TraceID:     req.TraceID,
				TaskID:      req.TaskID,
				ToolCallID:  req.ToolCallID,
				Approved:    false,
				Reason:      "timeout",
				DecidedAtMs: time.Now().UnixMilli(),
			}, fmt.Errorf("approval timeout")
		},
	}
	r := NewRuntime(p, &mockExecutor{}, gate)

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	res, err := r.Run(context.Background(), InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}, fanout)
	if err == nil || err.Error() != "approval timeout" {
		t.Fatalf("expected approval timeout error, got %v", err)
	}
	if res.Status != "timeout" {
		t.Fatalf("expected timeout status, got %s", res.Status)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	events := []string{chunks[0].Event, chunks[1].Event, chunks[2].Event}
	expected := []string{"tool_call", "waiting_approval", "timeout"}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if !chunks[2].Done {
		t.Fatal("expected timeout chunk to be terminal")
	}
}

func TestRuntimeLoopPublishesWaitingApprovalAndCancelledChunks(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think":"need approval","call_skill":{"id":"c1","name":"write_file","arguments":{},"is_write_operation":true}}`,
		},
	}

	ctx, cancel := context.WithCancel(context.Background())
	gate := &mockApprovalGate{
		wait: func(waitCtx context.Context, req ApprovalRequest, timeout time.Duration) (ApprovalDecision, error) {
			cancel()
			<-waitCtx.Done()
			return ApprovalDecision{}, waitCtx.Err()
		},
	}
	r := NewRuntime(p, &mockExecutor{}, gate)

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	res, err := r.Run(ctx, InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}, fanout)
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context canceled, got %v", err)
	}
	if res.Status != "cancelled" {
		t.Fatalf("expected cancelled status, got %s", res.Status)
	}
	if len(chunks) != 3 {
		t.Fatalf("expected 3 chunks, got %d", len(chunks))
	}
	events := []string{chunks[0].Event, chunks[1].Event, chunks[2].Event}
	expected := []string{"tool_call", "waiting_approval", "cancelled"}
	for i := range expected {
		if events[i] != expected[i] {
			t.Fatalf("expected events %v, got %v", expected, events)
		}
	}
	if !chunks[2].Done {
		t.Fatal("expected cancelled chunk to be terminal")
	}
}

func TestRuntimeLoopStopsAtMaxTurns(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think":"need tool","call_skill":{"id":"c1","name":"test_tool","arguments":"{}"}}`,
			`{"think":"need tool again","call_skill":{"id":"c2","name":"test_tool","arguments":"{}"}}`,
		},
	}
	e := &mockExecutor{}
	r := NewRuntime(p, e, nil)
	r.MaxTurns = 2

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	res, err := r.Run(context.Background(), InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}, fanout)
	if err == nil || err.Error() != "exceeded max turns" {
		t.Fatalf("expected exceeded max turns error, got %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("expected failed status, got %s", res.Status)
	}
	if len(chunks) != 5 {
		t.Fatalf("expected 5 chunks, got %d", len(chunks))
	}
	if chunks[4].Event != "error" {
		t.Fatalf("expected terminal error event, got %s", chunks[4].Event)
	}
	if !chunks[4].Done {
		t.Fatal("expected max-turn terminal chunk to be done")
	}
	if !strings.Contains(chunks[4].Error, "exceeded max turns") {
		t.Fatalf("expected max-turn error in terminal chunk, got %s", chunks[4].Error)
	}
}

func TestRuntimeLoopWriteToolRequiresApprovalGate(t *testing.T) {
	p := &mockProvider{
		responses: []string{
			`{"think":"need approval","call_skill":{"id":"c1","name":"write_file","arguments":{},"is_write_operation":true}}`,
		},
	}
	e := &mockExecutor{}
	r := NewRuntime(p, e, nil)

	var chunks []StreamChunk
	fanout := NewFanout("trace-1", "session-1", "task-1")
	fanout.SetPublisher(func(ctx context.Context, chunk StreamChunk) error {
		chunks = append(chunks, chunk)
		return nil
	})

	res, err := r.Run(context.Background(), InferenceRequest{
		TraceID: "trace-1",
		TaskID:  "task-1",
	}, fanout)
	if err == nil || err.Error() != "approval gate required for write operation" {
		t.Fatalf("expected approval gate required error, got %v", err)
	}
	if res.Status != "failed" {
		t.Fatalf("expected failed status, got %s", res.Status)
	}
	if e.lastCall.Name != "" {
		t.Fatalf("expected write tool not executed without approval gate, got %s", e.lastCall.Name)
	}
	if len(chunks) != 2 {
		t.Fatalf("expected tool_call and terminal error chunks, got %d", len(chunks))
	}
	if chunks[0].Event != "tool_call" {
		t.Fatalf("expected first chunk tool_call, got %s", chunks[0].Event)
	}
	if chunks[1].Event != "error" {
		t.Fatalf("expected terminal error chunk, got %s", chunks[1].Event)
	}
	if !chunks[1].Done {
		t.Fatal("expected terminal chunk marked done")
	}
	if !strings.Contains(chunks[1].Error, "approval gate required") {
		t.Fatalf("expected terminal error to mention approval gate, got %s", chunks[1].Error)
	}
}
