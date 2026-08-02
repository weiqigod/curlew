package openapi

import (
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
)

func noopWarn(_ string) {}

func newWalker() *schemaWalker {
	return &schemaWalker{warn: noopWarn, warnedRefs: map[string]bool{}}
}

func strRef() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: openapi3.NewStringSchema()}
}

func intRef() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: openapi3.NewIntegerSchema()}
}

func numRef() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: openapi3.NewFloat64Schema()}
}

func boolRef() *openapi3.SchemaRef {
	return &openapi3.SchemaRef{Value: openapi3.NewBoolSchema()}
}

func objRef(props map[string]*openapi3.SchemaRef) *openapi3.SchemaRef {
	s := openapi3.NewObjectSchema()
	s.Properties = openapi3.Schemas{}
	for k, v := range props {
		s.Properties[k] = v
	}
	return &openapi3.SchemaRef{Value: s}
}

func arrRef(items *openapi3.SchemaRef) *openapi3.SchemaRef {
	s := openapi3.NewArraySchema()
	s.Items = items
	return &openapi3.SchemaRef{Value: s}
}

func strRefWithFormat(format string) *openapi3.SchemaRef {
	s := openapi3.NewStringSchema()
	s.Format = format
	return &openapi3.SchemaRef{Value: s}
}

