package main

import (
	"bytes"
	"go/ast"
	"go/parser"
	"go/token"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// M23-001: help text is a contract surface, and it drifted.
//
// Three flags reached users while `curlew --help` stayed silent about them:
// --events (a documented feature in the README), --project (curlew schema's
// project-schema selector), and the `markdown` value of --format, which the
// CLI's own error message advertised but its help did not.
//
// Each of those was added by writing a `case "--flag":` in an argument parser
// and forgetting the corresponding Fprintln. Nothing failed. The tests below
// derive the accepted set from the parsers themselves, so the next flag added
// without a help line fails on the day it is added rather than on the day a
// user files an issue.

// helpFlagExceptions lists flags that are deliberately absent from help.
// Keep this empty unless there is a stated reason: an entry here is a promise
// that users are not meant to discover the flag.
var helpFlagExceptions = map[string]string{}

// runSynopsisExceptions lists flags deliberately absent from the run parse-
// error synopsis (main.go's runUsageSynopsis). Keep this empty unless there
// is a stated reason: an entry here is a promise that users are not meant to
// see the flag at the moment they mistype one.
var runSynopsisExceptions = map[string]string{}

// allHelpText concatenates every help surface the CLI can print. A flag is
// "documented" if it appears in any of them — subcommand flags belong in their
// subcommand's help, not in the top-level block.
func allHelpText(t *testing.T) string {
	t.Helper()

	printers := []struct {
		name string
		fn   func(io.Writer)
	}{
		{"top-level", printHelpTo},
		{"init", printInitHelpTo},
		{"vault", printVaultHelpTo},
		{"pr-check", printPrCheckHelpTo},
		{"perf", printPerfHelpTo},
		{"plugins", printPluginsHelpTo},
		{"telemetry", printTelemetryHelpTo},
		{"ui", printUIHelpTo},
	}

	var all strings.Builder
	for _, p := range printers {
		var buf bytes.Buffer
		p.fn(&buf)
		if buf.Len() == 0 {
			t.Errorf("%s help printer produced no output", p.name)
		}
		all.WriteString(buf.String())
		all.WriteString("\n")
	}
	return all.String()
}

// walkFlagLiterals visits every string literal in this package's non-test
// sources that a parser compares an argument against — a `case "--flag":`
// value or an `==` operand — and calls record with the unquoted value.
// Flags are accepted two ways: a `case "--flag":` in a switch, and an
// `if a == "--flag"` outside one. Reading only the case clauses missed the
// second kind entirely — `--clear` on `curlew watch` was accepted by the
// binary and invisible to this test.
//
// When funcName is non-empty the walk is confined to that function's body,
// and the walk fails if no function of that name exists in the package: a
// renamed parser must fail loudly rather than enumerate nothing.
func walkFlagLiterals(t *testing.T, funcName string, record func(val string)) {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	fset := token.NewFileSet()
	found := funcName == ""

	recordLit := func(lit *ast.BasicLit) {
		if lit.Kind != token.STRING {
			return
		}
		val, err := strconv.Unquote(lit.Value)
		if err != nil {
			return
		}
		record(val)
	}
	visit := func(n ast.Node) bool {
		switch node := n.(type) {
		case *ast.CaseClause:
			for _, expr := range node.List {
				if lit, ok := expr.(*ast.BasicLit); ok {
					recordLit(lit)
				}
			}
		case *ast.BinaryExpr:
			// `a == "--flag"` or `"--flag" == a`.
			if node.Op != token.EQL {
				return true
			}
			if lit, ok := node.X.(*ast.BasicLit); ok {
				recordLit(lit)
			}
			if lit, ok := node.Y.(*ast.BasicLit); ok {
				recordLit(lit)
			}
		}
		return true
	}

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		if funcName == "" {
			ast.Inspect(file, visit)
			continue
		}
		for _, decl := range file.Decls {
			fn, ok := decl.(*ast.FuncDecl)
			if !ok || fn.Name.Name != funcName || fn.Body == nil {
				continue
			}
			found = true
			ast.Inspect(fn.Body, visit)
		}
	}

	if !found {
		t.Fatalf("no function named %s found in this package — the walk target was renamed", funcName)
	}
}

