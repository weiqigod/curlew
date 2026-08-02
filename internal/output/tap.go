package output

import (
	"fmt"
	"io"
	"strings"
)

// TAPRetryDetail holds per-attempt info for TAP diagnostics.
type TAPRetryDetail struct {
	Attempt    int
	StatusCode int
	DurationMs int64
	DelayMs    int64
	Error      string
}

// ParallelTAP carries parallel-execution metadata for a TAP YAML diagnostic
// block. Mirror of ParallelExecutionJSON in fields and computation:
// SpeedupFactor is sum(WaveDurations) / Duration, rounded to one decimal by
// the caller. WriteTAP only emits the diagnostic block when WaveCount > 1;
// single-wave runs are not informative. (M11-003)
type ParallelTAP struct {
	WaveCount      int
	MaxParallelism int
	SpeedupFactor  float64
}

// TAPResult represents one test point in TAP version 13 output.
type TAPResult struct {
	Name            string
	Passed          bool
	Skipped         bool
	SkipReason      string           // non-empty human-readable reason when Skipped is true (M19-001)
	Error           string           // non-empty for execution errors (not assertion failures)
	Failures        []TAPFailure     // individual assertion failures (only when !Passed && Error == "")
	DurationMs      int64            // 0 for skipped/error requests
	RetryCount      int              // included in description if > 0
	RetryDetails    []TAPRetryDetail // shown as diagnostic comment when non-empty
	WaveIndex       *int             // nil = non-parallel; 0+ = wave index (0-based)
	DataDrivenGroup *string          // non-nil on first result of a data-driven group; value = "Name (N iterations)"
}

// TAPFailure represents a single assertion failure for TAP YAML diagnostics.
type TAPFailure struct {
	Type     string
	Expected string
	Actual   string
}

// WriteTAP writes TAP version 13 formatted output to w.
// The plan line uses len(results) as the total test count.
// A trailing comment line summarises pass/fail counts.
//
// When parallel is non-nil and parallel.WaveCount > 1, a YAML diagnostic
// block carrying speedup_factor, wave_count, and max_parallelism is emitted
// between the last test point and the trailing summary comment. Single-wave
// runs and sequential runs pass nil. (M11-003)
func WriteTAP(w io.Writer, results []TAPResult, passed, failed int, parallel *ParallelTAP) error {
	if _, err := fmt.Fprintln(w, "TAP version 13"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "1..%d\n", len(results)); err != nil {
		return err
	}

	currentWave := -1
	for i, r := range results {
		// Insert data-driven group comment
		if r.DataDrivenGroup != nil {
			if _, err := fmt.Fprintf(w, "# Data-Driven: %s\n", *r.DataDrivenGroup); err != nil {
				return err
			}
		}

		// Insert wave comment at wave boundaries
		if r.WaveIndex != nil && *r.WaveIndex != currentWave {
			currentWave = *r.WaveIndex
			if _, err := fmt.Fprintf(w, "# Wave %d\n", currentWave+1); err != nil {
				return err
			}
		}

		n := i + 1
		name := sanitizeTAPName(r.Name)

		retrySuffix := ""
		if r.RetryCount > 0 {
			retrySuffix = fmt.Sprintf(" (retry: %d)", r.RetryCount)
		}

		switch {
		case r.Skipped:
			if _, err := fmt.Fprintf(w, "ok %d - %s # SKIP\n", n, name); err != nil {
				return err
			}
			if r.SkipReason != "" {
				if _, err := fmt.Fprintf(w, "  # SKIP %s\n", r.SkipReason); err != nil {
					return err
				}
			}
		case !r.Passed:
			if _, err := fmt.Fprintf(w, "not ok %d - %s%s\n", n, name, retrySuffix); err != nil {
				return err
			}
			if err := writeTAPDiagnostics(w, r); err != nil {
				return err
			}
		default:
			if r.DurationMs > 0 {
				if _, err := fmt.Fprintf(w, "ok %d - %s (%dms)%s\n", n, name, r.DurationMs, retrySuffix); err != nil {
					return err
				}
			} else {
				if _, err := fmt.Fprintf(w, "ok %d - %s%s\n", n, name, retrySuffix); err != nil {
					return err
				}
			}
		}

		// Write retry diagnostic comments when retry details are present.
		if r.RetryCount > 0 && len(r.RetryDetails) > 0 {
			if _, err := fmt.Fprintf(w, "  # retry: %d attempt(s)\n", r.RetryCount+1); err != nil {
				return err
			}
			for _, d := range r.RetryDetails {
				if d.Error != "" {
					if _, err := fmt.Fprintf(w, "  # attempt %d: error (%dms)\n", d.Attempt, d.DurationMs); err != nil {
						return err
					}
				} else {
					if _, err := fmt.Fprintf(w, "  # attempt %d: %d (%dms, delay %dms)\n", d.Attempt, d.StatusCode, d.DurationMs, d.DelayMs); err != nil {
						return err
					}
				}
			}
		}
	}

	// Parallel execution YAML diagnostic. Emitted only when wave_count > 1;
	// single-wave runs already convey their structure via per-test-point
	// # Wave M comments. (M11-003)
	if parallel != nil && parallel.WaveCount > 1 {
		if err := writeTAPParallelDiagnostic(w, parallel); err != nil {
			return err
		}
	}

	_, err := fmt.Fprintf(w, "# Summary: %d passed, %d failed\n", passed, failed)
	return err
}

// writeTAPParallelDiagnostic writes a free-comment header line followed by a
// TAP 13 YAML diagnostic block (two-space indented, bracketed by `  ---` and
// `  ...`) carrying speedup_factor, wave_count, and max_parallelism. (M11-003)
func writeTAPParallelDiagnostic(w io.Writer, p *ParallelTAP) error {
	if _, err := fmt.Fprintln(w, "# Parallel execution:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, "  ---"); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  speedup_factor: %.1f\n", p.SpeedupFactor); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  wave_count: %d\n", p.WaveCount); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(w, "  max_parallelism: %d\n", p.MaxParallelism); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, "  ...")
	return err
}

// writeTAPDiagnostics writes a TAP 13 YAML diagnostic block for a failing result.
func writeTAPDiagnostics(w io.Writer, r TAPResult) error {
	if _, err := fmt.Fprintln(w, "  ---"); err != nil {
		return err
	}
	if r.Error != "" {
		if _, err := fmt.Fprintf(w, "  error: %q\n", r.Error); err != nil {
			return err
		}
	} else if len(r.Failures) > 0 {
		if _, err := fmt.Fprintln(w, "  failures:"); err != nil {
			return err
		}
		for _, f := range r.Failures {
			if _, err := fmt.Fprintf(w, "    - type: %q\n      expected: %q\n      actual: %q\n",
				f.Type, f.Expected, f.Actual); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w, "  ...")
	return err
}

// sanitizeTAPName removes characters that could break TAP parsing.
// TAP uses '#' as a directive delimiter, so we strip it from names.
// Whitespace runs (including those left by stripped characters) are collapsed.
func sanitizeTAPName(name string) string {
	name = strings.ReplaceAll(name, "#", "")
	name = strings.ReplaceAll(name, "\n", " ")
	return strings.Join(strings.Fields(name), " ")
}
