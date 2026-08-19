package docs

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strings"
)

// LayoutHeading is the README section that accounts for the repository's
// top-level directories. The guard keys on this heading rather than on the
// first three-column table in the file, so a table added elsewhere in the
// README cannot be mistaken for the accounting.
const LayoutHeading = "Repository layout"

// LayoutColumns are the header cells that identify the accounting table.
// Columns are resolved by name, not position, so reordering the table cannot
// silently change what the guard asserts.
var LayoutColumns = []string{"Directory", "What it is", "Relationship to the CLI"}

// PlatformStatus is M28-001's decision, stated once so that the three
// documents which must agree about it cannot drift into three different
// answers.
const PlatformStatus = "The `src/` backend and `web/` dashboard stay in this " +
	"repository, frozen: they build and pass their tests in " +
	"`./scripts/ci-local.sh --full`, no new feature work is planned, and the " +
	"`curlew` CLI does not call them."

// PlatformStatusDocs are the documents required to state PlatformStatus,
// relative to Root.
var PlatformStatusDocs = []string{"README.md", "docs/TECH_CHOICES.md", "docs/SPECIFICATION.md"}

// ErrNoLayoutTable reports that the source carries no table under
// LayoutHeading with LayoutColumns, or one with a header and no rows. Zero
// rows is an error rather than an empty slice: a guard that reads nothing and
// reports clean is the false clear this package exists to prevent.
var ErrNoLayoutTable = errors.New("no repository-layout table")

// ErrNoTopLevelDirs reports that a repository yielded no top-level
// directories at all, for the same reason.
var ErrNoTopLevelDirs = errors.New("no top-level directories")

// LayoutRow is one top-level directory as README.md accounts for it.
type LayoutRow struct {
	Line         int    // 1-based line of the row
	Dir          string // top-level directory name, backticks and trailing slash removed
	What         string // the "What it is" cell
	Relationship string // the "Relationship to the CLI" cell -- the load-bearing one
}

// String renders a row the way a test failure should print it: clickable,
// with enough context to find it without counting rows.
func (r LayoutRow) String() string {
	return fmt.Sprintf("README.md:%d %s/", r.Line, r.Dir)
}

// LayoutRows returns the rows of the repository-layout table in README
// source.
//
// It takes the text rather than a filename so the guards can be proven
// against synthetic documents, the same way ExtractProse and AuditProse are.
// Tables inside fenced code blocks and tables under other headings are not
// the accounting. A cell naming a sub-path rather than a top-level directory
// is an error: normalising cmd/curlew/ to cmd would just as happily accept
// cmd/curlew/internal/whatever/, which hollows the guard out.
func LayoutRows(src string) ([]LayoutRow, error) {
	lines := strings.Split(src, "\n")

	var (
		heading    string
		inFence    bool
		headerLine = -1
		dirIdx     = -1
		whatIdx    = -1
		relIdx     = -1
	)

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}

		if strings.HasPrefix(trimmed, "#") {
			heading = trimmed
			continue
		}

		if headerLine == -1 && strings.HasPrefix(trimmed, "|") && strings.Contains(heading, LayoutHeading) {
			cells := SplitRow(line)
			if containsAll(cells, LayoutColumns) {
				headerLine = i
				dirIdx = Column(cells, LayoutColumns[0])
				whatIdx = Column(cells, LayoutColumns[1])
				relIdx = Column(cells, LayoutColumns[2])
			}
		}
	}

	if headerLine == -1 {
		return nil, ErrNoLayoutTable
	}

	var rows []LayoutRow
	// The line after the header is the |---|---|---| separator; rows run
	// until the table ends, mirroring the convention already established by
	// Table and rowsAt.
	for i := headerLine + 2; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(trimmed, "|") {
			break
		}
		cells := SplitRow(lines[i])
		dir, err := normalizeLayoutDir(cellAt(cells, dirIdx))
		if err != nil {
			return nil, fmt.Errorf("README.md:%d: %w", i+1, err)
		}
		rows = append(rows, LayoutRow{
			Line:         i + 1,
			Dir:          dir,
			What:         cellAt(cells, whatIdx),
			Relationship: cellAt(cells, relIdx),
		})
	}

	if len(rows) == 0 {
		return nil, ErrNoLayoutTable
	}
	return rows, nil
}

