package jsonpath

import (
	"errors"
	"reflect"
	"testing"
)

func TestEvaluate(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		doc     any
		want    any
		wantErr error
	}{
		{
			"root object returns document",
			"$",
			map[string]any{"a": float64(1)},
			map[string]any{"a": float64(1)},
			nil,
		},
		{
			"simple field access",
			"$.name",
			map[string]any{"name": "Alice"},
			"Alice",
			nil,
		},
		{
			"nested field access",
			"$.data.id",
			map[string]any{"data": map[string]any{"id": float64(1)}},
			float64(1),
			nil,
		},
		{
			"deeply nested",
			"$.a.b.c.d",
			map[string]any{
				"a": map[string]any{
					"b": map[string]any{
						"c": map[string]any{
							"d": "value",
						},
					},
				},
			},
			"value",
			nil,
		},
		{
			"array index zero",
			"$.items[0]",
			map[string]any{"items": []any{"a", "b"}},
			"a",
			nil,
		},
		{
			"array index non-zero",
			"$.items[1]",
			map[string]any{"items": []any{"a", "b", "c"}},
			"b",
			nil,
		},
		{
			"array index nested field",
			"$.items[0].name",
			map[string]any{"items": []any{
				map[string]any{"name": "first"},
				map[string]any{"name": "second"},
			}},
			"first",
			nil,
		},
		{
			"null value is found",
			"$.value",
			map[string]any{"value": nil},
			nil,
			nil,
		},
		{
			"boolean value",
			"$.active",
			map[string]any{"active": true},
			true,
			nil,
		},
		{
			"missing field returns ErrNotFound",
			"$.missing",
			map[string]any{"name": "x"},
			nil,
			ErrNotFound,
		},
		{
			"missing nested field returns ErrNotFound",
			"$.a.b.c",
			map[string]any{"a": map[string]any{"x": float64(1)}},
			nil,
			ErrNotFound,
		},
		{
			"array index out of bounds returns ErrNotFound",
			"$.items[5]",
			map[string]any{"items": []any{"a", "b", "c"}},
			nil,
			ErrNotFound,
		},
		{
			"negative array index returns ErrInvalidPath",
			"$.items[-1]",
			map[string]any{"items": []any{"a", "b"}},
			nil,
			ErrInvalidPath,
		},
		{
			"field on non-object returns ErrNotFound",
			"$.name.x",
			map[string]any{"name": "str"},
			nil,
			ErrNotFound,
		},
		{
			"index on non-array returns ErrNotFound",
			"$.name[0]",
			map[string]any{"name": "str"},
			nil,
			ErrNotFound,
		},
		{
			"invalid path - no dollar",
			"no dollar",
			map[string]any{"a": float64(1)},
			nil,
			ErrInvalidPath,
		},
		{
			"invalid path - empty string",
			"",
			map[string]any{"a": float64(1)},
			nil,
			ErrInvalidPath,
		},
		{
			"invalid path - unclosed bracket",
			"$.items[",
			map[string]any{"items": []any{"a"}},
			nil,
			ErrInvalidPath,
		},
		{
			"invalid path - non-numeric index",
			"$.items[abc]",
			map[string]any{"items": []any{"a"}},
			nil,
			ErrInvalidPath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Evaluate(tt.path, tt.doc)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("got %v (%T), want %v (%T)", got, got, tt.want, tt.want)
			}
		})
	}
}
