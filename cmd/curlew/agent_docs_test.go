package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/weiqigod/curlew/internal/docs"
	"github.com/weiqigod/curlew/internal/output"
)

func TestAgentHelpDiscovery(t *testing.T) {
	if _, err := docs.Prose("CLI_SPECIFICATION.md", "by itself after the command name"); err != nil {
		t.Fatal(err)
	}
	for _, command := range dispatchedCommands(t) {
		for _, flag := range []string{"--help", "-h"} {
			t.Run(command+flag, func(t *testing.T) {
				stdout, stderr, code := captureRun(t, command, flag)
				if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage:") {
					t.Fatalf("help must succeed without side effects: exit=%d stdout=%q stderr=%q", code, stdout, stderr)
				}
			})
		}
	}
}

func TestAgentNestedHelpDiscovery(t *testing.T) {
	for _, command := range []string{"import openapi", "vault list", "plugins list"} {
		for _, flag := range []string{"--help", "-h"} {
			args := append(strings.Fields(command), flag)
			stdout, stderr, code := captureRun(t, args...)
			if code != 0 || stderr != "" || !strings.Contains(stdout, "Usage:") {
				t.Fatalf("%v: exit %d: %s %s", args, code, stdout, stderr)
			}
		}
	}
	stdout, stderr, code := captureRun(t, "missing-command", "--help")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "Unknown command") {
		t.Fatalf("unknown command help: %d %s %s", code, stdout, stderr)
	}
}

func TestAgentManualRecipe(t *testing.T) {
	doc := readmeReadFileOrFatal(t, filepath.Join(readmeRepoRoot(t), "docs", "MANUAL.md"))
	start := strings.Index(doc, "A typical agent pattern:")
	if start < 0 {
		t.Fatal("missing agent pattern")
	}
	section := doc[start:]
	begin := strings.Index(section, "```bash\n")
	if begin < 0 {
		t.Fatal("missing agent command block")
	}
	section = section[begin+len("```bash\n"):]
	end := strings.Index(section, "```")
	if end < 0 {
		t.Fatal("unclosed command block")
	}
	script := section[:end]
	if strings.Count(script, "curlew ") < 4 {
		t.Fatal("agent recipe lost an execution step")
	}
	binary := buildBinary(t)
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	base := quickstartServer(t)
	dir := t.TempDir()
	stdout, stderr, code := runBinaryInDir(t, binary, dir, "init")
	if code != 0 {
		t.Fatalf("init: %s %s", stdout, stderr)
	}
	// The manual's example uses users.yaml; create its prerequisites explicitly.
	sample := filepath.Join(dir, "collections", "sample.yaml")
	body, err := os.ReadFile(sample)
	if err != nil {
		t.Fatal(err)
	}
	body = []byte(strings.ReplaceAll(string(body), "{{base_url}}", base))
	if err := os.WriteFile(filepath.Join(dir, "collections", "users.yaml"), body, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(sample); err != nil {
		t.Fatal(err)
	}
	cmd := exec.CommandContext(ctx, testBash(t), "-euo", "pipefail", "-c", script)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "PATH="+filepath.Dir(binary)+string(os.PathListSeparator)+os.Getenv("PATH"))
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("documented agent recipe failed: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), `"passed"`) {
		t.Fatalf("no successful run: %s", out)
	}
}

