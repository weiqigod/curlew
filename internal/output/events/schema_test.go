package events_test

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/output/events"
)

// schemaPath returns the absolute path to docs/events-schema/<version>.json
// by walking up from the test file location. version is e.g. "v1.2".
func schemaPath(t *testing.T, version string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../internal/output/events/schema_test.go
	// walk up 4 directories to repo root
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(root, "docs", "events-schema", version+".json")
}

// compileEventSchema compiles the current (v1.5) JSON Schema. Callers that
// exercise the live emitter call this; most tests should use this helper.
func compileEventSchema(t *testing.T) *jsonschema.Schema {
	t.Helper()
	return compileEventSchemaVersion(t, "v1.5")
}

// compileEventSchemaVersion compiles the JSON Schema for the given version
// string (e.g. "v1.0", "v1.1", "v1.2"). Useful for regression tests that
// validate events against historical schema versions.
func compileEventSchemaVersion(t *testing.T, version string) *jsonschema.Schema {
	t.Helper()
	path := schemaPath(t, version)

	f, err := os.Open(path)
	if err != nil {
		t.Fatalf("open schema %s: %v", path, err)
	}
	doc, err := jsonschema.UnmarshalJSON(f)
	_ = f.Close()
	if err != nil {
		t.Fatalf("unmarshal schema %s: %v", path, err)
	}

	c := jsonschema.NewCompiler()
	if err := c.AddResource(path, doc); err != nil {
		t.Fatalf("AddResource %s: %v", path, err)
	}
	sch, err := c.Compile(path)
	if err != nil {
		t.Fatalf("Compile %s: %v", path, err)
	}
	return sch
}

// validateLine validates a single JSON line against the compiled schema.
func validateLine(t *testing.T, sch *jsonschema.Schema, line string) {
	t.Helper()
	var doc any
	if err := json.Unmarshal([]byte(line), &doc); err != nil {
		t.Fatalf("invalid JSON in line: %v\n  line: %s", err, line)
	}
	if err := sch.Validate(doc); err != nil {
		t.Errorf("schema validation failed for line:\n  %s\n  error: %v", line, err)
	}
}

// goldenDir returns the testdata/golden directory path.
func goldenDir(t *testing.T) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(thisFile), "testdata", "golden")
}

// TestEmitter_AllKindsValidateAgainstSchema emits one event of each kind into a
// buffer and validates every line against the v1.5 JSON Schema.
func TestEmitter_AllKindsValidateAgainstSchema(t *testing.T) {
	sch := compileEventSchema(t)

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "schema-validate-001",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	// Emit one of each kind.
	if err := em.EmitRunStart([]string{"run", "test.yaml"}, "test.yaml", "dev"); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://example.com/user", "setup", "test.yaml", 3); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	if err := em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true,
	}); err != nil {
		t.Fatalf("EmitAssertionResult: %v", err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:  "req-1",
		Outcome:    events.OutcomePassed,
		StatusCode: 200,
		Duration:   42 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}
	if err := em.EmitRunError(&apierrors.NetworkError{
		Kind:    apierrors.NetworkDNS,
		Message: "DNS lookup failed for example.com",
		Hint:    "check your DNS configuration",
	}); err != nil {
		t.Fatalf("EmitRunError: %v", err)
	}
	if err := em.EmitRunEnd(1, 1, 0, 0, 0); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines (one per kind), got %d", len(lines))
	}

	for i, line := range lines {
		t.Run(kindFromLine(t, line), func(t *testing.T) {
			_ = i
			validateLine(t, sch, line)
		})
	}
}

// kindFromLine extracts the "kind" field from a JSON line for use as a test name.
func kindFromLine(t *testing.T, line string) string {
	t.Helper()
	m := parseLine(t, line)
	if k, ok := m["kind"].(string); ok {
		return k
	}
	return "unknown"
}

