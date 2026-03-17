package workflow

import "testing"

func TestDAGTopologicalOrder(t *testing.T) {
	dag := NewDAG()
	dag.AddNode("fetch")
	dag.AddNode("plan")
	dag.AddNode("execute")

	if err := dag.AddDependency("plan", "fetch"); err != nil {
		t.Fatalf("add dependency failed: %v", err)
	}
	if err := dag.AddDependency("execute", "plan"); err != nil {
		t.Fatalf("add dependency failed: %v", err)
	}

	order, err := dag.TopologicalOrder()
	if err != nil {
		t.Fatalf("topological sort failed: %v", err)
	}

	if len(order) != 3 {
		t.Fatalf("expected 3 nodes, got %d", len(order))
	}
	if order[0] != "fetch" || order[1] != "plan" || order[2] != "execute" {
		t.Fatalf("unexpected order: %v", order)
	}
}

func TestDAGDetectsCycle(t *testing.T) {
	dag := NewDAG()
	dag.AddNode("a")
	dag.AddNode("b")

	if err := dag.AddDependency("b", "a"); err != nil {
		t.Fatalf("add dependency failed: %v", err)
	}
	if err := dag.AddDependency("a", "b"); err != nil {
		t.Fatalf("add dependency failed: %v", err)
	}

	if _, err := dag.TopologicalOrder(); err == nil {
		t.Fatal("expected cycle detection error")
	}
}
