package harness_test

import (
	"strings"
	"testing"
)

// TestContract_Match covers the contract matcher logic end-to-end using
// synthesised event streams and expectation objects.
func TestContract_Match(t *testing.T) {
	tests := []struct {
		name       string
		expect     Expectation
		stream     []map[string]any
		exitCode   int
		wantErrSub string // empty = no error expected
	}{
		{
			name: "exact match",
			expect: Expectation{
				ExitCode: 5,
				Events: []ExpectedEvent{{
					Kind: "run.error",
					MustHave: map[string]any{
						"error.category": "input",
						"error.code":     "VAR_UNDEFINED",
					},
					HintContainsAny: []string{"Set", "Define"},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.start", "schema_version": "0.1"},
				{
					"kind": "run.error",
					"error": map[string]any{
						"category": "input",
						"code":     "VAR_UNDEFINED",
						"message":  "undefined variable",
						"hint":     "Define the variable in the environment file.",
						"file":     "collection.yaml",
						"line":     float64(5),
					},
				},
				{"kind": "run.end", "exit_code": float64(5)},
			},
			exitCode:   5,
			wantErrSub: "",
		},
		{
			name: "missing kind",
			expect: Expectation{
				Events: []ExpectedEvent{{Kind: "run.error"}},
			},
			stream: []map[string]any{
				{"kind": "run.start"},
				{"kind": "run.end", "exit_code": float64(0)},
			},
			exitCode:   0,
			wantErrSub: "no event with kind=run.error",
		},
		{
			name: "empty file when nonempty required",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind: "run.error",
					MustHave: map[string]any{
						"error.file_nonempty": true,
					},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.error", "error": map[string]any{"category": "input", "file": ""}},
			},
			wantErrSub: "error.file is empty, want non-empty",
		},
		{
			name: "line=0 when nonzero required",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind: "run.error",
					MustHave: map[string]any{
						"error.line_nonzero": true,
					},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.error", "error": map[string]any{"category": "parse", "line": float64(0)}},
			},
			wantErrSub: "error.line is 0, want non-zero",
		},
		{
			name: "hint contains none of allowed verbs",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:            "run.error",
					HintContainsAny: []string{"Set", "Add", "Pass"},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.error", "error": map[string]any{"hint": "something happened", "category": "input"}},
			},
			wantErrSub: "contains none of [Set Add Pass]",
		},
		{
			name: "line=0 allowed for category=network",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind: "request.end",
					MustHave: map[string]any{
						"error.category":     "network",
						"error.line_nonzero": true,
					},
				}},
			},
			stream: []map[string]any{
				{
					"kind": "request.end",
					"error": map[string]any{
						"category": "network",
						"line":     float64(0),
					},
				},
			},
			wantErrSub: "", // network category allows line=0
		},
		{
			name: "pattern mismatch",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind: "run.error",
					MustHave: map[string]any{
						"error.code_pattern": "^VAR_",
					},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.error", "error": map[string]any{"code": "PARSE_INVALID_YAML"}},
			},
			wantErrSub: "does not match pattern",
		},
		{
			name: "exit code mismatch",
			expect: Expectation{
				ExitCode: 5,
				Events:   []ExpectedEvent{},
			},
			stream: []map[string]any{
				{"kind": "run.end", "exit_code": float64(1)},
			},
			exitCode:   1,
			wantErrSub: "exit_code = 1, want 5",
		},
		{
			name: "exact match on boolean value passed=false",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:     "assertion.result",
					MustHave: map[string]any{"passed": false},
				}},
			},
			stream: []map[string]any{
				{"kind": "assertion.result", "passed": false},
			},
			wantErrSub: "",
		},
		{
			name: "boolean value mismatch",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:     "assertion.result",
					MustHave: map[string]any{"passed": false},
				}},
			},
			stream: []map[string]any{
				{"kind": "assertion.result", "passed": true},
			},
			wantErrSub: "passed = true, want false",
		},
		{
			name: "exact match on float64 value",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:     "request.end",
					MustHave: map[string]any{"status_code": float64(200)},
				}},
			},
			stream: []map[string]any{
				{"kind": "request.end", "status_code": float64(200)},
			},
			wantErrSub: "",
		},
		{
			name: "fmt.Sprintf fallback comparison for unrecognised type",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:     "run.error",
					MustHave: map[string]any{"error.category": "input"},
				}},
			},
			stream: []map[string]any{
				{"kind": "run.error", "error": map[string]any{"category": "input"}},
			},
			wantErrSub: "",
		},
		{
			// int64 values can appear in MustHave when constructed directly in Go
			// (YAML decodes small integers as int, but large integers as int64;
			// this test exercises the int64 branch in toFloat64 via a Go-level
			// synthetic expectation so coverage reaches that defensive branch).
			name: "exact match on int64 value",
			expect: Expectation{
				Events: []ExpectedEvent{{
					Kind:     "request.end",
					MustHave: map[string]any{"status_code": int64(200)},
				}},
			},
			stream: []map[string]any{
				{"kind": "request.end", "status_code": float64(200)},
			},
			wantErrSub: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.expect.Verify(tt.stream, tt.exitCode)
			if tt.wantErrSub == "" {
				if err != nil {
					t.Errorf("expected no error; got: %v", err)
				}
				return
			}
			if err == nil {
				t.Fatalf("expected error containing %q; got nil", tt.wantErrSub)
			}
			if !strings.Contains(err.Error(), tt.wantErrSub) {
				t.Errorf("error = %q\nwant substring: %q", err.Error(), tt.wantErrSub)
			}
		})
	}
}

// TestContract_Load verifies that well-formed expect.yaml files parse correctly.
func TestContract_Load(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
	}{
		{
			name: "minimal valid expectation",
			yaml: `
exit_code: 5
events:
  - kind: run.error
    must_have:
      error.category: input
    hint_contains_any:
      - Set
`,
		},
		{
			name: "expectation with nonempty/nonzero specifiers",
			yaml: `
exit_code: 1
events:
  - kind: request.end
    must_have:
      error.category: network
      error.file_nonempty: true
      error.line_nonzero: true
`,
		},
		{
			name: "empty events list",
			yaml: `
exit_code: 0
events: []
`,
		},
		{
			name:    "invalid yaml",
			yaml:    "  - - - [invalid",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseExpectation([]byte(tt.yaml))
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseExpectation error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
