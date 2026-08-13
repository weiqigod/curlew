package jsonpath

import (
	"encoding/json"
	"regexp"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §2.3 teaches JSONPath against a sample body printed immediately above the
// table. Both halves are in the document, so the whole thing runs: parse the
// body out of the fenced block, evaluate each Expression, compare to Result.
//
// This is the section a reader learns the syntax from. An expression here that
// the engine does not support is a reader's first assertion failing for a
// reason the manual just told them was correct.

// jsonBlock captures the first fenced json block in a section.
var jsonBlock = regexp.MustCompile("(?s)```json\\s*\\n(.*?)```")

// resultLiteral returns the value a Result cell states, dropping any
// parenthetical gloss: `["A", "B"]` (all skus) -> ["A", "B"]. SplitRow has
// already unwrapped a cell that is nothing but code.
func resultLiteral(cell string) string {
	if i := strings.Index(cell, "`"); i >= 0 {
		if j := strings.Index(cell[i+1:], "`"); j >= 0 {
			return cell[i+1 : i+1+j]
		}
	}
	if i := strings.Index(cell, " ("); i > 0 {
		return strings.TrimSpace(cell[:i])
	}
	return cell
}

func TestDocTables_jsonPathCrashCourseEvaluates(t *testing.T) {
	manual, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}

	// The sample body is the json block in §2.3, between that heading and the
	// table it teaches.
	start := strings.Index(manual, "### 2.3 JSONPath crash course")
	if start < 0 {
		t.Fatal("§2.3 is gone; this check has nothing to run")
	}
	section := manual[start:]
	block := jsonBlock.FindStringSubmatch(section)
	if block == nil {
		t.Fatal("§2.3 no longer prints a sample body; the table has nothing to be about")
	}

	var doc any
	if err := json.Unmarshal([]byte(block[1]), &doc); err != nil {
		t.Fatalf("the sample body in §2.3 is not valid JSON: %v", err)
	}

	hdr, rows, err := docs.Table("MANUAL.md", "Expression", "Result")
	if err != nil {
		t.Fatalf("crash-course table: %v", err)
	}
	exprCol, resCol := docs.Column(hdr, "Expression"), docs.Column(hdr, "Result")
	if exprCol < 0 || resCol < 0 {
		t.Fatalf("crash-course table lost a column: %v", hdr)
	}

	checked := 0
	for _, row := range rows {
		if len(row) <= resCol {
			continue
		}
		expr := strings.Trim(row[exprCol], "`")
		if expr == "" {
			continue
		}

		// One row exists to say what not to write — "use the `length:` operator
		// instead" — and names no value to compare against.
		cell := strings.TrimSpace(row[resCol])
		if strings.Contains(cell, "instead") {
			if _, evalErr := Evaluate(expr, doc); evalErr == nil {
				t.Errorf("§2.3 tells readers to use an operator instead of %q, but it evaluates", expr)
			}
			continue
		}
		want := resultLiteral(cell)

		got, evalErr := Evaluate(expr, doc)
		if evalErr != nil {
			t.Errorf("§2.3 teaches %q, which does not evaluate: %v", expr, evalErr)
			continue
		}
		checked++

		if want == "the whole document" {
			continue // `$` — the Result cell is prose, and the value is the body
		}
		if !sameJSON(t, got, want) {
			gotJSON, _ := json.Marshal(got)
			t.Errorf("§2.3 says %s is %s, engine gives %s", expr, want, gotJSON)
		}
	}
	if checked == 0 {
		t.Fatal("no expressions evaluated; the table moved or its header changed")
	}
}

// sameJSON compares an evaluated value with the JSON literal a Result cell
// prints, so `["A", "B"]` and the engine's []any{"A","B"} compare equal without
// depending on how either is spelled.
func sameJSON(t *testing.T, got any, want string) bool {
	t.Helper()

	var wantVal any
	if err := json.Unmarshal([]byte(want), &wantVal); err != nil {
		return false
	}
	gotJSON, err := json.Marshal(got)
	if err != nil {
		return false
	}
	wantJSON, err := json.Marshal(wantVal)
	if err != nil {
		return false
	}
	return string(gotJSON) == string(wantJSON)
}

// The manual now states plainly that wildcard and recursive descent are not
// implemented. That is only worth writing if it stays true: if either is built
// later, this fails and the paragraph has to be rewritten rather than quietly
// becoming wrong in the other direction.
func TestDocTables_theUnsupportedSyntaxIsStillUnsupported(t *testing.T) {
	manual, err := docs.ReadDoc("MANUAL.md")
	if err != nil {
		t.Fatalf("reading the manual: %v", err)
	}
	if !strings.Contains(manual, "recursive descent") {
		t.Fatal("§2.3 no longer says which JSONPath syntax is unimplemented")
	}

	doc := map[string]any{"items": []any{map[string]any{"sku": "A"}}}
	for _, expr := range []string{"$.items[*].sku", "$..sku"} {
		if !strings.Contains(manual, expr) {
			t.Errorf("§2.3 no longer names %q among the unsupported forms", expr)
		}
		if _, evalErr := Evaluate(expr, doc); evalErr == nil {
			t.Errorf("%q evaluates now; §2.3 still tells readers it does not", expr)
		}
	}
}
