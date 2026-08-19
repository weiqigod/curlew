package main

// .github/ISSUE_TEMPLATE/bug_report.yml's exit-code dropdown is the front-
// door surface M28-002's plan committed to holding open: a bug report is not
// actionable without knowing which of curlew's own exit codes the reporter
// hit, and a dropdown built by hand-typing the current code list, like every
// other exit-code surface in this package before M26-001, drifts the same
// way -- a future exit code added to (or removed from) cmd/curlew leaves the
// dropdown silently stale with nothing to catch it.
//
// Containment, not equality: the dropdown must offer every code cmd/curlew
// can actually return, but it may also offer "other / not sure" as an escape
// valve so a reporter hitting an unlisted code is never blocked from filing
// at all. Requiring equality would forbid that valve. This mirrors the sense
// of exit_codes_parity_test.go's exitCodeSurface{full: false} precedent
// (documenting a legitimate, not-the-whole-set surface is correct by
// construction) while checking the direction that precedent's containment
// entries do not: every reachable code must appear, not merely every
// documented one must be reachable.
//
// Deliberately its own small test rather than a new exitCodeSurfaces() entry:
// that registry's "full" surfaces are asserted for two-way equality against
// the reachable set (TestExitCodes_all_surfaces_agree), which is a stronger
// claim than this dropdown makes for itself by design (the escape valve).

import (
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/exitcodes"
	"gopkg.in/yaml.v3"
)

// bugReportIssueForm is the minimal shape read out of bug_report.yml: enough
// to find the "exit-code" dropdown field's options, nothing more.
type bugReportIssueForm struct {
	Body []struct {
		ID         string `yaml:"id"`
		Attributes struct {
			Options []string `yaml:"options"`
		} `yaml:"attributes"`
	} `yaml:"body"`
}

// bugReportExitCodeOptionRE matches an option's leading exit code, e.g. the
// "0" in "0 — every assertion passed". An option with no leading digit
// (`"other / not sure"`) never matches, which is what lets the escape valve
// coexist with the numeric options without being misread as one.
var bugReportExitCodeOptionRE = regexp.MustCompile(`^(\d+)\s`)

// bugReportExitCodeOptions parses the exit-code dropdown's options out of
// body, returning the numeric codes it offers. Fails the test outright if
// no dropdown field named "exit-code" is found at all -- a parse that
// silently found nothing must not be mistaken for a template that
// legitimately offers no codes.
func bugReportExitCodeOptions(t *testing.T, body string) []int {
	t.Helper()

	var form bugReportIssueForm
	if err := yaml.Unmarshal([]byte(body), &form); err != nil {
		t.Fatalf("parse bug_report.yml: %v", err)
	}

	for _, field := range form.Body {
		if field.ID != "exit-code" {
			continue
		}
		var codes []int
		for _, opt := range field.Attributes.Options {
			m := bugReportExitCodeOptionRE.FindStringSubmatch(opt)
			if m == nil {
				continue
			}
			n, err := strconv.Atoi(m[1])
			if err != nil {
				continue // the pattern only captures \d+; unreachable in practice
			}
			codes = append(codes, n)
		}
		return codes
	}

	t.Fatal(`bug_report.yml: no dropdown field with id "exit-code" found -- the template was restructured, or the parser is reading the wrong field`)
	return nil
}

// TestBugReportTemplate_exitCodeDropdownCoversReachableCodes is the
// containment check the M28-002 plan committed but left unimplemented: every
// code cmd/curlew can actually return must have a matching numeric option in
// bug_report.yml's exit-code dropdown.
func TestBugReportTemplate_exitCodeDropdownCoversReachableCodes(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	body := readmeReadFileOrFatal(t, filepath.Join(repoRoot, ".github", "ISSUE_TEMPLATE", "bug_report.yml"))

	options := bugReportExitCodeOptions(t, body)
	if len(options) == 0 {
		t.Fatal("bug_report.yml's exit-code dropdown has no numeric options -- the parse is broken, not the template")
	}
	offered := map[int]bool{}
	for _, c := range options {
		offered[c] = true
	}

	reachable := reachableExitCodes(t)
	for _, c := range exitcodes.Set(reachable) {
		if !offered[c] {
			t.Errorf("cmd/curlew can return exit %d but bug_report.yml's exit-code dropdown offers no matching "+
				"option (offers: %v) -- a reporter hitting this code has nothing to select", c, options)
		}
	}

	const escapeValve = "other / not sure"
	if !strings.Contains(body, escapeValve) {
		t.Errorf("bug_report.yml's exit-code dropdown has no %q escape option -- a reporter hitting an "+
			"undocumented code would otherwise be blocked from filing at all", escapeValve)
	}
}
