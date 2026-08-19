package docs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

// GateScript is the one gate, relative to Root. scripts/ci-local.sh is not a
// fast pre-check ahead of CI; nothing runs in CI, so it is the whole gate.
const GateScript = "scripts/ci-local.sh"

// GateDefaultMode is the mode a bare `./scripts/ci-local.sh` selects.
const GateDefaultMode = "auto"

// GateTools are the tools the gate runs directly and CONTRIBUTING.md names by
// hand. Stated once here so the document and the script cannot drift apart
// without this list failing for one of them.
var GateTools = []string{"go build", "go test", "go test -race", "golangci-lint", "./smoke/run.sh"}

// SecurityAdvisoryPath is GitHub's private reporting route, relative to a
// repository's https://github.com/<owner>/<repo> base.
const SecurityAdvisoryPath = "/security/advisories/new"

// SecurityPublicIssueRule is the sentence SECURITY.md must state verbatim,
// recorded once here for the reason PlatformStatus is.
const SecurityPublicIssueRule = "Do not report a vulnerability in a public issue, " +
	"a pull request, or a discussion: use the private advisory route above."

// QualityGateHeadings are the CLAUDE.md sections whose checklist items the
// pull-request template must carry.
var QualityGateHeadings = []string{"Before any commit:", "Before task completion (`/verify`):"}

// IssueFields are the three things every diagnosis starts from. An issue
// template that does not ask for all three generates a second round trip.
var IssueFields = []string{"version", "collection", "exit-code"}

var (
	// ErrNoGateModes reports that a gate script declares no mode at all -- an
	// empty result here means the parse broke, not that the gate has no modes.
	ErrNoGateModes = errors.New("no gate modes")
	// ErrNoGateInvocations reports that a document names no gate invocation.
	ErrNoGateInvocations = errors.New("no gate invocations")
	// ErrNoChecklist reports that a heading carries no checklist items.
	ErrNoChecklist = errors.New("no checklist items")
	// ErrNoModulePath reports that go.mod declares no github.com module path.
	ErrNoModulePath = errors.New("no module path")
	// ErrNoFrontDoorFiles reports that the front-door registry is empty.
	ErrNoFrontDoorFiles = errors.New("no front-door files")
)

// GateMode is one arm of scripts/ci-local.sh's mode dispatch. Aliases[0] is
// canonical; the rest are the undashed spellings the arm also accepts.
type GateMode struct {
	Line    int
	Aliases []string
}

// Canonical is the mode's primary spelling: Aliases[0].
func (m GateMode) Canonical() string {
	if len(m.Aliases) == 0 {
		return ""
	}
	return m.Aliases[0]
}

// Accepts reports whether word is any of the mode's aliases.
func (m GateMode) Accepts(word string) bool {
	for _, a := range m.Aliases {
		if a == word {
			return true
		}
	}
	return false
}

// String renders a mode as scripts/ci-local.sh:LINE and the way its case arm
// spells it, following the file:line convention every other line-carrying
// type in this package uses (layout.go's Row, prose.go's ProseRef).
func (m GateMode) String() string {
	return fmt.Sprintf("%s:%d %s", GateScript, m.Line, strings.Join(m.Aliases, "|"))
}

// gateCaseHeader is the exact line scripts/ci-local.sh opens its mode
// dispatch with. Matched literally rather than by a looser "case ... MODE"
// pattern, so a case statement over some other variable is never mistaken
// for the gate's own dispatch.
const gateCaseHeader = `case "$MODE" in`

// gateArmRE matches one case-arm header, trimmed: a pipe-separated list of
// bare words (the spellings bash patterns take here -- no globs beyond the
// bare "*" default arm) ending in ")".
var gateArmRE = regexp.MustCompile(`^([A-Za-z0-9_.*+/-]+(?:\|[A-Za-z0-9_.*+/-]+)*)\)$`)

