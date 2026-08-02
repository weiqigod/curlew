package cel

import (
	"errors"
	"fmt"

	celgo "github.com/google/cel-go/cel"
	"github.com/google/cel-go/ext"
)

// ErrCelParse marks errors produced during expression parsing or by the
// deterministic-function check (rejection of now/zero-arg timestamp).
var ErrCelParse = errors.New("CEL parse error")

// ErrCelType marks errors produced when the expression's result type does
// not match the type requested by the caller via Evaluator.Compile.
var ErrCelType = errors.New("CEL type error")

// CelError is the structured carrier for ErrCelParse and ErrCelType.
// Callers may match the sentinel with errors.Is and read the structured
// fields with errors.As.
type CelError struct {
	// Sentinel is ErrCelParse or ErrCelType.
	Sentinel error
	// FieldPath is filled in by callers (e.g. "requests[3].assertions[1].cel").
	FieldPath string
	// Source is the expression source, truncated to 200 runes + ellipsis marker.
	Source string
	// Actual is the actual result type (e.g. "int"); set for ErrCelType errors.
	Actual string
	// Expected is the expected result type (e.g. "bool"); set for ErrCelType errors.
	Expected string
	// Inner is the original cel-go error message.
	Inner string
}

// Error returns a one-line description of the error.
func (e *CelError) Error() string {
	if errors.Is(e.Sentinel, ErrCelType) {
		return fmt.Sprintf("CEL type error: got %s, expected %s (source: %s)", e.Actual, e.Expected, e.Source)
	}
	return fmt.Sprintf("CEL parse error: %s (source: %s)", e.Inner, e.Source)
}

// Unwrap returns the sentinel error for errors.Is traversal.
func (e *CelError) Unwrap() error { return e.Sentinel }

// Response is the Go-side type registered with the CEL environment.
// Body is bound as CEL dyn so JSON-decoded payloads flow through the type system.
type Response struct {
	Status  int               `json:"status"`
	Headers map[string]string `json:"headers"`
	Body    any               `json:"body"`
}

// StandardActivation is the binding shape exposed to every CEL expression.
type StandardActivation struct {
	Response *Response
	Previous *Response
	Vars     map[string]any
	Env      map[string]string
}

// SensitiveObserver is invoked once per referenced sensitive variable name
// per Eval call, before Eval returns. name is the vars key, value is its
// stringified resolved value.
type SensitiveObserver func(name, value string)

// EvalOptions carry per-evaluation hooks.
type EvalOptions struct {
	// SensitiveNames is the set of vars keys considered sensitive.
	SensitiveNames map[string]struct{}
	// SensitiveObserver is invoked for each referenced sensitive name.
	SensitiveObserver SensitiveObserver
}

// Program is a compiled CEL expression bound to a specific result type.
type Program interface {
	// Eval evaluates the compiled program against the given activation and
	// options. Returns the native Go value of the result (bool, int64, string,
	// etc.) or an error wrapping ErrCelParse for runtime failures.
	Eval(activation StandardActivation, opts EvalOptions) (any, error)
}

// Evaluator compiles CEL expressions against the standard activation schema.
type Evaluator interface {
	// Compile parses, runs the deterministic-function check, type-checks, and
	// returns a Program ready for evaluation. If expectType is non-nil, the
	// expression's result type must be assignable to it; otherwise ErrCelType
	// is returned.
	Compile(src string, expectType *celgo.Type) (Program, error)
}

// evaluator is the concrete implementation of Evaluator.
type evaluator struct {
	env *celgo.Env
}

// program is the concrete implementation of Program.
type program struct {
	src      string
	prog     celgo.Program
	varsRefs []string // sorted unique vars.<name> references in the expression
}

