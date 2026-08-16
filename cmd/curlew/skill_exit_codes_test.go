package main

import (
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/exitcodes"
)

// The shipped agent skill states the exit-code contract in four separate
// places. Parsing one and calling it done would reproduce the very defect
// this file exists to catch, one level down: SKILL.md could be held to the
// binary while failure-playbook.md regrew a stale row unnoticed. So every
// statement is read independently, each is checked against the codes
// cmd/curlew can actually return (both directions), and the four are
// checked against each other.
//
// skillRoot is the function whose return statements define the binary's
// exit-code surface. It dispatches to every subcommand, so a walk rooted
// here reaches every *CmdOut function's own returns.
const skillRoot = "runWithWriters"

// skillStatement is one place the skill states the exit-code contract.
type skillStatement struct {
	file string                          // base name under .claude/skills/curlew/
	what string                          // names the statement in failure output, e.g. "master table"
	sub  string                          // t.Run subtest name suffix, e.g. "master_table"
	read func(*testing.T, string) []int
}

// skillExitCodeStatements enumerates the four statements, in the order they
// appear across the skill's files.
func skillExitCodeStatements() []skillStatement {
	return []skillStatement{
		{
			file: "SKILL.md",
			what: "failure playbook",
			sub:  "failure_playbook_table",
			read: func(t *testing.T, body string) []int {
				return exitCodesFromTable(t, body, "Failure playbook", "Exit")
			},
		},
		{
			file: "exit-codes.md",
			what: "master table",
			sub:  "master_table",
			read: func(t *testing.T, body string) []int {
				return exitCodesFromTable(t, body, "Exit code table", "Code")
			},
		},
		{
			file: "exit-codes.md",
			what: "how to read list",
			sub:  "how_to_read_list",
			read: func(t *testing.T, body string) []int {
				return exitCodesFromNumberedList(t, body)
			},
		},
		{
			file: "failure-playbook.md",
			what: "per-code headings",
			sub:  "per_code_headings",
			read: func(t *testing.T, body string) []int {
				return exitCodesFromHeadings(t, body)
			},
		},
	}
}

// skillFiles scaffolds `curlew init --skill agent` into a fresh temp
// directory and returns the three exit-code-bearing skill files, keyed by
// base name. Reading the scaffolded OUTPUT, not templates/ directly, is
// deliberate: it is what actually lands in a user's repository, and it
// exercises templates.Walk + installSkill on the way.
func skillFiles(t *testing.T) map[string]string {
	t.Helper()
	tree := scaffoldTreeWithSkill(t, "agent")
	const root = ".claude/skills/curlew/"
	out := map[string]string{}
	for _, name := range []string{"SKILL.md", "exit-codes.md", "failure-playbook.md"} {
		body, ok := tree[root+name]
		if !ok {
			t.Fatalf("scaffolded skill missing %s%s", root, name)
		}
		out[name] = body
	}
	return out
}

// reachableExitCodes derives the exit codes cmd/curlew can actually return.
// Its three guards protect against the same failure shape from three angles:
// a walk that resolves nothing at all (ErrRootNotFound), a walk that finds
// too little to be real (size), and a walk whose call resolution silently
// stopped working, which would otherwise pass both of the above by finding
// only runWithWriters's own literal `return 0`/`return 1` (depth).
func reachableExitCodes(t *testing.T) []exitcodes.Code {
	t.Helper()
	codes, err := exitcodes.Reachable(".", skillRoot)
	if err != nil {
		t.Fatalf("no exit codes read from cmd/curlew: %v — the AST walk is broken, not the CLI", err)
	}
	if len(codes) < 2 {
		t.Fatalf("the walk found %d exit code(s) in cmd/curlew (%v) — a CLI returns at least "+
			"success and failure, so the AST walk is broken, not the CLI", len(codes), exitcodes.Set(codes))
	}
	if exitcodes.MaxDepth(codes) == 0 {
		t.Fatalf("every exit code came from %s itself (%v); nothing was collected through a "+
			"return-position call — the call-graph walk is broken, not the CLI", skillRoot, exitcodes.Set(codes))
	}
	return codes
}

// readStatement runs stmt's reader against its file's body and enforces the
// per-statement vacuity guard: a reader that found nothing is far more
// likely broken than a skill whose exit-code table is genuinely empty.
func readStatement(t *testing.T, stmt skillStatement, files map[string]string) []int {
	t.Helper()
	body, ok := files[stmt.file]
	if !ok {
		t.Fatalf("%s: not among the scaffolded skill files", stmt.file)
	}
	codes := stmt.read(t, body)
	if len(codes) == 0 {
		t.Fatalf("%s (%s): no exit codes read — the parser is broken, not the skill", stmt.file, stmt.what)
	}
	return codes
}

