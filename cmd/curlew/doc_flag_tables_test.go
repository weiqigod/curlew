package main

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Eleven tables in the two documents list flags. Each was a promise that the
// binary accepts what the row names, and nothing checked any of them: a flag
// could be renamed in the parser, or removed with a feature, and its row would
// sit there being read by users.
//
// The accepted set is derived from the argument parsers themselves — the same
// walk help_parity_test.go uses, widened to short flags — so this cannot drift
// into agreeing with a list rather than with the binary.

// flagToken matches a long or short flag as a table cell spells it:
// `-X, --method <M>`, `-v` / `-vv` / `-q`, `--var k=v`.
var flagToken = regexp.MustCompile(`--?[A-Za-z][A-Za-z0-9-]*`)

// acceptedArgTokens returns every flag-shaped string literal the parsers in
// this package compare an argument against.
//
// Flags are accepted two ways — a `case "--flag":` in a switch and an
// `if a == "--flag"` outside one — and reading only the first missed `--clear`
// on `curlew watch` entirely.
func acceptedArgTokens(t *testing.T) map[string]bool {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	seen := map[string]bool{}
	fset := token.NewFileSet()

	record := func(lit *ast.BasicLit) {
		if lit.Kind != token.STRING {
			return
		}
		val, unquoteErr := strconv.Unquote(lit.Value)
		if unquoteErr != nil {
			return
		}
		if len(val) > 1 && strings.HasPrefix(val, "-") {
			seen[val] = true
		}
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, parseErr := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", name, parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch node := n.(type) {
			case *ast.CaseClause:
				for _, expr := range node.List {
					if lit, ok := expr.(*ast.BasicLit); ok {
						record(lit)
					}
				}
			case *ast.BinaryExpr:
				if node.Op != token.EQL {
					return true
				}
				if lit, ok := node.X.(*ast.BasicLit); ok {
					record(lit)
				}
				if lit, ok := node.Y.(*ast.BasicLit); ok {
					record(lit)
				}
			}
			return true
		})
	}

	if len(seen) == 0 {
		t.Fatal("found no accepted flags — the AST walk is broken, not the CLI")
	}
	return seen
}

// docFlagTable names one table. Every one of them holds its flags in a column
// called "Flag"; flagColumn is that name, not a per-table choice.
type docFlagTable struct {
	doc    string
	where  string   // heading or lead-in that picks this table out
	header []string // cells that identify it
}

const flagColumn = "Flag"

// The eleven flag tables. A table missing from this list is caught by the
// inventory test, not by this one.
var docFlagTables = []docFlagTable{
	{"MANUAL.md", "curlew exec", []string{"Flag", "Meaning"}},
	{"MANUAL.md", "curlew ui", []string{"Flag", "Meaning"}},
	{"MANUAL.md", "9.2 Performance testing", []string{"Flag", "Meaning"}},
	{"MANUAL.md", "4.2 Streams, verbosity", []string{"Flag", "Shows"}},
	{"CLI_SPECIFICATION.md", "18.1", []string{"Flag", "Meaning"}},
	{"CLI_SPECIFICATION.md", "18.2", []string{"Flag", "Meaning"}},
	{"CLI_SPECIFICATION.md", "18.4", []string{"Flag", "Meaning"}},
	{"CLI_SPECIFICATION.md", "18.9", []string{"Flag", "Meaning"}},
	{"CLI_SPECIFICATION.md", "20. Local Web UI", []string{"Flag", "Meaning"}},
	{"CLI_SPECIFICATION.md", "21. Performance Testing", []string{"Flag", "Required", "Meaning"}},
	{"CLI_SPECIFICATION.md", "16.3 Verbosity", []string{"Flag", "Level", "Shows"}},
}

func TestDocTables_everyDocumentedFlagIsAccepted(t *testing.T) {
	accepted := acceptedArgTokens(t)

	checked := 0
	for _, ft := range docFlagTables {
		header, rows, err := docs.TableUnder(ft.doc, ft.where, ft.header...)
		if err != nil {
			t.Errorf("%s under %q: %v", ft.doc, ft.where, err)
			continue
		}
		col := docs.Column(header, flagColumn)
		if col < 0 {
			t.Errorf("%s under %q: no %q column in %v", ft.doc, ft.where, flagColumn, header)
			continue
		}
		if len(rows) == 0 {
			t.Errorf("%s under %q: table has no rows", ft.doc, ft.where)
			continue
		}

		for _, row := range rows {
			if len(row) <= col {
				continue
			}
			for _, flag := range flagToken.FindAllString(row[col], -1) {
				checked++
				if !accepted[flag] {
					t.Errorf("%s under %q documents %s, which no parser in cmd/curlew accepts",
						ft.doc, ft.where, flag)
				}
			}
		}
	}

	// A table that yields no flags at all would pass silently, which is the
	// failure mode this whole exercise exists to remove.
	if checked == 0 {
		t.Fatal("no flags extracted from any documented table; the column lookup is broken")
	}
	t.Logf("%d documented flag mentions checked against the parsers", checked)
}
