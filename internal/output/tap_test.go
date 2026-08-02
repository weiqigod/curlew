package output

import (
	"bytes"
	"strings"
	"testing"
)

func TestWriteTAP(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		passed  int
		failed  int
		check   func(t *testing.T, out string)
	}{
		{
			name:    "version line and plan line emitted",
			results: []TAPResult{{Name: "req", Passed: true, DurationMs: 50}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.HasPrefix(out, "TAP version 13\n") {
					t.Errorf("expected output to start with 'TAP version 13\\n', got: %q", out)
				}
				if !strings.Contains(out, "1..1\n") {
					t.Errorf("expected '1..1' plan line, got: %q", out)
				}
			},
		},
		{
			name:    "passing request emits ok line with duration",
			results: []TAPResult{{Name: "Get User", Passed: true, DurationMs: 120}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "ok 1 - Get User (120ms)\n") {
					t.Errorf("expected 'ok 1 - Get User (120ms)' line, got: %q", out)
				}
			},
		},
		{
			name: "failing request emits not ok line with YAML diagnostic block",
			results: []TAPResult{{
				Name:   "Get User",
				Passed: false,
				Failures: []TAPFailure{
					{Type: "status", Expected: "200", Actual: "404"},
				},
			}},
			passed: 0,
			failed: 1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "not ok 1 - Get User\n") {
					t.Errorf("expected 'not ok 1 - Get User' line, got: %q", out)
				}
				if !strings.Contains(out, "  ---\n") {
					t.Errorf("expected YAML block start '  ---', got: %q", out)
				}
				if !strings.Contains(out, "  ...\n") {
					t.Errorf("expected YAML block end '  ...', got: %q", out)
				}
				if !strings.Contains(out, "failures:") {
					t.Errorf("expected 'failures:' in YAML block, got: %q", out)
				}
			},
		},
		{
			name:    "skipped request emits ok line with SKIP directive",
			results: []TAPResult{{Name: "req", Skipped: true}},
			passed:  0,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "ok 1 - req # SKIP\n") {
					t.Errorf("expected 'ok 1 - req # SKIP' line, got: %q", out)
				}
			},
		},
		{
			name:    "error request emits not ok line with error diagnostic",
			results: []TAPResult{{Name: "req", Passed: false, Error: "connection refused"}},
			passed:  0,
			failed:  1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "not ok 1 - req\n") {
					t.Errorf("expected 'not ok 1 - req' line, got: %q", out)
				}
				if !strings.Contains(out, "connection refused") {
					t.Errorf("expected error message in output, got: %q", out)
				}
			},
		},
		{
			name:    "no results - plan is 1..0",
			results: nil,
			passed:  0,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "1..0\n") {
					t.Errorf("expected '1..0' plan line for empty results, got: %q", out)
				}
			},
		},
		{
			name: "mixed pass and fail - correct numbering",
			results: []TAPResult{
				{Name: "first", Passed: true, DurationMs: 10},
				{Name: "second", Passed: false, Failures: []TAPFailure{{Type: "status", Expected: "200", Actual: "500"}}},
				{Name: "third", Passed: true, DurationMs: 20},
			},
			passed: 2,
			failed: 1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "ok 1 - first") {
					t.Errorf("expected 'ok 1 - first', got: %q", out)
				}
				if !strings.Contains(out, "not ok 2 - second") {
					t.Errorf("expected 'not ok 2 - second', got: %q", out)
				}
				if !strings.Contains(out, "ok 3 - third") {
					t.Errorf("expected 'ok 3 - third', got: %q", out)
				}
				if !strings.Contains(out, "1..3\n") {
					t.Errorf("expected '1..3' plan line, got: %q", out)
				}
			},
		},
		{
			name: "multiple assertion failures all appear in diagnostic block",
			results: []TAPResult{{
				Name:   "req",
				Passed: false,
				Failures: []TAPFailure{
					{Type: "status", Expected: "200", Actual: "404"},
					{Type: "body $.id equals", Expected: "1", Actual: "2"},
				},
			}},
			passed: 0,
			failed: 1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.Count(out, "type:") != 2 {
					t.Errorf("expected 2 failure entries, got: %q", out)
				}
			},
		},
		{
			name:    "summary comment at end",
			results: []TAPResult{{Name: "req", Passed: true}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# Summary: 1 passed, 0 failed\n") {
					t.Errorf("expected summary comment, got: %q", out)
				}
			},
		},
		{
			name:    "hash in test name does not break TAP parsing",
			results: []TAPResult{{Name: "req # with hash", Passed: true}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				// # in the result line (outside the name) is the directive delimiter
				// The name should have # stripped so it cannot be parsed as a directive
				lines := strings.Split(out, "\n")
				for _, line := range lines {
					if strings.HasPrefix(line, "ok ") || strings.HasPrefix(line, "not ok ") {
						// Ensure no unintended # appears in the name portion
						// The sanitized name must not contain #
						dashIdx := strings.Index(line, " - ")
						if dashIdx >= 0 {
							namePart := line[dashIdx+3:]
							// Strip any trailing directive like # SKIP or (Nms)
							if strings.Contains(namePart, "#") {
								t.Errorf("# in test name not sanitized: %q", line)
							}
						}
					}
				}
			},
		},
		{
			name:    "failing request with no error and no failures emits empty diagnostic block",
			results: []TAPResult{{Name: "req", Passed: false}},
			passed:  0,
			failed:  1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "not ok 1 - req\n") {
					t.Errorf("expected 'not ok 1 - req' line, got: %q", out)
				}
				if !strings.Contains(out, "  ---\n") {
					t.Errorf("expected YAML block start '  ---', got: %q", out)
				}
				if !strings.Contains(out, "  ...\n") {
					t.Errorf("expected YAML block end '  ...', got: %q", out)
				}
				if strings.Contains(out, "error:") {
					t.Errorf("unexpected 'error:' in empty diagnostic block, got: %q", out)
				}
				if strings.Contains(out, "failures:") {
					t.Errorf("unexpected 'failures:' in empty diagnostic block, got: %q", out)
				}
			},
		},
		{
			name:    "passing request with retry count shows retry suffix",
			results: []TAPResult{{Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 2}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "ok 1 - Get User (120ms) (retry: 2)") {
					t.Errorf("expected retry suffix '(retry: 2)', got: %q", out)
				}
			},
		},
		{
			name:    "passing request with zero retry count has no retry suffix",
			results: []TAPResult{{Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 0}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "retry") {
					t.Errorf("expected no retry suffix when RetryCount=0, got: %q", out)
				}
			},
		},
		{
			name: "failing request with retry count shows retry suffix",
			results: []TAPResult{{
				Name:       "Get User",
				Passed:     false,
				RetryCount: 3,
				Failures:   []TAPFailure{{Type: "status", Expected: "200", Actual: "503"}},
			}},
			passed: 0,
			failed: 1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "not ok 1 - Get User (retry: 3)") {
					t.Errorf("expected retry suffix on failing request, got: %q", out)
				}
			},
		},
		{
			name: "failing request with zero retry count has no retry suffix",
			results: []TAPResult{{
				Name:       "Get User",
				Passed:     false,
				RetryCount: 0,
				Failures:   []TAPFailure{{Type: "status", Expected: "200", Actual: "503"}},
			}},
			passed: 0,
			failed: 1,
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "retry") {
					t.Errorf("expected no retry suffix when RetryCount=0, got: %q", out)
				}
			},
		},
		{
			name:    "request with no assertions and zero duration is ok",
			results: []TAPResult{{Name: "req", Passed: true, DurationMs: 0}},
			passed:  1,
			failed:  0,
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "ok 1 - req\n") {
					t.Errorf("expected 'ok 1 - req' line (no duration suffix), got: %q", out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed, nil); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}

