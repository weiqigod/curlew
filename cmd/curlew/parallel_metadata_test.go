package main

import (
	"bytes"
	"encoding/json"
	"math"
	"regexp"
	"strconv"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/output"
	"github.com/weiqigod/curlew/internal/runner"
)

// The TAP and JSON formatters must report the same parallel-execution metadata.
// One `curlew run` emits one format — --report is bound to the stdout format
// rather than being an independent emitter — so that invariant cannot be
// observed by running the binary twice: speedup_factor is derived from
// wall-clock durations, and two executions of the same collection legitimately
// measure different speedups. Measured over 25 identical runs, this collection
// produced 0.7, 0.9 and 1.0.
//
// So the invariant is tested here instead, where the two paths meet:
// buildParallelMetadata is called identically at main.go:1106 (TAP) and
// main.go:1563 (JSON), and the results are rendered by output.WriteTAP (%.1f)
// and encoding/json (raw float64). Feeding one fixed summary through both and
// comparing what comes out is both deterministic and stricter than the
// cross-run comparison it replaces — it catches formatting divergence, which
// two noisy wall-clock samples could only catch by luck.

// summaryFor builds a runner.Summary with the timing fields buildParallelMetadata reads.
func summaryFor(waveCount, maxParallelism int, total time.Duration, waves ...time.Duration) *runner.Summary {
	return &runner.Summary{
		WaveCount:      waveCount,
		MaxParallelism: maxParallelism,
		Duration:       total,
		WaveDurations:  waves,
		IsParallel:     true,
	}
}

// TestBuildParallelMetadata pins the speedup formula: sum(WaveDurations) over
// Duration, rounded to one decimal, and zero whenever the run was not actually
// parallel or was not measurably long.
func TestBuildParallelMetadata(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name         string
		summary      *runner.Summary
		wantWaves    int
		wantParallel int
		wantSpeedup  float64
	}{
		{"nil summary", nil, 0, 0, 0},
		{"two waves, twice as fast", summaryFor(2, 2, 100*ms, 100*ms, 100*ms), 2, 2, 2.0},
		{"three waves, no speedup", summaryFor(3, 1, 300*ms, 100*ms, 100*ms, 100*ms), 3, 1, 1.0},
		{"single wave means no speedup to report", summaryFor(1, 3, 100*ms, 100*ms), 1, 3, 0},
		{"zero waves", summaryFor(0, 0, 100*ms), 0, 0, 0},
		{"zero duration cannot be divided by", summaryFor(2, 2, 0, 50*ms, 50*ms), 2, 2, 0},
		{"no wave durations recorded", summaryFor(2, 2, 100*ms), 2, 2, 0},
		{"rounds down below the half step", summaryFor(2, 2, 100*ms, 60*ms, 74*ms), 2, 2, 1.3},
		{"rounds up at the half step", summaryFor(2, 2, 100*ms, 60*ms, 75*ms), 2, 2, 1.4},
		{"repeating decimal is rounded, not truncated", summaryFor(2, 2, 300*ms, 250*ms, 250*ms), 2, 2, 1.7},
		{"slower than sequential is reported honestly", summaryFor(2, 2, 200*ms, 40*ms, 40*ms), 2, 2, 0.4},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			waves, parallelism, speedup := buildParallelMetadata(tc.summary)
			if waves != tc.wantWaves {
				t.Errorf("waveCount = %d, want %d", waves, tc.wantWaves)
			}
			if parallelism != tc.wantParallel {
				t.Errorf("maxParallelism = %d, want %d", parallelism, tc.wantParallel)
			}
			if math.Abs(speedup-tc.wantSpeedup) > 1e-9 {
				t.Errorf("speedup = %v, want %v", speedup, tc.wantSpeedup)
			}
		})
	}
}

// renderTAPSpeedup runs a summary through the real TAP metadata constructor and
// the TAP formatter, then reads speedup_factor back out of the diagnostic block.
func renderTAPSpeedup(t *testing.T, summary *runner.Summary) (float64, string) {
	t.Helper()
	var buf bytes.Buffer
	err := output.WriteTAP(&buf, []output.TAPResult{{Name: "r", Passed: true}}, 1, 0,
		newParallelTAP(summary))
	if err != nil {
		t.Fatalf("WriteTAP: %v", err)
	}
	got := buf.String()
	m := regexp.MustCompile(`(?m)^\s+speedup_factor: (\S+)`).FindStringSubmatch(got)
	if len(m) != 2 {
		t.Fatalf("no speedup_factor in TAP output:\n%s", got)
	}
	v, err := strconv.ParseFloat(m[1], 64)
	if err != nil {
		t.Fatalf("TAP speedup_factor %q is not a float: %v", m[1], err)
	}
	return v, m[1]
}

// renderJSONSpeedup runs the same summary through the real JSON metadata
// constructor and encoding/json, so a change in either the constructor or the
// struct tag is visible here.
func renderJSONSpeedup(t *testing.T, summary *runner.Summary) (float64, string) {
	t.Helper()
	raw, err := json.Marshal(newParallelExecutionJSON(summary))
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var back map[string]any
	if err := json.Unmarshal(raw, &back); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	v, ok := back["speedup_factor"].(float64)
	if !ok {
		// omitempty drops a zero speedup; that is the documented shape.
		return 0, "absent"
	}
	m := regexp.MustCompile(`"speedup_factor":([^,}]+)`).FindStringSubmatch(string(raw))
	if len(m) != 2 {
		t.Fatalf("could not read raw speedup_factor from %s", raw)
	}
	return v, m[1]
}

