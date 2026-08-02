package validator_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/validator"
)

func TestSeverityString(t *testing.T) {
	if got := validator.SeverityError.String(); got != "error" {
		t.Errorf("SeverityError.String() = %q, want %q", got, "error")
	}
	if got := validator.SeverityWarning.String(); got != "warning" {
		t.Errorf("SeverityWarning.String() = %q, want %q", got, "warning")
	}
	if got := validator.Severity(99).String(); got != "unknown" {
		t.Errorf("Severity(99).String() = %q, want %q", got, "unknown")
	}
}

func writeYAML(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	return path
}

func TestValidate(t *testing.T) {
	// behavior 1: valid file
	t.Run("valid_minimal_collection_exit_0", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", "name: Test\nrequests: []\n")

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got issues: %v", result.Issues)
		}
		if len(result.Issues) != 0 {
			t.Errorf("expected no issues, got: %v", result.Issues)
		}
	})

	t.Run("valid_collection_with_all_sections", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Full
variables:
  base: https://example.com
setup:
  - name: Setup
    request:
      url: "{{base}}/setup"
requests:
  - name: Main
    request:
      url: "{{base}}/main"
teardown:
  - name: Teardown
    request:
      url: "{{base}}/teardown"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got issues: %v", result.Issues)
		}
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityError {
				t.Errorf("unexpected error issue: %v", iss.Message)
			}
		}
	})

	// behavior 2: YAML syntax errors
	t.Run("invalid_yaml_returns_parse_error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "bad.yaml", "invalid: yaml: :\n")

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		if result.Issues[0].Severity != validator.SeverityError {
			t.Errorf("severity = %v, want SeverityError", result.Issues[0].Severity)
		}
	})

	t.Run("invalid_yaml_includes_line_number", func(t *testing.T) {
		dir := t.TempDir()
		// A YAML error that the go yaml lib will report with a line number
		path := writeYAML(t, dir, "bad.yaml", "name: Test\nrequests:\n  - foo: [unclosed\n")

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		// yaml.v3 should include a line number
		if result.Issues[0].Line == 0 {
			t.Errorf("expected line number in issue, got Line=0 (parser should extract line from YAML error)")
		}
	})

	// behavior 3: missing required fields
	t.Run("missing_request_name_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - request:
      url: "https://example.com"
`)

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityError && strings.Contains(iss.Message, "name") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error about missing name, got: %v", result.Issues)
		}
	})

	t.Run("missing_url_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: NoURL
    request:
      method: GET
`)

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		if result.Issues[0].Severity != validator.SeverityError {
			t.Errorf("severity = %v, want SeverityError", result.Issues[0].Severity)
		}
	})

	t.Run("unsupported_http_method_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: BadMethod
    request:
      method: BOGUS
      url: "https://example.com"
`)

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		if result.Issues[0].Severity != validator.SeverityError {
			t.Errorf("severity = %v, want SeverityError", result.Issues[0].Severity)
		}
	})

	// behavior 4: undefined variable references → warnings
	t.Run("undefined_variable_in_url_returns_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com/{{undefined_var}}"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid (warning only), got invalid")
		}
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "undefined_var") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about undefined_var, got: %v", result.Issues)
		}
	})

	t.Run("defined_collection_var_no_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
variables:
  base_url: "https://example.com"
requests:
  - name: Req
    request:
      url: "{{base_url}}/path"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got invalid")
		}
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "base_url") {
				t.Errorf("should not warn about defined variable base_url")
			}
		}
	})

	t.Run("defined_request_var_no_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    variables:
      token: "abc"
    request:
      url: "https://example.com"
      headers:
        Authorization: "Bearer {{token}}"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got invalid")
		}
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "token") {
				t.Errorf("should not warn about request-level variable token")
			}
		}
	})

	t.Run("extracted_var_no_warning_downstream", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: First
    request:
      url: "https://example.com/data"
    extract:
      token: "$.token"
  - name: Second
    request:
      url: "https://example.com/api"
      headers:
        Authorization: "Bearer {{token}}"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got invalid: %v", result.Issues)
		}
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "token") {
				t.Errorf("should not warn about extracted variable token")
			}
		}
	})

	t.Run("dynamic_func_ref_no_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com?id={{$uuid}}"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got invalid: %v", result.Issues)
		}
		if len(result.Issues) != 0 {
			t.Errorf("expected no issues for dynamic func ref, got: %v", result.Issues)
		}
	})

	t.Run("undefined_variable_in_query_param_returns_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com/items"
      query:
        id: "{{item_id}}"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid (warning only), got invalid")
		}
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "item_id") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about item_id in query param, got: %v", result.Issues)
		}
	})

	t.Run("known_vars_suppress_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com/{{external_var}}"
`)

		result := validator.Validate(path, map[string]string{"external_var": "value"})

		if !result.Valid {
			t.Errorf("expected valid, got invalid")
		}
		for _, iss := range result.Issues {
			if strings.Contains(iss.Message, "external_var") {
				t.Errorf("should not warn about var provided in knownVars")
			}
		}
	})

	// behavior 5: external file references
	t.Run("missing_external_ref_returns_error", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - path: nonexistent.yaml
`)

		result := validator.Validate(path, nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		if result.Issues[0].Severity != validator.SeverityError {
			t.Errorf("severity = %v, want SeverityError", result.Issues[0].Severity)
		}
	})

	t.Run("existing_external_ref_no_error", func(t *testing.T) {
		dir := t.TempDir()
		extPath := writeYAML(t, dir, "req.yaml", `name: External
request:
  url: "https://example.com"
`)
		_ = extPath
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - path: req.yaml
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got issues: %v", result.Issues)
		}
	})

	// edge cases
	t.Run("nonexistent_file_returns_error", func(t *testing.T) {
		result := validator.Validate("/tmp/nonexistent_file_that_does_not_exist.yaml", nil)

		if result.Valid {
			t.Error("expected invalid, got valid")
		}
		if len(result.Issues) == 0 {
			t.Fatal("expected issues, got none")
		}
		if result.Issues[0].Severity != validator.SeverityError {
			t.Errorf("severity = %v, want SeverityError", result.Issues[0].Severity)
		}
	})

	t.Run("empty_requests_list_is_valid", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", "name: Test\nrequests: []\n")

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid, got issues: %v", result.Issues)
		}
	})

	t.Run("undefined_variable_in_body_array_returns_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com"
      body:
        - "{{array_var}}"
        - "literal"
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid (warning only)")
		}
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "array_var") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about array_var, got: %v", result.Issues)
		}
	})

	t.Run("undefined_variable_in_body_map_returns_warning", func(t *testing.T) {
		dir := t.TempDir()
		path := writeYAML(t, dir, "col.yaml", `name: Test
requests:
  - name: Req
    request:
      url: "https://example.com"
      body:
        token: "{{body_var}}"
        count: 1
`)

		result := validator.Validate(path, nil)

		if !result.Valid {
			t.Errorf("expected valid (warning only)")
		}
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityWarning && strings.Contains(iss.Message, "body_var") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected warning about body_var, got: %v", result.Issues)
		}
	})
}

// TestValidate_If_ParseError verifies that a collection with an invalid CEL
// expression in an if: field is rejected with an error mentioning ERR_CEL_PARSE.
func TestValidate_If_ParseError(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "parse_error.yaml", `
name: Test
requests:
  - name: A
    if: "previous.body.status =="
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if result.Valid {
		t.Fatal("expected invalid, got valid")
	}
	found := false
	for _, iss := range result.Issues {
		if strings.Contains(iss.Message, "ERR_CEL_PARSE") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue with ERR_CEL_PARSE; got issues: %v", result.Issues)
	}
}

