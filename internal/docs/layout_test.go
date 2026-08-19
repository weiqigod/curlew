package docs_test

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// joinLines builds markdown source from explicit lines, one per slice
// element, so a test can name the exact 1-based line it expects a row on
// without counting characters in a literal.
func joinLines(lines ...string) string {
	return strings.Join(lines, "\n") + "\n"
}

func TestLayoutRows(t *testing.T) {
	const header = "| Directory | What it is | Relationship to the CLI |"
	const sep = "|---|---|---|"

	tests := []struct {
		name            string
		src             string
		want            []docs.LayoutRow
		wantErr         error
		wantErrContains string
	}{
		{
			name: "header row and one data row",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| `cmd/` | CLI entry point | build time — is the binary |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "CLI entry point", Relationship: "build time — is the binary"},
			},
		},
		{
			name: "columns are resolved by name not position",
			src: joinLines(
				"## Repository layout",
				"",
				"| Relationship to the CLI | Directory | What it is |",
				sep,
				"| build time | `internal/` | CLI packages |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "internal", What: "CLI packages", Relationship: "build time"},
			},
		},
		{
			name: "backticked directory cell is unwrapped",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| `cmd/` | x | y |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "x", Relationship: "y"},
			},
		},
		{
			name: "trailing slash is stripped",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| cmd/ | x | y |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "x", Relationship: "y"},
			},
		},
		{
			name: "a sub-path is rejected, not normalised",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| `cmd/curlew/` | x | y |",
			),
			wantErrContains: "top-level directory",
		},
		{
			name: "a table under a different heading is not the layout table",
			src: joinLines(
				"## Some Other Heading",
				"",
				header,
				sep,
				"| `cmd/` | x | y |",
				"",
				"## Repository layout",
				"",
				header,
				sep,
				"| `internal/` | a | b |",
			),
			want: []docs.LayoutRow{
				{Line: 11, Dir: "internal", What: "a", Relationship: "b"},
			},
		},
		{
			name: "a table inside a fenced code block is not the layout table",
			src: joinLines(
				"## Repository layout",
				"",
				"```",
				header,
				sep,
				"| `cmd/` | x | y |",
				"```",
				"",
				header,
				sep,
				"| `internal/` | a | b |",
			),
			want: []docs.LayoutRow{
				{Line: 11, Dir: "internal", What: "a", Relationship: "b"},
			},
		},
		{
			name: "the table ends at the first non-pipe line",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| `cmd/` | x | y |",
				"",
				"Some trailing prose that must not be read as a row.",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "x", Relationship: "y"},
			},
		},
		{
			name: "the separator row is not mistaken for data",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				"|:---|:---:|---:|",
				"| `cmd/` | x | y |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "x", Relationship: "y"},
			},
		},
		{
			name: "a row with fewer cells than the header does not panic",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
				"| `cmd/` | x |",
			),
			want: []docs.LayoutRow{
				{Line: 5, Dir: "cmd", What: "x", Relationship: ""},
			},
		},
		{
			name: "no layout table returns ErrNoLayoutTable",
			src: joinLines(
				"## Repository layout",
				"",
				"No table here, just prose.",
			),
			wantErr: docs.ErrNoLayoutTable,
		},
		{
			name: "a header with no rows returns ErrNoLayoutTable",
			src: joinLines(
				"## Repository layout",
				"",
				header,
				sep,
			),
			wantErr: docs.ErrNoLayoutTable,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := docs.LayoutRows(tc.src)

			switch {
			case tc.wantErr != nil:
				if !errors.Is(err, tc.wantErr) {
					t.Fatalf("LayoutRows() error = %v; want %v", err, tc.wantErr)
				}
			case tc.wantErrContains != "":
				if err == nil || !strings.Contains(err.Error(), tc.wantErrContains) {
					t.Fatalf("LayoutRows() error = %v; want error containing %q", err, tc.wantErrContains)
				}
			default:
				if err != nil {
					t.Fatalf("LayoutRows() unexpected error: %v", err)
				}
				if !reflect.DeepEqual(got, tc.want) {
					t.Errorf("LayoutRows() = %+v; want %+v", got, tc.want)
				}
			}
		})
	}
}