// acceptedFlags returns every long flag any parser in this package accepts.
// Reading the parsers rather than a hand-maintained list is the point: a
// hand-maintained list drifts the same way the help text did.
//
// Signature preserved: doc_prose_test.go depends on it.
func acceptedFlags(t *testing.T) []string {
	t.Helper()

	seen := map[string]bool{}
	walkFlagLiterals(t, "", func(val string) {
		if strings.HasPrefix(val, "--") && len(val) > 2 {
			seen[val] = true
		}
	})

	if len(seen) == 0 {
		t.Fatal("found no accepted flags — the AST walk is broken, not the CLI")
	}

	flags := make([]string, 0, len(seen))
	for f := range seen {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	return flags
}

// acceptedArgTokensIn returns every flag-shaped token, long or short, that
// the named parser function accepts, sorted. Widening beyond acceptedFlags's
// `--`-only filter to include short flags like -v/-vv/-q costs nothing here
// — all three are already listed wherever they need to be — and is strictly
// stronger: a short flag added without a synopsis entry now fails too.
//
// t.Fatals if fewer than 10 tokens come back: the floor guard that keeps a
// broken walk from reporting a vacuous, all-green pass (the same pattern as
// this repo's len(supportedLocales) != 15 / len(commands) < 5 checks).
func acceptedArgTokensIn(t *testing.T, funcName string) []string {
	t.Helper()

	seen := map[string]bool{}
	walkFlagLiterals(t, funcName, func(val string) {
		if len(val) > 1 && strings.HasPrefix(val, "-") {
			seen[val] = true
		}
	})

	if len(seen) < 10 {
		t.Fatalf("found only %d flag-shaped tokens in %s — the walk is broken, not the CLI", len(seen), funcName)
	}

	flags := make([]string, 0, len(seen))
	for f := range seen {
		flags = append(flags, f)
	}
	sort.Strings(flags)
	return flags
}

// flagSurface is one place the CLI tells a user which flags exist. A third
// surface joins the check by adding a value here and asserting it in a new
// entry-point test — the watch synopsis (main.go's watchCmdOut) is the known
// next candidate; see docs/PRODUCT_ROADMAP.md.
type flagSurface struct {
	name    string                    // for failure messages
	text    func(t *testing.T) string // the surface as a user sees it
	flags   func(t *testing.T) []string
	exempt  map[string]string // deliberately absent, with a reason
	fixHint string            // what to edit when a flag is missing
}

// helpSurface is every print*HelpTo writer, checked against every flag any
// parser in the package accepts.
var helpSurface = flagSurface{
	name:    "help",
	text:    allHelpText,
	flags:   acceptedFlags,
	exempt:  helpFlagExceptions,
	fixHint: "add a line to the relevant print*HelpTo, or record an exception in helpFlagExceptions",
}

// runSynopsisSurface is the one-line synopsis `curlew run` prints on a parse
// error, checked against the flags parseRunArgs itself accepts.
var runSynopsisSurface = flagSurface{
	name: "run usage synopsis",
	text: runParseErrorStderr,
	flags: func(t *testing.T) []string {
		return acceptedArgTokensIn(t, "parseRunArgs")
	},
	exempt:  runSynopsisExceptions,
	fixHint: "add the flag to runUsageSynopsis in cmd/curlew/main.go, or record an exception in runSynopsisExceptions",
}

// runParseErrorStderr returns the first line curlew writes to stderr when the
// run parser rejects an argument — the synopsis a user sees at the exact
// moment they mistyped a flag. Drives the real dispatcher (runWithWriters,
// production code) rather than reading the const directly, so the test
// asserts what a user actually sees. The parse-error branch returns before
// any file I/O, events emitter, or telemetry defer runs, so this call has no
// side effects.
func runParseErrorStderr(t *testing.T) string {
	t.Helper()
	var stdout, stderr bytes.Buffer
	runWithWriters([]string{"run", "--definitely-not-a-flag"}, &stdout, &stderr)
	line, _, _ := strings.Cut(stderr.String(), "\n")
	if !strings.HasPrefix(line, "Usage: curlew run") {
		t.Fatalf("first stderr line is not the run synopsis; got %q", line)
	}
	return line
}

// assertSurfaceDocumentsFlags checks that every flag surface s is
// responsible for appears in its text as its own token. One sub-test per
// flag, so a single missing flag names itself in `go test -run` output
// instead of disappearing into one combined failure.
func assertSurfaceDocumentsFlags(t *testing.T, s flagSurface) {
	t.Helper()
	text := s.text(t)

	for _, flag := range s.flags(t) {
		t.Run(flag, func(t *testing.T) {
			if reason, exempt := s.exempt[flag]; exempt {
				t.Logf("%s deliberately undocumented in %s: %s", flag, s.name, reason)
				return
			}
			// Match the flag followed by a word boundary so that "--env" does
			// not count itself as documented by a line describing "--env-var".
			if !mentionsFlag(text, flag) {
				t.Errorf("%s is accepted but appears in no %s output —\n%s", flag, s.name, s.fixHint)
			}
		})
	}
}

func TestHelp_documents_every_accepted_flag(t *testing.T) {
	assertSurfaceDocumentsFlags(t, helpSurface)
}

// TestUsage_synopsis_lists_every_accepted_flag guards the second surface a
// user sees at the exact moment they mistype a run flag: the one-line
// synopsis printed on a parse error. TestHelp_documents_every_accepted_flag
// only ever covered curlew --help; nothing caught the synopsis itself
// falling behind parseRunArgs.
func TestUsage_synopsis_lists_every_accepted_flag(t *testing.T) {
	assertSurfaceDocumentsFlags(t, runSynopsisSurface)
}

// mentionsFlag reports whether help documents flag as its own token, rather
// than as a prefix of a longer flag.
func mentionsFlag(help, flag string) bool {
	for idx := 0; ; {
		i := strings.Index(help[idx:], flag)
		if i < 0 {
			return false
		}
		end := idx + i + len(flag)
		if end >= len(help) {
			return true
		}
		switch c := help[end]; {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			// Prefix of a longer flag; keep looking.
		default:
			return true
		}
		idx = end
	}
}

func TestHelp_run_format_list_matches_the_error_message(t *testing.T) {
	// The CLI already states its supported formats when it rejects one. That
	// message and the help text are two user-facing answers to the same
	// question, and they disagreed: the error listed markdown, the help did not.
	var stdout, stderr bytes.Buffer
	runWithWriters([]string{"run", "nonexistent.yaml", "--format", "definitely-not-a-format"}, &stdout, &stderr)

	msg := stderr.String()
	const marker = "supported: "
	i := strings.Index(msg, marker)
	if i < 0 {
		t.Fatalf("format error message no longer lists supported formats; got: %q", msg)
	}
	list := msg[i+len(marker):]
	if j := strings.IndexAny(list, ")\n"); j >= 0 {
		list = list[:j]
	}

	formats := strings.Split(list, ",")
	if len(formats) < 2 {
		t.Fatalf("parsed %d formats from %q — the parse is broken, not the CLI", len(formats), list)
	}

	// Scope the assertion to the Run Options "--format" line itself. Asserting
	// against the whole help text is a false pass: `curlew init --help` also
	// lists output formats, so a format missing from Run Options still looked
	// documented. That is how this drifted in the first place.
	line := runFormatHelpLine(t)
	for _, f := range formats {
		f = strings.TrimSpace(f)
		if f == "" {
			continue
		}
		if !strings.Contains(line, f) {
			t.Errorf("--format %s is accepted (the error message advertises it) but the Run Options help line omits it.\nline: %q", f, line)
		}
	}
}

func TestReadme_lists_every_command_the_CLI_advertises(t *testing.T) {
	// The README drifted behind the CLI: schema, vault, pr-check and telemetry
	// all shipped without ever reaching it. Tying it to the help output means
	// a new command has to be documented in both places or this fails.
	readme, err := os.ReadFile(filepath.Join("..", "..", "README.md"))
	if err != nil {
		t.Fatalf("read README.md: %v", err)
	}
	text := string(readme)

	commands := advertisedCommands(t)
	if len(commands) < 5 {
		t.Fatalf("parsed only %d commands from help — the parse is broken, not the README", len(commands))
	}

	for _, cmd := range commands {
		if !strings.Contains(text, cmd) {
			t.Errorf("`curlew %s` is advertised in --help but never mentioned in README.md", cmd)
		}
	}
}

// advertisedCommands returns the command names listed in the "Commands:"
// section of the top-level help. Continuation lines (which are indented
// further than the two spaces a command entry uses) are skipped.
func advertisedCommands(t *testing.T) []string {
	t.Helper()

	var buf bytes.Buffer
	printHelpTo(&buf)

	seen := map[string]bool{}
	var out []string
	inCommands := false
	for _, line := range strings.Split(buf.String(), "\n") {
		if strings.HasPrefix(line, "Commands:") {
			inCommands = true
			continue
		}
		if !inCommands {
			continue
		}
		if line == "" {
			break // end of the Commands block
		}
		if !strings.HasPrefix(line, "  ") || strings.HasPrefix(line, "   ") {
			continue // continuation line, not a command entry
		}
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		name := fields[0]
		if !seen[name] {
			seen[name] = true
			out = append(out, name)
		}
	}
	return out
}

// runFormatHelpLine returns the "--format <type>" line from the Run Options
// section of the top-level help.
func runFormatHelpLine(t *testing.T) string {
	t.Helper()

	var buf bytes.Buffer
	printHelpTo(&buf)

	inRunOptions := false
	for _, line := range strings.Split(buf.String(), "\n") {
		switch {
		case strings.HasPrefix(line, "Run Options:"):
			inRunOptions = true
			continue
		case inRunOptions && line != "" && !strings.HasPrefix(line, " "):
			// Left the Run Options block without finding the line.
			t.Fatal("no '--format' line inside the Run Options section of help")
		}
		if inRunOptions && strings.Contains(line, "--format") {
			return line
		}
	}
	t.Fatal("no Run Options section with a '--format' line in help")
	return ""
}
