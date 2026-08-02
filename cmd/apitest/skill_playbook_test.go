package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/runner"
)

// TestSkillClaude_PlaybookMatchesBinary asserts every exit code documented in
// templates/skills/claude/apitest/SKILL.md's failure playbook matches the
// binary's actual behaviour. One sub-test per documented exit code: it
// constructs the scenario, runs the binary, asserts the exit code, and asserts
// the artifact the playbook tells the agent to read exists and is parseable.
//
// Skill drift fails this test: either the table changes without the binary
// changing, or the binary changes without the table changing. Either way, an
// agent reading the skill would receive stale instructions, and that is what
// this test prevents.
func TestSkillClaude_PlaybookMatchesBinary(t *testing.T) {
	t.Run("exit_0_all_passed", testPlaybookExit0)
	t.Run("exit_1_assertion_failed", testPlaybookExit1)
	t.Run("exit_2_guard_rail", testPlaybookExit2)
	t.Run("exit_3_parse_error", testPlaybookExit3Parse)
	t.Run("exit_3_config_error", testPlaybookExit3Config)
	t.Run("exit_3_only_no_match", testPlaybookExit3OnlyNoMatch)
	t.Run("exit_4_network_error", testPlaybookExit4)
	t.Run("exit_5_undefined_variable", testPlaybookExit5)
}

// Each helper below: (a) construct minimal scenario, (b) run binary in-process
// (or out-of-process where required), (c) assert exit code, (d) assert
// artifact named in the playbook is reachable.

func testPlaybookExit0(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	col := writeSimpleCollectionForMain(t, dir, srv.URL)

	report := filepath.Join(dir, "responses")
	_, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", report)
	if code != 0 {
		t.Fatalf("exit = %d, want 0", code)
	}
	// Per playbook: agent reads responses/run.md.
	if _, err := os.Stat(filepath.Join(report, "run.md")); err != nil {
		t.Errorf("playbook says agent reads responses/run.md; missing: %v", err)
	}
}

func testPlaybookExit1(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(404)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	col := writeAssertingCollection(t, dir, srv.URL, 200) // expect 200, get 404

	report := filepath.Join(dir, "responses")
	_, _, code := captureRunCmd(t, col, "--format", "markdown", "--report", report)
	if code != 1 {
		t.Fatalf("exit = %d, want 1", code)
	}
	// Per playbook: agent reads responses/<slug>.md for failed-assertion context.
	// Assert at least one .md file exists below report/ (the run.md plus per-request file).
	matches, _ := filepath.Glob(filepath.Join(report, "*.md"))
	if len(matches) < 2 {
		t.Errorf("playbook says agent reads per-request .md; expected >= 2 .md files in %s, got %d", report, len(matches))
	}
}

func testPlaybookExit2(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	old := runner.MaxRequests
	runner.MaxRequests = 1
	t.Cleanup(func() { runner.MaxRequests = old })
	dir := t.TempDir()
	col := writeMultiRequestCollection(t, dir, srv.URL, 3) // 3 requests, limit 1

	events := filepath.Join(dir, "run.ndjson")
	_, _, code := captureRunCmd(t, col, "--events", events)
	if code != 2 {
		t.Fatalf("exit = %d, want 2", code)
	}
	// Per playbook: agent reads .apitest/run.ndjson. The guard rail emits a
	// run.end event with exit_code=2; skipped requests emit request.end with
	// outcome="skipped". Either way the events file records the guard rail.
	if !ndjsonContainsRunEnd(t, events, 2) {
		t.Errorf("playbook says agent reads .apitest/run.ndjson; run.end with exit_code=2 not found in %s", events)
	}
}

func testPlaybookExit3Parse(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	col := filepath.Join(dir, "broken.yaml")
	// unclosed YAML string causes a parse error
	if err := os.WriteFile(col, []byte("name: Broken\nrequests:\n  - name: foo\n    request:\n      method: GET\n      url: \"unclosed-string\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, stderr, code := captureRunCmd(t, col)
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	// Per playbook: stderr names the line.
	if stderr == "" {
		t.Errorf("playbook says stderr names the YAML line; got empty stderr")
	}
}

func testPlaybookExit3Config(t *testing.T) {
	t.Helper()
	// Missing collection file: pre-flight error before HTTP fires.
	_, stderr, code := captureRunCmd(t, "no-such-file.yaml")
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	if stderr == "" {
		t.Errorf("playbook says stderr names the config concern; got empty stderr")
	}
}

func testPlaybookExit3OnlyNoMatch(t *testing.T) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(200)
	}))
	t.Cleanup(srv.Close)
	dir := t.TempDir()
	col := writePlaybookNamedCollection(t, dir, srv.URL, "Get user")

	_, stderr, code := captureRunCmd(t, col, "--only", "Nope")
	if code != 3 {
		t.Fatalf("exit = %d, want 3", code)
	}
	// Per playbook: stderr lists available request names.
	if !strings.Contains(stderr, "Get user") {
		t.Errorf("playbook says stderr lists available names; got: %q", stderr)
	}
}