func TestAuditLayout(t *testing.T) {
	row := func(dir, what, rel string) docs.LayoutRow {
		return docs.LayoutRow{Dir: dir, What: what, Relationship: rel}
	}

	tests := []struct {
		name            string
		dirs            []string
		rows            []docs.LayoutRow
		wantUnaccounted []string
		wantStale       []string
		wantUnexplained int
		wantDuplicate   []string
	}{
		{
			name: "clean: every directory has exactly one row that explains it",
			dirs: []string{"cmd", "internal"},
			rows: []docs.LayoutRow{
				row("cmd", "entry point", "build time"),
				row("internal", "packages", "build time"),
			},
		},
		{
			name: "MUTATION unaccounted: a new top-level directory with no row",
			dirs: []string{"cmd", "internal", "newthing"},
			rows: []docs.LayoutRow{
				row("cmd", "entry point", "build time"),
				row("internal", "packages", "build time"),
			},
			wantUnaccounted: []string{"newthing"},
		},
		{
			name: "MUTATION stale: a row naming a directory that no longer exists",
			dirs: []string{"cmd"},
			rows: []docs.LayoutRow{
				row("cmd", "entry point", "build time"),
				row("removed", "gone", "gone"),
			},
			wantStale: []string{"removed"},
		},
		{
			name:            "MUTATION unexplained: relationship cell empty",
			dirs:            []string{"cmd"},
			rows:            []docs.LayoutRow{row("cmd", "entry point", "")},
			wantUnexplained: 1,
		},
		{
			name:            "MUTATION unexplained: relationship cell whitespace only",
			dirs:            []string{"cmd"},
			rows:            []docs.LayoutRow{row("cmd", "entry point", "   ")},
			wantUnexplained: 1,
		},
		{
			name:            "MUTATION unexplained: relationship cell is an em-dash placeholder",
			dirs:            []string{"cmd"},
			rows:            []docs.LayoutRow{row("cmd", "entry point", "—")},
			wantUnexplained: 1,
		},
		{
			name:            "MUTATION unexplained: relationship cell is TBD, any case",
			dirs:            []string{"cmd"},
			rows:            []docs.LayoutRow{row("cmd", "entry point", "TbD")},
			wantUnexplained: 1,
		},
		{
			name:            `MUTATION unexplained: "what it is" cell empty`,
			dirs:            []string{"cmd"},
			rows:            []docs.LayoutRow{row("cmd", "", "build time")},
			wantUnexplained: 1,
		},
		{
			name: "MUTATION duplicate: two rows for the same directory",
			dirs: []string{"cmd"},
			rows: []docs.LayoutRow{
				row("cmd", "entry point", "build time"),
				row("cmd", "entry point again", "build time again"),
			},
			wantDuplicate: []string{"cmd"},
		},
		{
			name: "MUTATION both: one unaccounted and one stale",
			dirs: []string{"a", "b"},
			rows: []docs.LayoutRow{
				row("b", "x", "y"),
				row("c", "x", "y"),
			},
			wantUnaccounted: []string{"a"},
			wantStale:       []string{"c"},
		},
		{
			name:            "no rows at all leaves every directory unaccounted",
			dirs:            []string{"c", "a", "b"},
			rows:            nil,
			wantUnaccounted: []string{"a", "b", "c"},
		},
		{
			name: "no directories at all makes every row stale",
			dirs: nil,
			rows: []docs.LayoutRow{
				row("b", "x", "y"),
				row("a", "x", "y"),
			},
			wantStale: []string{"a", "b"},
		},
		{
			name: "comparison does not depend on order",
			dirs: []string{"zebra", "alpha", "middle"},
			rows: []docs.LayoutRow{
				row("middle", "x", "y"),
			},
			wantUnaccounted: []string{"alpha", "zebra"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			audit := docs.AuditLayout(tc.dirs, tc.rows)

			if !reflect.DeepEqual(audit.Unaccounted, tc.wantUnaccounted) {
				t.Errorf("Unaccounted = %v; want %v", audit.Unaccounted, tc.wantUnaccounted)
			}
			if !reflect.DeepEqual(audit.Stale, tc.wantStale) {
				t.Errorf("Stale = %v; want %v", audit.Stale, tc.wantStale)
			}
			if len(audit.Unexplained) != tc.wantUnexplained {
				t.Errorf("Unexplained = %v; want len %d", audit.Unexplained, tc.wantUnexplained)
			}
			if !reflect.DeepEqual(audit.Duplicate, tc.wantDuplicate) {
				t.Errorf("Duplicate = %v; want %v", audit.Duplicate, tc.wantDuplicate)
			}

			wantClean := tc.wantUnaccounted == nil && tc.wantStale == nil && tc.wantUnexplained == 0 && tc.wantDuplicate == nil
			if audit.Clean() != wantClean {
				t.Errorf("Clean() = %v; want %v", audit.Clean(), wantClean)
			}
		})
	}
}

