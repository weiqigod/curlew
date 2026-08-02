package parallel

import (
	"fmt"
	"strings"
)

// topologicalSort performs Kahn's algorithm on the graph.
// Returns the sorted order and whether the graph is acyclic.
// If a cycle exists, returns the nodes processed so far and false.
func topologicalSort(graph *DependencyGraph) ([]int, bool) {
	n := len(graph.Nodes)
	if n == 0 {
		return nil, true
	}

	// Compute in-degree for each node
	inDegree := make([]int, n)
	for i := range graph.Nodes {
		inDegree[i] = len(graph.Nodes[i].Dependencies)
	}

	// Seed queue with nodes having zero in-degree
	queue := make([]int, 0, n)
	for i, deg := range inDegree {
		if deg == 0 {
			queue = append(queue, i)
		}
	}

	// Build reverse adjacency: for each node, which nodes depend on it
	dependents := make([][]int, n)
	for i := range graph.Nodes {
		for dep := range graph.Nodes[i].Dependencies {
			dependents[dep] = append(dependents[dep], i)
		}
	}

	var order []int
	for len(queue) > 0 {
		// Pop from front
		node := queue[0]
		queue = queue[1:]
		order = append(order, node)

		// For each dependent of this node, decrement in-degree
		for _, dep := range dependents[node] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}

	if len(order) != n {
		return order, false // cycle detected
	}
	return order, true
}

// findCyclePath uses DFS to find and describe a cycle starting from any
// of the given remaining nodes. Returns a human-readable cycle description.
func findCyclePath(graph *DependencyGraph, remaining []int) string {
	if len(remaining) == 0 {
		return "unknown cycle"
	}

	remainSet := make(map[int]bool, len(remaining))
	for _, idx := range remaining {
		remainSet[idx] = true
	}

	// DFS from each remaining node to find a cycle
	const (
		white = 0 // unvisited
		gray  = 1 // in current path
		black = 2 // fully processed
	)

	color := make(map[int]int, len(remaining))
	parent := make(map[int]int, len(remaining))

	var cyclePath []int
	var cycleFound bool

	var dfs func(node int)
	dfs = func(node int) {
		if cycleFound {
			return
		}
		color[node] = gray
		for dep := range graph.Nodes[node].Dependencies {
			if !remainSet[dep] {
				continue
			}
			if cycleFound {
				return
			}
			if color[dep] == gray {
				// Found cycle: trace back from node to dep
				cyclePath = []int{dep, node}
				cur := node
				for cur != dep {
					cur = parent[cur]
					if cur == dep {
						break
					}
					cyclePath = append(cyclePath, cur)
				}
				cycleFound = true
				return
			}
			if color[dep] == white {
				parent[dep] = node
				dfs(dep)
			}
		}
		color[node] = black
	}

	for _, idx := range remaining {
		if color[idx] == white {
			dfs(idx)
			if cycleFound {
				break
			}
		}
	}

	if !cycleFound || len(cyclePath) == 0 {
		// Fallback: just list remaining nodes
		names := make([]string, len(remaining))
		for i, idx := range remaining {
			names[i] = fmt.Sprintf("%q", graph.Nodes[idx].Name)
		}
		return fmt.Sprintf("circular dependency among: %s", strings.Join(names, ", "))
	}

	// Build human-readable cycle description
	// Reverse the path since we traced backwards
	for i, j := 0, len(cyclePath)-1; i < j; i, j = i+1, j-1 {
		cyclePath[i], cyclePath[j] = cyclePath[j], cyclePath[i]
	}

	parts := make([]string, len(cyclePath))
	for i, idx := range cyclePath {
		parts[i] = fmt.Sprintf("%q", graph.Nodes[idx].Name)
	}
	return fmt.Sprintf("circular dependency: %s -> %s",
		strings.Join(parts, " -> "), parts[0])
}
