package assertion

import (
	"fmt"
	"strings"

	celgo "github.com/google/cel-go/cel"
	apicel "github.com/weiqigod/curlew/internal/cel"
)

// CELInput describes one CEL assertion to evaluate.
type CELInput struct {
	// Index is the position in the assertions.cel list (for error attribution).
	Index int
	// Source is the CEL expression string.
	Source string
	// Line is the 1-based source line of this cel: list entry; 0 when unknown.
	Line int
}

// CELContext bundles the activation, evaluator, compile cache, and
// sensitive-value snapshot needed for CEL assertion evaluation. The
// assertion package does not import internal/variable; sensitive values
// are passed as a pre-sorted []string (longest-first) for substring
// replacement in failure messages.
type CELContext struct {
	// Evaluator compiles CEL expressions.
	Evaluator apicel.Evaluator
	// ProgCache is a per-run cache of compiled programs keyed by expression
	// source. Multiple assertions with the same expression share one compile.
	ProgCache map[string]apicel.Program
	// Response is the CEL-typed representation of the HTTP response.
	Response *apicel.Response
	// Previous is the previous request's response (or nil).
	Previous *apicel.Response
	// Vars is the resolved variable map for the current scope.
	Vars map[string]any
	// Env is the OS-environment import map.
	Env map[string]string
	// SensitiveNames is the set of vars keys considered sensitive.
	SensitiveNames map[string]struct{}
	// SensitiveValues is the snapshot of concrete sensitive strings, sorted
	// longest-first. Any occurrence in a failure message is replaced with
	// [REDACTED] before the message is stored in Result.Actual.
	SensitiveValues []string
	// SensitiveObserve, when non-nil, is called once per referenced sensitive
	// variable name/value pair. Used to register newly-observed values with
	// the run's RuntimeSensitiveSet.
	SensitiveObserve func(name, value string)
}

// CheckCEL evaluates each CELInput against the provided context and returns
// one Result per input. On runtime or compile errors the result is recorded
// as failed with error detail in the Actual field. Returns nil when inputs is
// empty.
func CheckCEL(inputs []CELInput, ctx CELContext) []Result {
	if len(inputs) == 0 {
		return nil
	}
	out := make([]Result, 0, len(inputs))
	for _, in := range inputs {
		out = append(out, evalCELAssertion(in, ctx))
	}
	return out
}

// evalCELAssertion evaluates a single CEL assertion and returns the Result.
func evalCELAssertion(in CELInput, ctx CELContext) Result {
	// Built once so every return below keeps the same identity and source
	// pointer; a literal per branch is how those drift apart.
	id := Result{
		Type:       TypeCEL,
		Target:     fmt.Sprintf("assertions[%d]", in.Index),
		SourceLine: in.Line,
	}

	prog, compileErr := compileCELAssertion(ctx, in.Source)
	if compileErr != nil {
		return id.with("compiled CEL bool expression", redactSensitive(compileErr.Error(), ctx.SensitiveValues), false)
	}

	out, evalErr := prog.Eval(apicel.StandardActivation{
		Response: ctx.Response,
		Previous: ctx.Previous,
		Vars:     ctx.Vars,
		Env:      ctx.Env,
	}, apicel.EvalOptions{
		SensitiveNames:    ctx.SensitiveNames,
		SensitiveObserver: ctx.SensitiveObserve,
	})
	if evalErr != nil {
		return id.with(in.Source, redactSensitive(evalErr.Error(), ctx.SensitiveValues), false)
	}

	b, ok := out.(bool)
	if !ok {
		// Should not happen if Compile correctly enforced bool type,
		// but handle defensively.
		return id.with("boolean result", fmt.Sprintf("non-bool %T", out), false)
	}

	if b {
		return id.with(in.Source, "true", true)
	}

	// Assertion failed: build a failure message that includes the literal
	// expression source and the resolved value of each top-level named ref.
	msg := buildCELFailureMessage(in.Source, ctx)
	return id.with(in.Source, redactSensitive(msg, ctx.SensitiveValues), false)
}

// compileCELAssertion retrieves a compiled program from the cache or compiles
// and caches a new one. Expects the expression to produce a bool.
func compileCELAssertion(ctx CELContext, src string) (apicel.Program, error) {
	if ctx.ProgCache != nil {
		if p, ok := ctx.ProgCache[src]; ok {
			return p, nil
		}
	}
	p, err := ctx.Evaluator.Compile(src, celgo.BoolType)
	if err != nil {
		return nil, err
	}
	if ctx.ProgCache != nil {
		ctx.ProgCache[src] = p
	}
	return p, nil
}

// buildCELFailureMessage constructs the human-readable failure message for a
// failed CEL assertion. It includes the literal expression source and, for
// each top-level named reference (e.g. response.body.total), the resolved
// value from the activation.
//
// Output shape:
//
//	<source>
//	  response.body.total = 9.5
//	  response.body.items.map(i, i.price).sum() = 10.5
func buildCELFailureMessage(src string, ctx CELContext) string {
	refs := apicel.CollectTopLevelRefs(src, ctx.Evaluator)
	if len(refs) == 0 {
		return src
	}

	var b strings.Builder
	b.WriteString(src)

	for _, ref := range refs {
		val, err := evaluateSubexpression(ref, ctx)
		var rendered string
		if err != nil {
			rendered = fmt.Sprintf("<eval error: %s>", err)
		} else {
			rendered = fmt.Sprintf("%v", val)
		}
		fmt.Fprintf(&b, "\n  %s = %s", ref, rendered)
	}
	return b.String()
}

// evaluateSubexpression evaluates a sub-expression string (e.g.
// "response.body.total") against the same activation as the parent assertion.
// Returns the native Go value or an error.
func evaluateSubexpression(sub string, ctx CELContext) (any, error) {
	// Use DynType so any expression type is accepted.
	prog, err := ctx.Evaluator.Compile(sub, nil)
	if err != nil {
		return nil, err
	}
	val, evalErr := prog.Eval(apicel.StandardActivation{
		Response: ctx.Response,
		Previous: ctx.Previous,
		Vars:     ctx.Vars,
		Env:      ctx.Env,
	}, apicel.EvalOptions{
		// Do not fire the sensitive observer for sub-evaluations.
	})
	if evalErr != nil {
		return nil, evalErr
	}
	return val, nil
}

// redactSensitive replaces every occurrence of a sensitive value in s with
// the standard redaction marker [REDACTED]. Values must be sorted longest-first
// to ensure that longer secrets take precedence over shorter substrings.
func redactSensitive(s string, sensitiveValues []string) string {
	for _, v := range sensitiveValues {
		if v == "" {
			continue
		}
		s = strings.ReplaceAll(s, v, "[REDACTED]")
	}
	return s
}
