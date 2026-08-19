package cel

import (
	"errors"
	"testing"

	celgo "github.com/google/cel-go/cel"

	"github.com/weiqigod/curlew/internal/fuzzseed"
)

// FuzzCEL fuzzes Evaluator.Compile -- the parse-check-and-type-check path
// exercised by `curlew validate` for every `if:` and `assertions: - cel:`
// expression in a collection. It compiles only; it does not evaluate. See
// management/plans/M27-002-plan.md, design decision D1: the environment
// built by NewEvaluator carries no cel-go CostLimit or interrupt-check
// frequency, so evaluating fuzzer-authored expressions (nested
// comprehensions cost 2^depth) is unbounded work and would produce hang
// reports rather than defects.
//
// The invariant is stronger than "no panic": every error returned by
// Compile must be a *CelError wrapping one of the two declared sentinels,
// so an error escaping this package's own error contract is a finding too.
func FuzzCEL(f *testing.F) {
	root, err := fuzzseed.Root(".")
	if err != nil {
		f.Fatalf("locating repo root: %v", err)
	}
	exprs, err := fuzzseed.CELExpressions(root)
	if err != nil {
		f.Fatalf("loading CEL seeds: %v", err)
	}
	for _, src := range exprs {
		f.Add(src)
	}

	// One evaluator for the whole target: NewEvaluator builds a cel-go
	// environment, which is far more expensive than a single compile.
	ev, err := NewEvaluator()
	if err != nil {
		f.Fatalf("building evaluator: %v", err)
	}

	f.Fuzz(func(t *testing.T, src string) {
		for _, want := range []*celgo.Type{celgo.BoolType, nil} {
			prog, err := ev.Compile(src, want)
			switch {
			case err != nil && prog != nil:
				t.Fatalf("Compile(%q) returned both a program and an error %v", src, err)
			case err == nil && prog == nil:
				t.Fatalf("Compile(%q) returned neither a program nor an error", src)
			case err != nil:
				var ce *CelError
				if !errors.As(err, &ce) {
					t.Fatalf("Compile(%q) error %T is not a *CelError: %v", src, err, err)
				}
				if !errors.Is(err, ErrCelParse) && !errors.Is(err, ErrCelType) {
					t.Fatalf("Compile(%q) error matches neither sentinel: %v", src, err)
				}
			}
		}
		// Same input, the other user-facing reader in this package.
		_ = CollectTopLevelRefs(src, ev)
	})
}
