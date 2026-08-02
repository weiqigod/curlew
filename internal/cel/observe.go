package cel

import (
	"sort"

	celgo "github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
)

// collectVarsRefs returns the sorted, deduplicated set of names X such that
// the expression references vars.X (a Select whose Operand is the identifier
// "vars").
func collectVarsRefs(ast *celgo.Ast) []string {
	seen := map[string]struct{}{}
	celast.PostOrderVisit(ast.NativeRep().Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if e.Kind() != celast.SelectKind {
			return
		}
		sel := e.AsSelect()
		op := sel.Operand()
		if op.Kind() != celast.IdentKind || op.AsIdent() != "vars" {
			return
		}
		seen[sel.FieldName()] = struct{}{}
	}))
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}