// TestEmitter_GoldenSchemaValidates reads every golden NDJSON file in
// testdata/golden/ and validates each line against the v1.5 JSON Schema.
func TestEmitter_GoldenSchemaValidates(t *testing.T) {
	sch := compileEventSchema(t)
	dir := goldenDir(t)

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir %s: %v", dir, err)
	}

	var goldenFiles []string
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".ndjson") {
			goldenFiles = append(goldenFiles, filepath.Join(dir, e.Name()))
		}
	}

	if len(goldenFiles) == 0 {
		t.Skip("no golden files found — run with UPDATE_GOLDEN=1 to generate")
	}

	for _, gf := range goldenFiles {
		t.Run(filepath.Base(gf), func(t *testing.T) {
			f, err := os.Open(gf)
			if err != nil {
				t.Fatalf("open golden %s: %v", gf, err)
			}
			defer func() {
				if closeErr := f.Close(); closeErr != nil {
					t.Errorf("close golden %s: %v", gf, closeErr)
				}
			}()

			scanner := bufio.NewScanner(f)
			lineNum := 0
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line == "" {
					continue
				}
				lineNum++
				validateLine(t, sch, line)
			}
			if err := scanner.Err(); err != nil {
				t.Fatalf("scan %s: %v", gf, err)
			}
			if lineNum == 0 {
				t.Errorf("golden file %s is empty", gf)
			}
		})
	}
}

// TestEmitter_GoldenRunHappy writes a deterministic happy-path run and compares
// to (or writes) the golden file testdata/golden/run_happy.ndjson.
func TestEmitter_GoldenRunHappy(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "golden-run-happy",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStart([]string{"run", "happy.yaml"}, "happy.yaml", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "create-user", "Create user", "POST", "https://api.example.com/users", "", "happy.yaml", 1); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	if err := em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "req-1", Type: "status", Expected: "201", Actual: "201", Passed: true,
	}); err != nil {
		t.Fatalf("EmitAssertionResult: %v", err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "create-user",
		Outcome:     events.OutcomePassed,
		StatusCode:  201,
		Duration:    37 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}
	if err := em.EmitRunEnd(1, 1, 0, 0, 0); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	compareOrUpdateGolden(t, "run_happy.ndjson", buf.Bytes())
}

// TestEmitter_GoldenRunError writes a deterministic error scenario.
func TestEmitter_GoldenRunError(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "golden-run-error",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStart([]string{"run", "error.yaml"}, "error.yaml", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}
	if err := em.EmitRunError(&apierrors.Structured{
		Category: apierrors.CategoryParse,
		Code:     "PARSE_INVALID_YAML",
		Message:  "invalid YAML syntax",
		Hint:     "check indentation",
		FilePath: "error.yaml",
		Line:     5,
	}); err != nil {
		t.Fatalf("EmitRunError: %v", err)
	}
	if err := em.EmitRunEnd(0, 0, 0, 0, 1); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	compareOrUpdateGolden(t, "run_error.ndjson", buf.Bytes())
}

// TestEmitter_GoldenRunFailedAssertion writes a deterministic failed-assertion scenario.
func TestEmitter_GoldenRunFailedAssertion(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "golden-run-failed",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStart([]string{"run", "assert.yaml"}, "assert.yaml", "prod"); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://api.example.com/users/1", "", "assert.yaml", 3); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	if err := em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "req-1", Type: "status", Expected: "200", Actual: "404", Passed: false,
	}); err != nil {
		t.Fatalf("EmitAssertionResult: %v", err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "get-user",
		Outcome:     events.OutcomeFailed,
		StatusCode:  404,
		Duration:    28 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}
	if err := em.EmitRunEnd(1, 0, 1, 0, 1); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	compareOrUpdateGolden(t, "run_failed_assertion.ndjson", buf.Bytes())
}

