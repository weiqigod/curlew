package docs_test

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Every table in the two documents held to the binary must be executed by some
// test, or carry a marker saying why it states nothing executable.
//
// The three documentation surfaces were closed one at a time — examples parsed
// (M21/#27), tables executed (#28), prose names checked (#28) — but "tables
// executed" was only ever true of the three tables someone had wired. The
// NO_COLOR defect landed in the gap: both documents' environment-variable
// tables said "any non-empty value disables ANSI colour", the binary disabled
// on presence, and the row had been right and unread since M1-019.
//
// A count of executed tables cannot close that gap; only a count of
// *unexecuted* ones can. This test is that count.

// repoRoot is where the test walk for claims starts, relative to internal/docs.
const repoRoot = "../.."

// maxExemptTables caps how much of the documentation can declare itself
// unexecutable. Raising it is a deliberate act with a diff, which is the point:
// without a cap the checks can be hollowed out one table at a time, which is
// how the ignore-names markers are governed too.
const maxExemptTables = 8

func TestDocTables_everyTableIsExecutedOrDeclaredProse(t *testing.T) {
	claims, err := docs.Claims(repoRoot)
	if err != nil {
		t.Fatalf("reading claims from test sources: %v", err)
	}
	if len(claims) == 0 {
		t.Fatal("no test anywhere reads a documentation table; the extractor is broken")
	}

	var (
		all       []docs.TableRef
		exemptCnt int
	)
	for _, doc := range docs.Docs {
		refs, invErr := docs.Inventory(doc)
		if invErr != nil {
			t.Fatalf("inventory of %s: %v", doc, invErr)
		}
		if len(refs) == 0 {
			t.Fatalf("no tables found in %s; the inventory is broken, not the document", doc)
		}
		all = append(all, refs...)
	}

	// Resolve every claim the way docs.Table would, and mark what it reads.
	executed := make(map[string]bool)
	unresolved := make([]docs.Claim, 0)
	for _, c := range claims {
		if c.Fn == docs.AllTablesReader {
			matches := docs.FindAll(all, c.Doc, c.Header)
			if len(matches) == 0 {
				unresolved = append(unresolved, c)
				continue
			}
			for _, ref := range matches {
				executed[key(ref)] = true
			}
			continue
		}
		ref, ok := docs.Find(all, c.Doc, c.Header)
		if !ok {
			unresolved = append(unresolved, c)
			continue
		}
		executed[key(ref)] = true
	}

	var unexecuted []docs.TableRef
	for _, ref := range all {
		if ref.Exempt != "" {
			exemptCnt++
			continue
		}
		if !executed[key(ref)] {
			unexecuted = append(unexecuted, ref)
		}
	}

	sort.Slice(unexecuted, func(i, j int) bool {
		if unexecuted[i].Doc != unexecuted[j].Doc {
			return unexecuted[i].Doc < unexecuted[j].Doc
		}
		return unexecuted[i].Line < unexecuted[j].Line
	})

	// The baseline is a debt register, not an exemption list: everything in it
	// is a table that should be executed and is not yet. It can only shrink —
	// an entry that stops being unexecuted must be deleted, and an unexecuted
	// table that is not listed fails the build. Same contract as
	// testapi/harness/redaction-known-leaks.txt, which emptied itself.
	baseline, err := readBaseline()
	if err != nil {
		t.Fatalf("reading %s: %v", baselineFile, err)
	}

	var newlyUnexecuted []docs.TableRef
	stillOwed := map[string]bool{}
	for _, ref := range unexecuted {
		id := baselineKey(ref)
		if baseline[id] {
			stillOwed[id] = true
			continue
		}
		newlyUnexecuted = append(newlyUnexecuted, ref)
	}

	if len(newlyUnexecuted) > 0 {
		var b strings.Builder
		fmt.Fprintf(&b, "%d table(s) state something about the binary that no test runs.\n", len(newlyUnexecuted))
		b.WriteString("Execute each one, or mark it with\n    ")
		b.WriteString(docs.ExemptMarker)
		b.WriteString(" <why> -->\non the line before it.\n\n")
		for _, ref := range newlyUnexecuted {
			fmt.Fprintf(&b, "  %s\n    baseline id: %s\n", ref, baselineKey(ref))
		}
		if len(unresolved) > 0 {
			b.WriteString("\nSignatures that matched no table (a renamed header, or a test-driven case " +
				"whose other fields were read as header cells):\n")
			for _, c := range unresolved {
				fmt.Fprintf(&b, "  %s:%d %s | %s\n", c.File, c.Line, c.Doc, strings.Join(c.Header, " | "))
			}
		}
		t.Fatal(b.String())
	}

	// A baseline entry that is no longer owed has been paid off, and leaving it
	// listed would let the same table silently stop being executed later.
	var paid []string
	for id := range baseline {
		if !stillOwed[id] {
			paid = append(paid, id)
		}
	}
	sort.Strings(paid)
	if len(paid) > 0 {
		t.Errorf("%d baseline entr(ies) are now executed — delete them from %s so the debt cannot grow back:\n  %s",
			len(paid), baselineFile, strings.Join(paid, "\n  "))
	}

	if exemptCnt > maxExemptTables {
		t.Fatalf("%d tables are marked unexecutable, cap is %d; raise the cap deliberately or execute one",
			exemptCnt, maxExemptTables)
	}
	t.Logf("%d tables: %d executed, %d declared prose (cap %d), %d owed an executor (%s)",
		len(all), len(all)-exemptCnt-len(stillOwed), exemptCnt, maxExemptTables,
		len(stillOwed), baselineFile)
}

func key(r docs.TableRef) string {
	return fmt.Sprintf("%s:%d", r.Doc, r.Line)
}

// baselineFile holds the tables still owed an executor, beside the documents
// it is about.
var baselineFile = filepath.Join(docs.Dir, "table-execution-baseline.txt")

// baselineKey identifies a table by what it says rather than where it sits, so
// editing the prose above a table does not silently retire its debt.
func baselineKey(r docs.TableRef) string {
	return fmt.Sprintf("%s\t%s\t%s", r.Doc, r.Heading, strings.Join(r.Header, " | "))
}

func readBaseline() (map[string]bool, error) {
	data, err := os.ReadFile(baselineFile)
	if err != nil {
		if os.IsNotExist(err) {
			return map[string]bool{}, nil
		}
		return nil, err
	}
	out := map[string]bool{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimRight(line, "\r")
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out[line] = true
	}
	return out, nil
}
