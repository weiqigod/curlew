package main

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/signer"
	_ "github.com/weiqigod/curlew/internal/signer/awssigv4"
	_ "github.com/weiqigod/curlew/internal/signer/oauth1"
)

// Tables naming things that live in a registry: signer types and vault
// providers. Both are lists a reader copies into a collection, and a name that
// has drifted produces an error at run time rather than at edit time.

// signerTables are the two that list signer types.
var signerTables = []struct {
	doc    string
	where  string
	header []string
}{
	{"CLI_SPECIFICATION.md", "14. Request Signing", []string{"Type", "Purpose"}},
	{"MANUAL.md", "6.8 Request signing", []string{"Type", "Required params", "Optional params", "Summary"}},
}

func TestDocTables_everyDocumentedSignerTypeIsRegistered(t *testing.T) {
	registry := signer.NewWithBuiltins()

	for _, tbl := range signerTables {
		hdr, rows, err := docs.TableUnder(tbl.doc, tbl.where, tbl.header...)
		if err != nil {
			t.Errorf("%s under %q: %v", tbl.doc, tbl.where, err)
			continue
		}
		col := docs.Column(hdr, "Type")
		if col < 0 {
			t.Errorf("%s under %q: no Type column in %v", tbl.doc, tbl.where, hdr)
			continue
		}

		documented := 0
		for _, row := range rows {
			if len(row) <= col {
				continue
			}
			name := docs.FirstName(row[col])
			if name == "" {
				continue
			}
			documented++
			if _, lookupErr := registry.Lookup(name); lookupErr != nil {
				if errors.Is(lookupErr, signer.ErrUnknownSignerType) {
					t.Errorf("%s under %q documents signer type %q, which is not registered",
						tbl.doc, tbl.where, name)
					continue
				}
				t.Errorf("%s under %q: looking up %q: %v", tbl.doc, tbl.where, name, lookupErr)
			}
		}
		if documented == 0 {
			t.Errorf("%s under %q: no signer types read from the table", tbl.doc, tbl.where)
		}
	}
}

// The vault provider names are constants, so both directions are checkable:
// no documented provider may be unknown, and no constant may go undocumented.
func TestDocTables_vaultProviderTableMatchesTheConstants(t *testing.T) {
	known := vaultProviderConstants(t)

	hdr, rows, err := docs.TableUnder("CLI_SPECIFICATION.md", "13.3 Vault Providers",
		"Provider", "Key form", "Requires")
	if err != nil {
		t.Fatalf("vault provider table: %v", err)
	}
	col := docs.Column(hdr, "Provider")
	if col < 0 {
		t.Fatalf("vault table lost its Provider column: %v", hdr)
	}

	documented := map[string]bool{}
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		name := docs.FirstName(row[col])
		if name == "" {
			continue
		}
		documented[name] = true
		if !known[name] {
			t.Errorf("§13.3 documents vault provider %q, which no Provider constant names (%v)",
				name, sortedNames(known))
		}
	}
	if len(documented) == 0 {
		t.Fatal("no providers read from §13.3; the column lookup is broken")
	}
	for name := range known {
		if !documented[name] {
			t.Errorf("vault provider %q is a Provider constant with no row in §13.3", name)
		}
	}
}

// vaultProviderConstants reads the Provider* constants out of internal/vault,
// so the set comes from the source rather than from a list restated here.
func vaultProviderConstants(t *testing.T) map[string]bool {
	t.Helper()

	dir := filepath.Join("..", "..", "internal", "vault")
	found := map[string]bool{}
	fset := token.NewFileSet()

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() && path != dir {
			return fs.SkipDir // the provider constants live in the package root
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		file, parseErr := parser.ParseFile(fset, path, nil, 0)
		if parseErr != nil {
			return parseErr
		}
		ast.Inspect(file, func(n ast.Node) bool {
			spec, ok := n.(*ast.ValueSpec)
			if !ok {
				return true
			}
			for i, name := range spec.Names {
				if !strings.HasPrefix(name.Name, "Provider") || i >= len(spec.Values) {
					continue
				}
				lit, isLit := spec.Values[i].(*ast.BasicLit)
				if !isLit || lit.Kind != token.STRING {
					continue
				}
				if val, unquoteErr := strconv.Unquote(lit.Value); unquoteErr == nil && val != "" {
					found[val] = true
				}
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatalf("walking internal/vault: %v", err)
	}
	if len(found) == 0 {
		t.Fatal("no Provider constants found; the AST walk is broken, not the package")
	}
	return found
}

func sortedNames(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
