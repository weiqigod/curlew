package runner

import (
	"errors"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/retry"
)

// A retention policy throws away response detail. It must not throw away
// anything else.
//
// filterDataDrivenResults rebuilt RequestResult field by field, as a whitelist,
// so every field added to the struct after the filter was written was absent
// from a summary or failed_only result unless someone remembered to add a line.
// AssertionResults was one of them, and losing it reported a failing run as
// passing. RequestID and RequestSlug were two more: the markdown report's
// correlation sentinel came out as `id=-iter-0`, so the file an events line
// names could no longer be found from the events line.
//
// This test is the guard that makes the whitelist unnecessary. A field added to
// RequestResult tomorrow is preserved by default, and dropping one deliberately
// means naming it here.

// clearedByRetention are the fields a policy may zero: the response itself and
// the per-attempt record of getting it.
var clearedByRetention = map[string]bool{
	"Result":         true,
	"RequestHeaders": true,
	"RequestBody":    true,
	"AttemptDetails": true,
}

// reducedByRetention are fields kept in a smaller form rather than whole.
// AssertionResults keeps its verdict and loses its items, which is what
// "aggregate counts only" means: the run's outcome survives, its evidence
// does not.
var reducedByRetention = map[string]bool{
	"AssertionResults": true,
}

// fullResult is a RequestResult with every field set to something non-zero, so
// that "was it preserved?" is answerable for all of them.
func fullResult() RequestResult {
	return RequestResult{
		Name:             "Each [1/2]",
		Phase:            PhaseMain,
		Method:           "POST",
		URL:              "http://example.test/r/1",
		RequestHeaders:   map[string]string{"X-A": "1"},
		RequestBody:      "body",
		Result:           &httpexec.Result{StatusCode: 200, Headers: http.Header{"X-B": {"2"}}},
		Err:              errors.New("boom"),
		Skipped:          true,
		SkipReason:       "because",
		AssertionResults: &assertion.Results{Passed: false, Items: []assertion.Result{{Passed: false}}},
		RetryCount:       2,
		RetryWarnings:    []string{"POST is not idempotent"},
		AttemptDetails:   []retry.AttemptDetail{{Number: 1}},
		WaveIndex:        3,
		Warnings:         []string{"graphql partial success"},
		IsDataDriven:     true,
		DataDrivenName:   "Each",
		IterationIndex:   0,
		IterationTotal:   2,
		IterationData:    map[string]string{"n": "1"},
		SourceFile:       "c.yaml",
		SourceLine:       4,
		RequestID:        "req-1-iter-0",
		RequestSlug:      "each-1-2",
	}
}

// zeroFields returns the names of fields left at their zero value, which is how
// a fully-populated fixture reports the ones a filter dropped.
func zeroFields(r RequestResult) map[string]bool {
	out := map[string]bool{}
	v := reflect.ValueOf(r)
	for i := 0; i < v.NumField(); i++ {
		if v.Field(i).IsZero() {
			out[v.Type().Field(i).Name] = true
		}
	}
	return out
}

func TestFilterDataDrivenResults_keepsEverythingButResponseDetail(t *testing.T) {
	// IterationIndex is deliberately 0 in the fixture — it is the first
	// iteration — so it would read as "dropped" whatever the filter did.
	unprovable := map[string]bool{"IterationIndex": true}

	// failed_only strips only the iterations that passed, so its fixture has to
	// be one: a failing iteration is kept whole and would prove nothing here.
	for _, policy := range []string{"summary", "failed_only"} {
		t.Run(policy, func(t *testing.T) {
			in := fullResult()
			if policy == "failed_only" {
				in.Err = nil
				in.AssertionResults = &assertion.Results{Passed: true, Items: []assertion.Result{{Passed: true}}}
			}
			out := filterDataDrivenResults([]RequestResult{in}, policy)
			if len(out) != 1 {
				t.Fatalf("filter returned %d results, want 1", len(out))
			}

			var lost []string
			for name := range zeroFields(out[0]) {
				switch {
				case clearedByRetention[name], reducedByRetention[name], unprovable[name]:
					continue
				}
				if zeroFields(in)[name] {
					continue // never set; nothing to lose
				}
				lost = append(lost, name)
			}
			sort.Strings(lost)
			if len(lost) > 0 {
				t.Errorf("store_results: %s dropped %s. A retention policy decides what response "+
					"detail is kept, not which of a result's own fields survive — add the field to "+
					"clearedByRetention only if losing it is deliberate.", policy, strings.Join(lost, ", "))
			}

			// And what it does clear, it must actually clear.
			for name := range clearedByRetention {
				if !zeroFields(out[0])[name] {
					t.Errorf("store_results: %s kept %s, which is response detail", policy, name)
				}
			}

			// The verdict survives without its evidence.
			ar := out[0].AssertionResults
			if ar == nil {
				t.Errorf("store_results: %s dropped the assertion verdict; a failing iteration would "+
					"be counted as passing", policy)
			} else if ar.Passed != in.AssertionResults.Passed || len(ar.Items) != 0 {
				t.Errorf("store_results: %s kept assertions as %+v; want the verdict alone", policy, ar)
			}
		})
	}
}

// A failing iteration under failed_only keeps everything, which is the row's
// whole point.
func TestFilterDataDrivenResults_failedOnlyKeepsFailingIterationsWhole(t *testing.T) {
	in := fullResult() // Err is set, so this iteration failed
	out := filterDataDrivenResults([]RequestResult{in}, "failed_only")
	if len(out[0].AssertionResults.Items) == 0 {
		t.Error("failed_only stripped a failing iteration's assertion detail")
	}
	if out[0].Result == nil {
		t.Error("failed_only stripped a failing iteration's response")
	}
	if out[0].RequestID != in.RequestID {
		t.Errorf("failed_only lost a failing iteration's request id: %q", out[0].RequestID)
	}
}

// "all" is the identity, and nothing above should have made it otherwise.
func TestFilterDataDrivenResults_allIsUnchanged(t *testing.T) {
	in := fullResult()
	out := filterDataDrivenResults([]RequestResult{in}, "all")
	if !reflect.DeepEqual(out[0], in) {
		t.Error("store_results: all is no longer the identity")
	}
}
