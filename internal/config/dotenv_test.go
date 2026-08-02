package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/peterlindqvist/apitest/internal/variable"
)

func TestParseDotenv(t *testing.T) {
	tests := []struct {
		name         string
		input        string
		want         map[string]string
		wantErr      bool
		wantSentinel error // if non-nil, verify errors.Is match
	}{
		{"simple key=value", "KEY=value", map[string]string{"KEY": "value"}, false, nil},
		{"multiple pairs", "A=1\nB=2", map[string]string{"A": "1", "B": "2"}, false, nil},
		{"comment lines ignored", "# comment\nKEY=val", map[string]string{"KEY": "val"}, false, nil},
		{"empty lines ignored", "\nKEY=val\n\n", map[string]string{"KEY": "val"}, false, nil},
		{"whitespace-only lines ignored", "  \nKEY=val", map[string]string{"KEY": "val"}, false, nil},
		{"empty value", "KEY=", map[string]string{"KEY": ""}, false, nil},
		{"value with equals sign", "KEY=a=b=c", map[string]string{"KEY": "a=b=c"}, false, nil},
		{"double-quoted value", `KEY="value with spaces"`, map[string]string{"KEY": "value with spaces"}, false, nil},
		{"single-quoted value", "KEY='value with spaces'", map[string]string{"KEY": "value with spaces"}, false, nil},
		{"quoted value preserves inner spaces", `KEY="  spaced  "`, map[string]string{"KEY": "  spaced  "}, false, nil},
		{"inline hash not treated as comment", "KEY=value#notcomment", map[string]string{"KEY": "value#notcomment"}, false, nil},
		{"empty input", "", map[string]string{}, false, nil},
		{"only comments and blanks", "# comment\n\n# another", map[string]string{}, false, nil},
		{"leading/trailing whitespace on key trimmed", "  KEY  =value", map[string]string{"KEY": "value"}, false, nil},
		{"leading/trailing whitespace on unquoted value trimmed", "KEY=  value  ", map[string]string{"KEY": "value"}, false, nil},
		{"quoted value does not trim internal spaces", `KEY="  value  "`, map[string]string{"KEY": "  value  "}, false, nil},
		{"line with no equals sign", "BADLINE", nil, true, ErrInvalidDotenv},
		{"empty key", "=value", nil, true, ErrInvalidDotenv},
		{"duplicate keys last wins", "KEY=first\nKEY=second", map[string]string{"KEY": "second"}, false, nil},
		{"URL as value", "URL=https://example.com/api?q=1&x=2", map[string]string{"URL": "https://example.com/api?q=1&x=2"}, false, nil},
		{"windows line endings", "A=1\r\nB=2\r\n", map[string]string{"A": "1", "B": "2"}, false, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, _, err := ParseDotenv([]byte(tt.input))
			if (err != nil) != tt.wantErr {
				t.Fatalf("ParseDotenv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.wantSentinel != nil && !errors.Is(err, tt.wantSentinel) {
					t.Errorf("ParseDotenv() error does not match sentinel: got %v, want errors.Is(%v)", err, tt.wantSentinel)
				}
				return
			}
			if len(got) != len(tt.want) {
				t.Fatalf("ParseDotenv() got %d entries, want %d", len(got), len(tt.want))
			}
			for k, wantV := range tt.want {
				gotV, ok := got[k]
				if !ok {
					t.Errorf("missing key %q", k)
					continue
				}
				if gotV != wantV {
					t.Errorf("key %q = %q, want %q", k, gotV, wantV)
				}
			}
		})
	}
}

func ptr(s string) *string { return &s }

func TestLoadDotenv(t *testing.T) {
	tests := []struct {
		name         string
		content      *string // nil = don't create file
		wantCount    int
		wantErr      bool
		wantSentinel error // if non-nil, verify errors.Is match
	}{
		{"file exists with valid content", ptr("KEY=val"), 1, false, nil},
		{"file does not exist", nil, 0, false, nil},
		{"file is empty", ptr(""), 0, false, nil},
		{"invalid content", ptr("BADLINE"), 0, true, ErrInvalidDotenv},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			if tt.content != nil {
				err := os.WriteFile(filepath.Join(dir, ".env"), []byte(*tt.content), 0o644)
				if err != nil {
					t.Fatalf("setup: %v", err)
				}
			}
			got, _, err := LoadDotenv(dir)
			if (err != nil) != tt.wantErr {
				t.Fatalf("LoadDotenv() error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.wantErr {
				if tt.wantSentinel != nil && !errors.Is(err, tt.wantSentinel) {
					t.Errorf("LoadDotenv() error does not match sentinel: got %v, want errors.Is(%v)", err, tt.wantSentinel)
				}
				return
			}
			if len(got) != tt.wantCount {
				t.Errorf("LoadDotenv() got %d entries, want %d", len(got), tt.wantCount)
			}
		})
	}
}

func TestLoadDotenv_unreadable_file(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("test requires non-root user")
	}

	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("KEY=val"), 0o000); err != nil {
		t.Fatalf("setup: %v", err)
	}

	_, _, err := LoadDotenv(dir)
	if err == nil {
		t.Fatal("LoadDotenv() expected error for unreadable file, got nil")
	}
	if errors.Is(err, ErrInvalidDotenv) {
		t.Error("LoadDotenv() error should not be ErrInvalidDotenv for permission error")
	}
}

func TestParseDotenvSensitive(t *testing.T) {
	tests := []struct {
		name          string
		input         string
		wantValues    map[string]string
		wantSensitive []string
	}{
		{
			name:          "sensitive_prefix_marks_variable",
			input:         "!sensitive API_KEY=sk_live_abc",
			wantValues:    map[string]string{"API_KEY": "sk_live_abc"},
			wantSensitive: []string{"API_KEY"},
		},
		{
			name:          "sensitive_prefix_preserves_value",
			input:         "!sensitive SECRET=myvalue",
			wantValues:    map[string]string{"SECRET": "myvalue"},
			wantSensitive: []string{"SECRET"},
		},
		{
			name:          "no_sensitive_returns_empty_set",
			input:         "KEY=value",
			wantValues:    map[string]string{"KEY": "value"},
			wantSensitive: []string{},
		},
		{
			name:          "multiple_sensitive_lines",
			input:         "!sensitive A=1\n!sensitive B=2",
			wantValues:    map[string]string{"A": "1", "B": "2"},
			wantSensitive: []string{"A", "B"},
		},
		{
			name:          "regular_lines_not_in_sensitive_set",
			input:         "PUBLIC=value\n!sensitive SECRET=s3cr3t",
			wantValues:    map[string]string{"PUBLIC": "value", "SECRET": "s3cr3t"},
			wantSensitive: []string{"SECRET"},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, sensitive, err := ParseDotenv([]byte(tt.input))
			if err != nil {
				t.Fatalf("ParseDotenv() error = %v", err)
			}
			for k, want := range tt.wantValues {
				if got[k] != want {
					t.Errorf("key %q = %q, want %q", k, got[k], want)
				}
			}
			for _, name := range tt.wantSensitive {
				if !sensitive.IsSensitive(name) {
					t.Errorf("expected %q to be sensitive", name)
				}
			}
			// Verify non-sensitive keys aren't in the set
			for k := range got {
				isSensitive := false
				for _, name := range tt.wantSensitive {
					if name == k {
						isSensitive = true
						break
					}
				}
				if !isSensitive && sensitive.IsSensitive(k) {
					t.Errorf("key %q should not be sensitive", k)
				}
			}
			_ = variable.IsSensitiveName("") // ensure import used
		})
	}
}
