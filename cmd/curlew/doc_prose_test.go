package main

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// Prose, held to the binary by the names it uses.
//
// The documents state things about curlew in three ways. Examples are parsed on
// every build (internal/parser); tables are executed (internal/docs); prose was
// checked by nothing, and prose is the largest surface of the three.
//
// A sentence cannot be executed. But almost every prose claim worth making
// NAMES something concrete — a command, a flag, an environment variable — and
// those names can be checked even when the sentence around them cannot. That is
// not a general solution to prose, and it is not offered as one. It is aimed at
// the specific drift this repository has actually suffered:
//
//   - the licensing strip removed `curlew license`
//   - the backend strip removed `curlew login`, `curlew worker`, `--workers`,
//     `--report-upload` and every CURLEW_BACKEND_* / CURLEW_COORDINATOR_URL
//     variable
//   - M21-002 was four docs/MANUAL.md surfaces still describing the removed
//     backend, found by reading, months later
//
// Every one of those left prose naming something that no longer existed, and
// nothing failed. A name is the checkable part of a sentence, so these tests
// check names.

// proseDocs are the user-facing contracts. CHANGELOG.md and
// TESTAPI_SPECIFICATION.md are deliberately excluded: both discuss removed and
// unbuilt things on purpose, and holding them to the current binary would be
// wrong rather than merely noisy.
var proseDocs = []string{"MANUAL.md", "CLI_SPECIFICATION.md"}

func readDoc(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("..", "..", "docs", name))
	if err != nil {
		t.Fatalf("reading %s: %v", name, err)
	}
	return string(data)
}

// docLine is one checkable line and its 1-based position.
type docLine struct {
	num  int
	text string
}

// ignoreMarker suppresses name checking from the line it appears on until the
// next markdown heading.
//
// Some sections name removed things ON PURPOSE — CLI_SPECIFICATION's
// "Deliberately Absent Surfaces" appendix and the manual's "No account, no
// backend" both exist to tell a reader that `curlew login` is gone rather than
// missing. That is good documentation, and flagging it would be wrong.
//
// The suppression is an explicit marker in the document rather than a list of
// headings in this test, so an exemption is a visible, deliberate act at the
// place it applies, and renaming a heading cannot silently widen it.
const ignoreMarker = "<!-- doc-check: ignore-names -->"

// checkableLines returns the document's lines minus any suppressed region.
// Fenced blocks are included: a command named in an example is as much a claim
// as one named in a sentence.
func checkableLines(t *testing.T, name string) []docLine {
	t.Helper()
	all := strings.Split(readDoc(t, name), "\n")

	out := make([]docLine, 0, len(all))
	suppressed := 0
	ignoring := false
	for i, text := range all {
		if strings.Contains(text, ignoreMarker) {
			ignoring = true
			continue
		}
		if ignoring {
			if strings.HasPrefix(text, "#") {
				ignoring = false
			} else {
				suppressed++
				continue
			}
		}
		out = append(out, docLine{num: i + 1, text: text})
	}

	// A marker that swallowed the document would turn every test here into a
	// vacuous pass, which is the failure mode this whole exercise exists to
	// remove.
	if len(all) > 0 && suppressed*100/len(all) > 15 {
		t.Fatalf("%s: %d of %d lines are suppressed by %s — the exemption is too "+
			"wide to be deliberate", name, suppressed, len(all), ignoreMarker)
	}
	return out
}

var (
	curlewCommandRe = regexp.MustCompile(`curlew\s+([a-z][a-z-]*)`)
	flagRe          = regexp.MustCompile(`--[a-z][a-z0-9-]*`)
	envVarRe        = regexp.MustCompile(`CURLEW_[A-Z0-9_]+`)
)

// nonCommandWords follow "curlew " in prose without naming a subcommand —
// "curlew run" is a command, "curlew reads the body" is a sentence. Each entry
// is a word that would otherwise be mistaken for one.
var nonCommandWords = map[string]bool{
	"is": true, "was": true, "does": true, "will": true, "can": true,
	"reads": true, "writes": true, "never": true, "always": true,
	"resolves": true, "treats": true, "sends": true, "exits": true,
	"and": true, "or": true, "to": true, "in": true, "with": true,
	"has": true, "had": true, "supports": true, "requires": true,
	"collection": true, "collections": true, "binary": true,
	"itself": true, "then": true, "also": true, "only": true,
	"stores": true, "keeps": true, "loads": true, "applies": true,
	"uses": true, "runs": true, "makes": true, "prints": true,
	"emits": true, "sent": true, "received": true, "adds": true, "sets": true, "needs": true,
	"cannot": true, "must": true, "may": true, "should": true,
	"the": true, "a": true, "an": true, "no": true, "not": true,
	"does-not": true, "on": true, "at": true, "by": true, "for": true,
	"from": true, "of": true, "as": true, "into": true, "that": true,
	"this": true, "these": true, "those": true, "when": true, "where": true,
	"generates": true, "produces": true, "returns": true, "reports": true,
	"parses": true, "validates": true, "records": true, "measures": true,
}

