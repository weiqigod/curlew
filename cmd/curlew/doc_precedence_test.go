package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// The ten-rung precedence ladder, in both documents.
//
// It is the most-consulted table in either document and the least checkable by
// reading: ten sources in one namespace, and the only thing that makes it true
// is that nine adjacent pairs each resolve the way the numbers say. A ladder is
// wrong in exactly one way — two rungs swapped — and nothing about the prose
// shows it. So every adjacent pair is run, with the same variable name defined
// from both rungs and the winner read off the wire.
//
// Rung 6 shells out to a cloud provider's CLI. A stub named `aws`, first on the
// test's PATH, supplies the secret: the project forbids anything that could
// bill, and shadowing the real CLI is what makes it impossible for this test to
// reach AWS at all.

// precedenceTables are the two ladders.
var precedenceTables = []struct {
	doc    string
	header []string
}{
	{"CLI_SPECIFICATION.md", []string{"#", "Source", "Where"}},
	{"MANUAL.md", []string{"#", "Source", "Where it lives"}},
}

// ladderFixture accumulates one definition of the variable per rung, then
// writes the files and builds the command line that puts them all in play.
type ladderFixture struct {
	name string // the variable's name

	project     string
	envFile     string
	dotEnv      string
	fromCommand string
	vault       string
	collection  string
	item        string
	envVar      string
	cliVar      string

	// dynamic records that a rung supplies the value by being a dynamic
	// function call rather than a definition.
	dynamic bool
}

// rung is one row of the ladder: the name the Source column gives it, and how
// to define the variable from that source.
type rung struct {
	key   string // docs.FirstName of the Source cell
	apply func(f *ladderFixture, value string)
}

// rungs maps every row of the ladder to a fixture. A row whose Source cell
// matches nothing here fails the test rather than being skipped — a renamed or
// inserted rung must be wired, not silently dropped from the ordering proof.
var rungs = []rung{
	{"Dynamic", func(f *ladderFixture, _ string) { f.dynamic = true }},
	{"Project", func(f *ladderFixture, v string) { f.project = v }},
	{"Environment", func(f *ladderFixture, v string) { f.envFile = v }},
	{".env", func(f *ladderFixture, v string) { f.dotEnv = v }},
	{"from_command", func(f *ladderFixture, v string) { f.fromCommand = v }},
	{"Vault", func(f *ladderFixture, v string) { f.vault = v }},
	{"Collection", func(f *ladderFixture, v string) { f.collection = v }},
	{"Request-item", func(f *ladderFixture, v string) { f.item = v }},
	{"Request-scoped", func(f *ladderFixture, v string) { f.item = v }},
	{"--env-var", func(f *ladderFixture, v string) { f.envVar = v }},
	{"--var", func(f *ladderFixture, v string) { f.cliVar = v }},
}

func rungFor(key string) (rung, bool) {
	for _, r := range rungs {
		if r.key == key {
			return r, true
		}
	}
	return rung{}, false
}

// ladderRow is one row as the document states it.
type ladderRow struct {
	number int
	source string // docs.FirstName of the Source cell
	rung   rung
}

func readLadder(t *testing.T, doc string, header []string) []ladderRow {
	t.Helper()

	hdr, rows, err := docs.Table(doc, header...)
	if err != nil {
		t.Fatalf("%s precedence ladder: %v", doc, err)
	}
	numCol, srcCol := docs.Column(hdr, "#"), docs.Column(hdr, "Source")
	if numCol < 0 || srcCol < 0 {
		t.Fatalf("%s precedence ladder lost a column: %v", doc, hdr)
	}

	var out []ladderRow
	for _, row := range rows {
		if len(row) <= srcCol {
			continue
		}
		n, convErr := strconv.Atoi(strings.TrimSpace(row[numCol]))
		if convErr != nil {
			continue
		}
		source := docs.FirstName(row[srcCol])
		r, ok := rungFor(source)
		if !ok {
			t.Errorf("%s ladder rung %d names source %q, which no fixture knows how to define; "+
				"wire it up rather than leaving the pair below it unproven", doc, n, source)
			continue
		}
		out = append(out, ladderRow{number: n, source: source, rung: r})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].number < out[j].number })
	for i, r := range out {
		if r.number != i+1 {
			t.Fatalf("%s ladder is numbered %d at position %d; the rungs must be 1..N with no gaps",
				doc, r.number, i+1)
		}
	}
	if len(out) < 2 {
		t.Fatalf("%s ladder read %d rungs; there is no ordering to prove", doc, len(out))
	}
	return out
}

