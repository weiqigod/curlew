package assertion

import (
	"go/ast"
	"go/parser"
	"go/token"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The operator catalogue, checked in both directions.
//
// docs/CLI_SPECIFICATION.md §7.2 and §7.3 list the header and body operators in
// tables. A table is not a snippet, so nothing rejects it when it drifts: it
// can promise an operator that does not exist, or fall silent about one that
// does, and the build stays green either way. That is the same shape as
// §11C.2 — a table describing behaviour the binary did not have.
//
// Forward: every documented operator must actually evaluate. An operator the
// documents promise and the evaluator answers "unsupported operator" to is the
// dangerous direction, because a reader writes it and gets a failing assertion
// that looks like a failing API.
//
// Backward: every implemented operator must be documented. The case labels are
// read from this package's own source with go/ast rather than restated here,
// because a list restated in a test is just a second thing to forget to update.

// operatorsFromSwitch returns the case labels of the operator switch inside the
// named function in assertion.go.
func operatorsFromSwitch(t *testing.T, funcName string) []string {
	t.Helper()
	fset := token.NewFileSet()
	file, err := parser.ParseFile(fset, "assertion.go", nil, 0)
	if err != nil {
		t.Fatalf("parsing assertion.go: %v", err)
	}

	var found []string
	ast.Inspect(file, func(n ast.Node) bool {
		fn, ok := n.(*ast.FuncDecl)
		if !ok || fn.Name.Name != funcName {
			return true
		}
		ast.Inspect(fn, func(inner ast.Node) bool {
			sw, isSwitch := inner.(*ast.SwitchStmt)
			if !isSwitch {
				return true
			}
			// Only the switch on a.Operator.
			sel, isSel := sw.Tag.(*ast.SelectorExpr)
			if !isSel || sel.Sel.Name != "Operator" {
				return true
			}
			for _, stmt := range sw.Body.List {
				cc, isCase := stmt.(*ast.CaseClause)
				if !isCase {
					continue
				}
				for _, expr := range cc.List {
					lit, isLit := expr.(*ast.BasicLit)
					if !isLit || lit.Kind != token.STRING {
						continue
					}
					if v, uErr := strconv.Unquote(lit.Value); uErr == nil {
						found = append(found, v)
					}
				}
			}
			return true
		})
		return false
	})

	if len(found) == 0 {
		// A test that finds no operators and passes would be worse than absent.
		t.Fatalf("no operator case labels found in %s; the switch shape changed "+
			"and this test is no longer reading it", funcName)
	}
	sort.Strings(found)
	return found
}

// documentedOperators reads the first column of a table in the specification.
func documentedOperators(t *testing.T, headerCells ...string) []string {
	t.Helper()
	_, rows, err := docs.Table("CLI_SPECIFICATION.md", headerCells...)
	if err != nil {
		t.Fatal(err)
	}
	var out []string
	for _, r := range rows {
		if len(r) > 0 && r[0] != "" {
			out = append(out, r[0])
		}
	}
	if len(out) == 0 {
		t.Fatalf("no operators listed in the table with header %v", headerCells)
	}
	sort.Strings(out)
	return out
}

func TestDocTable_bodyOperatorsMatchTheImplementation(t *testing.T) {
	documented := documentedOperators(t, "Operator", "Applies to", "Meaning")
	implemented := operatorsFromSwitch(t, "evalBodyAssertion")
	compareOperatorSets(t, "body", documented, implemented)
}

func TestDocTable_headerOperatorsMatchTheImplementation(t *testing.T) {
	// §7.2's table is the first "Operator | Meaning" table in the document.
	documented := documentedOperators(t, "Operator", "Meaning")
	implemented := operatorsFromSwitch(t, "evalHeaderAssertion")
	compareOperatorSets(t, "header", documented, implemented)
}

func compareOperatorSets(t *testing.T, kind string, documented, implemented []string) {
	t.Helper()
	impl := map[string]bool{}
	for _, o := range implemented {
		impl[o] = true
	}
	doc := map[string]bool{}
	for _, o := range documented {
		doc[o] = true
	}

	for _, o := range documented {
		if !impl[o] {
			t.Errorf("CLI_SPECIFICATION documents %s operator %q, which the evaluator does not implement; "+
				"a reader writing it gets a failing assertion that looks like a failing API", kind, o)
		}
	}
	for _, o := range implemented {
		if !doc[o] {
			t.Errorf("the evaluator implements %s operator %q, which CLI_SPECIFICATION does not document", kind, o)
		}
	}
	t.Logf("%s: %d documented, %d implemented", kind, len(documented), len(implemented))
}

// TestDocTable_everyDocumentedBodyOperatorEvaluates is the forward direction
// end to end: the operator names are taken from the document and actually run,
// so a documented operator that reaches the default branch is caught even if it
// somehow has a case label.
func TestDocTable_everyDocumentedBodyOperatorEvaluates(t *testing.T) {
	// A value per operator that is type-appropriate. An operator listed in the
	// document with no value here fails rather than being skipped.
	values := map[string]any{
		"equals":                "x",
		"matches":               "^x$",
		"exists":                true,
		"not_exists":            false,
		"type":                  "string",
		"contains":              "x",
		"contains_all":          []any{"x"},
		"length":                1,
		"greater_than":          0,
		"less_than":             2,
		"greater_than_or_equal": 0,
		"less_than_or_equal":    2,
		"approximately":         map[string]any{"value": 1, "tolerance": 1},
		"in_range":              map[string]any{"min": 0, "max": 2},
	}

	for _, op := range documentedOperators(t, "Operator", "Applies to", "Meaning") {
		v, known := values[op]
		if !known {
			t.Errorf("no sample value for documented operator %q; add one rather than "+
				"letting the operator go unchecked", op)
			continue
		}
		got := CheckBody([]BodyInput{{Path: "$.n", Operator: op, Value: v}}, []byte(`{"n":"x"}`))
		if len(got) != 1 {
			t.Fatalf("%s: results = %d, want 1", op, len(got))
		}
		// Pass or fail is beside the point — "unsupported operator" is not.
		if strings.Contains(got[0].Actual, "unsupported operator") {
			t.Errorf("documented body operator %q is not implemented", op)
		}
	}
}

func TestDocTable_everyDocumentedHeaderOperatorEvaluates(t *testing.T) {
	values := map[string]string{"equals": "v", "exists": "true", "matches": "^v$"}
	headers := http.Header{"X-Test": []string{"v"}}

	for _, op := range documentedOperators(t, "Operator", "Meaning") {
		v, known := values[op]
		if !known {
			t.Errorf("no sample value for documented header operator %q", op)
			continue
		}
		got := CheckHeaders([]HeaderInput{{Name: "X-Test", Operator: op, Value: v}}, headers)
		if len(got) != 1 {
			t.Fatalf("%s: results = %d, want 1", op, len(got))
		}
		if strings.Contains(got[0].Actual, "unsupported operator") {
			t.Errorf("documented header operator %q is not implemented", op)
		}
	}
}

// numberWords covers the counts the documents actually spell out. A count
// written as a word that this map does not know fails rather than being
// skipped, so the guard cannot go quietly blind.
var numberWords = map[string]int{
	"One": 1, "Two": 2, "Three": 3, "Four": 4, "Five": 5, "Six": 6, "Seven": 7,
	"Eight": 8, "Nine": 9, "Ten": 10, "Eleven": 11, "Twelve": 12,
	"Thirteen": 13, "Fourteen": 14, "Fifteen": 15, "Sixteen": 16,
	"Seventeen": 17, "Eighteen": 18, "Nineteen": 19, "Twenty": 20,
}

var countClaimRe = regexp.MustCompile(`([A-Z][a-z]+) operators`)

// TestDocPose_operatorCountsMatchTheirTables checks the prose against the table
// immediately below it. Both documents said "Thirteen operators" above a table
// of fourteen — a claim contradicting the very list it introduces, in two
// places, which is the §11C.2 shape at its smallest.
func TestDocPose_operatorCountsMatchTheirTables(t *testing.T) {
	cases := []struct {
		doc         string
		headerCells []string
		// claim is the ordinal occurrence of "<Word> operators" in the document
		// that introduces this table: 1 for the first, 2 for the second.
		claim int
	}{
		{"CLI_SPECIFICATION.md", []string{"Operator", "Meaning"}, 1},
		{"CLI_SPECIFICATION.md", []string{"Operator", "Applies to", "Meaning"}, 2},
	}

	for _, tc := range cases {
		data, err := os.ReadFile(filepath.Join(docs.Dir, tc.doc))
		if err != nil {
			t.Fatalf("reading %s: %v", tc.doc, err)
		}
		claims := countClaimRe.FindAllStringSubmatch(string(data), -1)
		if len(claims) < tc.claim {
			t.Fatalf("%s: expected at least %d spelled-out operator counts, found %d",
				tc.doc, tc.claim, len(claims))
		}
		word := claims[tc.claim-1][1]
		claimed, known := numberWords[word]
		if !known {
			t.Fatalf("%s: %q operators — this test does not know that number word", tc.doc, word)
		}

		_, rows, tblErr := docs.Table(tc.doc, tc.headerCells...)
		if tblErr != nil {
			t.Fatal(tblErr)
		}
		if claimed != len(rows) {
			t.Errorf("%s: prose claims %s (%d) operators, but the table lists %d",
				tc.doc, word, claimed, len(rows))
		}
	}
}
