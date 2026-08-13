package docs

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Docs are the two documents held to the binary. CHANGELOG.md and
// TESTAPI_SPECIFICATION.md are excluded for the same reason the prose checks
// exclude them: both discuss removed and unbuilt things deliberately.
var Docs = []string{"MANUAL.md", "CLI_SPECIFICATION.md"}

// ExemptMarker declares that the table following it states nothing the binary
// can be asked to demonstrate — navigation, rationale, a worked example whose
// inputs are illustrative. The reason is the rest of the line.
//
//	<!-- doc-check: table-not-executable a map of the document, not a claim -->
//
// A marker applies to the next table only, so one cannot blanket a section, and
// the total is capped (see the inventory test) so the checks cannot be hollowed
// out a table at a time.
const ExemptMarker = "<!-- doc-check: table-not-executable"

// TableRef identifies one markdown table in a document.
type TableRef struct {
	Doc     string   // "MANUAL.md"
	Line    int      // 1-based line number of the header row
	Heading string   // nearest preceding heading, "" before the first one
	Lead    string   // last prose line before the table, "" when none
	Header  []string // trimmed, backtick-stripped header cells
	Exempt  string   // reason from a preceding marker; "" when none
}

// String renders a reference the way a test failure should print it: clickable,
// with enough context to find the table without counting rows.
func (r TableRef) String() string {
	return fmt.Sprintf("%s:%d %s | header: %s", r.Doc, r.Line, r.Heading, strings.Join(r.Header, " | "))
}

// Under reports whether where names this table, matching either its heading or
// the prose line introducing it. Several tables commonly share a heading —
// the manual's CLI reference lists flags for `curlew exec` and `curlew ui`
// under one — and the lead-in is what a reader uses to tell them apart.
func (r TableRef) Under(where string) bool {
	return strings.Contains(r.Heading, where) || strings.Contains(r.Lead, where)
}

// Inventory returns every markdown table in the named document, in order.
//
// Tables inside fenced code blocks are not included: a table shown as example
// output is a picture of a table, not a claim being made in the document's own
// voice.
func Inventory(doc string) ([]TableRef, error) {
	path := filepath.Join(Dir, doc)
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}

	var (
		refs    []TableRef
		heading string
		lead    string
		exempt  string
		inFence bool
		inTable bool
	)
	for i, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)

		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			inTable = false
			continue
		}
		if inFence {
			continue
		}

		switch {
		case strings.HasPrefix(trimmed, "#"):
			heading = trimmed
			lead = ""
			inTable = false
			exempt = ""
		case strings.HasPrefix(trimmed, ExemptMarker):
			exempt = strings.TrimSpace(strings.TrimSuffix(
				strings.TrimPrefix(trimmed, ExemptMarker), "-->"))
			inTable = false
		case strings.HasPrefix(trimmed, "|"):
			if !inTable {
				inTable = true
				refs = append(refs, TableRef{
					Doc:     doc,
					Line:    i + 1,
					Heading: heading,
					Lead:    lead,
					Header:  SplitRow(line),
					Exempt:  exempt,
				})
				exempt = "" // one marker, one table
			}
		default:
			inTable = false
			if trimmed != "" {
				lead = trimmed
				// A marker is spent by any prose between it and the table, so
				// it cannot drift onto a table it was not written for.
				exempt = ""
			}
		}
	}
	return refs, nil
}

// FindAll returns every table a signature matches, which is what an AllTables
// claim covers.
func FindAll(refs []TableRef, doc string, sig []string) []TableRef {
	var out []TableRef
	for _, r := range refs {
		if r.Doc == doc && containsAll(r.Header, sig) {
			out = append(out, r)
		}
	}
	return out
}

// Find returns the table a signature resolves to, using the same rules Table
// and TableUnder do, so a test's signature and the inventory always agree about
// which table is being read.
//
// A signature is tried as header cells first; failing that, its first element
// is tried as the heading-or-lead-in that TableUnder takes.
func Find(refs []TableRef, doc string, sig []string) (TableRef, bool) {
	for _, r := range refs {
		if r.Doc == doc && containsAll(r.Header, sig) {
			return r, true
		}
	}
	if len(sig) < 2 {
		return TableRef{}, false
	}
	for _, r := range refs {
		if r.Doc == doc && r.Under(sig[0]) && containsAll(r.Header, sig[1:]) {
			return r, true
		}
	}
	return TableRef{}, false
}
