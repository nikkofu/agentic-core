package workflow

import "fmt"

type NodeState string

const (
	NodeStatePending   NodeState = "pending"
	NodeStateRunning   NodeState = "running"
	NodeStateCompleted NodeState = "completed"
	NodeStateFailed    NodeState = "failed"
)

type NodeStateMachine struct {
	states map[string]NodeState
}

func NewNodeStateMachine() *NodeStateMachine {
	return &NodeStateMachine{states: make(map[string]NodeState)}
}

func (sm *NodeStateMachine) InitNode(nodeID string) {
	sm.states[nodeID] = NodeStatePending
}

func (sm *NodeStateMachine) State(nodeID string) (NodeState, bool) {
	state, ok := sm.states[nodeID]
	return state, ok
}

func (sm *NodeStateMachine) Transition(nodeID string, target NodeState) error {
	current, ok := sm.states[nodeID]
	if !ok {
		return fmt.Errorf("unknown node: %s", nodeID)
	}

	switch current {
	case NodeStatePending:
		if target == NodeStateRunning {
			sm.states[nodeID] = target
			return nil
		}
	case NodeStateRunning:
		if target == NodeStateCompleted || target == NodeStateFailed {
			sm.states[nodeID] = target
			return nil
		}
	}

	return fmt.Errorf("invalid transition: %s -> %s", current, target)
}
