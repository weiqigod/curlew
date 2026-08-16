package main

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/exitcodes"
)

// TestExitCodes_no_unreachable_mapping asserts every key in exitCodeSeverity
// is a code cmd/curlew can actually return.
//
// Containment, not equality, and the asymmetry is deliberate. 130 is
// reachable (a SIGINT during `curlew perf`, perf.go) but has no severity
// rank, because it is raised inside a single perf run and never takes part
// in the worst-wins fold across discovered collections. Requiring equality
// would force a rank for it, which is inventing an exit code's meaning. The
// direction that goes wrong is a code the map ranks but the binary cannot
// return, and that is the one asserted.
func TestExitCodes_no_unreachable_mapping(t *testing.T) {
	if len(exitCodeSeverity) == 0 {
		t.Fatal("exitCodeSeverity is empty — the map moved or was renamed; " +
			"this test is asserting nothing")
	}

	reachable := reachableExitCodes(t) // guarded: root, size, depth
	set := map[int]bool{}
	for _, v := range exitcodes.Set(reachable) {
		set[v] = true
	}

	for _, code := range sortedSeverityKeys() {
		if !set[code] {
			t.Errorf("exitCodeSeverity ranks exit %d, which cmd/curlew cannot return "+
				"(reachable: %v) — a severity rank for an unreachable code is not "+
				"documentation, it is a claim the feature might come back",
				code, exitcodes.Set(reachable))
		}
	}
}

// sortedSeverityKeys makes failure order deterministic; map iteration is not.
func sortedSeverityKeys() []int {
	out := make([]int, 0, len(exitCodeSeverity))
	for k := range exitCodeSeverity {
		out = append(out, k)
	}
	sort.Ints(out)
	return out
}

// exitCodeSurface is one place the project publishes an exit-code contract.
//
// full means the surface states the whole contract and must equal the
// reachable set. A per-command surface states a subset by design — `curlew
// ui` cannot fail an assertion — and is asserted by containment only.
type exitCodeSurface struct {
	name string // as it appears in failure output
	doc  string // "" for the skill; otherwise a file under docs/
	sect string // heading fragment, for the Step 5 coverage check
	full bool
	read func(*testing.T) []int
}

// exitCodeSurfaces enumerates every surface. The three full doc entries are
// built from exitCodeTables — doc_exit_codes_test.go's existing registry — so
// a table that moves is relocated once, not twice.
func exitCodeSurfaces() []exitCodeSurface {
	surfaces := make([]exitCodeSurface, 0, len(exitCodeTables)+3)
	for _, tbl := range exitCodeTables {
		tbl := tbl // capture for the closure below
		surfaces = append(surfaces, exitCodeSurface{
			name: fmt.Sprintf("docs/%s under %q", tbl.doc, tbl.where),
			doc:  tbl.doc,
			sect: tbl.where,
			full: true,
			read: func(t *testing.T) []int {
				return documentedExitCodes(t, tbl.doc, tbl.where, tbl.column, tbl.header)
			},
		})
	}

	// The skill states its contract in four files, not one document under
	// docs/; doc and sect stay empty so the Step 5 coverage sweep (which
	// walks docs/ only) never tries to match a heading against it.
	surfaces = append(surfaces, exitCodeSurface{
		name: "the scaffolded agent skill",
		full: true,
		read: func(t *testing.T) []int {
			files := skillFiles(t)
			union := map[int]bool{}
			for _, stmt := range skillExitCodeStatements() {
				for _, c := range readStatement(t, stmt, files) {
					union[c] = true
				}
			}
			out := make([]int, 0, len(union))
			for c := range union {
				out = append(out, c)
			}
			sort.Ints(out)
			return out
		},
	})

	// Per-command surfaces: correct by stating a subset, so containment only.
	surfaces = append(surfaces, exitCodeSurface{
		name: `docs/UI_SPECIFICATION.md under "2.4 Exit codes"`,
		doc:  "UI_SPECIFICATION.md",
		sect: "2.4 Exit codes",
		full: false,
		read: func(t *testing.T) []int {
			return documentedExitCodes(t, "UI_SPECIFICATION.md", "2.4 Exit codes", "Exit",
				[]string{"Exit", "Meaning"})
		},
	})
	surfaces = append(surfaces, exitCodeSurface{
		name: `docs/plugins.md under "Exit codes for"`,
		doc:  "plugins.md",
		sect: "Exit codes for",
		full: false,
		read: func(t *testing.T) []int {
			return documentedExitCodes(t, "plugins.md", "Exit codes for", "Code",
				[]string{"Code", "Meaning"})
		},
	})

	return surfaces
}

// countFull returns how many surfaces state the whole exit-code contract.
func countFull(surfaces []exitCodeSurface) int {
	n := 0
	for _, s := range surfaces {
		if s.full {
			n++
		}
	}
	return n
}