// GateModes returns the modes a gate script accepts, read out of the arms of
// its `case "$MODE" in` statement rather than out of its usage comment: the
// comment is prose about the script, the case statement is the script.
//
// The help arms and the default arm are not modes and are excluded; that
// exclusion is named, not implicit, so it cannot quietly grow.
func GateModes(src string) ([]GateMode, error) {
	lines := strings.Split(src, "\n")

	entry := -1
	for i, line := range lines {
		if strings.TrimSpace(line) == gateCaseHeader {
			entry = i
			break
		}
	}
	if entry == -1 {
		return nil, ErrNoGateModes
	}

	var modes []GateMode
	for i := entry + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "esac" {
			break
		}

		if delim, ok := heredocDelimiter(lines[i]); ok {
			i++
			for i < len(lines) && strings.TrimSpace(lines[i]) != delim {
				i++
			}
			continue
		}

		m := gateArmRE.FindStringSubmatch(trimmed)
		if m == nil {
			continue
		}
		aliases := strings.Split(m[1], "|")
		if isGateHelpArm(aliases) || isGateDefaultArm(aliases) {
			continue
		}
		modes = append(modes, GateMode{Line: i + 1, Aliases: aliases})
	}

	if len(modes) == 0 {
		return nil, ErrNoGateModes
	}
	return modes, nil
}

// isGateHelpArm reports whether an arm's aliases are exactly the help arm's.
func isGateHelpArm(aliases []string) bool {
	return len(aliases) == 2 &&
		((aliases[0] == "-h" && aliases[1] == "--help") || (aliases[0] == "--help" && aliases[1] == "-h"))
}

// isGateDefaultArm reports whether an arm is the bare `*)` fallthrough.
func isGateDefaultArm(aliases []string) bool {
	return len(aliases) == 1 && aliases[0] == "*"
}

// heredocDelimiter reports the delimiter word of a heredoc `<<[-]WORD` (or
// quoted `<<'WORD'`/`<<"WORD"`) starting on line, and whether one was found.
// A here-string `<<<...` is deliberately rejected: it is a single-line
// redirection, not a multi-line block to skip.
func heredocDelimiter(line string) (string, bool) {
	idx := strings.Index(line, "<<")
	if idx < 0 {
		return "", false
	}
	if idx > 0 && line[idx-1] == '<' {
		return "", false // part of a longer run of '<'
	}
	if idx+2 < len(line) && line[idx+2] == '<' {
		return "", false // '<<<', a here-string
	}

	rest := strings.TrimLeft(line[idx+2:], "-")
	rest = strings.TrimLeft(rest, " \t")
	if rest == "" {
		return "", false
	}
	if rest[0] == '\'' || rest[0] == '"' {
		rest = rest[1:]
	}

	j := 0
	for j < len(rest) && isWordChar(rest[j]) {
		j++
	}
	if j == 0 {
		return "", false
	}
	return rest[:j], true
}

func isWordChar(b byte) bool {
	return b == '_' || (b >= 'a' && b <= 'z') || (b >= 'A' && b <= 'Z') || (b >= '0' && b <= '9')
}

// gateInvocationMarker is the literal text an invocation names. A mention of
// bare "ci-local.sh" with no path is not an invocation: the gate is always
// run relative to the repository root, and a path-less mention is prose
// about the script, not a runnable command.
const gateInvocationMarker = "./scripts/ci-local.sh"

// GateInvocations returns every `./scripts/ci-local.sh <mode>` a document
// names, as the mode word -- GateDefaultMode for a bare invocation.
func GateInvocations(src string) ([]string, error) {
	var invocations []string

	idx := 0
	for {
		pos := strings.Index(src[idx:], gateInvocationMarker)
		if pos < 0 {
			break
		}
		abs := idx + pos
		rest := src[abs+len(gateInvocationMarker):]

		mode := GateDefaultMode
		i := 0
		for i < len(rest) && (rest[i] == ' ' || rest[i] == '\t') {
			i++
		}
		if i > 0 {
			j := i
			for j < len(rest) && (isWordChar(rest[j]) || rest[j] == '-') {
				j++
			}
			if j > i {
				mode = rest[i:j]
			}
		}
		invocations = append(invocations, mode)
		idx = abs + len(gateInvocationMarker)
	}

	if len(invocations) == 0 {
		return nil, ErrNoGateInvocations
	}
	return invocations, nil
}

