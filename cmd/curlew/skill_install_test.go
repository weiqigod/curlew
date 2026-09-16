package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

func TestSkillInstallCLI(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "install or update the same bundled skill"); err != nil {
		t.Fatal(err)
	}
	binary := buildBinary(t)
	for agent, target := range map[string]string{"codex": ".agents", "claude": ".claude", "copilot": ".github"} {
		t.Run(agent, func(t *testing.T) {
			dir := t.TempDir()
			project := "name: existing\n"
			if err := os.WriteFile(filepath.Join(dir, "curlew.yaml"), []byte(project), 0o600); err != nil {
				t.Fatal(err)
			}
			root := filepath.Join(dir, target, "skills", "curlew")
			for _, verb := range []string{"install", "install", "update"} {
				stdout, stderr, code := runBinaryInDir(t, binary, dir, "skill", verb, "--agent", agent)
				if code != 0 || stderr != "" || !strings.Contains(stdout, root) {
					t.Fatalf("%s: %d %s %s", verb, code, stdout, stderr)
				}
			}
			skill := readmeReadFileOrFatal(t, filepath.Join(root, "SKILL.md"))
			if !strings.Contains(skill, "name: curlew") || strings.Contains(skill, "{{curlew_version}}") {
				t.Fatal(skill)
			}
			if got := readmeReadFileOrFatal(t, filepath.Join(dir, "curlew.yaml")); got != project {
				t.Fatal("configuration changed")
			}
			original := readmeReadFileOrFatal(t, filepath.Join(root, "variables.md"))
			if err := os.WriteFile(filepath.Join(root, "SKILL.md"), []byte(skill+"\nTeam rules\n"), 0o600); err != nil {
				t.Fatal(err)
			}
			_, stderr, code := runBinaryInDir(t, binary, dir, "skill", "update", "--agent", agent)
			if code != 3 || !strings.Contains(stderr, "SKILL.md") {
				t.Fatalf("conflict: %d %s", code, stderr)
			}
			if got := readmeReadFileOrFatal(t, filepath.Join(root, "SKILL.md")); got != skill+"\nTeam rules\n" {
				t.Fatal("lost edit")
			}
			if got := readmeReadFileOrFatal(t, filepath.Join(root, "variables.md")); got != original {
				t.Fatal("partial update")
			}
		})
	}
}

func TestSkillCommandArguments(t *testing.T) {
	for _, args := range [][]string{{"skill"}, {"skill", "bad"}, {"skill", "install"}, {"skill", "install", "--agent"}, {"skill", "install", "--agent", "unknown"}, {"skill", "install", "--agent", "codex", "--force"}, {"skill", "install", "--agent", "codex", "one", "two"}} {
		_, stderr, code := captureRun(t, args...)
		if code == 0 || stderr == "" {
			t.Fatalf("accepted %v", args)
		}
	}
	for _, args := range [][]string{{"skill", "--help"}, {"skill", "install", "--help"}, {"skill", "update", "--help"}} {
		stdout, stderr, code := captureRun(t, args...)
		if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage:") {
			t.Fatalf("help: %d %s %s", code, stdout, stderr)
		}
	}
}