func TestWriteTAP_WaveAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		passed  int
		failed  int
		check   func(t *testing.T, out string)
	}{
		{
			"wave comments inserted at wave boundaries",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: intPtr(0)},
				{Name: "B", Passed: true, DurationMs: 20, WaveIndex: intPtr(0)},
				{Name: "C", Passed: true, DurationMs: 15, WaveIndex: intPtr(1)},
			},
			3, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# Wave 1\n") {
					t.Errorf("expected '# Wave 1' comment, got: %q", out)
				}
				if !strings.Contains(out, "# Wave 2\n") {
					t.Errorf("expected '# Wave 2' comment, got: %q", out)
				}
			},
		},
		{
			"no wave comments when WaveIndex is nil",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10},
			},
			1, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "# Wave") {
					t.Errorf("unexpected wave comment for non-parallel results, got: %q", out)
				}
			},
		},
		{
			"single wave still shows wave comment",
			[]TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: intPtr(0)},
				{Name: "B", Passed: true, DurationMs: 20, WaveIndex: intPtr(0)},
			},
			2, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# Wave 1\n") {
					t.Errorf("expected '# Wave 1' comment, got: %q", out)
				}
				if strings.Count(out, "# Wave") != 1 {
					t.Errorf("expected exactly 1 wave comment, got: %q", out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed, nil); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}

