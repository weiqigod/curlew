package parallel

import (
	"fmt"
	"sort"
	"testing"

	"github.com/peterlindqvist/apitest/internal/parser"
)

// threeIndependentRequests creates three requests with no variable sharing.
func threeIndependentRequests() []parser.RequestItem {
	return []parser.RequestItem{
		{Name: "Get Users", Request: parser.Request{Method: "GET", URL: "http://example.com/users"}},
		{Name: "Get Orders", Request: parser.Request{Method: "GET", URL: "http://example.com/orders"}},
		{Name: "Get Products", Request: parser.Request{Method: "GET", URL: "http://example.com/products"}},
	}
}

// linearChainItems creates A extracts user_id, B uses user_id and extracts order_id, C uses order_id.
func linearChainItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Request A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/users"},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "Request B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/users/{{user_id}}/orders"},
			Extract: map[string]string{"order_id": "$.orders[0].id"},
		},
		{
			Name:    "Request C",
			Request: parser.Request{Method: "GET", URL: "http://example.com/orders/{{order_id}}"},
		},
	}
}

// diamondDependencyItems creates:
//
//	A extracts token
//	B uses token, extracts user_id
//	C uses token, extracts order_id
//	D uses user_id and order_id
func diamondDependencyItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Login",
			Request: parser.Request{Method: "POST", URL: "http://example.com/login"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:    "Get User",
			Request: parser.Request{Method: "GET", URL: "http://example.com/user", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "Get Orders",
			Request: parser.Request{Method: "GET", URL: "http://example.com/orders", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
			Extract: map[string]string{"order_id": "$.orders[0].id"},
		},
		{
			Name:    "Get Order Detail",
			Request: parser.Request{Method: "GET", URL: "http://example.com/users/{{user_id}}/orders/{{order_id}}"},
		},
	}
}

// requestsUsingPreExecVarsItems creates two requests using only pre-exec vars (should be parallel).
func requestsUsingPreExecVarsItems() []parser.RequestItem {
	return []parser.RequestItem{
		{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/users", Headers: map[string]string{"X-Key": "{{api_key}}"}}},
		{Name: "B", Request: parser.Request{Method: "GET", URL: "{{base_url}}/orders", Headers: map[string]string{"X-Key": "{{api_key}}"}}},
	}
}

// requestsUsingDynamicFunctionsItems creates two requests using only dynamic functions (should be parallel).
func requestsUsingDynamicFunctionsItems() []parser.RequestItem {
	return []parser.RequestItem{
		{Name: "A", Request: parser.Request{Method: "POST", URL: "http://example.com/a", Body: `{"id": "{{$uuid}}"}`}},
		{Name: "B", Request: parser.Request{Method: "POST", URL: "http://example.com/b", Body: `{"ts": "{{$timestamp}}"}`}},
	}
}

// circularRequestItems creates a circular dependency: A extracts x, B uses x and extracts y, A uses y.
// Since requests are in a list, we simulate this by having:
// A: extracts x, references y
// B: extracts y, references x
func circularRequestItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Request A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/{{y}}"},
			Extract: map[string]string{"x": "$.x"},
		},
		{
			Name:    "Request B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/{{x}}"},
			Extract: map[string]string{"y": "$.y"},
		},
	}
}

// collisionRequestItems creates two parallel requests extracting the same variable.
func collisionRequestItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Request A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"user": "$.user"},
		},
		{
			Name:    "Request B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b"},
			Extract: map[string]string{"user": "$.user"},
		},
	}
}

// singleRequestItems creates one request.
func singleRequestItems() []parser.RequestItem {
	return []parser.RequestItem{
		{Name: "Only Request", Request: parser.Request{Method: "GET", URL: "http://example.com"}},
	}
}

// multiVarEdgeItems creates: A extracts token and user_id, B uses both (single edge with two variables).
func multiVarEdgeItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Request A",
			Request: parser.Request{Method: "POST", URL: "http://example.com/login"},
			Extract: map[string]string{"token": "$.token", "user_id": "$.user.id"},
		},
		{
			Name:    "Request B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/users/{{user_id}}", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
		},
	}
}

// dynamicExtractKeyItems creates a request with a dynamic variable in the extract key.
func dynamicExtractKeyItems() []parser.RequestItem {
	return []parser.RequestItem{
		{
			Name:    "Bad Extract",
			Request: parser.Request{Method: "GET", URL: "http://example.com"},
			Extract: map[string]string{"{{prefix}}_id": "$.id"},
		},
	}
}

