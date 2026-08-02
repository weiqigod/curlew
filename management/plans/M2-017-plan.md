# Implementation Plan: M2-017

## Overview

Extend the parallel execution engine with comprehensive edge case handling: detailed skip messages naming specific variables, auth profile variable exclusion from dependency creation, external file extraction support, impact analysis output, default value interaction with failed producers, and nested variable resolution depth warnings.

## Task Details
- **ID:** M2-017
- **Title:** Parallel execution edge cases (failed dependencies, skipped requests)
- **Phase:** M2: Parallel Execution
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-016 | Parallel request execution with wave-based scheduling | done |

## Implementation Steps

### Step 1: Enhance `checkDependencyFailure` with Variable-Specific Skip Messages

**Rationale:** This is the smallest, most self-contained change. The function already exists and has a clear contract. Enhancing the message format is a pure refinement with no downstream signature changes needed (the function already returns a string).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor_test.go` | modify | Add tests for variable-specific skip messages |
| `internal/parallel/executor.go` | modify | Enhance `checkDependencyFailure` to include variable names in skip reason |

#### Current Code
```go
func checkDependencyFailure(graph *DependencyGraph, idx int, failedIndices map[int]bool) (bool, string) {
	if idx >= len(graph.Nodes) {
		return false, ""
	}
	node := graph.Nodes[idx]
	for dep := range node.Dependencies {
		if failedIndices[dep] {
			return true, fmt.Sprintf("dependency %q failed", graph.Nodes[dep].Name)
		}
	}
	return false, ""
}
```

#### New Code
```go
func checkDependencyFailure(graph *DependencyGraph, idx int, failedIndices map[int]bool) (bool, string) {
	if idx >= len(graph.Nodes) {
		return false, ""
	}
	node := graph.Nodes[idx]
	for dep := range node.Dependencies {
		if failedIndices[dep] {
			// Find which variables this dependency provides
			vars := findEdgeVariables(graph, dep, idx)
			if len(vars) > 0 {
				return true, fmt.Sprintf("depends on %s from %q, which failed",
					formatVarList(vars), graph.Nodes[dep].Name)
			}
			return true, fmt.Sprintf("dependency %q failed", graph.Nodes[dep].Name)
		}
	}
	return false, ""
}

// findEdgeVariables returns the variable names on the edge from producer to consumer.
func findEdgeVariables(graph *DependencyGraph, from, to int) []string {
	for _, edge := range graph.Edges {
		if edge.From == from && edge.To == to {
			return edge.Variables
		}
	}
	return nil
}

