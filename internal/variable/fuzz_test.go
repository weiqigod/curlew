package variable

import (
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/fuzzseed"
)

// chainVars builds a linear reference chain of n variables: v0 -> v1 -> ... ->
// v(n-1), where v(n-1) is a literal. Resolving v0 walks n-1 hops, so
// chainVars(MaxDepth) is the shortest chain guaranteed to exceed MaxDepth.
func chainVars(n int) map[string]string {
	vars := make(map[string]string, n)
	for i := 0; i < n-1; i++ {
		vars[fmt.Sprintf("v%d", i)] = fmt.Sprintf("{{v%d}}", i+1)
	}
	vars[fmt.Sprintf("v%d", n-1)] = "literal"
	return vars
}

// TestScope_self_reference_terminates_with_ErrCircularReference asserts
// termination rather than assuming it: each case runs on its own goroutine
// with a 5s deadline, so a regression that turns cycle detection into
// unbounded recursion or an infinite loop fails with a clear timeout message
// instead of hanging the whole test binary until the package timeout dumps
// every goroutine's stack.
func TestScope_self_reference_terminates_with_ErrCircularReference(t *testing.T) {
	tests := []struct {
		name string
		vars map[string]string
		want error // nil means "resolves without error"
	}{
		{"direct self reference", map[string]string{"a": "{{a}}"}, ErrCircularReference},
		{"mutual two-cycle", map[string]string{"a": "{{b}}", "b": "{{a}}"}, ErrCircularReference},
		{"three-cycle", map[string]string{"a": "{{b}}", "b": "{{c}}", "c": "{{a}}"}, ErrCircularReference},
		{"self reference inside a dyn-fn arg", map[string]string{"a": "{{$base64('{{a}}')}}"}, ErrCircularReference},
		{"chain of 9 resolves", chainVars(9), nil},
		{"chain of 12 exceeds depth", chainVars(12), ErrDepthExceeded},
		// findVarRefs uses varPattern, which cannot match a name followed by
		// "|" -- so this value is not a self-reference at resolve time and
		// must resolve, not cycle. The asymmetry between varPattern and
		// defaultPattern is exactly what this case pins down.
		{"self reference behind a default", map[string]string{"a": "{{a|default:x}}"}, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			done := make(chan error, 1)
			go func() {
				s := NewScope(tt.vars)
				done <- s.Resolve()
			}()

			select {
			case err := <-done:
				if tt.want == nil {
					if err != nil {
						t.Fatalf("Resolve() = %v, want nil", err)
					}
					return
				}
				if !errors.Is(err, tt.want) {
					t.Fatalf("Resolve() = %v, want error matching %v", err, tt.want)
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("did not terminate within 5s")
			}
		})
	}
}

// fuzzVars turns an arbitrary blob into a variable map deterministically:
// split on '\n', then on the first '='. Deterministic and dependency-free, so
// a corpus entry reproduces identically on any machine.
func fuzzVars(blob string) map[string]string {
	vars := make(map[string]string)
	if blob == "" {
		return vars
	}
	for _, line := range strings.Split(blob, "\n") {
		name, value, ok := strings.Cut(line, "=")
		if !ok || name == "" {
			continue
		}
		vars[name] = value
	}
	return vars
}

// FuzzInterpolate fuzzes Scope.Resolve, Interpolate, InterpolateMap and
// InterpolateBody together against a scope carrying a dynamic-function
// registry and a secrets namespace, both attached deliberately: Interpolate
// returns early at "if s.registry == nil" before it ever parses dynamic
// function arguments, so a scope without one leaves the entire {{$fn(...)}}
// path -- including its recursive Interpolate call on each argument --
// unfuzzed.
func FuzzInterpolate(f *testing.F) {
	root, err := fuzzseed.Root(".")
	if err != nil {
		f.Fatalf("locating repo root: %v", err)
	}
	tmpls, err := fuzzseed.Templates(root)
	if err != nil {
		f.Fatalf("loading template seeds: %v", err)
	}
	for _, tmpl := range tmpls {
		f.Add(tmpl, "base_url=http://127.0.0.1\ntoken=abc\nname=curlew")
	}
	// Shapes the fixtures do not contain, added explicitly because they are
	// the recursion and cycle paths.
	f.Add("{{a}}", "a={{a}}")                       // direct self reference
	f.Add("{{a}}", "a={{b}}\nb={{a}}")              // mutual cycle
	f.Add("{{$base64('{{$base64('a')}}')}}", "a=1") // nested dyn-fn args
	f.Add("{{$randomBase64('32')}}", "a=1")         // F2: the digits mutate
	f.Add("{{secrets.API_KEY}}", "a=1")             // secrets namespace
	f.Add("{{a|default:x}}", "")                    // default fallback

	f.Fuzz(func(t *testing.T, tmpl, blob string) {
		seed := int64(1)
		s := NewScope(fuzzVars(blob)).
			WithDynamic(NewRegistry(&seed)). // fixed seed: reproducible
			WithSecrets(map[string]string{"API_KEY": "s3cr3t"})

		if err := s.Resolve(); err != nil {
			// Resolution may legitimately fail, but only in declared ways.
			if !errors.Is(err, ErrCircularReference) &&
				!errors.Is(err, ErrDepthExceeded) &&
				!errors.Is(err, ErrUndefinedVariable) {
				t.Fatalf("Resolve() error matches no declared sentinel: %v", err)
			}
			return
		}
		s.BeginRequest()
		defer s.EndRequest()
		_, _ = s.Interpolate(tmpl)
		_, _ = s.InterpolateMap(map[string]string{"k": tmpl})
		_, _ = s.InterpolateBody(map[string]any{"k": []any{tmpl, 1, true}})
	})
}
