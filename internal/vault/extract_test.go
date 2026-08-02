package vault

import (
	"errors"
	"testing"
)

func TestExtractField(t *testing.T) {
	tests := []struct {
		name      string
		rawJSON   string
		fieldName string
		want      string
		wantErr   error
	}{
		{"extracts_string_field", `{"password":"s3cret"}`, "password", "s3cret", nil},
		{"extracts_numeric_field", `{"port":5432}`, "port", "5432", nil},
		{"extracts_boolean_field", `{"active":true}`, "active", "true", nil},
		{"extracts_nested_object_as_json", `{"cfg":{"a":1}}`, "cfg", `{"a":1}`, nil},
		{"missing_field_returns_error", `{"a":"b"}`, "c", "", ErrFieldNotFound},
		{"non_json_returns_error", `not json`, "x", "", ErrNotJSONSecret},
		{"empty_json_object", `{}`, "x", "", ErrFieldNotFound},
		{"null_field_value", `{"x":null}`, "x", "", nil},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := ExtractField(tc.rawJSON, tc.fieldName)
			if tc.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("expected %v, got %v", tc.wantErr, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("got %q, want %q", got, tc.want)
			}
		})
	}
}
