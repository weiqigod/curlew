package schema_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// repoRoot walks up from this test file to the repo root.
func repoRoot(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../internal/schema/validate_coverage_test.go → up 3 levels.
	return filepath.Dir(filepath.Dir(filepath.Dir(thisFile)))
}

// TestSchema_examples validates every shipped example/fixture collection
// against the published schema. Fixtures are:
//   - internal/schema/testdata/*.yaml   (gap-closing fixtures owned by M8-002)
//   - sample/hello.yaml                 (repo-root sample)
//
// Deliberately invalid fixtures under internal/parser/testdata, cmd/curlew/testdata,
// etc. are NOT included — those exercise parser error paths by design.
func TestSchema_examples(t *testing.T) {
	root := repoRoot(t)
	var files []string

	testdataGlob := filepath.Join(root, "internal", "schema", "testdata", "*.yaml")
	matches, err := filepath.Glob(testdataGlob)
	if err != nil {
		t.Fatalf("glob %s: %v", testdataGlob, err)
	}
	for _, m := range matches {
		// Skip project-schema fixtures (output_project_*): they lack the
		// required collection fields (name, requests) and are validated
		// separately in TestSchema_accepts_output against the project schema.
		base := filepath.Base(m)
		if strings.HasPrefix(base, "output_project_") {
			continue
		}
		files = append(files, m)
	}

	samplePath := filepath.Join(root, "sample", "hello.yaml")
	if _, err := os.Stat(samplePath); err == nil {
		files = append(files, samplePath)
	}

	if len(files) == 0 {
		t.Fatalf("no example fixtures discovered under %s or %s", testdataGlob, samplePath)
	}

	sch := compileCollectionSchema(t)
	for _, path := range files {
		rel, err := filepath.Rel(root, path)
		if err != nil {
			rel = path
		}
		t.Run(rel, func(t *testing.T) {
			doc := decodeYAMLFile(t, path) // reused from validate_test.go (same package)
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", rel, err)
			}
		})
	}
}

// TestSchema_rejects_malformed_status is the negative control for Gap 8:
// schema must reject status values that are neither an integer nor an array of integers.
func TestSchema_rejects_malformed_status(t *testing.T) {
	cases := []struct {
		name string
		doc  map[string]any
	}{
		{"status_is_map", map[string]any{
			"name": "x",
			"requests": []any{map[string]any{
				"name":       "r",
				"request":    map[string]any{"method": "GET", "url": "u"},
				"assertions": map[string]any{"status": map[string]any{"foo": "bar"}},
			}},
		}},
		{"status_is_string", map[string]any{
			"name": "x",
			"requests": []any{map[string]any{
				"name":       "r",
				"request":    map[string]any{"method": "GET", "url": "u"},
				"assertions": map[string]any{"status": "200"},
			}},
		}},
	}
	sch := compileCollectionSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := sch.Validate(tc.doc); err == nil {
				t.Fatal("expected validation error, got nil")
			}
		})
	}
}

// TestSchema_rejects_variable_with_both_value_and_from_command is the negative
// control for Gap 7: schema must reject a variable entry with both value and
// from_command set simultaneously (mutually exclusive per parser semantics).
func TestSchema_rejects_variable_with_both_value_and_from_command(t *testing.T) {
	doc := map[string]any{
		"name": "x",
		"variables": map[string]any{
			"k": map[string]any{"from_command": "x", "value": "y"},
		},
		"requests": []any{map[string]any{
			"name":    "r",
			"request": map[string]any{"method": "GET", "url": "u"},
		}},
	}
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for variable with both value and from_command, got nil")
	}
}

