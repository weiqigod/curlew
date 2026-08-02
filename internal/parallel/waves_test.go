package parallel

import (
	"sort"
	"testing"
)

// noCollisionGraph: two parallel requests producing different vars.
func noCollisionGraph() *DependencyGraph {
	g := noEdgeGraph(2)
	g.Nodes[0].ProducedVars = set("user_id")
	g.Nodes[1].ProducedVars = set("order_id")
	return g
}

// parallelCollisionGraph: two parallel requests producing the same var.
func parallelCollisionGraph() *DependencyGraph {
	g := noEdgeGraph(2)
	g.Nodes[0].ProducedVars = set("token")
	g.Nodes[1].ProducedVars = set("token")
	return g
}

// sequentialSameVarGraph: two sequential requests producing the same var.
// B depends on A, so they are not parallel and no collision.
func sequentialSameVarGraph() *DependencyGraph {
	g := noEdgeGraph(2)
	g.Nodes[0].ProducedVars = set("token")
	g.Nodes[1].ProducedVars = set("token")
	g.Nodes[1].Dependencies[0] = true
	g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"token"}}}
	return g
}

// multipleCollisionGraph: three parallel requests, two pairs collide on different vars.
func multipleCollisionGraph() *DependencyGraph {
	g := noEdgeGraph(3)
	g.Nodes[0].ProducedVars = set("token", "user_id")
	g.Nodes[1].ProducedVars = set("token")
	g.Nodes[2].ProducedVars = set("user_id")
	return g
}

// threeProducerCollisionGraph: three parallel requests all produce same var.
func threeProducerCollisionGraph() *DependencyGraph {
	g := noEdgeGraph(3)
	g.Nodes[0].ProducedVars = set("token")
	g.Nodes[1].ProducedVars = set("token")
	g.Nodes[2].ProducedVars = set("token")
	return g
}

func TestDetectCollisions(t *testing.T) {
	tests := []struct {
		name       string
		graph      *DependencyGraph
		wantErrors int
	}{
		{"no collision - different vars", noCollisionGraph(), 0},
		{"collision - same var parallel", parallelCollisionGraph(), 1},
		{"no collision - same var but sequential", sequentialSameVarGraph(), 0},
		{"multiple collisions", multipleCollisionGraph(), 2},
		{"three producers same var", threeProducerCollisionGraph(), 1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.graph.Errors = nil // reset
			detectCollisions(tt.graph)
			if len(tt.graph.Errors) != tt.wantErrors {
				t.Errorf("detectCollisions() produced %d errors, want %d\nerrors: %v",
					len(tt.graph.Errors), tt.wantErrors, tt.graph.Errors)
			}
		})
	}
}

// allIndependentWaveGraph creates n independent nodes.
func allIndependentWaveGraph(n int) *DependencyGraph {
	g := noEdgeGraph(n)
	g.IsValid = true
	return g
}

// linearChainWaveGraph creates a linear chain of n nodes.
func linearChainWaveGraph(n int) *DependencyGraph {
	g := noEdgeGraph(n)
	g.IsValid = true
	for i := 1; i < n; i++ {
		g.Nodes[i].Dependencies[i-1] = true
	}
	return g
}

// diamondPatternWaveGraph creates:
//
//	  A
//	 / \
//	B   C
//	 \ /
//	  D
func diamondPatternWaveGraph() *DependencyGraph {
	g := noEdgeGraph(4)
	g.IsValid = true
	g.Nodes[1].Dependencies[0] = true
	g.Nodes[2].Dependencies[0] = true
	g.Nodes[3].Dependencies[1] = true
	g.Nodes[3].Dependencies[2] = true
	return g
}

// complexDAGWaveGraph creates:
// A and B independent; C depends on A; D and E depend on C.
func complexDAGWaveGraph() *DependencyGraph {
	g := noEdgeGraph(5)
	g.IsValid = true
	g.Nodes[2].Dependencies[0] = true
	g.Nodes[3].Dependencies[2] = true
	g.Nodes[4].Dependencies[2] = true
	return g
}

// singleNodeWaveGraph creates a single-node graph.
func singleNodeWaveGraph() *DependencyGraph {
	g := noEdgeGraph(1)
	g.IsValid = true
	return g
}

// emptyWaveGraph creates an empty graph.
func emptyWaveGraph() *DependencyGraph {
	return &DependencyGraph{IsValid: true}
}

