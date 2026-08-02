package schema_test

import (
	"encoding/json"
	"testing"

	"github.com/peterlindqvist/apitest/internal/schema"
)

func TestProjectSchema(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{"embedded_is_valid_json", func(t *testing.T) {
			if !json.Valid(schema.ProjectSchema) {
				t.Fatalf("ProjectSchema is not valid JSON")
			}
		}},
		{"has_schema_keyword", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(schema.ProjectSchema, &m)
			if _, ok := m["$schema"]; !ok {
				t.Fatal("missing $schema")
			}
		}},
		{"has_title", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(schema.ProjectSchema, &m)
			if m["title"] != "ApiTest Project v1" {
				t.Errorf("title = %v, want \"ApiTest Project v1\"", m["title"])
			}
		}},
		{"has_id", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(schema.ProjectSchema, &m)
			want := "https://raw.githubusercontent.com/peterlindqvist/apitest/main/schemas/project-v1.json"
			if m["$id"] != want {
				t.Errorf("$id = %v, want %v", m["$id"], want)
			}
		}},
		{"additional_properties_false", func(t *testing.T) {
			var m map[string]any
			_ = json.Unmarshal(schema.ProjectSchema, &m)
			if m["additionalProperties"] != false {
				t.Errorf("additionalProperties = %v, want false", m["additionalProperties"])
			}
		}},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.check(t) })
	}
}
