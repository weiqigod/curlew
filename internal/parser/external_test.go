package parser

import (
	"errors"
	"reflect"
	"testing"
)

func TestParseExternalFile(t *testing.T) {
	tests := []struct {
		name     string
		file     string
		wantErr  error
		wantName string
		wantURL  string
	}{
		{"valid minimal external request", "testdata/requests/get-user.yaml", nil, "Get User", "https://example.com/users/1"},
		{"external request with assertions and extract", "testdata/requests/create-user.yaml", nil, "Create User", "https://example.com/users"},
		{"external file not found", "testdata/requests/nonexistent.yaml", ErrExternalFileNotFound, "", ""},
		{"external file invalid YAML", "testdata/requests/invalid.yaml", ErrInvalidYAML, "", ""},
		{"external request missing url", "testdata/requests/missing-url.yaml", ErrMissingRequiredField, "", ""},
		{"external request method defaults handled", "testdata/requests/no-method.yaml", nil, "No Method", "https://example.com/test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ext, err := parseExternalFile(tt.file)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if ext.Name != tt.wantName {
				t.Errorf("Name = %q, want %q", ext.Name, tt.wantName)
			}
			if ext.Request.URL != tt.wantURL {
				t.Errorf("URL = %q, want %q", ext.Request.URL, tt.wantURL)
			}
		})
	}
}

func TestParseExternalFile_preserves_extract(t *testing.T) {
	ext, err := parseExternalFile("testdata/requests/create-user.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ext.Extract == nil || ext.Extract["user_id"] != "$.id" {
		t.Errorf("Extract = %v, want map with user_id=$.id", ext.Extract)
	}
}

func TestParseExternalFile_preserves_assertions(t *testing.T) {
	ext, err := parseExternalFile("testdata/requests/create-user.yaml")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ext.Assertions.Status.Codes) != 1 || ext.Assertions.Status.Codes[0] != 201 {
		t.Errorf("Assertions.Status.Codes = %v, want [201]", ext.Assertions.Status.Codes)
	}
}

func TestParseFile_ExternalReferences(t *testing.T) {
	tests := []struct {
		name        string
		file        string
		wantErr     error
		wantCount   int
		checkResult func(t *testing.T, col *Collection)
	}{
		{
			"external reference loads and merges",
			"testdata/with_external_ref.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Name != "Get User" {
					t.Errorf("Name = %q, want %q", col.Requests.Items[0].Name, "Get User")
				}
				if col.Requests.Items[0].Request.URL != "https://example.com/users/1" {
					t.Errorf("URL = %q, want %q", col.Requests.Items[0].Request.URL, "https://example.com/users/1")
				}
			},
		},
		{
			"external reference relative to collection dir",
			"testdata/subdir/with_relative_ref.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Name != "Get User" {
					t.Errorf("Name = %q, want %q", col.Requests.Items[0].Name, "Get User")
				}
			},
		},
		{
			"external reference not found",
			"testdata/with_external_ref_not_found.yaml",
			ErrExternalFileNotFound, 0, nil,
		},
		{
			"circular self-reference detected",
			"testdata/with_external_self_ref.yaml",
			ErrCircularFileReference, 0, nil,
		},
		{
			"external with variable overrides",
			"testdata/with_external_ref_overrides.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				want := map[string]string{"user_id": "42", "token": "abc123"}
				if !reflect.DeepEqual(col.Requests.Items[0].Variables.Values, want) {
					t.Errorf("Variables = %v, want %v", col.Requests.Items[0].Variables.Values, want)
				}
			},
		},
		{
			"external with extract block",
			"testdata/with_external_ref_extract.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Extract == nil || col.Requests.Items[0].Extract["user_id"] != "$.id" {
					t.Errorf("Extract = %v, want map with user_id=$.id", col.Requests.Items[0].Extract)
				}
			},
		},
		{
			"external behaves identically to inline",
			"testdata/with_external_ref.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				ri := col.Requests.Items[0]
				// Path should be cleared after resolution
				if ri.Path != "" {
					t.Errorf("Path = %q, want empty after resolution", ri.Path)
				}
				if ri.Request.Method != "GET" {
					t.Errorf("Method = %q, want GET", ri.Request.Method)
				}
			},
		},
		{
			"external reference name override",
			"testdata/with_external_ref_name_override.yaml",
			nil, 1,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Name != "Custom Name" {
					t.Errorf("Name = %q, want %q", col.Requests.Items[0].Name, "Custom Name")
				}
			},
		},
		{
			"mixed inline and external requests",
			"testdata/with_mixed_inline_external.yaml",
			nil, 2,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Name != "Inline Request" {
					t.Errorf("first request name = %q, want %q", col.Requests.Items[0].Name, "Inline Request")
				}
				if col.Requests.Items[0].Request.URL != "https://example.com/inline" {
					t.Errorf("first request URL = %q, want inline URL", col.Requests.Items[0].Request.URL)
				}
				if col.Requests.Items[1].Name != "Get User" {
					t.Errorf("second request name = %q, want %q", col.Requests.Items[1].Name, "Get User")
				}
			},
		},
		{
			"same external file referenced twice with different variables",
			"testdata/with_external_ref_reuse.yaml",
			nil, 2,
			func(t *testing.T, col *Collection) {
				t.Helper()
				if col.Requests.Items[0].Variables.Values["token"] != "first" {
					t.Errorf("first ref token = %q, want %q", col.Requests.Items[0].Variables.Values["token"], "first")
				}
				if col.Requests.Items[1].Variables.Values["token"] != "second" {
					t.Errorf("second ref token = %q, want %q", col.Requests.Items[1].Variables.Values["token"], "second")
				}
			},
		},
		{
			"both path and request specified",
			"testdata/with_path_and_request.yaml",
			ErrMutuallyExclusive, 0, nil,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, err := ParseFile(tt.file)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("got error %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if len(col.Requests.Items) != tt.wantCount {
				t.Fatalf("got %d requests, want %d", len(col.Requests.Items), tt.wantCount)
			}
			if tt.checkResult != nil {
				tt.checkResult(t, col)
			}
		})
	}
}

func TestParseFile_ExternalRef_AuthPropagated(t *testing.T) {
	col, err := ParseFile("testdata/ref_with_auth.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	if got := col.Requests.Items[0].Auth; got != "admin_token" {
		t.Errorf("Requests[0].Auth = %q, want %q", got, "admin_token")
	}
}

func TestParseFile_ExternalRef_NoAuth_EmptyWhenAbsent(t *testing.T) {
	col, err := ParseFile("testdata/ref_without_auth.yaml")
	if err != nil {
		t.Fatalf("ParseFile error: %v", err)
	}
	if len(col.Requests.Items) != 1 {
		t.Fatalf("got %d requests, want 1", len(col.Requests.Items))
	}
	if got := col.Requests.Items[0].Auth; got != "" {
		t.Errorf("Requests[0].Auth = %q, want empty", got)
	}
}
