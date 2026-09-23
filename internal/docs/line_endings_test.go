package docs_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestRepositoryDeterministicTextUsesLF(t *testing.T) {
	root := filepath.Join("..", "..")
	patterns := []string{
		"README.md",
		"docs/AGENT_GUIDE.md",
		"docs/CLI_SPECIFICATION.md",
		"docs/MANUAL.md",
		":(glob)examples/**/*.md",
		":(glob)templates/skills/**/*.md",
		"cmd/curlew/testdata/skill_agent_golden.md",
		"internal/output/markdown/testdata/golden/*.md",
		":(glob)scripts/**/*.sh",
		":(glob)smoke/**/*.sh",
		":(glob)testapi/harness/**/*.sh",
		":(glob)testdata/**/*.sh",
		":(glob)scripts/**/*.ps1",
		":(glob)smoke/**/*.ps1",
		"internal/output/events/testdata/golden/*.ndjson",
		"site/src/content/examples/*.svx",
		"examples/agent/*.yaml",
		"examples/cookbook/*.yaml",
		":(glob)testapi/collections/**/*.yaml",
		"internal/schema/testdata/output_project_skill_claude.yaml",
		":(glob)internal/signer/awssigv4/testdata/**/*.txt",
		"ui/index.html",
		"internal/uiserver/assets/dist/*.html",
		"internal/uiserver/assets/dist/*.js",
		"internal/uiserver/assets/dist/*.css",
	}
	args := append([]string{"ls-files", "--"}, patterns...)
	cmd := exec.Command("git", args...)
	cmd.Dir = root
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("list deterministic text: %v", err)
	}

	paths := strings.Fields(string(output))
	if len(paths) == 0 {
		t.Fatal("no deterministic text files found")
	}
	attr := exec.Command("git", "check-attr", "--stdin", "eol")
	attr.Dir = root
	attr.Stdin = strings.NewReader(strings.Join(paths, "\n") + "\n")
	attrOutput, err := attr.Output()
	if err != nil {
		t.Fatalf("check eol attributes: %v", err)
	}
	attributes := make(map[string]string, len(paths))
	for _, line := range strings.Split(strings.TrimSpace(string(attrOutput)), "\n") {
		path, value, ok := strings.Cut(line, ": eol: ")
		if ok {
			attributes[path] = value
		}
	}
	for _, path := range paths {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			if attributes[path] != "lf" {
				t.Fatalf("%s does not declare eol=lf: %q", path, attributes[path])
			}

			content, readErr := os.ReadFile(filepath.Join(root, filepath.FromSlash(path)))
			if readErr != nil {
				t.Fatalf("read file: %v", readErr)
			}
			if bytes.Contains(content, []byte("\r\n")) {
				t.Fatalf("%s contains CRLF in the working tree", path)
			}
		})
	}
}
