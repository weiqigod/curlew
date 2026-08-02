# Implementation Plan: M2-015

## Overview
Create the `internal/parallel/` package implementing the 6-phase dependency analysis algorithm from the specification. This includes variable analysis, dependency graph building, cycle detection (Kahn's algorithm), variable collision detection, wave computation, DOT graph export, and CLI integration via `--show-dependencies` and `--dry-run` flags on the `run` command. Parallel execution is gated as a Professional-tier feature.

## Task Details
- **ID:** M2-015
- **Title:** Dependency analysis algorithm (variable analysis and graph building)
- **Phase:** M2: Parallel Execution
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-009 | Collection variables | done |
| M1-010 | Variable extraction | done |
| M1-015 | External request files | done |
| M1-028 | Feature gate framework | done |

## Design Decisions

1. **Regex pattern**: The spec defines `\{\{([a-zA-Z_][a-zA-Z0-9_\.]*(?:\|[^}]+)?)\}\}` for dependency scanning, which includes dots for nested access and pipe for defaults. The existing `variable.varPattern` uses `[a-zA-Z0-9_]*` (no dots, no defaults). The parallel package will define its own regex for scanning since it needs the broader pattern. This avoids modifying the existing variable package.

2. **Pre-execution variables**: The algorithm needs to know which variables are "pre-execution" (collection vars, env vars, CLI args, from_command, vault) so they don't create inter-request dependencies. The `Analyze()` function will accept a set of pre-execution variable names.

3. **Setup/teardown handling**: For this task, dependency analysis only applies to main requests. Setup and teardown remain sequential per spec. Synthetic barrier nodes are mentioned in the spec but actual parallel execution is M2-016's concern. This task computes the graph and waves for main requests only.

4. **Feature gate**: The `--show-dependencies` flag itself does NOT require a paid tier (it's a diagnostic tool). The actual parallel execution (M2-016) will check the gate. However, the gate registration happens in this task to prepare for M2-016.

5. **DOT export**: `--show-dependencies` prints the DOT graph to stdout and exits (no HTTP requests). `--show-dependencies --dry-run` additionally shows execution waves. Both exit with code 0.

## Implementation Steps

### Step 1: Core Data Types (`internal/parallel/graph.go`)
**Rationale:** Define the foundational types before any logic. All subsequent steps depend on these types.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/graph.go` | create | Core data structures: RequestNode, Edge, DependencyGraph |

#### New Code
```go
package parallel

// RequestNode represents a request in the dependency graph.
type RequestNode struct {
    Index          int
    Name           string
    ProducedVars   map[string]bool // variables extracted by this request
    ReferencedVars map[string]bool // variables referenced by this request (excluding pre-exec and dynamic)
    Dependencies   map[int]bool    // indices of requests this depends on
}

// Edge represents a dependency between two requests.
type Edge struct {
    From      int      // index of producer request
    To        int      // index of consumer request
    Variables []string // variable names creating this dependency
}

// DependencyGraph holds the full analysis result.
type DependencyGraph struct {
    Nodes    []RequestNode
    Edges    []Edge
    Waves    [][]int  // execution wave groupings
    IsValid  bool
    Errors   []string
    Warnings []string
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestRequestNode_ZeroValue(t *testing.T) {
    tests := []struct {
        name string
        node RequestNode
    }{
        {"zero value has empty maps", RequestNode{}},
    }
}

func TestDependencyGraph_ZeroValue(t *testing.T) {
    tests := []struct {
        name  string
        graph DependencyGraph
    }{
        {"zero value is not valid", DependencyGraph{}},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 2: Variable Scanning (`internal/parallel/scan.go`)
**Rationale:** Variable scanning is the foundation of Phase 2 (variable analysis). Must be implemented and tested before building the dependency graph.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/scan.go` | create | Variable scanning regex and extraction functions |
| `internal/parallel/scan_test.go` | create | Table-driven tests for variable scanning |

#### New Code
```go
package parallel

import "regexp"

// varScanPattern matches {{varName}} and {{varName|default}} patterns.
// Broader than variable.varPattern: includes dots for nested access and pipe for defaults.
var varScanPattern = regexp.MustCompile(`\{\{([a-zA-Z_][a-zA-Z0-9_.]*(?:\|[^}]+)?)\}\}`)

// ScanVariables extracts all variable references from a string.
// Returns variable names (before the pipe if default syntax is used).
// Dynamic functions (starting with $) are excluded.
// Pre-execution variables are excluded.
func ScanVariables(input string, preExecVars map[string]bool) map[string]bool

// ScanRequestFields scans all interpolatable fields of a request item
// and returns the set of referenced variable names.
func ScanRequestFields(item *parser.RequestItem, preExecVars map[string]bool) map[string]bool

// ExtractProducedVars returns the set of variable names from extract blocks.
// Returns an error string if any extract key contains {{.
func ExtractProducedVars(extract map[string]string) (map[string]bool, string)
```

#### Tests to Write FIRST (RED phase)

```go
func TestScanVariables(t *testing.T) {
    tests := []struct {
        name        string
        input       string
        preExecVars map[string]bool
        want        map[string]bool
    }{
        {"simple variable", "{{user_id}}", nil, set("user_id")},
        {"multiple variables", "{{a}} and {{b}}", nil, set("a", "b")},
        {"dynamic function excluded", "{{$timestamp}}", nil, set()},
        {"pre-exec var excluded", "{{base_url}}", set("base_url"), set()},
        {"default syntax strips pipe", "{{user_id|default:123}}", nil, set("user_id")},
        {"nested dot access", "{{user.id}}", nil, set("user.id")},
        {"mixed dynamic and regular", "{{$uuid}} {{token}}", nil, set("token")},
        {"no variables", "plain text", nil, set()},
        {"single braces ignored", "{not_a_var}", nil, set()},
        {"empty input", "", nil, set()},
    }
}

func TestScanRequestFields(t *testing.T) {
    tests := []struct {
        name        string
        item        *parser.RequestItem
        preExecVars map[string]bool
        want        map[string]bool
    }{
        {"url variable", urlItem("{{base_url}}/users/{{user_id}}"), set("base_url"), set("user_id")},
        {"header variables", headerItem(map[string]string{"Auth": "Bearer {{token}}"}), nil, set("token")},
        {"body string variable", bodyStringItem(`{"id": "{{id}}"}`), nil, set("id")},
        {"query param variable", queryItem(map[string]string{"page": "{{page}}"}), nil, set("page")},
        {"extract path variable", extractPathItem(map[string]string{"id": "$.users[{{idx}}].id"}), nil, set("idx")},
        {"assertion body path variable", assertBodyItem("$.users.{{field}}"), nil, set("field")},
        {"no variables anywhere", plainItem(), nil, set()},
    }
}

func TestExtractProducedVars(t *testing.T) {
    tests := []struct {
        name      string
        extract   map[string]string
        wantVars  map[string]bool
        wantError string
    }{
        {"single extraction", map[string]string{"user_id": "$.id"}, set("user_id"), ""},
        {"multiple extractions", map[string]string{"a": "$.a", "b": "$.b"}, set("a", "b"), ""},
        {"dynamic key rejected", map[string]string{"{{prefix}}_id": "$.id"}, nil, "Dynamic variable names"},
        {"nil extract", nil, set(), ""},
        {"empty extract", map[string]string{}, set(), ""},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 3: Dependency Graph Building (`internal/parallel/analyze.go`)
**Rationale:** Implements Phase 1 (node creation from parsed collection), Phase 2 (wiring variable analysis), and Phase 3 (edge construction). Builds on Step 2's scanning functions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/analyze.go` | create | Main Analyze function orchestrating all 6 phases |
| `internal/parallel/analyze_test.go` | create | Integration tests for full analysis pipeline |

#### New Code
```go
package parallel

import "github.com/weiqigod/curlew/internal/parser"

// Analyze performs the full 6-phase dependency analysis on a collection's main requests.
// preExecVars contains variable names available before execution (collection vars, env vars, CLI args).
// Returns a DependencyGraph with nodes, edges, waves, and validation status.
func Analyze(items []parser.RequestItem, preExecVars map[string]bool) *DependencyGraph
```

#### Tests to Write FIRST (RED phase)

```go
func TestAnalyze(t *testing.T) {
    tests := []struct {
        name        string
        items       []parser.RequestItem
        preExecVars map[string]bool
        wantWaves   [][]int
        wantValid   bool
        wantErrors  int
    }{
        {"independent requests all in wave 0",
            threeIndependentRequests(), nil,
            [][]int{{0, 1, 2}}, true, 0},
        {"linear chain A->B->C",
            linearChain(), nil,
            [][]int{{0}, {1}, {2}}, true, 0},
        {"diamond dependency",
            diamondDependency(), nil,
            [][]int{{0}, {1, 2}, {3}}, true, 0},
        {"pre-exec vars do not create dependencies",
            requestsUsingPreExecVars(), set("base_url", "api_key"),
            [][]int{{0, 1}}, true, 0},
        {"dynamic functions do not create dependencies",
            requestsUsingDynamicFunctions(), nil,
            [][]int{{0, 1}}, true, 0},
        {"circular dependency detected",
            circularRequests(), nil,
            nil, false, 1},
        {"variable collision detected",
            collisionRequests(), nil,
            nil, false, 1},
        {"single request",
            singleRequest(), nil,
            [][]int{{0}}, true, 0},
        {"empty request list",
            nil, nil,
            [][]int{}, true, 0},
        {"dynamic variable name in extract rejected",
            dynamicExtractKeyRequests(), nil,
            nil, false, 1},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 4: Cycle Detection (`internal/parallel/cycle.go`)
**Rationale:** Implements Phase 4 (Kahn's algorithm for topological sort and cycle detection). Separated into its own file for clarity as it's a self-contained algorithm.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/cycle.go` | create | Kahn's algorithm and cycle path finder |
| `internal/parallel/cycle_test.go` | create | Tests for cycle detection edge cases |

#### New Code
```go
package parallel

// topologicalSort performs Kahn's algorithm on the graph.
// Returns the sorted order and whether the graph is acyclic.
// If a cycle exists, returns the nodes processed so far and false.
func topologicalSort(graph *DependencyGraph) ([]int, bool)

// findCyclePath uses DFS to find and describe a cycle starting from any
// of the given remaining nodes. Returns a human-readable cycle description.
func findCyclePath(graph *DependencyGraph, remaining []int) string
```

#### Tests to Write FIRST (RED phase)

```go
func TestTopologicalSort(t *testing.T) {
    tests := []struct {
        name      string
        graph     *DependencyGraph
        wantOrder []int
        wantValid bool
    }{
        {"no edges - all nodes in order", noEdgeGraph(3), []int{0, 1, 2}, true},
        {"linear chain", linearGraph(), []int{0, 1, 2}, true},
        {"diamond", diamondGraph(), nil, true}, // order varies but valid
        {"simple cycle A->B->A", simpleCycleGraph(), nil, false},
        {"self-cycle", selfCycleGraph(), nil, false},
        {"partial cycle with valid nodes", partialCycleGraph(), nil, false},
    }
}

func TestFindCyclePath(t *testing.T) {
    tests := []struct {
        name      string
        graph     *DependencyGraph
        remaining []int
        wantParts []string // substrings expected in cycle description
    }{
        {"simple two-node cycle", twoNodeCycle(), []int{0, 1}, []string{"Request A", "Request B"}},
        {"three-node cycle", threeNodeCycle(), []int{0, 1, 2}, []string{"Request A", "Request B", "Request C"}},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 5: Collision Detection and Wave Computation (`internal/parallel/waves.go`)
**Rationale:** Implements Phase 5 (collision detection) and Phase 6 (wave computation). These are the final analysis phases and depend on the validated graph from Step 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/waves.go` | create | Collision detection and wave grouping |
| `internal/parallel/waves_test.go` | create | Tests for collision and wave edge cases |

#### New Code
```go
package parallel

// detectCollisions checks for variable name collisions among parallel requests.
// Two requests that can run in parallel (no dependency between them) must not
// both produce the same variable. Appends errors to graph.Errors.
func detectCollisions(graph *DependencyGraph)

// computeWaves groups requests into execution waves based on dependencies.
// Requests in the same wave have no mutual dependencies and can run in parallel.
func computeWaves(graph *DependencyGraph)
```

#### Tests to Write FIRST (RED phase)

```go
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
        {"collision with three producers", threeProducerCollisionGraph(), 1},
    }
}

func TestComputeWaves(t *testing.T) {
    tests := []struct {
        name      string
        graph     *DependencyGraph
        wantWaves [][]int
    }{
        {"all independent - single wave", allIndependentGraph(3), [][]int{{0, 1, 2}}},
        {"linear chain - N waves", linearChainGraph(3), [][]int{{0}, {1}, {2}}},
        {"diamond pattern", diamondPatternGraph(), [][]int{{0}, {1, 2}, {3}}},
        {"complex DAG", complexDAGGraph(), [][]int{{0, 1}, {2}, {3, 4}}},
        {"single node", singleNodeGraph(), [][]int{{0}}},
        {"empty graph", emptyGraph(), [][]int{}},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 6: DOT Graph Export (`internal/parallel/dot.go`)
**Rationale:** Output formatting is isolated from analysis logic. The DOT format is simple and self-contained.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parallel/dot.go` | create | DOT format graph rendering |
| `internal/parallel/dot_test.go` | create | Tests for DOT output format |

#### New Code
```go
package parallel

import "io"

// WriteDOT writes the dependency graph in Graphviz DOT format to w.
func WriteDOT(w io.Writer, graph *DependencyGraph) error

// FormatWaves returns a human-readable description of execution waves.
func FormatWaves(graph *DependencyGraph) string
```

#### Tests to Write FIRST (RED phase)

```go
func TestWriteDOT(t *testing.T) {
    tests := []struct {
        name     string
        graph    *DependencyGraph
        wantParts []string // substrings expected in DOT output
    }{
        {"empty graph", emptyValidGraph(), []string{"digraph", "}"}},
        {"single node no edges", singleNodeDOTGraph(), []string{"digraph", `"Get Token"`}},
        {"edge with label", edgeDOTGraph(), []string{"->", `label="token"`}},
        {"multiple edges", multiEdgeDOTGraph(), []string{"->", "->"}},
    }
}

func TestFormatWaves(t *testing.T) {
    tests := []struct {
        name string
        graph *DependencyGraph
        wantParts []string
    }{
        {"single wave", singleWaveGraph(), []string{"Wave 1", "concurrent"}},
        {"multiple waves", multiWaveGraph(), []string{"Wave 1", "Wave 2", "Wave 3"}},
        {"empty waves", emptyWaveGraph(), []string{"Total waves: 0"}},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected (new package)

### Step 7: Feature Gate Registration
**Rationale:** Register "parallel_execution" in the default registry as Professional tier. Small change, no blast radius.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add parallel_execution feature definition |
| `internal/auth/registry_test.go` | modify | Verify new feature is registered |

#### Current Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	r.Register(FeatureDefinition{
		Name:         "retry",
		RequiredTier: TierSolo,
		Description:  "Retry logic requires Solo tier ($9/month)",
		Workaround:   "Run requests manually or use shell scripting for retry logic",
	})
	return r
}
```

#### New Code
```go
func DefaultRegistry() *Registry {
	r := NewRegistry()
	// ... existing registrations ...
	r.Register(FeatureDefinition{
		Name:         "retry",
		RequiredTier: TierSolo,
		Description:  "Retry logic requires Solo tier ($9/month)",
		Workaround:   "Run requests manually or use shell scripting for retry logic",
	})
	r.Register(FeatureDefinition{
		Name:         "parallel_execution",
		RequiredTier: TierProfessional,
		Description:  "Parallel execution requires Professional tier ($19/month)",
		Workaround:   "Requests execute sequentially in Free and Solo tiers",
	})
	return r
}
```

#### Tests to Write FIRST (RED phase)

```go
// In registry_test.go, verify the new feature is registered:
func TestDefaultRegistry_ParallelExecution(t *testing.T) {
    r := DefaultRegistry()
    def, ok := r.Lookup("parallel_execution")
    if !ok {
        t.Fatal("parallel_execution not registered")
    }
    if def.RequiredTier != TierProfessional {
        t.Errorf("want Professional tier, got %s", def.RequiredTier)
    }
}
```

#### Impact on Existing Tests
- `TestDefaultRegistry` in `registry_test.go` may need updating if it asserts on the total count of registered features

### Step 8: CLI Integration (`--show-dependencies` and `--dry-run` flags)
**Rationale:** This is the user-facing integration step. It connects the analysis engine to the CLI. Done last because it depends on all internal pieces being ready.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Add --show-dependencies flag to parseRunArgs and runCmdInner |

#### Current Code (parseRunArgs)
```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive bool, err error) {
```

#### New Code (parseRunArgs)
```go
func parseRunArgs(args []string) (file, envName, format string, vars, envVarVars map[string]string, seed *int64, noColor bool, verbosity output.Verbosity, allowSensitive, showDeps, dryRun bool, err error) {
```

The function will parse `--show-dependencies` and `--dry-run` flags. When `--show-dependencies` is set:
1. Parse the collection file
2. Build the pre-execution variable set from all variable sources
3. Call `parallel.Analyze()` on the main requests
4. If analysis has errors, print them and exit with code 3
5. If `--dry-run` is also set, print wave execution plan via `parallel.FormatWaves()`
6. Otherwise, print DOT graph via `parallel.WriteDOT()`
7. Exit with code 0

The `--dry-run` flag on the run command is introduced here (it already exists on exec). When used alone (without `--show-dependencies`), it will be a no-op for now (future M2-016 will use it for parallel dry-run). When used with `--show-dependencies`, it shows wave info instead of DOT.

#### Tests to Write FIRST (RED phase)

```go
// In main_test.go:
func TestParseRunArgs_ShowDependencies(t *testing.T) {
    tests := []struct {
        name     string
        args     []string
        wantShow bool
        wantDry  bool
        wantErr  bool
    }{
        {"show-dependencies flag", []string{"--show-dependencies", "test.yaml"}, true, false, false},
        {"show-dependencies with dry-run", []string{"--show-dependencies", "--dry-run", "test.yaml"}, true, true, false},
        {"dry-run alone", []string{"--dry-run", "test.yaml"}, false, true, false},
        {"no flags", []string{"test.yaml"}, false, false, false},
    }
}

func TestRun_ShowDependencies_DOTOutput(t *testing.T) {
    // Integration test: create a collection YAML, run with --show-dependencies,
    // verify DOT output on stdout
}

func TestRun_ShowDependencies_DryRunWaves(t *testing.T) {
    // Integration test: create a collection YAML, run with --show-dependencies --dry-run,
    // verify wave output on stdout
}

func TestRun_ShowDependencies_CircularError(t *testing.T) {
    // Integration test: collection with circular dependency,
    // verify error message and exit code 3
}
```

#### Help text update
Add to the "Run Options" section:
```
  --show-dependencies   Show dependency graph (DOT format) without executing
  --dry-run             With --show-dependencies: show execution waves
```

#### Impact on Existing Tests
- All callers of `parseRunArgs` in `main_test.go` will need to handle the two new return values (`showDeps`, `dryRun`)
- The `runCmdInner` callers (watch mode) pass through correctly since they use `runCmdInner(args)` with the raw args

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/curlew/main_test.go` | `TestParseRunArgs*` | breaks | update to handle new return values (showDeps, dryRun) |
| `cmd/curlew/main_test.go` | `TestRun*` | none | no change |
| `internal/auth/registry_test.go` | `TestDefaultRegistry` | may break | update if it asserts feature count |
| `internal/parallel/*_test.go` | all | new | all new tests |

## Risks and Edge Cases
- **Risk:** Modifying `parseRunArgs` signature breaks all callers. **Mitigation:** Update all call sites systematically. Consider using an options struct in the future, but for now match existing pattern with positional returns.
- **Risk:** Regex discrepancy between `variable.varPattern` (no dots) and `parallel.varScanPattern` (with dots). **Mitigation:** The parallel scanner is intentionally broader to match the spec. Document the difference clearly.
- **Edge case:** Empty collection (no requests). **Handling:** Return valid graph with no nodes, no edges, empty waves.
- **Edge case:** Request with extract key containing `{{`. **Handling:** Add error to graph and mark invalid per spec.
- **Edge case:** Very large collection (>1000 requests). **Handling:** O(N^2) is acceptable per spec targets. No special handling needed.
- **Edge case:** Variables referenced only in assertions or extract paths. **Handling:** ScanRequestFields scans ALL fields including assertion expressions and extract JSONPath values.
- **Edge case:** Body is a map (not string). **Handling:** Recursively scan all string values in nested maps/slices.
- **Edge case:** Default values `{{var|default:x}}` should extract `var` not `var|default:x`. **Handling:** Strip everything after the pipe in variable name extraction.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# DOT graph output
curlew run --show-dependencies tests.yaml

# Execution wave display
curlew run --show-dependencies --dry-run tests.yaml

# Unit tests
go test ./internal/parallel/...
```
