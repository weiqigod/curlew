package markdown

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/assertion"
)

var (
	fixedTime  = time.Date(2026, 4, 25, 12, 0, 0, 0, time.UTC)
	fixedRunID = "0123456789abcdef0123456789abcdef"
)

func makePassReport() *Report {
	return &Report{
		CollectionName: "Sample API",
		RunID:          fixedRunID,
		StartedAt:      fixedTime,
		Summary:        SummaryCounts{Total: 1, Passed: 1},
		Requests: []RequestEntry{{
			RequestID:  "req-1",
			Slug:       "get-user",
			Name:       "Get user",
			Method:     "GET",
			URL:        "https://api.example.com/users/1",
			StatusCode: 200,
			DurationMs: 42,
			WaveIndex:  -1,
			StartedAt:  fixedTime,
			RespHeaders: http.Header{
				"Content-Type": {"application/json"},
			},
			RespBody: []byte(`{"id":1,"name":"Alice"}`),
			Assertions: &assertion.Results{
				Passed: true,
				Items: []assertion.Result{
					{Type: "status", Expected: "200", Actual: "200", Passed: true},
				},
			},
		}},
	}
}

// TestMarkdown_Render_PassJSON verifies that a passing JSON request produces
// the expected 11-section markdown structure (M9-003 adds ### Response metadata).
func TestMarkdown_Render_PassJSON(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Verify the 11 required section markers (M9-003 adds ### Response metadata).
	markerREs := []*regexp.Regexp{
		regexp.MustCompile(`^# `),
		regexp.MustCompile(`^## Notes$`),
		regexp.MustCompile(`^<!-- BEGIN apitest:response`),
		regexp.MustCompile(`^## Response \(deterministic\)$`),
		regexp.MustCompile(`^### Request$`),
		regexp.MustCompile(`^### Response `),
		regexp.MustCompile(`^### Response metadata$`),
		regexp.MustCompile(`^### Timing$`),
		regexp.MustCompile(`^### Assertions$`),
		regexp.MustCompile(`^<!-- END apitest:response`),
		regexp.MustCompile(`^## Analysis$`),
	}
	lines := strings.Split(string(got), "\n")
	matched := make([]bool, len(markerREs))
	for _, line := range lines {
		for i, re := range markerREs {
			if re.MatchString(line) {
				matched[i] = true
			}
		}
	}
	for i, m := range matched {
		if !m {
			t.Errorf("section marker %d not found in output:\n%s", i, got)
		}
	}

	// Verify sentinel format.
	sentinelRE := regexp.MustCompile(
		`<!-- BEGIN apitest:response id=req-1 slug=get-user run=[0-9a-f]{32} -->`,
	)
	if !sentinelRE.Match(got) {
		t.Errorf("BEGIN sentinel not found or malformed:\n%s", got)
	}

	// Verify JSON body is pretty-printed.
	if !strings.Contains(string(got), "```json") {
		t.Error("expected ```json fenced block for JSON response")
	}
	if !strings.Contains(string(got), `"name": "Alice"`) {
		t.Error("expected pretty-printed JSON with 'name: Alice'")
	}

	// Verify passing assertion.
	if !strings.Contains(string(got), "[x] status") {
		t.Error("expected [x] marker for passing assertion")
	}

	compareOrUpdateGolden(t, "pass_json.md", got)
}

// TestMarkdown_Render_FailJSON verifies that a failing assertion produces
// the [ ] marker in the Assertions section.
func TestMarkdown_Render_FailJSON(t *testing.T) {
	report := makePassReport()
	report.Requests[0].Assertions = &assertion.Results{
		Passed: false,
		Items: []assertion.Result{
			{Type: "status", Expected: "201", Actual: "200", Passed: false},
		},
	}
	report.Summary = SummaryCounts{Total: 1, Failed: 1}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !strings.Contains(string(got), "[ ] status") {
		t.Errorf("expected [ ] marker for failing assertion:\n%s", got)
	}

	compareOrUpdateGolden(t, "fail_json.md", got)
}

// TestMarkdown_EmptyAssertions verifies that an empty assertions list renders
// `_No assertions declared._` per the spec.
func TestMarkdown_EmptyAssertions(t *testing.T) {
	report := makePassReport()
	report.Requests[0].Assertions = &assertion.Results{Items: nil}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !strings.Contains(string(got), "_No assertions declared._") {
		t.Errorf("expected _No assertions declared._ in output:\n%s", got)
	}

	compareOrUpdateGolden(t, "empty_assertions.md", got)
}

// TestMarkdown_Newlines verifies that LF is used inside the sentinel block
// and that CRLF above the BEGIN line is preserved verbatim.
func TestMarkdown_Newlines(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()

	// Write initial file with CRLF above the BEGIN sentinel.
	initial := "# Get user\r\n\r\n## Notes\r\n\r\nAGENT_ABOVE\r\n"
	target := filepath.Join(dir, "get-user.md")
	if err := os.WriteFile(target, []byte(initial), 0o644); err != nil {
		t.Fatalf("write initial file: %v", err)
	}

	// The file has no sentinel, so WriteReport should write get-user.md.new.
	var errBuf strings.Builder
	if err := WriteReport(report, dir, WriteOptions{Stderr: &errBuf}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	// Original should be untouched (CRLF preserved).
	original, _ := os.ReadFile(target)
	if !strings.Contains(string(original), "\r\n") {
		t.Error("original file CRLF should be preserved")
	}

	// Dot-new should exist and use LF inside sentinels.
	newFile, err := os.ReadFile(target + ".new")
	if err != nil {
		t.Fatalf("read .new file: %v", err)
	}
	// Inside the sentinel block, check that there are no CRLF.
	loc, _ := parseSentinels(newFile)
	if loc != nil {
		lines := splitLines(newFile)
		for i := loc.BeginLine; i <= loc.EndLine; i++ {
			line := string(lines[i])
			if strings.Contains(line, "\r\n") {
				t.Errorf("CRLF found inside sentinel block at line %d: %q", i, line)
			}
		}
	}

	if !strings.Contains(errBuf.String(), "no sentinel pair") {
		t.Errorf("expected 'no sentinel pair' warning, got: %s", errBuf.String())
	}
}

// TestMarkdown_RenderRequest_Headers verifies that multiple request headers are
// rendered in sorted order (deterministic across runs, regardless of map iteration).
// Note: X-Custom-Header used instead of X-Request-ID because X-Request-ID is
// a volatile header filtered from the request signal block (M9-003).
func TestMarkdown_RenderRequest_Headers(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RequestHdr = map[string]string{
		"Authorization":   "Bearer token123",
		"Accept":          "application/json",
		"X-Custom-Header": "my-value",
	}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	// All three headers must appear.
	for _, hdr := range []string{"Authorization: Bearer token123", "Accept: application/json", "X-Custom-Header: my-value"} {
		if !strings.Contains(content, hdr) {
			t.Errorf("expected header %q in output:\n%s", hdr, content)
		}
	}

	// Headers must appear in sorted order (Accept < Authorization < X-Custom-Header).
	acceptPos := strings.Index(content, "Accept:")
	authPos := strings.Index(content, "Authorization:")
	xrPos := strings.Index(content, "X-Custom-Header:")
	if acceptPos >= authPos || authPos >= xrPos {
		t.Errorf("headers not in sorted order: Accept=%d Authorization=%d X-Custom-Header=%d\n%s",
			acceptPos, authPos, xrPos, content)
	}
}

// TestMarkdown_RenderRequest_JSONBody verifies that a non-nil RequestBody is
// pretty-printed inside a ```json fenced block in the ### Request section.
func TestMarkdown_RenderRequest_JSONBody(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RequestBody = map[string]any{"name": "Alice", "age": 30}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	if !strings.Contains(content, "```json") {
		t.Error("expected ```json fenced block for request body")
	}
	// json.Marshal on a map with string+int produces stable output;
	// just check both fields appear pretty-printed.
	if !strings.Contains(content, `"age"`) {
		t.Error("expected 'age' key in pretty-printed request body")
	}
	if !strings.Contains(content, `"name"`) {
		t.Error("expected 'name' key in pretty-printed request body")
	}
}

// TestMarkdown_RenderRequest_HeadersDeterminism verifies that rendering the
// same entry twice produces byte-identical ### Request sections regardless of
// map iteration order — the core guarantee behind Finding #3.
func TestMarkdown_RenderRequest_HeadersDeterminism(t *testing.T) {
	headers := map[string]string{
		"Z-Last":  "z",
		"A-First": "a",
		"M-Mid":   "m",
	}
	entry := &RequestEntry{
		RequestID:  "req-1",
		Slug:       "get-user",
		Name:       "Get user",
		Method:     "GET",
		URL:        "https://api.example.com/users/1",
		WaveIndex:  -1,
		StartedAt:  fixedTime,
		RequestHdr: headers,
	}

	var run1, run2 bytes.Buffer
	renderRequest(&run1, entry)
	renderRequest(&run2, entry)

	if run1.String() != run2.String() {
		t.Errorf("renderRequest is not deterministic:\nrun1:\n%s\nrun2:\n%s", run1.String(), run2.String())
	}
}

// TestMarkdown_SentinelExactFormat verifies the exact byte format of the
// BEGIN sentinel line.
func TestMarkdown_SentinelExactFormat(t *testing.T) {
	report := makePassReport()
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	re := regexp.MustCompile(
		`^<!-- BEGIN apitest:response id=req-1 slug=get-user run=[0-9a-f]{32} -->$`,
	)
	matched := false
	for _, line := range strings.Split(string(got), "\n") {
		if re.MatchString(line) {
			matched = true
			break
		}
	}
	if !matched {
		t.Errorf("BEGIN sentinel exact format not matched in:\n%s", got)
	}
}

// TestMarkdown_Render_Text verifies that a text/plain body renders verbatim
// inside a ```text fence.
func TestMarkdown_Render_Text(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"text/plain"}}
	report.Requests[0].RespBody = []byte("hello world")

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !regexp.MustCompile("(?m)^```text$").Match(got) {
		t.Errorf("expected ```text fence in:\n%s", got)
	}
	if !strings.Contains(string(got), "hello world") {
		t.Errorf("expected verbatim text in:\n%s", got)
	}
}

