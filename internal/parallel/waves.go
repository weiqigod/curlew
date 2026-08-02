package parallel

import (
	"fmt"
	"sort"
	"strings"
)

// detectCollisions checks for variable name collisions among parallel requests.
// Two requests that can run in parallel (no dependency between them) must not
// both produce the same variable. Appends errors to graph.Errors.
func detectCollisions(graph *DependencyGraph) {
	// Build a map: varName -> list of node indices that produce it
	producers := make(map[string][]int)
	for i := range graph.Nodes {
		for v := range graph.Nodes[i].ProducedVars {
			producers[v] = append(producers[v], i)
		}
	}

	// For each variable with multiple producers, check if any pair is parallel
	// (i.e., neither depends on the other directly or transitively).
	seen := make(map[string]bool) // track reported variable collisions
	for varName, indices := range producers {
		if len(indices) < 2 {
			continue
		}
		// Check all pairs
		hasCollision := false
		for i := 0; i < len(indices) && !hasCollision; i++ {
			for j := i + 1; j < len(indices) && !hasCollision; j++ {
				a, b := indices[i], indices[j]
				if !hasTransitiveDep(graph, a, b) && !hasTransitiveDep(graph, b, a) {
					hasCollision = true
				}
			}
		}
		if hasCollision && !seen[varName] {
			seen[varName] = true
			names := make([]string, len(indices))
			for i, idx := range indices {
				names[i] = fmt.Sprintf("%q", graph.Nodes[idx].Name)
			}
			graph.Errors = append(graph.Errors,
				fmt.Sprintf("variable collision: %q is produced by parallel requests %s",
					varName, strings.Join(names, ", ")))
		}
	}
}

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

// AncestorClosure returns the set of node indices reachable by following
// the Dependencies edges (consumer -> producer) starting from the given
// start indices. The returned set is inclusive of the starts. Out-of-range
// start indices are ignored. Uses iterative BFS so it is safe on deep
// graphs without blowing the stack.
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

// computeWaves groups requests into execution waves based on dependencies.
// Requests in the same wave have no mutual dependencies and can run in parallel.
func computeWaves(graph *DependencyGraph) {
	n := len(graph.Nodes)
	if n == 0 {
		graph.Waves = [][]int{}
		return
	}

	// Compute wave number for each node: wave[i] = max(wave[dep] for dep in deps) + 1
	// Nodes with no dependencies are wave 0.
	waveNum := make([]int, n)
	computed := make([]bool, n)

	var computeNode func(idx int) int
	computeNode = func(idx int) int {
		if computed[idx] {
			return waveNum[idx]
		}
		computed[idx] = true
		maxDep := -1
		for dep := range graph.Nodes[idx].Dependencies {
			w := computeNode(dep)
			if w > maxDep {
				maxDep = w
			}
		}
		waveNum[idx] = maxDep + 1
		return waveNum[idx]
	}

	maxWave := 0
	for i := 0; i < n; i++ {
		w := computeNode(i)
		if w > maxWave {
			maxWave = w
		}
	}

	// Group nodes by wave number
	waves := make([][]int, maxWave+1)
	for i := 0; i < n; i++ {
		waves[waveNum[i]] = append(waves[waveNum[i]], i)
	}

	// Sort indices within each wave for deterministic output
	for i := range waves {
		sort.Ints(waves[i])
	}

	graph.Waves = waves
}
