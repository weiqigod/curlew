package parser

import (
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	graphqlfiles "github.com/peterlindqvist/apitest/internal/graphql/files"
	"github.com/peterlindqvist/apitest/internal/httpbody"
	"github.com/peterlindqvist/apitest/internal/variable"
	"gopkg.in/yaml.v3"
)

func TestParseFile(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr error
		wantCol *Collection
	}{
		{
			name: "valid minimal collection",
			file: "testdata/minimal.yaml",
			wantCol: &Collection{
				Name: "Minimal Test",
				Requests: Section{Items: []RequestItem{
					{
						Name:    "Get Example",
						Request: Request{Method: "GET", URL: "https://example.com"},
					},
				}},
			},
		},
		{
			name:    "file not found",
			file:    "testdata/nonexistent.yaml",
			wantErr: ErrFileNotFound,
		},
		{
			name:    "invalid YAML",
			file:    "testdata/invalid.yaml",
			wantErr: ErrInvalidYAML,
		},
		{
			name:    "empty collection name",
			file:    "testdata/no_name.yaml",
			wantErr: ErrEmptyCollection,
		},
		{
			name: "collection with headers",
			file: "testdata/with_headers.yaml",
			wantCol: &Collection{
				Name: "Headers Test",
				Requests: Section{Items: []RequestItem{
					{
						Name: "Get With Headers",
						Request: Request{
							Method:  "GET",
							URL:     "https://example.com",
							Headers: map[string]string{"Accept": "application/json"},
						},
					},
				}},
			},
		},
		{
			name: "method defaults to GET when empty",
			file: "testdata/no_method.yaml",
			wantCol: &Collection{
				Name: "No Method Test",
				Requests: Section{Items: []RequestItem{
					{
						Name:    "Default Method",
						Request: Request{Method: "GET", URL: "https://example.com"},
					},
				}},
			},
		},
		{
			name: "method is normalized to uppercase",
			file: "testdata/post_lowercase.yaml",
			wantCol: &Collection{
				Name: "Lowercase Method Test",
				Requests: Section{Items: []RequestItem{
					{
						Name:    "Post Lowercase",
						Request: Request{Method: "POST", URL: "https://example.com"},
					},
				}},
			},
		},
		{
			name:    "unsupported method returns error",
			file:    "testdata/unsupported_method.yaml",
			wantErr: ErrUnsupportedMethod,
		},
		{
			name: "collection with stop_on_failure true",
			file: "testdata/with_options.yaml",
			wantCol: &Collection{
				Name: "Options Test",
				Requests: Section{Items: []RequestItem{
					{
						Name:    "Get Example",
						Request: Request{Method: "GET", URL: "https://example.com"},
					},
				}},
				Options: Options{StopOnFailure: true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			stripSourceLocations(col)
			if !reflect.DeepEqual(col, tt.wantCol) {
				t.Errorf("got %+v, want %+v", col, tt.wantCol)
			}
		})
	}
}

func TestParseFile_empty_requests(t *testing.T) {
	col, err := ParseFile("testdata/empty_requests.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 0 {
		t.Errorf("got %d requests, want 0", len(col.Requests.Items))
	}
}

func TestParseFile_body_as_map(t *testing.T) {
	col, err := ParseFile("testdata/with_body_map.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}

	body, ok := col.Requests.Items[0].Request.Body.(map[string]interface{})
	if !ok {
		t.Fatalf("body is %T, want map[string]interface{}", col.Requests.Items[0].Request.Body)
	}
	if body["email"] != "test@example.com" {
		t.Errorf("body[email] = %v, want test@example.com", body["email"])
	}
	if body["name"] != "Test" {
		t.Errorf("body[name] = %v, want Test", body["name"])
	}
}

func TestParseFile_body_as_string(t *testing.T) {
	col, err := ParseFile("testdata/with_body_string.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}

	body, ok := col.Requests.Items[0].Request.Body.(string)
	if !ok {
		t.Fatalf("body is %T, want string", col.Requests.Items[0].Request.Body)
	}
	if body != "raw string body" {
		t.Errorf("body = %q, want %q", body, "raw string body")
	}
}

func TestParseFile_query_params(t *testing.T) {
	col, err := ParseFile("testdata/with_query.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}

	want := map[string]string{"page": "1", "limit": "10"}
	got := col.Requests.Items[0].Request.QueryParams
	if !reflect.DeepEqual(got, want) {
		t.Errorf("query params = %v, want %v", got, want)
	}
}

func TestParseFile_single_status_assertion(t *testing.T) {
	col, err := ParseFile("testdata/with_status_assertion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	codes := col.Requests.Items[0].Assertions.Status.Codes
	want := []int{200}
	if !reflect.DeepEqual(codes, want) {
		t.Errorf("status codes = %v, want %v", codes, want)
	}
}

func TestParseFile_list_status_assertion(t *testing.T) {
	col, err := ParseFile("testdata/with_status_list_assertion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	codes := col.Requests.Items[0].Assertions.Status.Codes
	want := []int{200, 201}
	if !reflect.DeepEqual(codes, want) {
		t.Errorf("status codes = %v, want %v", codes, want)
	}
}

func TestParseFile_no_assertions_parses_to_empty(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	codes := col.Requests.Items[0].Assertions.Status.Codes
	if len(codes) != 0 {
		t.Errorf("status codes = %v, want empty", codes)
	}
}

func TestParseFile_status_invalid_scalar(t *testing.T) {
	_, err := ParseFile("testdata/status_invalid_scalar.yaml")
	if err == nil {
		t.Fatal("expected error for non-integer status code")
	}
	if !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("error chain missing ErrInvalidYAML: %v", err)
	}
}

func TestParseFile_status_invalid_sequence(t *testing.T) {
	_, err := ParseFile("testdata/status_invalid_sequence.yaml")
	if err == nil {
		t.Fatal("expected error for non-integer in status list")
	}
	if !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("error chain missing ErrInvalidYAML: %v", err)
	}
}

func TestParseFile_status_invalid_type(t *testing.T) {
	_, err := ParseFile("testdata/status_invalid_type.yaml")
	if err == nil {
		t.Fatal("expected error for map status type")
	}
	if !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("error chain missing ErrInvalidYAML: %v", err)
	}
}

func TestParseFile_body_assertion_equals(t *testing.T) {
	col, err := ParseFile("testdata/with_body_assertion_equals.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 1 {
		t.Fatalf("got %d body assertions, want 1", len(items))
	}
	if items[0].Path != "$.data.id" {
		t.Errorf("Path = %q, want %q", items[0].Path, "$.data.id")
	}
	if items[0].Operator != "equals" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "equals")
	}
	if items[0].Value != 1 {
		t.Errorf("Value = %v (%T), want 1 (int)", items[0].Value, items[0].Value)
	}
}

func TestParseFile_body_assertion_exists(t *testing.T) {
	col, err := ParseFile("testdata/with_body_assertion_exists.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 1 {
		t.Fatalf("got %d body assertions, want 1", len(items))
	}
	if items[0].Path != "$.token" {
		t.Errorf("Path = %q, want %q", items[0].Path, "$.token")
	}
	if items[0].Operator != "exists" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "exists")
	}
}

func TestParseFile_body_assertion_not_exists(t *testing.T) {
	col, err := ParseFile("testdata/with_body_assertion_not_exists.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 1 {
		t.Fatalf("got %d body assertions, want 1", len(items))
	}
	if items[0].Path != "$.deleted" {
		t.Errorf("Path = %q, want %q", items[0].Path, "$.deleted")
	}
	if items[0].Operator != "not_exists" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "not_exists")
	}
}

func TestParseFile_body_assertion_type(t *testing.T) {
	col, err := ParseFile("testdata/with_body_assertion_type.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 1 {
		t.Fatalf("got %d body assertions, want 1", len(items))
	}
	if items[0].Operator != "type" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "type")
	}
	if items[0].Value != "string" {
		t.Errorf("Value = %v, want %q", items[0].Value, "string")
	}
}

func TestParseFile_body_assertion_multiple(t *testing.T) {
	col, err := ParseFile("testdata/with_body_assertion_multiple.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 3 {
		t.Fatalf("got %d body assertions, want 3", len(items))
	}
	// Verify all three are present (order preserved from YAML)
	paths := []string{items[0].Path, items[1].Path, items[2].Path}
	wantPaths := []string{"$.id", "$.name", "$.token"}
	if !reflect.DeepEqual(paths, wantPaths) {
		t.Errorf("paths = %v, want %v", paths, wantPaths)
	}
}

