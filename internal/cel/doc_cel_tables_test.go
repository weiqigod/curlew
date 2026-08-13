package cel

import (
	"errors"
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"strings"
	"testing"

	celgo "github.com/google/cel-go/cel"

	"github.com/weiqigod/curlew/internal/docs"
)

// The manual's three CEL tables: the bindings every expression evaluates
// against, the functions deliberately blocked, and the error codes a bad
// expression produces.
//
// The disabled-function table is the one with teeth. Blocking `now()` is what
// makes a run replayable, and a table saying so is worth nothing unless calling
// it is actually refused.

// celVariables reads the celgo.Variable("name", ...) declarations out of
// cel.go, so the binding set comes from the environment the evaluator builds.
func celVariables(t *testing.T) map[string]bool {
	t.Helper()

	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "cel.go", nil, 0)
	if err != nil {
		t.Fatalf("parse cel.go: %v", err)
	}

	found := map[string]bool{}
	ast.Inspect(file, func(n ast.Node) bool {
		call, ok := n.(*ast.CallExpr)
		if !ok || len(call.Args) == 0 {
			return true
		}
		sel, isSel := call.Fun.(*ast.SelectorExpr)
		if !isSel || sel.Sel.Name != "Variable" {
			return true
		}
		lit, isLit := call.Args[0].(*ast.BasicLit)
		if !isLit || lit.Kind != token.STRING {
			return true
		}
		if name, unquoteErr := strconv.Unquote(lit.Value); unquoteErr == nil {
			found[name] = true
		}
		return true
	})

	if len(found) == 0 {
		t.Fatal("no CEL variables found in cel.go; the AST walk is broken, not the environment")
	}
	return found
}

func TestDocTables_celBindingsAreTheOnesDeclared(t *testing.T) {
	declared := celVariables(t)

	hdr, rows, err := docs.TableUnder("MANUAL.md", "Standard activation",
		"Binding", "Type", "Description")
	if err != nil {
		t.Fatalf("CEL binding table: %v", err)
	}
	col := docs.Column(hdr, "Binding")
	if col < 0 {
		t.Fatalf("binding table lost its Binding column: %v", hdr)
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
		if !declared[name] {
			t.Errorf("the manual documents the CEL binding %q, which the environment does not declare", name)
		}
	}
	if len(documented) == 0 {
		t.Fatal("no bindings read from the table; the column lookup is broken")
	}
	for name := range declared {
		if !documented[name] {
			t.Errorf("the CEL environment declares %q, which the manual's binding table omits", name)
		}
	}

	// The prose above the table counts them.
	manual, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}
	if len(declared) == 4 && !strings.Contains(manual, "the same four bindings") {
		t.Errorf("there are 4 bindings and the manual no longer says so")
	}
}

// Every function the table says is blocked must actually be refused.
func TestDocTables_everyBlockedFunctionIsRefused(t *testing.T) {
	ev, err := NewEvaluator()
	if err != nil {
		t.Fatalf("new evaluator: %v", err)
	}

	hdr, rows, err := docs.Table("MANUAL.md", "Function", "Why blocked")
	if err != nil {
		t.Fatalf("disabled function table: %v", err)
	}
	col := docs.Column(hdr, "Function")
	if col < 0 {
		t.Fatalf("disabled function table lost its Function column: %v", hdr)
	}

	// Premise: an ordinary function must compile. Without this the test passes
	// just as happily against an evaluator that refuses everything.
	if _, controlErr := ev.Compile(`size("ab") == 2`, celgo.BoolType); controlErr != nil {
		t.Fatalf("a permitted function does not compile either, so refusals prove nothing: %v", controlErr)
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		call := docs.FirstName(row[col])
		if call == "" {
			continue
		}
		if !strings.HasSuffix(call, ")") {
			call += "()"
		}
		checked++

		// Compared, so a refusal is about the function rather than about the
		// expression's result type.
		expr := call + " == null"
		if _, compileErr := ev.Compile(expr, celgo.BoolType); compileErr == nil {
			t.Errorf("the manual says %s is blocked, but %q compiles", call, expr)
		}
	}
	if checked == 0 {
		t.Fatal("no blocked functions read from the table; it moved or its header changed")
	}
}

// The error-code table names the codes a bad expression produces. Both are
// registered hints, and both must be reachable from a real expression.
func TestDocTables_celErrorCodesAreProduced(t *testing.T) {
	ev, err := NewEvaluator()
	if err != nil {
		t.Fatalf("new evaluator: %v", err)
	}

	hdr, rows, err := docs.TableUnder("MANUAL.md", "Validation error codes", "Code", "Meaning")
	if err != nil {
		t.Fatalf("CEL error code table: %v", err)
	}
	col := docs.Column(hdr, "Code")
	if col < 0 {
		t.Fatalf("error code table lost its Code column: %v", hdr)
	}

	// An expression that produces each documented code, and the sentinel that
	// carries it.
	producers := map[string]struct {
		expr     string
		sentinel error
	}{
		"ERR_CEL_PARSE": {"response.status ===", ErrCelParse}, // syntactically invalid
		"ERR_CEL_TYPE":  {"response.status + 1", ErrCelType},  // compiles, result is not bool
	}

	documented := 0
	for _, row := range rows {
		if len(row) <= col {
			continue
		}
		code := docs.FirstName(row[col])
		if code == "" {
			continue
		}
		documented++

		p, known := producers[code]
		if !known {
			t.Errorf("the manual documents CEL error code %q, which this test cannot produce; "+
				"add an expression for it or correct the row", code)
			continue
		}
		_, compileErr := ev.Compile(p.expr, celgo.BoolType)
		if compileErr == nil {
			t.Errorf("%q was expected to fail with %s and compiled", p.expr, code)
			continue
		}
		if !errors.Is(compileErr, p.sentinel) {
			t.Errorf("%q was expected to carry %s; got %v", p.expr, code, compileErr)
		}
	}
	if documented == 0 {
		t.Fatal("no error codes read from the table; it moved or its header changed")
	}
}