func testPlaybookExit4(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	col := writeSimpleCollectionForMain(t, dir, "http://127.0.0.1:1") // closed port
	events := filepath.Join(dir, "run.ndjson")

	_, _, code := captureRunCmd(t, col, "--events", events, "--format", "json")
	if code != 4 {
		t.Fatalf("exit = %d, want 4", code)
	}
	// Per playbook: request.end event has error.category = network.
	if !ndjsonContainsRequestEndNetworkError(t, events) {
		t.Errorf("playbook says request.end carries network error; not found in %s", events)
	}
}

func testPlaybookExit5(t *testing.T) {
	t.Helper()
	col := `name: Undefined Var
variables:
  a: "{{b}}"
  b: "{{a}}"
requests:
  - name: Should Not Run
    request:
      method: GET
      url: "https://httpbin.org/get"
`
	dir := t.TempDir()
	colFile := writeCollection(t, dir, "col.yaml", col)
	events := filepath.Join(dir, "run.ndjson")

	_, _, code := captureRunCmd(t, colFile, "--events", events, "--format", "json")
	if code != 5 {
		t.Fatalf("exit = %d, want 5", code)
	}
	// Per playbook: run.error event names the variable.
	if !ndjsonContainsRunError(t, events, "circular") {
		t.Errorf("playbook says run.error names the variable; not found in %s", events)
	}
}

// --- helpers ---

// writeAssertingCollection writes a collection that asserts a specific status
// code. Used to trigger assertion failures (exit 1).
func writeAssertingCollection(t *testing.T, dir, serverURL string, expectedStatus int) string {
	t.Helper()
	f := filepath.Join(dir, "asserting.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: Ping\n    request:\n      method: GET\n      url: %q\n    assertions:\n      status: %d\n", serverURL, expectedStatus)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// writeMultiRequestCollection writes a collection with n identical requests.
// Used to trigger the guard rail (exit 2) when MaxRequests is set below n.
func writeMultiRequestCollection(t *testing.T, dir, serverURL string, n int) string {
	t.Helper()
	f := filepath.Join(dir, "multi.yaml")
	var sb strings.Builder
	sb.WriteString("name: Test\nrequests:\n")
	for i := range n {
		fmt.Fprintf(&sb, "  - name: req-%d\n    request:\n      method: GET\n      url: %q\n", i, serverURL)
	}
	if err := os.WriteFile(f, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// writePlaybookNamedCollection writes a collection with a single request using
// the given request name. Used to test --only no-match (exit 3).
func writePlaybookNamedCollection(t *testing.T, dir, serverURL, requestName string) string {
	t.Helper()
	f := filepath.Join(dir, "named.yaml")
	content := fmt.Sprintf("name: Test\nrequests:\n  - name: %q\n    request:\n      method: GET\n      url: %q\n", requestName, serverURL)
	if err := os.WriteFile(f, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return f
}

// ndjsonContainsRunEnd reads an NDJSON file and returns true if any run.end
// event has the given exit_code value.
func ndjsonContainsRunEnd(t *testing.T, path string, exitCode int) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("ndjsonContainsRunEnd: events file missing or unreadable: %v", err)
		return false
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev["kind"] != "run.end" {
			continue
		}
		// JSON numbers unmarshal to float64.
		if code, ok := ev["exit_code"].(float64); ok && int(code) == exitCode {
			return true
		}
	}
	return false
}

// ndjsonContainsRunError reads an NDJSON file and returns true if any run.error
// event's message contains the given substring (case-insensitive).
func ndjsonContainsRunError(t *testing.T, path, substr string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("ndjsonContainsRunError: events file missing or unreadable: %v", err)
		return false
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev["kind"] != "run.error" {
			continue
		}
		if errObj, ok := ev["error"].(map[string]any); ok {
			msg, _ := errObj["message"].(string)
			if strings.Contains(strings.ToLower(msg), strings.ToLower(substr)) {
				return true
			}
		}
	}
	return false
}

// ndjsonContainsRequestEndNetworkError reads an NDJSON file and returns true
// if any request.end event has error.category = "network".
func ndjsonContainsRequestEndNetworkError(t *testing.T, path string) bool {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Logf("ndjsonContainsRequestEndNetworkError: events file missing or unreadable: %v", err)
		return false
	}
	for _, line := range bytes.Split(data, []byte("\n")) {
		if len(line) == 0 {
			continue
		}
		var ev map[string]any
		if err := json.Unmarshal(line, &ev); err != nil {
			continue
		}
		if ev["kind"] != "request.end" {
			continue
		}
		if errObj, ok := ev["error"].(map[string]any); ok {
			cat, _ := errObj["category"].(string)
			if cat == "network" {
				return true
			}
		}
	}
	return false
}