func TestParseFile_body_and_status_assertion(t *testing.T) {
	col, err := ParseFile("testdata/with_body_and_status_assertion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertions := col.Requests.Items[0].Assertions

	// Status assertion present
	if !reflect.DeepEqual(assertions.Status.Codes, []int{200}) {
		t.Errorf("status codes = %v, want [200]", assertions.Status.Codes)
	}

	// Body assertion present
	if len(assertions.Body.Items) != 1 {
		t.Fatalf("got %d body assertions, want 1", len(assertions.Body.Items))
	}
	if assertions.Body.Items[0].Path != "$.id" {
		t.Errorf("body path = %q, want %q", assertions.Body.Items[0].Path, "$.id")
	}
}

func TestParseFile_no_body_assertions_parses_to_empty(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Body.Items
	if len(items) != 0 {
		t.Errorf("body assertions = %v, want empty", items)
	}
}

func TestParseFile_header_assertion_equals(t *testing.T) {
	col, err := ParseFile("testdata/with_header_assertion_equals.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Headers.Items
	if len(items) != 1 {
		t.Fatalf("got %d header assertions, want 1", len(items))
	}
	if items[0].Name != "Content-Type" {
		t.Errorf("Name = %q, want %q", items[0].Name, "Content-Type")
	}
	if items[0].Operator != "equals" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "equals")
	}
	if items[0].Value != "application/json" {
		t.Errorf("Value = %v, want %q", items[0].Value, "application/json")
	}
}

func TestParseFile_header_assertion_exists(t *testing.T) {
	col, err := ParseFile("testdata/with_header_assertion_exists.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Headers.Items
	if len(items) != 1 {
		t.Fatalf("got %d header assertions, want 1", len(items))
	}
	if items[0].Name != "X-Request-Id" {
		t.Errorf("Name = %q, want %q", items[0].Name, "X-Request-Id")
	}
	if items[0].Operator != "exists" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "exists")
	}
}

func TestParseFile_header_assertion_matches(t *testing.T) {
	col, err := ParseFile("testdata/with_header_assertion_matches.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Headers.Items
	if len(items) != 1 {
		t.Fatalf("got %d header assertions, want 1", len(items))
	}
	if items[0].Name != "Content-Type" {
		t.Errorf("Name = %q, want %q", items[0].Name, "Content-Type")
	}
	if items[0].Operator != "matches" {
		t.Errorf("Operator = %q, want %q", items[0].Operator, "matches")
	}
	if items[0].Value != "^application/.*" {
		t.Errorf("Value = %v, want %q", items[0].Value, "^application/.*")
	}
}

func TestParseFile_timing_assertion(t *testing.T) {
	col, err := ParseFile("testdata/with_timing_assertion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	timing := col.Requests.Items[0].Assertions.Timing
	if timing.MaxDurationMs != 500 {
		t.Errorf("MaxDurationMs = %d, want 500", timing.MaxDurationMs)
	}
}

func TestParseFile_header_and_timing_combined(t *testing.T) {
	col, err := ParseFile("testdata/with_header_and_timing_assertion.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	assertions := col.Requests.Items[0].Assertions

	// Headers
	if len(assertions.Headers.Items) != 2 {
		t.Fatalf("got %d header assertions, want 2", len(assertions.Headers.Items))
	}
	if assertions.Headers.Items[0].Name != "Content-Type" {
		t.Errorf("first header name = %q, want %q", assertions.Headers.Items[0].Name, "Content-Type")
	}
	if assertions.Headers.Items[1].Name != "X-Request-Id" {
		t.Errorf("second header name = %q, want %q", assertions.Headers.Items[1].Name, "X-Request-Id")
	}

	// Timing
	if assertions.Timing.MaxDurationMs != 1000 {
		t.Errorf("MaxDurationMs = %d, want 1000", assertions.Timing.MaxDurationMs)
	}
}

func TestParseFile_no_header_assertions_parses_to_empty(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	items := col.Requests.Items[0].Assertions.Headers.Items
	if len(items) != 0 {
		t.Errorf("header assertions = %v, want empty", items)
	}
}

func TestParseFile_invalid_yaml_preserves_inner_error(t *testing.T) {
	_, err := ParseFile("testdata/invalid.yaml")
	if err == nil {
		t.Fatal("expected error")
	}
	if !errors.Is(err, ErrInvalidYAML) {
		t.Fatalf("error chain missing ErrInvalidYAML: %v", err)
	}

	// Error should be a Structured error wrapping ErrInvalidYAML
	var se *apierrors.Structured
	if !errors.As(err, &se) {
		t.Fatal("expected *apierrors.Structured error")
	}
}

func TestParseFile_StructuredErrors(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		wantSentinel error
		wantFilePath bool
		wantLine     bool
	}{
		{"invalid yaml returns structured error with file path", "testdata/invalid.yaml", ErrInvalidYAML, true, true},
		{"file not found returns structured error with file path", "testdata/nonexistent.yaml", ErrFileNotFound, true, false},
		{"empty collection returns structured error", "testdata/no_name.yaml", ErrEmptyCollection, true, false},
		{"unsupported method returns structured error with file path", "testdata/unsupported_method.yaml", ErrUnsupportedMethod, true, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseFile(tt.file)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantSentinel) {
				t.Fatalf("error chain missing %v: got %v", tt.wantSentinel, err)
			}

			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
			}
			if tt.wantFilePath && se.FilePath == "" {
				t.Error("expected FilePath to be set")
			}
			if tt.wantLine && se.Line <= 0 {
				t.Error("expected Line > 0 for YAML parse errors")
			}
			if !tt.wantLine && se.Line != 0 {
				t.Errorf("expected Line = 0, got %d", se.Line)
			}
		})
	}
}

func TestParseFile_MissingRequiredFields(t *testing.T) {
	// Create a temp file with a request missing the url field
	tmpDir := t.TempDir()

	tests := []struct {
		name        string
		content     string
		wantErr     error
		wantMessage string
	}{
		{
			"missing url returns structured error",
			"name: test\nrequests:\n  - name: no-url\n    request:\n      method: GET\n",
			ErrMissingRequiredField,
			"missing required field 'url'",
		},
		{
			"missing url names the request",
			"name: test\nrequests:\n  - name: my-req\n    request:\n      method: GET\n",
			ErrMissingRequiredField,
			"my-req",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := filepath.Join(tmpDir, tt.name+".yaml")
			if err := os.WriteFile(f, []byte(tt.content), 0o644); err != nil {
				t.Fatal(err)
			}
			_, err := ParseFile(f)
			if err == nil {
				t.Fatal("expected error")
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error chain missing %v: got %v", tt.wantErr, err)
			}
			var se *apierrors.Structured
			if !errors.As(err, &se) {
				t.Fatalf("expected *apierrors.Structured, got %T", err)
			}
			if se.Message == "" || !strings.Contains(se.Message, tt.wantMessage) {
				t.Errorf("Message = %q, want to contain %q", se.Message, tt.wantMessage)
			}
		})
	}
}

func TestParseFile_with_variables(t *testing.T) {
	col, err := ParseFile("testdata/with_variables.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Variables.Values) != 2 {
		t.Fatalf("got %d variables, want 2", len(col.Variables.Values))
	}
	if col.Variables.Values["base_url"] != "https://example.com" {
		t.Errorf("base_url = %q, want %q", col.Variables.Values["base_url"], "https://example.com")
	}
	if col.Variables.Values["api_version"] != "v1" {
		t.Errorf("api_version = %q, want %q", col.Variables.Values["api_version"], "v1")
	}
}