// TestEmitter_GoldenAllAssertionKinds pins one event of every assertion kind.
//
// M24-001: until this existed, every golden fixture and every direct
// EmitAssertionResult call used "status" — the single kind whose emitted type
// happened to satisfy the published enum. The schema tests passed for four
// versions while body, header, schema, cel and timing assertions all violated
// it. A corpus that only exercises the conforming case is not a guard, so this
// fixture deliberately covers each kind including the ones that carry a target.
func TestEmitter_GoldenAllAssertionKinds(t *testing.T) {
	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-21T10:00:00Z"),
		RunID:         "golden-all-assertion-kinds",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStart([]string{"run", "kinds.yaml"}, "kinds.yaml", ""); err != nil {
		t.Fatalf("EmitRunStart: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://api.example.com/users/1", "main", "kinds.yaml", 3); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}

	kinds := []events.AssertionResultInput{
		{RequestID: "req-1", Type: "status", Expected: "200", Actual: "404", Passed: false},
		{RequestID: "req-1", Type: "body", Target: "$.user.name", Operator: "equals", Expected: "alice", Actual: "bob", Passed: false},
		{RequestID: "req-1", Type: "body", Target: "$.items", Operator: "contains_all", Expected: "contains all of [a b]", Actual: "[a]", Passed: false},
		{RequestID: "req-1", Type: "header", Target: "X-Request-Id", Operator: "exists", Expected: "exists", Actual: "header not present", Passed: false},
		{RequestID: "req-1", Type: "schema", Target: "$.user.id", Expected: "required: id", Actual: "missing", Passed: false},
		{RequestID: "req-1", Type: "timing", Expected: "<= 100ms", Actual: "250ms", Passed: false},
		{RequestID: "req-1", Type: "cel", Target: "assertions[0]", Expected: "compiled CEL bool expression", Actual: "got int, expected bool", Passed: false},
	}
	for _, k := range kinds {
		if err := em.EmitAssertionResult(k); err != nil {
			t.Fatalf("EmitAssertionResult(%s): %v", k.Type, err)
		}
	}

	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "get-user",
		Outcome:     events.OutcomeFailed,
		StatusCode:  404,
		Duration:    28 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}
	if err := em.EmitRunEnd(1, 0, 1, 0, 1); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	compareOrUpdateGolden(t, "all_assertion_kinds.ndjson", buf.Bytes())
}

// compareOrUpdateGolden compares got to the golden file at testdata/golden/name,
// or writes it if UPDATE_GOLDEN=1 is set or the file doesn't exist.
func compareOrUpdateGolden(t *testing.T, name string, got []byte) {
	t.Helper()
	dir := goldenDir(t)
	path := filepath.Join(dir, name)

	if os.Getenv("UPDATE_GOLDEN") == "1" {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		t.Logf("updated golden %s", name)
		return
	}

	existing, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		// Auto-create on first run.
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatalf("MkdirAll %s: %v", dir, err)
		}
		if err := os.WriteFile(path, got, 0o644); err != nil {
			t.Fatalf("WriteFile %s: %v", path, err)
		}
		t.Logf("created golden %s", name)
		return
	}
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}

	if !bytes.Equal(existing, got) {
		t.Errorf("golden %s mismatch:\n  want: %s\n  got:  %s", name, existing, got)
	}
}

// eventSchemaDocPath returns the absolute path to docs/EVENTS_SCHEMA_<version>.md
// by walking up from the test file location. version is e.g. "v1.2".
func eventSchemaDocPath(t *testing.T, version string) string {
	t.Helper()
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	// thisFile is .../internal/output/events/schema_test.go
	// walk up 4 directories to repo root
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))
	return filepath.Join(root, "docs", "EVENTS_SCHEMA_"+version+".md")
}