func TestAnalyze(t *testing.T) {
	tests := []struct {
		name        string
		items       []parser.RequestItem
		preExecVars map[string]bool
		wantWaves   [][]int
		wantValid   bool
		wantErrors  int
	}{
		{
			"independent requests all in wave 0",
			threeIndependentRequests(), nil,
			[][]int{{0, 1, 2}},
			true, 0,
		},
		{
			"linear chain A->B->C",
			linearChainItems(), nil,
			[][]int{{0}, {1}, {2}},
			true, 0,
		},
		{
			"diamond dependency",
			diamondDependencyItems(), nil,
			[][]int{{0}, {1, 2}, {3}},
			true, 0,
		},
		{
			"pre-exec vars do not create dependencies",
			requestsUsingPreExecVarsItems(), set("base_url", "api_key"),
			[][]int{{0, 1}},
			true, 0,
		},
		{
			"dynamic functions do not create dependencies",
			requestsUsingDynamicFunctionsItems(), nil,
			[][]int{{0, 1}},
			true, 0,
		},
		{
			"circular dependency detected",
			circularRequestItems(), nil,
			nil, false, 1,
		},
		{
			"variable collision detected",
			collisionRequestItems(), nil,
			nil, false, 1,
		},
		{
			"single request",
			singleRequestItems(), nil,
			[][]int{{0}},
			true, 0,
		},
		{
			"empty request list",
			nil, nil,
			[][]int{},
			true, 0,
		},
		{
			"dynamic variable name in extract rejected",
			dynamicExtractKeyItems(), nil,
			nil, false, 1,
		},
		{
			"multi-variable edge merges into single edge",
			multiVarEdgeItems(), nil,
			[][]int{{0}, {1}},
			true, 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := Analyze(tt.items, tt.preExecVars)

			if graph.IsValid != tt.wantValid {
				t.Errorf("Analyze().IsValid = %v, want %v\nerrors: %v", graph.IsValid, tt.wantValid, graph.Errors)
			}

			if len(graph.Errors) != tt.wantErrors {
				t.Errorf("Analyze() produced %d errors, want %d\nerrors: %v",
					len(graph.Errors), tt.wantErrors, graph.Errors)
			}

			if tt.wantWaves == nil {
				// Don't check waves for invalid graphs
				return
			}

			if len(graph.Waves) != len(tt.wantWaves) {
				t.Errorf("Analyze() produced %d waves, want %d\ngot:  %v\nwant: %v",
					len(graph.Waves), len(tt.wantWaves), graph.Waves, tt.wantWaves)
				return
			}

			for i := range tt.wantWaves {
				got := make([]int, len(graph.Waves[i]))
				copy(got, graph.Waves[i])
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

func TestAnalyze_NestedVariableResolutionWarning(t *testing.T) {
	tests := []struct {
		name         string
		preExecVals  map[string]string
		preExecVars  map[string]bool
		items        []parser.RequestItem
		wantWarnings int
	}{
		{
			"depth 3 - no warning",
			map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "value"},
			set("a", "b", "c"),
			[]parser.RequestItem{{Name: "R", Request: parser.Request{URL: "http://example.com/{{a}}"}}},
			0,
		},
		{
			"depth > 10 - produces warning",
			func() map[string]string {
				vals := make(map[string]string)
				for i := 0; i < 12; i++ {
					vals[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
				}
				vals["v12"] = "final"
				return vals
			}(),
			func() map[string]bool {
				s := make(map[string]bool)
				for i := 0; i <= 12; i++ {
					s[fmt.Sprintf("v%d", i)] = true
				}
				return s
			}(),
			[]parser.RequestItem{{Name: "R", Request: parser.Request{URL: "http://example.com/{{v0}}"}}},
			1,
		},
		{
			"no pre-exec values - no warning",
			nil,
			nil,
			[]parser.RequestItem{{Name: "R", Request: parser.Request{URL: "http://example.com/test"}}},
			0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			graph := Analyze(tt.items, tt.preExecVars, AnalyzeOptions{PreExecValues: tt.preExecVals})
			if len(graph.Warnings) != tt.wantWarnings {
				t.Errorf("warnings = %d, want %d: %v", len(graph.Warnings), tt.wantWarnings, graph.Warnings)
			}
		})
	}
}

func TestAnalyze_ExternalFileExtractionsIncluded(t *testing.T) {
	// Simulates what parser produces after resolving external files:
	// request items with extract blocks that came from external .yaml files.
	// The parallel analyzer should treat them identically to inline requests.
	items := []parser.RequestItem{
		{
			Name:    "Create User (external)",
			Request: parser.Request{Method: "POST", URL: "http://example.com/users"},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "Get User (inline)",
			Request: parser.Request{Method: "GET", URL: "http://example.com/users/{{user_id}}"},
		},
	}
	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	// Should have dependency: Create User -> Get User
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d", len(graph.Edges))
	}
	if graph.Edges[0].From != 0 || graph.Edges[0].To != 1 {
		t.Errorf("edge from %d to %d, want 0->1", graph.Edges[0].From, graph.Edges[0].To)
	}
	// Should be 2 waves: wave 0 [Create User], wave 1 [Get User]
	if len(graph.Waves) != 2 {
		t.Errorf("expected 2 waves, got %d: %v", len(graph.Waves), graph.Waves)
	}
}

func TestAnalyze_AuthProfileVarsNoDepCreated(t *testing.T) {
	// Multiple requests use {{admin_token}} which is in preExecVars (from auth profile).
	// No dependency should be created between them.
	items := []parser.RequestItem{
		{
			Name: "Create User",
			Request: parser.Request{
				Method:  "POST",
				URL:     "http://example.com/users",
				Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"},
			},
		},
		{
			Name: "Get Roles",
			Request: parser.Request{
				Method:  "GET",
				URL:     "http://example.com/roles",
				Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"},
			},
		},
	}
	graph := Analyze(items, set("admin_token"))
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	if len(graph.Waves) != 1 {
		t.Errorf("expected 1 wave (all parallel), got %d: %v", len(graph.Waves), graph.Waves)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected 0 edges, got %d: %+v", len(graph.Edges), graph.Edges)
	}
}

// TestAnalyze_MultiVarEdgeMerge verifies that when request A extracts two variables
// (token and user_id) and request B uses both, addEdge merges them into a single edge
// with two variables instead of creating two separate edges.
func TestAnalyze_MultiVarEdgeMerge(t *testing.T) {
	graph := Analyze(multiVarEdgeItems(), nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}

	// There should be exactly 1 edge (from A to B) with 2 variables
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %+v", len(graph.Edges), graph.Edges)
	}

	edge := graph.Edges[0]
	if edge.From != 0 || edge.To != 1 {
		t.Errorf("expected edge from 0 to 1, got from %d to %d", edge.From, edge.To)
	}
	if len(edge.Variables) != 2 {
		t.Errorf("expected 2 variables on edge, got %d: %v", len(edge.Variables), edge.Variables)
	}

	// Verify both variables are present (order may vary)
	varSet := make(map[string]bool, len(edge.Variables))
	for _, v := range edge.Variables {
		varSet[v] = true
	}
	if !varSet["token"] {
		t.Error("edge missing variable 'token'")
	}
	if !varSet["user_id"] {
		t.Error("edge missing variable 'user_id'")
	}
}

// TestAnalyze_ExplicitDependsOnCreatesEdge verifies that an explicit
// depends_on: declaration creates a wave edge even when no variable
// flows between the two requests (MANUAL.md section 5.6).
func TestAnalyze_ExplicitDependsOnCreatesEdge(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "First", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}},
		{Name: "Second", Request: parser.Request{Method: "GET", URL: "http://example.com/b"}, DependsOn: []string{"First"}},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}

	wantWaves := [][]int{{0}, {1}}
	if len(graph.Waves) != len(wantWaves) {
		t.Fatalf("expected %d waves, got %d: %v", len(wantWaves), len(graph.Waves), graph.Waves)
	}
	for i, want := range wantWaves {
		got := graph.Waves[i]
		if len(got) != len(want) || got[0] != want[0] {
			t.Errorf("wave %d: got %v, want %v", i, got, want)
		}
	}

	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %+v", len(graph.Edges), graph.Edges)
	}
	edge := graph.Edges[0]
	if edge.From != 0 || edge.To != 1 {
		t.Errorf("expected edge from 0 to 1, got from %d to %d", edge.From, edge.To)
	}
	if !edge.Explicit {
		t.Error("expected edge to be marked Explicit")
	}
	if len(edge.Variables) != 0 {
		t.Errorf("expected no variables on explicit edge, got %v", edge.Variables)
	}
}

