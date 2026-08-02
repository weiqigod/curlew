package cel_test

import (
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	celgo "github.com/google/cel-go/cel"

	apicel "github.com/weiqigod/curlew/internal/cel"
)

// ─────────────────────────────────────────────────────────────────────────────
// Helpers
// ─────────────────────────────────────────────────────────────────────────────

func mustNewEvaluator(t *testing.T) apicel.Evaluator {
	t.Helper()
	ev, err := apicel.NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	return ev
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 2 – basic happy path
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_ParseValidExpression(t *testing.T) {
	ev := mustNewEvaluator(t)
	prog, err := ev.Compile(`response.body.x == 1`, celgo.BoolType)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Eval(apicel.StandardActivation{
		Response: &apicel.Response{Body: map[string]any{"x": int64(1)}},
	}, apicel.EvalOptions{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != true {
		t.Fatalf("want true, got %v", got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 3 – stdlib string / list ops, standard activation bindings
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_StdLibStringOpsAvailable(t *testing.T) {
	ev := mustNewEvaluator(t)
	prog, err := ev.Compile(`"hello".upperAscii() == "HELLO"`, celgo.BoolType)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Eval(apicel.StandardActivation{}, apicel.EvalOptions{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != true {
		t.Fatalf("want true, got %v", got)
	}
}

func TestEvaluator_StdLibListOpsAvailable(t *testing.T) {
	ev := mustNewEvaluator(t)
	prog, err := ev.Compile(`[1, 2, 3].size() == 3`, celgo.BoolType)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Eval(apicel.StandardActivation{}, apicel.EvalOptions{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != true {
		t.Fatalf("want true, got %v", got)
	}
}

func TestEvaluator_StandardActivationBindsResponsePreviousVarsEnv(t *testing.T) {
	tests := []struct {
		name string
		src  string
		act  apicel.StandardActivation
		want any
	}{
		{
			name: "response.status",
			src:  `response.status == 200`,
			act:  apicel.StandardActivation{Response: &apicel.Response{Status: 200}},
			want: true,
		},
		{
			name: "previous.body access",
			src:  `previous.body.id == "p1"`,
			act: apicel.StandardActivation{
				Previous: &apicel.Response{Body: map[string]any{"id": "p1"}},
			},
			want: true,
		},
		{
			name: "vars lookup",
			src:  `vars.api_key == "secret"`,
			act: apicel.StandardActivation{
				Vars: map[string]any{"api_key": "secret"},
			},
			want: true,
		},
		{
			name: "env lookup",
			src:  `env.STAGE == "prod"`,
			act: apicel.StandardActivation{
				Env: map[string]string{"STAGE": "prod"},
			},
			want: true,
		},
		{
			name: "response.headers access",
			src:  `response.headers.Authorization == "Bearer x"`,
			act: apicel.StandardActivation{
				Response: &apicel.Response{Headers: map[string]string{"Authorization": "Bearer x"}},
			},
			want: true,
		},
	}

	ev := mustNewEvaluator(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			prog, err := ev.Compile(tc.src, celgo.BoolType)
			if err != nil {
				t.Fatalf("Compile: %v", err)
			}
			got, err := prog.Eval(tc.act, apicel.EvalOptions{})
			if err != nil {
				t.Fatalf("Eval: %v", err)
			}
			if got != tc.want {
				t.Fatalf("want %v, got %v", tc.want, got)
			}
		})
	}
}

func TestEvaluator_ResponseBodyIsDyn(t *testing.T) {
	ev := mustNewEvaluator(t)
	prog, err := ev.Compile(`response.body.items[0].price`, celgo.DynType)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	body := map[string]any{
		"items": []any{
			map[string]any{"price": int64(99)},
		},
	}
	got, err := prog.Eval(apicel.StandardActivation{
		Response: &apicel.Response{Body: body},
	}, apicel.EvalOptions{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != int64(99) {
		t.Fatalf("want int64(99), got %v (%T)", got, got)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 4 – time-of-day rejection
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_RejectsTimeOfDayNow(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("now()", celgo.TimestampType)
	if !errors.Is(err, apicel.ErrCelParse) {
		t.Fatalf("want ErrCelParse, got %v", err)
	}
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CelError, got %T: %v", err, err)
	}
	if !strings.Contains(strings.ToLower(ce.Error()), "now") {
		t.Fatalf("want error mentioning 'now', got %q", ce.Error())
	}
}

func TestEvaluator_RejectsZeroArgTimestamp(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("timestamp()", celgo.TimestampType)
	if !errors.Is(err, apicel.ErrCelParse) {
		t.Fatalf("want ErrCelParse, got %v", err)
	}
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CelError, got %T: %v", err, err)
	}
	if !strings.Contains(strings.ToLower(ce.Error()), "timestamp") {
		t.Fatalf("want error mentioning 'timestamp', got %q", ce.Error())
	}
	// Single-argument form must remain available.
	_, err = ev.Compile(`timestamp("2024-01-01T00:00:00Z")`, celgo.TimestampType)
	if err != nil {
		t.Fatalf("timestamp(string) must remain available: %v", err)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 5 – type check errors
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_TypeCheckRejectsNonBool(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("1 + 1", celgo.BoolType)
	if !errors.Is(err, apicel.ErrCelType) {
		t.Fatalf("want ErrCelType, got %v", err)
	}
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CelError, got %T", err)
	}
	if ce.Actual != "int" || ce.Expected != "bool" {
		t.Fatalf("want actual=int expected=bool, got actual=%q expected=%q", ce.Actual, ce.Expected)
	}
}

func TestEvaluator_ParseErrorReturnsErrCelParse(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("response.body.[", celgo.BoolType)
	if !errors.Is(err, apicel.ErrCelParse) {
		t.Fatalf("want ErrCelParse, got %v", err)
	}
}

func TestEvaluator_TypeErrorReturnsErrCelType(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("1 + 1", celgo.BoolType)
	if !errors.Is(err, apicel.ErrCelType) {
		t.Fatalf("want errors.Is(err, ErrCelType), got %v", err)
	}
	// Also confirm errors.As works.
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want errors.As(err, *CelError), got %T", err)
	}
	// errors.Is match through Unwrap.
	if !errors.Is(err, apicel.ErrCelType) {
		t.Fatalf("errors.Is must work through Unwrap")
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 6 – 200-char source truncation
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_TruncatesSourceTo200CharsInErrorMessage(t *testing.T) {
	long := strings.Repeat("a", 250) + "[" // forces parse error
	ev := mustNewEvaluator(t)
	_, err := ev.Compile(long, celgo.BoolType)
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CelError, got %T: %v", err, err)
	}
	runeCount := utf8.RuneCountInString(ce.Source)
	// 200 chars + 1 ellipsis rune = 201 total runes
	if runeCount != 201 {
		t.Fatalf("want 201 runes (200 chars + ellipsis), got %d; source=%q", runeCount, ce.Source)
	}
	if !strings.HasSuffix(ce.Source, "…") {
		t.Fatalf("want trailing ellipsis '…', got %q", ce.Source)
	}
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 7 – sensitive observer
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_SensitiveObserverFiresOnVarReference(t *testing.T) {
	type call struct{ name, value string }

	t.Run("fires once for referenced sensitive var", func(t *testing.T) {
		ev := mustNewEvaluator(t)
		prog, err := ev.Compile(`vars.api_key == "secret"`, celgo.BoolType)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		var calls []call
		opts := apicel.EvalOptions{
			SensitiveNames: map[string]struct{}{"api_key": {}},
			SensitiveObserver: func(n, v string) {
				calls = append(calls, call{n, v})
			},
		}
		_, err = prog.Eval(apicel.StandardActivation{
			Vars: map[string]any{"api_key": "secret"},
		}, opts)
		if err != nil {
			t.Fatal(err)
		}
		if len(calls) != 1 || calls[0].name != "api_key" || calls[0].value != "secret" {
			t.Fatalf("want one call (api_key, secret), got %+v", calls)
		}
	})

	t.Run("not called when name is not in sensitive set", func(t *testing.T) {
		ev := mustNewEvaluator(t)
		prog, err := ev.Compile(`vars.api_key == "secret"`, celgo.BoolType)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		var calls []call
		opts := apicel.EvalOptions{
			SensitiveNames:    map[string]struct{}{}, // empty set
			SensitiveObserver: func(n, v string) { calls = append(calls, call{n, v}) },
		}
		_, _ = prog.Eval(apicel.StandardActivation{
			Vars: map[string]any{"api_key": "secret"},
		}, opts)
		if len(calls) != 0 {
			t.Fatalf("want zero calls, got %+v", calls)
		}
	})

	t.Run("fires once even if referenced multiple times", func(t *testing.T) {
		ev := mustNewEvaluator(t)
		// Reference vars.api_key twice in the expression.
		prog, err := ev.Compile(`vars.api_key == "secret" && vars.api_key != ""`, celgo.BoolType)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		var calls []call
		opts := apicel.EvalOptions{
			SensitiveNames:    map[string]struct{}{"api_key": {}},
			SensitiveObserver: func(n, v string) { calls = append(calls, call{n, v}) },
		}
		_, _ = prog.Eval(apicel.StandardActivation{
			Vars: map[string]any{"api_key": "secret"},
		}, opts)
		if len(calls) != 1 {
			t.Fatalf("want exactly 1 call, got %d: %+v", len(calls), calls)
		}
	})

	t.Run("not called when observer is nil", func(t *testing.T) {
		ev := mustNewEvaluator(t)
		prog, err := ev.Compile(`vars.api_key == "secret"`, celgo.BoolType)
		if err != nil {
			t.Fatalf("Compile: %v", err)
		}
		// Must not panic with nil observer.
		opts := apicel.EvalOptions{
			SensitiveNames:    map[string]struct{}{"api_key": {}},
			SensitiveObserver: nil,
		}
		_, err = prog.Eval(apicel.StandardActivation{
			Vars: map[string]any{"api_key": "secret"},
		}, opts)
		if err != nil {
			t.Fatal(err)
		}
	})
}

// ─────────────────────────────────────────────────────────────────────────────
// Step 9 – edge cases for coverage
// ─────────────────────────────────────────────────────────────────────────────

func TestEvaluator_NilResponseAndPreviousAllowed(t *testing.T) {
	ev := mustNewEvaluator(t)
	prog, err := ev.Compile(`vars.x == 1`, celgo.BoolType)
	if err != nil {
		t.Fatalf("Compile: %v", err)
	}
	got, err := prog.Eval(apicel.StandardActivation{
		// Response and Previous are nil
		Vars: map[string]any{"x": int64(1)},
	}, apicel.EvalOptions{})
	if err != nil {
		t.Fatalf("Eval: %v", err)
	}
	if got != true {
		t.Fatalf("want true, got %v", got)
	}
}

func TestEvaluator_EmptyExpressionRejected(t *testing.T) {
	ev := mustNewEvaluator(t)
	_, err := ev.Compile("", celgo.BoolType)
	if !errors.Is(err, apicel.ErrCelParse) {
		t.Fatalf("want ErrCelParse for empty expression, got %v", err)
	}
}

// TestCelError_Unwrap verifies that CelError.Unwrap returns the sentinel and
// that errors.Is traversal works through the chain.
func TestCelError_Unwrap(t *testing.T) {
	ev := mustNewEvaluator(t)
	// ErrCelType path
	_, err := ev.Compile("1 + 1", celgo.BoolType)
	var ce *apicel.CelError
	if !errors.As(err, &ce) {
		t.Fatalf("want *CelError")
	}
	if ce.Unwrap() != apicel.ErrCelType {
		t.Fatalf("want Unwrap() == ErrCelType")
	}
	// ErrCelType branch in Error()
	msg := ce.Error()
	if !strings.Contains(msg, "CEL type error") {
		t.Fatalf("want 'CEL type error' in %q", msg)
	}

	// ErrCelParse path
	_, err2 := ev.Compile("response.body.[", celgo.BoolType)
	var ce2 *apicel.CelError
	if !errors.As(err2, &ce2) {
		t.Fatalf("want *CelError for parse error")
	}
	if ce2.Unwrap() != apicel.ErrCelParse {
		t.Fatalf("want Unwrap() == ErrCelParse")
	}
	msg2 := ce2.Error()
	if !strings.Contains(msg2, "CEL parse error") {
		t.Fatalf("want 'CEL parse error' in %q", msg2)
	}
}