// TestMarkdown_Render_Empty verifies that an empty body renders the
// `_(empty body)_` marker with no code fence.
func TestMarkdown_Render_Empty(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{}
	report.Requests[0].RespBody = nil
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !strings.Contains(string(got), "_(empty body)_") {
		t.Errorf("expected _(empty body)_ marker:\n%s", got)
	}
}

// TestMarkdown_Render_HEAD verifies that a HEAD response renders the
// `_(HEAD — no body)_` marker regardless of body bytes.
func TestMarkdown_Render_HEAD(t *testing.T) {
	report := makePassReport()
	report.Requests[0].Method = "HEAD"
	report.Requests[0].RespBody = nil
	report.Requests[0].RespHeaders = http.Header{"Content-Length": {"42"}}
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !strings.Contains(string(got), "_(HEAD") {
		t.Errorf("expected HEAD marker:\n%s", got)
	}
}

// TestMarkdown_Render_Binary verifies that a binary body renders a ```hexdump
// fence followed by the total-bytes footer.
func TestMarkdown_Render_Binary(t *testing.T) {
	pngHdr := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	body := append(pngHdr, bytes.Repeat([]byte{0xff}, 600)...)
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"image/png"}}
	report.Requests[0].RespBody = body
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !regexp.MustCompile("(?m)^```hexdump$").Match(got) {
		t.Errorf("expected ```hexdump fence:\n%s", got)
	}
	if !regexp.MustCompile(`_\d+ bytes total_`).Match(got) {
		t.Errorf("expected total-bytes footer:\n%s", got)
	}
}

