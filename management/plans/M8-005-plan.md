# Implementation Plan: M8-005

## Overview
When `--only` is set, prune the setup phase to the transitive closure of
`{{variable}}` references starting from the selected main requests, using a new
`parallel.AncestorClosure` graph primitive. No-extract setup items (pure
seeders) are always kept, the analyzer-invalid path falls back to full setup
with a stderr diagnostic, and teardown/without-`--only` semantics are
byte-identical to M8-004.

## Task Details
- **ID:** M8-005
- **Title:** run --only: prune setup to transitive closure of {{variable}} references
- **Phase:** M8: Developer & Agent Exploration Experience
- **Priority:** 1
- **Complexity:** low
- **Estimated effort:** 1 day
- **Branch:** `feature/M8-005-only-prune-setup`

## Dependencies

| Task   | Title                                                                                     | Status |
|--------|-------------------------------------------------------------------------------------------|--------|
| M8-004 | run --only "<name>" for single-request execution with duplicate-name rejection            | done   |

---

## Code Exploration Summary

### Where the new primitive goes
- `internal/parallel/waves.go` already contains a forward-traversal helper
  `hasTransitiveDep(graph, a, b)` at lines 51-71 that does DFS over
  `graph.Nodes[current].Dependencies`. `AncestorClosure` is the natural
  companion: it walks the **same** `Dependencies` edges (which point from
  consumer → producer) but collects *all* reachable nodes via BFS from a set
  of start indices. Colocate with `hasTransitiveDep` in `waves.go` rather
  than create a new file — the existing helper already makes this file the
  "graph traversal primitives" home. A reviewer can always split later if
  `waves.go` grows too large.
- Exported API: `func AncestorClosure(g *DependencyGraph, starts []int) map[int]bool`.
  Returns a set inclusive of starts. `map[int]bool` matches the existing
  `Dependencies map[int]bool` idiom inside `RequestNode`.

### Where the runner pruning block goes
- `internal/runner/runner.go:455-477` — the existing M8-004 `--only` filter
  block. The pruning logic must land **after** `filterMainItems` succeeds
  (so `filtered` exists and we know there is ≥1 main item to anchor the
  closure on) and **before** the shallow-copy replacement (we want to
  replace both `col.Setup.Items` and `col.Requests.Items` in one
  shallow-copy pass).
- `VarSources.fullMainForCliff` already exists at runner.go:302-305.
  Add a sibling `fullSetupForCliff []parser.RequestItem` field (same
  caller-leave-nil contract) so `enrichSelectionCliff` can scan setup
  producers too.
- `enrichSelectionCliff(err, fullMainItems, selection)` at runner.go:633-692
  needs a **new parameter** `fullSetupItems []parser.RequestItem`. After
  failing to find a producer in fullMainItems it scans fullSetupItems; if
  found there, the error message indicates the analyzer missed a reference.
  This path is expected to be unreachable in practice; it is a safety net.
- All **three** call sites of `enrichSelectionCliff` (found via grep) must
  pass the new parameter. Currently wrapped inside `executePhase`-adjacent
  error-return paths; sweep is small.

### Where the preExecVars helper goes
- `executeParallelMain` at runner.go:1077-1083 already does:
  ```go
  resolved := scope.Resolved()
  preExecVars := make(map[string]bool, len(resolved))
  for k := range resolved {
      preExecVars[k] = true
  }
  ```
- Lift into an unexported helper `buildPreExecVarSet(scope *variable.Scope) map[string]bool`
  at a stable spot in runner.go (just above `executeParallelMain`). Reuse in
  the new pruning block and in `executeParallelMain`. This is a pure
  refactor — identical semantics, one source of truth.
- Caveat: `scope.Resolved()` returns a copy of the *current* scope. Auth
  profiles have already run at the pruning point (auth runs at runner.go:432-452,
  before the `--only` block). This matches the parallel executor's contract
  and is exactly what we want — variables available before any phase runs.

### Where the diagnostic stream comes from
- The runner currently **does not** accept a stderr writer. There are zero
  `Fprintln(os.Stderr)` calls in `internal/runner/*.go`. Adding one directly
  would (a) couple the runner to `os.Stderr` globally and (b) make tests
  flaky.
- Cleanest injection: add an `io.Writer` field `Diagnostics` to `VarSources`.
  When nil (zero-value default), no diagnostic is emitted. cmd/curlew
  passes `stderr` (already threaded as `io.Writer` through `runCmd`).
  Tests pass a `*bytes.Buffer` and assert on it.
- Verbosity: the task says "verbosity: normal and above". The runner has
  no verbosity knowledge. Reinterpretation: the cmd layer decides whether
  to suppress the stream by passing `io.Discard` in quiet mode. This keeps
  the runner simple and matches how the M7 output discipline work resolved
  similar questions (per M7-005, stderr writers are threaded into `runCmd`).
  Plan decision documented below.

### Where Selection indices map back to combinedItems
- `filtered` is a `[]parser.RequestItem` produced by `filterMainItemsBySelection`.
  Items retain their identity; we just need indices in `combinedItems`
  (which is `setup ++ filtered`). Since filtered items come *after* all
  setup items, their combined-graph index is `len(col.Setup.Items) + i`
  for `i` in `0..len(filtered)-1`. No complex lookup needed.

### What the cliff diagnostic needs to learn
- Current `enrichSelectionCliff` searches only `fullMainItems`. If a pruned
  setup item produced the missing var, the error says "normally extracted
  from X which was not included by --only" — wrong, because X was not
  named by --only directly; it was pruned by the closure.
- Extension: after failing to find the producer in main, scan
  `fullSetupItems`. If found: enrich with a message explicitly tagged as a
  likely-analyzer-bug path. Suffix: `" (producer was in setup but the
  minimal-setup analysis did not include it — this is likely an curlew
  bug; please report)"`.
