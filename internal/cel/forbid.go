package cel

import (
	"fmt"

	celgo "github.com/google/cel-go/cel"
	celast "github.com/google/cel-go/common/ast"
)

// rejectTimeOfDay walks the parsed AST and returns a non-nil error if the
// expression references any disallowed time-of-day functions:
//   - now() with any argument count
//   - timestamp() with zero arguments
//
// The single-argument form timestamp(string) remains available because it is
// deterministic.
func rejectTimeOfDay(ast *celgo.Ast) error {
	var firstErr error
	celast.PostOrderVisit(ast.NativeRep().Expr(), celast.NewExprVisitor(func(e celast.Expr) {
		if firstErr != nil {
			return
		}
		if e.Kind() != celast.CallKind {
			return
		}
		call := e.AsCall()
		switch call.FunctionName() {
		case "now":
			firstErr = fmt.Errorf("now() is not allowed (time-of-day functions are disabled)")
		case "timestamp":
			if len(call.Args()) == 0 {
				firstErr = fmt.Errorf("zero-argument timestamp() is not allowed; use timestamp(string)")
			}
		}
	}))
	return firstErr
}
