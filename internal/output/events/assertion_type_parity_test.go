package events_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// M24-001: the assertion `type` field is a discriminator, and for three years
// it was not one.
//
// The published schema has declared enum ["status","body","header","schema"]
// since v1.0, but the assertion package built the field with
// fmt.Sprintf("body %s %s", path, operator), so every body, header, schema and
// cel assertion violated it — and "timing" was never in the enum at all. Of
// the four declared members exactly one ("status") was ever emitted verbatim.
//
// The existing schema tests did not catch it because every golden fixture and
// every direct EmitAssertionResult call used "status": the single conforming
// value. A corpus that only exercises the passing case is not a guard.
//
// This test derives the emitted vocabulary from the assertion package's source
// and holds it to the published enum in BOTH directions, so that neither a new
// unlisted type nor a stale unemitted enum member can survive.

// assertionTypeLiterals walks the assertion package and returns every value
// assigned to a Result.Type field. Assignments must be compile-time constants —
// either a string literal or one of the package's Type* constants. Anything
// computed fails the test outright: a runtime-derived discriminator is exactly
// the defect this task removed, and allowing one back would let the composite
// string return unnoticed.
func assertionTypeLiterals(t *testing.T) []string {
	t.Helper()

	// Walk the whole tree, not just internal/assertion: assertion.Result is a
	// plain struct and any package can build one. The runner does — it appends
	// a "graphql_error" result of its own — and a walk scoped to the assertion
	// package would have declared the vocabulary complete without it.
	root := filepath.Join("..", "..", "..")
	var paths []string
	for _, sub := range []string{"cmd", "internal"} {
		err := filepath.WalkDir(filepath.Join(root, sub), func(path string, d os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			paths = append(paths, path)
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", sub, err)
		}
	}
	if len(paths) == 0 {
		t.Fatal("walked no source files — the walk is broken, not the tree")
	}

	fset := token.NewFileSet()
	parsed := make(map[string]*ast.File, len(paths))
	for _, path := range paths {
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}
		parsed[path] = file
	}

	// Resolve the Type* constants first so an assignment naming one counts as
	// the string it holds.
	consts := map[string]string{}
	for _, file := range parsed {
		for _, decl := range file.Decls {
			gd, ok := decl.(*ast.GenDecl)
			if !ok || gd.Tok != token.CONST {
				continue
			}
			for _, spec := range gd.Specs {
				vs, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for i, ident := range vs.Names {
					if i >= len(vs.Values) {
						continue
					}
					lit, ok := vs.Values[i].(*ast.BasicLit)
					if !ok || lit.Kind != token.STRING {
						continue
					}
					if val, unqErr := strconv.Unquote(lit.Value); unqErr == nil {
						consts[ident.Name] = val
					}
				}
			}
		}
	}

	seen := map[string]bool{}
	for name, file := range parsed {
		ast.Inspect(file, func(n ast.Node) bool {
			lit, ok := n.(*ast.CompositeLit)
			if !ok || !isAssertionResultType(lit.Type) {
				return true
			}
			for _, el := range lit.Elts {
				kv, ok := el.(*ast.KeyValueExpr)
				if !ok {
					continue
				}
				key, ok := kv.Key.(*ast.Ident)
				if !ok || key.Name != "Type" {
					continue
				}
				switch v := kv.Value.(type) {
				case *ast.BasicLit:
					if v.Kind == token.STRING {
						if val, unqErr := strconv.Unquote(v.Value); unqErr == nil && val != "" {
							seen[val] = true
						}
					}
				case *ast.Ident:
					val, known := consts[v.Name]
					if !known {
						t.Errorf("%s:%d: Result.Type is assigned %s, which is not a Type* constant in internal/assertion.",
							name, fset.Position(kv.Pos()).Line, v.Name)
						continue
					}
					seen[val] = true
				case *ast.SelectorExpr:
					// assertion.TypeGraphQLError and friends, from outside the package.
					val, known := consts[v.Sel.Name]
					if !known {
						t.Errorf("%s:%d: Result.Type is assigned %s, which is not a Type* constant in internal/assertion.",
							name, fset.Position(kv.Pos()).Line, v.Sel.Name)
						continue
					}
					seen[val] = true
				default:
					t.Errorf("%s:%d: Result.Type is assigned a computed value.\n"+
						"The discriminator must be a compile-time constant from the published enum;\n"+
						"anything derived at runtime is how the composite \"body <path> <op>\" string got in.",
						name, fset.Position(kv.Pos()).Line)
				}
			}
			return true
		})
	}

	if len(seen) == 0 {
		t.Fatal("found no Result.Type assignments — the AST walk is broken, not the assertion package")
	}

	out := make([]string, 0, len(seen))
	for v := range seen {
		out = append(out, v)
	}
	sort.Strings(out)
	return out
}

// isAssertionResultType reports whether a composite literal's type is
// assertion.Result — as `Result` inside the package, or `assertion.Result`
// outside it. Matching on the literal's type (rather than on any `Type:` key)
// keeps unrelated structs that happen to have a Type field out of the walk.
func isAssertionResultType(expr ast.Expr) bool {
	switch t := expr.(type) {
	case *ast.Ident:
		return t.Name == "Result"
	case *ast.SelectorExpr:
		pkg, ok := t.X.(*ast.Ident)
		return ok && pkg.Name == "assertion" && t.Sel.Name == "Result"
	}
	return false
}

// publishedAssertionEnum reads the enum the current schema version declares for
// assertion.result.type.
func publishedAssertionEnum(t *testing.T, version string) []string {
	t.Helper()

	path := filepath.Join("..", "..", "..", "docs", "events-schema", version+".json")
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}

	var doc struct {
		Definitions struct {
			AssertionResult struct {
				Properties struct {
					Type struct {
						Enum []string `json:"enum"`
					} `json:"type"`
				} `json:"properties"`
			} `json:"AssertionResult"`
		} `json:"definitions"`
	}
	if err := json.Unmarshal(raw, &doc); err != nil {
		t.Fatalf("parse %s: %v", path, err)
	}

	got := doc.Definitions.AssertionResult.Properties.Type.Enum
	if len(got) == 0 {
		t.Fatalf("%s declares no enum for assertion.result.type — the schema lookup is broken, not the schema", path)
	}
	sort.Strings(got)
	return got
}

func TestAssertionType_conforms_to_published_enum(t *testing.T) {
	emitted := assertionTypeLiterals(t)
	declared := publishedAssertionEnum(t, "v1.5")

	declaredSet := map[string]bool{}
	for _, d := range declared {
		declaredSet[d] = true
	}
	emittedSet := map[string]bool{}
	for _, e := range emitted {
		emittedSet[e] = true
	}

	for _, e := range emitted {
		if !declaredSet[e] {
			t.Errorf("the assertion package emits type %q, which the published schema does not permit.\n"+
				"declared: %v", e, declared)
		}
	}

	// The reverse direction matters just as much: "header" and "schema" sat in
	// the enum for four versions while nothing ever emitted them verbatim, and
	// that silence is what made the violation look like conformance.
	for _, d := range declared {
		if !emittedSet[d] {
			t.Errorf("the published schema declares type %q, but nothing in the assertion package emits it.\n"+
				"emitted: %v", d, emitted)
		}
	}
}
