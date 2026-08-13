package hooks

import (
	"go/ast"
	"go/parser"
	"go/token"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Both documents list the three plugin hooks and what each may change.
//
// The "may do" column is the interesting half. It is a security-adjacent claim
// — on_response says status, headers and body are **not** replaceable — and it
// was written in prose beside a struct that decides the matter. Here the column
// is read against the payload types themselves.

// dispatchedHooks reads the hook names the dispatcher actually routes, from the
// `case "on_request":` clauses in hooks.go, rather than from a list restated
// here that could drift from the switch it describes.
func dispatchedHooks(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "hooks.go", nil, 0)
	if err != nil {
		t.Fatalf("parse hooks.go: %v", err)
	}

	found := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		cc, ok := n.(*ast.CaseClause)
		if !ok {
			return true
		}
		for _, expr := range cc.List {
			lit, isLit := expr.(*ast.BasicLit)
			if !isLit || lit.Kind != token.STRING {
				continue
			}
			val, unquoteErr := strconv.Unquote(lit.Value)
			if unquoteErr == nil && strings.HasPrefix(val, "on_") {
				found[val] = true
			}
		}
		return true
	})

	if len(found) == 0 {
		t.Fatal("no on_* hooks found in hooks.go; the AST walk is broken, not the dispatcher")
	}
	return found
}

// fieldNames returns the JSON names of a payload struct's fields, which is how
// both documents spell them.
func fieldNames(v any) map[string]bool {
	out := map[string]bool{}
	rt := reflect.TypeOf(v)
	for i := 0; i < rt.NumField(); i++ {
		tag := rt.Field(i).Tag.Get("json")
		name := strings.Split(tag, ",")[0]
		if name == "" || name == "-" {
			name = strings.ToLower(rt.Field(i).Name)
		}
		out[name] = true
	}
	return out
}

var hookTables = []struct {
	doc       string
	where     string
	header    []string
	mutateCol string
}{
	{"CLI_SPECIFICATION.md", "15.3 Hooks", []string{"Hook", "When", "May do"}, "May do"},
	{"MANUAL.md", "10.1 What plugins can do", []string{"Hook", "When it fires", "Can mutate"}, "Can mutate"},
}

func TestDocTables_hookTablesMatchTheDispatcher(t *testing.T) {
	dispatched := dispatchedHooks(t)

	for _, ht := range hookTables {
		hdr, rows, err := docs.TableUnder(ht.doc, ht.where, ht.header...)
		if err != nil {
			t.Errorf("%s under %q: %v", ht.doc, ht.where, err)
			continue
		}
		col := docs.Column(hdr, "Hook")
		if col < 0 {
			t.Errorf("%s under %q: no Hook column in %v", ht.doc, ht.where, hdr)
			continue
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
			if !dispatched[name] {
				t.Errorf("%s under %q documents hook %q, which the dispatcher does not route",
					ht.doc, ht.where, name)
			}
		}
		if len(documented) == 0 {
			t.Errorf("%s under %q: no hooks read from the table", ht.doc, ht.where)
			continue
		}
		for name := range dispatched {
			if !documented[name] {
				t.Errorf("%s under %q omits hook %q, which the dispatcher routes",
					ht.doc, ht.where, name)
			}
		}
	}
}

// mentions reports whether a cell names a payload field.
//
// The two documents spell them differently — the manual writes `query_params`,
// the specification writes "query parameters" — and both are naming the same
// field. A check that insists on one spelling tests house style rather than the
// claim, so every word of the field must appear in the cell, matched by prefix
// so "params" finds "parameters".
func mentions(cell, field string) bool {
	cellWords := strings.FieldsFunc(strings.ToLower(cell), func(r rune) bool {
		return r < 'a' || r > 'z'
	})
	for _, want := range strings.Split(field, "_") {
		want = strings.TrimSuffix(want, "s")
		if want == "" {
			continue
		}
		found := false
		for _, got := range cellWords {
			got = strings.TrimSuffix(got, "s")
			if strings.HasPrefix(got, want) || strings.HasPrefix(want, got) {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func TestDocTables_onRequestMayMutateExactlyItsPayload(t *testing.T) {
	request := fieldNames(RequestPayload{})

	// Fields carried by the response but never by the request. A "may mutate"
	// cell naming one of these would be promising a plugin something the
	// dispatcher cannot give it.
	responseOnly := []string{}
	for f := range fieldNames(ResponsePayload{}) {
		if !request[f] {
			responseOnly = append(responseOnly, f)
		}
	}
	sort.Strings(responseOnly)

	for _, ht := range hookTables {
		hdr, rows, err := docs.TableUnder(ht.doc, ht.where, ht.header...)
		if err != nil {
			t.Fatalf("%s under %q: %v", ht.doc, ht.where, err)
		}
		hookCol, mutCol := docs.Column(hdr, "Hook"), docs.Column(hdr, ht.mutateCol)
		if hookCol < 0 || mutCol < 0 {
			t.Fatalf("%s under %q lost a column: %v", ht.doc, ht.where, hdr)
		}

		seen := 0
		for _, row := range rows {
			if len(row) <= mutCol || docs.FirstName(row[hookCol]) != "on_request" {
				continue
			}
			seen++
			cell := row[mutCol]

			for f := range request {
				if !mentions(cell, f) {
					t.Errorf("%s under %q omits %q from what on_request may mutate, though RequestPayload carries it: %q",
						ht.doc, ht.where, f, cell)
				}
			}
			for _, f := range responseOnly {
				if mentions(cell, f) {
					t.Errorf("%s under %q says on_request may mutate %q, which only the response payload carries: %q",
						ht.doc, ht.where, f, cell)
				}
			}
		}
		if seen == 0 {
			t.Errorf("%s under %q: no on_request row found", ht.doc, ht.where)
		}
	}
}

// The read-only claim about on_response is the one worth pinning: a plugin may
// append annotations and must not be able to replace what the server said.
func TestDocTables_onResponseSaysWhatItCannotReplace(t *testing.T) {
	response := fieldNames(ResponsePayload{})
	for _, want := range []string{"annotations", "status_code", "headers", "body"} {
		if !response[want] {
			t.Fatalf("ResponsePayload no longer carries %q; the tables describe a different type", want)
		}
	}

	for _, ht := range hookTables {
		hdr, rows, err := docs.TableUnder(ht.doc, ht.where, ht.header...)
		if err != nil {
			t.Fatalf("%s under %q: %v", ht.doc, ht.where, err)
		}
		hookCol, mutCol := docs.Column(hdr, "Hook"), docs.Column(hdr, ht.mutateCol)

		for _, row := range rows {
			if len(row) <= mutCol || docs.FirstName(row[hookCol]) != "on_response" {
				continue
			}
			cell := strings.ToLower(row[mutCol])
			if !strings.Contains(cell, "annotation") {
				t.Errorf("%s under %q no longer says on_response may append annotations: %q",
					ht.doc, ht.where, row[mutCol])
			}
			if !strings.Contains(cell, "not") {
				t.Errorf("%s under %q dropped the claim that on_response cannot replace the response: %q",
					ht.doc, ht.where, row[mutCol])
			}
		}
	}
}