// TestSchema_DocInSyncWithCode verifies that every exported field on the Go
// event structs in events.go appears in the corresponding schema definition in
// docs/events-schema/v1.5.json, and that every field without an ",omitempty"
// JSON tag is listed in that definition's "required" array.
//
// This test prevents silent drift: when a developer adds, removes, or renames
// a field on an event struct, the test fails unless the schema file is updated
// to match.
func TestSchema_DocInSyncWithCode(t *testing.T) {
	// Load the raw JSON schema document (we need to introspect "required" and
	// "properties" text directly, not compile it).
	path := schemaPath(t, "v1.5")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read schema: %v", err)
	}
	var root map[string]any
	if err := json.Unmarshal(data, &root); err != nil {
		t.Fatalf("unmarshal schema: %v", err)
	}
	defs, ok := root["definitions"].(map[string]any)
	if !ok {
		t.Fatal("schema missing definitions")
	}

	// Mapping: Go struct type -> schema definition name.
	cases := []struct {
		name       string
		structType reflect.Type
		defName    string
	}{
		{"RunStart", reflect.TypeOf(events.RunStart{}), "RunStart"},
		{"RunError", reflect.TypeOf(events.RunError{}), "RunError"},
		{"RequestStart", reflect.TypeOf(events.RequestStart{}), "RequestStart"},
		{"RequestEnd", reflect.TypeOf(events.RequestEnd{}), "RequestEnd"},
		{"AssertionResult", reflect.TypeOf(events.AssertionResult{}), "AssertionResult"},
		{"RunEnd", reflect.TypeOf(events.RunEnd{}), "RunEnd"},
		{"EventError", reflect.TypeOf(events.EventError{}), "EventError"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			def, ok := defs[tc.defName].(map[string]any)
			if !ok {
				t.Fatalf("definition %q missing from schema", tc.defName)
			}

			wantProps, wantRequired := collectJSONFields(tc.structType)

			gotProps := keysOf(def["properties"])
			gotRequired := stringsOf(def["required"])

			if diff := diffStringSets(wantProps, gotProps); diff != "" {
				t.Errorf("%s: properties mismatch (struct vs schema):\n%s",
					tc.defName, diff)
			}
			if diff := diffStringSets(wantRequired, gotRequired); diff != "" {
				t.Errorf("%s: required mismatch (struct vs schema):\n%s",
					tc.defName, diff)
			}
		})
	}
}

// collectJSONFields walks an event struct (including its embedded Header) and
// returns the set of JSON field names and the subset that are required
// (no ",omitempty" tag). Pointer fields are dereferenced before inspection.
func collectJSONFields(t reflect.Type) (all, required []string) {
	allSet := make(map[string]struct{})
	requiredSet := make(map[string]struct{})

	var walk func(rt reflect.Type)
	walk = func(rt reflect.Type) {
		// Dereference pointers.
		for rt.Kind() == reflect.Ptr {
			rt = rt.Elem()
		}
		if rt.Kind() != reflect.Struct {
			return
		}
		for i := 0; i < rt.NumField(); i++ {
			f := rt.Field(i)
			tag := f.Tag.Get("json")
			if tag == "" || tag == "-" {
				// Anonymous embedded struct without json tag — recurse.
				if f.Anonymous {
					walk(f.Type)
				}
				continue
			}
			parts := strings.SplitN(tag, ",", 2)
			jsonName := parts[0]
			if jsonName == "" || jsonName == "-" {
				continue
			}
			omitempty := len(parts) > 1 && strings.Contains(parts[1], "omitempty")
			allSet[jsonName] = struct{}{}
			if !omitempty {
				requiredSet[jsonName] = struct{}{}
			}
		}
	}
	walk(t)

	all = sortedKeys(allSet)
	required = sortedKeys(requiredSet)
	return all, required
}

// keysOf returns the sorted key set of a JSON object value.
func keysOf(v any) []string {
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// stringsOf converts a JSON array value to a sorted []string.
func stringsOf(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	result := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			result = append(result, s)
		}
	}
	sort.Strings(result)
	return result
}