// gitTestRepo creates a fresh git repository in t.TempDir() with the
// author/committer identity set, mirroring internal/prcheck/git_test.go's
// fixture so TopLevelDirs is exercised against a real repository rather than
// a mocked one.
func gitTestRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	runGitCmd(t, dir, "init")
	runGitCmd(t, dir, "config", "user.email", "test@example.com")
	runGitCmd(t, dir, "config", "user.name", "Test")
	return dir
}

func runGitCmd(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"GIT_AUTHOR_NAME=Test",
		"GIT_AUTHOR_EMAIL=test@example.com",
		"GIT_COMMITTER_NAME=Test",
		"GIT_COMMITTER_EMAIL=test@example.com",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, out)
	}
}

func writeTestFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}

func stringSliceContains(s []string, v string) bool {
	for _, e := range s {
		if e == v {
			return true
		}
	}
	return false
}

func TestTopLevelDirs(t *testing.T) {
	t.Run("a tracked directory is listed", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", "initial")

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err != nil {
			t.Fatalf("TopLevelDirs() error: %v", err)
		}
		if !stringSliceContains(got, "cmd") {
			t.Errorf("TopLevelDirs() = %v; want it to contain cmd", got)
		}
	})

	t.Run("an ignored directory is not listed", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")
		writeTestFile(t, filepath.Join(dir, ".gitignore"), "dist/\n")
		writeTestFile(t, filepath.Join(dir, "dist", "curlew"), "binary")
		runGitCmd(t, dir, "add", "cmd", ".gitignore")
		runGitCmd(t, dir, "commit", "-m", "initial")

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err != nil {
			t.Fatalf("TopLevelDirs() error: %v", err)
		}
		if stringSliceContains(got, "dist") {
			t.Errorf("TopLevelDirs() = %v; dist/ is gitignored and must not be listed", got)
		}
	})

	t.Run("a file at the repository root is not a directory", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "go.mod"), "module x")
		writeTestFile(t, filepath.Join(dir, "README.md"), "# x")
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", "initial")

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err != nil {
			t.Fatalf("TopLevelDirs() error: %v", err)
		}
		if stringSliceContains(got, "go.mod") || stringSliceContains(got, "README.md") {
			t.Errorf("TopLevelDirs() = %v; root files must not appear", got)
		}
	})

	t.Run("a newly added uncommitted directory is listed", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", "initial")

		writeTestFile(t, filepath.Join(dir, "newthing", "file.txt"), "x")
		runGitCmd(t, dir, "add", "newthing")
		// deliberately not committed -- --cached must still see it.

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err != nil {
			t.Fatalf("TopLevelDirs() error: %v", err)
		}
		if !stringSliceContains(got, "newthing") {
			t.Errorf("TopLevelDirs() = %v; staged-new newthing/ must be listed", got)
		}
	})

	t.Run("an untracked directory that is not ignored is listed", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", "initial")

		writeTestFile(t, filepath.Join(dir, "scratch", "file.txt"), "x")
		// not staged at all -- --others --exclude-standard must still see it.

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err != nil {
			t.Fatalf("TopLevelDirs() error: %v", err)
		}
		if !stringSliceContains(got, "scratch") {
			t.Errorf("TopLevelDirs() = %v; untracked-unignored scratch/ must be listed", got)
		}
	})

	t.Run("a repository with no directories returns ErrNoTopLevelDirs", func(t *testing.T) {
		dir := gitTestRepo(t)
		writeTestFile(t, filepath.Join(dir, "README.md"), "# x")
		runGitCmd(t, dir, "add", ".")
		runGitCmd(t, dir, "commit", "-m", "initial")

		_, err := docs.TopLevelDirs(context.Background(), dir)
		if !errors.Is(err, docs.ErrNoTopLevelDirs) {
			t.Fatalf("TopLevelDirs() error = %v; want ErrNoTopLevelDirs", err)
		}
	})

	t.Run("a directory that is not a git repository errors, not empty", func(t *testing.T) {
		dir := t.TempDir()
		writeTestFile(t, filepath.Join(dir, "cmd", "main.go"), "package main")

		got, err := docs.TopLevelDirs(context.Background(), dir)
		if err == nil {
			t.Fatalf("TopLevelDirs() = %v, nil error; want an error for a non-git directory", got)
		}
	})
}
