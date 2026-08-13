package retry

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
)

// Three tables describe the backoff: the specification's formulae, its worked
// exponential sequence, and the manual's copy of that sequence. All three are
// arithmetic, which makes them the rarest kind of documentation — a claim that
// can simply be run.

// The worked sequences both use these inputs, stated in the prose above them:
// "Exponential from 500 ms capped at 30000 ms".
const (
	docInitialDelayMs = 500
	docMaxDelayMs     = 30000
)

// msValue reads "500 ms" or "30000 ms (capped)" as 500 / 30000.
var msValue = regexp.MustCompile(`^(\d+)\s*ms`)

// attemptValue reads "1 (first retry)" or "3" as 1 / 3.
var attemptValue = regexp.MustCompile(`^(\d+)`)

// The formula table states each strategy in terms of n. Rather than parse the
// algebra, each formula is checked against the strategy's own output over a
// range of n — which is what the formula is a claim about.
func TestDocTables_backoffFormulaeMatchTheImplementation(t *testing.T) {
	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Strategy", "Delay for retry *n* (0-based)")
	if err != nil {
		t.Fatalf("backoff formula table: %v", err)
	}
	stratCol := docs.Column(hdr, "Strategy")
	formulaCol := docs.Column(hdr, "Delay for retry *n* (0-based)")
	if stratCol < 0 || formulaCol < 0 {
		t.Fatalf("formula table lost a column: %v", hdr)
	}

	// What each documented formula computes, as a function of n. The mapping is
	// keyed by the formula the table prints, so rewording a formula without
	// changing the code fails here rather than passing silently.
	formulae := map[string]func(n int) int{
		"initial_delay_ms * 2^n":     func(n int) int { return docInitialDelayMs << n },
		"initial_delay_ms * (n + 1)": func(n int) int { return docInitialDelayMs * (n + 1) },
		"initial_delay_ms":           func(int) int { return docInitialDelayMs },
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= formulaCol {
			continue
		}
		strategy := docs.FirstName(row[stratCol])
		formula := strings.Trim(row[formulaCol], "`")

		compute, known := formulae[formula]
		if !known {
			t.Errorf("§9.3 states %q for `%s`, which this test cannot evaluate; "+
				"add it to formulae or correct the row", formula, strategy)
			continue
		}
		if !ValidStrategy(strategy) {
			t.Errorf("§9.3 documents strategy %q, which ValidStrategy rejects", strategy)
			continue
		}

		for n := 0; n < 8; n++ {
			want := compute(n)
			if want > docMaxDelayMs {
				want = docMaxDelayMs // every result is clamped, as the prose says
			}
			got := CalculateDelay(Strategy(strategy), n, docInitialDelayMs, docMaxDelayMs)
			if got != time.Duration(want)*time.Millisecond {
				t.Errorf("`%s` at n=%d: §9.3 says %s (= %d ms), CalculateDelay gives %s",
					strategy, n, formula, want, got)
			}
			checked++
		}
	}
	if checked == 0 {
		t.Fatal("no backoff formulae evaluated; the table moved or its header changed")
	}
}

// Both worked sequences must be what the implementation produces, including
// the clamp on the last row. They are separate tables in separate documents
// saying the same thing, so they are also checked against each other.
func TestDocTables_workedBackoffSequencesReproduce(t *testing.T) {
	sequences := []struct {
		doc        string
		where      string
		header     []string
		attemptCol string
		delayCol   string
	}{
		{"CLI_SPECIFICATION.md", "9.3 Backoff", []string{"Retry", "Delay"}, "Retry", "Delay"},
		{
			"MANUAL.md", "5.4 Retry logic",
			[]string{"Attempt", "Computed delay (no jitter)"},
			"Attempt", "Computed delay (no jitter)",
		},
	}

	var first []string
	for _, seq := range sequences {
		hdr, rows, err := docs.TableUnder(seq.doc, seq.where, seq.header...)
		if err != nil {
			t.Fatalf("%s under %q: %v", seq.doc, seq.where, err)
		}
		aCol, dCol := docs.Column(hdr, seq.attemptCol), docs.Column(hdr, seq.delayCol)
		if aCol < 0 || dCol < 0 {
			t.Fatalf("%s under %q lost a column: %v", seq.doc, seq.where, hdr)
		}

		var rendered []string
		for _, row := range rows {
			if len(row) <= dCol {
				continue
			}
			am := attemptValue.FindStringSubmatch(strings.TrimSpace(row[aCol]))
			dm := msValue.FindStringSubmatch(strings.TrimSpace(row[dCol]))
			if am == nil || dm == nil {
				t.Errorf("%s under %q: cannot read row %v", seq.doc, seq.where, row)
				continue
			}
			attempt, _ := strconv.Atoi(am[1])
			wantMs, _ := strconv.Atoi(dm[1])

			// Both tables number their first retry 1; CalculateDelay takes the
			// 0-based n the formula table uses.
			got := CalculateDelay(StrategyExponential, attempt-1, docInitialDelayMs, docMaxDelayMs)
			if got != time.Duration(wantMs)*time.Millisecond {
				t.Errorf("%s under %q: retry %d documents %d ms, CalculateDelay gives %s",
					seq.doc, seq.where, attempt, wantMs, got)
			}
			rendered = append(rendered, fmt.Sprintf("%d=%dms", attempt, wantMs))
		}
		if len(rendered) == 0 {
			t.Fatalf("%s under %q: no rows read", seq.doc, seq.where)
		}

		if first == nil {
			first = rendered
			continue
		}
		if strings.Join(first, " ") != strings.Join(rendered, " ") {
			t.Errorf("the two worked sequences disagree:\n  %s\n  %s",
				strings.Join(first, " "), strings.Join(rendered, " "))
		}
	}
}

// The prose under both tables promises jitter lands a 2000 ms delay in
// [1600, 2400] with jitter_factor 0.2. That is a range, so it is checked at
// its extremes rather than by sampling.
func TestDocTables_jitterRangeIsWhatBothDocumentsPromise(t *testing.T) {
	const (
		base   = 2000 * time.Millisecond
		factor = 0.2
		lowMs  = 1600
		highMs = 2400
	)

	// rng returns the documented random(-1, 1) at its bounds.
	for _, tc := range []struct {
		name   string
		r      float64
		wantMs int
	}{
		{"lower bound", -1, lowMs},
		{"upper bound", 1, highMs},
	} {
		got := ApplyJitter(base, factor, func() float64 { return tc.r })
		if got != time.Duration(tc.wantMs)*time.Millisecond {
			t.Errorf("jitter at %s: documents %d ms, ApplyJitter gives %s", tc.name, tc.wantMs, got)
		}
	}
}
