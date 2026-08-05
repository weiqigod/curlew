package assertion

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// mustCompile compiles schemaJSON into a *CompiledSchema using a temp file.
// Fails the test if compilation fails.
func mustCompile(t *testing.T, schemaJSON string) *CompiledSchema {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(p, []byte(schemaJSON), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	cs, err := CompileSchemaFile(p)
	if err != nil {
		t.Fatalf("CompileSchemaFile: %v", err)
	}
	return cs
}

func TestCompileSchemaFile(t *testing.T) {
	tests := []struct {
		name    string
		write   string // contents; "" = don't create file
		wantErr error  // sentinel to check with errors.Is (nil = success)
	}{
		{
			name:    "valid draft 2020-12",
			write:   `{"$schema":"https://json-schema.org/draft/2020-12/schema","type":"object"}`,
			wantErr: nil,
		},
		{
			name:    "file missing",
			write:   "", // don't create
			wantErr: ErrSchemaFileNotFound,
		},
		{
			name:    "invalid json",
			write:   `{not json`,
			wantErr: ErrSchemaInvalid,
		},
		{
			name:    "invalid schema syntax",
			write:   `{"type":"not-a-type"}`,
			wantErr: ErrSchemaInvalid,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			var schemaPath string
			if tt.write != "" {
				schemaPath = filepath.Join(dir, "schema.json")
				if err := os.WriteFile(schemaPath, []byte(tt.write), 0o644); err != nil {
					t.Fatalf("write: %v", err)
				}
			} else {
				schemaPath = filepath.Join(dir, "missing.json")
			}
			cs, err := CompileSchemaFile(schemaPath)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				if cs == nil {
					t.Fatal("expected non-nil CompiledSchema")
				}
				if cs.Path() != schemaPath {
					t.Errorf("Path() = %q, want %q", cs.Path(), schemaPath)
				}
				return
			}
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Errorf("errors.Is(%v) = false, want true for %v", err, tt.wantErr)
			}
		})
	}
}

func TestCheckSchema(t *testing.T) {
	schemaJSON := `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"required":["id","email"],
		"properties":{
			"id":{"type":"integer"},
			"email":{"type":"string"}
		}
	}`
	compiled := mustCompile(t, schemaJSON)

	// schema with tags array for nested test
	nestedSchemaJSON := `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{
			"tags":{
				"type":"array",
				"items":{"type":"string"}
			}
		}
	}`
	nestedCompiled := mustCompile(t, nestedSchemaJSON)

	tests := []struct {
		name       string
		cmp        *CompiledSchema
		body       string
		wantPassed bool
		wantCount  int
		wantType   string // substring match against Result.Type
		wantExp    string // substring match against Result.Expected
		wantAct    string // substring match against Result.Actual
	}{
		{
			name:       "valid body",
			cmp:        compiled,
			body:       `{"id":1,"email":"a@b"}`,
			wantPassed: true,
			wantCount:  1,
			wantType:   "schema ",
			wantExp:    "valid",
			wantAct:    "valid",
		},
		{
			name:       "missing required email",
			cmp:        compiled,
			body:       `{"id":1}`,
			wantPassed: false,
			wantCount:  1,
			wantType:   "schema $",
			wantExp:    "email",
			wantAct:    "missing",
		},
		{
			name:       "type mismatch id string for int",
			cmp:        compiled,
			body:       `{"id":"not-an-int","email":"a@b"}`,
			wantPassed: false,
			wantCount:  1,
			wantType:   "schema $.id",
			wantExp:    "integer",
			wantAct:    "string",
		},
		{
			name:       "body not JSON",
			cmp:        compiled,
			body:       `<html/>`,
			wantPassed: false,
			wantCount:  1,
			wantType:   "schema ",
			wantExp:    "valid JSON",
			wantAct:    "response body is not JSON",
		},
		{
			name:       "empty body",
			cmp:        compiled,
			body:       ``,
			wantPassed: false,
			wantCount:  1,
			wantType:   "schema ",
			wantExp:    "valid JSON",
			wantAct:    "response body is not JSON",
		},
		{
			name:       "nested array index",
			cmp:        nestedCompiled,
			body:       `{"tags":[1]}`,
			wantPassed: false,
			wantCount:  1,
			wantType:   "schema $.tags[0]",
			wantExp:    "string",
			wantAct:    "number",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results := CheckSchema(tt.cmp, []byte(tt.body))
			if len(results) != tt.wantCount {
				t.Fatalf("got %d results, want %d: %+v", len(results), tt.wantCount, results)
			}
			r := results[0]
			if r.Passed != tt.wantPassed {
				t.Errorf("Passed = %v, want %v", r.Passed, tt.wantPassed)
			}
			if tt.wantType != "" && !strings.Contains(r.Label(), tt.wantType) {
				t.Errorf("Label() %q does not contain %q", r.Label(), tt.wantType)
			}
			if tt.wantExp != "" && !strings.Contains(r.Expected, tt.wantExp) {
				t.Errorf("Expected %q does not contain %q", r.Expected, tt.wantExp)
			}
			if tt.wantAct != "" && !strings.Contains(r.Actual, tt.wantAct) {
				t.Errorf("Actual %q does not contain %q", r.Actual, tt.wantAct)
			}
		})
	}
}