func TestParseFile_extract_single(t *testing.T) {
	col, err := ParseFile("testdata/with_extract_single.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	extract := col.Requests.Items[0].Extract
	if len(extract) != 1 {
		t.Fatalf("got %d extract entries, want 1", len(extract))
	}
	if extract["token"] != "$.data.token" {
		t.Errorf("token = %q, want %q", extract["token"], "$.data.token")
	}
}

func TestParseFile_extract_multiple(t *testing.T) {
	col, err := ParseFile("testdata/with_extract_multiple.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	extract := col.Requests.Items[0].Extract
	if len(extract) != 3 {
		t.Fatalf("got %d extract entries, want 3", len(extract))
	}
	if extract["token"] != "$.data.token" {
		t.Errorf("token = %q, want %q", extract["token"], "$.data.token")
	}
	if extract["user_id"] != "$.data.user.id" {
		t.Errorf("user_id = %q, want %q", extract["user_id"], "$.data.user.id")
	}
	if extract["role"] != "$.data.role" {
		t.Errorf("role = %q, want %q", extract["role"], "$.data.role")
	}
}

func TestParseFile_no_extract_remains_nil(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.Requests.Items[0].Extract != nil {
		t.Errorf("Extract = %v, want nil", col.Requests.Items[0].Extract)
	}
}

func TestParseFile_request_level_variables(t *testing.T) {
	col, err := ParseFile("testdata/with_request_variables.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	vars := col.Requests.Items[0].Variables.Values
	if len(vars) != 2 {
		t.Fatalf("got %d variables, want 2", len(vars))
	}
	if vars["key"] != "val" {
		t.Errorf("key = %q, want %q", vars["key"], "val")
	}
	if vars["another"] != "value2" {
		t.Errorf("another = %q, want %q", vars["another"], "value2")
	}
}

func TestParseFile_request_without_variables(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if col.Requests.Items[0].Variables.Values != nil {
		t.Errorf("Variables = %v, want nil", col.Requests.Items[0].Variables.Values)
	}
}

func TestParseFile_request_with_empty_variables(t *testing.T) {
	col, err := ParseFile("testdata/with_request_variables_empty.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	vars := col.Requests.Items[0].Variables.Values
	if vars == nil {
		t.Fatal("Variables should not be nil for empty map")
	}
	if len(vars) != 0 {
		t.Errorf("got %d variables, want 0", len(vars))
	}
}

func TestParseFile_no_variables(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Variables.Values) != 0 {
		t.Errorf("got %d variables, want 0", len(col.Variables.Values))
	}
}

func TestParseFile_setup_teardown(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		wantSetupLen int
		wantMainLen  int
		wantTdLen    int
	}{
		{"with setup only", "testdata/with_setup.yaml", 1, 2, 0},
		{"with teardown only", "testdata/with_teardown.yaml", 0, 2, 1},
		{"with both", "testdata/with_setup_teardown.yaml", 1, 2, 1},
		{"no setup or teardown", "testdata/minimal.yaml", 0, 1, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(col.Setup.Items) != tt.wantSetupLen {
				t.Errorf("Setup len = %d, want %d", len(col.Setup.Items), tt.wantSetupLen)
			}
			if len(col.Requests.Items) != tt.wantMainLen {
				t.Errorf("Requests len = %d, want %d", len(col.Requests.Items), tt.wantMainLen)
			}
			if len(col.Teardown.Items) != tt.wantTdLen {
				t.Errorf("Teardown len = %d, want %d", len(col.Teardown.Items), tt.wantTdLen)
			}
		})
	}
}

func TestParseFile_required_field(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		setupIdx     int
		wantRequired bool
	}{
		{"required:true", "testdata/with_setup_required.yaml", 0, true},
		{"required not set defaults false", "testdata/with_setup.yaml", 0, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(col.Setup.Items) <= tt.setupIdx {
				t.Fatalf("no setup item at index %d", tt.setupIdx)
			}
			got := col.Setup.Items[tt.setupIdx].IsRequired()
			if got != tt.wantRequired {
				t.Errorf("IsRequired() = %v, want %v", got, tt.wantRequired)
			}
		})
	}
}

func TestParseFile_setup_extract(t *testing.T) {
	col, err := ParseFile("testdata/with_setup_extract.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(col.Setup.Items) != 1 {
		t.Fatalf("got %d setup items, want 1", len(col.Setup.Items))
	}
	extract := col.Setup.Items[0].Extract
	if extract["resource_id"] != "$.data.id" {
		t.Errorf("extract[resource_id] = %q, want %q", extract["resource_id"], "$.data.id")
	}
}

func TestParseFile_setup_teardown_resolution(t *testing.T) {
	tests := []struct {
		name       string
		file       string
		wantErr    error
		checkSetup func(t *testing.T, col *Collection)
		checkTdwn  func(t *testing.T, col *Collection)
	}{
		{
			name: "setup external ref resolves",
			file: "testdata/with_setup_external_ref.yaml",
			checkSetup: func(t *testing.T, col *Collection) {
				t.Helper()
				if len(col.Setup.Items) != 1 {
					t.Fatalf("got %d setup items, want 1", len(col.Setup.Items))
				}
				if col.Setup.Items[0].Request.URL == "" {
					t.Error("setup external ref URL not resolved")
				}
				if col.Setup.Items[0].Name == "" {
					t.Error("setup external ref Name not resolved")
				}
			},
		},
		{
			name: "teardown external ref resolves",
			file: "testdata/with_teardown_external_ref.yaml",
			checkTdwn: func(t *testing.T, col *Collection) {
				t.Helper()
				if len(col.Teardown.Items) != 1 {
					t.Fatalf("got %d teardown items, want 1", len(col.Teardown.Items))
				}
				if col.Teardown.Items[0].Request.URL == "" {
					t.Error("teardown external ref URL not resolved")
				}
			},
		},
		{
			name:    "setup invalid method errors",
			file:    "testdata/with_setup_bad_method.yaml",
			wantErr: ErrUnsupportedMethod,
		},
		{
			name:    "setup missing url returns error",
			file:    "testdata/with_setup_missing_url.yaml",
			wantErr: ErrMissingRequiredField,
		},
		{
			name:    "teardown missing url returns error",
			file:    "testdata/with_teardown_missing_url.yaml",
			wantErr: ErrMissingRequiredField,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.checkSetup != nil {
				tt.checkSetup(t, col)
			}
			if tt.checkTdwn != nil {
				tt.checkTdwn(t, col)
			}
		})
	}
}

func TestSensitiveVarsObjectForm(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantValues    map[string]string
		wantCommands  map[string]CommandVar
		wantSensitive []string
		wantErr       bool
	}{
		{
			name:       "from_command_simple",
			fixture:    "testdata/with_from_command_simple.yaml",
			wantValues: map[string]string{},
			wantCommands: map[string]CommandVar{
				"secret": {Command: "echo secret123"},
			},
			wantSensitive: []string{},
		},
		{
			name:       "from_command_with_sensitive_true",
			fixture:    "testdata/with_from_command_sensitive.yaml",
			wantValues: map[string]string{},
			wantCommands: map[string]CommandVar{
				"secret": {Command: "echo secret123", Sensitive: true},
			},
			wantSensitive: []string{"secret"},
		},
		{
			name:       "from_command_with_sensitive_yes",
			fixture:    "testdata/with_from_command_sensitive_yes.yaml",
			wantValues: map[string]string{},
			wantCommands: map[string]CommandVar{
				"secret": {Command: "echo secret123", Sensitive: true},
			},
			wantSensitive: []string{"secret"},
		},
		{
			name:       "from_command_with_cache",
			fixture:    "testdata/with_from_command_cache.yaml",
			wantValues: map[string]string{},
			wantCommands: map[string]CommandVar{
				"token": {Command: "cat /tmp/token", Cache: 300},
			},
			wantSensitive: []string{},
		},
		{
			name:       "from_command_all_fields",
			fixture:    "testdata/with_from_command_all_fields.yaml",
			wantValues: map[string]string{},
			wantCommands: map[string]CommandVar{
				"vault_secret": {Command: "vault read -field=value secret/data", Sensitive: true, Cache: 600},
			},
			wantSensitive: []string{"vault_secret"},
		},
		{
			name:          "object_form_value_only",
			fixture:       "testdata/with_object_form_value.yaml",
			wantValues:    map[string]string{"host": "example.com"},
			wantCommands:  map[string]CommandVar{},
			wantSensitive: []string{},
		},
		{
			name:    "mixed_scalar_and_from_command",
			fixture: "testdata/with_mixed_scalar_and_command.yaml",
			wantValues: map[string]string{
				"base_url": "https://example.com",
			},
			wantCommands: map[string]CommandVar{
				"secret": {Command: "echo secret123"},
			},
			wantSensitive: []string{},
		},
		{
			name:    "error_value_and_from_command_mutually_exclusive",
			fixture: "testdata/with_value_and_command_conflict.yaml",
			wantErr: true,
		},
		{
			name:    "error_empty_from_command",
			fixture: "testdata/with_empty_from_command.yaml",
			wantErr: true,
		},
		{
			name:          "backward_compat_scalar_values",
			fixture:       "testdata/with_variables.yaml",
			wantValues:    map[string]string{"base_url": "https://example.com", "api_version": "v1"},
			wantCommands:  map[string]CommandVar{},
			wantSensitive: []string{},
		},
		{
			name:          "backward_compat_sensitive_tag",
			fixture:       "testdata/with_sensitive_vars.yaml",
			wantValues:    map[string]string{"base_url": "https://example.com", "api_key": "sk_live_abc123", "password": "secret123"},
			wantCommands:  map[string]CommandVar{},
			wantSensitive: []string{"api_key"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.fixture)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile(%q): %v", tt.fixture, err)
			}
			for k, want := range tt.wantValues {
				got := col.Variables.Values[k]
				if got != want {
					t.Errorf("Values[%q] = %q, want %q", k, got, want)
				}
			}
			if tt.wantCommands != nil {
				if col.Variables.Commands == nil {
					col.Variables.Commands = map[string]CommandVar{}
				}
				if len(col.Variables.Commands) != len(tt.wantCommands) {
					t.Fatalf("Commands len = %d, want %d", len(col.Variables.Commands), len(tt.wantCommands))
				}
				for k, want := range tt.wantCommands {
					got, ok := col.Variables.Commands[k]
					if !ok {
						t.Errorf("Commands[%q] not found", k)
						continue
					}
					if got.Command != want.Command {
						t.Errorf("Commands[%q].Command = %q, want %q", k, got.Command, want.Command)
					}
					if got.Sensitive != want.Sensitive {
						t.Errorf("Commands[%q].Sensitive = %v, want %v", k, got.Sensitive, want.Sensitive)
					}
					if got.Cache != want.Cache {
						t.Errorf("Commands[%q].Cache = %d, want %d", k, got.Cache, want.Cache)
					}
				}
			}
			for _, name := range tt.wantSensitive {
				if !col.Variables.Sensitive.IsSensitive(name) {
					t.Errorf("expected %q to be sensitive", name)
				}
			}
		})
	}
}

