package workflow

import "testing"

func TestNodeStateMachineInitSetsPending(t *testing.T) {
	sm := NewNodeStateMachine()
	sm.InitNode("n1")

	state, ok := sm.State("n1")
	if !ok {
		t.Fatal("expected node state to exist")
	}
	if state != NodeStatePending {
		t.Fatalf("expected pending, got %s", state)
	}
}

func TestNodeStateMachineAllowsPendingToRunningToCompleted(t *testing.T) {
	sm := NewNodeStateMachine()
	sm.InitNode("n1")

	if err := sm.Transition("n1", NodeStateRunning); err != nil {
		t.Fatalf("expected pending->running transition to succeed: %v", err)
	}
	if err := sm.Transition("n1", NodeStateCompleted); err != nil {
		t.Fatalf("expected running->completed transition to succeed: %v", err)
	}
}

func TestNodeStateMachineRejectsInvalidTransition(t *testing.T) {
	sm := NewNodeStateMachine()
	sm.InitNode("n1")

	if err := sm.Transition("n1", NodeStateCompleted); err == nil {
		t.Fatal("expected pending->completed to be rejected")
	}
}

func TestNodeStateMachineRejectsUnknownNode(t *testing.T) {
	sm := NewNodeStateMachine()
	if err := sm.Transition("missing", NodeStateRunning); err == nil {
		t.Fatal("expected unknown node transition to fail")
	}
}
