package harness_test

import (
	"bufio"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// buildCurlewBinary compiles the real curlew binary into a temp directory and
// returns its path. The binary is rebuilt fresh for every test run to avoid
// stale-build flakiness.
func buildCurlewBinary(t *testing.T) string {
	t.Helper()
	name := "curlew"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	binary := filepath.Join(t.TempDir(), name)
	// Locate the cmd/curlew package relative to this file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile = .../cmd/curlew-agent-harness/harness_test.go
	// pkg = .../cmd/curlew
	pkg := filepath.Join(filepath.Dir(filepath.Dir(thisFile)), "curlew")

	cmd := exec.Command("go", "build", "-o", binary, pkg)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build curlew binary: %v\n%s", err, out)
	}
	return binary
}

// Scenario holds the parsed details of a single harness scenario directory.
type Scenario struct {
	// Name is the scenario directory name (e.g. "missing-variable").
	Name string
	// Dir is the absolute path to the scenario directory.
	Dir string
	// Expect is the parsed expect.yaml contract.
	Expect Expectation
}

// discoverScenarios reads testdata/agent-harness/ and returns one Scenario per
// subdirectory that contains an expect.yaml file.
func discoverScenarios(t *testing.T, dir string) []Scenario {
	t.Helper()
	// Resolve dir relative to this file.
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	absDir := filepath.Join(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))), dir)

	entries, err := os.ReadDir(absDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", absDir, err)
	}

	var scenarios []Scenario
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		scenarioDir := filepath.Join(absDir, e.Name())
		expectFile := filepath.Join(scenarioDir, "expect.yaml")
		data, err := os.ReadFile(expectFile)
		if err != nil {
			// Skip directories without expect.yaml.
			continue
		}
		expect, err := ParseExpectation(data)
		if err != nil {
			t.Errorf("scenario %s: parse expect.yaml: %v", e.Name(), err)
			continue
		}
		scenarios = append(scenarios, Scenario{
			Name:   e.Name(),
			Dir:    scenarioDir,
			Expect: expect,
		})
	}
	return scenarios
}

// runCurlewForScenario executes curlew run against a scenario's collection
// and returns the path to the events NDJSON file and the process exit code.
//
// It handles the following optional scenario files:
//   - args.txt   — extra CLI args, one per line (# lines are comments)
//   - env.txt    — environment variables KEY=VALUE, one per line (# lines are comments)
//   - collection.template.yaml — collection with {{SERVER_URL}} placeholder
//     (substituted with a real httptest.Server for failing-assertion scenario)
func runCurlewForScenario(t *testing.T, binary string, s Scenario) (eventsPath string, exitCode int) {
	t.Helper()
	tmp := t.TempDir()
	eventsPath = filepath.Join(tmp, "events.jsonl")

	// Determine collection file path.
	collectionFile := findCollectionFile(t, s.Dir, tmp)

	// Build base args.
	args := []string{"run", collectionFile, "--events", eventsPath}

	// Append extra args from args.txt if present.
	args = append(args, readOptionalLines(t, filepath.Join(s.Dir, "args.txt"))...)

	// Build environment.
	env := append(os.Environ(), readOptionalLines(t, filepath.Join(s.Dir, "env.txt"))...)

	cmd := exec.Command(binary, args...)
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	} else if err != nil {
		t.Logf("scenario %s output:\n%s", s.Name, out)
		t.Fatalf("unexpected exec error: %v", err)
	}
	return eventsPath, exitCode
}

// findCollectionFile returns the path to the fixture collection for a scenario.
// If the scenario contains a "collection.template.yaml", it is instantiated
// ({{SERVER_URL}} substituted with a fresh httptest.Server) and the temp path
// is returned. Otherwise the first *.yaml file that is NOT "expect.yaml" is used.
func findCollectionFile(t *testing.T, scenarioDir, tmp string) string {
	t.Helper()

	// Check for a template.
	templatePath := filepath.Join(scenarioDir, "collection.template.yaml")
	if data, err := os.ReadFile(templatePath); err == nil {
		// Spin up an httptest.Server and substitute {{SERVER_URL}}.
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))
		t.Cleanup(srv.Close)
		instantiated := strings.ReplaceAll(string(data), "{{SERVER_URL}}", srv.URL)
		out := filepath.Join(tmp, "collection.yaml")
		if err := os.WriteFile(out, []byte(instantiated), 0o600); err != nil {
			t.Fatalf("write instantiated collection: %v", err)
		}
		return out
	}

	// Fall back to the first *.yaml that is not expect.yaml.
	entries, err := os.ReadDir(scenarioDir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", scenarioDir, err)
	}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".yaml") {
			continue
		}
		if e.Name() == "expect.yaml" {
			continue
		}
		return filepath.Join(scenarioDir, e.Name())
	}
	t.Fatalf("no collection file found in %s", scenarioDir)
	return ""
}

// readOptionalLines reads a text file and returns non-empty, non-comment lines.
// Returns nil if the file does not exist.
func readOptionalLines(t *testing.T, path string) []string {
	t.Helper()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	var lines []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		lines = append(lines, line)
	}
	return lines
}

