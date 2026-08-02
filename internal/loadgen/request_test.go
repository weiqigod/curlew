package loadgen

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadRequestFile(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr bool
		errIs   error
		check   func(*testing.T, *loadgenRequest)
	}{
		{
			name: "valid GET",
			yaml: "name: test\nrequest:\n  method: GET\n  url: http://example.com\n",
			check: func(t *testing.T, r *loadgenRequest) {
				t.Helper()
				if r.Method != "GET" {
					t.Errorf("Method = %q, want GET", r.Method)
				}
				if r.URL != "http://example.com" {
					t.Errorf("URL = %q, want http://example.com", r.URL)
				}
			},
		},
		{
			name: "default method is GET",
			yaml: "name: test\nrequest:\n  url: http://example.com\n",
			check: func(t *testing.T, r *loadgenRequest) {
				t.Helper()
				if r.Method != "GET" {
					t.Errorf("Method = %q, want GET (default)", r.Method)
				}
			},
		},
		{
			name:    "missing url rejected",
			yaml:    "name: test\nrequest:\n  method: GET\n",
			wantErr: true,
		},
		{
			name:    "invalid yaml rejected",
			yaml:    "not: [valid: yaml:::\n",
			wantErr: true,
		},
		{
			name:    "POST with body and headers",
			yaml:    "name: post\nrequest:\n  method: POST\n  url: http://example.com/api\n  headers:\n    Content-Type: application/json\n",
			wantErr: false,
			check: func(t *testing.T, r *loadgenRequest) {
				t.Helper()
				if r.Method != "POST" {
					t.Errorf("Method = %q, want POST", r.Method)
				}
				if ct := r.Headers["Content-Type"]; ct != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", ct)
				}
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var path string
			if tc.yaml != "" {
				// Write YAML to temp file.
				dir := t.TempDir()
				path = filepath.Join(dir, "req.yaml")
				if err := os.WriteFile(path, []byte(tc.yaml), 0o600); err != nil {
					t.Fatalf("write temp file: %v", err)
				}
			} else {
				path = "/nonexistent/file.yaml"
			}

			req, err := LoadRequestFile(path)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if tc.errIs != nil && !errors.Is(err, tc.errIs) {
					t.Errorf("error = %v, want Is(%v)", err, tc.errIs)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadRequestFile() error = %v", err)
			}
			if tc.check != nil {
				// Convert to loadgenRequest for checking.
				r := &loadgenRequest{
					Method:  req.Method,
					URL:     req.URL,
					Headers: req.Headers,
					Body:    req.Body,
				}
				tc.check(t, r)
			}
		})
	}
}

func TestLoadRequestFile_NotFound(t *testing.T) {
	_, err := LoadRequestFile("/tmp/definitely_does_not_exist_curlew_test.yaml")
	if err == nil {
		t.Fatal("expected error for missing file, got nil")
	}
	if !errors.Is(err, ErrRequestFileNotFound) {
		t.Errorf("error = %v, want ErrRequestFileNotFound", err)
	}
}

// loadgenRequest is a local struct for test checking purposes.
type loadgenRequest struct {
	Method  string
	URL     string
	Headers map[string]string
	Body    any
}
