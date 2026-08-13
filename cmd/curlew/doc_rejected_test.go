package main

import (
	"fmt"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §11.4's four rejected constructions.
//
// Every row is a promise that something is caught rather than run, with the
// exit code in its own column. A rejection that stopped happening looks like a
// passing run, which is the one outcome nobody investigates — so each row is
// written out as a collection and the exit code read back. The server is
// watched too: a construction the parser accepts and the runner then trips over
// would still exit non-zero, and only the absence of any request tells the two
// apart.

// rejectedCase builds a collection for one row of §11.4.
type rejectedCase struct {
	// match is the distinguishing text of the Construction cell.
	match string
	// build returns the collection, given a base URL nothing should reach.
	build func(base string) string
	// args are the flags the construction needs to be reachable.
	args []string
}

var rejectedCases = []rejectedCase{
	{
		match: "Dynamic extract key",
		build: func(base string) string {
			return "name: rejected\nrequests:\n" +
				fmt.Sprintf("  - name: One\n    request:\n      method: GET\n      url: \"%s/a\"\n"+
					"    extract:\n      \"{{prefix}}_id\": \"$.id\"\n", base)
		},
		args: []string{"--parallel"},
	},
	{
		match: "extracting the same name",
		build: func(base string) string {
			one := func(name, path string) string {
				return fmt.Sprintf("  - name: %s\n    request:\n      method: GET\n      url: \"%s%s\"\n"+
					"    extract:\n      user_id: \"$.id\"\n", name, base, path)
			}
			return "name: rejected\nrequests:\n" + one("A", "/a") + one("B", "/b")
		},
		args: []string{"--parallel"},
	},
	{
		match: "Cyclic",
		build: func(base string) string {
			one := func(name, path, dep string) string {
				return fmt.Sprintf("  - name: %s\n    depends_on: [%s]\n    request:\n      method: GET\n"+
					"      url: \"%s%s\"\n", name, dep, base, path)
			}
			return "name: rejected\nrequests:\n" + one("A", "/a", "B") + one("B", "/b", "A")
		},
		args: []string{"--parallel"},
	},
	{
		match: "not in the same phase",
		build: func(base string) string {
			return "name: rejected\nsetup:\n" +
				fmt.Sprintf("  - name: Prepare\n    request:\n      method: GET\n      url: \"%s/s\"\n", base) +
				"requests:\n" +
				fmt.Sprintf("  - name: One\n    depends_on: [Prepare]\n    request:\n      method: GET\n"+
					"      url: \"%s/a\"\n", base)
		},
		args: []string{"--parallel"},
	},
}

func caseFor(construction string) (rejectedCase, bool) {
	for _, rc := range rejectedCases {
		if strings.Contains(construction, rc.match) {
			return rc, true
		}
	}
	return rejectedCase{}, false
}

func TestDocTables_rejectedConstructionsAreRejected(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Construction", "Why", "Exit")
	if err != nil {
		t.Fatalf("rejected constructions table: %v", err)
	}
	constrCol, exitCol := docs.Column(hdr, "Construction"), docs.Column(hdr, "Exit")
	if constrCol < 0 || exitCol < 0 {
		t.Fatalf("rejected constructions table lost a column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= exitCol {
			continue
		}
		construction := row[constrCol]
		wantExit, convErr := strconv.Atoi(strings.TrimSpace(row[exitCol]))
		if convErr != nil {
			t.Errorf("§11.4 row %q has no exit code in its Exit column: %q", construction, row[exitCol])
			continue
		}
		rc, ok := caseFor(construction)
		if !ok {
			t.Errorf("§11.4 rejects %q, which no case constructs; write one rather than leaving the "+
				"rejection unproven", construction)
			continue
		}
		seen++

		t.Run(rc.match, func(t *testing.T) {
			rec := &pathRecorder{}
			srv := rec.server(t)
			defer srv.Close()

			out, code := runCollection(t, bin, rc.build(srv.URL), rc.args...)
			if code != wantExit {
				t.Errorf("§11.4 says %q exits %d; it exited %d\n%s", construction, wantExit, code, out)
			}
			if paths := rec.sortedPaths(); len(paths) != 0 {
				t.Errorf("§11.4 lists %q among the constructions that are rejected, and the run issued "+
					"%v before stopping", construction, paths)
			}
		})
	}
	if seen == 0 {
		t.Fatal("no constructions read from §11.4")
	}
}

// The premise: each construction is rejected for its own reason, not because
// the fixture around it fails to parse. The same collection with the one
// offending detail removed must run.
func TestDocTables_rejectedConstructionsAreOtherwiseValid(t *testing.T) {
	bin := buildBinary(t)

	controls := map[string]func(base string) string{
		"Dynamic extract key": func(base string) string {
			return "name: control\nrequests:\n" +
				fmt.Sprintf("  - name: One\n    request:\n      method: GET\n      url: \"%s/a\"\n"+
					"    extract:\n      static_id: \"$.id\"\n", base)
		},
		"extracting the same name": func(base string) string {
			one := func(name, path, varName string) string {
				return fmt.Sprintf("  - name: %s\n    request:\n      method: GET\n      url: \"%s%s\"\n"+
					"    extract:\n      %s: \"$.id\"\n", name, base, path, varName)
			}
			return "name: control\nrequests:\n" + one("A", "/a", "a_id") + one("B", "/b", "b_id")
		},
		"Cyclic": func(base string) string {
			return "name: control\nrequests:\n" +
				fmt.Sprintf("  - name: A\n    request:\n      method: GET\n      url: \"%s/a\"\n", base) +
				fmt.Sprintf("  - name: B\n    depends_on: [A]\n    request:\n      method: GET\n      url: \"%s/b\"\n", base)
		},
		"not in the same phase": func(base string) string {
			return "name: control\nrequests:\n" +
				fmt.Sprintf("  - name: Prepare\n    request:\n      method: GET\n      url: \"%s/s\"\n", base) +
				fmt.Sprintf("  - name: One\n    depends_on: [Prepare]\n    request:\n      method: GET\n"+
					"      url: \"%s/a\"\n", base)
		},
	}

	for _, rc := range rejectedCases {
		control, ok := controls[rc.match]
		if !ok {
			t.Errorf("no control collection for %q", rc.match)
			continue
		}
		t.Run(rc.match, func(t *testing.T) {
			rec := &pathRecorder{}
			srv := rec.server(t)
			defer srv.Close()

			out, code := runCollection(t, bin, control(srv.URL), rc.args...)
			if code != 0 {
				t.Errorf("the control for %q exits %d, so the rejection test above may be catching the "+
					"fixture rather than the construction\n%s", rc.match, code, out)
			}
			if len(rec.sortedPaths()) == 0 {
				t.Errorf("the control for %q issued no requests\n%s", rc.match, out)
			}
		})
	}
}
