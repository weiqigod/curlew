package main

import (
	"sort"
	"testing"

	"github.com/weiqigod/curlew/internal/exitcodes"
)

// TestExitCodes_no_unreachable_mapping asserts every key in exitCodeSeverity
// is a code cmd/curlew can actually return.
//
// Containment, not equality, and the asymmetry is deliberate. 130 is
// reachable (a SIGINT during `curlew perf`, perf.go) but has no severity
// rank, because it is raised inside a single perf run and never takes part
// in the worst-wins fold across discovered collections. Requiring equality
// would force a rank for it, which is inventing an exit code's meaning. The
// direction that goes wrong is a code the map ranks but the binary cannot
// return, and that is the one asserted.
func TestExitCodes_no_unreachable_mapping(t *testing.T) {
	if len(exitCodeSeverity) == 0 {
		t.Fatal("exitCodeSeverity is empty — the map moved or was renamed; " +
			"this test is asserting nothing")
	}

	reachable := reachableExitCodes(t) // guarded: root, size, depth
	set := map[int]bool{}
	for _, v := range exitcodes.Set(reachable) {
		set[v] = true
	}

	for _, code := range sortedSeverityKeys() {
		if !set[code] {
			t.Errorf("exitCodeSeverity ranks exit %d, which cmd/curlew cannot return "+
				"(reachable: %v) — a severity rank for an unreachable code is not "+
				"documentation, it is a claim the feature might come back",
				code, exitcodes.Set(reachable))
		}
	}
}

// sortedSeverityKeys makes failure order deterministic; map iteration is not.
func sortedSeverityKeys() []int {
	out := make([]int, 0, len(exitCodeSeverity))
	for k := range exitCodeSeverity {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}
