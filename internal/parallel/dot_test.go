package parallel

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteDOT(t *testing.T) {
	tests := []struct {
		name      string
		graph     *DependencyGraph
		wantParts []string
	}{
		{
			"empty graph", &DependencyGraph{IsValid: true, Waves: [][]int{}},
			[]string{"digraph", "}"},
		},
		{"single node no edges", func() *DependencyGraph {
			g := noEdgeGraph(1)
			g.Nodes[0].Name = "Get Token"
			return g
		}(), []string{"digraph", `"Get Token"`}},
		{"edge with label", func() *DependencyGraph {
			g := noEdgeGraph(2)
			g.Nodes[0].Name = "Login"
			g.Nodes[1].Name = "Get User"
			g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"token"}}}
			return g
		}(), []string{"->", `label="token"`}},
		{"multiple edges", func() *DependencyGraph {
			g := noEdgeGraph(3)
			g.Nodes[0].Name = "Login"
			g.Nodes[1].Name = "Get User"
			g.Nodes[2].Name = "Get Order"
			g.Edges = []Edge{
				{From: 0, To: 1, Variables: []string{"token"}},
				{From: 0, To: 2, Variables: []string{"token"}},
			}
			return g
		}(), []string{"->", "->"}},
		{"explicit depends_on edge labelled", func() *DependencyGraph {
			g := noEdgeGraph(2)
			g.Nodes[0].Name = "First"
			g.Nodes[1].Name = "Second"
			g.Edges = []Edge{{From: 0, To: 1, Explicit: true}}
			return g
		}(), []string{`"First" -> "Second"`, `label="depends_on"`}},
		{"multiple variables on edge", func() *DependencyGraph {
			g := noEdgeGraph(2)
			g.Nodes[0].Name = "Login"
			g.Nodes[1].Name = "Get User"
			g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"token", "user_id"}}}
			return g
		}(), []string{`label="token, user_id"`}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var buf bytes.Buffer
			err := WriteDOT(&buf, tt.graph)
			if err != nil {
				t.Fatalf("WriteDOT() error: %v", err)
			}
			output := buf.String()
			for _, part := range tt.wantParts {
				if !strings.Contains(output, part) {
					t.Errorf("WriteDOT() output missing %q\nfull output:\n%s", part, output)
				}
			}
		})
	}
}

func TestFormatWaves(t *testing.T) {
	tests := []struct {
		name       string
		graph      *DependencyGraph
		wantParts  []string
		wantAbsent []string
	}{
		{"single wave with concurrent label", func() *DependencyGraph {
			g := noEdgeGraph(3)
			g.Waves = [][]int{{0, 1, 2}}
			return g
		}(), []string{"Wave 1", "3 concurrent", "Maximum parallelism: 3"}, nil},
		{"multiple waves with names", func() *DependencyGraph {
			g := noEdgeGraph(4)
			g.Nodes[0].Name = "Login"
			g.Nodes[1].Name = "Get User"
			g.Nodes[2].Name = "Get Orders"
			g.Nodes[3].Name = "Summary"
			g.Waves = [][]int{{0}, {1, 2}, {3}}
			return g
		}(), []string{"Wave 1", "Wave 2", "Wave 3", "Login", "Get User", "Maximum parallelism: 2", "Expected speedup:"}, nil},
		{
			"empty waves", &DependencyGraph{Waves: [][]int{}},
			[]string{"Total waves: 0"},
			nil,
		},
		{
			"single request sequential label",
			func() *DependencyGraph {
				g := noEdgeGraph(1)
				g.Waves = [][]int{{0}}
				return g
			}(),
			[]string{"1 sequential"},
			[]string{"Expected speedup:"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatWaves(tt.graph)
			for _, part := range tt.wantParts {
				if !strings.Contains(result, part) {
					t.Errorf("FormatWaves() missing %q\nfull output:\n%s", part, result)
				}
			}
			for _, part := range tt.wantAbsent {
				if strings.Contains(result, part) {
					t.Errorf("FormatWaves() should not contain %q\nfull output:\n%s", part, result)
				}
			}
		})
	}
}
