package docs_test

// This file provides two layers of coverage for M28-002.
//
// The first layer (TestGateModes, TestGateInvocations, TestAuditGate,
// TestChecklist, TestModuleOwnerRepo, TestAuditFrontDoor) is hermetic: every
// case is a synthetic `src string` fed straight to a pure parser or audit
// function, following the internal/docs/layout.go precedent (LayoutRows,
// AuditLayout) of taking text rather than a filename so behaviour can be
// proven by mutation without touching the filesystem.
//
// The second layer (added once the first is green) reads the repository's
// own front-door files and holds them to what the first layer's functions
// compute from the repository's own source of truth (scripts/ci-local.sh,
// CLAUDE.md, go.mod) — the same "derive both sides from the artefacts"
// contract internal/schema/parity_test.go already carries.

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

func TestGateModes(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want [][]string // each entry is one mode's Aliases
		err  error
	}{
		{
			name: "case statement with one arm yields one mode",
			src: `case "$MODE" in
  auto)
    echo hi
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "an arm with aliases yields both, first is canonical",
			src: `case "$MODE" in
  --full|full)
    run_backend=1
    ;;
esac
`,
			want: [][]string{{"--full", "full"}},
		},
		{
			name: "the bare auto arm is the default mode",
			src: `case "$MODE" in
  auto)
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "the help arm is not a gate mode",
			src: `case "$MODE" in
  auto)
    ;;
  -h|--help)
    exit 0
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "the default arm is not a gate mode",
			src: `case "$MODE" in
  auto)
    ;;
  *)
    exit 2
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "arms outside the MODE case statement are ignored",
			src: `case "$OTHER" in
  weird)
    ;;
esac
case "$MODE" in
  auto)
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "a case arm inside a heredoc is not a mode",
			src: `case "$MODE" in
  auto)
    cat <<EOF
sneaky)
EOF
    ;;
esac
`,
			want: [][]string{{"auto"}},
		},
		{
			name: "MUTATION no case statement returns ErrNoGateModes",
			src:  "echo hello\n",
			err:  docs.ErrNoGateModes,
		},
		{
			name: "MUTATION only * and -h arms returns ErrNoGateModes",
			src: `case "$MODE" in
  *)
    ;;
  -h|--help)
    ;;