func strPtr(s string) *string { return &s }

func TestWriteTAP_RetryDiagnostics(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		passed  int
		failed  int
		check   func(t *testing.T, out string)
	}{
		{
			"retry diagnostic comment shows attempt count",
			[]TAPResult{
				{
					Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 2,
					RetryDetails: []TAPRetryDetail{
						{Attempt: 1, StatusCode: 503, DurationMs: 20, DelayMs: 0},
						{Attempt: 2, StatusCode: 503, DurationMs: 25, DelayMs: 100},
						{Attempt: 3, StatusCode: 200, DurationMs: 30, DelayMs: 200},
					},
				},
			},
			1, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# retry: 3 attempt(s)") {
					t.Errorf("expected retry attempt count comment, got: %q", out)
				}
			},
		},
		{
			"no retry diagnostic when retry_count is 0",
			[]TAPResult{
				{Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 0},
			},
			1, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "# retry:") {
					t.Errorf("expected no retry diagnostic, got: %q", out)
				}
			},
		},
		{
			"retry diagnostic includes per-attempt details",
			[]TAPResult{
				{
					Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 1,
					RetryDetails: []TAPRetryDetail{
						{Attempt: 1, StatusCode: 503, DurationMs: 20, DelayMs: 0},
						{Attempt: 2, StatusCode: 200, DurationMs: 30, DelayMs: 100},
					},
				},
			},
			1, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# attempt 1: 503") {
					t.Errorf("expected attempt 1 detail, got: %q", out)
				}
				if !strings.Contains(out, "# attempt 2: 200") {
					t.Errorf("expected attempt 2 detail, got: %q", out)
				}
			},
		},
		{
			"retry diagnostic shows error for failed attempts",
			[]TAPResult{
				{
					Name: "Get User", Passed: true, DurationMs: 120, RetryCount: 1,
					RetryDetails: []TAPRetryDetail{
						{Attempt: 1, DurationMs: 5, DelayMs: 0, Error: "connection refused"},
						{Attempt: 2, StatusCode: 200, DurationMs: 30, DelayMs: 100},
					},
				},
			},
			1, 0,
			func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# attempt 1: error") {
					t.Errorf("expected error attempt detail, got: %q", out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed, nil); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}

