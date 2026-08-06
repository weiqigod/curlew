package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// scaffoldTreeWithSkill runs `curlew init --skill <name>` into a fresh temp
// directory and returns every scaffolded file keyed by slash-separated
// relative path.
func scaffoldTreeWithSkill(t *testing.T, skillName string) map[string]string {
	t.Helper()
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--skill", skillName, "--project-name", "p", dir)
	if code != 0 {
		t.Fatalf("init --skill %s exit = %d; stderr=%q", skillName, code, stderr)
	}
	tree := map[string]string{}
	err := filepath.WalkDir(dir, func(p string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, rerr := filepath.Rel(dir, p)
		if rerr != nil {
			return rerr
		}
		body, rerr := os.ReadFile(p)
		if rerr != nil {
			return rerr
		}
		tree[filepath.ToSlash(rel)] = string(body)
		return nil
	})
	if err != nil {
		t.Fatalf("walking scaffold: %v", err)
	}
	return tree
}

// TestInit_SkillNames_ScaffoldIdenticalProjects asserts at the binary level
// that every accepted --skill value produces a byte-identical project. The
// scaffolded artifact is a standard Agent Skill; the value selects nothing.
func TestInit_SkillNames_ScaffoldIdenticalProjects(t *testing.T) {
	want := scaffoldTreeWithSkill(t, "agent")
	got := scaffoldTreeWithSkill(t, "claude")

	if len(got) != len(want) {
		t.Fatalf("--skill claude scaffolded %d files, --skill agent scaffolded %d", len(got), len(want))
	}
	for rel, wantBody := range want {
		gotBody, ok := got[rel]
		if !ok {
			t.Errorf("--skill claude is missing %s", rel)
			continue
		}
		if gotBody != wantBody {
			t.Errorf("%s differs between --skill agent and --skill claude", rel)
		}
	}
	if _, ok := want[".claude/skills/curlew/SKILL.md"]; !ok {
		t.Errorf("--skill agent did not scaffold .claude/skills/curlew/SKILL.md; got %v", keysOf(want))
	}
}

func keysOf(m map[string]string) []string {
	out := make([]string, 0, len(m))
	for k := range m {
		out = append(out, k)
	}
	return out
}

// TestInit_SkillHelp_DoesNotClaimVendorLock guards the help text against
// re-acquiring the premise that the skill is Claude-only. .claude/skills/ is
// a project skill directory for GitHub Copilot as well, so the help must name
// more than one agent and must offer the vendor-neutral flag value.
func TestInit_SkillHelp_DoesNotClaimVendorLock(t *testing.T) {
	stdout, _, code := captureRun(t, "init", "--help")
	if code != 0 {
		t.Fatalf("init --help exit = %d, want 0", code)
	}
	for _, want := range []string{"--skill agent", "Claude Code", "Copilot"} {
		if !strings.Contains(stdout, want) {
			t.Errorf("init --help missing %q:\n%s", want, stdout)
		}
	}
}

// TestInit_SkillUnknown_NamesVendorNeutralValueFirst checks the rejected-value
// error surfaces the canonical name, not only the legacy alias.
func TestInit_SkillUnknown_NamesVendorNeutralValueFirst(t *testing.T) {
	dir := t.TempDir()
	_, stderr, code := captureRun(t, "init", "--skill", "madeup", dir)
	if code != 3 {
		t.Fatalf("exit = %d, want 3; stderr=%q", code, stderr)
	}
	if !strings.Contains(stderr, "agent") {
		t.Errorf("stderr should offer the vendor-neutral value; got %q", stderr)
	}
}
