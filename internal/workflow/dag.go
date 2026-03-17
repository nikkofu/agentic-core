package workflow

import "errors"

type DAG struct {
	nodes map[string]struct{}
	edges map[string]map[string]struct{}
}

func NewDAG() *DAG {
	return &DAG{
		nodes: make(map[string]struct{}),
		edges: make(map[string]map[string]struct{}),
	}
}

func (d *DAG) AddNode(id string) {
	if _, ok := d.nodes[id]; ok {
		return
	}
	d.nodes[id] = struct{}{}
	if _, ok := d.edges[id]; !ok {
		d.edges[id] = make(map[string]struct{})
	}
}

func (d *DAG) AddDependency(node string, dependsOn string) error {
	if _, ok := d.nodes[node]; !ok {
		return errors.New("node not found")
	}
	if _, ok := d.nodes[dependsOn]; !ok {
		return errors.New("dependency node not found")
	}
	if d.edges[dependsOn] == nil {
		d.edges[dependsOn] = make(map[string]struct{})
	}
	d.edges[dependsOn][node] = struct{}{}
	return nil
}

func (d *DAG) Dependencies(nodeID string) []string {
	var deps []string
	for from, tos := range d.edges {
		for to := range tos {
			if to == nodeID {
				deps = append(deps, from)
			}
		}
	}
	return deps
}

func (d *DAG) Dependents(nodeID string) []string {
	var deps []string
	for to := range d.edges[nodeID] {
		deps = append(deps, to)
	}
	return deps
}

func (d *DAG) TopologicalOrder() ([]string, error) {
	indegree := make(map[string]int, len(d.nodes))
	for n := range d.nodes {
		indegree[n] = 0
	}

	for from := range d.edges {
		for to := range d.edges[from] {
			indegree[to]++
		}
	}

	queue := make([]string, 0)
	for n, deg := range indegree {
		if deg == 0 {
			queue = append(queue, n)
		}
	}

	order := make([]string, 0, len(d.nodes))
	for len(queue) > 0 {
		n := queue[0]
		queue = queue[1:]
		order = append(order, n)

		for to := range d.edges[n] {
			indegree[to]--
			if indegree[to] == 0 {
				queue = append(queue, to)
			}
		}
	}

	if len(order) != len(d.nodes) {
		return nil, errors.New("cycle detected")
	}
	return order, nil
}