func TestParseSensitiveVars(t *testing.T) {
	tests := []struct {
		name          string
		fixture       string
		wantValues    map[string]string
		wantSensitive []string
	}{
		{
			name:    "sensitive_yaml_tag_detected",
			fixture: "testdata/with_sensitive_vars.yaml",
			wantValues: map[string]string{
				"base_url": "https://example.com",
				"api_key":  "sk_live_abc123",
				"password": "secret123",
			},
			wantSensitive: []string{"api_key"},
		},
		{
			name:          "sensitive_tag_preserves_value",
			fixture:       "testdata/with_sensitive_vars.yaml",
			wantValues:    map[string]string{"api_key": "sk_live_abc123"},
			wantSensitive: []string{"api_key"},
		},
		{
			name:    "mixed_sensitive_and_regular_vars",
			fixture: "testdata/with_sensitive_vars.yaml",
			wantValues: map[string]string{
				"base_url": "https://example.com",
				"api_key":  "sk_live_abc123",
				"password": "secret123",
			},
			wantSensitive: []string{"api_key"},
		},
		{
			name:          "no_sensitive_tag_backward_compat",
			fixture:       "testdata/with_variables.yaml",
			wantValues:    map[string]string{"base_url": "https://example.com", "api_version": "v1"},
			wantSensitive: []string{},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.fixture)
			if err != nil {
				t.Fatalf("ParseFile(%q): %v", tt.fixture, err)
			}
			for k, want := range tt.wantValues {
				got := col.Variables.Values[k]
				if got != want {
					t.Errorf("Variables.Values[%q] = %q, want %q", k, got, want)
				}
			}
			for _, name := range tt.wantSensitive {
				if !col.Variables.Sensitive.IsSensitive(name) {
					t.Errorf("expected %q to be sensitive", name)
				}
			}
			// For no_sensitive_tag case, verify sensitive set is empty
			if len(tt.wantSensitive) == 0 {
				for _, name := range col.Variables.Sensitive.Names() {
					_ = variable.IsSensitiveName(name) // just ensure import used
					t.Errorf("unexpected sensitive variable: %q", name)
				}
			}
		})
	}
}

func TestParseFile_ExternalFiles(t *testing.T) {
	tests := []struct {
		name         string
		file         string
		wantExtCount int
		wantExtNames []string // basenames of expected external files
	}{
		{
			name:         "no external refs",
			file:         "testdata/minimal.yaml",
			wantExtCount: 0,
		},
		{
			name:         "single external ref in requests",
			file:         "testdata/with_external_ref.yaml",
			wantExtCount: 1,
			wantExtNames: []string{"get-user.yaml"},
		},
		{
			name:         "multiple external refs across sections",
			file:         "testdata/with_mixed_inline_external.yaml",
			wantExtCount: 1,
			wantExtNames: []string{"get-user.yaml"},
		},
		{
			name:         "setup external ref",
			file:         "testdata/with_setup_external_ref.yaml",
			wantExtCount: 1,
			wantExtNames: []string{"get-user.yaml"},
		},
		{
			name:         "teardown external ref",
			file:         "testdata/with_teardown_external_ref.yaml",
			wantExtCount: 1,
			wantExtNames: []string{"get-user.yaml"},
		},
		{
			name:         "reused external ref counted twice",
			file:         "testdata/with_external_ref_reuse.yaml",
			wantExtCount: 2,
			wantExtNames: []string{"get-user.yaml", "get-user.yaml"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if err != nil {
				t.Fatalf("ParseFile(%q) error: %v", tt.file, err)
			}
			if len(col.ExternalFiles) != tt.wantExtCount {
				t.Fatalf("ExternalFiles count = %d, want %d", len(col.ExternalFiles), tt.wantExtCount)
			}
			for i, wantName := range tt.wantExtNames {
				gotBase := filepath.Base(col.ExternalFiles[i])
				if gotBase != wantName {
					t.Errorf("ExternalFiles[%d] basename = %q, want %q", i, gotBase, wantName)
				}
				// All paths should be absolute
				if !filepath.IsAbs(col.ExternalFiles[i]) {
					t.Errorf("ExternalFiles[%d] = %q, want absolute path", i, col.ExternalFiles[i])
				}
			}
		})
	}
}

func TestParseFile_AuthField(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantAuth string
	}{
		{"auth field parsed on request", "testdata/with_auth.yaml", "admin_token"},
		{"auth field empty when absent", "testdata/minimal.yaml", ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col, err := ParseFile(tc.file)
			if err != nil {
				t.Fatalf("ParseFile(%q) error: %v", tc.file, err)
			}
			if len(col.Requests.Items) == 0 {
				t.Fatalf("ParseFile(%q): got 0 requests", tc.file)
			}
			if got := col.Requests.Items[0].Auth; got != tc.wantAuth {
				t.Errorf("Requests[0].Auth = %q, want %q", got, tc.wantAuth)
			}
		})
	}
}

func TestParseFile_withRetryConfig(t *testing.T) {
	col, err := ParseFile("testdata/with_retry.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if col.Retry == nil {
		t.Fatal("Retry should not be nil")
	}
	if !*col.Retry.Enabled {
		t.Error("Retry.Enabled = false, want true")
	}
	if *col.Retry.MaxAttempts != 3 {
		t.Errorf("Retry.MaxAttempts = %d, want 3", *col.Retry.MaxAttempts)
	}
	if *col.Retry.InitialDelayMs != 500 {
		t.Errorf("Retry.InitialDelayMs = %d, want 500", *col.Retry.InitialDelayMs)
	}
}

func TestParseFile_withRequestRetry(t *testing.T) {
	col, err := ParseFile("testdata/with_request_retry.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 3 {
		t.Fatalf("expected 3 requests, got %d", len(col.Requests.Items))
	}

	// First request: no per-request retry (nil = inherit)
	if col.Requests.Items[0].Retry != nil {
		t.Error("Requests[0].Retry should be nil (inherit collection)")
	}

	// Second request: custom retry
	r1 := col.Requests.Items[1].Retry
	if r1 == nil {
		t.Fatal("Requests[1].Retry should not be nil")
	}
	if !*r1.Enabled {
		t.Error("Requests[1].Retry.Enabled = false, want true")
	}
	if *r1.MaxAttempts != 5 {
		t.Errorf("Requests[1].Retry.MaxAttempts = %d, want 5", *r1.MaxAttempts)
	}
	if *r1.InitialDelayMs != 200 {
		t.Errorf("Requests[1].Retry.InitialDelayMs = %d, want 200", *r1.InitialDelayMs)
	}

	// Third request: retry explicitly disabled
	r2 := col.Requests.Items[2].Retry
	if r2 == nil {
		t.Fatal("Requests[2].Retry should not be nil")
	}
	if *r2.Enabled {
		t.Error("Requests[2].Retry.Enabled = true, want false")
	}
}