func TestProse_everyNamedCommandExists(t *testing.T) {
	commands := map[string]bool{}
	for _, c := range advertisedCommands(t) {
		commands[c] = true
	}
	if len(commands) == 0 {
		t.Fatal("no commands advertised; this test would pass vacuously")
	}

	checked := 0
	for _, doc := range proseDocs {
		for _, dl := range checkableLines(t, doc) {
			for _, m := range curlewCommandRe.FindAllStringSubmatch(dl.text, -1) {
				word := m[1]
				if nonCommandWords[word] {
					continue
				}
				checked++
				if !commands[word] {
					t.Errorf("docs/%s:%d names `curlew %s`, which is not a command "+
						"(the CLI advertises: %s)", doc, dl.num, word, strings.Join(sortedKeysOf(commands), ", "))
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no `curlew <command>` mentions found; the extraction is broken")
	}
	t.Logf("checked %d command mentions", checked)
}

func TestProse_everyNamedFlagIsAccepted(t *testing.T) {
	accepted := map[string]bool{}
	for _, f := range acceptedFlags(t) {
		accepted[f] = true
	}
	if len(accepted) == 0 {
		t.Fatal("no flags accepted; this test would pass vacuously")
	}

	checked := 0
	for _, doc := range proseDocs {
		for _, dl := range checkableLines(t, doc) {
			// Only flags on a line that also names curlew. A document may show
			// `curl --cacert` or `git diff --name-only`, and those are not
			// curlew's to accept.
			if !strings.Contains(dl.text, "curlew") {
				continue
			}
			for _, flag := range flagRe.FindAllString(dl.text, -1) {
				checked++
				if !accepted[flag] {
					t.Errorf("docs/%s:%d names %s on a curlew command line, "+
						"which no argument parser accepts", doc, dl.num, flag)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no flags found on any curlew command line; the extraction is broken")
	}
	t.Logf("checked %d flag mentions", checked)
}

// TestProse_everyNamedEnvironmentVariableIsRead is the check that would have
// caught M21-002 on the day of the strip. Every CURLEW_* variable the documents
// name must be read somewhere in the source; a variable that survives only in
// prose is a feature the reader will try to use and find missing.
func TestProse_everyNamedEnvironmentVariableIsRead(t *testing.T) {
	source := goSourceText(t)

	checked := 0
	for _, doc := range proseDocs {
		for _, dl := range checkableLines(t, doc) {
			for _, v := range envVarRe.FindAllString(dl.text, -1) {
				checked++
				if !strings.Contains(source, v) {
					t.Errorf("docs/%s:%d names %s, which no Go source reads", doc, dl.num, v)
				}
			}
		}
	}
	if checked == 0 {
		t.Fatal("no CURLEW_* variables found in the documents; the extraction is broken")
	}
	t.Logf("checked %d environment variable mentions", checked)
}

// goSourceText concatenates every non-test Go file in the module, so a name can
// be looked for without guessing which package owns it.
func goSourceText(t *testing.T) string {
	t.Helper()
	root := filepath.Join("..", "..")
	var sb strings.Builder
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			switch info.Name() {
			case ".git", "node_modules", "web", "src", "docs", "management":
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		sb.Write(data)
		sb.WriteByte('\n')
		return nil
	})
	if err != nil {
		t.Fatalf("walking source: %v", err)
	}
	if sb.Len() == 0 {
		t.Fatal("no Go source read; this test would pass vacuously")
	}
	return sb.String()
}

func sortedKeysOf(m map[string]bool) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// maxIgnoreMarkers bounds how many sections may opt out of name checking.
//
// A marker cannot outlive the next heading, so no single one can blanket a
// document — but a scattering of them could hollow the checks out a section at
// a time, which is how a guard dies quietly. Raising this number is a
// deliberate act that shows up in a diff, next to the reason.
//
// The two that exist name removed things ON PURPOSE, which is the only reason
// the exemption is defensible at all:
//
//	CLI_SPECIFICATION.md  Appendix B — Deliberately Absent Surfaces
//	MANUAL.md             §6.9 No account, no backend
const maxIgnoreMarkers = 2

func TestProse_ignoreMarkersAreFewAndDeliberate(t *testing.T) {
	var found []string
	for _, doc := range proseDocs {
		for i, line := range strings.Split(readDoc(t, doc), "\n") {
			if strings.Contains(line, ignoreMarker) {
				found = append(found, doc+":"+itoaLine(i+1))
			}
		}
	}
	if len(found) > maxIgnoreMarkers {
		t.Errorf("%d ignore markers, at most %d expected: %s\n"+
			"Each one exempts a section from being held to the binary. If the new "+
			"one is justified, raise maxIgnoreMarkers and say why.",
			len(found), maxIgnoreMarkers, strings.Join(found, ", "))
	}
	if len(found) == 0 {
		t.Fatal("no ignore markers found; the suppression mechanism is dead code " +
			"and the sections that need it are being checked wrongly")
	}
	t.Logf("ignore markers: %s", strings.Join(found, ", "))
}

func itoaLine(n int) string {
	if n == 0 {
		return "0"
	}
	var b []byte
	for n > 0 {
		b = append([]byte{byte('0' + n%10)}, b...)
		n /= 10
	}
	return string(b)
}