// NewEvaluator returns an Evaluator backed by cel-go with the deterministic
// function set described in the package doc.
func NewEvaluator() (Evaluator, error) {
	// response and previous are bound as map(string, dyn) so that:
	//   - status is int
	//   - headers is map(string, string)
	//   - body is dyn (any JSON-decoded value)
	// This avoids the native-types limitation where interface{}/any fields are
	// not mapped to the CEL dyn type.
	responseType := celgo.MapType(celgo.StringType, celgo.DynType)
	env, err := celgo.NewEnv(
		celgo.StdLib(),
		celgo.Variable("response", responseType),
		celgo.Variable("previous", responseType),
		celgo.Variable("vars", celgo.MapType(celgo.StringType, celgo.DynType)),
		celgo.Variable("env", celgo.MapType(celgo.StringType, celgo.StringType)),
		// Enable string extensions (upperAscii, etc.).
		ext.Strings(),
	)
	if err != nil {
		return nil, fmt.Errorf("cel: build environment: %w", err)
	}
	return &evaluator{env: env}, nil
}

// Compile implements Evaluator.
func (e *evaluator) Compile(src string, expectType *celgo.Type) (Program, error) {
	// Parse phase.
	ast, issues := e.env.Parse(src)
	if issues != nil && issues.Err() != nil {
		return nil, newParseError(src, issues.Err())
	}

	// Deterministic-function check (rejects now() and zero-arg timestamp()).
	if err := rejectTimeOfDay(ast); err != nil {
		return nil, newParseError(src, err)
	}

	// Collect vars.<name> references for the sensitive observer.
	varsRefs := collectVarsRefs(ast)

	// Type-check phase.
	checked, issues := e.env.Check(ast)
	if issues != nil && issues.Err() != nil {
		return nil, newParseError(src, issues.Err())
	}

	// Result type assertion.
	if expectType != nil && !expectType.IsAssignableType(checked.OutputType()) {
		return nil, newTypeError(
			src,
			checked.OutputType().DeclaredTypeName(),
			expectType.DeclaredTypeName(),
			fmt.Sprintf("expression returns %s, expected %s", checked.OutputType().DeclaredTypeName(), expectType.DeclaredTypeName()),
		)
	}

	// Build the executable program.
	prog, err := e.env.Program(checked)
	if err != nil {
		return nil, newParseError(src, err)
	}

	return &program{src: src, prog: prog, varsRefs: varsRefs}, nil
}

// Eval implements Program.
func (p *program) Eval(activation StandardActivation, opts EvalOptions) (any, error) {
	bindings := activationMap(activation)
	out, _, err := p.prog.Eval(bindings)
	if err != nil {
		return nil, newParseError(p.src, err)
	}

	// Invoke the sensitive observer for each referenced vars key that is in
	// the sensitive set. The observer fires exactly once per name per Eval call
	// (varsRefs is deduplicated at compile time).
	if opts.SensitiveObserver != nil && len(opts.SensitiveNames) > 0 {
		for _, name := range p.varsRefs {
			if _, sensitive := opts.SensitiveNames[name]; !sensitive {
				continue
			}
			var val any
			if activation.Vars != nil {
				val = activation.Vars[name]
			}
			opts.SensitiveObserver(name, fmt.Sprintf("%v", val))
		}
	}

	return out.Value(), nil
}

// activationMap converts a StandardActivation into the map[string]any that
// cel-go's Activation constructor expects. Response and Previous are converted
// to map[string]any so that their body field (typed as any) is accessible via
// the CEL dyn type.
func activationMap(a StandardActivation) map[string]any {
	m := map[string]any{
		"response": responseToMap(a.Response),
		"previous": responseToMap(a.Previous),
	}

	if a.Vars != nil {
		m["vars"] = a.Vars
	} else {
		m["vars"] = map[string]any{}
	}
	if a.Env != nil {
		m["env"] = a.Env
	} else {
		m["env"] = map[string]string{}
	}

	return m
}

// responseToMap converts a *Response to a map[string]any for CEL activation.
// A nil response maps to an empty map so that expressions referencing response
// fields produce a CEL missing-key error rather than a nil dereference.
func responseToMap(r *Response) map[string]any {
	if r == nil {
		return map[string]any{}
	}
	return map[string]any{
		"status":  int64(r.Status),
		"headers": r.Headers,
		"body":    r.Body,
	}
}