// TestValidate_If_TypeError verifies that a collection with a non-bool CEL
// expression in an if: field is rejected with an error mentioning ERR_CEL_TYPE.
func TestValidate_If_TypeError(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "type_error.yaml", `
name: Test
requests:
  - name: A
    if: "1 + 1"
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if result.Valid {
		t.Fatal("expected invalid, got valid")
	}
	found := false
	for _, iss := range result.Issues {
		if strings.Contains(iss.Message, "ERR_CEL_TYPE") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue with ERR_CEL_TYPE; got issues: %v", result.Issues)
	}
}

// TestValidate_If_BoolPasses verifies that a valid bool CEL expression in an
// if: field does not produce any additional error.
func TestValidate_If_BoolPasses(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "bool_ok.yaml", `
name: Test
requests:
  - name: A
    if: "1 + 1 == 2"
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	for _, iss := range result.Issues {
		if iss.Severity == validator.SeverityError && (strings.Contains(iss.Message, "ERR_CEL") || strings.Contains(iss.Message, "if:")) {
			t.Errorf("unexpected CEL error for valid expression: %v", iss)
		}
	}
}

// TestValidate_If_FieldPath verifies that the field path in a CEL error message
// identifies the location of the failing if: expression.
func TestValidate_If_FieldPath(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "field_path.yaml", `
name: Test
requests:
  - name: A
    if: "previous.body.status =="
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if result.Valid {
		t.Fatal("expected invalid, got valid")
	}
	// The message should indicate the field path.
	found := false
	for _, iss := range result.Issues {
		if strings.Contains(iss.Message, "requests[0].if") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue message to contain 'requests[0].if'; got issues: %v", result.Issues)
	}
}

// requireIssue locates an issue whose message contains both fieldPath and
// label, fails the test if absent, and returns it for further assertions.
func requireIssue(t *testing.T, r *validator.Result, fieldPath, label string) validator.Issue {
	t.Helper()
	for _, iss := range r.Issues {
		if strings.Contains(iss.Message, fieldPath) && strings.Contains(iss.Message, label) {
			return iss
		}
	}
	t.Fatalf("expected issue with field path %q and label %q; got: %v", fieldPath, label, r.Issues)
	return validator.Issue{}
}

// TestValidateCel_ParseErrorReported asserts that a syntactically invalid
// expression on assertions: - cel: produces an ERR_CEL_PARSE issue with the
// field path requests[0].assertions[0].cel.
func TestValidateCel_ParseErrorReported(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "cel_parse.yaml", `
name: Test
requests:
  - name: A
    request:
      method: GET
      url: https://example.com
    assertions:
      cel:
        - "response.status =="
`)
	r := validator.Validate(path, nil)
	if r.Valid {
		t.Fatal("expected invalid")
	}
	requireIssue(t, r, "requests[0].assertions[0].cel", "ERR_CEL_PARSE")
}

// TestValidateCel_TypeErrorReported asserts that a non-bool expression on
// an if: field produces an ERR_CEL_TYPE issue naming int as the actual type
// and bool as the expected type.
func TestValidateCel_TypeErrorReported(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "cel_type.yaml", `
name: Test
requests:
  - name: A
    if: "1 + 2"
    request:
      method: GET
      url: https://example.com
`)
	r := validator.Validate(path, nil)
	if r.Valid {
		t.Fatal("expected invalid")
	}
	iss := requireIssue(t, r, "requests[0].if", "ERR_CEL_TYPE")
	if !strings.Contains(iss.Message, "int") {
		t.Errorf("expected actual type 'int' in message; got %q", iss.Message)
	}
	if !strings.Contains(iss.Message, "bool") {
		t.Errorf("expected expected type 'bool' in message; got %q", iss.Message)
	}
}

// TestValidateCel_WalksAllCelSites asserts that the walker visits if: sites
// on setup, requests, and teardown and assertions: - cel: on requests, and
// emits one issue per failing site.
func TestValidateCel_WalksAllCelSites(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "all_sites.yaml", `
name: Test
setup:
  - name: S
    if: "1"
    request:
      method: GET
      url: https://example.com
requests:
  - name: A
    if: "previous.body.status =="
    request:
      method: GET
      url: https://example.com
    assertions:
      cel:
        - "response.status =="
teardown:
  - name: T
    if: "2 + 3"
    request:
      method: GET
      url: https://example.com
`)
	r := validator.Validate(path, nil)
	if r.Valid {
		t.Fatal("expected invalid")
	}
	requireIssue(t, r, "setup[0].if", "ERR_CEL_TYPE")
	requireIssue(t, r, "requests[0].if", "ERR_CEL_PARSE")
	requireIssue(t, r, "requests[0].assertions[0].cel", "ERR_CEL_PARSE")
	requireIssue(t, r, "teardown[0].if", "ERR_CEL_TYPE")
}

// TestValidateCel_TruncatesSourceTo200Chars asserts the printed source
// excerpt is exactly 200 runes followed by an ellipsis marker.
func TestValidateCel_TruncatesSourceTo200Chars(t *testing.T) {
	long := strings.Repeat("a", 250) + " == 1"
	dir := t.TempDir()
	path := writeYAML(t, dir, "long.yaml", "name: Test\nrequests:\n  - name: A\n    if: \""+long+"\"\n    request:\n      method: GET\n      url: https://example.com\n")
	r := validator.Validate(path, nil)
	if r.Valid {
		t.Fatal("expected invalid")
	}
	iss := requireIssue(t, r, "requests[0].if", "ERR_CEL_PARSE")
	// The printed source must contain 200 'a' runes followed by '…' and must
	// not contain the 201st rune of the original expression.
	expected := strings.Repeat("a", 200) + "…"
	if !strings.Contains(iss.Message, expected) {
		t.Errorf("expected truncated source %q in message; got %q", expected, iss.Message)
	}
}

// TestValidateCel_NoHttpRequestFiredDuringValidate asserts that validate
// performs no network I/O even when the collection URL points at an
// unreachable host. The test bounds wall time at 2s; validate is pure-CPU
// and should finish in milliseconds.
func TestValidateCel_NoHttpRequestFiredDuringValidate(t *testing.T) {
	dir := t.TempDir()
	// TEST-NET-1 host on a port that will hang or RST quickly.
	path := writeYAML(t, dir, "no_http.yaml", `
name: Test
requests:
  - name: A
    if: "vars.x == 1"
    request:
      method: GET
      url: http://192.0.2.1:1/unreachable
variables:
  x: 1
`)
	done := make(chan struct{})
	var r *validator.Result
	go func() {
		r = validator.Validate(path, nil)
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("validate did not return in 2s — likely performed network I/O")
	}
	if !r.Valid {
		t.Errorf("expected valid (no errors); got issues: %v", r.Issues)
	}
}

// TestValidate_RetryStatusRanges_InvalidRejected verifies that an unparsable
// status range in retry_on is reported as a validation error rather than
// silently ignored at runtime.
func TestValidate_RetryStatusRanges_InvalidRejected(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "bad_range.yaml", `
name: Test
requests:
  - name: A
    retry:
      enabled: true
      retry_on:
        status_ranges: ["banana"]
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if result.Valid {
		t.Fatal("expected invalid, got valid")
	}
	found := false
	for _, iss := range result.Issues {
		if iss.Severity == validator.SeverityError && strings.Contains(iss.Message, "banana") {
			found = true
			if !strings.Contains(iss.Message, "requests[0].retry.retry_on.status_ranges") {
				t.Errorf("expected field path in message, got %q", iss.Message)
			}
		}
	}
	if !found {
		t.Errorf("expected error mentioning %q; got issues: %v", "banana", result.Issues)
	}
}