// subtestNameReplacer turns a surface's display name into a safe t.Run
// subtest name: spaces become underscores, and characters that would either
// read as t.Run's own "/" path separator or confuse -run matching are
// stripped.
var subtestNameReplacer = strings.NewReplacer(
	" ", "_",
	`"`, "",
	"`", "",
	"/", "-",
)

func subtestName(name string) string {
	return subtestNameReplacer.Replace(name)
}

// TestExitCodes_all_surfaces_agree holds every published statement of the
// complete exit-code contract to the set cmd/curlew can actually return, and
// every per-command statement to a subset of it.
//
// Equality against one source-derived reference, rather than pairwise
// agreement between surfaces: pairwise agreement is satisfied by four copies
// that drifted together, which is how three of these came to publish the same
// wrong set before M26-001. Equality to the binary is not. Agreement between
// the surfaces follows transitively.
//
// Same technique as internal/schema/parity_test.go, which holds the published
// JSON Schemas to the parser structs, for the same reason.
func TestExitCodes_all_surfaces_agree(t *testing.T) {
	reachable := reachableExitCodes(t)
	want := exitcodes.Set(reachable)
	wantSet := map[int]bool{}
	for _, v := range want {
		wantSet[v] = true
	}

	surfaces := exitCodeSurfaces()
	if countFull(surfaces) < 4 {
		t.Fatalf("only %d full-contract surfaces enumerated; the task names four "+
			"(specification, manual §4.3, manual appendix D, agent skill) — a "+
			"surface was dropped from the registry, not from the project",
			countFull(surfaces))
	}

	for _, s := range surfaces {
		t.Run(subtestName(s.name), func(t *testing.T) {
			got := s.read(t)
			if len(got) == 0 {
				t.Fatalf("%s: no exit codes read — the reader is broken, not the surface", s.name)
			}
			gotSet := map[int]bool{}
			for _, v := range got {
				gotSet[v] = true
			}

			// Direction 1, asserted for every surface: a code promised that
			// the binary cannot deliver.
			for _, v := range got {
				if !wantSet[v] {
					t.Errorf("%s documents exit %d, which cmd/curlew cannot return "+
						"(reachable: %v) — a reader told to expect it will wait forever",
						s.name, v, want)
				}
			}
			// Direction 2, full-contract surfaces only: a code the binary can
			// return that the surface never mentions. A per-command surface
			// omitting codes is correct by construction.
			if !s.full {
				return
			}
			for _, v := range want {
				if !gotSet[v] {
					prov, _ := exitcodes.Find(reachable, v)
					t.Errorf("cmd/curlew can return exit %d (%s:%d, in %s) but %s does "+
						"not document it — a pipeline hitting it sees an unexplained number",
						v, prov.File, prov.Line, prov.Fn, s.name)
				}
			}
		})
	}
}

// exitCodeProseRe matches a prose exit-code mention such as "exit code 1"
// (assertions.md, variables.md) or "code 3" (vault.md's "exits with code
// 3"). The anchor is bare "code N" rather than "exit code N": "exit code 1"
// already contains "code 1" as a substring, so one pattern catches both
// phrasings without needing to enumerate them.
//
// It deliberately does not match the `exit_code: N` NDJSON field name that
// appears in backticked code spans (exit-codes.md, failure-playbook.md,
// SKILL.md): \b never fires between two word characters, "_" is a word
// character in Go's regexp package, so "code" inside "exit_code" sits at no
// word boundary and the leading \b cannot match there.
var exitCodeProseRe = regexp.MustCompile(`(?i)\bcode (\d+)\b`)

// skillTopicFiles are every file `curlew init --skill agent` scaffolds under
// .claude/skills/curlew/ that can carry exit-code prose. Kept as its own list
// rather than a directory walk of the scaffolded tree, so a file added to the
// skill without being added here fails the file-count guard below instead of
// silently going unchecked.
var skillTopicFiles = []string{
	"SKILL.md", "assertions.md", "exit-codes.md", "expressions.md",
	"failure-playbook.md", "output-formats.md", "parallel.md", "retry.md",
	"signing.md", "variables.md", "vault.md",
}