// TestAnalyze_ExplicitDependsOnNoDuplicateEdge verifies that when a request
// both consumes a variable from its parent AND declares depends_on on it
// (possibly more than once), only the single variable edge remains.
func TestAnalyze_ExplicitDependsOnNoDuplicateEdge(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "Login",
			Request: parser.Request{Method: "POST", URL: "http://example.com/login"},
			Extract: map[string]string{"token": "$.token"},
		},
		{
			Name:      "Get User",
			Request:   parser.Request{Method: "GET", URL: "http://example.com/user", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
			DependsOn: []string{"Login", "Login"},
		},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	if len(graph.Edges) != 1 {
		t.Fatalf("expected 1 edge, got %d: %+v", len(graph.Edges), graph.Edges)
	}
	if len(graph.Edges[0].Variables) != 1 || graph.Edges[0].Variables[0] != "token" {
		t.Errorf("expected variable edge [token], got %v", graph.Edges[0].Variables)
	}
	if len(graph.Waves) != 2 {
		t.Errorf("expected 2 waves, got %d: %v", len(graph.Waves), graph.Waves)
	}
}

// TestAnalyze_ExplicitDependsOnCycleDetected verifies that a cycle formed
// purely by depends_on: declarations invalidates the graph.
func TestAnalyze_ExplicitDependsOnCycleDetected(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}, DependsOn: []string{"B"}},
		{Name: "B", Request: parser.Request{Method: "GET", URL: "http://example.com/b"}, DependsOn: []string{"A"}},
	}

	graph := Analyze(items, nil)
	if graph.IsValid {
		t.Fatal("expected invalid graph for depends_on cycle")
	}
	if len(graph.Errors) == 0 {
		t.Fatal("expected at least one error describing the cycle")
	}
}