// Names must belong to the command that uses them, not merely occur somewhere
// in global help (the old --log-on-run and --non-interactive-on-run failures).
func TestDocumentedFlagsBelongToCommand(t *testing.T) {
	parsers := map[string]string{"run": "parseRunArgs", "watch": "parseRunArgs", "exec": "parseExecArgs", "validate": "parseValidateArgs", "info": "parseInfoArgs", "schema": "parseSchemaArgs", "perf": "parsePerfArgs", "pr-check": "parsePrCheckArgs"}
	allowed := map[string]map[string]bool{}
	for command, parser := range parsers {
		tokens := map[string]bool{"--help": true}
		walkFlagLiterals(t, parser, func(token string) {
			if strings.HasPrefix(token, "--") {
				tokens[token] = true
			}
		})
		if command == "watch" {
			tokens["--clear"] = true
		}
		allowed[command] = tokens
	}
	files := []string{"docs/MANUAL.md", "docs/CLI_SPECIFICATION.md", "docs/AGENT_GUIDE.md"}
	for _, file := range files {
		doc := readmeReadFileOrFatal(t, filepath.Join(readmeRepoRoot(t), file))
		// Only executable code is considered; prose may explicitly name unsupported flags.
		for _, block := range codeSpansWithLines(doc) {
			body := strings.ReplaceAll(block.text, "\\\n", " ")
			for _, line := range strings.Split(body, "\n") {
				line = strings.TrimSpace(line)
				if !strings.HasPrefix(line, "curlew ") {
					continue
				}
				words := strings.Fields(line)
				if len(words) < 2 {
					continue
				}
				tokens, ok := allowed[words[1]]
				if !ok {
					continue
				}
				// An inline span naming just a removed command in the migration appendix
				// is not an executable command recipe.
				if strings.Contains(line, "--report-upload") {
					continue
				}
				for _, word := range words[2:] {
					if word == "#" {
						break
					}
					if !strings.HasPrefix(word, "--") {
						continue
					}
					flag := strings.SplitN(strings.TrimRight(word, "`,;"), "=", 2)[0]
					if !tokens[flag] {
						t.Errorf("%s:%d: %s does not accept %s", file, block.line, words[1], flag)
					}
				}
			}
		}
	}
}

func TestAgentLogScope(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "records separate one-shot executions"); err != nil {
		t.Fatal(err)
	}
	if _, err := parseRunArgs([]string{"collection.yaml", "--log", "run.jsonl"}); err == nil {
		t.Fatal("run unexpectedly accepted exec-only log flag")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer srv.Close()
	path := filepath.Join(t.TempDir(), "exec.jsonl")
	input := fmt.Sprintf(`{"url":%q}`, srv.URL)
	for range 2 {
		_, stderr, code := captureExecCmd(t, input, "--stdin", "--log", path)
		if code != 0 {
			t.Fatalf("exec log: %d %s", code, stderr)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSpace(string(data)), "\n")
	if len(lines) != 2 {
		t.Fatalf("want two log entries: %s", data)
	}
	ids := map[string]bool{}
	for _, line := range lines {
		var entry output.JSONLEntry
		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			t.Fatal(err)
		}
		if entry.RunID == "" || ids[entry.RunID] {
			t.Fatalf("exec reused or omitted run ID: %s", line)
		}
		ids[entry.RunID] = true
	}
}

func TestAgentDryRunNeverSendsRequests(t *testing.T) {
	if _, err := docs.Prose("MANUAL.md", "plan does not resolve every runtime variable"); err != nil {
		t.Fatal(err)
	}
	var calls atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		calls.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()
	dir := t.TempDir()
	t.Chdir(dir)
	file := filepath.Join(dir, "preview.yaml")
	body := fmt.Sprintf("name: preview\nsetup:\n  - name: Setup\n    request: {method: POST, url: %q}\nrequests:\n  - name: Main\n    request: {method: POST, url: %q}\nteardown:\n  - name: Cleanup\n    request: {method: DELETE, url: %q}\n", srv.URL, srv.URL, srv.URL)
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{
		{"run", file, "--dry-run"},
		{"run", file, "--dry-run", "--format", "json"},
		{"run", file, "--dry-run", "--parallel"},
		{"run", "*.yaml", "--dry-run"},
	} {
		stdout, stderr, code := captureRun(t, args...)
		if code != 0 || !strings.Contains(stdout, "Wave") {
			t.Errorf("%v should print a plan: %d %s %s", args, code, stdout, stderr)
		}
	}
	// Planning must not invoke a variable resolver that would fail if executed.
	body = "variables:\n  deferred:\n    from_command: 'exit 19'\n" + body
	if err := os.WriteFile(file, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := captureRun(t, "run", file, "--dry-run")
	if code != 0 || !strings.Contains(stdout, "Wave") {
		t.Fatalf("dry run resolved commands: %d %s %s", code, stdout, stderr)
	}

	if n := calls.Load(); n != 0 {
		t.Fatalf("dry runs sent %d real requests", n)
	}
}