// stubAWS writes an `aws` executable that prints the secret and never reaches a
// network. Returning it first on PATH is what makes a cloud call impossible.
func stubAWS(t *testing.T, dir, value string) string {
	t.Helper()
	binDir := filepath.Join(dir, "stub-bin")
	if err := os.MkdirAll(binDir, 0o700); err != nil {
		t.Fatalf("stub dir: %v", err)
	}
	script := "#!/bin/sh\nprintf %s " + shellSingleQuote(value) + "\n"
	path := filepath.Join(binDir, "aws")
	if runtime.GOOS == "windows" {
		path += ".cmd"
		script = "@echo off\r\necho " + value + "\r\n"
	}
	if err := os.WriteFile(path, []byte(script), 0o700); err != nil {
		t.Fatalf("write stub aws: %v", err)
	}
	return binDir
}

func shellSingleQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// run writes the fixture out and executes it, returning the path the server was
// asked for — which carries the variable's winning value.
func (f *ladderFixture) run(t *testing.T, bin string) string {
	t.Helper()

	var seen string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer srv.Close()

	dir := t.TempDir()

	// curlew.yaml: project variables (2) and the vault block (6).
	project := "project_name: ladder\n"
	if f.project != "" {
		project += fmt.Sprintf("variables:\n  %s: %q\n", f.name, f.project)
	}
	if f.vault != "" {
		project += fmt.Sprintf("secrets:\n  provider: aws-secrets-manager\n  region: us-east-1\n"+
			"  keys:\n    %s: \"ladder/secret\"\n", f.name)
	}
	if err := os.WriteFile(filepath.Join(dir, "curlew.yaml"), []byte(project), 0o600); err != nil {
		t.Fatalf("write curlew.yaml: %v", err)
	}

	args := []string{"run", filepath.Join(dir, "c.yaml")}

	// environments/e.yaml (3)
	if f.envFile != "" {
		if err := os.MkdirAll(filepath.Join(dir, "environments"), 0o700); err != nil {
			t.Fatalf("environments dir: %v", err)
		}
		body := fmt.Sprintf("variables:\n  %s: %q\n", f.name, f.envFile)
		if err := os.WriteFile(filepath.Join(dir, "environments", "e.yaml"), []byte(body), 0o600); err != nil {
			t.Fatalf("write environment: %v", err)
		}
		args = append(args, "--env", "e")
	}

	// .env (4)
	if f.dotEnv != "" {
		body := fmt.Sprintf("%s=%s\n", f.name, f.dotEnv)
		if err := os.WriteFile(filepath.Join(dir, ".env"), []byte(body), 0o600); err != nil {
			t.Fatalf("write .env: %v", err)
		}
	}

	// The collection: from_command (5), collection variables (7), item
	// variables (8). from_command and a plain collection value are the same
	// YAML key, so no pair ever needs both.
	collection := "name: ladder\n"
	switch {
	case f.fromCommand != "":
		command := "printf %s " + shellSingleQuote(f.fromCommand)
		if runtime.GOOS == "windows" {
			command = "[Console]::Write('" + strings.ReplaceAll(f.fromCommand, "'", "''") + "')"
		}
		collection += fmt.Sprintf("variables:\n  %s:\n    from_command: %q\n", f.name, command)
	case f.collection != "":
		collection += fmt.Sprintf("variables:\n  %s: %q\n", f.name, f.collection)
	}
	reference := "{{" + f.name + "}}"
	collection += "requests:\n  - name: One\n"
	if f.item != "" {
		collection += fmt.Sprintf("    variables:\n      %s: %q\n", f.name, f.item)
	}
	collection += fmt.Sprintf("    request:\n      method: GET\n      url: \"%s/v/%s\"\n", srv.URL, reference)
	if err := os.WriteFile(filepath.Join(dir, "c.yaml"), []byte(collection), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}

	env := append([]string{}, os.Environ()...)
	env = append(env, "NO_COLOR=1")

	// --env-var (9) imports the OS variable of the same name.
	if f.envVar != "" {
		env = append(env, f.name+"="+f.envVar)
		args = append(args, "--env-var", f.name)
	}
	// --var (10)
	if f.cliVar != "" {
		args = append(args, "--var", f.name+"="+f.cliVar)
	}
	// The stub AWS CLI goes first on PATH whenever rung 6 is in play.
	if f.vault != "" {
		env = append(env, "PATH="+stubAWS(t, dir, f.vault)+string(os.PathListSeparator)+os.Getenv("PATH"))
	}

	cmd := exec.Command(bin, args...)
	cmd.Env = env
	cmd.Dir = dir
	out, runErr := cmd.CombinedOutput()
	if runErr != nil {
		t.Fatalf("run: %v\n%s", runErr, out)
	}
	if seen == "" {
		t.Fatalf("the server was never called:\n%s", out)
	}
	return strings.TrimPrefix(seen, "/v/")
}