// TestSchema_accepts_output validates that both collection and project schemas
// accept an optional output: block with all supported formats and verbosities.
func TestSchema_accepts_output(t *testing.T) {
	type acceptCase struct {
		name    string
		fixture string // relative to internal/schema/testdata
		target  string // "collection" or "project"
	}
	cases := []acceptCase{
		{"collection_output_terminal", "output_collection_terminal.yaml", "collection"},
		{"collection_output_json", "output_collection_json.yaml", "collection"},
		{"collection_output_tap", "output_collection_tap.yaml", "collection"},
		{"collection_output_junit", "output_collection_junit.yaml", "collection"},
		{"collection_output_html", "output_collection_html.yaml", "collection"},
		{"collection_output_markdown", "output_collection_markdown.yaml", "collection"},
		{"collection_output_quiet", "output_collection_verbosity_quiet.yaml", "collection"},
		{"collection_output_normal", "output_collection_verbosity_normal.yaml", "collection"},
		{"collection_output_verbose", "output_collection_verbosity_verbose.yaml", "collection"},
		{"collection_output_debug", "output_collection_verbosity_debug.yaml", "collection"},
		{"project_output_terminal", "output_project_terminal.yaml", "project"},
		{"project_output_json_report", "output_project_json_report.yaml", "project"},
		{"project_output_markdown", "output_project_markdown.yaml", "project"},
	}
	root := repoRoot(t)
	colSch := compileCollectionSchema(t)
	projSch := compileProjectSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "internal", "schema", "testdata", tc.fixture)
			doc := decodeYAMLFile(t, path)
			sch := colSch
			if tc.target == "project" {
				sch = projSch
			}
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", tc.fixture, err)
			}
		})
	}
}

// TestSchema_rejects_unknown_output_format is the negative control for
// output.format validation: yaml must be rejected (not a supported format).
func TestSchema_rejects_unknown_output_format(t *testing.T) {
	doc := map[string]any{
		"name":   "x",
		"output": map[string]any{"format": "yaml"},
		"requests": []any{map[string]any{
			"name":    "r",
			"request": map[string]any{"method": "GET", "url": "u"},
		}},
	}
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for output.format=yaml, got nil")
	}
}

// TestSchema_AcceptsMarkdownFormat verifies that both schemas/collection-v1.json
// and schemas/project-v1.json accept "markdown" as a valid format enum value.
func TestSchema_AcceptsMarkdownFormat(t *testing.T) {
	t.Run("collection", func(t *testing.T) {
		doc := map[string]any{
			"name":   "x",
			"output": map[string]any{"format": "markdown"},
			"requests": []any{map[string]any{
				"name":    "r",
				"request": map[string]any{"method": "GET", "url": "u"},
			}},
		}
		sch := compileCollectionSchema(t)
		if err := sch.Validate(doc); err != nil {
			t.Fatalf("expected markdown to validate in collection schema, got %v", err)
		}
	})

	t.Run("project", func(t *testing.T) {
		doc := map[string]any{
			"project_name": "test",
			"output":       map[string]any{"format": "markdown"},
		}
		sch := compileProjectSchema(t)
		if err := sch.Validate(doc); err != nil {
			t.Fatalf("expected markdown to validate in project schema, got %v", err)
		}
	})
}

// TestSchema_rejects_empty_output_path is the negative control for
// output.report validation: empty string must be rejected (minLength: 1).
func TestSchema_rejects_empty_output_path(t *testing.T) {
	doc := map[string]any{
		"name":   "x",
		"output": map[string]any{"report": ""},
		"requests": []any{map[string]any{
			"name":    "r",
			"request": map[string]any{"method": "GET", "url": "u"},
		}},
	}
	sch := compileCollectionSchema(t)
	if err := sch.Validate(doc); err == nil {
		t.Fatal("expected validation error for empty output.report, got nil")
	}
}

// collectionAround wraps a single request item in the minimal valid collection
// envelope, so a table row only has to state the part under test.
func collectionAround(item map[string]any) map[string]any {
	if _, ok := item["name"]; !ok {
		item["name"] = "r"
	}
	if _, ok := item["request"]; !ok {
		item["request"] = map[string]any{"method": "GET", "url": "https://example.com/"}
	}
	return map[string]any{"name": "x", "requests": []any{item}}
}

