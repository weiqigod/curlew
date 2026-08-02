package uiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"unicode/utf8"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/runner"
	"github.com/weiqigod/curlew/internal/variable"
)

// --- REST JSON shapes (spec §4.8, §4.9) ---

type bodyJSON struct {
	Content     string `json:"content,omitempty"`
	Encoding    string `json:"encoding,omitempty"` // "base64" for binary
	Size        int    `json:"size"`
	Truncated   bool   `json:"truncated"`
	ContentType string `json:"content_type,omitempty"`
}

type timingJSON struct {
	DNSUs      *int64 `json:"dns_us,omitempty"`
	ConnectUs  *int64 `json:"connect_us,omitempty"`
	TLSUs      *int64 `json:"tls_us,omitempty"`
	TTFBUs     *int64 `json:"ttfb_us,omitempty"`
	DownloadUs *int64 `json:"download_us,omitempty"`
	TotalUs    int64  `json:"total_us"`
	Reused     bool   `json:"connection_reused"`
	Attempts   int    `json:"attempts"`
}

// timingJSONFromExec converts httpexec timing to the §4.9 REST shape.
func timingJSONFromExec(t *httpexec.Timing, attempts int) *timingJSON {
	if t == nil {
		return nil
	}
	if attempts < 1 {
		attempts = 1
	}
	us := func(d int64) *int64 {
		if d <= 0 {
			return nil
		}
		return &d
	}
	return &timingJSON{
		DNSUs:      us(t.DNS.Microseconds()),
		ConnectUs:  us(t.Connect.Microseconds()),
		TLSUs:      us(t.TLS.Microseconds()),
		TTFBUs:     us(t.TTFB.Microseconds()),
		DownloadUs: us(t.Download.Microseconds()),
		TotalUs:    t.Total.Microseconds(),
		Reused:     t.Reused,
		Attempts:   attempts,
	}
}

type attemptJSON struct {
	Number     int     `json:"number"`
	StatusCode int     `json:"status_code"`
	DurationMs int64   `json:"duration_ms"`
	DelayMs    int64   `json:"delay_ms"`
	Error      *string `json:"error"`
}

type retryJSON struct {
	Count    int           `json:"count"`
	Warnings []string      `json:"warnings"`
	Attempts []attemptJSON `json:"attempts"`
}

type errorJSON struct {
	Category string `json:"category"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// compact returns the §4.8 compact form (no hint).
func (e *errorJSON) compact() *errorJSON {
	if e == nil {
		return nil
	}
	return &errorJSON{Category: e.Category, Code: e.Code, Message: e.Message}
}

func errorJSONFrom(err error) *errorJSON {
	if err == nil {
		return nil
	}
	s := apierrors.ClassifyError(err)
	return &errorJSON{Category: string(s.Category), Code: s.Code, Message: s.Message, Hint: s.Hint}
}

type iterationJSON struct {
	Index    int    `json:"index"`
	Total    int    `json:"total"`
	BaseName string `json:"base_name"`
	BaseSlug string `json:"base_slug"`
}

type sourceJSON struct {
	File             string   `json:"file"`
	Line             int      `json:"line"`
	Snippet          []string `json:"snippet"`
	SnippetStartLine int      `json:"snippet_start_line"`
}

type assertionItemJSON struct {
	Type     string `json:"type"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}

type assertionsJSON struct {
	Passed bool                `json:"passed"`
	Items  []assertionItemJSON `json:"items"`
}

type requestPayloadJSON struct {
	Headers map[string]string `json:"headers"`
	Body    *bodyJSON         `json:"body"`
}

type responsePayloadJSON struct {
	Headers http.Header `json:"headers"`
	Body    *bodyJSON   `json:"body"`
}