// diffStringSets returns a human-readable diff when two sets differ, or "" if
// equal.
func diffStringSets(want, got []string) string {
	wantSet := make(map[string]struct{}, len(want))
	for _, s := range want {
		wantSet[s] = struct{}{}
	}
	gotSet := make(map[string]struct{}, len(got))
	for _, s := range got {
		gotSet[s] = struct{}{}
	}

	var missing, extra []string
	for _, s := range want {
		if _, ok := gotSet[s]; !ok {
			missing = append(missing, s)
		}
	}
	for _, s := range got {
		if _, ok := wantSet[s]; !ok {
			extra = append(extra, s)
		}
	}

	if len(missing) == 0 && len(extra) == 0 {
		return ""
	}
	var sb strings.Builder
	if len(missing) > 0 {
		sort.Strings(missing)
		fmt.Fprintf(&sb, "  missing from schema: %v\n", missing)
	}
	if len(extra) > 0 {
		sort.Strings(extra)
		fmt.Fprintf(&sb, "  extra in schema:     %v\n", extra)
	}
	return sb.String()
}

// sortedKeys returns the sorted slice of keys from a set.
func sortedKeys(m map[string]struct{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// fenceBlock carries one fenced code block's metadata.
type fenceBlock struct {
	lang      string // "json" | "ndjson"
	startLine int    // 1-based, for error reporting
	body      string
}

// extractJSONFences scans src for ```json and ```ndjson fences and returns the
// contained bodies. Only recognises lines matching exactly ```json or ```ndjson
// (with optional trailing spaces) as openers, and ``` (with optional trailing
// spaces) as closers. Other languages are skipped.
func extractJSONFences(src string) []fenceBlock {
	var blocks []fenceBlock
	lines := strings.Split(src, "\n")
	var current *fenceBlock
	for i, line := range lines {
		trimmed := strings.TrimRight(line, " \t")
		if current == nil {
			// Look for an opener.
			switch trimmed {
			case "```json":
				current = &fenceBlock{lang: "json", startLine: i + 1}
			case "```ndjson":
				current = &fenceBlock{lang: "ndjson", startLine: i + 1}
			}
		} else {
			// Look for a closer.
			if trimmed == "```" {
				blocks = append(blocks, *current)
				current = nil
			} else {
				if current.body != "" {
					current.body += "\n"
				}
				current.body += line
			}
		}
	}
	return blocks
}

// TestSchema_MarkdownExamplesValidate extracts every fenced code block tagged
// ```json or ```ndjson from docs/EVENTS_SCHEMA_v1.5.md and validates each
// non-blank line against the v1.5 JSON Schema.
//
// Ensures the examples embedded in the agent-facing documentation stay
// consistent with the checked-in schema. A broken example fails fast.
func TestSchema_MarkdownExamplesValidate(t *testing.T) {
	sch := compileEventSchema(t)

	docPath := eventSchemaDocPath(t, "v1.5")
	src, err := os.ReadFile(docPath)
	if err != nil {
		t.Fatalf("read doc: %v", err)
	}

	blocks := extractJSONFences(string(src))
	if len(blocks) == 0 {
		t.Fatal("no ```json or ```ndjson fences found — doc is missing examples")
	}

	for i, block := range blocks {
		t.Run(fmt.Sprintf("block_%d_%s_line_%d", i, block.lang, block.startLine), func(t *testing.T) {
			// Each line is a complete JSON object (NDJSON convention).
			for _, line := range strings.Split(block.body, "\n") {
				line = strings.TrimSpace(line)
				if line == "" {
					continue
				}
				validateLine(t, sch, line)
			}
		})
	}
}

// TestSchema_v01ArtifactsRetained verifies that both v0.1.json and
// EVENTS_SCHEMA_v0.1.md still exist after the v1.0 promotion (M6-007 DoD #5).
// The v0.1 artefacts are retained as historical anchors; removing them would
// break consumers that documented or reference the old path.
func TestSchema_v01ArtifactsRetained(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))

	v01Schema := filepath.Join(root, "docs", "events-schema", "v0.1.json")
	if _, err := os.Stat(v01Schema); err != nil {
		t.Errorf("v0.1 schema artefact missing: %v — v0.1.json must be retained for history", err)
	}

	v01Doc := filepath.Join(root, "docs", "EVENTS_SCHEMA_v0.1.md")
	if _, err := os.Stat(v01Doc); err != nil {
		t.Errorf("v0.1 doc artefact missing: %v — EVENTS_SCHEMA_v0.1.md must be retained for history", err)
	}
}