// TestExitCodes_skillProseNamesOnlyReachableCodes sweeps every scaffolded
// skill file for a prose exit-code mention and requires it to be reachable.
//
// Containment only, by design: a topic file names the codes relevant to its
// own topic, not the whole contract, so naming a subset is correct rather
// than incomplete. M26-001 pinned the skill's four *table* statements of the
// contract; nothing pinned this prose before, and a regrown "exit code 6" in
// a topic file — assertions.md, variables.md, vault.md all make exactly this
// kind of claim — is precisely what an agent would act on literally.
func TestExitCodes_skillProseNamesOnlyReachableCodes(t *testing.T) {
	tree := scaffoldTreeWithSkill(t, "agent")
	reachable := reachableExitCodes(t)
	reachableSet := map[int]bool{}
	for _, v := range exitcodes.Set(reachable) {
		reachableSet[v] = true
	}

	const root = ".claude/skills/curlew/"
	filesWithMatch := map[string]bool{}
	for _, name := range skillTopicFiles {
		body, ok := tree[root+name]
		if !ok {
			t.Fatalf("scaffolded skill missing %s%s", root, name)
		}
		for _, m := range exitCodeProseRe.FindAllStringSubmatch(body, -1) {
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue // the pattern only captures \d+; unreachable in practice
			}
			filesWithMatch[name] = true
			if !reachableSet[n] {
				t.Errorf("%s names exit %d in prose, which cmd/curlew cannot return (reachable: %v)",
					name, n, exitcodes.Set(reachable))
			}
		}
	}

	if len(filesWithMatch) < 3 {
		t.Fatalf("prose exit-code mentions found in only %d file(s) (%v) — the regex has "+
			"stopped matching, not that the skill stopped naming exit codes in prose",
			len(filesWithMatch), filesWithMatch)
	}
}

// docsWithoutExitCodeContract are the documents whose exit-code sections are
// deliberately not held to the binary, each with its reason in code rather
// than in a commit message. A map[string]string rather than a []string: an
// entry without a reason does not compile.
var docsWithoutExitCodeContract = map[string]string{
	"SPECIFICATION.md": "platform spec — its own scope note (2026-08-04) states it " +
		"describes the src/ backend and web/ dashboard as designed through v4.4, " +
		"including the five-tier feature gating throughout, not code that exists",
	"history/IMPROVEMENT.md": "archived record of a past improvement pass",
}

// exitCodeHeadingRe matches a markdown heading naming an exit-code section:
// "## 17. Exit Codes", "### 4.3 Exit codes — master table", "## Exit codes
// for `curlew plugins list`", "## Failure playbook by exit code".
//
// Headings, not table headers: `| Code | Meaning |` also matches the CEL
// error table at MANUAL.md:1820 and the precedence table at
// CLI_SPECIFICATION.md:615, while a heading naming "exit code(s)" does not
// match either of those.
var exitCodeHeadingRe = regexp.MustCompile(`(?i)^#{1,6}\s+.*exit codes?\b`)

// exitCodeHeading is one markdown heading found while sweeping docs/ that
// names an exit-code section.
type exitCodeHeading struct {
	doc  string // path relative to docs.Dir, forward-slashed, e.g. "history/IMPROVEMENT.md"
	line int
	text string
}

// sweepExitCodeHeadings walks docs.Dir for every markdown heading naming an
// exit-code section.
//
// Rooted at docs.Dir specifically, never a repo-wide glob:
// .claude/worktrees/*/ is a second full checkout of this repository, and a
// glob would contribute a phantom copy of every document under it.
func sweepExitCodeHeadings(t *testing.T) []exitCodeHeading {
	t.Helper()
	var out []exitCodeHeading
	err := filepath.WalkDir(docs.Dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".md") {
			return nil
		}
		rel, relErr := filepath.Rel(docs.Dir, path)
		if relErr != nil {
			return relErr
		}
		rel = filepath.ToSlash(rel)
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		for i, line := range strings.Split(string(data), "\n") {
			if exitCodeHeadingRe.MatchString(line) {
				out = append(out, exitCodeHeading{doc: rel, line: i + 1, text: strings.TrimSpace(line)})
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("sweeping %s for exit-code headings: %v", docs.Dir, err)
	}
	return out
}

// TestExitCodes_everyExitCodeSectionIsRegistered walks docs/ for exit-code
// section headings and fails on any the surface registry does not classify.
//
// A hand-maintained list of surfaces is another document about the binary
// and drifts like the ones it guards: when this task was written, the
// four-surface list named in the task did not include
// docs/UI_SPECIFICATION.md §2.4 — publishing an exit 9 for a licensing grace
// check deleted long ago. This is the analogue of
// internal/schema/parity_test.go's
// TestSchema_parity_table_covers_every_parser_struct, which reflects over
// every struct reachable from parser.Collection and fails if one is in
// neither the parity table nor its exemption list.
func TestExitCodes_everyExitCodeSectionIsRegistered(t *testing.T) {
	headings := sweepExitCodeHeadings(t)
	if len(headings) < 5 {
		t.Fatalf("found %d exit-code section heading(s) under %s — the sweep is broken, "+
			"not that the documentation shrank (measured 7 on this tree)", len(headings), docs.Dir)
	}

	surfaces := exitCodeSurfaces()
	for _, h := range headings {
		if _, excluded := docsWithoutExitCodeContract[h.doc]; excluded {
			continue
		}
		covered := false
		for _, s := range surfaces {
			if s.doc == h.doc && s.sect != "" && strings.Contains(h.text, s.sect) {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("docs/%s:%d %q publishes exit codes but no surface in exitCodeSurfaces() "+
				"reads it, and it is not in docsWithoutExitCodeContract — either hold it to the "+
				"binary or record why not", h.doc, h.line, h.text)
		}
	}
}
