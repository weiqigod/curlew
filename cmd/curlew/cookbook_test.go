package main

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"
)

// Every published page must name executable source and commands; new pages are
// discovered automatically and cannot opt out of execution with a metadata flag.
func TestCookbookRecipes(t *testing.T) {
	root := readmeRepoRoot(t)
	pages, err := filepath.Glob(filepath.Join(root, "site/src/content/examples/*.svx"))
	if err != nil {
		t.Fatal(err)
	}
	if len(pages) < 14 {
		t.Fatalf("cookbook lost examples: found %d, require at least 14", len(pages))
	}
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
	defer cancel()
	base := strings.TrimSuffix(quickstartServer(ctx, t), "/anything")
	marker := regexp.MustCompile(`<!-- cookbook-source: ([a-zA-Z0-9_./-]+) -->`)
	for _, path := range pages {
		t.Run(filepath.Base(path), func(t *testing.T) {
			page := readmeReadFileOrFatal(t, path)
			match := marker.FindStringSubmatch(page)
			if len(match) != 2 || !filepath.IsLocal(match[1]) {
				t.Fatal("missing or nonlocal cookbook source")
			}
			source := readmeReadFileOrFatal(t, filepath.Join(root, match[1]))
			blocks := readmeSectionBlocks(page, "Complete collection")
			if len(blocks) != 1 || blocks[0].lang != "yaml" || strings.TrimSpace(blocks[0].body) != strings.TrimSpace(source) {
				t.Fatalf("displayed YAML differs from %s", match[1])
			}
			dir := t.TempDir()
			for _, tree := range []string{"testapi", "examples/cookbook"} {
				if err := os.CopyFS(filepath.Join(dir, tree), os.DirFS(filepath.Join(root, tree))); err != nil {
					t.Fatal(err)
				}
			}
			var script strings.Builder
			for _, heading := range []string{"Prerequisites", "Run it"} {
				commands := readmeSectionBlocks(page, heading)
				if len(commands) != 1 || commands[0].lang != "bash" {
					t.Fatalf("%s must contain one executable bash recipe", heading)
				}
				script.WriteString(commands[0].body)
				script.WriteString("\n")
			}
			if !strings.Contains(script.String(), "curlew ") {
				t.Fatal("recipe does not invoke curlew")
			}
			out := executeDocScript(t, ctx, binary, dir, base, script.String())
			if !strings.Contains(out, "passed") && !strings.Contains(out, "Results:") && !strings.Contains(out, "success: pass=") {
				t.Fatalf("recipe produced no run result: %s", out)
			}
			if strings.Contains(page, "perf-local.yaml") {
				data := readmeReadFileOrFatal(t, filepath.Join(dir, "perf-results.json"))
				if !json.Valid([]byte(data)) {
					t.Fatalf("invalid perf report: %s", data)
				}
			}
		})
	}
}

func executeDocScript(t *testing.T, ctx context.Context, binary, dir, base, script string) string {
	t.Helper()
	cmd := exec.CommandContext(ctx, testBash(t), "-euo", "pipefail", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
		"CURLEW_CONFIG_DIR="+t.TempDir(), "NO_COLOR=1",
		"MUDFLAT_URL="+base, "MUDFLAT_WS_URL="+strings.Replace(base, "http://", "ws://", 1),
		"BASE_URL="+base+"/anything")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("documented commands failed: %v\n%s\n%s", err, script, out)
	}
	return string(out)
}

func TestAgentGuideRecipes(t *testing.T) {
	doc := readmeReadFileOrFatal(t, filepath.Join(readmeRepoRoot(t), "docs/AGENT_GUIDE.md"))
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := strings.TrimSuffix(quickstartServer(ctx, t), "/anything")
	dir := t.TempDir()
	for _, heading := range []string{"A complete local workflow", "JSON and one-shot requests"} {
		blocks := readmeSectionBlocks(doc, heading)
		if len(blocks) == 0 {
			t.Fatalf("missing recipes under %s", heading)
		}
		for _, block := range blocks {
			if block.lang != "bash" {
				t.Fatalf("unexpected recipe format %s", block.lang)
			}
			executeDocScript(t, ctx, binary, dir, base, block.body)
		}
	}
	for _, artifact := range []string{"responses/run.md", ".curlew/run.ndjson", "results.json", "events.ndjson", "verdict.json"} {
		if info, err := os.Stat(filepath.Join(dir, artifact)); err != nil || info.Size() == 0 {
			t.Fatalf("missing/empty %s: %v", artifact, err)
		}
	}
	sample := filepath.Join(dir, "collections/sample.yaml")
	body := readmeReadFileOrFatal(t, sample)
	body = strings.ReplaceAll(body, "status: 200", "status: 418")
	if err := os.WriteFile(sample, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := runBinaryInDir(t, binary, dir, "run", "collections/sample.yaml", "--var", "base_url="+base+"/anything")
	if code != 1 {
		t.Fatalf("expected assertion failure: %d %s", code, stderr)
	}
	failedReport := readmeReadFileOrFatal(t, filepath.Join(dir, "responses/run.md"))
	if !strings.Contains(strings.ToLower(failedReport), "fail") {
		t.Fatal("failure report lacks failure outcome")
	}
	_, stderr, code = runBinaryInDir(t, binary, dir, "run", "collections/sample.yaml", "--not-a-real-option")
	if code != 1 || !strings.Contains(stderr, "unknown flag") {
		t.Fatalf("expected usage failure: %d %s", code, stderr)
	}
	if current := readmeReadFileOrFatal(t, filepath.Join(dir, "responses/run.md")); current != failedReport {
		t.Fatal("usage error unexpectedly rewrote the report")
	}
	// The usage error and assertion failure share an exit code: the documented
	// stderr/freshness distinction is necessary, not an optional narration style.
}

func TestAgentReferenceRecipes(t *testing.T) {
	root := readmeRepoRoot(t)
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := strings.TrimSuffix(quickstartServer(ctx, t), "/anything")
	marker := regexp.MustCompile(`<!-- agent-source: ([a-zA-Z0-9_./-]+) -->`)
	for _, topic := range []string{"assertions", "expressions", "parallel", "retry", "signing", "variables", "vault"} {
		t.Run(topic, func(t *testing.T) {
			doc := readmeReadFileOrFatal(t, filepath.Join(root, "templates/skills/agent/curlew", topic+".md"))
			match := marker.FindStringSubmatch(doc)
			if len(match) != 2 || !filepath.IsLocal(match[1]) {
				t.Fatal("missing executable reference example")
			}
			source := readmeReadFileOrFatal(t, filepath.Join(root, match[1]))
			blocks := readmeSectionBlocks(doc, "Complete collection")
			if len(blocks) != 1 || blocks[0].lang != "yaml" || strings.TrimSpace(blocks[0].body) != strings.TrimSpace(source) {
				t.Fatal("skill YAML differs from executable fixture")
			}
			dir := t.TempDir()
			if err := os.WriteFile(filepath.Join(dir, "example.yaml"), []byte(source), 0o600); err != nil {
				t.Fatal(err)
			}
			commands := readmeSectionBlocks(doc, "Run the example")
			if len(commands) != 1 || commands[0].lang != "bash" {
				t.Fatal("missing executable commands")
			}
			executeDocScript(t, ctx, binary, dir, base, commands[0].body)
		})
	}
}