// uuidShape is what the $uuid dynamic function produces, and the only way to
// recognise rung 1's contribution: it has no fixed value to compare against.
var uuidShape = regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-`)

// Every adjacent pair of the ladder, run.
func TestDocTables_precedenceLadderIsTheOrderThatHappens(t *testing.T) {
	bin := buildBinary(t)

	for _, pt := range precedenceTables {
		t.Run(pt.doc, func(t *testing.T) {
			ladder := readLadder(t, pt.doc, pt.header)

			for i := 0; i+1 < len(ladder); i++ {
				lower, higher := ladder[i], ladder[i+1]
				t.Run(fmt.Sprintf("%d_%s_under_%d_%s", lower.number, lower.source, higher.number, higher.source),
					func(t *testing.T) {
						f := &ladderFixture{name: "ladder_var"}
						lower.rung.apply(f, "LOWER")
						if f.dynamic {
							// Rung 1 has no name of its own: a dynamic function is
							// only overridable by a variable spelled the same way.
							f.name = "$uuid"
						}
						higher.rung.apply(f, "HIGHER")

						got := f.run(t, bin)
						if got != "HIGHER" {
							t.Errorf("%s says %d (%s) beats %d (%s); the request carried %q",
								pt.doc, higher.number, higher.source, lower.number, lower.source, got)
						}
					})
			}
		})
	}
}

// The lowest rung must actually supply a value, or every pair above it proves
// nothing: a source that contributes nothing is beaten by everything.
func TestDocTables_theBottomRungSuppliesAValue(t *testing.T) {
	bin := buildBinary(t)

	for _, pt := range precedenceTables {
		ladder := readLadder(t, pt.doc, pt.header)
		bottom := ladder[0]

		f := &ladderFixture{name: "ladder_var"}
		bottom.rung.apply(f, "LOWER")
		if f.dynamic {
			f.name = "$uuid"
		}

		got := f.run(t, bin)
		switch {
		case f.dynamic:
			if !uuidShape.MatchString(got) {
				t.Errorf("%s rung 1 is %s, and {{$uuid}} alone produced %q, which is not a uuid",
					pt.doc, bottom.source, got)
			}
		case got != "LOWER":
			t.Errorf("%s rung 1 is %s, and defining the variable there alone produced %q",
				pt.doc, bottom.source, got)
		}
	}
}

// Every rung must supply a value on its own. A rung that silently contributes
// nothing would let the pair below it pass for the wrong reason.
func TestDocTables_everyRungSuppliesAValueOnItsOwn(t *testing.T) {
	bin := buildBinary(t)

	for _, pt := range precedenceTables {
		t.Run(pt.doc, func(t *testing.T) {
			for _, row := range readLadder(t, pt.doc, pt.header) {
				t.Run(row.source, func(t *testing.T) {
					f := &ladderFixture{name: "ladder_var"}
					row.rung.apply(f, "MINE")
					if f.dynamic {
						f.name = "$uuid"
						if got := f.run(t, bin); !uuidShape.MatchString(got) {
							t.Errorf("rung %d (%s) alone produced %q, not a uuid", row.number, row.source, got)
						}
						return
					}
					if got := f.run(t, bin); got != "MINE" {
						t.Errorf("rung %d (%s) alone produced %q, not the value it defines",
							row.number, row.source, got)
					}
				})
			}
		})
	}
}

// The two documents must describe the same ladder.
func TestDocTables_bothLaddersAgree(t *testing.T) {
	var seen [][]string
	for _, pt := range precedenceTables {
		var order []string
		for _, row := range readLadder(t, pt.doc, pt.header) {
			order = append(order, fmt.Sprintf("%d:%s", row.number, row.rung.key))
		}
		seen = append(seen, order)
	}
	// Request-item and Request-scoped are the same rung under two names, which
	// is a wording difference rather than a disagreement about the order.
	normalise := func(s []string) string {
		out := strings.Join(s, ",")
		return strings.ReplaceAll(out, "Request-scoped", "Request-item")
	}
	if len(seen) == 2 && normalise(seen[0]) != normalise(seen[1]) {
		t.Errorf("the two precedence ladders disagree:\n  specification: %v\n  manual:        %v", seen[0], seen[1])
	}
}