// TestSchema_method_is_optional_url_is_not mirrors the parser: url is the only
// required request field. method defaults to GET for http, POST for graphql and
// WS for websocket, so requiring it flagged valid collections — including the
// WebSocket example in docs/CLI_SPECIFICATION.md §12.3.
func TestSchema_method_is_optional_url_is_not(t *testing.T) {
	tests := []struct {
		name    string
		request map[string]any
		wantErr bool
	}{
		{"http_without_method", map[string]any{"url": "https://example.com/"}, false},
		{"http_with_method", map[string]any{"method": "POST", "url": "https://example.com/"}, false},
		{"websocket_without_method", map[string]any{
			"url":       "wss://example.com/socket",
			"protocol":  "websocket",
			"websocket": map[string]any{"steps": []any{map[string]any{"action": "close"}}},
		}, false},
		{"graphql_without_method", map[string]any{
			"url":      "https://example.com/graphql",
			"protocol": "graphql",
			"graphql":  map[string]any{"query": "{ me { id } }"},
		}, false},
		{"error case - request without url", map[string]any{"method": "GET"}, true},
	}
	sch := compileCollectionSchema(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := sch.Validate(collectionAround(map[string]any{"request": tc.request}))
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected clean validation, got %v", err)
			}
		})
	}
}

// TestSchema_rejects_unknown_keys_in_new_definitions is the negative control
// for the added definitions. additionalProperties:false is the schema's whole
// value — the parser itself ignores unknown keys — so a typo inside the new
// blocks must still be flagged.
func TestSchema_rejects_unknown_keys_in_new_definitions(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
	}{
		{"typo in signing params key", collectionAround(map[string]any{
			"signing": map[string]any{"type": "aws-sigv4", "parms": map[string]any{}},
		})},
		{"signing without type", collectionAround(map[string]any{
			"signing": map[string]any{"params": map[string]any{"region": "us-east-1"}},
		})},
		{"unknown protocol", collectionAround(map[string]any{
			"request": map[string]any{"url": "u", "protocol": "grpc"},
		})},
		{"typo in graphql key", collectionAround(map[string]any{
			"request": map[string]any{"url": "u", "protocol": "graphql", "graphql": map[string]any{"querry": "{ me }"}},
		})},
		{"unknown graphql error_handling", collectionAround(map[string]any{
			"request": map[string]any{"url": "u", "protocol": "graphql", "graphql": map[string]any{"query": "{me}", "error_handling": "explode"}},
		})},
		{"unknown websocket action", collectionAround(map[string]any{
			"request": map[string]any{"url": "u", "protocol": "websocket", "websocket": map[string]any{"steps": []any{map[string]any{"action": "yell"}}}},
		})},
		{"websocket without steps", collectionAround(map[string]any{
			"request": map[string]any{"url": "u", "protocol": "websocket", "websocket": map[string]any{}},
		})},
		{"depends_on as a bare string", collectionAround(map[string]any{"depends_on": "A"})},
		{"cel entry carrying an operator key", collectionAround(map[string]any{
			"assertions": map[string]any{"cel": []any{map[string]any{"cel": "x", "eq": 1}}},
		})},
		{"empty cel expression", collectionAround(map[string]any{
			"assertions": map[string]any{"cel": []any{""}},
		})},
		{"typo in collection config key", map[string]any{
			"name":     "x",
			"config":   map[string]any{"locail": "de-DE"},
			"requests": []any{map[string]any{"name": "r", "request": map[string]any{"url": "u"}}},
		}},
	}
	sch := compileCollectionSchema(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := sch.Validate(tc.doc); err == nil {
				t.Fatal("expected a validation error, got nil")
			}
		})
	}
}