func TestParseFile_withSectionRetry(t *testing.T) {
	col, err := ParseFile("testdata/with_section_retry.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}

	// Collection-level retry
	if col.Retry == nil || !*col.Retry.Enabled {
		t.Error("collection Retry.Enabled should be true")
	}

	// Setup section: object form with retry + items
	if col.Setup.Retry == nil {
		t.Fatal("Setup.Retry should not be nil")
	}
	if !*col.Setup.Retry.Enabled {
		t.Error("Setup.Retry.Enabled should be true")
	}
	if *col.Setup.Retry.MaxAttempts != 5 {
		t.Errorf("Setup.Retry.MaxAttempts = %d, want 5", *col.Setup.Retry.MaxAttempts)
	}
	if len(col.Setup.Items) != 1 {
		t.Fatalf("Setup.Items len = %d, want 1", len(col.Setup.Items))
	}
	if col.Setup.Items[0].Name != "Auth setup" {
		t.Errorf("Setup.Items[0].Name = %q, want %q", col.Setup.Items[0].Name, "Auth setup")
	}

	// Requests section: array form (no section retry)
	if col.Requests.Retry != nil {
		t.Error("Requests.Retry should be nil for array form")
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("Requests.Items len = %d, want 1", len(col.Requests.Items))
	}

	// Teardown section: object form with retry disabled
	if col.Teardown.Retry == nil {
		t.Fatal("Teardown.Retry should not be nil")
	}
	if *col.Teardown.Retry.Enabled {
		t.Error("Teardown.Retry.Enabled should be false")
	}
	if len(col.Teardown.Items) != 1 {
		t.Fatalf("Teardown.Items len = %d, want 1", len(col.Teardown.Items))
	}
}

func TestSection_UnmarshalYAML_arrayForm(t *testing.T) {
	data := []byte("- name: A\n  request:\n    method: GET\n    url: https://example.com\n")
	var s Section
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Retry != nil {
		t.Error("Retry should be nil for array form")
	}
	if len(s.Items) != 1 {
		t.Fatalf("Items len = %d, want 1", len(s.Items))
	}
}

func TestSection_UnmarshalYAML_objectForm(t *testing.T) {
	data := []byte("retry:\n  enabled: true\n  max_attempts: 5\nitems:\n  - name: A\n    request:\n      method: GET\n      url: https://example.com\n")
	var s Section
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Retry == nil || !*s.Retry.Enabled {
		t.Error("Retry.Enabled should be true")
	}
	if len(s.Items) != 1 {
		t.Fatalf("Items len = %d, want 1", len(s.Items))
	}
}

func TestSection_UnmarshalYAML_emptyMapping(t *testing.T) {
	data := []byte("retry:\n  enabled: true\n")
	var s Section
	if err := yaml.Unmarshal(data, &s); err != nil {
		t.Fatalf("unmarshal error: %v", err)
	}
	if s.Retry == nil || !*s.Retry.Enabled {
		t.Error("Retry.Enabled should be true")
	}
	if len(s.Items) != 0 {
		t.Errorf("Items len = %d, want 0", len(s.Items))
	}
}

func TestParseFile_retryDefaultsWhenOmitted(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if col.Retry != nil && *col.Retry.Enabled {
		t.Error("Retry.Enabled should be false when omitted")
	}
	if col.Retry != nil && *col.Retry.MaxAttempts != 0 {
		t.Errorf("Retry.MaxAttempts = %d, want 0 (zero value)", *col.Retry.MaxAttempts)
	}
}

func TestParseFile_dataDrivenCSV(t *testing.T) {
	col, err := ParseFile("testdata/data_driven_csv.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
	}
	item := col.Requests.Items[0]
	if item.DataDriven == nil {
		t.Fatal("DataDriven is nil, expected non-nil")
	}
	if item.DataDriven.Source != "./users.csv" {
		t.Errorf("Source = %q, want %q", item.DataDriven.Source, "./users.csv")
	}
	if item.DataDriven.Format != "" {
		t.Errorf("Format = %q, want empty (auto-detect)", item.DataDriven.Format)
	}
}

func TestParseFile_dataDrivenJSON(t *testing.T) {
	col, err := ParseFile("testdata/data_driven_json.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
	}
	item := col.Requests.Items[0]
	if item.DataDriven == nil {
		t.Fatal("DataDriven is nil, expected non-nil")
	}
	if item.DataDriven.Source != "./users.json" {
		t.Errorf("Source = %q, want %q", item.DataDriven.Source, "./users.json")
	}
	if item.DataDriven.Format != "json" {
		t.Errorf("Format = %q, want %q", item.DataDriven.Format, "json")
	}
}

func TestParseFile_dataDrivenParallelConfig(t *testing.T) {
	col, err := ParseFile("testdata/data_driven_parallel.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
	}
	item := col.Requests.Items[0]
	if item.DataDriven == nil {
		t.Fatal("DataDriven is nil, expected non-nil")
	}
	if item.DataDriven.Source != "./users.csv" {
		t.Errorf("Source = %q, want %q", item.DataDriven.Source, "./users.csv")
	}
	if !item.DataDriven.Parallel {
		t.Error("Parallel should be true")
	}
	if item.DataDriven.RateLimitRPS == nil || *item.DataDriven.RateLimitRPS != 100 {
		t.Errorf("RateLimitRPS = %v, want 100", item.DataDriven.RateLimitRPS)
	}
	if item.DataDriven.StoreResults != "failed_only" {
		t.Errorf("StoreResults = %q, want %q", item.DataDriven.StoreResults, "failed_only")
	}
}

func TestParseFile_noDataDrivenIsNil(t *testing.T) {
	col, err := ParseFile("testdata/minimal.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	item := col.Requests.Items[0]
	if item.DataDriven != nil {
		t.Errorf("DataDriven = %v, want nil for collection without data_driven", item.DataDriven)
	}
}

func TestParseFile_graphql(t *testing.T) {
	t.Run("basic graphql query parses", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_basic.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		if len(col.Requests.Items) != 1 {
			t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
		}
		req := col.Requests.Items[0].Request
		if req.Protocol != "graphql" {
			t.Errorf("Protocol = %q, want %q", req.Protocol, "graphql")
		}
		if req.GraphQL == nil {
			t.Fatal("GraphQL is nil, expected non-nil")
		}
		if req.GraphQL.Query == "" {
			t.Error("GraphQL.Query is empty")
		}
	})

	t.Run("graphql with variables parses", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_with_vars.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		gql := col.Requests.Items[0].Request.GraphQL
		if gql == nil {
			t.Fatal("GraphQL is nil")
		}
		if gql.Variables == nil {
			t.Fatal("Variables is nil")
		}
		if gql.Variables["id"] != "123" {
			t.Errorf("Variables[id] = %v, want %q", gql.Variables["id"], "123")
		}
	})

	t.Run("invalid protocol value", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_invalid_protocol.yaml")
		if err == nil {
			t.Fatal("expected error for invalid protocol")
		}
		if !errors.Is(err, ErrUnsupportedProtocol) {
			t.Errorf("expected ErrUnsupportedProtocol, got %v", err)
		}
	})

	t.Run("graphql without query field", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_no_query.yaml")
		if err == nil {
			t.Fatal("expected error for graphql without query")
		}
		if !errors.Is(err, ErrMissingRequiredField) {
			t.Errorf("expected ErrMissingRequiredField, got %v", err)
		}
	})

	t.Run("default protocol is empty", func(t *testing.T) {
		col, err := ParseFile("testdata/minimal.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		if col.Requests.Items[0].Request.Protocol != "" {
			t.Errorf("expected empty protocol, got %q", col.Requests.Items[0].Request.Protocol)
		}
	})

	t.Run("graphql error_handling parses", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_error_handling.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		gql := col.Requests.Items[0].Request.GraphQL
		if gql == nil {
			t.Fatal("GraphQL is nil")
		}
		if gql.ErrorHandling != "warn" {
			t.Errorf("ErrorHandling = %q, want %q", gql.ErrorHandling, "warn")
		}
	})

	t.Run("graphql invalid error_handling rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_invalid_error_handling.yaml")
		if err == nil {
			t.Fatal("expected error for invalid error_handling value")
		}
		if !errors.Is(err, ErrInvalidFieldValue) {
			t.Errorf("expected ErrInvalidFieldValue, got %v", err)
		}
	})

	t.Run("graphql error_handling ignore is valid", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_error_handling_ignore.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		gql := col.Requests.Items[0].Request.GraphQL
		if gql == nil {
			t.Fatal("GraphQL is nil")
		}
		if gql.ErrorHandling != "ignore" {
			t.Errorf("ErrorHandling = %q, want %q", gql.ErrorHandling, "ignore")
		}
	})

	t.Run("graphql query_file loads external query", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_query_file.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		gql := col.Requests.Items[0].Request.GraphQL
		if gql == nil {
			t.Fatal("GraphQL is nil")
		}
		if !strings.Contains(gql.Query, "query GetUser") {
			t.Errorf("Query missing loaded content: %q", gql.Query)
		}
		if len(col.ExternalFiles) == 0 {
			t.Error("ExternalFiles not populated from query_file")
		}
	})

	t.Run("graphql fragments concatenated in dependency order", func(t *testing.T) {
		col, err := ParseFile("testdata/graphql_fragments.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		q := col.Requests.Items[0].Request.GraphQL.Query
		userIdx := strings.Index(q, "fragment UserFields")
		postIdx := strings.Index(q, "fragment PostFields")
		queryIdx := strings.Index(q, "query GetUser")
		if userIdx < 0 || postIdx < 0 || queryIdx < 0 {
			t.Fatalf("concatenated query missing parts:\n%s", q)
		}
		if userIdx > postIdx {
			t.Errorf("UserFields should come before PostFields (dep order); got UserFields@%d PostFields@%d", userIdx, postIdx)
		}
		if postIdx > queryIdx {
			t.Errorf("fragments should come before query; got PostFields@%d query@%d", postIdx, queryIdx)
		}
		if len(col.ExternalFiles) != 3 {
			t.Errorf("ExternalFiles count = %d, want 3", len(col.ExternalFiles))
		}
	})

	t.Run("graphql circular fragment dependency returns ErrFragmentCycle", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_fragment_cycle.yaml")
		if err == nil {
			t.Fatal("expected error for circular fragments")
		}
		if !errors.Is(err, graphqlfiles.ErrFragmentCycle) {
			t.Errorf("expected ErrFragmentCycle, got %v", err)
		}
	})

	t.Run("graphql missing query_file returns clear error", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_missing_query_file.yaml")
		if err == nil {
			t.Fatal("expected error for missing query_file")
		}
		if !errors.Is(err, graphqlfiles.ErrQueryFileNotFound) {
			t.Errorf("expected ErrQueryFileNotFound, got %v", err)
		}
		if !strings.Contains(err.Error(), "does_not_exist.graphql") {
			t.Errorf("error missing expected path: %v", err)
		}
	})

	t.Run("graphql query and query_file are mutually exclusive", func(t *testing.T) {
		_, err := ParseFile("testdata/graphql_query_and_file.yaml")
		if err == nil {
			t.Fatal("expected error for both query and query_file")
		}
		if !errors.Is(err, graphqlfiles.ErrQueryMutuallyExclusive) {
			t.Errorf("expected ErrQueryMutuallyExclusive, got %v", err)
		}
	})
}