// TestSchema_v10ArtifactsRetained verifies that both v1.0.json and
// EVENTS_SCHEMA_v1.0.md still exist after the v1.1 promotion (M8-004 DoD).
// The v1.0 artefacts are retained as historical anchors; removing them would
// break consumers that documented or reference the old path.
func TestSchema_v10ArtifactsRetained(t *testing.T) {
	_, thisFile, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	root := filepath.Dir(filepath.Dir(filepath.Dir(filepath.Dir(thisFile))))

	v10Schema := filepath.Join(root, "docs", "events-schema", "v1.0.json")
	if _, err := os.Stat(v10Schema); err != nil {
		t.Errorf("v1.0 schema artefact missing: %v — v1.0.json must be retained for history", err)
	}

	v10Doc := filepath.Join(root, "docs", "EVENTS_SCHEMA_v1.0.md")
	if _, err := os.Stat(v10Doc); err != nil {
		t.Errorf("v1.0 doc artefact missing: %v — EVENTS_SCHEMA_v1.0.md must be retained for history", err)
	}
}

// TestSchema_v10_validates compiles docs/events-schema/v1.0.json and validates
// hand-crafted v1.0-shaped events against it. This is a historical regression
// test; the v1.0 schema file must remain byte-unchanged.
func TestSchema_v10_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.0")
	// Hand-crafted v1.0 lines (no selection on run.start, no request_slug).
	lines := []string{
		`{"schema_version":"1.0","run_id":"r","id":1,"at_ms":0,"kind":"run.start","started_at":"2026-04-25T10:00:00Z","curlew_version":"0.1.0-test","cli_args":["run","t.yaml"]}`,
		`{"schema_version":"1.0","run_id":"r","id":2,"at_ms":1,"kind":"request.start","request_id":"req-1","name":"Get user","method":"GET","url":"https://example.com"}`,
		`{"schema_version":"1.0","run_id":"r","id":3,"at_ms":2,"kind":"assertion.result","request_id":"req-1","type":"status","passed":true,"expected":"200","actual":"200"}`,
		`{"schema_version":"1.0","run_id":"r","id":4,"at_ms":3,"kind":"request.end","request_id":"req-1","outcome":"passed","duration_ms":42}`,
		`{"schema_version":"1.0","run_id":"r","id":5,"at_ms":4,"kind":"run.error","error":{"category":"network","message":"DNS failed"}}`,
		`{"schema_version":"1.0","run_id":"r","id":6,"at_ms":5,"kind":"run.end","duration_ms":5,"total":1,"passed":1,"failed":0,"skipped":0,"exit_code":0,"event_count":6}`,
	}
	for _, line := range lines {
		t.Run(kindFromLine(t, line), func(t *testing.T) {
			validateLine(t, sch, line)
		})
	}
}