// codeCellKind classifies one Code-column table cell so a malformed cell
// fails loudly rather than being silently skipped, the way a `continue` on a
// failed ^\d+$ match would drop a row written `| six |` without a trace.
type codeCellKind int

const (
	cellCode codeCellKind = iota // "3", "`3`"
	cellNone                     // "—", "-", ""
	cellBad                      // "six", "ERR_CEL_PARSE"
)

// classifyCodeCell reads one Code-column cell, unwrapping a cell that is
// entirely backtick-quoted code.
func classifyCodeCell(cell string) (int, codeCellKind) {
	c := strings.TrimSpace(cell)
	if len(c) >= 2 && strings.HasPrefix(c, "`") && strings.HasSuffix(c, "`") {
		c = strings.Trim(c, "`")
	}
	switch c {
	case "", "—", "-":
		return 0, cellNone
	}
	n, err := strconv.Atoi(c)
	if err != nil {
		return 0, cellBad
	}
	return n, cellCode
}

// tableUnder returns the header and rows of the first markdown table
// appearing under a "## "+heading section of body, before the next level-2
// heading.
func tableUnder(t *testing.T, body, heading string) (header []string, rows [][]string) {
	t.Helper()
	lines := strings.Split(body, "\n")
	marker := "## " + heading
	inSection := false
	for i := 0; i < len(lines); i++ {
		line := lines[i]
		if strings.HasPrefix(line, "## ") {
			if strings.TrimSpace(line) == marker {
				inSection = true
				continue
			}
			if inSection {
				break // left the section without finding a table
			}
			continue
		}
		if !inSection || !strings.HasPrefix(strings.TrimSpace(line), "|") {
			continue
		}
		header = splitTableRow(line)
		for j := i + 2; j < len(lines); j++ { // i+1 is the |---|---| separator
			r := lines[j]
			if !strings.HasPrefix(strings.TrimSpace(r), "|") {
				break
			}
			rows = append(rows, splitTableRow(r))
		}
		return header, rows
	}
	t.Fatalf("no table found under %q — the heading moved, or the parser is reading the wrong section", marker)
	return nil, nil
}

// splitTableRow splits one markdown table row into trimmed cells.
func splitTableRow(line string) []string {
	trimmed := strings.Trim(strings.TrimSpace(line), "|")
	parts := strings.Split(trimmed, "|")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		out = append(out, strings.TrimSpace(p))
	}
	return out
}

// exitCodesFromTable reads the named column of the table under heading in
// body, classifying every cell in it.
func exitCodesFromTable(t *testing.T, body, heading, column string) []int {
	t.Helper()
	header, rows := tableUnder(t, body, heading)
	col := -1
	for i, h := range header {
		if h == column {
			col = i
			break
		}
	}
	if col < 0 {
		t.Fatalf("%q: no %q column in header %v — the table was renamed, or the parser is reading the wrong table",
			heading, column, header)
	}
	var out []int
	for i, row := range rows {
		if col >= len(row) {
			continue
		}
		n, kind := classifyCodeCell(row[col])
		switch kind {
		case cellCode:
			out = append(out, n)
		case cellNone:
			// An explicit non-code, e.g. the CEL rows' em-dash Code cell.
		case cellBad:
			t.Errorf("%q: %s cell %q in row %d is neither an exit code nor an explicit non-code — "+
				"the row is malformed, or the parser is", heading, column, row[col], i)
		}
	}
	return out
}

// numberedExitRe matches a numbered-list item naming an exit code, e.g.
// "7. **Exit 130** — ...".
var numberedExitRe = regexp.MustCompile(`(?m)^\s*\d+\.\s+\*\*Exit\s+(\d+)\*\*`)

func exitCodesFromNumberedList(t *testing.T, body string) []int {
	t.Helper()
	return matchedInts(numberedExitRe, body)
}

// headingExitRe matches a per-exit-code section heading, e.g.
// "### Exit 130 — interrupted (SIGINT)".
var headingExitRe = regexp.MustCompile(`(?m)^###\s+Exit\s+(\d+)\s+—`)

func exitCodesFromHeadings(t *testing.T, body string) []int {
	t.Helper()
	return matchedInts(headingExitRe, body)
}