func TestParseFile_websocket(t *testing.T) {
	t.Run("basic websocket parses", func(t *testing.T) {
		col, err := ParseFile("testdata/websocket_basic.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		if len(col.Requests.Items) != 1 {
			t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
		}
		req := col.Requests.Items[0].Request
		if req.Protocol != "websocket" {
			t.Errorf("Protocol = %q, want %q", req.Protocol, "websocket")
		}
		if req.Method != "WS" {
			t.Errorf("Method = %q, want %q", req.Method, "WS")
		}
		if req.WebSocket == nil {
			t.Fatal("WebSocket is nil, expected non-nil")
		}
		if len(req.WebSocket.Steps) != 1 {
			t.Fatalf("Steps = %d, want 1", len(req.WebSocket.Steps))
		}
		step := req.WebSocket.Steps[0]
		if step.Action != "send" {
			t.Errorf("Action = %q, want %q", step.Action, "send")
		}
		if step.Message == nil || step.Message["type"] != "ping" {
			t.Errorf("Message = %v, want {type: ping}", step.Message)
		}
	})

	t.Run("full lifecycle parses", func(t *testing.T) {
		col, err := ParseFile("testdata/websocket_full_lifecycle.yaml")
		if err != nil {
			t.Fatalf("ParseFile error: %v", err)
		}
		req := col.Requests.Items[0].Request
		if req.WebSocket == nil {
			t.Fatal("WebSocket is nil")
		}
		steps := req.WebSocket.Steps
		if len(steps) != 6 {
			t.Fatalf("Steps = %d, want 6", len(steps))
		}
		// send with message map
		if steps[0].Action != "send" || steps[0].Message["channel"] != "orders" {
			t.Errorf("step 0 = %+v", steps[0])
		}
		// expect with JSONPath assertions and extract
		if steps[1].Action != "expect" {
			t.Errorf("step 1 action = %q", steps[1].Action)
		}
		if steps[1].TimeoutMs != 2000 {
			t.Errorf("step 1 timeout = %d, want 2000", steps[1].TimeoutMs)
		}
		if len(steps[1].ExpectAssertions.Items) != 2 {
			t.Errorf("expect assertions = %d, want 2", len(steps[1].ExpectAssertions.Items))
		}
		if steps[1].Extract["sid"] != "$.session_id" {
			t.Errorf("extract sid = %q", steps[1].Extract["sid"])
		}
		// wait
		if steps[2].Action != "wait" || steps[2].DurationMs != 50 {
			t.Errorf("step 2 = %+v", steps[2])
		}
		// send raw
		if steps[3].Action != "send" || steps[3].MessageRaw != "ping" {
			t.Errorf("step 3 = %+v", steps[3])
		}
		// second expect
		if steps[4].Action != "expect" || len(steps[4].ExpectAssertions.Items) != 1 {
			t.Errorf("step 4 = %+v", steps[4])
		}
		// close
		if steps[5].Action != "close" || steps[5].Code != 1000 || steps[5].Reason != "done" {
			t.Errorf("step 5 = %+v", steps[5])
		}
	})

	t.Run("invalid action rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_invalid_action.yaml")
		if err == nil {
			t.Fatal("expected error for invalid action")
		}
		if !errors.Is(err, ErrInvalidFieldValue) {
			t.Errorf("expected ErrInvalidFieldValue, got %v", err)
		}
	})

	t.Run("empty steps rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_no_steps.yaml")
		if err == nil {
			t.Fatal("expected error for empty steps")
		}
		if !errors.Is(err, ErrMissingRequiredField) {
			t.Errorf("expected ErrMissingRequiredField, got %v", err)
		}
	})

	t.Run("missing websocket block rejected", func(t *testing.T) {
		_, err := ParseFile("testdata/websocket_no_config.yaml")
		if err == nil {
			t.Fatal("expected error for missing websocket block")
		}
		if !errors.Is(err, ErrMissingRequiredField) {
			t.Errorf("expected ErrMissingRequiredField, got %v", err)
		}
	})
}

func TestParseFile_WebSocketAnyOf(t *testing.T) {
	col, err := ParseFile("testdata/websocket_any_of.yaml")
	if err != nil {
		t.Fatal(err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("requests = %d, want 1", len(col.Requests.Items))
	}
	step := col.Requests.Items[0].Request.WebSocket.Steps[0]
	if len(step.AnyOf) != 2 {
		t.Fatalf("AnyOf = %d, want 2", len(step.AnyOf))
	}
	if len(step.AnyOf[0].Items) == 0 {
		t.Fatal("AnyOf[0] has no items")
	}
	if step.AnyOf[0].Items[0].Value != "success" {
		t.Errorf("AnyOf[0].Items[0].Value = %v, want success", step.AnyOf[0].Items[0].Value)
	}
	if step.AnyOf[0].Items[0].Operator != "equals" {
		t.Errorf("AnyOf[0].Items[0].Operator = %q, want equals", step.AnyOf[0].Items[0].Operator)
	}
	if len(step.AnyOf[1].Items) == 0 {
		t.Fatal("AnyOf[1] has no items")
	}
	if step.AnyOf[1].Items[0].Value != "error" {
		t.Errorf("AnyOf[1].Items[0].Value = %v, want error", step.AnyOf[1].Items[0].Value)
	}
}

func TestParseFile_WebSocketCount(t *testing.T) {
	col, err := ParseFile("testdata/websocket_count.yaml")
	if err != nil {
		t.Fatal(err)
	}
	step := col.Requests.Items[0].Request.WebSocket.Steps[0]
	if step.Count != 5 {
		t.Errorf("Count = %d, want 5", step.Count)
	}
	if step.Extract["notifications"] != "$.data" {
		t.Errorf("Extract[notifications] = %q, want $.data", step.Extract["notifications"])
	}
}

func TestParseFile_WebSocketMessageTemplate(t *testing.T) {
	col, err := ParseFile("testdata/websocket_template.yaml")
	if err != nil {
		t.Fatal(err)
	}
	step := col.Requests.Items[0].Request.WebSocket.Steps[0]
	if step.MessageRawTemplate == "" {
		t.Fatal("MessageRawTemplate not loaded")
	}
	if !strings.Contains(step.MessageRawTemplate, "{{channel}}") {
		t.Errorf("template should preserve placeholders, got: %q", step.MessageRawTemplate)
	}
	if step.StepVariables["channel"] != "news" {
		t.Errorf("StepVariables[channel] = %q, want news", step.StepVariables["channel"])
	}
	found := false
	for _, p := range col.ExternalFiles {
		if strings.HasSuffix(p, "websocket_template.json") {
			found = true
		}
	}
	if !found {
		t.Errorf("template path missing from ExternalFiles, got: %v", col.ExternalFiles)
	}
}

func TestParseFile_WebSocketSendMutualExclusion(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		wantErr string
	}{
		{"message_and_template", "testdata/websocket_conflict_msg.yaml", "mutually exclusive"},
		{"message_and_raw", "testdata/websocket_conflict_msg_raw.yaml", "mutually exclusive"},
		{"raw_and_template", "testdata/websocket_conflict_raw_template.yaml", "mutually exclusive"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, err := ParseFile(tc.file)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantErr) {
				t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantErr)
			}
		})
	}
}