// TestParallel_AncestorClosure verifies reverse-BFS over Dependencies edges
// returns the correct ancestor set for linear-chain, diamond, disconnected,
// and empty-starts shapes.
func TestParallel_AncestorClosure(t *testing.T) {
	// Helper: build a map[int]bool from explicit indices.
	idxSet := func(indices ...int) map[int]bool {
		m := make(map[int]bool, len(indices))
		for _, i := range indices {
			m[i] = true
		}
		return m
	}
	// Disconnected: two independent linear chains 0<-1 and 2<-3
	disconnected := func() *DependencyGraph {
		g := noEdgeGraph(4)
		g.IsValid = true
		g.Nodes[1].Dependencies[0] = true
		g.Nodes[3].Dependencies[2] = true
		return g
	}()

	tests := []struct {
		name   string
		graph  *DependencyGraph
		starts []int
		want   map[int]bool
	}{
		{"linear chain from leaf includes all", linearChainWaveGraph(4), []int{3}, idxSet(0, 1, 2, 3)},
		{"linear chain from middle includes self and ancestors", linearChainWaveGraph(4), []int{2}, idxSet(0, 1, 2)},
		{"linear chain from root is singleton", linearChainWaveGraph(4), []int{0}, idxSet(0)},
		{"diamond from sink includes full diamond", diamondPatternWaveGraph(), []int{3}, idxSet(0, 1, 2, 3)},
		{"diamond from one branch skips other branch", diamondPatternWaveGraph(), []int{1}, idxSet(0, 1)},
		{"diamond from root is singleton", diamondPatternWaveGraph(), []int{0}, idxSet(0)},
		{"disconnected reaches only its own component", disconnected, []int{1}, idxSet(0, 1)},
		{"disconnected from multiple starts unions components", disconnected, []int{1, 3}, idxSet(0, 1, 2, 3)},
		{"empty starts yields empty set", linearChainWaveGraph(4), nil, idxSet()},
		{"out-of-range start ignored", linearChainWaveGraph(4), []int{99}, idxSet()},
		{"duplicate start indices deduped", linearChainWaveGraph(4), []int{3, 3, 2}, idxSet(0, 1, 2, 3)},
		{"nil graph yields empty set", nil, []int{0}, idxSet()},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := AncestorClosure(tt.graph, tt.starts)
			if len(got) != len(tt.want) {
				t.Errorf("AncestorClosure size = %d, want %d (got=%v want=%v)",
					len(got), len(tt.want), got, tt.want)
			}
			for k := range tt.want {
				if !got[k] {
					t.Errorf("AncestorClosure missing index %d; got=%v", k, got)
				}
			}
			for k := range got {
				if !tt.want[k] {
					t.Errorf("AncestorClosure contains extra index %d; want=%v", k, tt.want)
				}
			}
		})
	}
}

func TestComputeWaves(t *testing.T) {
	tests := []struct {
		name      string
		graph     *DependencyGraph
		wantWaves [][]int
	}{
		{"all independent - single wave", allIndependentWaveGraph(3), [][]int{{0, 1, 2}}},
		{"linear chain - N waves", linearChainWaveGraph(3), [][]int{{0}, {1}, {2}}},
		{"diamond pattern", diamondPatternWaveGraph(), [][]int{{0}, {1, 2}, {3}}},
		{"complex DAG", complexDAGWaveGraph(), [][]int{{0, 1}, {2}, {3, 4}}},
		{"single node", singleNodeWaveGraph(), [][]int{{0}}},
		{"empty graph", emptyWaveGraph(), [][]int{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			computeWaves(tt.graph)
			if len(tt.graph.Waves) != len(tt.wantWaves) {
				t.Errorf("computeWaves() produced %d waves, want %d\ngot:  %v\nwant: %v",
					len(tt.graph.Waves), len(tt.wantWaves), tt.graph.Waves, tt.wantWaves)
				return
			}
			for i := range tt.wantWaves {
				got := make([]int, len(tt.graph.Waves[i]))
				copy(got, tt.graph.Waves[i])
				sort.Ints(got)
				want := make([]int, len(tt.wantWaves[i]))
				copy(want, tt.wantWaves[i])
				sort.Ints(want)
				if len(got) != len(want) {
					t.Errorf("wave %d: got %v, want %v", i, got, want)
					continue
				}
				for j := range want {
					if got[j] != want[j] {
						t.Errorf("wave %d: got %v, want %v", i, got, want)
						break
					}
				}
			}
		})
	}
}
