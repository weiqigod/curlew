package docs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strconv"
	"strings"
)

// Claim is one test asking for one documentation table.
type Claim struct {
	File   string
	Line   int
	Doc    string
	Header []string
	// Fn is the reader being called, when the claim came from a call rather
	// than a table-driven case. It distinguishes a first-match reader from
	// AllTables, which credits every table its signature matches.
	Fn string
}

// AllTablesReader is the reader whose claims cover a whole family of tables.
const AllTablesReader = "AllTables"

// Claims returns every documentation table a test somewhere under root asks
// for, read out of the test sources themselves.
//
// Deriving the set from the source rather than from a hand-kept list is the
// whole point: a list of "tables we execute" is another document about the
// binary, and would drift exactly like the ones it claims to guard.
//
// A claim is any expression carrying a "*.md" string literal followed by
// further string literals — the header signature. Both shapes a test naturally
// takes are recognised:
//
//	docs.Table("MANUAL.md", "Code", "Meaning")
//	{doc: "MANUAL.md", header: []string{"Code", "Meaning"}}
//
// An over-collected signature cannot create a false claim: it simply matches no
// table, and the table it was meant to cover is reported unexecuted.
func Claims(root string) ([]Claim, error) {
	var claims []Claim
	fset := token.NewFileSet()

	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			switch d.Name() {
			case "node_modules", "vendor", ".git", "bin", "obj":
				return fs.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, "_test.go") {
			return nil
		}

		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return fmt.Errorf("parsing %s: %w", path, parseErr)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch n.(type) {
			case *ast.CallExpr, *ast.CompositeLit:
			default:
				return true
			}
			doc, header := signature(n)
			if doc == "" || len(header) == 0 {
				return true
			}
			claims = append(claims, Claim{
				File:   path,
				Line:   fset.Position(n.Pos()).Line,
				Doc:    doc,
				Header: header,
				Fn:     calleeName(n),
			})
			return true
		})
		return nil
	})
	if err != nil {
		return nil, err
	}
	return claims, nil
}

// calleeName returns the name of the function a call expression invokes,
// unqualified, or "" for anything that is not a call.
func calleeName(n ast.Node) string {
	call, ok := n.(*ast.CallExpr)
	if !ok {
		return ""
	}
	switch fn := call.Fun.(type) {
	case *ast.Ident:
		return fn.Name
	case *ast.SelectorExpr:
		return fn.Sel.Name
	}
	return ""
}

// signature pulls a document name and header signature out of one expression.
// Nested string slices win over trailing literals, so a table-driven case that
// carries other fields alongside the header still yields just the header.
func signature(n ast.Node) (doc string, header []string) {
	var (
		direct []string
		nested [][]string
	)
	switch e := n.(type) {
	case *ast.CallExpr:
		direct, nested = splitArgs(e.Args)
	case *ast.CompositeLit:
		direct, nested = splitArgs(e.Elts)
	default:
		return "", nil
	}

	for i, s := range direct {
		if !strings.HasSuffix(s, ".md") {
			continue
		}
		if len(nested) > 0 {
			// A case that carries both — {doc, where, []string{cells...}} —
			// means the section qualifier and the header together. Dropping the
			// qualifier would resolve every such case to the document's first
			// matching table and credit it for tables no test reads.
			return s, append(append([]string{}, direct[i+1:]...), nested[0]...)
		}
		return s, direct[i+1:]
	}
	return "", nil
}

// splitArgs separates an expression list into its own string literals and the
// string-literal groups held in any nested slice or key-value element.
func splitArgs(exprs []ast.Expr) (direct []string, nested [][]string) {
	for _, e := range exprs {
		if kv, ok := e.(*ast.KeyValueExpr); ok {
			e = kv.Value
		}
		switch v := e.(type) {
		case *ast.BasicLit:
			if s, ok := stringLit(v); ok {
				direct = append(direct, s)
			}
		case *ast.CompositeLit:
			if group := stringGroup(v); len(group) > 0 {
				nested = append(nested, group)
			}
		}
	}
	return direct, nested
}

// stringGroup returns the string literals of a composite literal, or nil when
// it holds anything else.
func stringGroup(c *ast.CompositeLit) []string {
	out := make([]string, 0, len(c.Elts))
	for _, e := range c.Elts {
		lit, ok := e.(*ast.BasicLit)
		if !ok {
			return nil
		}
		s, ok := stringLit(lit)
		if !ok {
			return nil
		}
		out = append(out, s)
	}
	return out
}

func stringLit(lit *ast.BasicLit) (string, bool) {
	if lit.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(lit.Value)
	if err != nil {
		return "", false
	}
	return s, true
}
