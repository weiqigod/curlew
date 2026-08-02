package parallel

import (
	"strings"
	"testing"
)

// noEdgeGraph creates a graph with n nodes and no edges.
func noEdgeGraph(n int) *DependencyGraph {
	g := &DependencyGraph{
		Nodes:   make([]RequestNode, n),
		IsValid: true,
	}
	for i := 0; i < n; i++ {
		g.Nodes[i] = RequestNode{
			Index:        i,
			Name:         string(rune('A' + i)),
			Dependencies: make(map[int]bool),
		}
	}
	return g
}

// linearGraph creates A -> B -> C (0 -> 1 -> 2).
func linearGraph() *DependencyGraph {
	g := noEdgeGraph(3)
	g.Nodes[1].Dependencies[0] = true
	g.Nodes[2].Dependencies[1] = true
	g.Edges = []Edge{
		{From: 0, To: 1, Variables: []string{"v1"}},
		{From: 1, To: 2, Variables: []string{"v2"}},
	}
	return g
}

// diamondGraph creates:
//
//	  A
//	 / \
//	B   C
//	 \ /
//	  D
func diamondGraph() *DependencyGraph {
	g := noEdgeGraph(4)
	g.Nodes[1].Dependencies[0] = true
	g.Nodes[2].Dependencies[0] = true
	g.Nodes[3].Dependencies[1] = true
	g.Nodes[3].Dependencies[2] = true
	g.Edges = []Edge{
		{From: 0, To: 1, Variables: []string{"v1"}},
		{From: 0, To: 2, Variables: []string{"v1"}},
		{From: 1, To: 3, Variables: []string{"v2"}},
		{From: 2, To: 3, Variables: []string{"v3"}},
	}
	return g
}

// simpleCycleGraph creates A -> B -> A.
func simpleCycleGraph() *DependencyGraph {
	g := noEdgeGraph(2)
	g.Nodes[0].Dependencies[1] = true
	g.Nodes[1].Dependencies[0] = true
	return g
}

// selfCycleGraph creates A -> A.
func selfCycleGraph() *DependencyGraph {
	g := noEdgeGraph(1)
	g.Nodes[0].Dependencies[0] = true
	return g
}

// partialCycleGraph creates: A (no deps), B -> C -> B.
func partialCycleGraph() *DependencyGraph {
	g := noEdgeGraph(3)
	g.Nodes[1].Dependencies[2] = true
	g.Nodes[2].Dependencies[1] = true
	return g
}

func TestTopologicalSort(t *testing.T) {
	tests := []struct {
		name      string
		graph     *DependencyGraph
		wantValid bool
	}{
		{"no edges - all nodes in order", noEdgeGraph(3), true},
		{"linear chain", linearGraph(), true},
		{"diamond", diamondGraph(), true},
		{"simple cycle A->B->A", simpleCycleGraph(), false},
		{"self-cycle", selfCycleGraph(), false},
		{"partial cycle with valid nodes", partialCycleGraph(), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			order, valid := topologicalSort(tt.graph)
			if valid != tt.wantValid {
				t.Errorf("topologicalSort() valid = %v, want %v", valid, tt.wantValid)
			}
			if tt.wantValid && len(order) != len(tt.graph.Nodes) {
				t.Errorf("topologicalSort() returned %d nodes, want %d", len(order), len(tt.graph.Nodes))
			}
			if tt.wantValid {
				// Verify topological ordering: for each edge, From comes before To
				pos := make(map[int]int, len(order))
				for i, idx := range order {
					pos[idx] = i
				}
				for _, node := range tt.graph.Nodes {
					for dep := range node.Dependencies {
						if pos[dep] >= pos[node.Index] {
							t.Errorf("topological order violated: node %d depends on %d but appears before it",
								node.Index, dep)
						}
					}
				}
			}
		})
	}
}

func TestFindCyclePath(t *testing.T) {
	tests := []struct {
		name      string
		graph     *DependencyGraph
		remaining []int
		wantParts []string
	}{
		{"simple two-node cycle", simpleCycleGraph(), []int{0, 1}, []string{"A", "B"}},
		{"three-node cycle", func() *DependencyGraph {
			g := noEdgeGraph(3)
			g.Nodes[0].Name = "Request A"
			g.Nodes[1].Name = "Request B"
			g.Nodes[2].Name = "Request C"
			g.Nodes[0].Dependencies[2] = true
			g.Nodes[1].Dependencies[0] = true
			g.Nodes[2].Dependencies[1] = true
			return g
		}(), []int{0, 1, 2}, []string{"Request A", "Request B", "Request C"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := findCyclePath(tt.graph, tt.remaining)
			for _, part := range tt.wantParts {
				if !strings.Contains(result, part) {
					t.Errorf("findCyclePath() = %q, missing expected substring %q", result, part)
				}
			}
		})
	}
}