// TestParallelMetadata_formatters_agree is the deterministic replacement for the
// cross-run comparison that used to live in TestTAPOutput_ParallelSpeedup. One
// summary, one call to buildParallelMetadata, rendered through both formatters:
// both must report the same number.
//
// Comparing the parsed values rather than the rendered text is deliberate. The
// two formats legitimately serialise numbers differently — TAP prints %.1f, so a
// speedup of 2 renders as "2.0", while encoding/json emits "2" — and both mean
// the same thing to a consumer.
//
// What the comparison does catch is the rounding being lost. buildParallelMetadata
// rounds to one decimal before either formatter sees the value; drop that and TAP
// would still print 1.7 (its %.1f is a display format, not a rounding step) while
// JSON emitted 1.6666666666666667, so the parsed values diverge. The old
// wall-clock assertion could only have caught that by luck.
func TestParallelMetadata_formatters_agree(t *testing.T) {
	ms := time.Millisecond
	tests := []struct {
		name    string
		summary *runner.Summary
	}{
		{"exact value", summaryFor(2, 2, 100*ms, 100*ms, 100*ms)},
		{"repeating decimal before rounding", summaryFor(2, 2, 300*ms, 250*ms, 250*ms)},
		{"rounds up at the half step", summaryFor(2, 2, 100*ms, 60*ms, 75*ms)},
		{"below one", summaryFor(2, 2, 200*ms, 40*ms, 40*ms)},
		{"three waves", summaryFor(3, 4, 120*ms, 50*ms, 50*ms, 50*ms)},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Each helper calls the same constructor the production path calls,
			// so a divergence introduced in either is caught here.
			tapValue, tapText := renderTAPSpeedup(t, tc.summary)
			jsonValue, _ := renderJSONSpeedup(t, tc.summary)

			if tapValue != jsonValue {
				t.Errorf("speedup_factor: TAP %v, JSON %v — the two paths have diverged", tapValue, jsonValue)
			}
			// TAP's contract is one decimal place. Asserting it against the
			// shared value means the comparison above cannot be satisfied by TAP
			// silently re-rounding something the JSON path emitted unrounded.
			_, _, speedup := buildParallelMetadata(tc.summary)
			if want := strconv.FormatFloat(speedup, 'f', 1, 64); tapText != want {
				t.Errorf("TAP rendered speedup_factor as %q, want %q (one decimal place)", tapText, want)
			}
		})
	}
}

// TestParallelMetadata_call_sites_agree guards the arrangement the test above
// relies on: both formatters are fed from buildParallelMetadata rather than
// from a locally recomputed value. It runs the real collection once per format
// and compares the fields that do not depend on the clock — wave_count and
// max_parallelism are properties of the dependency graph, so they are identical
// across runs where speedup_factor is not.
func TestParallelMetadata_call_sites_agree(t *testing.T) {
	dir := t.TempDir()
	srv := newParallelFixtureServer(t)
	colFile := writeCollection(t, dir, "col.yaml", parallelFixtureCollection(srv.URL))

	tapStdout, _, tapExit := captureRunCmd(t, colFile, "--format", "tap", "--parallel")
	if tapExit != 0 {
		t.Fatalf("tap exit = %d, want 0\nstdout: %s", tapExit, tapStdout)
	}
	jsonStdout, _, jsonExit := captureRunCmd(t, colFile, "--format", "json", "--parallel")
	if jsonExit != 0 {
		t.Fatalf("json exit = %d, want 0\nstdout: %s", jsonExit, jsonStdout)
	}

	var jsonOut map[string]any
	if err := json.Unmarshal([]byte(jsonStdout), &jsonOut); err != nil {
		t.Fatalf("invalid JSON output: %v\nraw: %s", err, jsonStdout)
	}
	pe, ok := jsonOut["parallel_execution"].(map[string]any)
	if !ok {
		t.Fatalf("missing parallel_execution in JSON output: %v", jsonOut)
	}

	tests := []struct {
		name    string
		tapKey  string
		jsonKey string
	}{
		{"wave_count", "wave_count", "wave_count"},
		{"max_parallelism", "max_parallelism", "max_parallelism"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			m := regexp.MustCompile(`(?m)^\s+` + tc.tapKey + `: (\d+)`).FindStringSubmatch(tapStdout)
			if len(m) != 2 {
				t.Fatalf("could not parse %s from TAP output:\n%s", tc.tapKey, tapStdout)
			}
			tapValue, err := strconv.Atoi(m[1])
			if err != nil {
				t.Fatalf("TAP %s %q is not an integer: %v", tc.tapKey, m[1], err)
			}
			jsonFloat, ok := pe[tc.jsonKey].(float64)
			if !ok {
				t.Fatalf("JSON parallel_execution missing %s: %v", tc.jsonKey, pe)
			}
			if tapValue != int(jsonFloat) {
				t.Errorf("%s: TAP %d, JSON %d — the two paths have diverged", tc.tapKey, tapValue, int(jsonFloat))
			}
		})
	}
}
