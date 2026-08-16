package main

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"
)

// The skill tells an agent to run `curlew <subcommand>` in a dozen places.
// `curlew license` was one of them, and it named a command that had already
// been removed. doc_prose_test.go's TestProse_everyNamedCommandExists checks
// the same shape of claim in docs/ using a curlew\s+([a-z][a-z-]*) regex over
// raw prose, which needs a 60-entry nonCommandWords blocklist ("with", "to",
// "reads", ...) because prose says things like "curlew reads the body" that
// are not invocations.
//
// Scoping extraction to markdown code spans instead of prose removes the
// need for that blocklist: a sentence is never wrapped in backticks, so
// "curlew with an agent" produces no code span to even look at. What is left
// to check is only ever something an agent could actually copy into a shell.

// inlineCodeRe matches one inline code span, not crossing a line boundary —
// backtick pairs in markdown do not span lines.
var inlineCodeRe = regexp.MustCompile("`([^`\n]+)`")

// codeSpan is one span of code-formatted text and the 1-based line it starts
// on.
type codeSpan struct {
	text string
	line int
}

// codeSpansWithLines returns the contents of every inline code span and
// fenced code block in body, each paired with its starting line.
func codeSpansWithLines(body string) []codeSpan {
	var out []codeSpan
	inFence := false
	fenceStart := 0
	var fenceLines []string

	lines := strings.Split(body, "\n")
	for i, line := range lines {
		lineNo := i + 1
		if strings.HasPrefix(strings.TrimSpace(line), "```") {
			if inFence {
				out = append(out, codeSpan{text: strings.Join(fenceLines, "\n"), line: fenceStart})
				fenceLines = nil
			} else {
				fenceStart = lineNo
			}
			inFence = !inFence
			continue
		}
		if inFence {
			fenceLines = append(fenceLines, line)
			continue
		}
		for _, m := range inlineCodeRe.FindAllStringSubmatch(line, -1) {
			out = append(out, codeSpan{text: m[1], line: lineNo})
		}
	}
	return out
}

// codeSpans returns the contents of every inline code span (`...`) and
// fenced code block (```...```) in body, in order. Scoping to code rather
// than prose is what removes the need for a blocklist: doc_prose_test.go
// carries a 60-entry nonCommandWords map for exactly the problem this
// sidesteps.
func codeSpans(body string) []string {
	spans := codeSpansWithLines(body)
	out := make([]string, 0, len(spans))
	for _, s := range spans {
		out = append(out, s.text)
	}
	return out
}

// commandInvocationRe matches a `curlew <word>` invocation inside code-span
// text. Applying the same shape of pattern doc_prose_test.go uses, but only
// to text already known to be code, is what makes the blocklist unnecessary
// here: a real prose false positive like "curlew with" does not occur inside
// a code span in this skill's actual content.
var commandInvocationRe = regexp.MustCompile(`curlew\s+([a-z][a-z-]*)`)

// allSkillFiles scaffolds `curlew init --skill agent` and returns every
// scaffolded skill markdown file keyed by base name — unlike skillFiles
// (cmd/curlew/skill_exit_codes_test.go), which is scoped to the three
// exit-code statements, a `curlew <subcommand>` invocation can appear in any
// topic file.
func allSkillFiles(t *testing.T) map[string]string {
	t.Helper()
	tree := scaffoldTreeWithSkill(t, "agent")
	const root = ".claude/skills/curlew/"
	out := map[string]string{}
	for path, body := range tree {
		name, ok := strings.CutPrefix(path, root)
		if !ok || !strings.HasSuffix(name, ".md") {
			continue
		}
		out[name] = body
	}
	if len(out) == 0 {
		t.Fatalf("no .md files found under %s in the scaffolded tree", root)
	}
	return out
}

// skillNamedCommands maps each command named in a code span, across every
// skill file, to the "file:line" locations it was found at.
func skillNamedCommands(t *testing.T, files map[string]string) map[string][]string {
	t.Helper()
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names) // deterministic location ordering in failure messages

	out := map[string][]string{}
	for _, name := range names {
		for _, span := range codeSpansWithLines(files[name]) {
			for _, m := range commandInvocationRe.FindAllStringSubmatch(span.text, -1) {
				cmd := m[1]
				out[cmd] = append(out[cmd], fmt.Sprintf("%s:%d", name, span.line))
			}
		}
	}
	return out
}