// TestAnalyze_ExplicitDependsOnUnknownNameIgnored verifies that depends_on
// names not present in the analyzed item slice (e.g. setup/teardown items,
// which phase ordering already sequences, or items removed by --only) do
// not create edges and do not invalidate the graph.
func TestAnalyze_ExplicitDependsOnUnknownNameIgnored(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "Only Request", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}, DependsOn: []string{"Seed token"}},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected no edges, got %+v", graph.Edges)
	}
	if len(graph.Waves) != 1 {
		t.Errorf("expected 1 wave, got %d: %v", len(graph.Waves), graph.Waves)
	}
}

// TestAnalyze_ExplicitDependsOnSelfIgnored verifies that a self-referential
// depends_on entry is ignored rather than reported as a cycle, matching the
// sequential path where it is harmless.
func TestAnalyze_ExplicitDependsOnSelfIgnored(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "Solo", Request: parser.Request{Method: "GET", URL: "http://example.com/a"}, DependsOn: []string{"Solo"}},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	if len(graph.Edges) != 0 {
		t.Errorf("expected no edges, got %+v", graph.Edges)
	}
}

// TestAnalyze_ExplicitDependsOnSuppressesCollision verifies that two
// producers of the same variable are not flagged as a collision when
// depends_on sequences them.
func TestAnalyze_ExplicitDependsOnSuppressesCollision(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "Create A",
			Request: parser.Request{Method: "POST", URL: "http://example.com/a"},
			Extract: map[string]string{"id": "$.id"},
		},
		{
			Name:      "Create B",
			Request:   parser.Request{Method: "POST", URL: "http://example.com/b"},
			Extract:   map[string]string{"id": "$.id"},
			DependsOn: []string{"Create A"},
		},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph (collision suppressed by depends_on), got errors: %v", graph.Errors)
	}
	if len(graph.Waves) != 2 {
		t.Errorf("expected 2 waves, got %d: %v", len(graph.Waves), graph.Waves)
	}
}

// TestAnalyze_ExplicitDependsOnInAncestorClosure verifies that depends_on
// parents are reachable via AncestorClosure, so --only minimal-setup
// pruning keeps explicitly depended-on setup items.
func TestAnalyze_ExplicitDependsOnInAncestorClosure(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "Seed", Request: parser.Request{Method: "POST", URL: "http://example.com/seed"}, Extract: map[string]string{"seed_id": "$.id"}},
		{Name: "Main", Request: parser.Request{Method: "GET", URL: "http://example.com/main"}, DependsOn: []string{"Seed"}},
	}

	graph := Analyze(items, nil)
	if !graph.IsValid {
		t.Fatalf("expected valid graph, got errors: %v", graph.Errors)
	}
	reachable := AncestorClosure(graph, []int{1})
	if !reachable[0] {
		t.Errorf("expected depends_on parent (index 0) in ancestor closure, got %v", reachable)
	}
}
