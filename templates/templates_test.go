package templates_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/templates"
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
	want := ".claude/skills/apitest"
	if got != want {
		t.Errorf("SkillRootDir(claude) = %q, want %q", got, want)
	}
}

func TestRender_ClaudeSubstitutesVersion(t *testing.T) {
	got, err := templates.Render("claude", "1.2.3")
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(got, "apitest 1.2.3") {
		t.Errorf("expected version 1.2.3 in output, got:\n%s", got)
	}
	if strings.Contains(got, "{{apitest_version}}") {
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
	want := "<!-- apitest-skill: claude v1.0 (apitest 9.9.9) -->"
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
	want := ".claude/skills/apitest/SKILL.md"
	if got != want {
		t.Errorf("SkillRelativePath(claude) = %q, want %q", got, want)
	}
}