// formatVarList formats a list of variable names for human-readable output.
func formatVarList(vars []string) string {
	if len(vars) == 1 {
		return fmt.Sprintf("'%s'", vars[0])
	}
	quoted := make([]string, len(vars))
	for i, v := range vars {
		quoted[i] = fmt.Sprintf("'%s'", v)
	}
	return strings.Join(quoted, ", ")
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckDependencyFailure_VariableSpecificMessage(t *testing.T) {
	tests := []struct {
		name       string
		graph      *DependencyGraph
		idx        int
		failed     map[int]bool
		wantSkip   bool
		wantReason string
	}{
		{
			"single variable dependency failed - includes var name",
			func() *DependencyGraph {
				g := noEdgeGraph(2)
				g.Nodes[1].Dependencies[0] = true
				g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"user_id"}}}
				return g
			}(), 1, map[int]bool{0: true},
			true, "depends on 'user_id' from \"A\", which failed",
		},
		{
			"multiple variable dependency failed - lists all vars",
			func() *DependencyGraph {
				g := noEdgeGraph(2)
				g.Nodes[1].Dependencies[0] = true
				g.Edges = []Edge{{From: 0, To: 1, Variables: []string{"token", "user_id"}}}
				return g
			}(), 1, map[int]bool{0: true},
			true, "depends on 'token', 'user_id' from \"A\", which failed",
		},
		{
			"no failed dependencies",
			noEdgeGraph(2), 1, map[int]bool{},
			false, "",
		},
		{
			"out of range index",
			noEdgeGraph(1), 5, map[int]bool{0: true},
			false, "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			skip, reason := checkDependencyFailure(tt.graph, tt.idx, tt.failed)
			if skip != tt.wantSkip {
				t.Errorf("skip = %v, want %v", skip, tt.wantSkip)
			}
			if reason != tt.wantReason {
				t.Errorf("reason = %q, want %q", reason, tt.wantReason)
			}
		})
	}
}
```

#### Impact on Existing Tests
- `TestExecuteWaves_FailedRequest_SkipsDependents` — skip reason string will change from `dependency "A" failed` to `depends on 'token' from "A", which failed`. Update assertion to check for the new format (use `strings.Contains` for robustness).

---

### Step 2: Add `SkipReason` to Runner's `RequestResult` and Propagate from Parallel Outcomes

**Rationale:** The runner must propagate skip reasons so output formatters (terminal, JSON) can display them. This is a data plumbing step that enables Step 3 (output display).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `SkipReason` field to `RequestResult`; propagate from parallel `RequestOutcome` |
| `internal/runner/runner_test.go` | modify | Add test verifying SkipReason propagation in parallel mode |

#### Current Code (RequestResult)
```go
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	RequestHeaders   map[string]string
	RequestBody      any
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	AssertionResults *assertion.Results
	RetryCount       int
}
```

#### New Code (RequestResult)
```go
type RequestResult struct {
	Name             string
	Phase            Phase
	Method           string
	URL              string
	RequestHeaders   map[string]string
	RequestBody      any
	Result           *httpexec.Result
	Err              error
	Skipped          bool
	SkipReason       string // human-readable reason when Skipped is true
	AssertionResults *assertion.Results
	RetryCount       int
}
```

#### Current Code (executeParallelMain conversion)
```go
rr := RequestResult{
	Name:             outcome.Name,
	Phase:            PhaseMain,
	Method:           outcome.Method,
	URL:              outcome.URL,
	RequestHeaders:   outcome.RequestHeaders,
	RequestBody:      outcome.RequestBody,
	Result:           outcome.Result,
	Err:              outcome.Err,
	Skipped:          outcome.Skipped,
	AssertionResults: outcome.AssertionResults,
	RetryCount:       outcome.RetryCount,
}
```

#### New Code (executeParallelMain conversion)
```go
rr := RequestResult{
	Name:             outcome.Name,
	Phase:            PhaseMain,
	Method:           outcome.Method,
	URL:              outcome.URL,
	RequestHeaders:   outcome.RequestHeaders,
	RequestBody:      outcome.RequestBody,
	Result:           outcome.Result,
	Err:              outcome.Err,
	Skipped:          outcome.Skipped,
	SkipReason:       outcome.SkipReason,
	AssertionResults: outcome.AssertionResults,
	RetryCount:       outcome.RetryCount,
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_ParallelMode_SkipReasonPropagated(t *testing.T) {
	// A extracts user_id, B uses {{user_id}}. A fails => B skipped with reason.
	// Verify RequestResult.SkipReason is populated.
	tests := []struct {
		name           string
		wantSkipReason string
	}{
		{"failed dependency propagates skip reason", "depends on"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// ... setup collection with A->B dependency where A fails
			// ... verify results[1].SkipReason contains tt.wantSkipReason
		})
	}
}
```

#### Impact on Existing Tests
- No existing tests check `SkipReason` since it's a new field — no breakage.

---

### Step 3: Update Terminal and JSON Output to Show Skip Reasons

**Rationale:** With SkipReason now available in `RequestResult`, the output layer can display it. Terminal and JSON are the two primary output formats.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `SkippedWithReason` method to `Printer` |
| `internal/output/terminal_test.go` | modify | Add test for `SkippedWithReason` output |
| `internal/output/json.go` | modify | Add `SkipReason` field to `JSONRequest` |
| `cmd/apitest/main.go` | modify | Use `SkippedWithReason` when reason is available; set `skip_reason` in JSON |

#### New Code (terminal.go)
```go
// SkippedWithReason writes a skipped request line with a reason.
func (p *Printer) SkippedWithReason(name, reason string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  %s  SKIPPED\n", colorize(name, ansiGray, p.color))
	if reason != "" {
		_, _ = fmt.Fprintf(p.w, "    %s\n", colorize("Reason: "+reason, ansiGray, p.color))
	}
}
```

#### New Code (json.go addition to JSONRequest)
```go
type JSONRequest struct {
	// ... existing fields ...
	SkipReason string `json:"skip_reason,omitempty"`
}
```

#### New Code (main.go changes)
```go
case r.Skipped:
	if r.SkipReason != "" {
		out.SkippedWithReason(r.Name, r.SkipReason)
	} else {
		out.Skipped(r.Name)
	}