// TestSchema_accepts_null_signing pins the explicit-null semantics the parser
// gives signing: ~ — an explicit null disables an inherited collection-level
// default for that item, so it must validate at both levels.
func TestSchema_accepts_null_signing(t *testing.T) {
	tests := []struct {
		name string
		doc  map[string]any
	}{
		{"null at request item", collectionAround(map[string]any{"signing": nil})},
		{"null at collection level", map[string]any{
			"name":     "x",
			"signing":  nil,
			"requests": []any{map[string]any{"name": "r", "request": map[string]any{"url": "u"}}},
		}},
		{"signing at both levels", map[string]any{
			"name":    "x",
			"signing": map[string]any{"type": "aws-sigv4", "params": map[string]any{"region": "us-east-1"}},
			"requests": []any{map[string]any{
				"name":    "r",
				"signing": nil,
				"request": map[string]any{"url": "u"},
			}},
		}},
	}
	sch := compileCollectionSchema(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if err := sch.Validate(tc.doc); err != nil {
				t.Fatalf("expected clean validation, got %v", err)
			}
		})
	}
}

// TestSchema_project_accepts_config_block is the acceptance test for the other
// half of the same defect: internal/config binds config: from curlew.yaml, but
// project-v1.json omitted it under additionalProperties:false.
func TestSchema_project_accepts_config_block(t *testing.T) {
	tests := []struct {
		name    string
		doc     map[string]any
		wantErr bool
	}{
		{"config with locale", map[string]any{"project_name": "p", "config": map[string]any{"locale": "de-DE"}}, false},
		{"empty config block", map[string]any{"project_name": "p", "config": map[string]any{}}, false},
		{"error case - typo inside config", map[string]any{"project_name": "p", "config": map[string]any{"locail": "de-DE"}}, true},
	}
	sch := compileProjectSchema(t)
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			err := sch.Validate(tc.doc)
			if tc.wantErr && err == nil {
				t.Fatal("expected a validation error, got nil")
			}
			if !tc.wantErr && err != nil {
				t.Fatalf("expected clean validation, got %v", err)
			}
		})
	}
}

// TestSchema_accepts is populated by Steps 2–9 (one sub-test per schema gap).
// Each sub-test is a {name, fixture} row in a table; the body unmarshals the
// fixture YAML and validates it against the embedded schema.
func TestSchema_accepts(t *testing.T) {
	type acceptCase struct {
		name    string
		fixture string // path relative to internal/schema/testdata
	}
	cases := []acceptCase{
		{"request_auth_string", "gap_1_request_auth.yaml"},
		{"status_integer", "gap_8_status_integer.yaml"},
		{"status_array", "gap_8_status_array.yaml"},
		{"section_object_form", "gap_6_section_object_form.yaml"},
		{"requests_object_form", "gap_6b_requests_object_form.yaml"},
		{"collection_retry", "gap_2_collection_retry.yaml"},
		{"section_retry", "gap_3_section_retry.yaml"},
		{"request_retry", "gap_4_request_retry.yaml"},
		{"data_driven_request", "gap_5_data_driven.yaml"},
		{"variables_object_form", "gap_7_variables_object.yaml"},
		{"conditional_execution", "gap_9_conditional_execution.yaml"},
		{"request_signing", "gap_10_signing.yaml"},
		{"graphql_protocol", "gap_11_graphql.yaml"},
		{"websocket_protocol", "gap_12_websocket.yaml"},
		{"cel_assertions", "gap_13_cel_assertions.yaml"},
		{"collection_config_block", "gap_14_collection_config.yaml"},
		{"all_nine_fields", "gap_15_all_nine_fields.yaml"},
	}
	if len(cases) == 0 {
		t.Skip("no gap fixtures registered yet — populated incrementally by Steps 2–9")
	}

	root := repoRoot(t)
	sch := compileCollectionSchema(t)
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			path := filepath.Join(root, "internal", "schema", "testdata", tc.fixture)
			doc := decodeYAMLFile(t, path)
			if err := sch.Validate(doc); err != nil {
				t.Fatalf("validate %s: %v", tc.fixture, err)
			}
		})
	}
}
