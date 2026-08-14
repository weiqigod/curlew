package parallel

import (
	"fmt"
	"sort"

	"github.com/weiqigod/curlew/internal/parser"
)

// AnalyzeOptions provides optional configuration for dependency analysis.
type AnalyzeOptions struct {
	// PreExecValues maps pre-execution variable names to their values.
	// Used for nested variable resolution depth checking.
	// If nil, nested resolution is not performed.
	PreExecValues map[string]string

	// OtherPhaseNames holds the request names defined in the collection's
	// other phases. A depends_on resolving to one of them is rejected (§11.4):
	// the wave planner only ever sees one phase, so it cannot honour the edge,
	// and skip propagation is same-phase — the line does nothing at all. When
	// nil, such names are tolerated and dropped, which is what the sequential
	// path and the dependency viewer want.
	OtherPhaseNames map[string]bool
}

// Analyze performs the full 6-phase dependency analysis on a collection's main requests.
// preExecVars contains variable names available before execution (collection vars, env vars, CLI args).
// An optional AnalyzeOptions parameter enables advanced analysis features like nested variable depth checking.
// Returns a DependencyGraph with nodes, edges, waves, and validation status.
func Analyze(items []parser.RequestItem, preExecVars map[string]bool, opts ...AnalyzeOptions) *DependencyGraph {
	graph := &DependencyGraph{
		IsValid: true,
	}

	if len(items) == 0 {
		graph.Waves = [][]int{}
		return graph
	}

	// Phase 1: Create nodes from request items
	graph.Nodes = make([]RequestNode, len(items))
	for i := range items {
		produced, errMsg := ExtractProducedVars(items[i].Extract)
		if errMsg != "" {
			graph.IsValid = false
			graph.Errors = append(graph.Errors, fmt.Sprintf("request %q: %s", items[i].Name, errMsg))
			return graph
		}

		referenced := ScanRequestFields(&items[i], preExecVars)

		graph.Nodes[i] = RequestNode{
			Index:          i,
			Name:           items[i].Name,
			ProducedVars:   produced,
			ReferencedVars: referenced,
			Dependencies:   make(map[int]bool),
		}
	}

	// Optional nested variable resolution depth check
	if len(opts) > 0 && opts[0].PreExecValues != nil {
		warnings := checkNestingDepth(opts[0].PreExecValues, maxNestedDepth)
		graph.Warnings = append(graph.Warnings, warnings...)
	}

	// Phase 2 & 3: Build dependency edges
	// For each consumer request, find producers of its referenced variables.
	for i := range graph.Nodes {
		consumer := &graph.Nodes[i]
		for varName := range consumer.ReferencedVars {
			// Find producer(s) of this variable
			for j := range graph.Nodes {
				if j == i {
					continue
				}
				if graph.Nodes[j].ProducedVars[varName] {
					consumer.Dependencies[j] = true
					// Add edge (check for duplicate)
					addEdge(graph, j, i, varName)
				}
			}
		}
	}

	// Explicit depends_on edges. Parse-time validation (validateDependsOn)
	// has already rejected names unknown to the collection, so a name that does
	// not resolve here belongs either to another phase or to an item --only
	// removed. The two are not the same: a name in another phase is a link the
	// planner cannot honour and skip propagation will not honour either, so it
	// is rejected when the caller says which names those are (§11.4). A name
	// left behind by --only is skipped, as before. Self-references are skipped
	// to match the sequential path, where they are harmless. A pair already
	// ordered by a variable edge keeps that edge unchanged.
	var otherPhase map[string]bool
	if len(opts) > 0 {
		otherPhase = opts[0].OtherPhaseNames
	}
	nameToIndex := make(map[string]int, len(items))
	for i := range items {
		if _, ok := nameToIndex[items[i].Name]; !ok {
			nameToIndex[items[i].Name] = i
		}
	}
	for i := range items {
		for _, dep := range items[i].DependsOn {
			j, ok := nameToIndex[dep]
			if !ok {
				if otherPhase[dep] {
					graph.IsValid = false
					graph.Errors = append(graph.Errors, fmt.Sprintf(
						"request %q: depends_on %q names an item in another phase, which cannot be "+
							"ordered against this one", items[i].Name, dep))
				}
				continue
			}
			if j == i || graph.Nodes[i].Dependencies[j] {
				continue
			}
			graph.Nodes[i].Dependencies[j] = true
			graph.Edges = append(graph.Edges, Edge{From: j, To: i, Explicit: true})
		}
	}
	if !graph.IsValid {
		return graph
	}

	// Phase 4: Cycle detection using Kahn's algorithm
	_, acyclic := topologicalSort(graph)
	if !acyclic {
		graph.IsValid = false
		// Find remaining nodes (those not in topological order)
		remaining := findRemainingNodes(graph)
		cyclePath := findCyclePath(graph, remaining)
		graph.Errors = append(graph.Errors, cyclePath)
		return graph
	}

	// Phase 5: Collision detection
	detectCollisions(graph)
	if len(graph.Errors) > 0 {
		graph.IsValid = false
		return graph
	}

	// Phase 6: Wave computation
	computeWaves(graph)

	return graph
}

// addEdge adds a variable to an existing edge or creates a new one.
func addEdge(graph *DependencyGraph, from, to int, varName string) {
	for i := range graph.Edges {
		if graph.Edges[i].From == from && graph.Edges[i].To == to {
			graph.Edges[i].Variables = append(graph.Edges[i].Variables, varName)
			return
		}
	}
	graph.Edges = append(graph.Edges, Edge{
		From:      from,
		To:        to,
		Variables: []string{varName},
	})
}

// maxNestedDepth is the maximum allowed depth for nested variable resolution.
// A chain deeper than this produces a warning.
const maxNestedDepth = 10

// checkNestingDepth traces variable reference chains through pre-execution values
// and returns a warning if any chain exceeds maxDepth levels.
func checkNestingDepth(preExecVals map[string]string, maxDepth int) []string {
	for name := range preExecVals {
		depth := traceDepth(name, preExecVals, maxDepth, make(map[string]bool))
		if depth > maxDepth {
			return []string{"deep variable nesting (10+ levels) detected; consider flattening variable definitions"}
		}
	}
	return nil
}

// traceDepth follows a variable reference chain and returns the depth.
// visited prevents infinite loops from circular references.
func traceDepth(name string, vals map[string]string, maxDepth int, visited map[string]bool) int {
	if visited[name] {
		return 0 // circular, don't count further
	}
	val, ok := vals[name]
	if !ok {
		return 0
	}
	visited[name] = true
	refs := ScanVariables(val, nil)
	maxChild := 0
	for ref := range refs {
		d := traceDepth(ref, vals, maxDepth, visited)
		if d > maxChild {
			maxChild = d
		}
		if maxChild+1 > maxDepth {
			break // early exit
		}
	}
	return maxChild + 1
}

// findRemainingNodes returns the indices of nodes that were not processed
// by the topological sort (i.e., nodes involved in cycles).
func findRemainingNodes(graph *DependencyGraph) []int {
	order, _ := topologicalSort(graph)
	processed := make(map[int]bool, len(order))
	for _, idx := range order {
		processed[idx] = true
	}
	var remaining []int
	for i := range graph.Nodes {
		if !processed[i] {
			remaining = append(remaining, i)
		}
	}
	sort.Ints(remaining)
	return remaining
}
