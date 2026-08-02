package openapi

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/validator"
)

// errWriter is an io.Writer that always returns an error, used to test
// Emit's encode-failure and close-error paths.
type errWriter struct{}

func (errWriter) Write(_ []byte) (int, error) {
	return 0, errors.New("write error")
}

func TestEmit_ContainsExpectedKeys(t *testing.T) {
	col, err := Import("testdata/petstore.yaml")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var buf bytes.Buffer
	if err := Emit(&buf, col); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	out := buf.String()
	wantSubstrings := []string{
		"name: Petstore",
		"variables:",
		"base_url: https://api.example.com/v1",
		"method: GET",
		"method: POST",
		"name: listPets",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(out, s) {
			t.Errorf("emitted YAML missing %q:\n%s", s, out)
		}
	}
	// Check URL is present (yaml.v3 may quote with single or double quotes)
	if !strings.Contains(out, "{{base_url}}/pets") {
		t.Errorf("emitted YAML missing URL with base_url:\n%s", out)
	}
	if !strings.Contains(out, "{{base_url}}/pets/{{petId}}") {
		t.Errorf("emitted YAML missing URL with petId param:\n%s", out)
	}
}

func TestEmit_FullFixture(t *testing.T) {
	col, err := Import("testdata/petstore_full.yaml")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	var buf bytes.Buffer
	if err := Emit(&buf, col); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	out := buf.String()

	wantSubstrings := []string{
		"x_api_key:", // collection variable declared (yaml.v3 uses "" for empty strings)
		"X-API-Key:", // header key preserved with original casing
		"x_api_key",  // variable reference in header value
		"limit={{limit}}",
		"name: string",
		"status:",
		"- 200",
		"- 404",
		"- 201",
	}
	for _, s := range wantSubstrings {
		if !strings.Contains(out, s) {
			t.Errorf("emitted YAML missing %q:\n%s", s, out)
		}
	}

	// Round-trip through parser and validator.
	tmp := filepath.Join(t.TempDir(), "full.collection.yaml")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Emit(f, col); err != nil {
		t.Fatalf("Emit to file: %v", err)
	}
	_ = f.Close()

	reparsed, err := parser.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if reparsed.Name != "Petstore Full" {
		t.Errorf("Name = %q, want Petstore Full", reparsed.Name)
	}
	if len(reparsed.Requests.Items) != 3 {
		t.Errorf("re-parsed len = %d, want 3", len(reparsed.Requests.Items))
	}

	res := validator.Validate(tmp, nil)
	if !res.Valid {
		t.Errorf("validate failed: %+v", res.Issues)
	}
}

func TestEmit_RoundTripsThroughParser(t *testing.T) {
	col, err := Import("testdata/petstore.yaml")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	tmp := filepath.Join(t.TempDir(), "petstore.collection.yaml")
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := Emit(f, col); err != nil {
		t.Fatalf("Emit: %v", err)
	}
	_ = f.Close()

	// Must parse back cleanly.
	reparsed, err := parser.ParseFile(tmp)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if reparsed.Name != "Petstore" {
		t.Errorf("Name = %q, want Petstore", reparsed.Name)
	}
	if len(reparsed.Requests.Items) != 3 {
		t.Fatalf("re-parsed len = %d, want 3", len(reparsed.Requests.Items))
	}
	if reparsed.Variables.Values["base_url"] != "https://api.example.com/v1" {
		t.Errorf("base_url did not round-trip: %v", reparsed.Variables.Values)
	}

	// Must validate clean.
	res := validator.Validate(tmp, nil)
	if !res.Valid {
		t.Errorf("validate failed: %+v", res.Issues)
	}
}

func TestEmit_WriteError(t *testing.T) {
	col, err := Import("testdata/petstore.yaml")
	if err != nil {
		t.Fatalf("Import: %v", err)
	}
	// errWriter fails on every write, exercising the enc.Encode failure path
	// and the deferred enc.Close() error capture branch in Emit.
	if err := Emit(errWriter{}, col); err == nil {
		t.Error("Emit(errWriter) returned nil, want an error")
	}
}
