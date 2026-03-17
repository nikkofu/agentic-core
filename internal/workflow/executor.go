package workflow

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

var (
	ErrNodeNotFound     = errors.New("node not found")
	ErrTransitionFailed = errors.New("state transition failed")
)

// OnNodeReady 现在接收 agentType, nodeID
type OnNodeReady func(ctx context.Context, agentType string, nodeID string) error

type Executor struct {
	dag         *DAG
	sm          *NodeStateMachine
	onNodeReady OnNodeReady
	mu          sync.Mutex
	getAgent    func(nodeID string) string // 映射 nodeID 到 agentType
}

func NewExecutor(dag *DAG, sm *NodeStateMachine, onReady OnNodeReady) *Executor {
	return &Executor{
		dag:         dag,
		sm:          sm,
		onNodeReady: onReady,
	}
}

func (e *Executor) SetAgentMapper(fn func(nodeID string) string) {
	e.getAgent = fn
}

// Start 启动 DAG 的入口节点
func (e *Executor) Start(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	for nodeID := range e.dag.nodes {
		deps := e.dag.Dependencies(nodeID)
		if len(deps) == 0 {
			if err := e.readyNode(ctx, nodeID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) MarkNodeCompleted(ctx context.Context, nodeID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.sm.Transition(nodeID, NodeStateCompleted); err != nil {
		return fmt.Errorf("%w: %s -> %s: %v", ErrTransitionFailed, nodeID, NodeStateCompleted, err)
	}

	dependents := e.dag.Dependents(nodeID)
	for _, depID := range dependents {
		if e.isNodeReady(depID) {
			if err := e.readyNode(ctx, depID); err != nil {
				return err
			}
		}
	}
	return nil
}

func (e *Executor) MarkNodeFailed(ctx context.Context, nodeID string, err string) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	return e.sm.Transition(nodeID, NodeStateFailed)
}

func (e *Executor) isNodeReady(nodeID string) bool {
	deps := e.dag.Dependencies(nodeID)
	for _, depID := range deps {
		state, ok := e.sm.State(depID)
		if !ok || state != NodeStateCompleted {
			return false
		}
	}
	state, ok := e.sm.State(nodeID)
	return ok && (state == NodeStatePending || state == NodeStateRunning)
}

func (e *Executor) readyNode(ctx context.Context, nodeID string) error {
	if err := e.sm.Transition(nodeID, NodeStateRunning); err != nil {
		return err
	}
	agentType := "generic"
	if e.getAgent != nil {
		agentType = e.getAgent(nodeID)
	}
	return e.onNodeReady(ctx, agentType, nodeID)
}
