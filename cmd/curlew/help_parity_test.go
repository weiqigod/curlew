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

// acceptedFlags walks every non-test source file in this package and collects
// the string literals used as `case` values in switch statements — i.e. the
// exact set of arguments the CLI accepts. Reading the parsers rather than a
// hand-maintained list is the point: a hand-maintained list drifts the same
// way the help text did.
func acceptedFlags(t *testing.T) []string {
	t.Helper()

	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatalf("read package dir: %v", err)
	}

	seen := map[string]bool{}
	fset := token.NewFileSet()

	for _, e := range entries {
		name := e.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(".", name), nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", name, err)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			clause, ok := n.(*ast.CaseClause)
			if !ok {
				return true
			}
			for _, expr := range clause.List {
				lit, ok := expr.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					continue
				}
				val, err := strconv.Unquote(lit.Value)
				if err != nil {
					continue
				}
				if strings.HasPrefix(val, "--") && len(val) > 2 {
					seen[val] = true
				}
			}
			return true
		})
	}

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

func TestHelp_documents_every_accepted_flag(t *testing.T) {
	help := allHelpText(t)

	for _, flag := range acceptedFlags(t) {
		if reason, exempt := helpFlagExceptions[flag]; exempt {
			t.Logf("%s deliberately undocumented: %s", flag, reason)
			continue
		}
		// Match the flag followed by a word boundary so that "--env" does not
		// count itself as documented by a line describing "--env-var".
		if !mentionsFlag(help, flag) {
			t.Errorf("%s is accepted by an argument parser but appears in no help output —\n"+
				"add a line to the relevant print*HelpTo, or record an exception in helpFlagExceptions", flag)
		}
	}
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
