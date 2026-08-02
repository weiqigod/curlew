package output

import (
	"encoding/json"
	"io"
)

// JSONOutput is the top-level JSON output structure for --format json.
type JSONOutput struct {
	Name              string                 `json:"name"`
	Status            string                 `json:"status"`
	DurationMs        int64                  `json:"duration_ms"`
	Summary           *SummaryJSON           `json:"summary"` // always populated; see buildSummaryJSON in cmd/curlew/main.go
	Requests          []JSONRequest          `json:"requests"`
	Errors            []JSONError            `json:"errors,omitempty"`
	GuardRail         *GuardRailJSON         `json:"guard_rail,omitempty"`
	Impact            []JSONImpact           `json:"impact,omitempty"`
	ParallelExecution *ParallelExecutionJSON `json:"parallel_execution,omitempty"`
	DataDriven        []DataDrivenJSON       `json:"data_driven,omitempty"`
}

// SummaryJSON is the root execution summary block. Populated unconditionally
// by buildJSONOutput so consumers can rely on `jq '.summary.passed'` without
// traversing requests[]. (M11-002)
type SummaryJSON struct {
	Total   int `json:"total"`
	Passed  int `json:"passed"`
	Failed  int `json:"failed"`
	Skipped int `json:"skipped"`
}

// DataDrivenJSON holds data-driven aggregate metadata in JSON output.
type DataDrivenJSON struct {
	Type             string                    `json:"type"` // always "data_driven"
	Name             string                    `json:"name"` // base request name
	TotalIterations  int                       `json:"total_iterations"`
	PassedIterations int                       `json:"passed_iterations"`
	FailedIterations int                       `json:"failed_iterations"`
	TotalDurationMs  int64                     `json:"total_duration_ms"`
	AvgDurationMs    int64                     `json:"average_duration_ms"`
	Iterations       []DataDrivenIterationJSON `json:"iterations,omitempty"`
}

// DataDrivenIterationJSON is one iteration entry inside a DataDrivenJSON.
// Surfaces per-iteration visibility that the events stream emits but the
// aggregate-only JSON formerly collapsed. (M11-002)
type DataDrivenIterationJSON struct {
	Name        string            `json:"name"`   // BaseName [i/N]
	Status      string            `json:"status"` // "passed" | "failed" | "skipped"
	DurationMs  int64             `json:"duration_ms"`
	DataColumns map[string]string `json:"data_columns,omitempty"`
}

// ParallelExecutionJSON holds parallel execution metadata in JSON output.
type ParallelExecutionJSON struct {
	WaveCount      int     `json:"wave_count"`
	MaxParallelism int     `json:"max_parallelism"`
	SpeedupFactor  float64 `json:"speedup_factor,omitempty"`
}

// JSONImpact represents an impact analysis entry in JSON output.
type JSONImpact struct {
	FailedRequest string `json:"failed_request"`
	SkippedCount  int    `json:"skipped_count"`
}

// GuardRailJSON represents the guard rail limit in JSON output.
type GuardRailJSON struct {
	LimitExceeded    bool   `json:"limit_exceeded"`
	RequestsExecuted int    `json:"requests_executed"`
	Limit            int    `json:"limit"`
	Message          string `json:"message"`
}

// JSONAttemptDetail represents a single retry attempt in JSON output.
type JSONAttemptDetail struct {
	Attempt    int    `json:"attempt"`
	StatusCode int    `json:"status_code,omitempty"`
	DurationMs int64  `json:"duration_ms"`
	DelayMs    int64  `json:"delay_ms"`
	Error      string `json:"error,omitempty"`
}

// JSONRequest represents one request result in JSON output.
type JSONRequest struct {
	Name            string              `json:"name"`
	Status          string              `json:"status"`
	Method          string              `json:"method"`
	URL             string              `json:"url"`
	StatusCode      int                 `json:"status_code"`
	DurationMs      int64               `json:"duration_ms"`
	RetryCount      int                 `json:"retry_count,omitempty"`
	AttemptDetails  []JSONAttemptDetail `json:"attempt_details,omitempty"`
	WaveIndex       *int                `json:"wave_index,omitempty"`
	SkipReason      string              `json:"skip_reason,omitempty"`
	Assertions      []JSONAssertion     `json:"assertions"` // never omitempty — always []
	Warnings        []string            `json:"warnings,omitempty"`
	Error           *JSONError          `json:"error,omitempty"`
	RequestHeaders  map[string]string   `json:"request_headers,omitempty"`
	ResponseHeaders map[string][]string `json:"response_headers,omitempty"`
	ResponseBody    string              `json:"response_body,omitempty"`
}

// JSONAssertion represents one assertion outcome in JSON output.
type JSONAssertion struct {
	Type     string `json:"type"`
	Operator string `json:"operator"`
	Expected string `json:"expected"`
	Actual   string `json:"actual"`
	Passed   bool   `json:"passed"`
}

// JSONError represents an error in JSON output.
type JSONError struct {
	Message string `json:"message"`
	Hint    string `json:"hint,omitempty"`
}

// ValidationJSONOutput is the top-level JSON structure for validate --format json.
type ValidationJSONOutput struct {
	Files []ValidationFileJSON `json:"files"`
	Valid bool                 `json:"valid"`
}

// ValidationFileJSON represents validation results for one file.
type ValidationFileJSON struct {
	File   string                `json:"file"`
	Valid  bool                  `json:"valid"`
	Issues []ValidationIssueJSON `json:"issues"`
}

// ValidationIssueJSON represents a single validation issue.
type ValidationIssueJSON struct {
	Severity string `json:"severity"` // "error" or "warning"
	Line     int    `json:"line,omitempty"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

// WriteValidationJSON serializes validation output to indented JSON.
func WriteValidationJSON(w io.Writer, out *ValidationJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// InfoJSONOutput is the JSON structure for curlew info --format json.
type InfoJSONOutput struct {
	ProjectRoot  string   `json:"project_root"`
	ProjectName  string   `json:"project_name"`
	Collections  []string `json:"collections"`
	Environments []string `json:"environments"`
	Version      string   `json:"version"`
}

// WriteInfoJSON serializes info output to indented JSON.
func WriteInfoJSON(w io.Writer, out *InfoJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// VaultListJSONOutput is the JSON structure for vault list --format json.
type VaultListJSONOutput struct {
	Provider string         `json:"provider"`
	Keys     []VaultKeyJSON `json:"keys"`
}

// VaultKeyJSON represents a single vault key mapping.
type VaultKeyJSON struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// WriteVaultListJSON serializes vault list output as indented JSON.
func WriteVaultListJSON(w io.Writer, out *VaultListJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// WriteJSON serializes out as indented JSON to w.
func WriteJSON(w io.Writer, out *JSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}

// MultiJSONOutput wraps multiple per-collection JSON outputs into a
// single document produced by `curlew run <glob>`.
type MultiJSONOutput struct {
	Status      string       `json:"status"`
	DurationMs  int64        `json:"duration_ms"`
	Collections []JSONOutput `json:"collections"`
	Total       int          `json:"total_collections"`
	Passed      int          `json:"passed_collections"`
	Failed      int          `json:"failed_collections"`
}

// WriteMultiJSON serializes a MultiJSONOutput as indented JSON to w.
func WriteMultiJSON(w io.Writer, out *MultiJSONOutput) error {
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	return enc.Encode(out)
}