// RequestDetail is the full per-request inspector payload (spec §4.9). The
// unexported raw* fields hold the complete redacted bodies for the /body
// endpoint and are not serialized.
type RequestDetail struct {
	RequestID  string               `json:"request_id"`
	Slug       string               `json:"slug"`
	Name       string               `json:"name"`
	Phase      string               `json:"phase"`
	Method     string               `json:"method"`
	URL        string               `json:"url"` // resolved, redacted as-sent
	Outcome    *string              `json:"outcome"`
	StatusCode int                  `json:"status_code,omitempty"`
	DurationMs int64                `json:"duration_ms"`
	WaveIndex  int                  `json:"wave_index"`
	Timing     *timingJSON          `json:"timing"`
	Retry      *retryJSON           `json:"retry"`
	Skipped    bool                 `json:"skipped"`
	SkipReason *string              `json:"skip_reason"`
	Warnings   []string             `json:"warnings"`
	Iteration  *iterationJSON       `json:"iteration"`
	Source     *sourceJSON          `json:"source"`
	Request    *requestPayloadJSON  `json:"request"`
	Response   *responsePayloadJSON `json:"response"`
	Assertions *assertionsJSON      `json:"assertions"`
	Error      *errorJSON           `json:"error"`

	rawRequestBody  []byte // full redacted bytes
	rawResponseBody []byte
	dataDrivenSeed  bool // planned seed for a data-driven item (expands at run time)
	failMessage     string
	retryCount      int
}

// requestListEntry is the §4.8 light list row.
type requestListEntry struct {
	RequestID  string         `json:"request_id"`
	Slug       string         `json:"slug"`
	Name       string         `json:"name"`
	Phase      string         `json:"phase"`
	Method     string         `json:"method"`
	Outcome    *string        `json:"outcome"`
	StatusCode int            `json:"status_code,omitempty"`
	DurationMs int64          `json:"duration_ms"`
	WaveIndex  int            `json:"wave_index"`
	RetryCount int            `json:"retry_count"`
	SkipReason *string        `json:"skip_reason"`
	FailMsg    *string        `json:"fail_message"`
	Error      *errorJSON     `json:"error"`
	Iteration  *iterationJSON `json:"iteration"`
	SourceFile string         `json:"source_file"`
	SourceLine int            `json:"source_line"`
}

// syntheticID is the planned-entry id used until the runner mints a real
// request id (parallel wave order is not predictable pre-run, and skipped
// requests never receive one).
func syntheticID(slug string) string { return "slug:" + slug }

// DetailCollector accumulates full redacted request details mid-run. It
// implements runner.EventSink and is fanned out alongside the EmitterSink;
// both share the same pre-run SensitiveSet (spec §6.3).
type DetailCollector struct {
	mu        sync.RWMutex
	order     []string                  // request ids in display order
	byID      map[string]*RequestDetail // request_id → detail
	sensitive *variable.SensitiveSet
	root      string // project root for snippet reads + path relativization
	total     int    // planned total (after selection filtering)
}

// NewDetailCollector builds an empty collector rooted at the project root.
func NewDetailCollector(root string) *DetailCollector {
	return &DetailCollector{byID: make(map[string]*RequestDetail), root: root}
}

// SetSensitive installs the shared pre-run redaction set (called by
// runservice.Execute through the fan-out sink).
func (c *DetailCollector) SetSensitive(s *variable.SensitiveSet) {
	c.mu.Lock()
	c.sensitive = s
	c.mu.Unlock()
}

// plannedItem describes one planned request for seeding.
type plannedItem struct {
	Slug, Name, Phase, Method string
	SourceFile                string // root-relative
	SourceLine                int
	WaveIndex                 int // -1 sequential / unknown
	DataDriven                bool
}

// Seed registers planned requests as pending rows (outcome null) so the
// client can group by wave/phase/collection from second zero (spec §4.8).
func (c *DetailCollector) Seed(items []plannedItem) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, it := range items {
		id := syntheticID(it.Slug)
		if _, exists := c.byID[id]; exists {
			continue
		}
		d := &RequestDetail{
			RequestID:      id,
			Slug:           it.Slug,
			Name:           it.Name,
			Phase:          it.Phase,
			Method:         it.Method,
			WaveIndex:      it.WaveIndex,
			Source:         &sourceJSON{File: it.SourceFile, Line: it.SourceLine},
			dataDrivenSeed: it.DataDriven,
		}
		c.byID[id] = d
		c.order = append(c.order, id)
	}
	c.total += len(items)
}