// TestValidate_RetryStatusRanges_ValidFormsAccepted verifies that both the
// "min-max" form and the "Nxx" shorthand pass validation.
func TestValidate_RetryStatusRanges_ValidFormsAccepted(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "good_ranges.yaml", `
name: Test
retry:
  enabled: true
  retry_on:
    status_ranges: ["5xx", "500-599"]
requests:
  - name: A
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if !result.Valid {
		t.Fatalf("expected valid, got issues: %v", result.Issues)
	}
}

// TestValidate_RetryStatusRanges_AllSitesChecked verifies that collection-level,
// section-level, and do_not_retry_on status ranges are all validated.
func TestValidate_RetryStatusRanges_AllSitesChecked(t *testing.T) {
	dir := t.TempDir()
	path := writeYAML(t, dir, "all_sites.yaml", `
name: Test
retry:
  enabled: true
  retry_on:
    status_ranges: ["col-bad"]
setup:
  retry:
    enabled: true
    do_not_retry_on:
      status_ranges: ["setup-bad"]
  items:
    - name: S
      request:
        method: GET
        url: https://example.com
requests:
  - name: A
    retry:
      enabled: true
      do_not_retry_on:
        status_ranges: ["req-bad"]
    request:
      method: GET
      url: https://example.com
`)
	result := validator.Validate(path, nil)

	if result.Valid {
		t.Fatal("expected invalid, got valid")
	}
	for _, want := range []string{
		"retry.retry_on.status_ranges",
		"setup.retry.do_not_retry_on.status_ranges",
		"requests[0].retry.do_not_retry_on.status_ranges",
	} {
		found := false
		for _, iss := range result.Issues {
			if iss.Severity == validator.SeverityError && strings.Contains(iss.Message, want) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected error for site %q; got issues: %v", want, result.Issues)
		}
	}
}