func TestCheckSchema_nil_compiled(t *testing.T) {
	if got := CheckSchema(nil, []byte("{}")); got != nil {
		t.Errorf("expected nil results for nil compiled, got %v", got)
	}
}

func TestToJSONPath(t *testing.T) {
	tests := []struct {
		name string
		in   []string
		want string
	}{
		{"empty", nil, "$"},
		{"single key", []string{"id"}, "$.id"},
		{"nested key", []string{"user", "email"}, "$.user.email"},
		{"array index", []string{"tags", "0"}, "$.tags[0]"},
		{"mixed", []string{"users", "2", "name"}, "$.users[2].name"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := toJSONPath(tt.in)
			if got != tt.want {
				t.Errorf("toJSONPath(%v) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

// TestCheckSchema_default_kind exercises the leafToResult default branch for
// schema violations that are not type or required errors (e.g. minimum, enum).
// This covers the two sub-branches: value found and value missing.
func TestCheckSchema_default_kind(t *testing.T) {
	// minimum violation: value is present but below the minimum
	minSchemaJSON := `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{
			"age":{"type":"integer","minimum":18}
		}
	}`
	minCompiled := mustCompile(t, minSchemaJSON)

	results := CheckSchema(minCompiled, []byte(`{"age":5}`))
	if len(results) == 0 {
		t.Fatal("expected at least one result for minimum violation, got none")
	}
	r := results[0]
	if r.Passed {
		t.Errorf("expected Passed=false for minimum violation")
	}
	// Actual should show the violating value (not "missing")
	if r.Actual == "missing" {
		t.Errorf("Actual = %q, expected a value (not 'missing') for minimum violation", r.Actual)
	}

	// enum violation with a field that resolves to a value
	enumSchemaJSON := `{
		"$schema":"https://json-schema.org/draft/2020-12/schema",
		"type":"object",
		"properties":{
			"status":{"enum":["active","inactive"]}
		}
	}`
	enumCompiled := mustCompile(t, enumSchemaJSON)

	enumResults := CheckSchema(enumCompiled, []byte(`{"status":"deleted"}`))
	if len(enumResults) == 0 {
		t.Fatal("expected at least one result for enum violation, got none")
	}
	er := enumResults[0]
	if er.Passed {
		t.Errorf("expected Passed=false for enum violation")
	}
	// Actual should contain the actual value "deleted"
	if !strings.Contains(er.Actual, "deleted") {
		t.Errorf("Actual = %q, expected to contain 'deleted'", er.Actual)
	}
}

func TestFlattenLeafErrors_collects_all(t *testing.T) {
	schemaJSON := `{
		"type":"object",
		"required":["a","b","c"],
		"properties":{
			"a":{"type":"integer"},
			"b":{"type":"integer"},
			"c":{"type":"integer"}
		}
	}`
	compiled := mustCompile(t, schemaJSON)
	// Two type errors (a and b are strings) plus c is missing
	results := CheckSchema(compiled, []byte(`{"a":"s","b":"s"}`))
	failed := 0
	for _, r := range results {
		if !r.Passed {
			failed++
		}
	}
	if failed < 3 {
		t.Errorf("want >= 3 failing results, got %d: %+v", failed, results)
	}
}
