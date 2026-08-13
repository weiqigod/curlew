package parser

import (
	"regexp"
	"sort"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// Both documents describe the WebSocket step actions, and both name the fields
// each action reads inside the prose of their columns.
//
// §11C.11 was exactly this going wrong: the specification's own example gave a
// `wait` step a `timeout_ms` the action never consults, so the step paused for
// zero milliseconds and said nothing. The parser now rejects a field outside
// its action's set — and these tables, which are where a reader learns which
// fields belong to which action, are now held to that same map.

// fieldMention matches a `field` name in a table cell.
var fieldMention = regexp.MustCompile("`([a-z_]+)`")

// actionTables are the two tables listing the actions.
var actionTables = []struct {
	doc    string
	where  string
	header []string
	// fieldColumns are the columns whose backticked names must be fields of
	// that row's action.
	fieldColumns []string
}{
	{"CLI_SPECIFICATION.md", "12.3 WebSocket", []string{"Action", "Purpose"}, []string{"Purpose"}},
	{"MANUAL.md", "7.2 WebSocket", []string{"Action", "Purpose", "Key fields"}, []string{"Key fields"}},
}

func TestDocTables_websocketActionTablesMatchTheParser(t *testing.T) {
	known := map[string]bool{}
	for action := range stepFieldsByAction {
		known[action] = true
	}
	if len(known) == 0 {
		t.Fatal("stepFieldsByAction is empty; the parser lost its action map")
	}

	for _, at := range actionTables {
		hdr, rows, err := docs.TableUnder(at.doc, at.where, at.header...)
		if err != nil {
			t.Errorf("%s under %q: %v", at.doc, at.where, err)
			continue
		}
		actionCol := docs.Column(hdr, "Action")
		if actionCol < 0 {
			t.Errorf("%s under %q: no Action column in %v", at.doc, at.where, hdr)
			continue
		}

		documented := map[string]bool{}
		for _, row := range rows {
			if len(row) <= actionCol {
				continue
			}
			action := docs.FirstName(row[actionCol])
			if action == "" {
				continue
			}
			documented[action] = true

			allowed := map[string]bool{}
			for _, f := range stepFieldsByAction[action] {
				allowed[f] = true
			}
			if !known[action] {
				t.Errorf("%s under %q documents action %q, which the parser does not accept",
					at.doc, at.where, action)
				continue
			}

			// Every field the row names for this action must be one the action
			// reads. A row naming a field the parser now rejects would send a
			// reader straight into a parse error.
			for _, col := range at.fieldColumns {
				ci := docs.Column(hdr, col)
				if ci < 0 || len(row) <= ci {
					continue
				}
				for _, m := range fieldMention.FindAllStringSubmatch(row[ci], -1) {
					field := m[1]
					if !allowed[field] {
						t.Errorf("%s under %q: `%s` names field `%s`, which is not one of %v",
							at.doc, at.where, action, field, stepFieldsByAction[action])
					}
				}
			}
		}

		if len(documented) == 0 {
			t.Errorf("%s under %q: no actions read from the table", at.doc, at.where)
			continue
		}
		for action := range known {
			if !documented[action] {
				t.Errorf("%s under %q omits action %q, which the parser accepts",
					at.doc, at.where, action)
			}
		}
	}
}

// The two tables must also agree with each other about which actions exist.
func TestDocTables_theTwoWebsocketTablesAgree(t *testing.T) {
	var seen [][]string
	for _, at := range actionTables {
		hdr, rows, err := docs.TableUnder(at.doc, at.where, at.header...)
		if err != nil {
			t.Fatalf("%s under %q: %v", at.doc, at.where, err)
		}
		col := docs.Column(hdr, "Action")
		var actions []string
		for _, row := range rows {
			if len(row) > col {
				if a := docs.FirstName(row[col]); a != "" {
					actions = append(actions, a)
				}
			}
		}
		sort.Strings(actions)
		seen = append(seen, actions)
	}
	if len(seen) == 2 && strings.Join(seen[0], ",") != strings.Join(seen[1], ",") {
		t.Errorf("the WebSocket action tables disagree:\n  specification: %v\n  manual:        %v",
			seen[0], seen[1])
	}
}
