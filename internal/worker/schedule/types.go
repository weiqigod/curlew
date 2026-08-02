package schedule

import "time"

// NextRunResponse mirrors the .NET NextRunResponse record from the M16-009
// backend. Returned by GET /api/v1/schedules/next-run on HTTP 200.
type NextRunResponse struct {
	RunID         string            `json:"run_id"`
	ScheduleID    string            `json:"schedule_id"`
	CollectionRef string            `json:"collection_ref"`
	EnvVars       map[string]string `json:"env_vars"`
	ClaimToken    string            `json:"claim_token"`
	Deadline      time.Time         `json:"deadline"`
}

// HeartbeatRequest mirrors ScheduleHeartbeatRequest.cs. Sent via
// POST /api/v1/schedules/runs/{id}/heartbeat every HeartbeatInterval.
type HeartbeatRequest struct {
	ClaimToken string `json:"claim_token"`
}

// ResultItem mirrors UploadResultItemRequest.cs. One item per request in the
// executed collection.
type ResultItem struct {
	// Name is the request name from the collection YAML.
	Name string `json:"name,omitempty"`
	// Status is one of "pass", "fail", or "error".
	Status string `json:"status,omitempty"`
	// DurationMs is the wall-clock time in milliseconds.
	DurationMs int64 `json:"duration_ms"`
	// Message is a human-readable description for non-pass statuses.
	Message string `json:"message,omitempty"`
}

// ResultRequest mirrors ScheduleResultRequest.cs. Sent via
// POST /api/v1/schedules/runs/{id}/result after collection execution.
type ResultRequest struct {
	ClaimToken     string       `json:"claim_token"`
	CollectionName string       `json:"collection_name,omitempty"`
	RunAt          time.Time    `json:"run_at"`
	DurationMs     int64        `json:"duration_ms"`
	PassCount      int          `json:"pass_count"`
	FailCount      int          `json:"fail_count"`
	SkippedCount   int          `json:"skipped_count"`
	TriggeredBy    string       `json:"triggered_by,omitempty"` // defaults to "schedule" server-side
	GitSha         string       `json:"git_sha,omitempty"`
	Items          []ResultItem `json:"items,omitempty"`
}
