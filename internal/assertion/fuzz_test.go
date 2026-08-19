package assertion

import (
	"encoding/json"
	"errors"
	"testing"

	"github.com/weiqigod/curlew/internal/fuzzseed"
	"github.com/weiqigod/curlew/internal/jsonpath"
)

// FuzzJSONPath fuzzes two surfaces at once: the JSONPath evaluator on its
// own, and the whole body-assertion surface (every operator evalBodyAssertion
// implements) against a fuzzed path, value and response body.
//
// See management/plans/M27-002-plan.md design decision D3: the operator
// matrix is read out of evalBodyAssertion's own switch with go/ast
// (operatorsFromSwitch, shared with the documentation-parity tests) and
// iterated inside the fuzz function rather than fuzzed as an argument --
// making the operator a fuzz argument would waste the mutation budget
// rediscovering a fixed set of case labels.
func FuzzJSONPath(f *testing.F) {
	root, err := fuzzseed.Root(".")
	if err != nil {
		f.Fatalf("locating repo root: %v", err)
	}
	paths, err := fuzzseed.JSONPaths(root)
	if err != nil {
		f.Fatalf("loading JSONPath seeds: %v", err)
	}
	bodies, err := fuzzseed.JSONBodies(root)
	if err != nil {
		f.Fatalf("loading JSON body seeds: %v", err)
	}
	// Cross the two sets, capped so the seed corpus stays a couple of hundred
	// entries rather than |paths| x |bodies|.
	for i, p := range paths {
		f.Add(p, p, bodies[i%len(bodies)].Data)
	}

	ops := operatorsFromSwitch(f, "evalBodyAssertion")

	f.Fuzz(func(t *testing.T, path, value string, body []byte) {
		// The path evaluator on its own, against whatever the body decodes to
		// -- doc stays nil when body is not valid JSON, which is itself a
		// legitimate input to Evaluate.
		var doc any
		_ = json.Unmarshal(body, &doc)
		if _, err := jsonpath.Evaluate(path, doc); err != nil {
			if !errors.Is(err, jsonpath.ErrNotFound) && !errors.Is(err, jsonpath.ErrInvalidPath) {
				t.Fatalf("Evaluate(%q) error matches neither sentinel: %v", path, err)
			}
		}

		// The whole assertion surface, every operator, including the
		// regexp.Compile that "matches" performs on a fuzzed value.
		for _, op := range ops {
			in := []BodyInput{{Path: path, Operator: op, Value: value}}
			got := CheckBody(in, body)
			if len(got) != len(in) {
				t.Fatalf("CheckBody(op=%q) returned %d results for %d assertions", op, len(got), len(in))
			}
			if got[0].Type != TypeBody {
				t.Fatalf("CheckBody(op=%q) result type = %q, want %q", op, got[0].Type, TypeBody)
			}
		}
	})
}
