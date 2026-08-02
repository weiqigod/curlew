package discovery

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testdataRoot returns the absolute path to testdata/ relative to this test file.
func testdataRoot(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs("testdata")
	if err != nil {
		t.Fatalf("cannot resolve testdata: %v", err)
	}
	return dir
}

func TestIsGlob(t *testing.T) {
	tests := []struct {
		name    string
		pattern string
		want    bool
	}{
		{"literal path", "testdata/a.yaml", false},
		{"star", "*.yaml", true},
		{"double star", "**/*.yaml", true},
		{"question mark", "a?.yaml", true},
		{"char class", "a[12].yaml", true},
		{"empty", "", false},
		{"path with no metachars", "some/path/file.yaml", false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := IsGlob(tt.pattern)
			if got != tt.want {
				t.Errorf("IsGlob(%q) = %v, want %v", tt.pattern, got, tt.want)
			}
		})
	}
}

func TestExpand(t *testing.T) {
	root := testdataRoot(t)

	tests := []struct {
		name      string
		pattern   string
		wantPaths []string // relative paths from root, sorted
		wantErr   error
	}{
		{
			name:    "matches all yaml recursively excluding ignored",
			pattern: "**/*.yaml",
			// drafts/d.yaml is excluded by .apitestignore
			wantPaths: []string{"a.yaml", "b.yaml", "sub/c.yaml"},
		},
		{
			name:      "matches top level only",
			pattern:   "*.yaml",
			wantPaths: []string{"a.yaml", "b.yaml"},
		},
		{
			name:      "zero matches returns ErrNoMatches",
			pattern:   "**/*.nomatch",
			wantPaths: nil,
			wantErr:   ErrNoMatches,
		},
		{
			name:    "traversal rejected",
			pattern: "../etc/passwd",
			wantErr: ErrTraversalOutsideRoot,
		},
		{
			name:    "traversal rejected: pattern ending with /..",
			pattern: "foo/..",
			wantErr: ErrTraversalOutsideRoot,
		},
		{
			name:    "traversal rejected: pattern ending with /.. nested",
			pattern: "a/b/..",
			wantErr: ErrTraversalOutsideRoot,
		},
		{
			name:    "absolute rejected",
			pattern: "/etc/*.yaml",
			wantErr: ErrAbsolutePattern,
		},
		{
			name:      "deterministic sort order",
			pattern:   "**/*.yaml",
			wantPaths: []string{"a.yaml", "b.yaml", "sub/c.yaml"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := Expand(root, tt.pattern)
			if tt.wantErr != nil {
				if err == nil {
					t.Fatalf("Expand() error = nil, want %v", tt.wantErr)
				}
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("Expand() error = %v, want %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("Expand() unexpected error: %v", err)
			}
			if len(got) != len(tt.wantPaths) {
				t.Fatalf("Expand() returned %d paths %v, want %d %v", len(got), got, len(tt.wantPaths), tt.wantPaths)
			}
			for i, p := range got {
				// Expand returns absolute paths; compare relative to root.
				rel, relErr := filepath.Rel(root, p)
				if relErr != nil {
					t.Fatalf("filepath.Rel: %v", relErr)
				}
				// Normalize to forward slashes for cross-platform comparison.
				rel = filepath.ToSlash(rel)
				if rel != tt.wantPaths[i] {
					t.Errorf("path[%d] = %q, want %q", i, rel, tt.wantPaths[i])
				}
			}
		})
	}

	t.Run("sort is deterministic across two calls", func(t *testing.T) {
		a, err1 := Expand(root, "**/*.yaml")
		b, err2 := Expand(root, "**/*.yaml")
		if err1 != nil || err2 != nil {
			t.Fatalf("Expand errors: %v, %v", err1, err2)
		}
		if len(a) != len(b) {
			t.Fatalf("got %d vs %d results", len(a), len(b))
		}
		for i := range a {
			if a[i] != b[i] {
				t.Errorf("results differ at index %d: %q vs %q", i, a[i], b[i])
			}
		}
	})
}

func TestLoadIgnore(t *testing.T) {
	t.Run("skips blank and comment lines", func(t *testing.T) {
		dir := t.TempDir()
		content := "# comment\n\n**/drafts/*.yaml\n\n"
		if err := os.WriteFile(filepath.Join(dir, ".apitestignore"), []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
		patterns, err := LoadIgnore(dir)
		if err != nil {
			t.Fatalf("LoadIgnore() error: %v", err)
		}
		if len(patterns) != 1 || patterns[0] != "**/drafts/*.yaml" {
			t.Errorf("LoadIgnore() = %v, want [\"**/drafts/*.yaml\"]", patterns)
		}
	})

	t.Run("missing file returns empty", func(t *testing.T) {
		dir := t.TempDir()
		patterns, err := LoadIgnore(dir)
		if err != nil {
			t.Fatalf("LoadIgnore() error: %v", err)
		}
		if len(patterns) != 0 {
			t.Errorf("LoadIgnore() = %v, want []", patterns)
		}
	})

	t.Run("reads testdata .apitestignore", func(t *testing.T) {
		root := testdataRoot(t)
		patterns, err := LoadIgnore(root)
		if err != nil {
			t.Fatalf("LoadIgnore() error: %v", err)
		}
		if len(patterns) == 0 {
			t.Error("LoadIgnore() returned empty, expected at least one pattern")
		}
	})
}