// relPath converts an absolute source path to root-relative.
func (c *DetailCollector) relPath(p string) string {
	if p == "" || !filepath.IsAbs(p) {
		return p
	}
	if rel, err := filepath.Rel(c.root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return filepath.Base(p)
}

// RequestStart implements runner.EventSink: adopt the seeded row (re-key from
// the synthetic id) or insert a fresh one (e.g. data-driven expansions the
// planner didn't predict).
func (c *DetailCollector) RequestStart(e runner.RequestEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sens := c.sensitive

	if d, ok := c.byID[syntheticID(e.RequestSlug)]; ok && e.RequestID != "" {
		// Adopt: re-key the planned entry under the real id.
		delete(c.byID, d.RequestID)
		for i, id := range c.order {
			if id == d.RequestID {
				c.order[i] = e.RequestID
				break
			}
		}
		d.RequestID = e.RequestID
		d.URL = redactString(e.URL, sens)
		c.byID[e.RequestID] = d
		return
	}

	// Data-driven expansion: the first iteration replaces the planned seed row.
	for _, id := range c.order {
		d := c.byID[id]
		if d != nil && d.dataDrivenSeed && strings.HasPrefix(e.RequestSlug, d.Slug+"-") {
			c.removeLocked(id)
			c.total-- // replaced by per-iteration rows counted as they start
			break
		}
	}

	id := e.RequestID
	if id == "" {
		id = syntheticID(e.RequestSlug)
	}
	if _, exists := c.byID[id]; exists {
		return
	}
	d := &RequestDetail{
		RequestID: id,
		Slug:      e.RequestSlug,
		Name:      e.Name,
		Phase:     e.Phase,
		Method:    e.Method,
		URL:       redactString(e.URL, sens),
		WaveIndex: -1,
		Source:    &sourceJSON{File: c.relPath(e.SourceFile), Line: e.SourceLine},
	}
	c.byID[id] = d
	c.order = append(c.order, id)
	c.total++
}

func (c *DetailCollector) removeLocked(id string) {
	delete(c.byID, id)
	for i, oid := range c.order {
		if oid == id {
			c.order = append(c.order[:i], c.order[i+1:]...)
			break
		}
	}
}

// AssertionResult implements runner.EventSink.
func (c *DetailCollector) AssertionResult(e runner.AssertionEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	d := c.byID[e.RequestID]
	if d == nil {
		return
	}
	if d.Assertions == nil {
		d.Assertions = &assertionsJSON{Passed: true, Items: []assertionItemJSON{}}
	}
	d.Assertions.Items = append(d.Assertions.Items, assertionItemJSON{
		Type: e.Type, Expected: e.Expected, Actual: e.Actual, Passed: e.Passed,
	})
	if !e.Passed {
		d.Assertions.Passed = false
		if d.failMessage == "" {
			d.failMessage = fmt.Sprintf("%s: expected %s, got %s", e.Type, e.Expected, e.Actual)
		}
	}
}

// RequestEnd implements runner.EventSink: stores the redacted full bodies,
// redacted headers, timing, and outcome — this is what makes the inspector
// work while the run is still going (spec §6.2).
func (c *DetailCollector) RequestEnd(e runner.RequestEndEvent) {
	c.mu.Lock()
	defer c.mu.Unlock()
	sens := c.sensitive

	id := e.RequestID
	if id == "" {
		id = syntheticID(e.RequestSlug)
	}
	d := c.byID[id]
	if d == nil {
		// request.end without a start (early skip paths): synthesize a row.
		d = &RequestDetail{RequestID: id, Slug: e.RequestSlug, Phase: "main", WaveIndex: -1}
		c.byID[id] = d
		c.order = append(c.order, id)
		c.total++
	}
	outcome := e.Outcome
	d.Outcome = &outcome
	d.StatusCode = e.StatusCode
	d.DurationMs = e.Duration.Milliseconds()
	if e.WaveIndex != 0 || d.WaveIndex == 0 {
		d.WaveIndex = e.WaveIndex
	}
	d.Timing = timingJSONFromExec(e.Timing, e.Attempts)
	if e.Attempts > 1 {
		d.retryCount = e.Attempts - 1
	}
	if outcome == "skipped" {
		d.Skipped = true
	}
	if outcome == "error" && e.Err != nil {
		d.Error = errorJSONFrom(e.Err)
	}
	if len(e.RequestHeaders) > 0 || d.Request == nil {
		d.Request = &requestPayloadJSON{
			Headers: variable.RedactHeaders(e.RequestHeaders, sens, false),
		}
	}
	if reqBody := redactBytes(e.RequestBody, sens); reqBody != nil {
		d.rawRequestBody = reqBody
		d.Request.Body = makeBodyJSON(reqBody, headerGet(e.RequestHeaders, "Content-Type"))
	}
	if e.ResponseHeaders != nil || e.ResponseBody != nil {
		respHeaders := redactHTTPHeaders(e.ResponseHeaders, sens)
		respBody := redactBytes(e.ResponseBody, sens)
		d.rawResponseBody = respBody
		d.Response = &responsePayloadJSON{
			Headers: respHeaders,
			Body:    makeBodyJSON(respBody, e.ResponseHeaders.Get("Content-Type")),
		}
	}
}

// Reconcile overwrites collector entries from the authoritative results,
// picking up the post-run Sensitive set (incl. Auth/RuntimeSensitive),
// WaveIndex, retry details, iteration metadata, and skip cascades (spec §6.3).
// prefix is the batch-run request-id prefix for this collection ("" single).
func (c *DetailCollector) Reconcile(results []runner.RequestResult, sensitive *variable.SensitiveSet) {
	c.mu.Lock()
	defer c.mu.Unlock()
	for i := range results {
		rr := &results[i]
		id := rr.RequestID
		if id == "" {
			id = syntheticID(rr.RequestSlug)
		}
		d := c.byID[id]
		if d == nil {
			// Entry unseen mid-run (e.g. skipped without events): create it.
			d = &RequestDetail{RequestID: id}
			c.byID[id] = d
			c.order = append(c.order, id)
		}
		c.fillFromResult(d, rr, sensitive)
	}
	// Drop never-started planned seeds that the authoritative results do not
	// know (e.g. a cancelled run's unreached items keep their pending rows
	// only if the runner reported them as skipped; otherwise remove).
	for _, id := range append([]string(nil), c.order...) {
		d := c.byID[id]
		if d != nil && d.Outcome == nil && strings.HasPrefix(d.RequestID, "slug:") {
			found := false
			for i := range results {
				if results[i].RequestSlug == d.Slug {
					found = true
					break
				}
			}
			if !found {
				c.removeLocked(id)
				c.total--
			}
		}
	}
}

// fillFromResult populates d from the authoritative RequestResult.
func (c *DetailCollector) fillFromResult(d *RequestDetail, rr *runner.RequestResult, sens *variable.SensitiveSet) {
	d.Slug = rr.RequestSlug
	d.Name = rr.Name
	if rr.Phase != "" {
		d.Phase = string(rr.Phase)
	} else if d.Phase == "" {
		d.Phase = "main"
	}
	if rr.Method != "" {
		d.Method = rr.Method
	}
	if rr.URL != "" {
		d.URL = redactString(rr.URL, sens)
	}
	outcome := outcomeOf(rr)
	d.Outcome = &outcome
	d.Skipped = rr.Skipped
	if rr.SkipReason != "" {
		reason := rr.SkipReason
		d.SkipReason = &reason
	}
	d.Warnings = rr.Warnings
	d.WaveIndex = rr.WaveIndex
	d.retryCount = rr.RetryCount
	if rr.RetryCount > 0 || len(rr.AttemptDetails) > 0 {
		retry := &retryJSON{Count: rr.RetryCount, Warnings: rr.RetryWarnings, Attempts: []attemptJSON{}}
		if retry.Warnings == nil {
			retry.Warnings = []string{}
		}
		for _, a := range rr.AttemptDetails {
			var errStr *string
			if a.Err != nil {
				s := a.Err.Error()
				errStr = &s
			}
			retry.Attempts = append(retry.Attempts, attemptJSON{
				Number:     a.Number,
				StatusCode: a.StatusCode,
				DurationMs: a.Duration.Milliseconds(),
				DelayMs:    a.Delay.Milliseconds(),
				Error:      errStr,
			})
		}
		d.Retry = retry
	}
	if rr.IsDataDriven {
		baseSlug := d.Slug
		if bs, err := sluggify(rr.DataDrivenName); err == nil {
			baseSlug = bs
		}
		d.Iteration = &iterationJSON{
			Index:    rr.IterationIndex,
			Total:    rr.IterationTotal,
			BaseName: rr.DataDrivenName,
			BaseSlug: baseSlug,
		}
	}
	if d.Source == nil {
		d.Source = &sourceJSON{}
	}
	d.Source.File = c.relPath(rr.SourceFile)
	d.Source.Line = rr.SourceLine
	c.fillSnippet(d.Source)

	if rr.RequestHeaders != nil {
		if d.Request == nil {
			d.Request = &requestPayloadJSON{}
		}
		d.Request.Headers = variable.RedactHeaders(rr.RequestHeaders, sens, false)
	}
	if rr.RequestBody != nil {
		raw := bodyToBytes(rr.RequestBody)
		raw = redactBytes(raw, sens)
		if d.Request == nil {
			d.Request = &requestPayloadJSON{Headers: map[string]string{}}
		}
		d.rawRequestBody = raw
		d.Request.Body = makeBodyJSON(raw, headerGet(rr.RequestHeaders, "Content-Type"))
	}
	if rr.Result != nil {
		d.StatusCode = rr.Result.StatusCode
		d.DurationMs = rr.Result.Duration.Milliseconds()
		raw := redactBytes(rr.Result.Body, sens)
		d.rawResponseBody = raw
		d.Response = &responsePayloadJSON{
			Headers: redactHTTPHeaders(rr.Result.Headers, sens),
			Body:    makeBodyJSON(raw, rr.Result.Headers.Get("Content-Type")),
		}
		d.Timing = timingJSONFromExec(rr.Result.Timing, rr.RetryCount+1)
	}
	if rr.AssertionResults != nil {
		aj := &assertionsJSON{Passed: rr.AssertionResults.Passed, Items: []assertionItemJSON{}}
		for _, item := range rr.AssertionResults.Items {
			aj.Items = append(aj.Items, assertionItemJSON{
				Type: item.Type, Expected: item.Expected, Actual: item.Actual, Passed: item.Passed,
			})
			if !item.Passed && d.failMessage == "" {
				d.failMessage = fmt.Sprintf("%s: expected %s, got %s", item.Type, item.Expected, item.Actual)
			}
		}
		d.Assertions = aj
	}
	if rr.Err != nil && outcome == "error" {
		d.Error = errorJSONFrom(rr.Err)
	}
}

// fillSnippet reads up to 5 lines of the source file starting at Line.
func (c *DetailCollector) fillSnippet(src *sourceJSON) {
	if src.File == "" || src.Line <= 0 {
		return
	}
	data, err := os.ReadFile(filepath.Join(c.root, src.File))
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	start := src.Line // 1-based
	if start > len(lines) {
		return
	}
	end := start + 4
	if end > len(lines) {
		end = len(lines)
	}
	src.Snippet = lines[start-1 : end]
	src.SnippetStartLine = start
}

// List renders the §4.8 light list in display order.
func (c *DetailCollector) List() []requestListEntry {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]requestListEntry, 0, len(c.order))
	for _, id := range c.order {
		d := c.byID[id]
		if d == nil {
			continue
		}
		entry := requestListEntry{
			RequestID:  d.RequestID,
			Slug:       d.Slug,
			Name:       d.Name,
			Phase:      d.Phase,
			Method:     d.Method,
			Outcome:    d.Outcome,
			StatusCode: d.StatusCode,
			DurationMs: d.DurationMs,
			WaveIndex:  d.WaveIndex,
			RetryCount: d.retryCount,
			SkipReason: d.SkipReason,
			Iteration:  d.Iteration,
		}
		if d.Source != nil {
			entry.SourceFile = d.Source.File
			entry.SourceLine = d.Source.Line
		}
		if d.failMessage != "" {
			fm := d.failMessage
			entry.FailMsg = &fm
		}
		entry.Error = d.Error.compact()
		out = append(out, entry)
	}
	return out
}

