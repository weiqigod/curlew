package variable

import (
	"errors"
	"testing"
)

func TestExtract(t *testing.T) {
	tests := []struct {
		name        string
		extractions map[string]string
		body        string
		want        map[string]string
		wantErr     error
	}{
		{"single string value", map[string]string{"token": "$.data.token"}, `{"data":{"token":"abc"}}`, map[string]string{"token": "abc"}, nil},
		{"single number value", map[string]string{"id": "$.id"}, `{"id":42}`, map[string]string{"id": "42"}, nil},
		{"multiple extractions", map[string]string{"a": "$.a", "b": "$.b"}, `{"a":"x","b":"y"}`, map[string]string{"a": "x", "b": "y"}, nil},
		{"nested path", map[string]string{"t": "$.data.auth.token"}, `{"data":{"auth":{"token":"deep"}}}`, map[string]string{"t": "deep"}, nil},
		{"boolean value", map[string]string{"ok": "$.ok"}, `{"ok":true}`, map[string]string{"ok": "true"}, nil},
		{"null value", map[string]string{"v": "$.v"}, `{"v":null}`, map[string]string{"v": ""}, nil},
		{"array value", map[string]string{"arr": "$.items"}, `{"items":[1,2]}`, map[string]string{"arr": "[1,2]"}, nil},
		{"object value", map[string]string{"obj": "$.data"}, `{"data":{"k":"v"}}`, map[string]string{"obj": `{"k":"v"}`}, nil},
		{"no extractions", map[string]string{}, `{}`, map[string]string{}, nil},
		{"path not found", map[string]string{"x": "$.missing"}, `{"a":1}`, nil, ErrExtractionFailed},
		{"non-JSON body", map[string]string{"x": "$.a"}, `<html>not json</html>`, nil, ErrNotJSON},
		{"empty body", map[string]string{"x": "$.a"}, ``, nil, ErrNotJSON},
		{"float value", map[string]string{"f": "$.f"}, `{"f":3.14}`, map[string]string{"f": "3.14"}, nil},
		{"integer float value", map[string]string{"n": "$.n"}, `{"n":100.0}`, map[string]string{"n": "100"}, nil},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := Extract(ExtractionInput{
				Extractions: tt.extractions,
				Body:        []byte(tt.body),
			})
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("want error %v, got %v", tt.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(result.Variables) != len(tt.want) {
				t.Fatalf("got %d variables, want %d", len(result.Variables), len(tt.want))
			}
			for k, wantV := range tt.want {
				gotV, ok := result.Variables[k]
				if !ok {
					t.Errorf("missing variable %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("variable %q = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}