// GateAudit is the outcome of holding a contributing document to the gate it
// describes. It fails in both directions, like LayoutAudit.
type GateAudit struct {
	Undocumented []string // a mode the gate accepts that the document never names
	Unknown      []string // a mode the document names that the gate would reject
}

// Clean reports whether the audit found nothing.
func (a GateAudit) Clean() bool {
	return len(a.Undocumented) == 0 && len(a.Unknown) == 0
}

// AuditGate holds a set of documented invocations to a set of gate modes. It
// takes two slices rather than reading files so both directions can be proven
// by mutation.
func AuditGate(modes []GateMode, invoked []string) GateAudit {
	var audit GateAudit

	matched := map[string]bool{}
	for _, m := range modes {
		found := false
		for _, w := range invoked {
			if m.Accepts(w) {
				found = true
				matched[w] = true
			}
		}
		if !found {
			audit.Undocumented = append(audit.Undocumented, m.Canonical())
		}
	}

	unknownSet := map[string]bool{}
	for _, w := range invoked {
		if !matched[w] {
			unknownSet[w] = true
		}
	}
	for w := range unknownSet {
		audit.Unknown = append(audit.Unknown, w)
	}

	sort.Strings(audit.Undocumented)
	sort.Strings(audit.Unknown)
	return audit
}

// checklistItemRE matches one `- [ ]`/`- [x]` list item, trimmed.
var checklistItemRE = regexp.MustCompile(`^- \[[ xX]\] (.+)$`)

// Checklist returns the `- [ ]` items stated under heading, in order. Items
// inside fenced code blocks are pictures of a checklist, not one.
func Checklist(src, heading string) ([]string, error) {
	lines := strings.Split(src, "\n")

	headingLine := -1
	inFence := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "```") {
			inFence = !inFence
			continue
		}
		if inFence {
			continue
		}
		if trimmed == heading {
			headingLine = i
			break
		}
	}
	if headingLine == -1 {
		return nil, fmt.Errorf("%q: %w", heading, ErrNoChecklist)
	}

	var items []string
	for i := headingLine + 1; i < len(lines); i++ {
		trimmed := strings.TrimSpace(lines[i])
		if trimmed == "" {
			if items == nil {
				continue // a blank line before the list has even started
			}
			break
		}
		m := checklistItemRE.FindStringSubmatch(trimmed)
		if m == nil {
			break
		}
		items = append(items, m[1])
	}

	if len(items) == 0 {
		return nil, fmt.Errorf("%q: %w", heading, ErrNoChecklist)
	}
	return items, nil
}

// gomodModuleRE matches go.mod's `module <path>` directive.
var gomodModuleRE = regexp.MustCompile(`(?m)^module\s+(\S+)\s*$`)

// ModuleOwnerRepo returns "<owner>/<repo>" from a go.mod's module directive,
// so every URL derived from it fails loudly rather than trusting a
// hand-typed org name -- the drift M21-001 found in the published schemas'
// $id.
func ModuleOwnerRepo(gomod string) (string, error) {
	m := gomodModuleRE.FindStringSubmatch(gomod)
	if m == nil {
		return "", ErrNoModulePath
	}
	const prefix = "github.com/"
	path := m[1]
	if !strings.HasPrefix(path, prefix) {
		return "", fmt.Errorf("%q: %w", path, ErrNoModulePath)
	}
	parts := strings.SplitN(strings.TrimPrefix(path, prefix), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", fmt.Errorf("%q: %w", path, ErrNoModulePath)
	}
	return parts[0] + "/" + parts[1], nil
}