func TestWalkSchema(t *testing.T) {
	tests := []struct {
		name string
		ref  *openapi3.SchemaRef
		want any
	}{
		{"nil_ref", nil, "string"},
		{"nil_value", &openapi3.SchemaRef{}, "string"},
		{"type_string", strRef(), "string"},
		{"type_integer", intRef(), 0},
		{"type_number", numRef(), 0},
		{"type_boolean", boolRef(), false},
		{"format_date_time", strRefWithFormat("date-time"), "2006-01-02T15:04:05Z"},
		{"format_date", strRefWithFormat("date"), "2006-01-02"},
		{"format_uuid", strRefWithFormat("uuid"), "00000000-0000-0000-0000-000000000000"},
		{"format_email", strRefWithFormat("email"), "user@example.com"},
		{"format_uri", strRefWithFormat("uri"), "https://example.com"},
		{
			"object_one_field",
			objRef(map[string]*openapi3.SchemaRef{"name": strRef()}),
			map[string]any{"name": "string"},
		},
		{
			"object_sorted_keys",
			objRef(map[string]*openapi3.SchemaRef{"z": strRef(), "a": strRef()}),
			map[string]any{"a": "string", "z": "string"},
		},
		{
			"array_of_string",
			arrRef(strRef()),
			[]any{"string"},
		},
		{
			"nested_object",
			objRef(map[string]*openapi3.SchemaRef{
				"pet": objRef(map[string]*openapi3.SchemaRef{"name": strRef()}),
			}),
			map[string]any{"pet": map[string]any{"name": "string"}},
		},
		{
			"example_wins",
			func() *openapi3.SchemaRef {
				s := openapi3.NewStringSchema()
				s.Example = "Fluffy"
				return &openapi3.SchemaRef{Value: s}
			}(),
			"Fluffy",
		},
		{
			"enum_wins_over_type",
			func() *openapi3.SchemaRef {
				s := openapi3.NewStringSchema()
				s.Enum = []any{"a", "b"}
				return &openapi3.SchemaRef{Value: s}
			}(),
			"a",
		},
		{
			"allof_merges",
			func() *openapi3.SchemaRef {
				s := &openapi3.Schema{
					AllOf: openapi3.SchemaRefs{
						objRef(map[string]*openapi3.SchemaRef{"a": strRef()}),
						objRef(map[string]*openapi3.SchemaRef{"b": intRef()}),
					},
				}
				return &openapi3.SchemaRef{Value: s}
			}(),
			map[string]any{"a": "string", "b": 0},
		},
		{
			"oneof_picks_first",
			func() *openapi3.SchemaRef {
				s := &openapi3.Schema{
					OneOf: openapi3.SchemaRefs{strRef(), intRef()},
				}
				return &openapi3.SchemaRef{Value: s}
			}(),
			"string",
		},
		{
			"anyof_picks_first",
			func() *openapi3.SchemaRef {
				s := &openapi3.Schema{
					AnyOf: openapi3.SchemaRefs{intRef(), strRef()},
				}
				return &openapi3.SchemaRef{Value: s}
			}(),
			0,
		},
		{
			"empty_object",
			objRef(map[string]*openapi3.SchemaRef{}),
			map[string]any{},
		},
		{
			"array_no_items",
			func() *openapi3.SchemaRef {
				s := openapi3.NewArraySchema()
				return &openapi3.SchemaRef{Value: s}
			}(),
			[]any{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			w := newWalker()
			got := w.placeholder(tt.ref, nil)
			if !deepEqual(got, tt.want) {
				t.Errorf("placeholder() = %#v (%T), want %#v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}

func TestWalkSchema_CycleWarn(t *testing.T) {
	t.Run("cycle_terminates", func(t *testing.T) {
		// Build: NodeA.child -> NodeA (cycle via named ref)
		nodeA := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
		}
		// Self-referential ref
		selfRef := &openapi3.SchemaRef{
			Ref:   "#/components/schemas/NodeA",
			Value: nodeA,
		}
		nodeA.Properties = openapi3.Schemas{
			"name":  strRef(),
			"child": selfRef,
		}
		topRef := &openapi3.SchemaRef{
			Ref:   "#/components/schemas/NodeA",
			Value: nodeA,
		}

		var warnings []string
		w := &schemaWalker{
			warn:       func(msg string) { warnings = append(warnings, msg) },
			warnedRefs: map[string]bool{},
		}
		got := w.placeholder(topRef, nil)
		if got == nil {
			t.Errorf("expected non-nil result, got nil")
		}
		if len(warnings) == 0 {
			t.Errorf("expected cycle warning, got none")
		}
		// warn-once
		if len(warnings) > 1 {
			t.Errorf("expected exactly 1 cycle warning, got %d: %v", len(warnings), warnings)
		}
	})

	t.Run("siblings_no_cycle", func(t *testing.T) {
		// Two sibling properties both ref the same non-recursive schema.
		// Neither should trigger a cycle warning.
		leaf := &openapi3.Schema{Type: &openapi3.Types{"string"}}
		leafRef := &openapi3.SchemaRef{
			Ref:   "#/components/schemas/Leaf",
			Value: leaf,
		}
		parent := &openapi3.Schema{
			Type: &openapi3.Types{"object"},
			Properties: openapi3.Schemas{
				"a": leafRef,
				"b": leafRef,
			},
		}
		topRef := &openapi3.SchemaRef{Value: parent}

		var warnings []string
		w := &schemaWalker{
			warn:       func(msg string) { warnings = append(warnings, msg) },
			warnedRefs: map[string]bool{},
		}
		got := w.placeholder(topRef, nil)
		if len(warnings) > 0 {
			t.Errorf("expected no cycle warnings for sibling refs, got: %v", warnings)
		}
		obj, ok := got.(map[string]any)
		if !ok {
			t.Fatalf("expected map, got %T", got)
		}
		if obj["a"] != "string" || obj["b"] != "string" {
			t.Errorf("expected both siblings to resolve to string, got %v", obj)
		}
	})
}

// deepEqual is a simple recursive equality check for test output comparison.
func deepEqual(a, b any) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch av := a.(type) {
	case map[string]any:
		bm, ok := b.(map[string]any)
		if !ok || len(av) != len(bm) {
			return false
		}
		for k, v := range av {
			if !deepEqual(v, bm[k]) {
				return false
			}
		}
		return true
	case []any:
		bs, ok := b.([]any)
		if !ok || len(av) != len(bs) {
			return false
		}
		for i := range av {
			if !deepEqual(av[i], bs[i]) {
				return false
			}
		}
		return true
	default:
		return a == b
	}
}
