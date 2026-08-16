package main

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// licensingSurface matches the vocabulary of the removed five-tier licensing
// system.
//
// Word-anchored on purpose. internal/schema/parity_test.go's tierWords lint
// (scoped to the published JSON Schemas, not the skill) matches with
// strings.Contains over a bare "free", which fires on this skill's own
// SKILL.md: "Edit it freely for your team's conventions" — verified, that
// line matches "free" under Contains today. \btiers?\b likewise keeps
// "prettier" and "frontier" out: both contain the substring "tier", but
// never with a word boundary in front of it. The colour words ("free",
// "solo", ...) appear only bound to "tier" — "free-tier" or "free tier" —
// since standalone they are ordinary English the skill legitimately uses.
var licensingSurface = regexp.MustCompile(
	`(?i)\b(?:` +
		`licen[cs](?:e|es|ed|ing)` + `|` +
		`feature[ _-]?gat(?:e|es|ed|ing)` + `|` +
		`tiers?` + `|` +
		`subscription` + `|` +
		`(?:free|solo|professional|enterprise)[ -]tier` +
		`)\b`,
)

// licensingExceptions records any "file:line" (relative to
// .claude/skills/curlew/) deliberately permitted to name the removed system.
// Empty, and an entry here is a diff — the same contract as
// help_parity_test.go's helpFlagExceptions.
var licensingExceptions = map[string]string{}

// TestSkill_scaffold_has_no_licensing_surface is the word-anchored version
// of the task's observable #3 (`! grep -ril "license\|feature gate\|tier"`)
// that runs in CI: it scans every scaffolded skill markdown file, line by
// line, for the vocabulary of the removed five-tier system.
func TestSkill_scaffold_has_no_licensing_surface(t *testing.T) {
	tree := scaffoldTreeWithSkill(t, "agent")
	const root = ".claude/skills/curlew/"

	var paths []string
	for path := range tree {
		if strings.HasPrefix(path, root) && strings.HasSuffix(path, ".md") {
			paths = append(paths, path)
		}
	}
	sort.Strings(paths) // deterministic failure ordering

	checked := 0
	for _, path := range paths {
		checked++
		rel := strings.TrimPrefix(path, root)
		for i, line := range strings.Split(tree[path], "\n") {
			loc := fmt.Sprintf("%s:%d", rel, i+1)
			if reason, exempt := licensingExceptions[loc]; exempt {
				t.Logf("%s deliberately permitted to name the removed system: %s", loc, reason)
				continue
			}
			if m := licensingSurface.FindString(line); m != "" {
				t.Errorf("%s names the removed licensing system (matched %q): %s", loc, m, strings.TrimSpace(line))
			}
		}
	}

	// init is broken, not the skill, and every prior assertion above passed
	// vacuously, if fewer than the 11 shipped skill files were even read.
	if checked < 11 {
		t.Fatalf("scanned %d file(s) under %s — init is broken, not the skill", checked, root)
	}
}

// TestSkill_licensingPattern_matchesWhatItIsFor is the guard that keeps
// TestSkill_scaffold_has_no_licensing_surface honest: a botched pattern
// cannot pass everything, because it must still match the lines it was
// written to catch.
func TestSkill_licensingPattern_matchesWhatItIsFor(t *testing.T) {
	tests := []struct {
		name string
		line string
		want bool
	}{
		{"the_exit_6_row_it_was_written_for", "| 6 | Feature gate denied | stderr (`feature_gated` line)", true},
		{"the_exit_9_row_it_was_written_for", "| 9 | License grace period expired |", true},
		{"the_html_row_it_was_written_for", "| `html` | Shareable report | `report:` HTML file (Professional tier) |", true},
		{"curlew_license_validate", "Suggest running `curlew license --validate`.", true},
		{"subscription_tier_prose", "Feature requires a higher subscription tier", true},
		{"freely_is_not_a_tier_word", "Edit it freely for your team's conventions", false},
		{"prettier_is_not_a_tier_word", "Formatted with prettier.", false},
		{"frontier_is_not_a_tier_word", "the frontier of the API", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := licensingSurface.MatchString(tc.line)
			if got != tc.want {
				t.Errorf("licensingSurface.MatchString(%q) = %v, want %v", tc.line, got, tc.want)
			}
		})
	}
}
