package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The documents' behavioural TABLES, executed.
//
// §11C.3 and §11C.11 were wrong examples, and examples are now parsed on every
// build. §11C.2 was a wrong TABLE — docs/MANUAL.md §7.1 stated the
// error-handling matrix as four outcomes against three modes, the binary
// ignored the mode for half of them, and no test looked at either. A table is
// not a snippet: no parser will ever reject it, so the only way it can be wrong
// is if something executes what it claims.
//
// This reads the matrix out of both documents, requires them to agree with each
// other — they describe one binary — and runs every cell.
//
// Deriving the response body from the "When" column would mean parsing prose,
// so the bodies are named here instead. What keeps that honest is that an
// outcome appearing in the documents with no body defined here FAILS: a row
// added to the table cannot be silently skipped.

// graphqlOutcomeBodies maps each documented outcome to a response that produces
// it. The "When" column of the table is the specification of these values.
var graphqlOutcomeBodies = map[string]string{
	// data non-null, no errors
	"success": `{"data":{"ok":true}}`,
	// data non-null, errors present
	"partial-success": `{"data":{"ok":true},"errors":[{"message":"a field failed"}]}`,
	// data null, errors present
	"full-failure": `{"data":null,"errors":[{"message":"everything failed"}]}`,
	// no data and no errors
	"empty": `{}`,
}

// readDocTable is a thin test wrapper over internal/docs.
func readDocTable(t *testing.T, doc string, headerCells ...string) (header []string, rows [][]string) {
	t.Helper()
	header, rows, err := docs.Table(doc, headerCells...)
	if err != nil {
		t.Fatal(err)
	}
	return header, rows
}

// TestDocTable_graphqlMatrixAgreesAcrossDocuments guards the precondition for
// the test below. Two documents stating the same matrix differently is exactly
// the §11C.2 situation — one of them wrong about the shipped binary — and
// executing only one of them would leave the other free to drift.
func TestDocTable_graphqlMatrixAgreesAcrossDocuments(t *testing.T) {
	manualHdr, manualRows := readDocTable(t, "MANUAL.md", "Outcome", "fail", "warn", "ignore")
	specHdr, specRows := readDocTable(t, "CLI_SPECIFICATION.md", "Outcome", "fail", "warn", "ignore")

	if strings.Join(manualHdr, "|") != strings.Join(specHdr, "|") {
		t.Errorf("matrix headers differ:\n  MANUAL: %v\n  SPEC:   %v", manualHdr, specHdr)
	}
	if len(manualRows) != len(specRows) {
		t.Fatalf("matrix row counts differ: MANUAL %d, SPEC %d", len(manualRows), len(specRows))
	}
	for i := range manualRows {
		m, s := strings.Join(manualRows[i], "|"), strings.Join(specRows[i], "|")
		if m != s {
			t.Errorf("matrix row %d differs:\n  MANUAL: %s\n  SPEC:   %s", i+1, m, s)
		}
	}
}

// TestDocTable_graphqlMatrixIsWhatTheBinaryDoes runs every cell of the
// documented matrix. This is the test §11C.2 needed and did not have.
func TestDocTable_graphqlMatrixIsWhatTheBinaryDoes(t *testing.T) {
	header, rows := readDocTable(t, "MANUAL.md", "Outcome", "fail", "warn", "ignore")
	if len(rows) == 0 {
		t.Fatal("the matrix has no rows; nothing was checked")
	}

	// Locate the mode columns by name, so reordering the table cannot silently
	// swap what is asserted.
	modeCol := map[string]int{}
	for _, m := range []string{"fail", "warn", "ignore"} {
		if i := docs.Column(header, m); i >= 0 {
			modeCol[m] = i
		}
	}
	if len(modeCol) != 3 {
		t.Fatalf("expected three mode columns, found %v", modeCol)
	}

	checked := 0
	for _, row := range rows {
		outcome := row[0]
		body, known := graphqlOutcomeBodies[outcome]
		if !known {
			t.Errorf("the documents describe outcome %q but no response body is defined for it here; "+
				"add one to graphqlOutcomeBodies rather than letting the row go unchecked", outcome)
			continue
		}

		for mode, col := range modeCol {
			if col >= len(row) {
				t.Errorf("row %q has no %q column", outcome, mode)
				continue
			}
			want := row[col]
			t.Run(outcome+"/"+mode, func(t *testing.T) {
				col, vars := makeGraphQLCollection(body, "", mode)
				results, summary, err := Run(context.Background(), col, makeGraphQLExecutor(body), vars)
				if err != nil {
					t.Fatalf("Run: %v", err)
				}
				if len(results) != 1 {
					t.Fatalf("results = %d, want 1", len(results))
				}
				warned := len(results[0].Warnings) > 0

				switch want {
				case "pass":
					if summary.Failed != 0 {
						t.Errorf("documented as pass under %q, but the request failed", mode)
					}
					if warned {
						t.Errorf("documented as pass under %q, but it warned: %v", mode, results[0].Warnings)
					}
				case "warn":
					if summary.Failed != 0 {
						t.Errorf("documented as warn under %q, but the request failed", mode)
					}
					if !warned {
						t.Errorf("documented as warn under %q, but no warning was produced", mode)
					}
				case "fail":
					if summary.Failed != 1 {
						t.Errorf("documented as fail under %q, but the request passed", mode)
					}
				default:
					t.Fatalf("unrecognised cell %q for %s/%s — the table says something this test "+
						"cannot check, which means it is unchecked", want, outcome, mode)
				}
			})
			checked++
		}
	}

	if checked == 0 {
		t.Fatal("no cells were checked")
	}
	t.Logf("checked %d matrix cells against the binary", checked)
}