// FrontDoorFile is one file a stranger looks for, and what it must state to
// be worth having. A file that exists and says nothing is the failure this
// registry is aimed at.
type FrontDoorFile struct {
	Path     string   // repo-relative
	Requires []string // substrings the file must contain
	Why      string   // printed on failure: what a reader loses without it
}

// FrontDoor is the registry. CLAUDE.md is addressed to an AI assistant; these
// are the files addressed to a person.
var FrontDoor = []FrontDoorFile{
	{
		Path:     "CONTRIBUTING.md",
		Requires: []string{"RED", "GREEN", "REFACTOR", "Never commit to main", "Nothing runs in CI"},
		Why:      "a contributor has no way to learn the branch rule, the TDD rule, or that ci-local.sh is the whole gate",
	},
	{
		Path:     "SECURITY.md",
		Requires: []string{SecurityPublicIssueRule, SecurityAdvisoryPath},
		Why:      "a reporter with a real vulnerability has no private route and may file it as a public issue",
	},
	{
		Path:     filepath.Join(".github", "ISSUE_TEMPLATE", "bug_report.yml"),
		Requires: IssueFields,
		Why:      "a bug report arrives missing the version, collection, or exit code every diagnosis starts from",
	},
	{
		Path:     filepath.Join(".github", "ISSUE_TEMPLATE", "feature_request.yml"),
		Requires: []string{"Problem", "Proposed solution"},
		Why:      "a feature request arrives as free text with no problem statement to evaluate",
	},
	{
		Path:     filepath.Join(".github", "ISSUE_TEMPLATE", "config.yml"),
		Requires: []string{"blank_issues_enabled: false", "security/advisories"},
		Why:      "blank issues bypass the templates above, and a vulnerability reporter finds no link to the private route",
	},
	{
		Path:     filepath.Join(".github", "pull_request_template.md"),
		Requires: []string{"Quality Gate", GateScript},
		Why:      "the quality-gate checklist from CLAUDE.md is not in front of the author when they open the PR",
	},
}

// FrontDoorAudit is the outcome of holding a repository to FrontDoor.
type FrontDoorAudit struct {
	Missing []FrontDoorFile     // no such file
	Empty   []FrontDoorFile     // present but whitespace
	Silent  map[string][]string // path -> the required statements it omits
}

// Clean reports whether the audit found nothing.
func (a FrontDoorAudit) Clean() bool {
	return len(a.Missing) == 0 && len(a.Empty) == 0 && len(a.Silent) == 0
}

// AuditFrontDoor holds a set of file contents, keyed by repo-relative path,
// to a registry. A path absent from contents is Missing.
func AuditFrontDoor(files []FrontDoorFile, contents map[string]string) FrontDoorAudit {
	var audit FrontDoorAudit

	for _, f := range files {
		content, ok := contents[f.Path]
		if !ok {
			audit.Missing = append(audit.Missing, f)
			continue
		}
		if strings.TrimSpace(content) == "" {
			audit.Empty = append(audit.Empty, f)
			continue
		}
		var missing []string
		for _, req := range f.Requires {
			if !strings.Contains(content, req) {
				missing = append(missing, req)
			}
		}
		if len(missing) > 0 {
			if audit.Silent == nil {
				audit.Silent = map[string][]string{}
			}
			audit.Silent[f.Path] = missing
		}
	}

	return audit
}

// ReadFrontDoor reads every registered front-door file under root. A file
// that does not exist yields no entry rather than an error, so
// AuditFrontDoor can report it as Missing with its Why attached.
func ReadFrontDoor(ctx context.Context, root string, files []FrontDoorFile) (map[string]string, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, ErrNoFrontDoorFiles
	}

	out := make(map[string]string, len(files))
	for _, f := range files {
		data, err := os.ReadFile(filepath.Join(root, f.Path))
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			return nil, fmt.Errorf("reading %s: %w", f.Path, err)
		}
		out[f.Path] = string(data)
	}
	return out, nil
}
