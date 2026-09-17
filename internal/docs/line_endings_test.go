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
		"*.md",
		"*.sh",
		"*.ps1",
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
	for _, path := range paths {
		t.Run(strings.ReplaceAll(path, "/", "_"), func(t *testing.T) {
			attr := exec.Command("git", "check-attr", "eol", "--", path)
			attr.Dir = root
			attrOutput, attrErr := attr.Output()
			if attrErr != nil {
				t.Fatalf("check eol attribute: %v", attrErr)
			}
			if !strings.HasSuffix(strings.TrimSpace(string(attrOutput)), "eol: lf") {
				t.Fatalf("%s does not declare eol=lf: %s", path, attrOutput)
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