func TestParseFile_WebSocketExpectMutualExclusion(t *testing.T) {
	_, err := ParseFile("testdata/websocket_conflict_expect.yaml")
	if err == nil {
		t.Fatal("expected error for message + any_of conflict, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
}

func TestParseFile_WebSocketVariablesWithoutTemplate(t *testing.T) {
	_, err := ParseFile("testdata/websocket_vars_no_template.yaml")
	if err == nil {
		t.Fatal("expected error for variables without message_template, got nil")
	}
	if !strings.Contains(err.Error(), "variables") {
		t.Errorf("error = %q, want to mention 'variables'", err.Error())
	}
}

func TestParse_websocketReconnectConfig(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantErr  bool
		wantMsg  string
		assertFn func(*testing.T, *Collection)
	}{
		{
			name:    "reconnect_enabled_defaults_applied",
			file:    "testdata/websocket_reconnect.yaml",
			wantErr: false,
			assertFn: func(t *testing.T, col *Collection) {
				t.Helper()
				req := col.Requests.Items[0].Request
				if req.WebSocket == nil {
					t.Fatal("WebSocket is nil")
				}
				if req.WebSocket.Reconnect == nil {
					t.Fatal("Reconnect is nil")
				}
				if !req.WebSocket.Reconnect.Enabled {
					t.Error("Reconnect.Enabled = false, want true")
				}
				if req.WebSocket.Reconnect.MaxAttempts != 3 {
					t.Errorf("MaxAttempts = %d, want 3", req.WebSocket.Reconnect.MaxAttempts)
				}
				if req.WebSocket.Reconnect.Backoff != "exponential" {
					t.Errorf("Backoff = %q, want exponential", req.WebSocket.Reconnect.Backoff)
				}
			},
		},
		{
			name:    "heartbeat_enabled_with_message",
			file:    "testdata/websocket_heartbeat.yaml",
			wantErr: false,
			assertFn: func(t *testing.T, col *Collection) {
				t.Helper()
				req := col.Requests.Items[0].Request
				if req.WebSocket == nil {
					t.Fatal("WebSocket is nil")
				}
				if req.WebSocket.Heartbeat == nil {
					t.Fatal("Heartbeat is nil")
				}
				if !req.WebSocket.Heartbeat.Enabled {
					t.Error("Heartbeat.Enabled = false, want true")
				}
				if req.WebSocket.Heartbeat.IntervalMs != 5000 {
					t.Errorf("IntervalMs = %d, want 5000", req.WebSocket.Heartbeat.IntervalMs)
				}
				if req.WebSocket.Heartbeat.Message == nil {
					t.Error("Heartbeat.Message is nil, want non-nil")
				}
			},
		},
		{
			name:    "reconnect_bad_backoff_rejected",
			file:    "testdata/websocket_reconnect_invalid_backoff.yaml",
			wantErr: true,
			wantMsg: "unsupported reconnect.backoff",
		},
		{
			name:    "heartbeat_bad_interval_rejected",
			file:    "testdata/websocket_heartbeat_bad_interval.yaml",
			wantErr: true,
			wantMsg: "heartbeat.interval_ms must be > 0",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col, err := ParseFile(tc.file)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.wantMsg != "" && !strings.Contains(err.Error(), tc.wantMsg) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.wantMsg)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tc.assertFn != nil {
				tc.assertFn(t, col)
			}
		})
	}
}

func TestParse_websocketAutoDetectURLScheme(t *testing.T) {
	tests := []struct {
		name      string
		file      string
		wantProto string
		wantErr   bool
	}{
		{"ws_scheme_sets_protocol", "testdata/websocket_autodetect_ws.yaml", "websocket", false},
		{"wss_scheme_sets_protocol", "testdata/websocket_autodetect_wss.yaml", "websocket", false},
		{"uppercase_WSS_scheme", "testdata/websocket_autodetect_uppercase.yaml", "websocket", false},
		{"ws_scheme_still_requires_steps", "testdata/websocket_autodetect_missing_steps.yaml", "", true},
		{"explicit_http_with_ws_url_is_honoured", "testdata/websocket_autodetect_explicit_http.yaml", "http", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col, err := ParseFile(tc.file)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(col.Requests.Items) == 0 {
				t.Fatal("no requests parsed")
			}
			got := col.Requests.Items[0].Request.Protocol
			if got != tc.wantProto {
				t.Errorf("Protocol = %q, want %q", got, tc.wantProto)
			}
		})
	}
}

func TestParseFile_rateLimitRPS(t *testing.T) {
	tests := []struct {
		name     string
		fixture  string
		wantRPS  int
		wantErr  bool
		wantLine int
	}{
		{"parses positive rate_limit_rps", "testdata/rate_limit_global.yaml", 5, false, 0},
		{"unset defaults to 0", "testdata/minimal.yaml", 0, false, 0},
		{"negative rejected with line number", "testdata/rate_limit_negative.yaml", 0, true, 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.fixture)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				var se *apierrors.Structured
				if !errors.As(err, &se) {
					t.Fatalf("expected *apierrors.Structured, got %T: %v", err, err)
				}
				if se.Line != tt.wantLine {
					t.Errorf("Line = %d, want %d", se.Line, tt.wantLine)
				}
				if !errors.Is(err, ErrInvalidFieldValue) {
					t.Errorf("expected ErrInvalidFieldValue wrapped, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if col.RateLimitRPS != tt.wantRPS {
				t.Errorf("RateLimitRPS = %d, want %d", col.RateLimitRPS, tt.wantRPS)
			}
		})
	}
}

