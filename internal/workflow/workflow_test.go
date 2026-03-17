package workflow

import (
	"context"
	"testing"
)

func TestWorkflowStart(t *testing.T) {
	var ran []string
	onReady := func(ctx context.Context, agentType string, nodeID string) error {
		ran = append(ran, nodeID)
		return nil
	}
	wf := NewWorkflow(onReady)

	_ = wf.AddTask("fetch", "researcher", nil)
	_ = wf.AddTask("plan", "orchestrator", []string{"fetch"})

	if err := wf.Start(context.Background()); err != nil {
		t.Fatalf("start failed: %v", err)
	}

	if len(ran) != 1 || ran[0] != "fetch" {
		t.Fatalf("expected fetch to run, got %v", ran)
	}

	state, ok := wf.NodeState("fetch")
	if !ok || state != NodeStateRunning {
		t.Fatalf("expected fetch running, got %v %v", state, ok)
	}
}

func TestWorkflowTransitions(t *testing.T) {
	var ran []string
	onReady := func(ctx context.Context, agentType string, nodeID string) error {
		ran = append(ran, nodeID)
		return nil
	}
	wf := NewWorkflow(onReady)

	_ = wf.AddTask("fetch", "researcher", nil)
	_ = wf.AddTask("plan", "orchestrator", []string{"fetch"})

	ctx := context.Background()
	_ = wf.Start(ctx)

	if err := wf.MarkCompleted(ctx, "fetch"); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}

	if len(ran) != 2 || ran[1] != "plan" {
		t.Fatalf("expected plan to run after fetch, got %v", ran)
	}

	state, _ := wf.NodeState("fetch")
	if state != NodeStateCompleted {
		t.Fatalf("expected fetch completed, got %v", state)
	}
	state, _ = wf.NodeState("plan")
	if state != NodeStateRunning {
		t.Fatalf("expected plan running, got %v", state)
	}
}
