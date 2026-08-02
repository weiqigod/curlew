// Package cel wraps github.com/google/cel-go with a deterministic, project-
// specific expression evaluator. It exposes the [Evaluator] interface, the
// [StandardActivation] binding shape (response, previous, vars, env), a Go-
// side [Response] type whose body is bound as the CEL dyn type, and the
// sentinel errors [ErrCelParse] and [ErrCelType].
//
// # Function set
//
// The evaluator uses cel-go's standard library MINUS the time-of-day
// functions: bare calls to now() and zero-argument timestamp() are rejected
// at compile time with ErrCelParse. The single-argument form timestamp(string)
// remains available because it is deterministic. String, list, map, math,
// and comparison operators are available unchanged.
//
// # Sensitive observer
//
// Callers may pass a SensitiveObserver to Program.Eval to be notified when
// the evaluated expression references a vars.<name> whose name is in a
// caller-supplied sensitive set. The observer fires exactly once per
// referenced sensitive name per Eval call, before Eval returns.
//
// # Errors
//
// Compilation failures return ErrCelParse-wrapped errors carrying the
// truncated source (first 200 runes plus an ellipsis marker for longer
// input). Type-check failures return ErrCelType-wrapped errors naming the
// actual and expected types. Use errors.Is to match the sentinel and
// errors.As against *CelError to read the structured fields.
package cel