esac
`,
			err: docs.ErrNoGateModes,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docs.GateModes(tt.src)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("GateModes() err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GateModes() unexpected error: %v", err)
			}
			if len(got) != len(tt.want) {
				t.Fatalf("GateModes() = %#v, want aliases %#v", got, tt.want)
			}
			for i := range got {
				if !reflect.DeepEqual(got[i].Aliases, tt.want[i]) {
					t.Errorf("mode %d Aliases = %v, want %v", i, got[i].Aliases, tt.want[i])
				}
			}
		})
	}
}

func TestGateInvocations(t *testing.T) {
	tests := []struct {
		name string
		src  string
		want []string
		err  error
	}{
		{
			name: "a bare ./scripts/ci-local.sh documents the auto mode",
			src:  "Run `./scripts/ci-local.sh` to gate.",
			want: []string{docs.GateDefaultMode},
		},
		{
			name: "an invocation with a flag yields that flag",
			src:  "./scripts/ci-local.sh --go",
			want: []string{"--go"},
		},
		{
			name: "an invocation inside prose backticks counts",
			src:  "Run `./scripts/ci-local.sh --full` to force everything.",
			want: []string{"--full"},
		},
		{
			name: "a path-less mention of ci-local.sh is not an invocation",
			src:  "See ci-local.sh for details.",
			err:  docs.ErrNoGateInvocations,
		},
		{
			name: "MUTATION a document naming no invocation returns ErrNoGateInvocations",
			src:  "This document mentions no gate at all.",
			err:  docs.ErrNoGateInvocations,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docs.GateInvocations(tt.src)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("GateInvocations() err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("GateInvocations() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GateInvocations() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestAuditGate(t *testing.T) {
	mode := func(aliases ...string) docs.GateMode { return docs.GateMode{Aliases: aliases} }

	tests := []struct {
		name    string
		modes   []docs.GateMode
		invoked []string
		want    docs.GateAudit
	}{
		{
			name:    "clean: every mode documented, no unknown modes",
			modes:   []docs.GateMode{mode("auto"), mode("--go", "go")},
			invoked: []string{"auto", "--go"},
			want:    docs.GateAudit{},
		},
		{
			name:    "MUTATION undocumented: ci-local.sh grows a mode CONTRIBUTING never names",
			modes:   []docs.GateMode{mode("auto"), mode("--go", "go")},
			invoked: []string{"auto"},
			want:    docs.GateAudit{Undocumented: []string{"--go"}},
		},
		{
			name:    "MUTATION unknown: CONTRIBUTING names a mode ci-local.sh would reject",
			modes:   []docs.GateMode{mode("auto")},
			invoked: []string{"auto", "--verbose"},
			want:    docs.GateAudit{Unknown: []string{"--verbose"}},
		},
		{
			name:    "an alias satisfies its canonical mode",
			modes:   []docs.GateMode{mode("--full", "full")},
			invoked: []string{"full"},
			want:    docs.GateAudit{},
		},
		{
			name:    "no invocations at all leaves every mode undocumented",
			modes:   []docs.GateMode{mode("auto"), mode("--go", "go")},
			invoked: nil,
			want:    docs.GateAudit{Undocumented: []string{"--go", "auto"}},
		},
		{
			name:    "no modes at all makes every invocation unknown",
			modes:   nil,
			invoked: []string{"auto", "--go"},
			want:    docs.GateAudit{Unknown: []string{"--go", "auto"}},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docs.AuditGate(tt.modes, tt.invoked)
			if !reflect.DeepEqual(got.Undocumented, tt.want.Undocumented) {
				t.Errorf("Undocumented = %v, want %v", got.Undocumented, tt.want.Undocumented)
			}
			if !reflect.DeepEqual(got.Unknown, tt.want.Unknown) {
				t.Errorf("Unknown = %v, want %v", got.Unknown, tt.want.Unknown)
			}
			if got.Clean() != tt.want.Clean() {
				t.Errorf("Clean() = %v, want %v", got.Clean(), tt.want.Clean())
			}
		})
	}
}

// TestGateMode_String proves the file:line + spelling format String()
// renders, following the same convention as layout.go's Row and prose.go's
// ProseRef -- the line a mode's case arm was read from is otherwise never
// surfaced anywhere.
func TestGateMode_String(t *testing.T) {
	tests := []struct {
		name string
		mode docs.GateMode
		want string
	}{
		{
			name: "single alias",
			mode: docs.GateMode{Line: 12, Aliases: []string{"auto"}},
			want: docs.GateScript + ":12 auto",
		},
		{
			name: "aliased mode joins with a pipe, canonical first",
			mode: docs.GateMode{Line: 40, Aliases: []string{"--go", "go"}},
			want: docs.GateScript + ":40 --go|go",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.mode.String(); got != tt.want {
				t.Errorf("String() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestChecklist(t *testing.T) {
	tests := []struct {
		name    string
		src     string
		heading string
		want    []string
		err     error
	}{
		{
			name: "items under the named heading are returned in order",
			src: `Before any commit:
- [ ] On feature branch (not main)
- [ ] go build succeeds
`,
			heading: "Before any commit:",
			want:    []string{"On feature branch (not main)", "go build succeeds"},
		},
		{
			name: "a checked box counts the same as an unchecked one",
			src: `Before any commit:
- [x] On feature branch (not main)
- [ ] go build succeeds
`,
			heading: "Before any commit:",
			want:    []string{"On feature branch (not main)", "go build succeeds"},
		},
		{
			name: "items under a later heading are not returned",
			src: `Before any commit:
- [ ] On feature branch (not main)