func TestParseFile_body_file_text(t *testing.T) {
	col, err := ParseFile("testdata/with_body_file.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := col.Requests.Items[0].Request
	if req.BodyFile != "bodies/payload.json" {
		t.Errorf("BodyFile = %q, want bodies/payload.json", req.BodyFile)
	}
	if !strings.Contains(req.BodyFileContent, "{{user_id}}") {
		t.Errorf("BodyFileContent should preserve placeholders for runtime interpolation, got %q", req.BodyFileContent)
	}
	if !strings.Contains(req.BodyFileContent, `"action":"create"`) {
		t.Errorf("BodyFileContent missing expected literal, got %q", req.BodyFileContent)
	}
	if req.BodyFileContentType != "application/json" {
		t.Errorf("BodyFileContentType = %q, want application/json", req.BodyFileContentType)
	}
	if req.Body != nil {
		t.Errorf("Body must stay nil when body_file is used; got %#v", req.Body)
	}
	// ExternalFiles should include the loaded body path so watch mode reruns
	// when the file changes.
	var found bool
	for _, p := range col.ExternalFiles {
		if strings.HasSuffix(p, "payload.json") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("ExternalFiles missing body_file path: %v", col.ExternalFiles)
	}
}

func TestParseFile_body_binary_file(t *testing.T) {
	col, err := ParseFile("testdata/with_body_binary_file.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	req := col.Requests.Items[0].Request
	if req.BodyBinaryFile != "bodies/payload.bin" {
		t.Errorf("BodyBinaryFile = %q", req.BodyBinaryFile)
	}
	wantBytes := []byte{0x00, 0x01, 0x02, 0xDE, 0xAD, 0xBE, 0xEF, 0xFF}
	if !reflect.DeepEqual(req.BodyBinaryFileContent, wantBytes) {
		t.Errorf("BodyBinaryFileContent = %x, want %x", req.BodyBinaryFileContent, wantBytes)
	}
	if req.BodyFileContentType == "" {
		t.Errorf("BodyFileContentType empty; binary variant should fall back to application/octet-stream")
	}
	if req.BodyFileContentType != "application/octet-stream" {
		t.Errorf("BodyFileContentType = %q, want application/octet-stream for .bin", req.BodyFileContentType)
	}
}

func TestParseFile_body_and_body_file_mutually_exclusive(t *testing.T) {
	_, err := ParseFile("testdata/body_and_body_file_conflict.yaml")
	if err == nil {
		t.Fatal("expected error for body + body_file conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q, want to contain 'mutually exclusive'", err.Error())
	}
	if !errors.Is(err, ErrInvalidFieldValue) {
		t.Errorf("expected ErrInvalidFieldValue wrapped, got %v", err)
	}
}

func TestParseFile_body_file_and_binary_mutually_exclusive(t *testing.T) {
	_, err := ParseFile("testdata/body_file_and_binary_conflict.yaml")
	if err == nil {
		t.Fatal("expected error for body_file + body_binary_file conflict")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("error = %q", err.Error())
	}
}

func TestParseFile_body_file_missing(t *testing.T) {
	_, err := ParseFile("testdata/body_file_missing.yaml")
	if err == nil {
		t.Fatal("expected error for missing body_file")
	}
	if !errors.Is(err, httpbody.ErrBodyFileNotFound) {
		t.Errorf("expected ErrBodyFileNotFound, got %v", err)
	}
	if !strings.Contains(err.Error(), "does_not_exist.json") {
		t.Errorf("error should name the missing file, got %q", err.Error())
	}
}

// TestParser_DuplicateNameRejection verifies that ParseFile rejects collections
// whose main phase has two or more items sharing the same name. The check
// applies to fully-resolved collections (including items spliced via include:)
// but not to setup/teardown items that share a name with a main item.
func TestParser_DuplicateNameRejection(t *testing.T) {
	tests := []struct {
		name         string
		makeFiles    func(t *testing.T, dir string) string // returns path to root collection
		wantErr      bool
		wantErrIs    error
		wantMsgParts []string // substrings that must appear in err.Error()
	}{
		{
			name: "same-file duplicate rejected",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "dup.yaml")
				body := `name: Dup
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/a"}
  - name: Get user
    request: {method: GET, url: "https://example.com/b"}
`
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr:      true,
			wantErrIs:    ErrDuplicateRequestName,
			wantMsgParts: []string{`duplicate request name "Get user"`, `dup.yaml:3`, `dup.yaml:5`},
		},
		{
			name: "cross-include duplicate rejected",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				// child.yaml has a main request named "Get user"
				childPath := filepath.Join(dir, "child.yaml")
				childBody := `name: Child
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/child"}
`
				if err := os.WriteFile(childPath, []byte(childBody), 0o600); err != nil {
					t.Fatal(err)
				}
				// parent.yaml also has a main request named "Get user" and includes child.yaml
				parentPath := filepath.Join(dir, "parent.yaml")
				parentBody := `name: Parent
include:
  - ./child.yaml
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/parent"}
`
				if err := os.WriteFile(parentPath, []byte(parentBody), 0o600); err != nil {
					t.Fatal(err)
				}
				return parentPath
			},
			wantErr:      true,
			wantErrIs:    ErrDuplicateRequestName,
			wantMsgParts: []string{`duplicate request name "Get user"`, `child.yaml`, `parent.yaml`},
		},
		{
			name: "setup and main with same name is allowed",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "setup_main_same.yaml")
				body := `name: SetupAndMain
setup:
  - name: Get token
    request: {method: GET, url: "https://example.com/token"}
requests:
  - name: Get token
    request: {method: GET, url: "https://example.com/data"}
`
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: false,
		},
		{
			name: "teardown and main with same name is allowed",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "teardown_main_same.yaml")
				body := `name: TeardownAndMain
requests:
  - name: Create user
    request: {method: POST, url: "https://example.com/users"}
teardown:
  - name: Create user
    request: {method: DELETE, url: "https://example.com/users/1"}
`
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: false,
		},
		{
			name: "no duplicates is accepted",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "no_dup.yaml")
				body := `name: NoDup
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/a"}
  - name: Update user
    request: {method: PUT, url: "https://example.com/b"}
`
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: false,
		},
		{
			name: "single main request is accepted",
			makeFiles: func(t *testing.T, dir string) string {
				t.Helper()
				p := filepath.Join(dir, "single.yaml")
				body := `name: Single
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/a"}
`
				if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
					t.Fatal(err)
				}
				return p
			},
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			root := tt.makeFiles(t, dir)
			_, err := ParseFile(root)
			if tt.wantErr {
				if err == nil {
					t.Fatal("want error, got nil")
				}
				if tt.wantErrIs != nil && !errors.Is(err, tt.wantErrIs) {
					t.Fatalf("got %v, want errors.Is %v", err, tt.wantErrIs)
				}
				for _, part := range tt.wantMsgParts {
					if !strings.Contains(err.Error(), part) {
						t.Errorf("err missing substring %q:\n  err: %s", part, err)
					}
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}

// stripSourceLocations zeros SourceFile, SourceLine, and Slug across every
// request item in c, making it amenable to DeepEqual against fixtures that
// predate the M6-002 source-location fields and M9-001 slug field. Use only
// in tests whose intent is content equality, not line-sensitive or
// slug-sensitive equality.
func stripSourceLocations(c *Collection) {
	if c == nil {
		return
	}
	for _, s := range []*[]RequestItem{&c.Setup.Items, &c.Requests.Items, &c.Teardown.Items} {
		for i := range *s {
			(*s)[i].SourceFile = ""
			(*s)[i].SourceLine = 0
			(*s)[i].Slug = ""
		}
	}
}

// TestParser_SlugEmptyRejection verifies that ParseFile rejects requests whose
// names slugify to empty strings, covering all three phases.
func TestParser_SlugEmptyRejection(t *testing.T) {
	tests := []struct {
		name     string
		yaml     string
		wantErr  bool
		errPhase string // informational
	}{
		{
			name: "main item with valid name accepted",
			yaml: `name: ok
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com"}`,
			wantErr: false,
		},
		{
			name: "main item with punctuation-only name rejected",
			yaml: `name: bad
requests:
  - name: "!!!"
    request: {method: GET, url: "https://example.com"}`,
			wantErr:  true,
			errPhase: "main",
		},
		{
			name: "main item with all-CJK name rejected (no ASCII letters)",
			yaml: `name: bad
requests:
  - name: "世界"
    request: {method: GET, url: "https://example.com"}`,
			wantErr:  true,
			errPhase: "main",
		},
		{
			name: "setup item with empty-slug name rejected",
			yaml: `name: bad
setup:
  - name: "---"
    request: {method: GET, url: "https://example.com"}
requests:
  - name: ok
    request: {method: GET, url: "https://example.com"}`,
			wantErr:  true,
			errPhase: "setup",
		},
		{
			name: "teardown item with empty-slug name rejected",
			yaml: `name: bad
requests:
  - name: ok
    request: {method: GET, url: "https://example.com"}
teardown:
  - name: "   "
    request: {method: GET, url: "https://example.com"}`,
			wantErr:  true,
			errPhase: "teardown",
		},
		{
			name: "unicode that normalizes to ascii is accepted",
			yaml: `name: ok
requests:
  - name: "Héllo"
    request: {method: GET, url: "https://example.com"}`,
			wantErr: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			p := filepath.Join(dir, "c.yaml")
			if err := os.WriteFile(p, []byte(tt.yaml), 0o600); err != nil {
				t.Fatal(err)
			}
			_, err := ParseFile(p)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseFile error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr && err != nil && !errors.Is(err, ErrSlugEmpty) {
				t.Errorf("ParseFile error = %v, expected wraps ErrSlugEmpty", err)
			}
		})
	}
}

// TestParser_SlugPopulated verifies that ParseFile populates the Slug field on
// every RequestItem across setup, main, and teardown phases.
func TestParser_SlugPopulated(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "c.yaml")
	body := `name: ok
setup:
  - name: Login
    request: {method: POST, url: "https://example.com/login"}
requests:
  - name: Get user
    request: {method: GET, url: "https://example.com/u"}
  - name: Create Post!
    request: {method: POST, url: "https://example.com/p"}
teardown:
  - name: Logout
    request: {method: POST, url: "https://example.com/logout"}`
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	col, err := ParseFile(p)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	want := map[string]string{
		"Login":        "login",
		"Get user":     "get-user",
		"Create Post!": "create-post",
		"Logout":       "logout",
	}
	for _, items := range [][]RequestItem{col.Setup.Items, col.Requests.Items, col.Teardown.Items} {
		for _, it := range items {
			if w, ok := want[it.Name]; ok {
				if it.Slug != w {
					t.Errorf("item %q: Slug = %q, want %q", it.Name, it.Slug, w)
				}
			}
		}
	}
}

// TestParseFile_if_field_parsed verifies that an `if:` field on a request item
// is parsed as a verbatim CEL expression string.
func TestParseFile_if_field_parsed(t *testing.T) {
	tests := []struct {
		name    string
		file    string
		itemIdx int
		wantIf  string
	}{
		{
			name:    "if scalar bound",
			file:    "testdata/with_if.yaml",
			itemIdx: 1,
			wantIf:  `previous.body.status == "pending"`,
		},
		{
			name:    "item without if has empty string",
			file:    "testdata/with_if.yaml",
			itemIdx: 0,
			wantIf:  "",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if err != nil {
				t.Fatalf("ParseFile: %v", err)
			}
			if got := col.Requests.Items[tt.itemIdx].If; got != tt.wantIf {
				t.Errorf("If = %q, want %q", got, tt.wantIf)
			}
		})
	}
}

// TestParseFile_depends_on_parsed verifies that a `depends_on:` list on a
// request item is parsed into a []string slice.
func TestParseFile_depends_on_parsed(t *testing.T) {
	col, err := ParseFile("testdata/with_if.yaml")
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}

	// Item at index 1 has depends_on: [seed]
	got := col.Requests.Items[1].DependsOn
	want := []string{"seed"}
	if len(got) != len(want) || (len(got) > 0 && got[0] != want[0]) {
		t.Errorf("DependsOn = %v, want %v", got, want)
	}

	// Item at index 2 has depends_on: [conditional-request]
	got2 := col.Requests.Items[2].DependsOn
	want2 := []string{"conditional-request"}
	if len(got2) != len(want2) || (len(got2) > 0 && got2[0] != want2[0]) {
		t.Errorf("DependsOn (item 2) = %v, want %v", got2, want2)
	}
}

// TestParseFile_depends_on_unknown_name_returns_error verifies that a
// depends_on: entry referencing an unknown request name is rejected at load
// time with ErrUnknownDependsOn.
func TestParseFile_depends_on_unknown_name_returns_error(t *testing.T) {
	_, err := ParseFile("testdata/with_unknown_dep.yaml")
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !errors.Is(err, ErrUnknownDependsOn) {
		t.Fatalf("want ErrUnknownDependsOn in chain, got %v", err)
	}
}