// TestMarkdown_Render_YAML verifies that a YAML body renders inside a ```yaml
// fence via canonical encoding.
func TestMarkdown_Render_YAML(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"application/yaml"}}
	report.Requests[0].RespBody = []byte("name: Alice\nage: 30\n")
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !regexp.MustCompile("(?m)^```yaml$").Match(got) {
		t.Errorf("expected ```yaml fence:\n%s", got)
	}
}

// TestMarkdown_Render_XML verifies that an XML body renders verbatim inside a
// ```xml fence.
func TestMarkdown_Render_XML(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"application/xml"}}
	report.Requests[0].RespBody = []byte("<root><a/></root>")
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !regexp.MustCompile("(?m)^```xml$").Match(got) {
		t.Errorf("expected ```xml fence:\n%s", got)
	}
}

// TestMarkdown_Render_HTML verifies that an HTML body renders verbatim inside a
// ```html fence.
func TestMarkdown_Render_HTML(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"text/html"}}
	report.Requests[0].RespBody = []byte("<html><body>hi</body></html>")
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if !regexp.MustCompile("(?m)^```html$").Match(got) {
		t.Errorf("expected ```html fence:\n%s", got)
	}
}

// TestMarkdown_VolatileHeaders verifies that Date, X-Request-Id, Set-Cookie,
// Etag, Server, Age appear in ### Response metadata only and NOT in the
// response signal block.
func TestMarkdown_VolatileHeaders(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{
		"Content-Type":  {"application/json"},
		"Date":          {"Thu, 25 Apr 2026 12:00:00 GMT"},
		"X-Request-Id":  {"abc-123"},
		"Set-Cookie":    {"session=xyz"},
		"Etag":          {`"v1"`},
		"Server":        {"nginx"},
		"Age":           {"60"},
		"Cache-Control": {"no-cache"},
	}
	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))

	// Volatile headers must NOT appear in the response signal block (between
	// `### Response 200` and `### Response metadata`).
	signalRE := regexp.MustCompile(`### Response 200\n([\s\S]*?)### Response metadata`)
	sig := signalRE.FindSubmatch(got)
	if sig == nil {
		t.Fatalf("could not find response signal block:\n%s", got)
	}
	for _, name := range []string{"Date:", "X-Request-Id:", "Set-Cookie:", "Etag:", "Server:", "Age:"} {
		if bytes.Contains(sig[1], []byte(name)) {
			t.Errorf("volatile header %q leaked into signal block:\n%s", name, sig[1])
		}
	}

	// Volatile headers MUST appear in the metadata block.
	metaRE := regexp.MustCompile(`### Response metadata\n\n([\s\S]*?)### Timing`)
	meta := metaRE.FindSubmatch(got)
	if meta == nil {
		t.Fatalf("could not find metadata block:\n%s", got)
	}
	for _, name := range []string{"Date:", "X-Request-Id:", "Set-Cookie:", "Etag:", "Server:", "Age:"} {
		if !bytes.Contains(meta[1], []byte(name)) {
			t.Errorf("volatile header %q missing from metadata:\n%s", name, meta[1])
		}
	}
}