- This path is unreachable when the analyzer is correct, because setup
  producers referenced by the selection would be included in the closure.
  It only fires when the analyzer misses a reference (e.g. a scanner
  false-negative on a new interpolation surface we haven't wired).

### Tests expected to break
**None.** The regression tests in `TestRunner_OnlyFilter` all use
setup items with empty `Extract:` maps, so they fall under the
"no-extract setup always runs" clause of the new logic. A diff with
M8-004 behaviour is expected for collections where a setup item
*has* an `extract:` block — but no existing test is in that shape.

`TestRun_OnlyVariableCliff` is an existing cliff test. With the
new signature on `enrichSelectionCliff` we must pass a nil/empty
fullSetupItems (the collection has no setup items) — semantics
identical.

---

## Implementation Steps

### Step 1: Add `parallel.AncestorClosure` primitive (smallest blast radius, no downstream callers yet)
**Rationale:** Pure in-package addition with full unit tests. No caller touches it until Step 2. Easiest to unit-test exhaustively.

#### Files to Modify

| File                                            | Action  | Description                                                                 |
|-------------------------------------------------|---------|-----------------------------------------------------------------------------|
| `internal/parallel/waves.go`                    | modify  | Add `AncestorClosure` exported function below `hasTransitiveDep`.          |
| `internal/parallel/waves_test.go`               | modify  | Add `TestParallel_AncestorClosure` table-driven test.                      |

#### Current Code (waves.go:51-71)
```go
// hasTransitiveDep returns true if node a transitively depends on node b.
func hasTransitiveDep(graph *DependencyGraph, a, b int) bool {
	visited := make(map[int]bool)
	return dfsReach(graph, a, b, visited)
}

func dfsReach(graph *DependencyGraph, current, target int, visited map[int]bool) bool {
	if current == target {
		return true
	}
	if visited[current] {
		return false
	}
	visited[current] = true
	for dep := range graph.Nodes[current].Dependencies {
		if dfsReach(graph, dep, target, visited) {
			return true
		}
	}
	return false
}
```

#### New Code (append to waves.go)
```go
// AncestorClosure returns the set of node indices reachable by following
// the Dependencies edges (consumer -> producer) starting from the given
// start indices. The returned set is inclusive of the starts. Out-of-range
// start indices are ignored. Uses reverse BFS (iterative) so it is safe on
// deep graphs without blowing the stack.
//
// Given the graph:
//
//	A   <- B <- C
//	            ^
//	            D
//
// AncestorClosure from [C] yields {A, B, C}; from [C, D] yields {A, B, C, D}.
// An empty starts slice returns an empty (non-nil) map.
func AncestorClosure(graph *DependencyGraph, starts []int) map[int]bool {
	reachable := make(map[int]bool, len(starts))
	if graph == nil || len(graph.Nodes) == 0 || len(starts) == 0 {
		return reachable
	}
	queue := make([]int, 0, len(starts))
	for _, s := range starts {
		if s < 0 || s >= len(graph.Nodes) {
			continue
		}
		if !reachable[s] {
			reachable[s] = true
			queue = append(queue, s)
		}
	}
	for len(queue) > 0 {
		cur := queue[0]
		queue = queue[1:]
		for dep := range graph.Nodes[cur].Dependencies {
			if !reachable[dep] {
				reachable[dep] = true
				queue = append(queue, dep)
			}
		}
	}
	return reachable
}
```

#### Tests to Write FIRST (RED phase)
```go
// TestParallel_AncestorClosure verifies reverse-BFS over Dependencies edges
// returns the correct ancestor set for linear-chain, diamond, disconnected,
// and empty-starts shapes.
func TestParallel_AncestorClosure(t *testing.T) {
	// Helper: build {0,1,2,...,n-1} as a fresh set for comparison.
	idxSet := func(indices ...int) map[int]bool {
		m := make(map[int]bool, len(indices))
		for _, i := range indices {
			m[i] = true
		}
		return m
	}
	// Shapes:
	linear := linearChainWaveGraph(4) // 0 <- 1 <- 2 <- 3 (Dependencies)
	diamond := diamondPatternWaveGraph()
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
		{"linear chain from leaf includes all", linear, []int{3}, idxSet(0, 1, 2, 3)},
		{"linear chain from middle includes self and ancestors", linear, []int{2}, idxSet(0, 1, 2)},
		{"linear chain from root is singleton", linear, []int{0}, idxSet(0)},
		{"diamond from sink includes full diamond", diamond, []int{3}, idxSet(0, 1, 2, 3)},
		{"diamond from one branch skips other branch", diamond, []int{1}, idxSet(0, 1)},
		{"diamond from root is singleton", diamond, []int{0}, idxSet(0)},
		{"disconnected reaches only its own component", disconnected, []int{1}, idxSet(0, 1)},
		{"disconnected from multiple starts unions components", disconnected, []int{1, 3}, idxSet(0, 1, 2, 3)},
		{"empty starts yields empty set", linear, nil, idxSet()},
		{"out-of-range start ignored", linear, []int{99}, idxSet()},
		{"duplicate start indices deduped", linear, []int{3, 3, 2}, idxSet(0, 1, 2, 3)},
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
```

#### Impact on Existing Tests
- None. Pure addition.

---

### Step 2: Lift `preExecVars` into a helper and add `fullSetupForCliff` field
**Rationale:** Mechanical refactor that two later steps depend on. Landing it separately keeps diffs clean.

#### Files to Modify

| File                          | Action | Description                                                                |
|-------------------------------|--------|----------------------------------------------------------------------------|
| `internal/runner/runner.go`   | modify | Add unexported helper `buildPreExecVarSet(scope) map[string]bool`; call from `executeParallelMain`. Add `fullSetupForCliff []parser.RequestItem` field and `Diagnostics io.Writer` field on `VarSources`. |

#### Current Code (runner.go:1077-1083 inside executeParallelMain)
```go
// Build pre-execution variable set from the scope's resolved vars
resolved := scope.Resolved()
preExecVars := make(map[string]bool, len(resolved))
for k := range resolved {
	preExecVars[k] = true
}
```

#### New Code
Add near line 305 on `VarSources`:
```go
// Diagnostics receives one-line, human-readable warnings the runner emits
// outside the structured error path (e.g. "--only minimal-setup analysis
// failed, falling back to full setup"). When nil (default), no
// diagnostics are emitted. The cmd layer threads stderr here; tests pass
// a *bytes.Buffer. M8-005.
Diagnostics io.Writer

// fullSetupForCliff is internal state populated by Run so the --only
// variable-cliff diagnostic can look up setup-phase producers after a
// minimal-setup prune. Callers should leave this nil.
fullSetupForCliff []parser.RequestItem
```

Add a helper near the bottom of the file, above `executeParallelMain`:
```go
// buildPreExecVarSet returns the name set of all variables already resolved
// on scope. Used to seed parallel.Analyze so that consumer->producer edges
// only form for variables that are *not* already available at phase start.
func buildPreExecVarSet(scope *variable.Scope) map[string]bool {
	resolved := scope.Resolved()
	out := make(map[string]bool, len(resolved))
	for k := range resolved {
		out[k] = true
	}
	return out
}
```

Replace the inline block in `executeParallelMain`:
```go
preExecVars := buildPreExecVarSet(scope)
```

Add `"io"` to the import block (if not already present — verify).

#### Tests to Write FIRST
- A unit test in `runner_test.go` named `TestRunner_BuildPreExecVarSet` that
  builds a scope with known vars and asserts set equality. Pure function, trivial.

```go
func TestRunner_BuildPreExecVarSet(t *testing.T) {
	scope := variable.NewScope(map[string]string{"foo": "1", "bar": "2"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := buildPreExecVarSet(scope)
	want := map[string]bool{"foo": true, "bar": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %q in %v", k, got)
		}
	}
}
```

#### Impact on Existing Tests
- None. `executeParallelMain` behaviour is byte-identical (pure refactor).
- Existing `TestRunner_OnlyFilter` continues to pass (new fields default to zero).

---

### Step 3: Prune setup on `--only` + fallback on analyzer invalid (the core vertical slice)
**Rationale:** The main feature. Depends on Steps 1 and 2. Gated behind the existing `len(vars.Selection) > 0` branch so callers without `--only` see zero change.

#### Files to Modify

| File                          | Action | Description                                                                                    |
|-------------------------------|--------|------------------------------------------------------------------------------------------------|
| `internal/runner/runner.go`   | modify | Extend runner.go:455-477 with setup-pruning logic. Populate `vars.fullSetupForCliff`.          |
| `internal/runner/runner_test.go` | modify | Add `TestRunner_OnlyPrunesSetup`, `..._ChainedProducers`, `..._NoExtractSeeder`, `..._AnalyzerFallback`. |

#### Current Code (runner.go:455-477)
```go
// M8-004: apply --only selection to main-phase items. Setup and teardown
// are unaffected. When Selection is set but yields zero matches, fail with
// a structured error before any HTTP runs. Capture the full main items
// first so the variable-cliff diagnostic can scan producers over the
// unfiltered list.
if len(vars.Selection) > 0 {
	vars.fullMainForCliff = col.Requests.Items
	filtered, filterErr := filterMainItemsBySelection(col.Requests.Items, vars.Selection)
	if filterErr != nil {
		// No main items matched: total for run.end should reflect only
		// setup + teardown (zero main items would have run).
		emptySummary.Total = len(col.Setup.Items) + len(col.Teardown.Items)
		return nil, emptySummary, filterErr
	}
	// Shallow-copy col so we replace Requests.Items without mutating the
	// caller's parsed collection. Setup and Teardown remain shared pointers
	// (no copy needed — we only read them).
	shallow := *col
	shallow.Requests = parser.Section{Retry: col.Requests.Retry, Items: filtered}
	col = &shallow
	// Update total to reflect the filtered main count.
	emptySummary.Total = len(col.Setup.Items) + len(filtered) + len(col.Teardown.Items)
}
```

#### New Code (replaces block above)
```go
// M8-004 + M8-005: apply --only selection to main-phase items and prune
// the setup phase to the transitive closure of {{variable}} references
// from the selection. Teardown is never pruned. When Selection yields
// zero matches, fail with a structured error before any HTTP runs.
// Capture the full main items so the variable-cliff diagnostic can scan
// producers over the unfiltered list.
if len(vars.Selection) > 0 {
	vars.fullMainForCliff = col.Requests.Items
	vars.fullSetupForCliff = col.Setup.Items
	filtered, filterErr := filterMainItemsBySelection(col.Requests.Items, vars.Selection)
	if filterErr != nil {
		// No main items matched: total for run.end should reflect only
		// setup + teardown (zero main items would have run).
		emptySummary.Total = len(col.Setup.Items) + len(col.Teardown.Items)
		return nil, emptySummary, filterErr
	}

	// M8-005: prune setup to the transitive closure of {{variable}}
	// references from the selected main items. Items with an empty
	// Extract: map are included unconditionally (pure seeders the
	// analyzer cannot prove unneeded).
	prunedSetup := col.Setup.Items
	if len(col.Setup.Items) > 0 {
		preExecVars := buildPreExecVarSet(scope)
		combined := make([]parser.RequestItem, 0, len(col.Setup.Items)+len(filtered))
		combined = append(combined, col.Setup.Items...)
		combined = append(combined, filtered...)
		graph := parallel.Analyze(combined, preExecVars)
		if !graph.IsValid {
			// Fallback: analyzer rejected the combined DAG (e.g. cycles,
			// collisions, duplicate producers). Run full setup and emit
			// a one-line diagnostic on stderr so the user knows pruning
			// was skipped.
			if vars.Diagnostics != nil {
				_, _ = fmt.Fprintf(vars.Diagnostics,
					"curlew: --only minimal-setup analysis failed (%s); running full setup\n",
					strings.Join(graph.Errors, "; "),
				)
			}
		} else {
			// Filtered main items come after setup items in the combined
			// slice, so their combined-graph indices are:
			//   len(col.Setup.Items) + i  for i in 0..len(filtered)-1
			starts := make([]int, 0, len(filtered))
			base := len(col.Setup.Items)
			for i := range filtered {
				starts = append(starts, base+i)
			}
			reachable := parallel.AncestorClosure(graph, starts)
			pruned := make([]parser.RequestItem, 0, len(col.Setup.Items))
			for i, item := range col.Setup.Items {
				// Keep if the analyzer placed it in the closure, OR if it
				// has no extract: block (pure seeder — the analyzer cannot
				// prove it unneeded so we conservatively always run it).
				if reachable[i] || len(item.Extract) == 0 {
					pruned = append(pruned, item)
				}
			}
			prunedSetup = pruned
		}
	}

	// Shallow-copy col so we replace Setup.Items and Requests.Items without
	// mutating the caller's parsed collection. Teardown remains shared.
	shallow := *col
	shallow.Setup = parser.Section{Retry: col.Setup.Retry, Items: prunedSetup}
	shallow.Requests = parser.Section{Retry: col.Requests.Retry, Items: filtered}
	col = &shallow
	// Update total to reflect the filtered main count AND pruned setup count.
	emptySummary.Total = len(prunedSetup) + len(filtered) + len(col.Teardown.Items)
}
```

#### Tests to Write FIRST

```go
// TestRunner_OnlyPrunesSetup verifies that when --only is set, the setup
// phase is pruned to the transitive closure of {{variable}} references
// from the selected main items. Setup items unreferenced by the closure
// and carrying an Extract: block are filtered out.
func TestRunner_OnlyPrunesSetup(t *testing.T) {
	// setup: [Login (extract token), Seed users (extract user_id),
	//         Seed posts (uses {{user_id}}, extract post_id),
	//         Warm cache (no extract)]
	// main:  [Get user (uses {{token}}), List posts (uses {{post_id}})]
	login := parser.RequestItem{
		Name:    "Login",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/login"},
		Extract: map[string]string{"token": "$.token"},
	}
	seedUsers := parser.RequestItem{
		Name:    "Seed users",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/users"},
		Extract: map[string]string{"user_id": "$.id"},
	}
	seedPosts := parser.RequestItem{
		Name:    "Seed posts",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/users/{{user_id}}/posts"},
		Extract: map[string]string{"post_id": "$.id"},
	}
	warm := parser.RequestItem{
		Name:    "Warm cache",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/health"},
		// no Extract
	}
	getUser := parser.RequestItem{
		Name:    "Get user",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/user", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
	}
	listPosts := parser.RequestItem{
		Name:    "List posts",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/posts/{{post_id}}"},
	}

	col := &parser.Collection{
		Name:     "Prune",
		Setup:    parser.Section{Items: []parser.RequestItem{login, seedUsers, seedPosts, warm}},
		Requests: parser.Section{Items: []parser.RequestItem{getUser, listPosts}},
	}

	bodyByPath := map[string]string{
		"/login":               `{"token":"abc"}`,
		"/users":               `{"id":"u1"}`,
		"/users/u1/posts":      `{"id":"p1"}`,
		"/health":              `{"status":"ok"}`,
		"/user":                `{"name":"x"}`,
		"/posts/p1":            `{"title":"y"}`,
	}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		// Strip query, extract path portion.
		url := req.URL
		path := url
		if i := strings.Index(url, "ex.com"); i >= 0 {
			path = url[i+6:]
		}
		body, ok := bodyByPath[path]
		if !ok {
			return &httpexec.Result{StatusCode: 404, Body: []byte(`{}`)}, nil
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(body)}, nil
	}

	t.Run("Get user prunes seed users and seed posts", func(t *testing.T) {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, exec, VarSources{
			Selection: []string{"Get user"},
			OnEvent:   sink,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sink.mu.Lock()
		defer sink.mu.Unlock()
		got := make([]string, len(sink.starts))
		for i, ev := range sink.starts {
			got[i] = ev.Name
		}
		want := []string{"Login", "Warm cache", "Get user"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("executed = %v, want %v", got, want)
		}
	})

	t.Run("List posts prunes Login only", func(t *testing.T) {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, exec, VarSources{
			Selection: []string{"List posts"},
			OnEvent:   sink,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sink.mu.Lock()
		defer sink.mu.Unlock()
		got := make([]string, len(sink.starts))
		for i, ev := range sink.starts {
			got[i] = ev.Name
		}
		want := []string{"Seed users", "Seed posts", "Warm cache", "List posts"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("executed = %v, want %v", got, want)
		}
	})
}

// TestRunner_OnlyPrunesSetup_ChainedProducers verifies the transitive
// closure traverses setup->setup producer chains (already covered by the
// List posts sub-test above, but isolated here as a single-path test for
// explicit DoD clarity).
func TestRunner_OnlyPrunesSetup_ChainedProducers(t *testing.T) {
	// A (extract a) -> B (uses a, extract b) -> C (uses b, extract c) -> main (uses c)
	A := parser.RequestItem{
		Name: "A", Request: parser.Request{Method: "GET", URL: "https://ex.com/a"},
		Extract: map[string]string{"a": "$.a"},
	}
	B := parser.RequestItem{
		Name: "B", Request: parser.Request{Method: "GET", URL: "https://ex.com/b/{{a}}"},
		Extract: map[string]string{"b": "$.b"},
	}
	C := parser.RequestItem{
		Name: "C", Request: parser.Request{Method: "GET", URL: "https://ex.com/c/{{b}}"},
		Extract: map[string]string{"c": "$.c"},
	}
	unused := parser.RequestItem{
		Name: "Unused setup", Request: parser.Request{Method: "GET", URL: "https://ex.com/skipme"},
		Extract: map[string]string{"unused": "$.x"},
	}
	main := parser.RequestItem{
		Name: "Main", Request: parser.Request{Method: "GET", URL: "https://ex.com/main/{{c}}"},
	}
	col := &parser.Collection{
		Setup:    parser.Section{Items: []parser.RequestItem{A, B, C, unused}},
		Requests: parser.Section{Items: []parser.RequestItem{main}},
	}
	exec := fixedBodyExec(map[string]string{
		"/a": `{"a":"1"}`, "/b/1": `{"b":"2"}`, "/c/2": `{"c":"3"}`, "/main/3": `{}`, "/skipme": `{}`,
	})

	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection: []string{"Main"},
		OnEvent:   sink,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	want := []string{"A", "B", "C", "Main"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("executed = %v, want %v (Unused setup must be pruned)", got, want)
	}
}

// TestRunner_OnlyPrunesSetup_NoExtractSeeder verifies setup items with
// empty extract: are included unconditionally even when --only targets a
// request that references none of their vars.
func TestRunner_OnlyPrunesSetup_NoExtractSeeder(t *testing.T) {
	seeder := parser.RequestItem{
		Name:    "Seeder",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/seed"},
		// No Extract field — pure side-effect seeder.
	}
	main := parser.RequestItem{
		Name:    "Main",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/main"},
	}
	col := &parser.Collection{
		Setup:    parser.Section{Items: []parser.RequestItem{seeder}},
		Requests: parser.Section{Items: []parser.RequestItem{main}},
	}
	exec := fixedBodyExec(map[string]string{"/seed": `{}`, "/main": `{}`})

	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection: []string{"Main"},
		OnEvent:   sink,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	if !sliceContains(got, "Seeder") {
		t.Errorf("no-extract seeder must always run; executed = %v", got)
	}
}

// TestRunner_OnlyPrunesSetup_AnalyzerFallback verifies that when
// parallel.Analyze returns IsValid==false on the combined DAG, the runner
// falls back to running the full setup and emits a diagnostic line on
// Diagnostics.
func TestRunner_OnlyPrunesSetup_AnalyzerFallback(t *testing.T) {
	// Two setup items extract the same variable — analyzer flags it as a
	// parallel-variable collision, IsValid==false.
	setup1 := parser.RequestItem{
		Name: "S1", Request: parser.Request{Method: "GET", URL: "https://ex.com/1"},
		Extract: map[string]string{"dup": "$.x"},
	}
	setup2 := parser.RequestItem{
		Name: "S2", Request: parser.Request{Method: "GET", URL: "https://ex.com/2"},
		Extract: map[string]string{"dup": "$.x"},
	}
	main := parser.RequestItem{
		Name: "Main", Request: parser.Request{Method: "GET", URL: "https://ex.com/main/{{dup}}"},
	}
	col := &parser.Collection{
		Setup:    parser.Section{Items: []parser.RequestItem{setup1, setup2}},
		Requests: parser.Section{Items: []parser.RequestItem{main}},
	}
	exec := fixedBodyExec(map[string]string{"/1": `{"x":"v"}`, "/2": `{"x":"v"}`, "/main/v": `{}`})

	diag := &bytes.Buffer{}
	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection:   []string{"Main"},
		OnEvent:     sink,
		Diagnostics: diag,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Full setup should have run (fallback).
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	want := []string{"S1", "S2", "Main"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("fallback must run full setup; executed = %v, want %v", got, want)
	}
	// Diagnostic line must be emitted.
	if !strings.Contains(diag.String(), "minimal-setup analysis failed") {
		t.Errorf("expected diagnostic 'minimal-setup analysis failed' on Diagnostics, got %q", diag.String())
	}
}

// TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly verifies that without
// --only, the setup execution is byte-identical to pre-M8-005 behaviour.
func TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly(t *testing.T) {
	// Same collection as TestRunner_OnlyPrunesSetup but no Selection.
	col := makeRichCollection(t) // helper returning the four-setup/two-main collection
	exec := fixedBodyExec(richCollectionBodies())
	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{OnEvent: sink})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	// All 4 setup items + both main items, in order.
	want := []string{"Login", "Seed users", "Seed posts", "Warm cache", "Get user", "List posts"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("without --only, execution must be unchanged; got %v want %v", got, want)
	}
}
```

Test helpers to add (co-located with the tests):
```go
// fixedBodyExec returns an ExecuteFunc that serves a map from URL path
// suffix to response body. Any path not in the map yields a 404 empty body.
func fixedBodyExec(bodies map[string]string) ExecuteFunc {
	return func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		path := req.URL
		if i := strings.Index(req.URL, "ex.com"); i >= 0 {
			path = req.URL[i+6:]
		}
		if b, ok := bodies[path]; ok {
			return &httpexec.Result{StatusCode: 200, Body: []byte(b)}, nil
		}
		return &httpexec.Result{StatusCode: 404, Body: []byte(`{}`)}, nil
	}
}

// eventNames extracts Name values from a slice of RequestEvents (recordingSink.starts).
func eventNames(evs []RequestEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Name
	}
	return out
}
```

#### Impact on Existing Tests
- `TestRunner_OnlyFilter` — all sub-tests use setup items with empty `Extract:` maps, so they fall under the "no-extract always runs" clause. Byte-identical behaviour. Should pass unmodified.
- `TestRun_OnlyVariableCliff` — collections have no setup phase. Byte-identical. Passes unmodified.
- `TestFilterMainItems` — tests the exported wrapper only. Untouched.

---

### Step 4: Extend cliff diagnostic to scan setup producers
**Rationale:** Safety-net path for when the analyzer misses a reference. Depends on Step 2 (`fullSetupForCliff` field). Keeps the pruning feature complete per the behavior spec.

#### Files to Modify

| File                           | Action | Description                                                                                |
|--------------------------------|--------|--------------------------------------------------------------------------------------------|
| `internal/runner/runner.go`    | modify | Change `enrichSelectionCliff` signature to accept `fullSetupItems`. Update three callers.  |
| `internal/runner/runner_test.go` | modify | Add `TestRunner_OnlyCliff_SetupProducer`.                                                |

#### Current Code (runner.go:638)
```go
func enrichSelectionCliff(err error, fullMainItems []parser.RequestItem, selection []string) error {
```

#### New Code
```go
// enrichSelectionCliff looks for an undefined-variable error in err's chain and,
// when --only is active, checks whether the missing variable's producer is a
// filtered-out main item. When the producer is not found in fullMainItems, it
// additionally scans fullSetupItems — in that case the analyzer missed a
// {{variable}} reference, which is a likely curlew bug; the enriched error
// notes that. Returns err unchanged when Selection is empty, when the error
// is not an undefined-variable error, or when no producer is found.
func enrichSelectionCliff(err error, fullMainItems, fullSetupItems []parser.RequestItem, selection []string) error {
	if err == nil || len(selection) == 0 {
		return err
	}
	if len(fullMainItems) == 0 && len(fullSetupItems) == 0 {
		return err
	}
	var s *apierrors.Structured
	if !errors.As(err, &s) {
		return err
	}
	if !errors.Is(err, variable.ErrUndefinedVariable) {
		return err
	}
	m := undefinedVarRe.FindStringSubmatch(s.Message)
	if len(m) != 2 {
		return err
	}
	varName := m[1]

	// Build the set of selected names.
	sel := make(map[string]struct{}, len(selection))
	for _, n := range selection {
		sel[n] = struct{}{}
	}

	// First, scan full main items (existing behaviour).
	for _, it := range fullMainItems {
		produced, _ := parallel.ExtractProducedVars(it.Extract)
		if !produced[varName] {
			continue
		}
		if _, inSelection := sel[it.Name]; inSelection {
			return err // producer IS selected — different failure mode
		}
		msg := fmt.Sprintf(
			"variable {{%s}} is not defined; normally extracted from %q which was not included by --only",
			varName, it.Name,
		)
		hint := fmt.Sprintf(
			"Add the producer to --only (e.g. --only %q --only %q) or pass the variable explicitly via --var %s=<value>.",
			it.Name, selection[0], varName,
		)
		return &apierrors.Structured{
			Category: s.Category,
			FilePath: s.FilePath,
			Line:     s.Line,
			Code:     s.Code,
			Message:  msg,
			Hint:     hint,
			Inner:    err,
		}
	}

	// M8-005 safety net: scan setup items. A producer found here indicates
	// the minimal-setup analyser missed a {{variable}} reference — likely a
	// bug (the scanner should have kept the setup item in the closure).
	for _, it := range fullSetupItems {
		produced, _ := parallel.ExtractProducedVars(it.Extract)
		if !produced[varName] {
			continue
		}
		msg := fmt.Sprintf(
			"variable {{%s}} is not defined; normally extracted from setup request %q which was pruned by --only (the minimal-setup analyser did not detect a reference; this is likely an curlew bug — please report)",
			varName, it.Name,
		)
		hint := fmt.Sprintf(
			"Workaround: pass --var %s=<value> or disable minimal-setup pruning by omitting --only.",
			varName,
		)
		return &apierrors.Structured{
			Category: s.Category,
			FilePath: s.FilePath,
			Line:     s.Line,
			Code:     s.Code,
			Message:  msg,
			Hint:     hint,
			Inner:    err,
		}
	}
	return err
}
```

Update all three call sites of `enrichSelectionCliff` in runner.go to pass
`vars.fullSetupForCliff`. Grep before edit to confirm the count.

#### Tests to Write FIRST
```go
// TestRunner_OnlyCliff_SetupProducer verifies that when an undefined
// variable's producer is in the (pruned) setup phase, the cliff diagnostic
// names it and flags the case as an analyzer miss (likely bug). This path
// is expected to be unreachable in practice with a correct analyzer.
func TestRunner_OnlyCliff_SetupProducer(t *testing.T) {
	// Simulate an analyser miss by constructing the scenario manually: a
	// setup item extracts var_x, but we rig the collection so main
	// references {{var_x}} via a field the scanner skips. The easiest
	// lever is to synthesise the pruned collection directly rather than
	// depend on a scanner hole — so test enrichSelectionCliff as a unit.
	setupItem := parser.RequestItem{
		Name:    "Seed",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/seed"},
		Extract: map[string]string{"var_x": "$.x"},
	}
	mainItem := parser.RequestItem{
		Name:    "Use",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/use"},
	}
	// Craft the exact error that enrichSelectionCliff expects.
	srcErr := &apierrors.Structured{
		Category: apierrors.CategoryVariable,
		Code:     "UNDEFINED_VARIABLE",
		Message:  `undefined variable "var_x"`,
		Inner:    variable.ErrUndefinedVariable,
	}

	enriched := enrichSelectionCliff(
		srcErr,
		[]parser.RequestItem{mainItem}, // fullMain
		[]parser.RequestItem{setupItem}, // fullSetup
		[]string{"Use"},
	)

	if !strings.Contains(enriched.Error(), `"Seed"`) {
		t.Errorf("expected error to name setup producer %q, got: %s", "Seed", enriched.Error())
	}
	if !strings.Contains(enriched.Error(), "pruned by --only") {
		t.Errorf("expected error to mention pruning, got: %s", enriched.Error())
	}
	if !strings.Contains(enriched.Error(), "likely an curlew bug") {
		t.Errorf("expected error to flag as a likely bug, got: %s", enriched.Error())
	}
	// Must still wrap the original error for errors.Is.
	if !errors.Is(enriched, variable.ErrUndefinedVariable) {
		t.Error("enriched error must still satisfy errors.Is(ErrUndefinedVariable)")
	}
}
```

#### Impact on Existing Tests
- `TestRun_OnlyVariableCliff` — the collection has no setup items, so `fullSetupForCliff` is empty, and the new setup-scan branch is skipped. Semantics unchanged. Call sites (three in runner.go) pass `vars.fullSetupForCliff` which is nil when not set — safe.
- `TestRunner_UndefinedVarMessageFormatStable` — untouched; it validates the message format produced by internal/variable, not by enrichSelectionCliff.

---

### Step 5: Wire stderr through cmd/curlew
**Rationale:** Smallest change in the cmd layer. Connect existing `stderr io.Writer` to `VarSources.Diagnostics` so the fallback diagnostic reaches the user.

#### Files to Modify

| File                           | Action | Description                                                                                |
|--------------------------------|--------|--------------------------------------------------------------------------------------------|
| `cmd/curlew/main.go`          | modify | Add `Diagnostics: stderr,` to the three VarSources constructions on lines 611, 1001, 1291. |
| `cmd/curlew/main_test.go`     | modify | Add `TestRun_OnlyAnalyzerFallback_StderrDiagnostic` (integration test).                     |

#### Current Code (main.go:1291 example)
```go
Selection:           flags.onlyNames,
```

#### New Code
```go
Diagnostics:         stderr,
Selection:           flags.onlyNames,
```

#### Tests to Write FIRST
Integration test at the cmd layer asserting the diagnostic appears on stderr:
```go
// TestRun_OnlyAnalyzerFallback_StderrDiagnostic verifies that when the
// minimal-setup analyser rejects the combined DAG (e.g. duplicate setup
// producers), the CLI emits a one-line diagnostic on stderr and runs the
// full setup.
func TestRun_OnlyAnalyzerFallback_StderrDiagnostic(t *testing.T) {
	// ... collection YAML with two setup items extracting the same var ...
	_, stderr, code := captureRunCmd(t, col, "--only", "Main")
	if code != 0 {
		t.Errorf("exit = %d, want 0 (fallback runs full setup successfully)", code)
	}
	if !strings.Contains(stderr, "minimal-setup analysis failed") {
		t.Errorf("stderr should contain fallback diagnostic, got: %s", stderr)
	}
}
```

#### Impact on Existing Tests
- None. `Diagnostics` is additive and zero-valued in tests that don't set it.

---

### Step 6: Documentation + metadata updates
**Rationale:** Pure docs, no code. Land as the final step.

#### Files to Modify

| File                           | Action | Description                                                                 |
|--------------------------------|--------|-----------------------------------------------------------------------------|
| `CHANGELOG.md`                 | modify | Add a `Changed` bullet under `[Unreleased]` for --only minimal-setup pruning. |
| `docs/SPECIFICATION.md`        | modify | Add a paragraph under `### Request Selection (--only)` for minimal-setup.   |
| `docs/MANUAL.md`               | modify | Add a short worked example in the `--only` section.                         |
| `IMPROVEMENT.md`               | modify | Update the W3 status block (line 179) and Q3 resolution (line 392-394) to mark M8-005 as shipped. |

#### Sample CHANGELOG entry
```markdown
### Changed
- CLI: `--only "<name>"` now prunes the setup phase to the transitive closure of `{{variable}}` references starting from the selected main requests, for a real inner-loop speedup. Setup items that have no `extract:` block (pure seeders) are always included. If the minimal-setup analyser rejects the combined DAG (cycles, duplicate producers, etc.), the runner falls back to running the full setup and emits a one-line diagnostic on stderr. Teardown is unaffected. (M8-005)
```

#### Sample docs/MANUAL.md paragraph
Insert after the existing "In watch mode" paragraph:

```markdown
**Minimal setup:** By default `--only` runs only the setup items whose extracted
variables are transitively referenced by the selected request. Setup items with
no `extract:` block are treated as pure seeders and always run. This speeds up
the inner loop when your setup phase is large but the request under test only
needs a subset. If curlew cannot analyse the dependency graph (e.g. duplicate
producers), it falls back to running the full setup and prints a one-line
warning on stderr.
```

#### IMPROVEMENT.md — W3 status block update
Replace the "The §8.3 V2 minimal-setup follow-up ... is still pending as `M8-004-followup`." clause with an explicit M8-005 shipped marker, mirroring the M8-001..M8-004 pattern.

#### Tests to Write FIRST
None (docs-only). Verification is via smoke test and manual read.

---

## Test Impact Summary

| Test File                              | Test Function                            | Impact      | Action Required                                   |
|----------------------------------------|------------------------------------------|-------------|---------------------------------------------------|
| `internal/parallel/waves_test.go`      | `TestParallel_AncestorClosure`           | new         | Add; RED→GREEN in Step 1.                         |
| `internal/runner/runner_test.go`       | `TestRunner_BuildPreExecVarSet`          | new         | Add; RED→GREEN in Step 2.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyPrunesSetup`             | new         | Add; RED→GREEN in Step 3.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyPrunesSetup_ChainedProducers` | new    | Add; RED→GREEN in Step 3.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyPrunesSetup_NoExtractSeeder`  | new    | Add; RED→GREEN in Step 3.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyPrunesSetup_AnalyzerFallback` | new    | Add; RED→GREEN in Step 3.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly` | new | Add regression; Step 3.                           |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyCliff_SetupProducer`     | new         | Add; RED→GREEN in Step 4.                         |
| `internal/runner/runner_test.go`       | `TestRunner_OnlyFilter`                  | unchanged   | Must still pass (regression).                     |
| `internal/runner/runner_test.go`       | `TestRun_OnlyVariableCliff`              | signature   | `enrichSelectionCliff` gains a param — not a public concern, but check no test dependency on old signature. |
| `internal/runner/runner_test.go`       | `TestFilterMainItems`                    | unchanged   | No change.                                        |
| `cmd/curlew/main_test.go`             | `TestRun_OnlyAnalyzerFallback_StderrDiagnostic` | new  | Add; RED→GREEN in Step 5.                         |
| `cmd/curlew/main_test.go`             | `TestParseRunArgs_Only` and friends      | unchanged   | No wiring change for arg parsing.                  |

---

## Risks and Edge Cases

- **Risk: `parallel.Analyze` performance on large collections.** → **Mitigation:** Analyze is already used by `executeParallelMain` on the main slice; linear in nodes+edges. Combined slice is `|setup| + |filtered| ≤ |setup| + |main|`, which is the same upper bound.
- **Risk: Stale setup items missed by reference scanner.** → **Mitigation:** The Step 4 cliff diagnostic names the pruned setup producer explicitly as a likely bug, so the user can file a precise reproducer. In practice this should never fire.
- **Edge case: Main item uses a dynamic function or pre-exec-only variables.** → **Handling:** Its `ReferencedVars` from `ScanRequestFields` is empty. `AncestorClosure` starts from the item index (inclusive of self). Setup items with `extract:` blocks get pruned; no-extract seeders always run. Correct.
- **Edge case: Data-driven main item in the selection.** → **Handling:** The parent `RequestItem` carries the `{{variable}}` references on its request fields; iterations reuse the same interpolation targets. Its index in the combined graph captures the full closure. Per-iteration refinement is deferred (already out of scope per task YAML).
- **Edge case: `setup:` has a request whose `extract:` block names a variable that main consumes via a header injected by an auth profile.** → **Handling:** Auth profiles run before setup, and `buildPreExecVarSet` is seeded *after* auth. So the injected variable appears in `preExecVars` and never forms an edge — neither setup item will claim to produce it. No false positives on pruning. Verified by reading buildScope and the auth-profile runner path.
- **Edge case: `col.Setup.Items` is empty.** → **Handling:** The new branch's `if len(col.Setup.Items) > 0` guard skips analysis entirely; `prunedSetup` stays as the empty slice. No overhead.
- **Edge case: `filtered` is identical to `col.Requests.Items` (--only lists every main item).** → **Handling:** The closure still runs but the shape reduces to "keep every setup item referenced plus no-extract" — identical to the default case when only one or two setup items lack extracts. No bug; just a tautological reduction. Behaviour: matches user expectation.
- **Risk: Shallow-copy of `col.Setup` drops a shared `Retry` pointer accidentally.** → **Mitigation:** Mirror the existing `parser.Section{Retry: col.Requests.Retry, Items: filtered}` line exactly; `Retry` is a pointer and is re-threaded explicitly (not dereferenced). Verified by reading parser.Section struct at collection.go:125-128.
- **Risk: Watch mode subtly regresses because --only is re-parsed on every file event.** → **Mitigation:** The task YAML explicitly notes `curlew watch` just re-invokes run with the same --only args — propagation is transparent. The cmd layer doesn't change its watch wiring. Verified by TestWatch_OnlyPropagates at main_test.go:8482.

---

## Go Signature Summary

New / changed exported symbols:

```go
// internal/parallel/waves.go
func AncestorClosure(graph *DependencyGraph, starts []int) map[int]bool
```

New / changed unexported symbols:

```go
// internal/runner/runner.go
func buildPreExecVarSet(scope *variable.Scope) map[string]bool

// Changed:
func enrichSelectionCliff(
    err error,
    fullMainItems, fullSetupItems []parser.RequestItem,
    selection []string,
) error
```

New `VarSources` fields:
```go
// internal/runner/runner.go (public-ish)
Diagnostics       io.Writer         // optional stderr-like writer for one-line warnings
fullSetupForCliff []parser.RequestItem // runner-internal, callers leave nil
```

---

## Verification

Build / test / lint / smoke:
```bash
go build ./cmd/curlew
go test ./...
go test -run 'TestParallel_AncestorClosure|TestRunner_OnlyPrunesSetup|TestRunner_OnlyPrunesSetup_ChainedProducers|TestRunner_OnlyPrunesSetup_NoExtractSeeder|TestRunner_OnlyPrunesSetup_AnalyzerFallback|TestRunner_OnlyCliff_SetupProducer' ./...
go test -cover ./internal/parallel/... ./internal/runner/...
~/go/bin/golangci-lint run
./smoke/run.sh
./scripts/ci-local.sh
```

Observable (case 1 — closure skips Seed users / Seed posts):
```bash
./curlew run collections/multi.yaml --only "Get user" --events run.ndjson
jq -c 'select(.kind=="request.start") | .name' run.ndjson
# Expected: "Login", "Warm cache", "Get user"
```

Observable (case 2 — chained producers traverse setup→setup):
```bash
./curlew run collections/multi.yaml --only "List posts" --events run.ndjson
jq -c 'select(.kind=="request.start") | .name' run.ndjson
# Expected: "Seed users", "Seed posts", "Warm cache", "List posts"
```

Observable (case 3 — analyser-invalid fallback):
```bash
./curlew run collections/ambiguous.yaml --only "Get user" 2>stderr.log
grep 'minimal-setup analysis failed' stderr.log
# Expected: diagnostic line present; all setup items execute.
```

Observable (case 4 — no --only, no pruning):
```bash
./curlew run collections/multi.yaml
# Expected: all 4 setup items run, both main items run.
```

---

## Open Decisions (resolved in-plan under Auto mode)

1. **Location of `AncestorClosure`.** Decided: `internal/parallel/waves.go` alongside `hasTransitiveDep`. Rationale: same-family graph traversal primitive; no new file justifies being split out for one function.
2. **Where the diagnostic writes to.** Decided: new `Diagnostics io.Writer` field on `VarSources`. Rationale: runner has no prior coupling to `os.Stderr` and no verbosity knob; adding a writer is both cleaner and test-friendly. cmd layer decides policy by choosing stderr vs `io.Discard`.
3. **Verbosity gating.** Decided: always emit the one-line diagnostic when the analyser falls back, regardless of verbosity. Rationale: the task YAML says "verbosity: normal and above" but the runner layer has no verbosity knowledge; pushing this to the cmd layer means a quiet mode can still suppress by passing `io.Discard`. Keeping it simple for the first slice; can add a gate later if it proves noisy.
4. **Signature change to `enrichSelectionCliff`.** Decided: add `fullSetupItems` as a third parameter (positional), not a struct. Rationale: only three callers, all internal to the runner package; a positional parameter is a one-line sweep whereas a new struct adds noise.
5. **"Setup pruned names" on `run.start`.** Deferred per the task YAML — not in scope.

---
