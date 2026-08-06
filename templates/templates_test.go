package templates_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/templates"
)

func TestWalk_ClaudeYieldsAllTopicFiles(t *testing.T) {
	want := []string{
		"SKILL.md",
		"variables.md",
		"output-formats.md",
		"assertions.md",
		"retry.md",
		"parallel.md",
		"vault.md",
		"signing.md",
		"expressions.md",
		"exit-codes.md",
		"failure-playbook.md",
	}
	got := map[string]bool{}
	seq, err := templates.Walk("claude")
	if err != nil {
		t.Fatal(err)
	}
	for rel, body := range seq {
		got[rel] = true
		if len(body) == 0 {
			t.Errorf("empty body for %s", rel)
		}
	}
	for _, w := range want {
		if !got[w] {
			t.Errorf("missing file %q in Walk output", w)
		}
	}
}

func TestWalk_AllFilesAreMarkdown(t *testing.T) {
	seq, err := templates.Walk("claude")
	if err != nil {
		t.Fatal(err)
	}
	for rel := range seq {
		if !strings.HasSuffix(rel, ".md") {
			t.Errorf("non-markdown file in skill embed: %q", rel)
		}
	}
}

func TestWalk_UnknownSkillReturnsSentinel(t *testing.T) {
	_, err := templates.Walk("madeup")
	if !errors.Is(err, templates.ErrUnknownSkill) {
		t.Errorf("Walk(madeup) err = %v, want ErrUnknownSkill", err)
	}
}

func TestSkillRootDir_Claude(t *testing.T) {
	got := templates.SkillRootDir("claude")
	want := ".claude/skills/curlew"
	if got != want {
		t.Errorf("SkillRootDir(claude) = %q, want %q", got, want)
	}
}

func TestRender_ClaudeSubstitutesVersion(t *testing.T) {
	got, err := templates.Render("claude", "1.2.3")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "curlew 1.2.3") {
		t.Errorf("expected version 1.2.3 in output, got:\n%s", got)
	}
	if strings.Contains(got, "{{curlew_version}}") {
		t.Errorf("untouched template token in output:\n%s", got)
	}
}

func TestRender_UnknownSkill_ReturnsSentinel(t *testing.T) {
	_, err := templates.Render("madeup", "0.0.1")
	if !errors.Is(err, templates.ErrUnknownSkill) {
		t.Errorf("err = %v, want ErrUnknownSkill", err)
	}
	if !strings.Contains(err.Error(), "claude") {
		t.Errorf("error should name supported skills; got: %v", err)
	}
}

func TestRender_VersionCommentPresent(t *testing.T) {
	got, err := templates.Render("claude", "9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	want := "<!-- curlew-skill: agent v1.0 (curlew 9.9.9) -->"
	if !strings.Contains(got, want) {
		t.Errorf("missing version comment %q in output:\n%s", want, got)
	}
}

func TestIsSupportedSkill(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want bool
	}{
		{"agent is supported", "agent", true},
		{"claude is supported", "claude", true},
		{"empty string is not supported", "", false},
		{"unknown name is not supported", "madeup", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := templates.IsSupportedSkill(tc.in); got != tc.want {
				t.Errorf("IsSupportedSkill(%q) = %v, want %v", tc.in, got, tc.want)
			}
		})
	}
}

func TestSkillRelativePath_Claude(t *testing.T) {
	got := templates.SkillRelativePath("claude")
	want := ".claude/skills/curlew/SKILL.md"
	if got != want {
		t.Errorf("SkillRelativePath(claude) = %q, want %q", got, want)
	}
}

// TestSupportedSkills_LeadWithVendorNeutralName pins "agent" first in the
// enum. Order is user-visible: it is the order printed by --help and by the
// rejected-value error, so the vendor-neutral name must lead.
func TestSupportedSkills_LeadWithVendorNeutralName(t *testing.T) {
	if len(templates.SupportedSkills) == 0 || templates.SupportedSkills[0] != "agent" {
		t.Errorf("SupportedSkills = %v; want the vendor-neutral %q first",
			templates.SupportedSkills, "agent")
	}
}

// TestSkillNames_AreAliasesOfOnePayload holds every accepted --skill value to
// the same bytes and the same destination. curlew ships one Agent Skill, not
// one per vendor: the SKILL.md format and the .claude/skills/ project
// directory are both read by Claude Code and by GitHub Copilot. If a future
// name ever needs a distinct payload, this test is the thing that has to be
// deliberately rewritten rather than silently outgrown.
func TestSkillNames_AreAliasesOfOnePayload(t *testing.T) {
	canonical := templates.SupportedSkills[0]
	wantFiles := walkToMap(t, canonical)
	wantRender, err := templates.Render(canonical, "1.2.3")
	if err != nil {
		t.Fatalf("Render(%q): %v", canonical, err)
	}

	for _, alias := range templates.SupportedSkills[1:] {
		t.Run(alias, func(t *testing.T) {
			gotFiles := walkToMap(t, alias)
			if len(gotFiles) != len(wantFiles) {
				t.Fatalf("Walk(%q) yielded %d files, Walk(%q) yielded %d",
					alias, len(gotFiles), canonical, len(wantFiles))
			}
			for rel, want := range wantFiles {
				got, ok := gotFiles[rel]
				if !ok {
					t.Errorf("Walk(%q) is missing %q", alias, rel)
					continue
				}
				if got != want {
					t.Errorf("Walk(%q)[%q] differs from Walk(%q)[%q]", alias, rel, canonical, rel)
				}
			}
			gotRender, err := templates.Render(alias, "1.2.3")
			if err != nil {
				t.Fatalf("Render(%q): %v", alias, err)
			}
			if gotRender != wantRender {
				t.Errorf("Render(%q) differs from Render(%q)", alias, canonical)
			}
			if got, want := templates.SkillRootDir(alias), templates.SkillRootDir(canonical); got != want {
				t.Errorf("SkillRootDir(%q) = %q, want %q", alias, got, want)
			}
			if got, want := templates.SkillRelativePath(alias), templates.SkillRelativePath(canonical); got != want {
				t.Errorf("SkillRelativePath(%q) = %q, want %q", alias, got, want)
			}
		})
	}
}

func walkToMap(t *testing.T, skillName string) map[string]string {
	t.Helper()
	seq, err := templates.Walk(skillName)
	if err != nil {
		t.Fatalf("Walk(%q): %v", skillName, err)
	}
	out := map[string]string{}
	for rel, body := range seq {
		out[rel] = string(body)
	}
	if len(out) == 0 {
		t.Fatalf("Walk(%q) yielded no files", skillName)
	}
	return out
}