// Get returns the detail for a request id (or nil).
func (c *DetailCollector) Get(id string) *RequestDetail {
	c.mu.RLock()
	defer c.mu.RUnlock()
	return c.byID[id]
}

// RawBody returns the full redacted body bytes for the /body endpoint.
func (c *DetailCollector) RawBody(id, which string) ([]byte, *bodyJSON, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	d := c.byID[id]
	if d == nil {
		return nil, nil, false
	}
	switch which {
	case "request":
		if d.Request == nil || d.Request.Body == nil {
			return nil, nil, false
		}
		return d.rawRequestBody, d.Request.Body, true
	case "response":
		if d.Response == nil || d.Response.Body == nil {
			return nil, nil, false
		}
		return d.rawResponseBody, d.Response.Body, true
	}
	return nil, nil, false
}

// Details returns all details in display order (for persistence/compare).
func (c *DetailCollector) Details() []*RequestDetail {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]*RequestDetail, 0, len(c.order))
	for _, id := range c.order {
		if d := c.byID[id]; d != nil {
			out = append(out, d)
		}
	}
	return out
}

// Progress returns §4.8 progress counts for /runs/current.
func (c *DetailCollector) Progress() (total, passed, failed, skipped, errored, completed int) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	total = len(c.order)
	for _, id := range c.order {
		d := c.byID[id]
		if d == nil || d.Outcome == nil {
			continue
		}
		completed++
		switch *d.Outcome {
		case "passed":
			passed++
		case "failed":
			failed++
		case "skipped":
			skipped++
		case "error":
			errored++
		}
	}
	return total, passed, failed, skipped, errored, completed
}

