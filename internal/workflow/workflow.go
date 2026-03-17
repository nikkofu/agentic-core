package workflow

import (
	"context"
)

type Workflow struct {
	dag        *DAG
	sm         *NodeStateMachine
	executor   *Executor
	agentTypes map[string]string // nodeID -> agentType
}

func NewWorkflow(onReady OnNodeReady) *Workflow {
	dag := NewDAG()
	sm := NewNodeStateMachine()
	w := &Workflow{
		dag:        dag,
		sm:         sm,
		agentTypes: make(map[string]string),
	}
	w.executor = NewExecutor(dag, sm, onReady)
	w.executor.SetAgentMapper(w.GetAgentType)
	return w
}

func (w *Workflow) AddTask(id string, agentType string, deps []string) error {
	w.dag.AddNode(id)
	w.sm.InitNode(id)
	if agentType == "" {
		agentType = "generic"
	}
	w.agentTypes[id] = agentType

	for _, dep := range deps {
		if err := w.dag.AddDependency(id, dep); err != nil {
			return err
		}
	}
	return nil
}

func (w *Workflow) GetAgentType(nodeID string) string {
	return w.agentTypes[nodeID]
}

func (w *Workflow) Start(ctx context.Context) error {
	return w.executor.Start(ctx)
}

func (w *Workflow) MarkCompleted(ctx context.Context, nodeID string) error {
	return w.executor.MarkNodeCompleted(ctx, nodeID)
}

func (w *Workflow) MarkFailed(ctx context.Context, nodeID string, err string) error {
	return w.executor.MarkNodeFailed(ctx, nodeID, err)
}

func (w *Workflow) NodeState(nodeID string) (NodeState, bool) {
	return w.sm.State(nodeID)
}
