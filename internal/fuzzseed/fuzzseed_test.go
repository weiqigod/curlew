package fuzzseed

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// mustRoot returns the repository root or fails the test. Every test in this
// file needs it, so it is factored out rather than repeated.
func mustRoot(t *testing.T) string {
	t.Helper()
	root, err := Root(".")
	if err != nil {
		t.Fatalf("Root(.): %v", err)
	}
	return root
}

func TestFuzzseed_every_reader_returns_real_fixtures(t *testing.T) {
	root := mustRoot(t)

	tests := []struct {
		name    string
		read    func(string) (int, error)
		atLeast int
	}{
		// atLeast floors are the counts measured on 2026-08-18 (collections=168,
		// templates=110, cel=18, jsonpaths=119, bodies=6), each with a margin so
		// the test fails if a fixture directory is moved or emptied rather than
		// only when it changes at all.
		{
			name: "collections from parser testdata and testapi",
			read: func(root string) (int, error) {
				seeds, err := Collections(root)
				return len(seeds), err
			},
			atLeast: 150,
		},
		{
			name: "templates from collection fixtures",
			read: func(root string) (int, error) {
				tmpls, err := Templates(root)
				return len(tmpls), err
			},
			atLeast: 90,
		},
		{
			name: "cel expressions from collections and manual",
			read: func(root string) (int, error) {
				exprs, err := CELExpressions(root)
				return len(exprs), err
			},
			atLeast: 12,
		},
		{
			name: "jsonpaths from body assertions",
			read: func(root string) (int, error) {
				paths, err := JSONPaths(root)
				return len(paths), err
			},
			atLeast: 100,
		},
		{
			name: "json bodies from internal testdata",
			read: func(root string) (int, error) {
				seeds, err := JSONBodies(root)
				return len(seeds), err
			},
			atLeast: 4,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			n, err := tt.read(root)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if n < tt.atLeast {
				t.Fatalf("got %d fixtures, want at least %d", n, tt.atLeast)
			}
		})
	}
}

func TestFuzzseed_missing_source_errors_rather_than_returning_empty(t *testing.T) {
	empty := t.TempDir()

	tests := []struct {
		name string
		read func(string) error
	}{
		{"collections", func(root string) error { _, err := Collections(root); return err }},
		{"templates", func(root string) error { _, err := Templates(root); return err }},
		{"cel", func(root string) error { _, err := CELExpressions(root); return err }},
		{"jsonpaths", func(root string) error { _, err := JSONPaths(root); return err }},
		{"json bodies", func(root string) error { _, err := JSONBodies(root); return err }},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.read(empty)
			if !errors.Is(err, ErrNoSeeds) {
				t.Fatalf("got error %v, want ErrNoSeeds", err)
			}
		})
	}
}

func TestFuzzseed_Root_walks_up_to_go_mod(t *testing.T) {
	t.Run("from package dir", func(t *testing.T) {
		root, err := Root(".")
		if err != nil {
			t.Fatalf("Root(.): %v", err)
		}
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
			t.Fatalf("resolved root %q has no go.mod: %v", root, err)
		}
	})

	t.Run("from nested testdata dir", func(t *testing.T) {
		// internal/fuzzseed -> internal -> internal/parser/testdata is not an
		// ancestor relationship, so build a real, deeply nested path that
		// exists in this checkout instead: internal/parser/testdata/schema.
		nested := filepath.Join("..", "parser", "testdata", "schema")
		if _, err := os.Stat(nested); err != nil {
			t.Fatalf("fixture directory missing, test assumption broken: %v", err)
		}
		root, err := Root(nested)
		if err != nil {
			t.Fatalf("Root(%q): %v", nested, err)
		}
		if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
			t.Fatalf("resolved root %q has no go.mod: %v", root, err)
		}
	})

	t.Run("error - no go.mod above tempdir", func(t *testing.T) {
		dir := t.TempDir()
		if _, err := Root(dir); err == nil {
			t.Fatalf("Root(%q): got nil error, want an error", dir)
		}
	})
}