// TestSkillClaude_MultiFile_PresentAfterInit asserts that all per-topic files
// are materialised after `apitest init --skill claude`.
func TestSkillClaude_MultiFile_PresentAfterInit(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "p", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	root := filepath.Join(dir, ".claude", "skills", "apitest")
	for _, name := range []string{
		"SKILL.md", "variables.md", "output-formats.md", "assertions.md",
		"retry.md", "parallel.md", "vault.md", "signing.md",
		"expressions.md", "exit-codes.md", "failure-playbook.md",
	} {
		if _, err := os.Stat(filepath.Join(root, name)); err != nil {
			t.Errorf("expected %s to be materialised: %v", name, err)
		}
	}
}

// TestSkillClaude_MultiFile_SkillMdIndexesTopicFiles asserts the root SKILL.md
// names each per-topic file in a "Topic files" index section.
func TestSkillClaude_MultiFile_SkillMdIndexesTopicFiles(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "p", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{
		"variables.md", "output-formats.md", "assertions.md",
		"retry.md", "parallel.md", "vault.md", "signing.md",
		"expressions.md", "exit-codes.md", "failure-playbook.md",
	} {
		if !strings.Contains(string(body), name) {
			t.Errorf("SKILL.md does not reference topic file %q", name)
		}
	}
}

// TestSkillClaude_MultiFile_ExpressionsDocumentsCEL asserts that expressions.md
// documents the CEL surface: if:, cel:, standard activation bindings, and
// disabled functions.
func TestSkillClaude_MultiFile_ExpressionsDocumentsCEL(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "p", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "expressions.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, substr := range []string{"if:", "cel:", "previous", "vars", "env", "response", "now()", "timestamp()", "Decision table"} {
		if !strings.Contains(s, substr) {
			t.Errorf("expressions.md missing %q", substr)
		}
	}
}

// TestSkillClaude_MultiFile_PlaybookHasCELErrorCodes asserts that
// failure-playbook.md includes entries for ERR_CEL_PARSE and ERR_CEL_TYPE.
func TestSkillClaude_MultiFile_PlaybookHasCELErrorCodes(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "p", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "failure-playbook.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, substr := range []string{"ERR_CEL_PARSE", "ERR_CEL_TYPE", "apitest validate"} {
		if !strings.Contains(s, substr) {
			t.Errorf("failure-playbook.md missing %q", substr)
		}
	}
}

// TestSkillClaude_MultiFile_TriggerPhrasesPreserved asserts the root SKILL.md
// still contains the original trigger phrases that agents key off.
func TestSkillClaude_MultiFile_TriggerPhrasesPreserved(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "p", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	body, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(body)
	for _, phrase := range []string{"run the test", "hit the staging API", "use apitest to"} {
		if !strings.Contains(s, phrase) {
			t.Errorf("SKILL.md missing trigger phrase %q", phrase)
		}
	}
}

// TestSkillClaude_SkillFileSnapshot asserts the rendered SKILL.md from
// init --skill claude matches a golden snapshot byte-for-byte (modulo the
// {{apitest_version}} substitution). Guards against accidental edits to the
// embedded template.
//
// Set APITEST_UPDATE_SNAPSHOTS=1 to regenerate the golden file.
func TestSkillClaude_SkillFileSnapshot(t *testing.T) {
	dir := t.TempDir()
	if _, _, code := captureRun(t, "init", "--skill", "claude", "--project-name", "snapshot-test", dir); code != 0 {
		t.Fatalf("init failed: code=%d", code)
	}
	got, err := os.ReadFile(filepath.Join(dir, ".claude", "skills", "apitest", "SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	// Substitute the binary's actual version with a stable placeholder so the
	// golden file is stable across version bumps. Two forms appear in the
	// rendered SKILL.md:
	//   1. "apitest 0.1.0-dev"  — in the version comment line
	//   2. "**0.1.0-dev**"      — in the Notes section (bold markdown)
	got = bytes.ReplaceAll(got, []byte("apitest "+version), []byte("apitest v0.0.0-test"))
	got = bytes.ReplaceAll(got, []byte("**"+version+"**"), []byte("**v0.0.0-test**"))

	goldenPath := filepath.Join("testdata", "skill_claude_golden.md")
	if os.Getenv("APITEST_UPDATE_SNAPSHOTS") == "1" {
		if err := os.WriteFile(goldenPath, got, 0o644); err != nil {
			t.Fatal(err)
		}
		t.Logf("snapshot updated: %s", goldenPath)
		return
	}

	want, err := os.ReadFile(goldenPath)
	if err != nil {
		t.Fatalf("read golden: %v (run with APITEST_UPDATE_SNAPSHOTS=1 to create)", err)
	}
	if !bytes.Equal(got, want) {
		t.Errorf("SKILL.md drift detected — re-run with APITEST_UPDATE_SNAPSHOTS=1 if intentional\nwant:\n%s\n\ngot:\n%s", want, got)
	}
}
