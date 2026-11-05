//go:build !windows

package main

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// TestReadme_quickstart_actually_works executes the README's Bash quickstart
// against a local mudflat server. Native Windows behavior has its own smoke gate.
func TestReadme_quickstart_actually_works(t *testing.T) {
	repoRoot := readmeRepoRoot(t)
	doc := readmeReadFileOrFatal(t, filepath.Join(repoRoot, "README.md"))

	quickstart, err := quickstartExtract(doc)
	if err != nil {
		t.Fatalf("extract quickstart: %v", err)
	}
	if len(quickstart.commands) < 3 {
		t.Fatalf("found %d quickstart command(s), want at least 3", len(quickstart.commands))
	}
	if nonBlankLines(quickstart.want) < 10 {
		t.Fatalf("quickstart expected-output block has only %d non-blank line(s), want at least 10", nonBlankLines(quickstart.want))
	}
	if !strings.Contains(quickstart.want, "passed") {
		t.Fatal("quickstart expected-output block never mentions passed")
	}
	if !strings.Contains(quickstart.script, "$BASE_URL") {
		t.Fatal("quickstart block cannot be redirected to the local server")
	}
	for _, host := range []string{"httpbin.org", "example.com", "https://", "http://"} {
		if strings.Contains(quickstart.script, host) {
			t.Fatalf("quickstart block names %q directly", host)
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	base := quickstartServer(t)
	binary := buildBinary(t)
	dir := t.TempDir()
	script := filepath.Join(dir, "quickstart.sh")
	if err := os.WriteFile(script, []byte(quickstart.script), 0o600); err != nil {
		t.Fatalf("write quickstart script: %v", err)
	}

	cmd := exec.CommandContext(ctx, "bash", "-euo", "pipefail", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(),
		"BASE_URL="+base,
		"NO_COLOR=1",
		"CURLEW_CONFIG_DIR="+t.TempDir(),
		"PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"),
	)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("README.md:%d: quickstart block failed: %v\n--- block ---\n%s\n--- output ---\n%s", quickstart.line, err, quickstart.script, output)
	}

	got := quickstartNormalizeDurations(strings.TrimSpace(string(output)))
	want := quickstartNormalizeDurations(strings.TrimSpace(quickstart.want))
	if got != want {
		t.Errorf("README.md:%d: quickstart output mismatch\n--- got ---\n%s\n--- want ---\n%s", quickstart.line, got, want)
	}
}

func nonBlankLines(value string) int {
	count := 0
	for _, line := range strings.Split(value, "\n") {
		if strings.TrimSpace(line) != "" {
			count++
		}
	}
	return count
}