```

And in JSON output:
```go
case r.Skipped:
	jr.Status = "skipped"
	jr.SkipReason = r.SkipReason
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_SkippedWithReason(t *testing.T) {
	tests := []struct {
		name      string
		reqName   string
		reason    string
		wantParts []string
	}{
		{"with reason", "Request B", "depends on 'user_id' from \"A\", which failed",
			[]string{"Request B", "SKIPPED", "Reason:", "user_id"}},
		{"empty reason falls back", "Request B", "",
			[]string{"Request B", "SKIPPED"}},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests check skip output format in this level of detail — no breakage.

---

### Step 4: Add Impact Analysis to ExecutionResult

**Rationale:** Per the spec, output should show "N requests skipped due to dependency on 'Request A'". This requires tracking skip causes aggregated across waves.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor.go` | modify | Add `ImpactSummary` computation after all waves complete |
| `internal/parallel/executor_test.go` | modify | Add tests for impact summary |
| `internal/parallel/graph.go` | modify | Add `ImpactEntry` type |

#### New Code (graph.go)
```go
// ImpactEntry describes how many requests were skipped due to a specific failed request.
type ImpactEntry struct {
	FailedIndex int
	FailedName  string
	SkippedCount int
}
```

#### New Code (executor.go)
```go
type ExecutionResult struct {
	Waves    []WaveResult
	Duration time.Duration
	Impact   []ImpactEntry // aggregated skip impact per failed request
}
```

After all waves, compute impact:
```go
// computeImpact builds impact entries from failed indices and skipped outcomes.
func computeImpact(graph *DependencyGraph, failedIndices map[int]bool, waves []WaveResult) []ImpactEntry {
	// Count skips caused by each failed request
	causeCount := make(map[int]int)
	for _, wave := range waves {
		for _, o := range wave.Outcomes {
			if !o.Skipped {
				continue
			}
			// Find which failed dependency caused this skip
			node := graph.Nodes[o.Index]
			for dep := range node.Dependencies {
				if failedIndices[dep] {
					causeCount[dep]++
					break // count each skip once for primary cause
				}
			}
		}
	}
	var entries []ImpactEntry
	for idx, count := range causeCount {
		entries = append(entries, ImpactEntry{
			FailedIndex:  idx,
			FailedName:   graph.Nodes[idx].Name,
			SkippedCount: count,
		})
	}
	sort.Slice(entries, func(i, j int) bool {
		return entries[i].SkippedCount > entries[j].SkippedCount
	})
	return entries
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestComputeImpact(t *testing.T) {
	tests := []struct {
		name          string
		setupFunc     func() (*DependencyGraph, map[int]bool, []WaveResult)
		wantEntries   int
		wantTopCount  int
		wantTopName   string
	}{
		{
			"single failure cascading to two dependents",
			// A fails, B and C depend on A => impact: A caused 2 skips
			func() (*DependencyGraph, map[int]bool, []WaveResult) { ... },
			1, 2, "A",
		},
		{
			"no failures - empty impact",
			func() (*DependencyGraph, map[int]bool, []WaveResult) { ... },
			0, 0, "",
		},
		{
			"diamond - A fails, B and C skip, D skips transitively",
			func() (*DependencyGraph, map[int]bool, []WaveResult) { ... },
			1, 3, "A",
		},
	}
	// ...
}
```

#### Impact on Existing Tests
- `ExecutionResult` gains a new field `Impact` — existing tests that check `result.Waves` and `result.Duration` remain unaffected since `Impact` is an additive field.

---

### Step 5: Auth Profile Variables as Pre-Execution Scope (Tests Only)

**Rationale:** Auth profile variables are already correctly added to the pre-execution scope in `executeParallelMain` (via `scope.Resolved()` → `preExecVars`). Auth profiles execute before main requests and their variables are in scope. This step only adds explicit tests confirming the behavior described in the task's behaviors list.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/analyze_test.go` | modify | Add test: auth profile vars in preExecVars don't create dependencies |

#### Tests to Write FIRST (RED phase)

```go
func TestAnalyze_AuthProfileVarsNoDepCreated(t *testing.T) {
	// Multiple requests use {{admin_token}} which is in preExecVars (from auth profile).
	// No dependency should be created between them.
	items := []parser.RequestItem{
		{Name: "Create User", Request: parser.Request{Method: "POST", URL: "http://example.com/users",
			Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"}}},
		{Name: "Get Roles", Request: parser.Request{Method: "GET", URL: "http://example.com/roles",
			Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"}}},
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
```

#### Impact on Existing Tests
- No existing tests affected. This is a pure addition.

---

### Step 6: External Request File Extractions in Dependency Analysis (Tests Only)

**Rationale:** External request files are already resolved by `parser.ParseFile` before the parallel analyzer sees them. The items passed to `Analyze()` are fully inlined. This step adds explicit tests confirming the behavior works end-to-end with external files that have extract blocks.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/analyze_test.go` | modify | Add test: external files with extractions analyzed correctly |

#### Tests to Write FIRST (RED phase)

```go
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
```

#### Impact on Existing Tests
- No existing tests affected. This is a pure addition.

---

### Step 7: Default Value Interaction with Failed Producers

**Rationale:** Per spec behavior: "Given a variable with a default value (`{{var|default:value}}`), when the producing request fails, then the dependent request still skips (producer failed, regardless of default)." The current `checkDependencyFailure` already implements this behavior since it checks whether the producing request (by index) is in `failedIndices`, not whether the variable has a default. This step adds an explicit test confirming this behavior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/executor_test.go` | modify | Add test: default value does not prevent skip when producer fails |

#### Tests to Write FIRST (RED phase)

```go
func TestExecuteWaves_DefaultValueDoesNotPreventSkipOnFailedProducer(t *testing.T) {
	// Request A extracts user_id. Request B uses {{user_id|default:fallback}}.
	// A fails => B is still skipped (producer failed, regardless of default).
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
			Extract: map[string]string{"user_id": "$.id"},
		},
		{
			Name:    "B",
			Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{user_id|default:fallback}}"},
		},
	}
	preExecVars := make(map[string]bool)
	graph := Analyze(items, preExecVars)
	if !graph.IsValid {
		t.Fatalf("graph invalid: %v", graph.Errors)
	}

	scope := makeScope(t, nil)
	result, err := ExecuteWaves(context.Background(), Config{
		Graph:       graph,
		Items:       items,
		Scope:       scope,
		ExecFunc:    failExecFunc("server error"),
		MaxRequests: 1000,
	})
	if err != nil {
		t.Fatalf("ExecuteWaves error: %v", err)
	}

	// B should be skipped despite having a default value
	if len(result.Waves) < 2 {
		t.Fatalf("expected at least 2 waves, got %d", len(result.Waves))
	}
	wave1 := result.Waves[1]
	if len(wave1.Outcomes) != 1 {
		t.Fatalf("wave 1 outcomes = %d, want 1", len(wave1.Outcomes))
	}
	if !wave1.Outcomes[0].Skipped {
		t.Error("expected B to be skipped when producer A failed, even with default")
	}
	if wave1.Outcomes[0].SkipReason == "" {
		t.Error("expected skip reason to be set")
	}
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

### Step 8: Nested Variable Resolution Depth Warning

**Rationale:** Per spec: "Given nested variable resolution depth > 10, when analyzed, then a warning is produced." This requires adding depth-limited resolution tracking in the scanner. The `ScanVariables` function currently does single-pass scanning. For the dependency analyzer, nested variables in the *pre-execution scope* could create deep chains. The warning needs to be surfaced in the `DependencyGraph.Warnings` field.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/analyze.go` | modify | Add nested variable resolution with depth limit and warning |
| `internal/parallel/analyze_test.go` | modify | Add tests for nested variable depth warning |

#### New Code (analyze.go)
```go
// resolveNestedRefs expands variable references through the pre-execution scope
// up to maxDepth levels. Returns the fully resolved set of referenced variables
// (those not found in pre-execution scope) and any warnings.
func resolveNestedRefs(refs map[string]bool, preExecVals map[string]string, maxDepth int) (map[string]bool, []string) {
	var warnings []string
	resolved := make(map[string]bool)
	for k := range refs {
		resolved[k] = true
	}

	for depth := 0; depth < maxDepth; depth++ {
		changed := false
		next := make(map[string]bool)
		for varName := range resolved {
			val, ok := preExecVals[varName]
			if !ok {
				next[varName] = true
				continue
			}
			// This var is pre-exec; scan its value for nested refs
			nested := ScanVariables(val, nil)
			for n := range nested {
				if !resolved[n] {
					next[n] = true
					changed = true
				}
			}
		}
		resolved = next
		if !changed {
			return resolved, warnings
		}
	}

	warnings = append(warnings, "deep variable nesting (10+ levels) detected; consider flattening variable definitions")
	return resolved, warnings
}
```

Note: This function is called during analysis only when pre-execution variable *values* are available. Currently `Analyze` only receives `preExecVars map[string]bool` (names only, not values). To support nested resolution, we need to change the signature or add an optional parameter. Given that the current behavior already works for all existing tests, the cleanest approach is to add a new `AnalyzeWithValues` variant or an `AnalyzeOptions` struct.

**Decision:** Add an `AnalyzeOptions` struct to keep the API extensible:

```go
// AnalyzeOptions provides optional configuration for dependency analysis.
type AnalyzeOptions struct {
	// PreExecValues maps pre-execution variable names to their values.
	// Used for nested variable resolution depth checking.
	// If nil, nested resolution is not performed.
	PreExecValues map[string]string
}

// Analyze performs the full 6-phase dependency analysis.
// preExecVars contains variable names available before execution.
func Analyze(items []parser.RequestItem, preExecVars map[string]bool, opts ...AnalyzeOptions) *DependencyGraph {
	// ... existing logic ...
	// After building nodes but before edge construction:
	if len(opts) > 0 && opts[0].PreExecValues != nil {
		// Resolve nested refs for each node
		for i := range graph.Nodes {
			expanded, warnings := resolveNestedRefs(graph.Nodes[i].ReferencedVars, opts[0].PreExecValues, 10)
			graph.Nodes[i].ReferencedVars = expanded
			graph.Warnings = append(graph.Warnings, warnings...)
		}
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
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
```

#### Impact on Existing Tests
- `Analyze` signature uses variadic `opts` parameter — all existing callers pass only `(items, preExecVars)` and continue to work without changes.
- No existing tests break.

---

### Step 9: Display Impact Summary in Terminal and JSON Output

**Rationale:** With `ImpactEntry` data available from Step 4, we can render the impact analysis in both terminal and JSON output. This is the final step tying together all the edge case handling.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/output/terminal.go` | modify | Add `ImpactSummary` method |
| `internal/output/terminal_test.go` | modify | Add test for impact summary rendering |
| `internal/output/json.go` | modify | Add `ImpactJSON` to `JSONOutput` |
| `internal/runner/runner.go` | modify | Add `Impact` field to `Summary`; populate from parallel execution |
| `cmd/apitest/main.go` | modify | Render impact summary when available |

#### New Code (terminal.go)
```go
// ImpactSummary writes the parallel execution impact analysis.
func (p *Printer) ImpactSummary(entries []ImpactLine) {
	if len(entries) == 0 || p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize("Impact Analysis:", ansiBold, p.color))
	for _, e := range entries {
		_, _ = fmt.Fprintf(p.w, "  %s\n",
			colorize(fmt.Sprintf("%d request(s) skipped due to dependency on '%s'",
				e.SkippedCount, e.FailedName), ansiYellow, p.color))
	}
}

// ImpactLine holds a single impact analysis entry for display.
type ImpactLine struct {
	FailedName   string
	SkippedCount int
}
```

#### New Code (json.go)
```go
type JSONImpact struct {
	FailedRequest string `json:"failed_request"`
	SkippedCount  int    `json:"skipped_count"`
}
```

And in `JSONOutput`:
```go
type JSONOutput struct {
	// ... existing fields ...
	Impact []JSONImpact `json:"impact,omitempty"`
}
```

#### New Code (runner.go Summary)
```go
type Summary struct {
	// ... existing fields ...
	Impact []parallel.ImpactEntry // parallel execution impact analysis
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestPrinter_ImpactSummary(t *testing.T) {
	tests := []struct {
		name      string
		entries   []ImpactLine
		wantParts []string
	}{
		{"single failure with 2 skips",
			[]ImpactLine{{FailedName: "Request A", SkippedCount: 2}},
			[]string{"Impact Analysis:", "2 request(s) skipped", "Request A"}},
		{"empty entries - no output",
			nil, nil},
	}
	// ...
}
```

#### Impact on Existing Tests
- `Summary` gains a new field `Impact` — existing tests don't check this field, so no breakage.

---

### Step 10: Integration Test - End-to-End Parallel Edge Cases

**Rationale:** A comprehensive integration test that exercises the full path from collection → runner → parallel execution → output, verifying all edge cases work together.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner_test.go` | modify | Add parallel edge case integration tests |

#### Tests to Write FIRST (RED phase)

```go
func TestRun_ParallelEdgeCases(t *testing.T) {
	tests := []struct {
		name           string
		collection     *parser.Collection
		exec           ExecuteFunc
		wantPassed     int
		wantFailed     int
		wantSkipped    int
		wantImpact     int
		checkSkipReason bool
	}{
		{
			"failed request skips dependents with variable-specific reason",
			// A extracts user_id, B uses {{user_id}}, A fails => B skipped
			// ...
		},
		{
			"auth profile vars don't create dependencies",
			// Multiple requests use {{admin_token}} from auth profile
			// ...
		},
		{
			"default value does not prevent skip on failed producer",
			// A fails, B has {{var|default:x}} => B still skipped
			// ...
		},
	}
	// ...
}
```

#### Impact on Existing Tests
- No existing tests affected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parallel/executor_test.go` | `TestExecuteWaves_FailedRequest_SkipsDependents` | skip reason format changes | update assertion to match new format |
| `internal/parallel/executor_test.go` | all other tests | no impact | none |
| `internal/parallel/analyze_test.go` | all tests | no impact | Analyze signature uses variadic opts |
| `internal/runner/runner_test.go` | all tests | no impact | new field is additive |
| `internal/output/terminal_test.go` | all tests | no impact | new methods are additive |
| `internal/output/json_test.go` | all tests | no impact | new field is additive |
| `cmd/apitest/main.go` | all tests | no impact | output changes are additive |

## Risks and Edge Cases
- **Risk:** Changing `checkDependencyFailure` message format could break any code parsing the skip reason string. **Mitigation:** Only `executor_test.go` checks the exact string — update the one test.
- **Risk:** `Analyze` variadic opts parameter might be confusing. **Mitigation:** The common case remains `Analyze(items, preExecVars)` without opts. Document the new parameter.
- **Edge case:** A request with multiple failed dependencies (e.g., depends on both A and B, both fail). **Handling:** Current code returns the first found failure. The message will reference whichever dependency is iterated first. The impact analysis will attribute the skip to the primary cause.
- **Edge case:** Cascading failures (A fails → B skipped → C skipped because B is "failed"). **Handling:** `failedIndices` already tracks skipped-due-to-dependency requests, so C is correctly skipped. The skip message for C will reference B (its direct dependency), not A.
- **Risk:** Nested variable resolution could expand the set of referenced vars, potentially creating unexpected dependencies. **Mitigation:** Only applies when `AnalyzeOptions.PreExecValues` is provided. Existing callers are unaffected.
- **Edge case:** Thread safety of `Impact` field population. **Handling:** Impact is computed after all waves complete (sequential), so no race condition.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Run parallel tests
go test ./internal/parallel/... -v -run "TestCheckDependencyFailure_VariableSpecificMessage|TestExecuteWaves_DefaultValue|TestAnalyze_AuthProfileVars|TestAnalyze_ExternalFile|TestAnalyze_NestedVariable|TestComputeImpact"

# Run runner integration tests
go test ./internal/runner/... -v -run "TestRun_ParallelEdgeCases"

# Run output tests
go test ./internal/output/... -v -run "TestPrinter_SkippedWithReason|TestPrinter_ImpactSummary"
```