// TestMarkdown_Render_TruncatedBody verifies that a body larger than BodyCapBytes
// produces the truncation marker line in the rendered markdown output.
// This exercises the `if truncated` branch inside renderBody (lines 270-272).
func TestMarkdown_Render_TruncatedBody(t *testing.T) {
	// Build a 1.1 MiB plain-text body so it exceeds the 1 MiB cap.
	bigBody := bytes.Repeat([]byte("x"), BodyCapBytes+100*1024)
	report := makePassReport()
	report.Requests[0].RespHeaders = http.Header{"Content-Type": {"text/plain"}}
	report.Requests[0].RespBody = bigBody

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// The truncation marker must be present.
	wantMarker := "_... truncated (body was"
	if !strings.Contains(string(got), wantMarker) {
		t.Errorf("expected truncation marker %q in output:\n%s", wantMarker, got[:min(len(got), 512)])
	}

	// The marker should include the original byte count.
	originalLen := len(bigBody)
	expectedSize := fmt.Sprintf("body was %d bytes", originalLen)
	if !strings.Contains(string(got), expectedSize) {
		t.Errorf("expected %q in truncation marker:\n%s", expectedSize, got[:min(len(got), 512)])
	}

	// The ```text fence must still be present (body is text/plain).
	if !regexp.MustCompile("(?m)^```text$").Match(got) {
		t.Errorf("expected ```text fence alongside truncation marker:\n%s", got[:min(len(got), 512)])
	}
}

