// Package events defines the NDJSON agent event stream emitted by `curlew run`
// when the --events flag is provided. The current schema is documented in
// docs/events-schema/v1.3.json. v1.2, v1.1 and v1.0 are retained at their
// respective paths as historical anchors.
//
// v1.0 → v1.1 (M8-004): additive change — run.start gains an optional
// "selection" field carrying the --only values for the run.
//
// v1.1 → v1.2 (M9-001): additive change — request.start and request.end gain
// an optional "request_slug" field carrying the URL-safe slug derived from the
// request name.
//
// v1.2 → v1.3: additive change — request.end gains an optional "timing"
// object carrying the connection-phase breakdown (integer microseconds)
// measured via net/http/httptrace, plus the retry attempt count.
package events

// SchemaVersion is the current event-stream schema version. Promoted from
// "1.2" to "1.3" with an additive optional "timing" object on request.end.
// Removals and renames now require a v2.0 bump.
const SchemaVersion = "1.3"

// DefaultBodyLimit is the byte threshold above which request and response
// bodies are truncated in event payloads.
const DefaultBodyLimit = 2048

// Kind identifies an event's type. Appears in every event's "kind" field.
type Kind string

const (
	KindRunStart        Kind = "run.start"
	KindRunError        Kind = "run.error"
	KindRequestStart    Kind = "request.start"
	KindRequestEnd      Kind = "request.end"
	KindAssertionResult Kind = "assertion.result"
	KindRunEnd          Kind = "run.end"
)

// Outcome is the terminal state of a request.end event.
type Outcome string

const (
	OutcomePassed  Outcome = "passed"
	OutcomeFailed  Outcome = "failed"
	OutcomeSkipped Outcome = "skipped"
	OutcomeError   Outcome = "error"
)

// Header is embedded in every event type. Field order is deliberate: it
// appears first in the serialized JSON in this order.
type Header struct {
	SchemaVersion string `json:"schema_version"`
	RunID         string `json:"run_id"`
	ID            int64  `json:"id"`
	AtMs          int64  `json:"at_ms"`
	Kind          Kind   `json:"kind"`
}

// EventError is the error payload shared by RunError and RequestEnd.
type EventError struct {
	Category string `json:"category"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
	File     string `json:"file,omitempty"`
	Line     int    `json:"line,omitempty"`
}

// RunStart is the first event emitted for every run.
type RunStart struct {
	Header
	StartedAt      string   `json:"started_at"` // RFC3339Nano UTC
	CurlewVersion  string   `json:"curlew_version"`
	CLIArgs        []string `json:"cli_args"`
	CollectionFile string   `json:"collection_file,omitempty"`
	EnvName        string   `json:"env_name,omitempty"`
	// Selection carries the --only values for this run (M8-004). Omitted when
	// --only was not supplied. Each entry is a main request name (case-sensitive).
	Selection []string `json:"selection,omitempty"`
}

// RunError is emitted when the run fails before normal completion.
type RunError struct {
	Header
	Error EventError `json:"error"`
}

// RequestStart marks the beginning of a single request execution.
type RequestStart struct {
	Header
	RequestID   string `json:"request_id"`
	RequestSlug string `json:"request_slug,omitempty"` // M9-001 v1.2 additive
	Name        string `json:"name,omitempty"`
	Method      string `json:"method"`
	URL         string `json:"url"`
	Phase       string `json:"phase,omitempty"`
	SourceFile  string `json:"source_file,omitempty"`
	SourceLine  int    `json:"source_line,omitempty"`
}

// RequestEnd marks the end of a single request execution.
type RequestEnd struct {
	Header
	RequestID             string      `json:"request_id"`
	RequestSlug           string      `json:"request_slug,omitempty"` // M9-001 v1.2 additive
	Outcome               Outcome     `json:"outcome"`
	StatusCode            int         `json:"status_code,omitempty"`
	DurationMs            int64       `json:"duration_ms"`
	WaveIndex             int         `json:"wave_index,omitempty"`
	RequestBody           string      `json:"request_body,omitempty"`
	RequestBodySize       int         `json:"request_body_size,omitempty"`
	RequestBodyTruncated  bool        `json:"request_body_truncated,omitempty"`
	RequestBodyEncoding   string      `json:"request_body_encoding,omitempty"` // "base64" when binary
	ResponseBody          string      `json:"response_body,omitempty"`
	ResponseBodySize      int         `json:"response_body_size,omitempty"`
	ResponseBodyTruncated bool        `json:"response_body_truncated,omitempty"`
	ResponseBodyEncoding  string      `json:"response_body_encoding,omitempty"` // "base64" when binary
	Timing                *TimingInfo `json:"timing,omitempty"`                 // v1.3 additive
	Error                 *EventError `json:"error,omitempty"`
}

// TimingInfo is the optional request.end connection-phase breakdown. Added in v1.3.
// All duration fields are integer microseconds. Phase fields are omitted when the
// phase did not occur (e.g. pooled connection: no dns/connect/tls).
type TimingInfo struct {
	DNSUs      *int64 `json:"dns_us,omitempty"`
	ConnectUs  *int64 `json:"connect_us,omitempty"`
	TLSUs      *int64 `json:"tls_us,omitempty"`
	TTFBUs     *int64 `json:"ttfb_us,omitempty"`
	DownloadUs *int64 `json:"download_us,omitempty"`
	TotalUs    int64  `json:"total_us"`
	Reused     bool   `json:"connection_reused,omitempty"`
	Attempts   int    `json:"attempts,omitempty"` // >1 only when retries occurred
}

// AssertionResult records one assertion outcome.
type AssertionResult struct {
	Header
	RequestID string `json:"request_id"`
	Type      string `json:"type"` // "status" | "body" | "header" | "schema"
	Passed    bool   `json:"passed"`
	Expected  string `json:"expected,omitempty"`
	Actual    string `json:"actual,omitempty"`
}

// RunEnd is the terminal event for every run.
type RunEnd struct {
	Header
	DurationMs int64 `json:"duration_ms"`
	Total      int   `json:"total"`
	Passed     int   `json:"passed"`
	Failed     int   `json:"failed"`
	Skipped    int   `json:"skipped"`
	ExitCode   int   `json:"exit_code"`
	EventCount int64 `json:"event_count"`
}