// --- helpers ---

func outcomeOf(rr *runner.RequestResult) string {
	switch {
	case rr.Skipped:
		return "skipped"
	case rr.Err != nil:
		return "error"
	case rr.AssertionResults != nil && !rr.AssertionResults.Passed:
		return "failed"
	default:
		return "passed"
	}
}

func headerGet(h map[string]string, key string) string {
	for k, v := range h {
		if strings.EqualFold(k, key) {
			return v
		}
	}
	return ""
}

func bodyToBytes(body any) []byte {
	switch b := body.(type) {
	case nil:
		return nil
	case []byte:
		return b
	case string:
		return []byte(b)
	default:
		if encoded, err := json.Marshal(body); err == nil {
			return encoded
		}
		return nil
	}
}

func redactBytes(raw []byte, sens *variable.SensitiveSet) []byte {
	if raw == nil {
		return nil
	}
	if rb, ok := variable.RedactBody(raw, sens, false).([]byte); ok {
		return rb
	}
	return raw
}

func redactString(s string, sens *variable.SensitiveSet) string {
	if rb, ok := variable.RedactBody(s, sens, false).(string); ok {
		return rb
	}
	return s
}

// redactHTTPHeaders applies the same name- and value-based redaction the CLI
// formatters use, preserving multi-value headers one value at a time.
func redactHTTPHeaders(h http.Header, sens *variable.SensitiveSet) http.Header {
	if h == nil {
		return nil
	}
	out := make(http.Header, len(h))
	for k, vals := range h {
		for _, v := range vals {
			red := variable.RedactHeaders(map[string]string{k: v}, sens, false)
			out[k] = append(out[k], red[k])
		}
	}
	return out
}

func isBinaryBody(raw []byte) bool {
	for _, b := range raw {
		if b == 0x00 {
			return true
		}
	}
	return !utf8.Valid(raw)
}

// makeBodyJSON applies the §4.9 inline policy: inline ≤ 256 KiB (base64 for
// binary), otherwise content omitted with truncated:true.
func makeBodyJSON(raw []byte, contentType string) *bodyJSON {
	if raw == nil {
		return nil
	}
	b := &bodyJSON{Size: len(raw), ContentType: contentType}
	if len(raw) > InlineBodyLimit {
		b.Truncated = true
		return b
	}
	if isBinaryBody(raw) {
		b.Encoding = "base64"
		b.Content = base64.StdEncoding.EncodeToString(raw)
		return b
	}
	b.Content = string(raw)
	return b
}

// sluggify mirrors parser.Slug for base-slug recovery on iterations.
func sluggify(name string) (string, error) {
	return parser.Slug(name)
}
