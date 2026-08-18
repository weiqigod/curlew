package docs

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
)

// ProseClaim is one test asking for one documented prose claim.
type ProseClaim struct {
	File   string
	Line   int
	Doc    string
	Substr string
}

// ProseClaims returns every prose claim a test somewhere under root asks
// for, read out of the test sources themselves rather than a hand-kept
// list -- the property that made docs.Claims (and the table register it
// backs) trustworthy.
//
// A claim is any call to a function named Prose, qualified or not, whose
// first argument is a ".md" string literal and whose second argument is a
// string literal. Both are required: a call built from variables cannot be
// resolved statically and is silently not a claim, the same way an
// over-collected table signature simply matches no table rather than
// fabricating one.
func ProseClaims(root string) ([]ProseClaim, error) {
	var claims []ProseClaim
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
			call, ok := n.(*ast.CallExpr)
			if !ok || calleeName(call) != "Prose" || len(call.Args) != 2 {
				return true
			}
			docLit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			substrLit, ok := call.Args[1].(*ast.BasicLit)
			if !ok {
				return true
			}
			doc, ok := stringLit(docLit)
			if !ok || !strings.HasSuffix(doc, ".md") {
				return true
			}
			substr, ok := stringLit(substrLit)
			if !ok {
				return true
			}
			claims = append(claims, ProseClaim{
				File:   path,
				Line:   fset.Position(n.Pos()).Line,
				Doc:    doc,
				Substr: substr,
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

// Prose returns the text of the claim in doc containing substr.
//
// It errors when substr matches no claim -- a stale executor, testing
// something the document no longer says -- or more than one, so a test
// cannot silently cover the wrong sentence when the document grows a second
// claim that happens to share the same words.
func Prose(doc, substr string) (string, error) {
	refs, err := ProseInventory(doc)
	if err != nil {
		return "", err
	}

	var matches []ProseRef
	for _, r := range refs {
		if strings.Contains(r.Text, substr) {
			matches = append(matches, r)
		}
	}

	switch len(matches) {
	case 0:
		return "", fmt.Errorf("no claim in %s matches %q", doc, substr)
	case 1:
		return matches[0].Text, nil
	default:
		var lines []string
		for _, m := range matches {
			lines = append(lines, fmt.Sprintf("  %s:%d %s", m.Doc, m.Line, m.Text))
		}
		return "", fmt.Errorf("%q matches %d claims in %s, ambiguous:\n%s",
			substr, len(matches), doc, strings.Join(lines, "\n"))
	}
}