// TestSchema_v11_validates compiles docs/events-schema/v1.1.json and validates
// hand-crafted v1.1-shaped events against it (including a run.start with the
// optional selection field). This is a historical regression test; v1.1.json
// must remain byte-unchanged.
func TestSchema_v11_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.1")
	// Hand-crafted v1.1 lines (selection on run.start, no request_slug).
	lines := []string{
		`{"schema_version":"1.1","run_id":"r","id":1,"at_ms":0,"kind":"run.start","started_at":"2026-04-25T10:00:00Z","curlew_version":"0.1.0-test","cli_args":["run","t.yaml","--only","Get user"],"selection":["Get user"]}`,
		`{"schema_version":"1.1","run_id":"r","id":2,"at_ms":1,"kind":"request.start","request_id":"req-1","name":"Get user","method":"GET","url":"https://example.com"}`,
		`{"schema_version":"1.1","run_id":"r","id":3,"at_ms":2,"kind":"assertion.result","request_id":"req-1","type":"status","passed":true,"expected":"200","actual":"200"}`,
		`{"schema_version":"1.1","run_id":"r","id":4,"at_ms":3,"kind":"request.end","request_id":"req-1","outcome":"passed","duration_ms":42}`,
		`{"schema_version":"1.1","run_id":"r","id":5,"at_ms":4,"kind":"run.error","error":{"category":"input","code":"ONLY_NO_MATCH","message":"no match","hint":"Pass --only with a name matching a main request."}}`,
		`{"schema_version":"1.1","run_id":"r","id":6,"at_ms":5,"kind":"run.end","duration_ms":5,"total":1,"passed":1,"failed":0,"skipped":0,"exit_code":0,"event_count":6}`,
	}
	for _, line := range lines {
		t.Run(kindFromLine(t, line), func(t *testing.T) {
			validateLine(t, sch, line)
		})
	}
}

// TestSchema_v15_validates compiles docs/events-schema/v1.5.json and validates
// one event of each kind against it, including request.start and request.end
// with request_slug set. This is the authoritative regression test for the
// current schema.
func TestSchema_v15_validates(t *testing.T) {
	sch := compileEventSchemaVersion(t, "v1.5")

	var buf bytes.Buffer
	em, err := events.NewEmitter(&buf, events.Options{
		Clock:         fixedClock(t, "2026-04-25T10:00:00Z"),
		RunID:         "v12-validate-001",
		CurlewVersion: "0.1.0-test",
	})
	if err != nil {
		t.Fatalf("NewEmitter: %v", err)
	}

	if err := em.EmitRunStartWithInput(events.RunStartInput{
		CLIArgs:        []string{"run", "test.yaml", "--only", "Get user"},
		CollectionFile: "test.yaml",
		EnvName:        "dev",
		Selection:      []string{"Get user"},
	}); err != nil {
		t.Fatalf("EmitRunStartWithInput: %v", err)
	}
	if err := em.EmitRequestStart("req-1", "get-user", "Get user", "GET", "https://example.com/users", "main", "test.yaml", 5); err != nil {
		t.Fatalf("EmitRequestStart: %v", err)
	}
	if err := em.EmitAssertionResult(events.AssertionResultInput{
		RequestID: "req-1", Type: "status", Expected: "200", Actual: "200", Passed: true,
	}); err != nil {
		t.Fatalf("EmitAssertionResult: %v", err)
	}
	if err := em.EmitRequestEnd(events.RequestEndInput{
		RequestID:   "req-1",
		RequestSlug: "get-user",
		Outcome:     events.OutcomePassed,
		StatusCode:  200,
		Duration:    15 * time.Millisecond,
	}); err != nil {
		t.Fatalf("EmitRequestEnd: %v", err)
	}
	if err := em.EmitRunError(&apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "ONLY_NO_MATCH",
		Message:  `no request named "Nope"; available: "Get user"`,
		Hint:     "Pass --only with a name matching a main request.",
	}); err != nil {
		t.Fatalf("EmitRunError: %v", err)
	}
	if err := em.EmitRunEnd(1, 1, 0, 0, 0); err != nil {
		t.Fatalf("EmitRunEnd: %v", err)
	}

	lines := splitLines(&buf)
	if len(lines) != 6 {
		t.Fatalf("expected 6 lines, got %d", len(lines))
	}

	for _, line := range lines {
		t.Run(kindFromLine(t, line), func(t *testing.T) {
			validateLine(t, sch, line)
		})
	}
}
