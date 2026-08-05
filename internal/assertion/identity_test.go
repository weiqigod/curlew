package assertion

import (
	"net/http"
	"testing"
	"time"
)

// M24-001: Result.Type was doing two incompatible jobs — a machine-readable
// discriminator and a human-readable label — and the label won. Every kind
// except "status" was emitted as a composite string ("body $.user.name
// equals"), which no published JSON Schema has ever permitted.
//
// These tests pin both halves of the split: the triple must be structured,
// and Label() must reproduce the historical string byte-for-byte so that
// terminal, HTML, pr-check and UI output do not move.

func TestResult_carries_target_and_operator(t *testing.T) {
	body := []byte(`{"user":{"name":"bob"}}`)

	t.Run("body", func(t *testing.T) {
		got := CheckBody([]BodyInput{{Path: "$.user.name", Operator: "equals", Value: "alice"}}, body)
		if len(got) != 1 {
			t.Fatalf("got %d results, want 1", len(got))
		}
		assertIdentity(t, got[0], "body", "$.user.name", "equals")
	})

	t.Run("header", func(t *testing.T) {
		h := http.Header{}
		h.Set("X-Request-Id", "abc")
		got := CheckHeaders([]HeaderInput{{Name: "X-Request-Id", Operator: "equals", Value: "abc"}}, h)
		if len(got) != 1 {
			t.Fatalf("got %d results, want 1", len(got))
		}
		assertIdentity(t, got[0], "header", "X-Request-Id", "equals")
	})

	t.Run("status has no target or operator", func(t *testing.T) {
		got := CheckStatus([]int{200}, 404)
		if got == nil {
			t.Fatal("CheckStatus returned nil")
		}
		assertIdentity(t, *got, "status", "", "")
	})

	t.Run("timing has no target or operator", func(t *testing.T) {
		got := CheckTiming(100, 250*time.Millisecond)
		if got == nil {
			t.Fatal("CheckTiming returned nil")
		}
		assertIdentity(t, *got, "timing", "", "")
	})

	t.Run("body assertion on a non-JSON body keeps its identity", func(t *testing.T) {
		// The parse-error path builds its own Result; it must not lose the
		// target, which is the only thing telling the reader what failed.
		got := CheckBody([]BodyInput{{Path: "$.user.name", Operator: "equals", Value: "alice"}}, []byte("not json"))
		if len(got) != 1 {
			t.Fatalf("got %d results, want 1", len(got))
		}
		assertIdentity(t, got[0], "body", "$.user.name", "equals")
	})

	t.Run("body assertion at a missing path keeps its identity", func(t *testing.T) {
		got := CheckBody([]BodyInput{{Path: "$.nope", Operator: "contains", Value: "x"}}, body)
		if len(got) != 1 {
			t.Fatalf("got %d results, want 1", len(got))
		}
		assertIdentity(t, got[0], "body", "$.nope", "contains")
	})
}

func assertIdentity(t *testing.T, r Result, wantType, wantTarget, wantOperator string) {
	t.Helper()
	if r.Type != wantType {
		t.Errorf("Type = %q, want %q", r.Type, wantType)
	}
	if r.Target != wantTarget {
		t.Errorf("Target = %q, want %q", r.Target, wantTarget)
	}
	if r.Operator != wantOperator {
		t.Errorf("Operator = %q, want %q", r.Operator, wantOperator)
	}
}

// TestResult_Label_reproduces_historical_strings is the no-regression contract
// for every human-facing surface. Each want string below is the exact value
// Result.Type carried before the split; if one of these moves, terminal
// output, HTML reports, pr-check messages and the UI all moved with it.
func TestResult_Label_reproduces_historical_strings(t *testing.T) {
	cases := []struct {
		name string
		r    Result
		want string
	}{
		{
			name: "status",
			r:    Result{Type: "status"},
			want: "status",
		},
		{
			name: "timing",
			r:    Result{Type: "timing"},
			want: "timing",
		},
		{
			name: "body equals",
			r:    Result{Type: "body", Target: "$.user.name", Operator: "equals"},
			want: "body $.user.name equals",
		},
		{
			name: "body exists",
			r:    Result{Type: "body", Target: "$.user.id", Operator: "exists"},
			want: "body $.user.id exists",
		},
		{
			name: "body contains_all",
			r:    Result{Type: "body", Target: "$.items", Operator: "contains_all"},
			want: "body $.items contains_all",
		},
		{
			name: "header equals",
			r:    Result{Type: "header", Target: "X-Request-Id", Operator: "equals"},
			want: "header X-Request-Id equals",
		},
		{
			// schema.go builds its tag from the absolute schema file path in
			// the "body is not JSON" and "valid" branches...
			name: "schema by file path",
			r:    Result{Type: "schema", Target: "/tmp/user.schema.json"},
			want: "schema /tmp/user.schema.json",
		},
		{
			// ...and from a JSONPath into the instance for each validation
			// error. Both are legitimate targets for the same type.
			name: "schema by instance path",
			r:    Result{Type: "schema", Target: "$.data.id"},
			want: "schema $.data.id",
		},
		{
			// CEL is the one kind whose historical label puts the type last.
			name: "cel",
			r:    Result{Type: "cel", Target: "assertions[0]"},
			want: "assertions[0].cel",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := tc.r.Label(); got != tc.want {
				t.Errorf("Label() = %q, want %q", got, tc.want)
			}
		})
	}
}
