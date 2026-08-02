package schema

import (
	"encoding/json"
	"testing"
)

func TestCollectionSchema(t *testing.T) {
	tests := []struct {
		name  string
		check func(t *testing.T)
	}{
		{
			name: "embedded_schema_is_valid_json",
			check: func(t *testing.T) {
				if !json.Valid(CollectionSchema) {
					t.Fatalf("CollectionSchema is not valid JSON: %s", CollectionSchema[:100])
				}
			},
		},
		{
			name: "schema_has_schema_keyword",
			check: func(t *testing.T) {
				var m map[string]any
				if err := json.Unmarshal(CollectionSchema, &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if _, ok := m["$schema"]; !ok {
					t.Fatal("missing $schema keyword")
				}
			},
		},
		{
			name: "schema_has_required_properties",
			check: func(t *testing.T) {
				var m map[string]any
				if err := json.Unmarshal(CollectionSchema, &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				props, ok := m["properties"].(map[string]any)
				if !ok {
					t.Fatal("missing or invalid properties")
				}
				for _, key := range []string{"name", "requests"} {
					if _, exists := props[key]; !exists {
						t.Errorf("missing required property %q", key)
					}
				}
			},
		},
		{
			name: "schema_type_is_object",
			check: func(t *testing.T) {
				var m map[string]any
				if err := json.Unmarshal(CollectionSchema, &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				if m["type"] != "object" {
					t.Errorf("type = %v, want \"object\"", m["type"])
				}
			},
		},
		{
			name: "schema_has_rate_limit_rps_property",
			check: func(t *testing.T) {
				var m map[string]any
				if err := json.Unmarshal(CollectionSchema, &m); err != nil {
					t.Fatalf("unmarshal: %v", err)
				}
				props, ok := m["properties"].(map[string]any)
				if !ok {
					t.Fatal("missing or invalid properties")
				}
				if _, exists := props["rate_limit_rps"]; !exists {
					t.Error("missing rate_limit_rps property in schema")
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			tc.check(t)
		})
	}
}

func TestSchema_validates_include_directive(t *testing.T) {
	// Schema should have an "include" property that accepts an array of strings.
	var m map[string]any
	if err := json.Unmarshal(CollectionSchema, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props, ok := m["properties"].(map[string]any)
	if !ok {
		t.Fatal("missing or invalid properties")
	}
	inc, ok := props["include"]
	if !ok {
		t.Fatal("schema missing 'include' property")
	}
	incMap, ok := inc.(map[string]any)
	if !ok {
		t.Fatalf("'include' property is not a map, got %T", inc)
	}
	if incMap["type"] != "array" {
		t.Errorf("include.type = %v, want array", incMap["type"])
	}
	items, ok := incMap["items"].(map[string]any)
	if !ok {
		t.Fatalf("include.items is not a map, got %T", incMap["items"])
	}
	if items["type"] != "string" {
		t.Errorf("include.items.type = %v, want string", items["type"])
	}
}

func TestSchema_request_has_body_file_fields(t *testing.T) {
	var m map[string]any
	if err := json.Unmarshal(CollectionSchema, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	defs, ok := m["$defs"].(map[string]any)
	if !ok {
		t.Fatal("missing $defs")
	}
	request, ok := defs["request"].(map[string]any)
	if !ok {
		t.Fatal("$defs.request missing")
	}
	props, ok := request["properties"].(map[string]any)
	if !ok {
		t.Fatal("request.properties missing")
	}
	for _, key := range []string{"body_file", "body_binary_file"} {
		p, ok := props[key].(map[string]any)
		if !ok {
			t.Errorf("request.properties.%s missing", key)
			continue
		}
		if p["type"] != "string" {
			t.Errorf("request.properties.%s.type = %v, want string", key, p["type"])
		}
	}
}

func TestSchema_rejects_non_string_include_item(t *testing.T) {
	// Schema should reject include items that are not strings.
	// We verify by checking the items schema enforces string type.
	var m map[string]any
	if err := json.Unmarshal(CollectionSchema, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	props := m["properties"].(map[string]any)
	inc := props["include"].(map[string]any)
	items := inc["items"].(map[string]any)
	if items["type"] != "string" {
		t.Errorf("include.items.type = %v; non-string items should be rejected", items["type"])
	}
}
