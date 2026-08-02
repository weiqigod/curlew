package parallel

import (
	"fmt"
	"io"
	"strings"
)

// WriteDOT writes the dependency graph in Graphviz DOT format to w.
func WriteDOT(w io.Writer, graph *DependencyGraph) error {
	if _, err := fmt.Fprintln(w, "digraph dependencies {"); err != nil {
		return fmt.Errorf("writing DOT header: %w", err)
	}

	if _, err := fmt.Fprintln(w, "  rankdir=LR;"); err != nil {
		return fmt.Errorf("writing DOT rankdir: %w", err)
	}

	// Write nodes
	for _, node := range graph.Nodes {
		if _, err := fmt.Fprintf(w, "  %q;\n", node.Name); err != nil {
			return fmt.Errorf("writing DOT node: %w", err)
		}
	}

	// Write edges
	for _, edge := range graph.Edges {
		label := strings.Join(edge.Variables, ", ")
		if label == "" && edge.Explicit {
			label = "depends_on"
		}
		if _, err := fmt.Fprintf(w, "  %q -> %q [label=%q];\n",
			graph.Nodes[edge.From].Name,
			graph.Nodes[edge.To].Name,
			label); err != nil {
			return fmt.Errorf("writing DOT edge: %w", err)
		}
	}

	if _, err := fmt.Fprintln(w, "}"); err != nil {
		return fmt.Errorf("writing DOT footer: %w", err)
	}

	return nil
}

// FormatWaves returns a human-readable description of execution waves.
// Includes max parallelism and expected speedup for multi-wave graphs.
func FormatWaves(graph *DependencyGraph) string {
	if len(graph.Waves) == 0 {
		return "Total waves: 0\nNo requests to execute.\n"
	}

	var sb strings.Builder
	maxPar := 0
	totalRequests := 0
	for i, wave := range graph.Waves {
		names := make([]string, len(wave))
		for j, idx := range wave {
			names[j] = graph.Nodes[idx].Name
		}
		concurrency := "concurrent"
		if len(wave) == 1 {
			concurrency = "sequential"
		}
		fmt.Fprintf(&sb, "  Wave %d: [%s] → %d %s\n", i+1, strings.Join(names, ", "), len(wave), concurrency)
		if len(wave) > maxPar {
			maxPar = len(wave)
		}
		totalRequests += len(wave)
	}
	fmt.Fprintf(&sb, "\nTotal waves: %d\n", len(graph.Waves))
	fmt.Fprintf(&sb, "Maximum parallelism: %d requests per wave\n", maxPar)
	if len(graph.Waves) > 1 && maxPar > 0 {
		speedup := float64(totalRequests) / float64(len(graph.Waves))
		fmt.Fprintf(&sb, "Expected speedup: %.1fx\n", speedup)
	}
	return sb.String()
}
