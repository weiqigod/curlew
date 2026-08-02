package datadriven

import (
	"errors"
	"fmt"
	"strings"
)

// ErrInvalidFilter is returned when a filter expression cannot be parsed.
var ErrInvalidFilter = errors.New("invalid filter expression")

// EvalFilter evaluates a filter expression against a data row.
// Variable references like {{col}} are resolved from the row.
// Returns true if the row matches the filter, false otherwise.
// An empty expression always returns true.
// Missing variables resolve to empty string (filter returns false for comparisons).
func EvalFilter(expr string, row Row) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	resolved, err := resolveVars(expr, row)
	if err != nil {
		return false, err
	}

	return evalExpr(resolved)
}

// resolveVars replaces all {{var}} references with their values from the row.
// Missing variables resolve to empty string. Returns ErrInvalidFilter if
// a variable reference is not properly closed.
func resolveVars(expr string, row Row) (string, error) {
	var result strings.Builder
	i := 0
	for i < len(expr) {
		if i+1 < len(expr) && expr[i] == '{' && expr[i+1] == '{' {
			end := strings.Index(expr[i:], "}}")
			if end < 0 {
				return "", fmt.Errorf("%w: unclosed variable reference at position %d", ErrInvalidFilter, i)
			}
			varName := strings.TrimSpace(expr[i+2 : i+end])
			val := row[varName] // empty string if missing
			result.WriteString(val)
			i += end + 2
		} else {
			result.WriteByte(expr[i])
			i++
		}
	}
	return result.String(), nil
}

// evalExpr evaluates a boolean expression with AND/OR/NOT operators.
// Precedence: NOT > AND > OR (left-associative).
func evalExpr(expr string) (bool, error) {
	return evalOr(expr)
}

// evalOr splits on " OR " and evaluates each part.
func evalOr(expr string) (bool, error) {
	parts := splitOnKeyword(expr, " OR ")
	if len(parts) == 1 {
		return evalAnd(expr)
	}

	for _, part := range parts {
		result, err := evalAnd(part)
		if err != nil {
			return false, err
		}
		if result {
			return true, nil
		}
	}
	return false, nil
}

// evalAnd splits on " AND " and evaluates each part.
func evalAnd(expr string) (bool, error) {
	parts := splitOnKeyword(expr, " AND ")
	if len(parts) == 1 {
		return evalNot(expr)
	}

	for _, part := range parts {
		result, err := evalNot(part)
		if err != nil {
			return false, err
		}
		if !result {
			return false, nil
		}
	}
	return true, nil
}

// evalNot handles the NOT prefix operator.
func evalNot(expr string) (bool, error) {
	trimmed := strings.TrimSpace(expr)
	if strings.HasPrefix(trimmed, "NOT ") {
		inner := strings.TrimPrefix(trimmed, "NOT ")
		result, err := evalComparison(inner)
		if err != nil {
			return false, err
		}
		return !result, nil
	}
	return evalComparison(trimmed)
}

// evalComparison evaluates a single comparison expression like "value op value".
func evalComparison(expr string) (bool, error) {
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return true, nil
	}

	// Try each operator in order of specificity (multi-char before single-char)
	operators := []struct {
		op string
		fn func(left, right string) bool
	}{
		{"starts_with", func(l, r string) bool { return strings.HasPrefix(l, r) }},
		{"ends_with", func(l, r string) bool { return strings.HasSuffix(l, r) }},
		{"contains", func(l, r string) bool { return strings.Contains(l, r) }},
		{">=", func(l, r string) bool { return compareValues(l, r) >= 0 }},
		{"<=", func(l, r string) bool { return compareValues(l, r) <= 0 }},
		{"!=", func(l, r string) bool { return compareValues(l, r) != 0 }},
		{"==", func(l, r string) bool { return compareValues(l, r) == 0 }},
		{">", func(l, r string) bool { return compareValues(l, r) > 0 }},
		{"<", func(l, r string) bool { return compareValues(l, r) < 0 }},
	}

	for _, op := range operators {
		idx := findOperator(expr, op.op)
		if idx < 0 {
			continue
		}
		left := strings.TrimSpace(expr[:idx])
		right := strings.TrimSpace(expr[idx+len(op.op):])
		right = unquote(right)
		return op.fn(left, right), nil
	}

	return false, fmt.Errorf("%w: no valid operator found in %q", ErrInvalidFilter, expr)
}

// findOperator finds the position of an operator in the expression,
// accounting for word-boundary operators like "contains", "starts_with", "ends_with".
func findOperator(expr, op string) int {
	// For symbol operators (==, !=, etc.), simple search
	if !isWordOperator(op) {
		return strings.Index(expr, op)
	}
	// For word operators, require whitespace boundaries
	needle := " " + op + " "
	idx := strings.Index(expr, needle)
	if idx >= 0 {
		return idx + 1 // return position of the operator itself (after leading space)
	}
	return -1
}

// isWordOperator returns true for operators that are words (need space boundaries).
func isWordOperator(op string) bool {
	switch op {
	case "contains", "starts_with", "ends_with":
		return true
	}
	return false
}

// compareValues compares two values. If both parse as numbers, compare
// numerically. Otherwise, compare as strings.
func compareValues(left, right string) int {
	lNum, lOk := tryNumeric(left)
	rNum, rOk := tryNumeric(right)

	if lOk && rOk {
		switch {
		case lNum < rNum:
			return -1
		case lNum > rNum:
			return 1
		default:
			return 0
		}
	}

	// String comparison
	return strings.Compare(left, right)
}

// tryNumeric delegates to the exported TryNumeric in typeconv.go.
func tryNumeric(s string) (float64, bool) {
	return TryNumeric(s)
}

// unquote removes surrounding single or double quotes from a string.
func unquote(s string) string {
	s = strings.TrimSpace(s)
	if len(s) >= 2 {
		if (s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"') {
			return s[1 : len(s)-1]
		}
	}
	return s
}

// splitOnKeyword splits an expression on a keyword (like " AND " or " OR "),
// respecting that the keyword must appear as a standalone token.
func splitOnKeyword(expr, keyword string) []string {
	parts := strings.Split(expr, keyword)
	if len(parts) <= 1 {
		return []string{expr}
	}
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		trimmed := strings.TrimSpace(p)
		if trimmed != "" {
			result = append(result, trimmed)
		}
	}
	if len(result) == 0 {
		return []string{expr}
	}
	return result
}