// TestWriteTAP_ParallelSpeedup verifies the parallel-execution YAML diagnostic
// block emitted by WriteTAP when parallel.WaveCount > 1. (M11-003)
func TestWriteTAP_ParallelSpeedup(t *testing.T) {
	tests := []struct {
		name     string
		results  []TAPResult
		passed   int
		failed   int
		parallel *ParallelTAP
		check    func(t *testing.T, out string)
	}{
		{
			name: "speedup block emitted when wave_count > 1",
			results: []TAPResult{
				{Name: "A", Passed: true, DurationMs: 10, WaveIndex: intPtr(0)},
				{Name: "B", Passed: true, DurationMs: 20, WaveIndex: intPtr(1)},
			},
			passed:   2,
			failed:   0,
			parallel: &ParallelTAP{WaveCount: 2, MaxParallelism: 1, SpeedupFactor: 1.5},
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# Parallel execution:\n") {
					t.Errorf("missing parallel header comment, got: %q", out)
				}
				if !strings.Contains(out, "  speedup_factor: 1.5\n") {
					t.Errorf("missing speedup_factor line, got: %q", out)
				}
				if !strings.Contains(out, "  wave_count: 2\n") {
					t.Errorf("missing wave_count line, got: %q", out)
				}
				if !strings.Contains(out, "  max_parallelism: 1\n") {
					t.Errorf("missing max_parallelism line, got: %q", out)
				}
				// Block ordering: # Parallel execution: must come AFTER all
				// test-point lines and BEFORE # Summary:.
				headerIdx := strings.Index(out, "# Parallel execution:")
				summaryIdx := strings.Index(out, "# Summary:")
				if headerIdx < 0 || summaryIdx < 0 || headerIdx >= summaryIdx {
					t.Errorf("ordering wrong: parallel block must precede summary; got header=%d summary=%d", headerIdx, summaryIdx)
				}
			},
		},
		{
			name:     "no speedup block when parallel is nil",
			results:  []TAPResult{{Name: "A", Passed: true}},
			passed:   1,
			failed:   0,
			parallel: nil,
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "# Parallel execution:") {
					t.Errorf("unexpected parallel block on nil parallel arg, got: %q", out)
				}
				if strings.Contains(out, "speedup_factor") {
					t.Errorf("unexpected speedup_factor on nil parallel arg, got: %q", out)
				}
			},
		},
		{
			name:     "no speedup block when wave_count == 1",
			results:  []TAPResult{{Name: "A", Passed: true, WaveIndex: intPtr(0)}},
			passed:   1,
			failed:   0,
			parallel: &ParallelTAP{WaveCount: 1, MaxParallelism: 1, SpeedupFactor: 1.0},
			check: func(t *testing.T, out string) {
				t.Helper()
				if strings.Contains(out, "# Parallel execution:") {
					t.Errorf("unexpected parallel block for wave_count == 1, got: %q", out)
				}
			},
		},
		{
			name: "speedup block coexists with wave comments",
			results: []TAPResult{
				{Name: "A", Passed: true, WaveIndex: intPtr(0)},
				{Name: "B", Passed: true, WaveIndex: intPtr(1)},
			},
			passed:   2,
			failed:   0,
			parallel: &ParallelTAP{WaveCount: 2, MaxParallelism: 2, SpeedupFactor: 1.7},
			check: func(t *testing.T, out string) {
				t.Helper()
				if !strings.Contains(out, "# Wave 1\n") {
					t.Errorf("expected '# Wave 1' marker preserved, got: %q", out)
				}
				if !strings.Contains(out, "# Wave 2\n") {
					t.Errorf("expected '# Wave 2' marker preserved, got: %q", out)
				}
				if !strings.Contains(out, "# Parallel execution:\n") {
					t.Errorf("expected parallel diagnostic block, got: %q", out)
				}
			},
		},
		{
			name:     "speedup_factor formatted with one decimal",
			results:  []TAPResult{{Name: "A", Passed: true, WaveIndex: intPtr(0)}, {Name: "B", Passed: true, WaveIndex: intPtr(1)}},
			passed:   2,
			failed:   0,
			parallel: &ParallelTAP{WaveCount: 2, MaxParallelism: 1, SpeedupFactor: 2.0},
			check: func(t *testing.T, out string) {
				t.Helper()
				// 2.0 should serialize as "2.0", not "2"
				if !strings.Contains(out, "  speedup_factor: 2.0\n") {
					t.Errorf("expected speedup_factor formatted as 2.0, got: %q", out)
				}
			},
		},
		{
			name: "block sits between last test point and summary",
			results: []TAPResult{
				{Name: "A", Passed: true, WaveIndex: intPtr(0)},
				{Name: "B", Passed: true, WaveIndex: intPtr(1)},
			},
			passed:   2,
			failed:   0,
			parallel: &ParallelTAP{WaveCount: 2, MaxParallelism: 1, SpeedupFactor: 1.5},
			check: func(t *testing.T, out string) {
				t.Helper()
				lines := strings.Split(out, "\n")
				summaryIdx := -1
				blockEndIdx := -1
				for i, line := range lines {
					if strings.HasPrefix(line, "# Summary:") {
						summaryIdx = i
					}
					if line == "  ..." {
						blockEndIdx = i
					}
				}
				if blockEndIdx < 0 || summaryIdx < 0 || blockEndIdx >= summaryIdx {
					t.Errorf("YAML block must close before summary; got blockEnd=%d summary=%d in:\n%s", blockEndIdx, summaryIdx, out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed, tc.parallel); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}

func TestWriteTAP_DataDrivenAnnotations(t *testing.T) {
	tests := []struct {
		name    string
		results []TAPResult
		passed  int
		failed  int
		check   func(t *testing.T, out string)
	}{
		{
			"data-driven comment at group start",
			[]TAPResult{
				{Name: "Create User [1/3]", Passed: true, DurationMs: 50, DataDrivenGroup: strPtr("Create User (3 iterations)")},
				{Name: "Create User [2/3]", Passed: true, DurationMs: 60},
				{Name: "Create User [3/3]", Passed: true, DurationMs: 55},
			},
			3, 0,
			func(t *testing.T, out string) {
				if !strings.Contains(out, "# Data-Driven: Create User (3 iterations)") {
					t.Errorf("expected data-driven comment, got: %q", out)
				}
			},
		},
		{
			"no comment when not data-driven",
			[]TAPResult{
				{Name: "Get Users", Passed: true, DurationMs: 100},
				{Name: "Get User", Passed: true, DurationMs: 80},
			},
			2, 0,
			func(t *testing.T, out string) {
				if strings.Contains(out, "# Data-Driven") {
					t.Errorf("unexpected data-driven comment, got: %q", out)
				}
			},
		},
		{
			"data-driven comment only on first of group",
			[]TAPResult{
				{Name: "A [1/2]", Passed: true, DurationMs: 10, DataDrivenGroup: strPtr("A (2 iterations)")},
				{Name: "A [2/2]", Passed: true, DurationMs: 20},
			},
			2, 0,
			func(t *testing.T, out string) {
				if strings.Count(out, "# Data-Driven") != 1 {
					t.Errorf("expected exactly 1 data-driven comment, got: %q", out)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, tc.passed, tc.failed, nil); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			tc.check(t, buf.String())
		})
	}
}

// TestWriteTAP_SkipReason verifies that when a TAPResult has SkipReason set,
// the TAP output includes an indented "# SKIP <reason>" comment after the ok line.
func TestWriteTAP_SkipReason(t *testing.T) {
	tests := []struct {
		name       string
		results    []TAPResult
		wantSubstr []string
		wantAbsent []string
	}{
		{
			name:       "skip reason emits comment line",
			results:    []TAPResult{{Name: "req", Skipped: true, SkipReason: "if: false"}},
			wantSubstr: []string{"ok 1 - req # SKIP\n", "  # SKIP if: false\n"},
		},
		{
			name:       "skip reason emits parent-skipped comment",
			results:    []TAPResult{{Name: "req", Skipped: true, SkipReason: "parent skipped: A"}},
			wantSubstr: []string{"ok 1 - req # SKIP\n", "  # SKIP parent skipped: A\n"},
		},
		{
			name:       "skip without reason has no comment line",
			results:    []TAPResult{{Name: "req", Skipped: true, SkipReason: ""}},
			wantSubstr: []string{"ok 1 - req # SKIP\n"},
			wantAbsent: []string{"  # SKIP \n"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			if err := WriteTAP(&buf, tc.results, 0, 0, nil); err != nil {
				t.Fatalf("WriteTAP error: %v", err)
			}
			out := buf.String()
			for _, s := range tc.wantSubstr {
				if !strings.Contains(out, s) {
					t.Errorf("output does not contain %q; got:\n%s", s, out)
				}
			}
			for _, s := range tc.wantAbsent {
				if strings.Contains(out, s) {
					t.Errorf("output should not contain %q; got:\n%s", s, out)
				}
			}
		})
	}
}