func matchedInts(re *regexp.Regexp, body string) []int {
	var out []int
	for _, m := range re.FindAllStringSubmatch(body, -1) {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			continue // the pattern only captures \d+; unreachable in practice
		}
		out = append(out, n)
	}
	return out
}

// sortedUniqueInts reduces a slice to sorted, deduplicated values, so two
// statements naming the same set in a different order (or, in principle,
// with an accidental repeat) compare equal.
func sortedUniqueInts(in []int) []int {
	seen := map[int]bool{}
	for _, v := range in {
		seen[v] = true
	}
	out := make([]int, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Ints(out)
	return out
}

// TestSkill_exit_codes_are_reachable asserts, in both directions, that the
// skill's exit-code contract matches what cmd/curlew can actually return.
func TestSkill_exit_codes_are_reachable(t *testing.T) {
	files := skillFiles(t)
	reachable := reachableExitCodes(t)
	reachableSet := map[int]bool{}
	for _, v := range exitcodes.Set(reachable) {
		reachableSet[v] = true
	}

	for _, stmt := range skillExitCodeStatements() {
		t.Run(stmt.file+"/"+stmt.sub, func(t *testing.T) {
			codes := readStatement(t, stmt, files)
			for _, c := range codes {
				if !reachableSet[c] {
					t.Errorf("%s (%s) documents exit %d, which cmd/curlew cannot return (reachable: %v)",
						stmt.file, stmt.what, c, exitcodes.Set(reachable))
				}
			}
		})
	}

	// The reverse: a code the binary can return that no statement documents
	// at all. Scoped to the union across all four statements, not each one
	// individually — a code present in three statements and missing from a
	// fourth is a disagreement between the skill's own statements (caught by
	// TestSkill_exit_code_statements_agree below), not an undocumented code.
	documented := map[int]bool{}
	for _, stmt := range skillExitCodeStatements() {
		for _, c := range readStatement(t, stmt, files) {
			documented[c] = true
		}
	}
	for _, c := range exitcodes.Set(reachable) {
		if !documented[c] {
			prov, _ := exitcodes.Find(reachable, c)
			t.Errorf("cmd/curlew can return exit %d (%s:%d, in %s) but no skill file documents it — "+
				"an agent hitting this code has no playbook", c, prov.File, prov.Line, prov.Fn)
		}
	}
}

// TestSkill_exit_code_statements_agree asserts the four statements of the
// exit-code contract name the same set of codes. Green from the moment the
// skill files are internally consistent; it is a regression guard against
// exactly the shape of drift this task exists to fix — SKILL.md's table
// changing without failure-playbook.md's headings following.
func TestSkill_exit_code_statements_agree(t *testing.T) {
	files := skillFiles(t)
	stmts := skillExitCodeStatements()

	type reading struct {
		stmt  skillStatement
		codes []int
	}
	readings := make([]reading, 0, len(stmts))
	for _, stmt := range stmts {
		readings = append(readings, reading{stmt, sortedUniqueInts(readStatement(t, stmt, files))})
	}

	first := readings[0]
	for _, r := range readings[1:] {
		if fmt.Sprint(r.codes) != fmt.Sprint(first.codes) {
			t.Errorf("%s (%s) lists %v; %s (%s) lists %v — the skill states the exit-code contract "+
				"in four places and they must agree",
				r.stmt.file, r.stmt.what, r.codes, first.stmt.file, first.stmt.what, first.codes)
		}
	}
}

func TestClassifyCodeCell(t *testing.T) {
	tests := []struct {
		name string
		cell string
		want int
		kind codeCellKind
	}{
		{"bare_integer_is_a_code", "3", 3, cellCode},
		{"backticked_integer_is_a_code", "`3`", 3, cellCode},
		{"em_dash_is_an_explicit_non_code", "—", 0, cellNone},
		{"empty_is_an_explicit_non_code", "", 0, cellNone},
		{"ERR_CEL_PARSE_is_malformed_not_skipped", "ERR_CEL_PARSE", 0, cellBad},
		{"word_six_is_malformed_not_skipped", "six", 0, cellBad},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			n, kind := classifyCodeCell(tc.cell)
			if kind != tc.kind {
				t.Errorf("classifyCodeCell(%q) kind = %v, want %v", tc.cell, kind, tc.kind)
			}
			if kind == cellCode && n != tc.want {
				t.Errorf("classifyCodeCell(%q) value = %d, want %d", tc.cell, n, tc.want)
			}
		})
	}
}