Before task completion:
- [ ] Coverage >= 80%
`,
			heading: "Before any commit:",
			want:    []string{"On feature branch (not main)"},
		},
		{
			name:    "a checklist inside a fenced block is not a checklist",
			src:     "```\nBefore any commit:\n- [ ] On feature branch (not main)\n```\n",
			heading: "Before any commit:",
			err:     docs.ErrNoChecklist,
		},
		{
			name: "MUTATION a heading with no items returns ErrNoChecklist",
			src: `Before any commit:

Nothing here.
`,
			heading: "Before any commit:",
			err:     docs.ErrNoChecklist,
		},
		{
			name:    "MUTATION a missing heading returns ErrNoChecklist",
			src:     "Some other text entirely.\n",
			heading: "Before any commit:",
			err:     docs.ErrNoChecklist,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docs.Checklist(tt.src, tt.heading)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("Checklist() err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("Checklist() unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Errorf("Checklist() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestModuleOwnerRepo(t *testing.T) {
	tests := []struct {
		name  string
		gomod string
		want  string
		err   error
	}{
		{
			name:  "a github module path yields owner/repo",
			gomod: "module github.com/weiqigod/curlew\n\ngo 1.24\n",
			want:  "weiqigod/curlew",
		},
		{
			name:  "MUTATION a go.mod with no module directive returns ErrNoModulePath",
			gomod: "go 1.24\n",
			err:   docs.ErrNoModulePath,
		},
		{
			name:  "MUTATION a non-github module path returns ErrNoModulePath",
			gomod: "module example.com/weiqigod/curlew\n\ngo 1.24\n",
			err:   docs.ErrNoModulePath,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := docs.ModuleOwnerRepo(tt.gomod)
			if tt.err != nil {
				if !errors.Is(err, tt.err) {
					t.Fatalf("ModuleOwnerRepo() err = %v, want %v", err, tt.err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ModuleOwnerRepo() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Errorf("ModuleOwnerRepo() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestAuditFrontDoor(t *testing.T) {
	files := []docs.FrontDoorFile{
		{Path: "CONTRIBUTING.md", Requires: []string{"TDD"}, Why: "a contributor needs the workflow"},
		{Path: "SECURITY.md", Requires: []string{"advisory"}, Why: "a reporter needs a private route"},
	}

	tests := []struct {
		name     string
		files    []docs.FrontDoorFile
		contents map[string]string
		want     docs.FrontDoorAudit
	}{
		{
			name:  "clean: every file present and stating what it must",
			files: files,
			contents: map[string]string{
				"CONTRIBUTING.md": "Follow TDD strictly.",
				"SECURITY.md":     "Use the private advisory route.",
			},
			want: docs.FrontDoorAudit{},
		},
		{
			name:  "MUTATION missing: a front-door file that does not exist",
			files: files,
			contents: map[string]string{
				"SECURITY.md": "Use the private advisory route.",
			},
			want: docs.FrontDoorAudit{Missing: []docs.FrontDoorFile{files[0]}},
		},
		{
			name:  "MUTATION silent: a file that exists but omits a required statement",
			files: files,
			contents: map[string]string{
				"CONTRIBUTING.md": "Nothing about the discipline here.",
				"SECURITY.md":     "Use the private advisory route.",
			},
			want: docs.FrontDoorAudit{Silent: map[string][]string{"CONTRIBUTING.md": {"TDD"}}},
		},
		{
			name:  "MUTATION empty: a file that exists but is whitespace",
			files: files,
			contents: map[string]string{
				"CONTRIBUTING.md": "   \n\t\n",
				"SECURITY.md":     "Use the private advisory route.",
			},
			want: docs.FrontDoorAudit{Empty: []docs.FrontDoorFile{files[0]}},
		},
		{
			name:     "no files at all reports every entry missing",
			files:    files,
			contents: map[string]string{},
			want:     docs.FrontDoorAudit{Missing: files},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := docs.AuditFrontDoor(tt.files, tt.contents)
			if !reflect.DeepEqual(got.Missing, tt.want.Missing) {
				t.Errorf("Missing = %#v, want %#v", got.Missing, tt.want.Missing)
			}
			if !reflect.DeepEqual(got.Empty, tt.want.Empty) {
				t.Errorf("Empty = %#v, want %#v", got.Empty, tt.want.Empty)
			}
			if !reflect.DeepEqual(got.Silent, tt.want.Silent) {
				t.Errorf("Silent = %#v, want %#v", got.Silent, tt.want.Silent)
			}
			if got.Clean() != tt.want.Clean() {
				t.Errorf("Clean() = %v, want %v", got.Clean(), tt.want.Clean())
			}
		})
	}
}

// TestReadFrontDoor_emptyRegistry is the producer TestRepo_front_door_files
// _are_present's own floor check documents but does not itself return:
// ErrNoFrontDoorFiles exists to report a registry emptied out from under a
// caller, and this proves ReadFrontDoor actually returns it in that case
// rather than the sentinel being dead weight only referenced by its own
// registration in hints_init.go.
func TestReadFrontDoor_emptyRegistry(t *testing.T) {
	tests := []struct {
		name  string
		files []docs.FrontDoorFile
	}{
		{name: "nil registry", files: nil},
		{name: "MUTATION empty (non-nil) registry", files: []docs.FrontDoorFile{}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := docs.ReadFrontDoor(context.Background(), t.TempDir(), tt.files)
			if !errors.Is(err, docs.ErrNoFrontDoorFiles) {
				t.Fatalf("ReadFrontDoor() err = %v, want %v", err, docs.ErrNoFrontDoorFiles)
			}
		})
	}
}

// The tests below are the second layer: they read the repository's own
// front-door files and hold them to what the first layer's functions compute
// from the repository's own source of truth. Filenames are declared as
// consts rather than inline literals so docs.Claims (internal/docs/claims.go)
// -- which treats a "*.md" string literal followed by further string
// literals in a _test.go file as a documentation-table claim -- does not
// mistake a plain file-read for a claim on a table this file does not read.
// internal/docs/layout_test.go:553 established the pattern (const readmeFile).
const (
	contributingFile = "CONTRIBUTING.md"
	securityFile     = "SECURITY.md"
	claudeMDFile     = "CLAUDE.md"
	goModFile        = "go.mod"
	prTemplateFile   = ".github/pull_request_template.md"
)

// readRepoFileOrFatal reads a repository-root-relative path or fails the
// test, mirroring cmd/curlew/readme_install_test.go's
// readmeReadFileOrFatal for the same reason: every one of these tests wants
// the same "fail loudly, not silently" behaviour on a missing file.
func readRepoFileOrFatal(t *testing.T, relPath string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(docs.Root, relPath))
	if err != nil {
		t.Fatalf("read %s: %v", relPath, err)
	}
	return string(data)
}

// TestRepo_front_door_files_are_present is the observable: the community
// files exist and point at real things. A registry emptied out from under
// this test would otherwise let it pass vacuously, so the floor below is
// independent of AuditFrontDoor finding anything to report.
func TestRepo_front_door_files_are_present(t *testing.T) {
	if len(docs.FrontDoor) < 6 {
		t.Fatalf("docs.FrontDoor has %d entries; want at least 6 -- the registry was emptied out from under this test", len(docs.FrontDoor))
	}

	contents, err := docs.ReadFrontDoor(context.Background(), docs.Root, docs.FrontDoor)
	if err != nil {
		t.Fatalf("ReadFrontDoor: %v", err)
	}

	audit := docs.AuditFrontDoor(docs.FrontDoor, contents)
	if audit.Clean() {
		return
	}
	for _, f := range audit.Missing {
		t.Errorf("missing %s: %s", f.Path, f.Why)
	}
	for _, f := range audit.Empty {
		t.Errorf("%s exists but is empty: %s", f.Path, f.Why)
	}
	for path, missing := range audit.Silent {
		t.Errorf("%s exists but does not state: %v (%s)", path, missing, path)
	}
}

// TestContributing_names_the_gate_that_ci_local_runs holds CONTRIBUTING.md to
// scripts/ci-local.sh in both directions (D4a), and checks docs.GateTools
// appears in both files (D4b) so the document and the script cannot drift
// apart without this failing for one of them.
func TestContributing_names_the_gate_that_ci_local_runs(t *testing.T) {
	gate := readRepoFileOrFatal(t, docs.GateScript)
	modes, err := docs.GateModes(gate)
	if err != nil {
		t.Fatalf("GateModes(%s): %v", docs.GateScript, err)
	}

	contributing := readRepoFileOrFatal(t, contributingFile)
	invoked, err := docs.GateInvocations(contributing)
	if err != nil {
		t.Fatalf("GateInvocations(%s): %v", contributingFile, err)
	}

	audit := docs.AuditGate(modes, invoked)
	if !audit.Clean() {
		if len(audit.Undocumented) > 0 {
			t.Errorf("%s accepts mode(s) %v that %s never names", docs.GateScript, audit.Undocumented, contributingFile)
		}
		if len(audit.Unknown) > 0 {
			t.Errorf("%s names mode(s) %v that %s would reject", contributingFile, audit.Unknown, docs.GateScript)
		}
	}

	for _, tool := range docs.GateTools {
		if !strings.Contains(gate, tool) {
			t.Errorf("%s no longer runs %q -- update docs.GateTools", docs.GateScript, tool)
		}
		if !strings.Contains(contributing, tool) {
			t.Errorf("%s does not mention %q, one of the tools the gate runs", contributingFile, tool)
		}
	}
}

// TestSecurity_names_a_reporting_route_that_exists holds SECURITY.md to the
// private GitHub advisory route derived from go.mod (F9), and to the exact
// public-issue rule, and forbids the noreply address that accepts no mail.
func TestSecurity_names_a_reporting_route_that_exists(t *testing.T) {
	ownerRepo, err := docs.ModuleOwnerRepo(readRepoFileOrFatal(t, goModFile))
	if err != nil {
		t.Fatalf("ModuleOwnerRepo(%s): %v", goModFile, err)
	}

	security := readRepoFileOrFatal(t, securityFile)

	wantURL := "https://github.com/" + ownerRepo + docs.SecurityAdvisoryPath
	if !strings.Contains(security, wantURL) {
		t.Errorf("%s does not name the private advisory route %q", securityFile, wantURL)
	}
	if !strings.Contains(security, docs.SecurityPublicIssueRule) {
		t.Errorf("%s does not state the public-issue rule verbatim:\n%s", securityFile, docs.SecurityPublicIssueRule)
	}

	const noreply = "@users.noreply.github.com"
	if strings.Contains(security, noreply) {
		t.Errorf("%s names a noreply address (%s), which accepts no mail", securityFile, noreply)
	}
}

// TestPullRequestTemplate_carries_the_quality_gate_checklist checks the
// forward direction only (the template may add PR-specific lines beyond
// CLAUDE.md's checklist): every item CLAUDE.md states under each of
// docs.QualityGateHeadings must appear in the PR template.
func TestPullRequestTemplate_carries_the_quality_gate_checklist(t *testing.T) {
	claude := readRepoFileOrFatal(t, claudeMDFile)
	template := readRepoFileOrFatal(t, prTemplateFile)

	total := 0
	for _, h := range docs.QualityGateHeadings {
		items, err := docs.Checklist(claude, h)
		if err != nil {
			t.Fatalf("Checklist(%s, %q): %v", claudeMDFile, h, err)
		}
		total += len(items)
		for _, item := range items {
			if !strings.Contains(template, item) {
				t.Errorf("%s is missing checklist item %q from %s heading %q", prTemplateFile, item, claudeMDFile, h)
			}
		}
	}
	if total == 0 {
		t.Fatalf("parsed zero checklist items from %s -- the parse is broken, not the template", claudeMDFile)
	}
}
