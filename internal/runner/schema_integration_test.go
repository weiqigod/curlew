package runner

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/assertion"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
)

// compileTestSchema creates a temp JSON Schema file from schemaJSON,
// compiles it, and returns the *CompiledSchema. Fails the test on error.
func compileTestSchema(t *testing.T, schemaJSON string) *assertion.CompiledSchema {
	t.Helper()
	dir := t.TempDir()
	p := filepath.Join(dir, "schema.json")
	if err := os.WriteFile(p, []byte(schemaJSON), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}
	cs, err := assertion.CompileSchemaFile(p)
	if err != nil {
		t.Fatalf("CompileSchemaFile: %v", err)
	}
	return cs
}

// bodyExecutorWith returns an ExecuteFunc that returns the given status code and body.
func bodyExecutorWith(status int, body string) ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: status,
			Duration:   10 * time.Millisecond,
			Body:       []byte(body),
		}, nil
	}
}

// TestRun_schema_assertion_pass verifies that a response body matching the
// compiled schema produces a passing assertion result.
func TestRun_schema_assertion_pass(t *testing.T) {
	schema := compileTestSchema(t, `{"type":"object","required":["id","name"],"properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`)
	item := parser.RequestItem{
		Name:    "Get User",
		Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"},
		Assertions: parser.Assertions{
			CompiledSchema: schema,
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, bodyExecutorWith(200, `{"id":1,"name":"Alice"}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0; results: %+v", summary.Failed, results[0].AssertionResults)
	}
	if results[0].AssertionResults == nil {
		t.Fatal("expected non-nil AssertionResults")
	}
	if !results[0].AssertionResults.Passed {
		t.Errorf("expected assertion to pass; items: %+v", results[0].AssertionResults.Items)
	}
}

// TestRun_schema_assertion_fail_missing_field verifies that a response body
// missing a required field produces a failing assertion result.
func TestRun_schema_assertion_fail_missing_field(t *testing.T) {
	schema := compileTestSchema(t, `{"type":"object","required":["id","name"],"properties":{"id":{"type":"integer"},"name":{"type":"string"}}}`)
	item := parser.RequestItem{
		Name:    "Get User",
		Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"},
		Assertions: parser.Assertions{
			CompiledSchema: schema,
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, bodyExecutorWith(200, `{"id":1}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if results[0].AssertionResults == nil {
		t.Fatal("expected non-nil AssertionResults")
	}
	if results[0].AssertionResults.Passed {
		t.Error("expected assertion to fail due to missing required field 'name'")
	}
	// At least one schema result item should be present
	sawSchemaResult := false
	for _, r := range results[0].AssertionResults.Items {
		if len(r.Type) >= 7 && r.Type[:7] == "schema " {
			sawSchemaResult = true
			break
		}
	}
	if !sawSchemaResult {
		t.Errorf("expected at least one schema assertion result, got: %+v", results[0].AssertionResults.Items)
	}
}

// TestRun_schema_assertion_fail_wrong_type verifies that a response body with
// a field of the wrong type produces a failing assertion result.
func TestRun_schema_assertion_fail_wrong_type(t *testing.T) {
	schema := compileTestSchema(t, `{"type":"object","properties":{"id":{"type":"integer"}}}`)
	item := parser.RequestItem{
		Name:    "Get User",
		Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"},
		Assertions: parser.Assertions{
			CompiledSchema: schema,
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, bodyExecutorWith(200, `{"id":"not-an-int"}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if results[0].AssertionResults == nil || results[0].AssertionResults.Passed {
		t.Error("expected assertion to fail due to wrong type for 'id'")
	}
}

// TestRun_schema_assertion_combined_with_status verifies that schema and
// status assertions can be combined and both are evaluated.
func TestRun_schema_assertion_combined_with_status(t *testing.T) {
	schema := compileTestSchema(t, `{"type":"object","required":["id"],"properties":{"id":{"type":"integer"}}}`)
	item := parser.RequestItem{
		Name:    "Get User",
		Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"},
		Assertions: parser.Assertions{
			Status:         parser.StatusCodes{Codes: []int{200}},
			CompiledSchema: schema,
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, bodyExecutorWith(200, `{"id":42}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0", summary.Failed)
	}
	ar := results[0].AssertionResults
	if ar == nil {
		t.Fatal("expected non-nil AssertionResults")
	}
	sawStatus, sawSchema := false, false
	for _, r := range ar.Items {
		if r.Type == "status" {
			sawStatus = true
		}
		if len(r.Type) >= 7 && r.Type[:7] == "schema " {
			sawSchema = true
		}
	}
	if !sawStatus {
		t.Error("expected status assertion result")
	}
	if !sawSchema {
		t.Error("expected schema assertion result")
	}
}

// TestRun_schema_nil_no_effect verifies that a nil CompiledSchema has no
// effect on assertion evaluation (backward compatibility).
func TestRun_schema_nil_no_effect(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Status:         parser.StatusCodes{Codes: []int{200}},
			CompiledSchema: nil,
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	_, summary, err := Run(context.Background(), col, bodyExecutorWith(200, `{"id":1}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0", summary.Failed)
	}
}

// TestRun_schema_assertion_end_to_end_parse verifies the full pipeline:
// ParseFile → Run with schema-validated response body.
func TestRun_schema_assertion_end_to_end_parse(t *testing.T) {
	// Write fixtures to a temp dir so we control the schema path.
	dir := t.TempDir()

	schemaContent := `{"type":"object","required":["id"],"properties":{"id":{"type":"integer"}}}`
	schemaPath := filepath.Join(dir, "user.schema.json")
	if err := os.WriteFile(schemaPath, []byte(schemaContent), 0o644); err != nil {
		t.Fatalf("write schema: %v", err)
	}

	colContent := "name: E2E Schema Test\nrequests:\n  - name: Get User\n    request:\n      method: GET\n      url: \"https://example.com/users/1\"\n    assertions:\n      schema: \"./user.schema.json\"\n"
	colPath := filepath.Join(dir, "collection.yaml")
	if err := os.WriteFile(colPath, []byte(colContent), 0o644); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	col, err := parser.ParseFile(colPath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	if col.Requests.Items[0].Assertions.CompiledSchema == nil {
		t.Fatal("expected CompiledSchema to be non-nil after ParseFile")
	}

	// Pass body — should pass
	results, summary, runErr := Run(context.Background(), col, bodyExecutorWith(200, `{"id":1}`), VarSources{})
	if runErr != nil {
		t.Fatalf("Run() error: %v", runErr)
	}
	if summary.Failed != 0 {
		t.Errorf("pass case: failed = %d, want 0; items: %+v", summary.Failed, results[0].AssertionResults)
	}

	// Fail body — should fail
	_, failSummary, runErr2 := Run(context.Background(), col, bodyExecutorWith(200, `{}`), VarSources{})
	if runErr2 != nil {
		t.Fatalf("Run() (fail case) error: %v", runErr2)
	}
	if failSummary.Failed != 1 {
		t.Errorf("fail case: failed = %d, want 1", failSummary.Failed)
	}
}