// findFuncDecl parses every non-test .go file in dir and returns the
// top-level function declaration named name.
func findFuncDecl(t *testing.T, dir, name string) *ast.FuncDecl {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("read dir %s: %v", dir, err)
	}
	fset := token.NewFileSet()
	for _, e := range entries {
		fname := e.Name()
		if e.IsDir() || !strings.HasSuffix(fname, ".go") || strings.HasSuffix(fname, "_test.go") {
			continue
		}
		file, perr := parser.ParseFile(fset, filepath.Join(dir, fname), nil, 0)
		if perr != nil {
			t.Fatalf("parse %s: %v", fname, perr)
		}
		for _, decl := range file.Decls {
			if fd, ok := decl.(*ast.FuncDecl); ok && fd.Name.Name == name {
				return fd
			}
		}
	}
	t.Fatalf("%s not found in %s — the AST walk is broken, not the CLI", name, dir)
	return nil
}

// dispatchedCommands reads the `switch args[0]` in runWithWriters and
// returns its non-flag case values — the commands the binary actually
// accepts. AST-derived for the same reason acceptedFlags (help_parity_test.go)
// is: a hand-kept list drifts the way help text already has.
func dispatchedCommands(t *testing.T) []string {
	t.Helper()
	fn := findFuncDecl(t, ".", "runWithWriters")

	var sw *ast.SwitchStmt
	for _, stmt := range fn.Body.List {
		if s, ok := stmt.(*ast.SwitchStmt); ok {
			sw = s
			break
		}
	}
	if sw == nil {
		t.Fatal("no switch statement found in runWithWriters — the AST walk is broken, not the CLI")
	}

	seen := map[string]bool{}
	for _, stmt := range sw.Body.List {
		cc, ok := stmt.(*ast.CaseClause)
		if !ok {
			continue
		}
		for _, expr := range cc.List {
			lit, ok := expr.(*ast.BasicLit)
			if !ok || lit.Kind != token.STRING {
				continue
			}
			val, uerr := strconv.Unquote(lit.Value)
			if uerr != nil || strings.HasPrefix(val, "-") {
				continue // flags ("--version", "-h"), not commands
			}
			seen[val] = true
		}
	}

	if len(seen) == 0 {
		t.Fatal("no commands read from runWithWriters — the AST walk is broken, not the CLI")
	}
	out := make([]string, 0, len(seen))
	for c := range seen {
		out = append(out, c)
	}
	sort.Strings(out)
	return out
}

// TestSkill_names_only_real_commands asserts every `curlew <subcommand>`
// invocation named in a code span anywhere in the shipped skill is a command
// the CLI actually dispatches. Direction is one-way: the skill legitimately
// never mentions several real commands (perf, ui, vault, ...), and requiring
// it to would be a documentation mandate this task did not take on.
func TestSkill_names_only_real_commands(t *testing.T) {
	files := allSkillFiles(t)
	named := skillNamedCommands(t, files)
	if len(named) == 0 {
		t.Fatal("no curlew-command invocations found in any skill file — the code-span extraction is broken, not the skill")
	}

	dispatchedList := dispatchedCommands(t)
	dispatched := map[string]bool{}
	for _, c := range dispatchedList {
		dispatched[c] = true
	}

	cmds := make([]string, 0, len(named))
	for cmd := range named {
		cmds = append(cmds, cmd)
	}
	sort.Strings(cmds)

	for _, cmd := range cmds {
		if !dispatched[cmd] {
			t.Errorf("skill names `curlew %s` (%s) but the CLI has no such command (dispatched: %v)",
				cmd, strings.Join(named[cmd], ", "), dispatchedList)
		}
	}
}

// TestCLI_dispatched_and_advertised_commands_agree closes a real
// pre-existing hole: acceptedFlags (help_parity_test.go) collects only
// `--`-prefixed literals, so a `case "deploy":` added to the dispatcher and
// forgotten in --help is invisible to that test.
func TestCLI_dispatched_and_advertised_commands_agree(t *testing.T) {
	advertised := map[string]bool{}
	for _, c := range advertisedCommands(t) {
		advertised[c] = true
	}
	for _, c := range dispatchedCommands(t) {
		if !advertised[c] {
			t.Errorf("runWithWriters dispatches `curlew %s` but --help's Commands: section never lists it", c)
		}
	}
}

func TestCodeSpans(t *testing.T) {
	tests := []struct {
		name string
		body string
		want []string
	}{
		{"inline_code_span_is_an_invocation", "run `curlew validate` first", []string{"curlew validate"}},
		{"fenced_block_is_an_invocation", "```bash\ncurlew run x.yaml\n```", []string{"curlew run x.yaml"}},
		{"curlew_with_in_prose_is_not_an_invocation", "driving curlew with an agent", nil},
		{"curlew_to_in_prose_is_not_an_invocation", `"use curlew to ..."`, nil},
		{"curlew_skill_in_heading_is_not_an_invocation", "# curlew skill", nil},
		{"curlew_can_in_prose_is_not_an_invocation", "every code curlew can return", nil},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := codeSpans(tc.body)
			if fmt.Sprint(got) != fmt.Sprint(tc.want) {
				t.Errorf("codeSpans(%q) = %v, want %v", tc.body, got, tc.want)
			}
		})
	}
}
