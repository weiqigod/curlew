package output

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Both documents describe the `output:` block: its fields, the values each
// accepts, and the default. Every column is checkable, and the Values column is
// the one that matters — it is where a reader learns what they may write.

var outputBlockTables = []struct {
	doc    string
	where  string
	header []string
}{
	{"CLI_SPECIFICATION.md", "5.9 Output Config Block", []string{"Field", "Values", "Default"}},
	{"MANUAL.md", "3.6.1", []string{"Field", "YAML type", "Values", "Default"}},
}

// docValues splits a Values cell into the individual values it offers, which
// both documents write as a comma-separated list of `code` spans.
func docValues(cell string) []string {
	var out []string
	for _, part := range strings.Split(cell, ",") {
		if v := docs.FirstName(strings.TrimSpace(part)); v != "" {
			out = append(out, v)
		}
	}
	return out
}

func TestDocTables_outputBlockValuesAreWhatIsAccepted(t *testing.T) {
	supported := map[string]bool{}
	for _, f := range SupportedFormats {
		supported[f] = true
	}

	for _, tbl := range outputBlockTables {
		hdr, rows, err := docs.TableUnder(tbl.doc, tbl.where, tbl.header...)
		if err != nil {
			t.Errorf("%s under %q: %v", tbl.doc, tbl.where, err)
			continue
		}
		fieldCol, valueCol := docs.Column(hdr, "Field"), docs.Column(hdr, "Values")
		defaultCol := docs.Column(hdr, "Default")
		if fieldCol < 0 || valueCol < 0 || defaultCol < 0 {
			t.Errorf("%s under %q lost a column: %v", tbl.doc, tbl.where, hdr)
			continue
		}

		seen := map[string]bool{}
		for _, row := range rows {
			if len(row) <= defaultCol {
				continue
			}
			field := docs.FirstName(row[fieldCol])
			seen[field] = true
			values := docValues(row[valueCol])
			def := docs.FirstName(row[defaultCol])

			switch field {
			case "format":
				documented := map[string]bool{}
				for _, v := range values {
					documented[v] = true
					if !supported[v] {
						t.Errorf("%s under %q offers format %q, which is not supported",
							tbl.doc, tbl.where, v)
					}
				}
				for _, f := range SupportedFormats {
					if !documented[f] {
						t.Errorf("%s under %q omits format %q from the `output:` block's values",
							tbl.doc, tbl.where, f)
					}
				}
				if def != "terminal" {
					t.Errorf("%s under %q says format defaults to %q, want terminal",
						tbl.doc, tbl.where, def)
				}

			case "verbosity":
				for _, v := range values {
					if _, ok, parseErr := ParseVerbosity(v); parseErr != nil || !ok {
						t.Errorf("%s under %q offers verbosity %q, which ParseVerbosity rejects: %v",
							tbl.doc, tbl.where, v, parseErr)
					}
				}
				// The default row says "normal", which must parse to the level
				// a run uses when nothing sets one.
				got, _, parseErr := ParseVerbosity(def)
				if parseErr != nil {
					t.Errorf("%s under %q says verbosity defaults to %q, which does not parse: %v",
						tbl.doc, tbl.where, def, parseErr)
				} else if got != VerbosityDefault {
					t.Errorf("%s under %q says verbosity defaults to %q, which is not the default level",
						tbl.doc, tbl.where, def)
				}
			}
		}

		for _, required := range []string{"format", "report", "events", "verbosity"} {
			if !seen[required] {
				t.Errorf("%s under %q no longer lists the %q field", tbl.doc, tbl.where, required)
			}
		}
	}
}
