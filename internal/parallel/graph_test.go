package parallel

import "testing"

func TestRequestNode_ZeroValue(t *testing.T) {
	tests := []struct {
		name string
		node RequestNode
	}{
		{"zero value has nil maps", RequestNode{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.node.ProducedVars != nil {
				t.Error("expected ProducedVars to be nil for zero value")
			}
			if tt.node.ReferencedVars != nil {
				t.Error("expected ReferencedVars to be nil for zero value")
			}
			if tt.node.Dependencies != nil {
				t.Error("expected Dependencies to be nil for zero value")
			}
			if tt.node.Index != 0 {
				t.Errorf("expected Index 0, got %d", tt.node.Index)
			}
			if tt.node.Name != "" {
				t.Errorf("expected empty Name, got %q", tt.node.Name)
			}
		})
	}
}

func TestDependencyGraph_ZeroValue(t *testing.T) {
	tests := []struct {
		name  string
		graph DependencyGraph
	}{
		{"zero value is not valid", DependencyGraph{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.graph.IsValid {
				t.Error("expected IsValid to be false for zero value")
			}
			if tt.graph.Nodes != nil {
				t.Error("expected Nodes to be nil for zero value")
			}
			if tt.graph.Edges != nil {
				t.Error("expected Edges to be nil for zero value")
			}
			if tt.graph.Waves != nil {
				t.Error("expected Waves to be nil for zero value")
			}
		})
	}
}

func TestEdge_Fields(t *testing.T) {
	e := Edge{From: 0, To: 1, Variables: []string{"token"}}
	if e.From != 0 {
		t.Errorf("expected From 0, got %d", e.From)
	}
	if e.To != 1 {
		t.Errorf("expected To 1, got %d", e.To)
	}
	if len(e.Variables) != 1 || e.Variables[0] != "token" {
		t.Errorf("expected Variables [token], got %v", e.Variables)
	}
}