// TestMarkdown_VolatileRequestHeaders verifies that volatile headers set in
// entry.RequestHdr are excluded from the ### Request section of the markdown.
// This exercises the IsVolatileHeader call inside renderRequest (line 218).
func TestMarkdown_VolatileRequestHeaders(t *testing.T) {
	report := makePassReport()
	report.Requests[0].RequestHdr = map[string]string{
		"Authorization": "Bearer token",
		"X-Request-Id":  "volatile-request-id",
		"Accept":        "application/json",
	}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}

	// Extract the ### Request section up to the next heading.
	requestRE := regexp.MustCompile(`### Request\n\n([\s\S]*?)\n### Response`)
	match := requestRE.FindSubmatch(got)
	if match == nil {
		t.Fatalf("could not find ### Request section:\n%s", got)
	}
	requestSection := match[1]

	// X-Request-Id is volatile — must NOT appear in the Request section.
	if bytes.Contains(requestSection, []byte("X-Request-Id:")) {
		t.Errorf("volatile header X-Request-Id leaked into ### Request section:\n%s", requestSection)
	}

	// Non-volatile headers must appear in the Request section.
	if !bytes.Contains(requestSection, []byte("Authorization: Bearer token")) {
		t.Errorf("expected Authorization header in ### Request section:\n%s", requestSection)
	}
	if !bytes.Contains(requestSection, []byte("Accept: application/json")) {
		t.Errorf("expected Accept header in ### Request section:\n%s", requestSection)
	}
}

// TestMarkdown_RedactionInvariant verifies that the formatter never re-emits
// raw secrets when the body and headers arrive already containing [REDACTED].
func TestMarkdown_RedactionInvariant(t *testing.T) {
	report := makePassReport()
	// Body and headers arrive at the formatter already redacted (mimicking
	// what main.go does before calling buildMarkdownReport).
	report.Requests[0].RespHeaders = http.Header{
		"Content-Type":  {"application/json"},
		"Authorization": {"Bearer [REDACTED]"},
	}
	report.Requests[0].RespBody = []byte(`{"token":"[REDACTED]"}`)
	report.Requests[0].RequestHdr = map[string]string{
		"Authorization": "Bearer [REDACTED]",
	}
	report.Requests[0].RequestBody = map[string]any{"secret": "[REDACTED]"}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}
	got, _ := os.ReadFile(filepath.Join(dir, "get-user.md"))
	// Must contain [REDACTED] tokens (proving headers/body flowed through).
	if count := bytes.Count(got, []byte("[REDACTED]")); count < 4 {
		t.Errorf("expected at least 4 [REDACTED] tokens, got %d:\n%s", count, got)
	}
	// Must NOT contain any non-redacted secret-looking values.
	for _, secret := range []string{"sk_live_", "ghp_", "Bearer eyJ"} {
		if bytes.Contains(got, []byte(secret)) {
			t.Errorf("unexpected raw secret %q leaked into markdown:\n%s", secret, got)
		}
	}
}

// TestMarkdown_Render_CelFailureAssertion verifies that a CEL assertion
// failure — whose Actual field is a multiline string containing the expression
// source and resolved sub-values — is rendered correctly in the Assertions
// section. The [ ] marker must appear, and the full multiline Actual string
// must be preserved (quoted by the %q format the renderer uses). This locks in
// the DoD requirement: "covered by golden tests in internal/output/".
func TestMarkdown_Render_CelFailureAssertion(t *testing.T) {
	celSrc := "response.body.total == response.body.items.map(i, i.price).sum()"
	celActual := celSrc + "\n  response.body.total = 9.5\n  response.body.items = [{price:5} {price:5.5}]"

	report := makePassReport()
	report.Requests[0].Assertions = &assertion.Results{
		Passed: false,
		Items: []assertion.Result{
			{
				Type:     "assertions[0].cel",
				Expected: celSrc,
				Actual:   celActual,
				Passed:   false,
			},
		},
	}
	report.Summary = SummaryCounts{Total: 1, Failed: 1}

	dir := t.TempDir()
	if err := WriteReport(report, dir, WriteOptions{}); err != nil {
		t.Fatalf("WriteReport: %v", err)
	}

	got, err := os.ReadFile(filepath.Join(dir, "get-user.md"))
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	content := string(got)

	// The [ ] marker must be present (failing assertion).
	if !strings.Contains(content, "[ ] assertions[0].cel") {
		t.Errorf("expected [ ] marker for failing CEL assertion:\n%s", content)
	}
	// The expression source must appear in the Actual field (quoted).
	if !strings.Contains(content, "response.body.total == response.body.items") {
		t.Errorf("expression source missing from markdown output:\n%s", content)
	}
	// The resolved sub-value line must be present.
	if !strings.Contains(content, "response.body.total = 9.5") {
		t.Errorf("resolved sub-value line missing from markdown output:\n%s", content)
	}
}
