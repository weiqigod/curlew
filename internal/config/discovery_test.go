package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestListCollections(t *testing.T) {
	tests := []struct {
		name     string
		setup    func(t *testing.T, dir string)
		expected []string
	}{
		{
			name: "collections_dir_with_yaml_files",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				colDir := filepath.Join(dir, "collections")
				if err := os.MkdirAll(colDir, 0o755); err != nil {
					t.Fatal(err)
				}
				for _, f := range []string{"a.yaml", "b.yml"} {
					if err := os.WriteFile(filepath.Join(colDir, f), []byte("name: test\n"), 0o600); err != nil {
						t.Fatal(err)
					}
				}
			},
			expected: []string{"collections/a.yaml", "collections/b.yml"},
		},
		{
			name: "collections_dir_empty",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				if err := os.MkdirAll(filepath.Join(dir, "collections"), 0o755); err != nil {
					t.Fatal(err)
				}
			},
			expected: nil,
		},
		{
			name:     "no_collections_dir",
			setup:    func(t *testing.T, dir string) { t.Helper() },
			expected: nil,
		},
		{
			name: "ignores_non_yaml_files",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				colDir := filepath.Join(dir, "collections")
				if err := os.MkdirAll(colDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(colDir, "readme.md"), []byte("# README\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			expected: nil,
		},
		{
			name: "nested_subdirectories_not_included",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				subDir := filepath.Join(dir, "collections", "sub")
				if err := os.MkdirAll(subDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(subDir, "nested.yaml"), []byte("name: nested\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			expected: nil,
		},
		{
			name: "yml_extension_included",
			setup: func(t *testing.T, dir string) {
				t.Helper()
				colDir := filepath.Join(dir, "collections")
				if err := os.MkdirAll(colDir, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(filepath.Join(colDir, "test.yml"), []byte("name: test\n"), 0o600); err != nil {
					t.Fatal(err)
				}
			},
			expected: []string{"collections/test.yml"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			tc.setup(t, dir)

			got := ListCollections(dir)

			if len(tc.expected) == 0 && len(got) == 0 {
				return
			}
			if len(got) != len(tc.expected) {
				t.Fatalf("ListCollections() = %v, want %v", got, tc.expected)
			}
			for i := range tc.expected {
				if got[i] != tc.expected[i] {
					t.Errorf("ListCollections()[%d] = %q, want %q", i, got[i], tc.expected[i])
				}
			}
		})
	}
}
