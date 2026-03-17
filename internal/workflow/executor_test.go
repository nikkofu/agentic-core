package workflow

import (
	"context"
	"testing"
)

func TestExecutorTransitionsNodesInOrder(t *testing.T) {
	dag := NewDAG()
	dag.AddNode("fetch")
	dag.AddNode("plan")
	_ = dag.AddDependency("plan", "fetch")

	sm := NewNodeStateMachine()
	sm.InitNode("fetch")
	sm.InitNode("plan")

	var ran []string
	exec := NewExecutor(dag, sm, func(ctx context.Context, agentType string, nodeID string) error {
		ran = append(ran, nodeID)
		return nil
	})

	ctx := context.Background()
	if err := exec.Start(ctx); err != nil {
		t.Fatalf("expected start success, got error: %v", err)
	}

	if len(ran) != 1 || ran[0] != "fetch" {
		t.Fatalf("expected fetch to run first, got %v", ran)
	}

	// Completing fetch should trigger plan
	if err := exec.MarkNodeCompleted(ctx, "fetch"); err != nil {
		t.Fatalf("mark completed failed: %v", err)
	}

	if len(ran) != 2 || ran[1] != "plan" {
		t.Fatalf("expected plan to run after fetch completes, got %v", ran)
	}

	for _, id := range []string{"fetch", "plan"} {
		state, ok := sm.State(id)
		if !ok {
			t.Fatalf("missing state for node %s", id)
		}
		if id == "fetch" && state != NodeStateCompleted {
			t.Fatalf("expected fetch completed, got %s", state)
		}
		if id == "plan" && state != NodeStateRunning {
			t.Fatalf("expected plan running, got %s", state)
		}
	}
}

func TestExecutorMarkNodeFailed(t *testing.T) {
	dag := NewDAG()
	dag.AddNode("a")
	sm := NewNodeStateMachine()
	sm.InitNode("a")

	exec := NewExecutor(dag, sm, func(ctx context.Context, agentType string, nodeID string) error {
		return nil
	})

	ctx := context.Background()
	_ = sm.Transition("a", NodeStateRunning)
	if err := exec.MarkNodeFailed(ctx, "a", "error message"); err != nil {
		t.Fatalf("mark failed failed: %v", err)
	}

	state, _ := sm.State("a")
	if state != NodeStateFailed {
		t.Fatalf("expected failed state, got %s", state)
	}
}