// parseNDJSON reads an NDJSON file and returns each line as a decoded map.
func parseNDJSON(t *testing.T, path string) []map[string]any {
	t.Helper()
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		// Events file may not exist if binary failed very early.
		return nil
	}
	if err != nil {
		t.Fatalf("open events file %s: %v", path, err)
	}
	defer func() {
		if closeErr := f.Close(); closeErr != nil {
			t.Errorf("close events file: %v", closeErr)
		}
	}()

	var events []map[string]any
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(line), &obj); err != nil {
			t.Fatalf("unmarshal NDJSON line %q: %v", line, err)
		}
		events = append(events, obj)
	}
	if err := sc.Err(); err != nil {
		t.Fatalf("scan events file: %v", err)
	}
	return events
}

// TestHarness_AllScenarios builds the curlew binary, discovers all scenario
// directories under testdata/agent-harness/, and asserts each scenario's event
// stream satisfies its expect.yaml contract.
func TestHarness_AllScenarios(t *testing.T) {
	if testing.Short() {
		t.Skip("harness builds the curlew binary; skipping in -short mode")
	}
	binary := buildCurlewBinary(t)
	scenarios := discoverScenarios(t, "testdata/agent-harness")
	if len(scenarios) < 6 {
		t.Fatalf("want at least 6 scenarios, got %d", len(scenarios))
	}
	for _, s := range scenarios {
		s := s // capture range variable
		t.Run(s.Name, func(t *testing.T) {
			evPath, exitCode := runCurlewForScenario(t, binary, s)
			stream := parseNDJSON(t, evPath)
			if err := s.Expect.Verify(stream, exitCode); err != nil {
				t.Fatalf("scenario %q failed contract:\n%s", s.Name, err)
			}
		})
	}
}

// TestHarness_FailsLoudlyOnContractBreach verifies that the harness itself
// produces a clear error message when an expectation is not met. This is the
// negative-test required by definition_of_done #3.
func TestHarness_FailsLoudlyOnContractBreach(t *testing.T) {
	syntheticStream := []map[string]any{
		{"kind": "run.start", "schema_version": "0.1"},
		{"kind": "run.end", "exit_code": float64(0)},
	}
	expect := Expectation{
		ExitCode: 5,
		Events: []ExpectedEvent{{
			Kind:     "run.error",
			MustHave: map[string]any{"error.category": "input"},
		}},
	}
	err := expect.Verify(syntheticStream, 0)
	if err == nil {
		t.Fatal("expected contract verification to fail; got nil")
	}
	if !strings.Contains(err.Error(), "no event with kind=run.error") {
		t.Errorf("error = %q, want substring %q", err.Error(), "no event with kind=run.error")
	}
	t.Logf("harness correctly reports contract breach:\n%s", err)
}

// TestHarness_ContractBreach_ExitCodeMismatch verifies that a wrong exit code
// produces a descriptive error.
func TestHarness_ContractBreach_ExitCodeMismatch(t *testing.T) {
	expect := Expectation{ExitCode: 5}
	err := expect.Verify(nil, 1)
	if err == nil {
		t.Fatal("expected error for exit code mismatch; got nil")
	}
	if !strings.Contains(err.Error(), "exit_code = 1, want 5") {
		t.Errorf("error = %q, want substring %q", err.Error(), "exit_code = 1, want 5")
	}
}

// TestHarness_ContractBreach_HintVerb verifies the hint verb requirement.
func TestHarness_ContractBreach_HintVerb(t *testing.T) {
	stream := []map[string]any{
		{
			"kind": "run.error",
			"error": map[string]any{
				"category": "input",
				"hint":     "something vague happened without any verb",
			},
		},
	}
	expect := Expectation{
		Events: []ExpectedEvent{{
			Kind:            "run.error",
			HintContainsAny: []string{"Set", "Add", "Pass"},
		}},
	}
	err := expect.Verify(stream, 0)
	if err == nil {
		t.Fatal("expected error for missing hint verb; got nil")
	}
	if !strings.Contains(err.Error(), "contains none of") {
		t.Errorf("error = %q, want substring 'contains none of'", err.Error())
	}
}

// TestHarness_NetworkExemption_LineZeroAllowed verifies that line=0 is allowed
// when error.category=network (even if error.line_nonzero is required).
func TestHarness_NetworkExemption_LineZeroAllowed(t *testing.T) {
	stream := []map[string]any{
		{
			"kind": "request.end",
			"error": map[string]any{
				"category": "network",
				"code":     "NETWORK_CONNECTION_REFUSED",
				"hint":     "Check the target host is reachable.",
				"line":     float64(0),
			},
		},
	}
	expect := Expectation{
		Events: []ExpectedEvent{{
			Kind: "request.end",
			MustHave: map[string]any{
				"error.category":     "network",
				"error.line_nonzero": true,
			},
			HintContainsAny: []string{"Check"},
		}},
	}
	if err := expect.Verify(stream, 0); err != nil {
		t.Errorf("network exemption not applied: %v", err)
	}
}
