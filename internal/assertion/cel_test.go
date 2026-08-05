package assertion

import (
	"strings"
	"testing"

	apicel "github.com/weiqigod/curlew/internal/cel"
)

// testCELContext creates a minimal CELContext for tests.
func testCELContext(t *testing.T) CELContext {
	t.Helper()
	ev, err := apicel.NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return CELContext{
		Evaluator: ev,
		ProgCache: make(map[string]apicel.Program),
		Response:  &apicel.Response{Status: 200, Body: map[string]any{}},
		Previous:  nil,
		Vars:      map[string]any{},
		Env:       map[string]string{},
	}
}

func TestCelAssertion_TruePasses(t *testing.T) {
	ctx := testCELContext(t)
	results := CheckCEL(
		[]CELInput{{Index: 0, Source: "1 + 1 == 2"}},
		ctx,
	)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("want pass, got fail: actual=%q", results[0].Actual)
	}
}

func TestCelAssertion_FalseFails(t *testing.T) {
	ctx := testCELContext(t)
	ctx.Response = &apicel.Response{Status: 200, Body: map[string]any{"total": 9.5}}
	results := CheckCEL(
		[]CELInput{{Index: 0, Source: "response.body.total == 10.0"}},
		ctx,
	)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("want fail")
	}
	if !strings.Contains(results[0].Actual, "response.body.total") {
		t.Errorf("failure message missing ref: actual=%q", results[0].Actual)
	}
	if !strings.Contains(results[0].Actual, "9.5") {
		t.Errorf("failure message missing value: actual=%q", results[0].Actual)
	}
}

func TestCelAssertion_FailureMessageIncludesResolvedSubvalues(t *testing.T) {
	ctx := testCELContext(t)
	ctx.Response = &apicel.Response{Status: 200, Body: map[string]any{
		"total": 9.5,
		"items": []any{
			map[string]any{"price": 5.0},
			map[string]any{"price": 5.5},
		},
	}}
	// response.body.total (9.5) != double(items.size()) (2.0) — assertion fails.
	src := "response.body.total == double(response.body.items.size())"
	results := CheckCEL([]CELInput{{Index: 0, Source: src}}, ctx)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("want fail")
	}
	if !strings.Contains(results[0].Actual, "response.body.total") {
		t.Errorf("missing response.body.total subvalue: %q", results[0].Actual)
	}
	// 9.5 should appear as the resolved value of response.body.total.
	if !strings.Contains(results[0].Actual, "9.5") {
		t.Errorf("missing resolved total value: %q", results[0].Actual)
	}
	if !strings.Contains(results[0].Actual, src) {
		t.Errorf("failure message missing literal source: %q", results[0].Actual)
	}
}

func TestCelAssertion_SensitiveValueRedactedInFailureMessage(t *testing.T) {
	ctx := testCELContext(t)
	ctx.Vars = map[string]any{"api_key": "secret-token-xyz"}
	ctx.SensitiveNames = map[string]struct{}{"api_key": {}}
	ctx.SensitiveValues = []string{"secret-token-xyz"}
	ctx.Response = &apicel.Response{Status: 200, Body: map[string]any{"received": "secret-token-xyz"}}
	src := `vars.api_key == "other-value"` // forces fail, references vars.api_key
	results := CheckCEL([]CELInput{{Index: 0, Source: src}}, ctx)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("want fail")
	}
	if strings.Contains(results[0].Actual, "secret-token-xyz") {
		t.Errorf("failure message leaked secret: %q", results[0].Actual)
	}
	if !strings.Contains(results[0].Actual, "[REDACTED]") {
		t.Errorf("failure message missing redaction marker: %q", results[0].Actual)
	}
}

func TestCelAssertion_TypeErrorBecomesFailedAssertion(t *testing.T) {
	ctx := testCELContext(t)
	// "1 + 1" compiles to int, not bool — type-check rejects it.
	results := CheckCEL([]CELInput{{Index: 0, Source: "1 + 1"}}, ctx)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("want fail (type error)")
	}
	// The human label must stay attributed to assertions[0].cel
	if results[0].Label() != "assertions[0].cel" {
		t.Errorf("Label() = %q, want %q", results[0].Label(), "assertions[0].cel")
	}
	// Actual must contain the CEL type-error framing ("expected" is present
	// in the cel-go "got int, expected bool" message).
	if !strings.Contains(results[0].Actual, "expected") {
		t.Errorf("Actual = %q, want type-error framing containing \"expected\"", results[0].Actual)
	}
}

// TestCheckCEL_NilInputReturnsNil verifies that CheckCEL returns nil (not an
// empty slice) when called with no inputs.
func TestCheckCEL_NilInputReturnsNil(t *testing.T) {
	ctx := testCELContext(t)
	results := CheckCEL(nil, ctx)
	if results != nil {
		t.Errorf("want nil for empty input, got %v", results)
	}
}

// TestCelAssertion_MutualExclusionWithOperatorAssertion verifies that
// CheckCEL correctly evaluates a single CEL assertion that would fail,
// confirming the assertion package returns a non-nil failed Result — the
// mutual-exclusion enforcement itself happens at parse time (see
// internal/parser/collection_test.go:TestParse_CelAssertion_MutualExclusionWithOperator).
func TestCelAssertion_MutualExclusionWithOperatorAssertion(t *testing.T) {
	ctx := testCELContext(t)
	// "false" always fails — confirms the evaluator returns a Result (not nil)
	// for a single non-passing expression, which is the runtime behaviour when
	// a cel: entry makes it past the parser without any operator keys.
	results := CheckCEL([]CELInput{{Index: 0, Source: "false"}}, ctx)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Error("want fail for 'false' expression")
	}
	if results[0].Label() != "assertions[0].cel" {
		t.Errorf("Label() = %q, want %q", results[0].Label(), "assertions[0].cel")
	}
}

func TestCelAssertion_RequestIdentifierRejected(t *testing.T) {
	ctx := testCELContext(t)
	// "request" is not in the activation; compilation will fail with an
	// undeclared reference error.
	results := CheckCEL([]CELInput{{Index: 0, Source: "request.body.id == 1"}}, ctx)
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Passed {
		t.Fatal("want fail")
	}
	if !strings.Contains(strings.ToLower(results[0].Actual), "request") {
		t.Errorf("actual = %q, want mention of undeclared request", results[0].Actual)
	}
}

func TestCheckCEL_MultipleInputs(t *testing.T) {
	ctx := testCELContext(t)
	ctx.Response = &apicel.Response{Status: 200, Body: map[string]any{"x": int64(1)}}
	results := CheckCEL([]CELInput{
		{Index: 0, Source: "response.status == 200"},
		{Index: 1, Source: "response.status == 404"},
	}, ctx)
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].Passed {
		t.Errorf("results[0] want pass, got fail: %q", results[0].Actual)
	}
	if results[1].Passed {
		t.Errorf("results[1] want fail, got pass")
	}
	// Labels should reflect index
	if results[0].Label() != "assertions[0].cel" {
		t.Errorf("results[0].Label() = %q, want %q", results[0].Label(), "assertions[0].cel")
	}
	if results[1].Label() != "assertions[1].cel" {
		t.Errorf("results[1].Label() = %q, want %q", results[1].Label(), "assertions[1].cel")
	}
}
