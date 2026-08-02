package parser

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/assertion"
)

// TestParseFile_schema_assertion_compiles verifies that a collection with
// assertions.schema resolves and compiles the schema file at parse time,
// storing a non-nil *CompiledSchema on the request's Assertions.
func TestParseFile_schema_assertion_compiles(t *testing.T) {
	col, err := ParseFile("testdata/with_schema_assertion.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) == 0 {
		t.Fatal("expected at least one request")
	}
	item := col.Requests.Items[0]
	if item.Assertions.CompiledSchema == nil {
		t.Errorf("expected CompiledSchema to be non-nil after parse, got nil")
	}
}

// TestParseFile_schema_path_relative_to_collection_dir verifies that the schema
// path is resolved relative to the collection file's directory, not the process CWD.
func TestParseFile_schema_path_relative_to_collection_dir(t *testing.T) {
	// Resolve absolute path BEFORE changing CWD.
	absPath, err := filepath.Abs("testdata/with_schema_assertion.yaml")
	if err != nil {
		t.Fatalf("filepath.Abs: %v", err)
	}

	t.Chdir(t.TempDir()) // CWD is now unrelated

	col, parseErr := ParseFile(absPath)
	if parseErr != nil {
		t.Fatalf("ParseFile with different CWD: %v", parseErr)
	}
	if col.Requests.Items[0].Assertions.CompiledSchema == nil {
		t.Error("schema path resolution must be relative to collection dir, not CWD")
	}
}

// TestParseFile_schema_missing_file_returns_parse_error verifies that a
// schema path that does not exist on disk produces a structured parse error.
func TestParseFile_schema_missing_file_returns_parse_error(t *testing.T) {
	_, err := ParseFile("testdata/with_schema_missing.yaml")
	if err == nil {
		t.Fatal("expected error for missing schema file, got nil")
	}
	if !errors.Is(err, assertion.ErrSchemaFileNotFound) {
		t.Errorf("expected error to wrap ErrSchemaFileNotFound, got %v", err)
	}
}

// TestParseFile_schema_invalid_json_returns_parse_error verifies that a
// schema file containing invalid JSON produces a structured parse error.
func TestParseFile_schema_invalid_json_returns_parse_error(t *testing.T) {
	_, err := ParseFile("testdata/with_schema_invalid.yaml")
	if err == nil {
		t.Fatal("expected error for invalid schema file, got nil")
	}
	if !errors.Is(err, assertion.ErrSchemaInvalid) {
		t.Errorf("expected error to wrap ErrSchemaInvalid, got %v", err)
	}
}

// TestParseFile_no_schema_assertion_compiles_ok verifies that parsing a
// collection with no schema assertion still produces a nil CompiledSchema.
func TestParseFile_no_schema_assertion_compiles_ok(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if len(col.Requests.Items) == 0 {
		t.Fatal("expected at least one request")
	}
	item := col.Requests.Items[0]
	if item.Assertions.CompiledSchema != nil {
		t.Errorf("expected CompiledSchema to be nil for requests without schema assertion")
	}
}
