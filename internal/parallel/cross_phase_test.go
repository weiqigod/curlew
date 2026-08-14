package parallel

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/parser"
)

// A depends_on naming an item in another phase is rejected, not dropped.
//
// The wave planner cannot honour the edge — it only ever sees one phase's
// items — and it used to skip the name silently on the reasoning that phases
// are ordered anyway. But skip propagation is same-phase (§3.3), so the line a
// user wrote as a safety link did nothing at all: the item ran even when the
// setup item it named had failed. §11.4 lists exactly this construction as
// rejected with exit 3, and nothing checked.
//
// A name that resolves nowhere in the collection is still skipped: that is
// --only having removed the item, which is not the user writing a dead link.
func TestAnalyze_dependsOnAnotherPhaseIsRejected(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "One", DependsOn: []string{"Prepare"}},
	}

	graph := Analyze(items, map[string]bool{}, AnalyzeOptions{
		OtherPhaseNames: map[string]bool{"Prepare": true},
	})
	if graph.IsValid {
		t.Fatal("a depends_on naming a setup item was accepted; §11.4 lists it as rejected")
	}
	joined := strings.Join(graph.Errors, "; ")
	for _, want := range []string{"One", "Prepare", "phase"} {
		if !strings.Contains(joined, want) {
			t.Errorf("the rejection does not mention %q: %q", want, joined)
		}
	}
}

func TestAnalyze_dependsOnRemovedByOnlyIsStillSkipped(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "One", DependsOn: []string{"FilteredAway"}},
	}

	// No OtherPhaseNames entry: the name belongs to this phase and is simply
	// not in the slice, which is what --only leaves behind.
	graph := Analyze(items, map[string]bool{}, AnalyzeOptions{
		OtherPhaseNames: map[string]bool{},
	})
	if !graph.IsValid {
		t.Errorf("an item removed by --only made the graph invalid: %v", graph.Errors)
	}
}

// Callers that pass no options keep the old permissive behaviour, which is what
// the sequential path and the UI orchestrator rely on.
func TestAnalyze_withoutOptionsCrossPhaseIsStillTolerated(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "One", DependsOn: []string{"Prepare"}},
	}
	if graph := Analyze(items, map[string]bool{}); !graph.IsValid {
		t.Errorf("analysis without options rejected a cross-phase depends_on: %v", graph.Errors)
	}
}
