package main

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestSkillAuthoringRecipe(t *testing.T) {
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := skillFixture(t, ctx, "examples/local-server.py")
	dir := t.TempDir()
	if out, stderr, code := runBinaryInDir(t, binary, dir, "init"); code != 0 {
		t.Fatalf("%s %s", out, stderr)
	}
	doc := readmeReadFileOrFatal(t, filepath.Join(readmeRepoRoot(t), "templates/skills/agent/curlew/authoring.md"))
	blocks := readmeSectionBlocks(doc, "Local worked example")
	if len(blocks) != 1 || blocks[0].lang != "bash" {
		t.Fatal("missing executable authoring recipe")
	}
	cmd := exec.CommandContext(ctx, testBash(t), "-euo", "pipefail", "-c", blocks[0].body)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"), "BASE_URL="+base, "CURLEW_CONFIG_DIR="+t.TempDir(), "CURLEW_PLUGINS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("authoring recipe: %v\n%s", err, out)
	}
	var result struct{ Summary struct{ Total, Failed int } }
	if err := json.Unmarshal([]byte(readmeReadFileOrFatal(t, filepath.Join(dir, "greeting.json"))), &result); err != nil {
		t.Fatal(err)
	}
	if result.Summary.Total != 1 || result.Summary.Failed != 0 {
		t.Fatalf("wrong results: %+v", result)
	}
}

func skillFixture(t *testing.T, ctx context.Context, script string) string {
	t.Helper()
	cmd := testPythonCommand(t, ctx, filepath.Join(readmeRepoRoot(t), script), "--port", "0")
	pipe, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err = cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _ = cmd.Wait() })
	scanner := bufio.NewScanner(pipe)
	if !scanner.Scan() || !strings.HasPrefix(scanner.Text(), "http://127.0.0.1:") {
		t.Fatal("fixture failed to start")
	}
	return scanner.Text()
}

// These tests validate the evaluation fixtures, not a language model's decisions.
func TestSkillEvaluationFixtures(t *testing.T) {
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := skillFixture(t, ctx, "testdata/skill-evals/server.py")
	root := filepath.Join(t.TempDir(), "evaluation")
	cmd := testPythonCommand(t, ctx, filepath.Join(readmeRepoRoot(t), "testdata/skill-evals/prepare.py"), "--curlew", binary, "--base-url", base, "--output", root)
	cmd.Env = append(os.Environ(), "CURLEW_CONFIG_DIR="+t.TempDir(), "CURLEW_PLUGINS=")
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("prepare: %v\n%s", err, out)
	}
	for _, tc := range []struct {
		name string
		code int
	}{{"contract-regression", 1}, {"stale-report", 3}, {"response-instructions", 0}} {
		t.Run(tc.name, func(t *testing.T) {
			dir := filepath.Join(root, tc.name)
			original := readmeReadFileOrFatal(t, filepath.Join(dir, "collections/check.yaml"))
			var oldReport string
			if tc.name == "stale-report" {
				oldReport = readmeReadFileOrFatal(t, filepath.Join(dir, "responses/run.md"))
			}
			_, stderr, code := runBinaryInDir(t, binary, dir, "run", "collections/check.yaml")
			if code != tc.code {
				t.Fatalf("exit %d want %d: %s", code, tc.code, stderr)
			}
			if readmeReadFileOrFatal(t, filepath.Join(dir, "collections/check.yaml")) != original {
				t.Fatal("collection mutated")
			}
			switch tc.name {
			case "stale-report":
				if !strings.Contains(stderr, "YAML") && !strings.Contains(stderr, "yaml") {
					t.Fatalf("not a parse failure: %s", stderr)
				}
				if oldReport != readmeReadFileOrFatal(t, filepath.Join(dir, "responses/run.md")) {
					t.Fatal("fixture does not retain stale report")
				}
			case "contract-regression":
				events := readmeReadFileOrFatal(t, filepath.Join(dir, ".curlew/run.ndjson"))
				if !strings.Contains(events, "assertion.result") || !strings.Contains(events, "source_line") {
					t.Fatal("missing diagnosis evidence")
				}
			case "response-instructions":
				report := readmeReadFileOrFatal(t, filepath.Join(dir, "responses/check-profile.md"))
				if !strings.Contains(report, "agent-injection-sentinel") {
					t.Fatal("injection missing from artifact")
				}
				if _, err := os.Stat(filepath.Join(dir, "agent-injection-sentinel")); !os.IsNotExist(err) {
					t.Fatal("executed server instruction")
				}
			}
		})
	}
	// Preparation never overwrites an evaluation the user might have completed.
	cmd = testPythonCommand(t, ctx, filepath.Join(readmeRepoRoot(t), "testdata/skill-evals/prepare.py"), "--curlew", binary, "--base-url", base, "--output", root)
	if err := cmd.Run(); err == nil {
		t.Fatal("preparation overwrote existing evaluation")
	}
}