// normalizeLayoutDir turns a table cell into a bare top-level directory name.
//
// A cell naming a sub-path (anything with a "/" left after the trailing
// slash is stripped) is rejected rather than normalised: silently reducing
// cmd/curlew/ to cmd would just as happily accept
// cmd/curlew/internal/whatever/, which hollows the guard out into an
// accounting of arbitrary paths instead of the repository root.
func normalizeLayoutDir(cell string) (string, error) {
	dir := strings.TrimSuffix(cell, "/")
	if strings.Contains(dir, "/") {
		return "", fmt.Errorf("%q names a sub-path, not a top-level directory: the repository-layout table accounts for the repository root's own directories, not paths inside them", cell)
	}
	return dir, nil
}

// cellAt returns cells[idx], or "" when idx is out of range -- a row with
// fewer cells than the header (a short row) reads as empty rather than
// panicking.
func cellAt(cells []string, idx int) string {
	if idx < 0 || idx >= len(cells) {
		return ""
	}
	return cells[idx]
}

// TopLevelDirs returns the top-level directories of the repository at root:
// every directory git tracks or has been told about, minus the ones it is
// told to ignore.
//
// Ignored paths are excluded deliberately. dist/ is created by the release
// step of scripts/ci-local.sh and survives between runs, so a plain
// directory listing reports a clean repository on the first gate and a
// broken one on every gate after it. Untracked-but-unignored directories ARE
// included, so a new one fails this guard before it is ever committed.
//
// An unavailable git, or a root that is not a repository, is an error rather
// than an empty set.
func TopLevelDirs(ctx context.Context, root string) ([]string, error) {
	cmd := exec.CommandContext(ctx, "git", "ls-files", "--cached", "--others", "--exclude-standard")
	cmd.Dir = root
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("listing files in %s: %w", root, err)
	}

	seen := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		idx := strings.Index(line, "/")
		if idx < 0 {
			continue // a root-level file, not a directory entry
		}
		seen[line[:idx]] = true
	}

	if len(seen) == 0 {
		return nil, ErrNoTopLevelDirs
	}

	dirs := make([]string, 0, len(seen))
	for d := range seen {
		dirs = append(dirs, d)
	}
	sort.Strings(dirs)
	return dirs, nil
}

// LayoutAudit is the outcome of holding a repository's top-level directories
// to README.md's accounting of them. It fails in both directions, the same
// contract the table and prose registers carry.
type LayoutAudit struct {
	Unaccounted []string    // a top-level directory with no row
	Stale       []string    // a row naming a directory that does not exist
	Unexplained []LayoutRow // a row that names a directory but states nothing about the CLI
	Duplicate   []string    // a directory named by more than one row
}

// Clean reports whether the audit found nothing.
func (a LayoutAudit) Clean() bool {
	return len(a.Unaccounted) == 0 && len(a.Stale) == 0 && len(a.Unexplained) == 0 && len(a.Duplicate) == 0
}

// AuditLayout holds a set of top-level directories to a set of README rows.
//
// It takes two slices rather than reading the filesystem so behaviour 2 -- a
// new top-level directory that no README row accounts for -- can be proven
// by mutation, without the test having to create a directory in the
// repository it is testing.
func AuditLayout(dirs []string, rows []LayoutRow) LayoutAudit {
	var audit LayoutAudit

	dirSet := make(map[string]bool, len(dirs))
	for _, d := range dirs {
		dirSet[d] = true
	}

	rowsByDir := map[string][]LayoutRow{}
	for _, r := range rows {
		rowsByDir[r.Dir] = append(rowsByDir[r.Dir], r)
	}

	for _, d := range dirs {
		if len(rowsByDir[d]) == 0 {
			audit.Unaccounted = append(audit.Unaccounted, d)
		}
	}
	sort.Strings(audit.Unaccounted)

	for dir, rs := range rowsByDir {
		if !dirSet[dir] {
			audit.Stale = append(audit.Stale, dir)
		}
		if len(rs) > 1 {
			audit.Duplicate = append(audit.Duplicate, dir)
		}
	}
	sort.Strings(audit.Stale)
	sort.Strings(audit.Duplicate)

	for _, r := range rows {
		if isLayoutPlaceholder(r.Relationship) || isLayoutPlaceholder(r.What) {
			audit.Unexplained = append(audit.Unexplained, r)
		}
	}
	sort.Slice(audit.Unexplained, func(i, j int) bool {
		return audit.Unexplained[i].Dir < audit.Unexplained[j].Dir
	})

	return audit
}

// isLayoutPlaceholder reports whether a table cell states nothing, in any of
// the forms a document tends to use for "not filled in yet": empty,
// whitespace, a dash, an em-dash, or a case-insensitive "n/a"/"tbd"/"?".
func isLayoutPlaceholder(cell string) bool {
	switch strings.ToLower(strings.TrimSpace(cell)) {
	case "", "-", "--", "—", "n/a", "tbd", "?":
		return true
	}
	return false
}
