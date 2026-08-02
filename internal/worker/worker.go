// Package worker implements the client side of the distributed-execution
// coordinator protocol (see internal/coordinator on the backend, M5-008).
//
// A worker authenticates with a bearer token, polls the coordinator for
// shards to claim, executes each shard's HTTP requests, submits per-shard
// outcomes, and exits when no shards remain.
package worker

import (
	"errors"
	"time"
)

// Sentinel errors for well-known failure modes.
var (
	// ErrCoordinatorURLMissing is returned when neither --coordinator-url
	// nor APITEST_COORDINATOR_URL is set.
	ErrCoordinatorURLMissing = errors.New("coordinator URL not configured (set APITEST_COORDINATOR_URL or --coordinator-url)")
	// ErrTokenMissing is returned when APITEST_BACKEND_TOKEN/--token is empty.
	ErrTokenMissing = errors.New("backend token not configured (set APITEST_BACKEND_TOKEN or --token)")
	// ErrUnauthorized is returned when the coordinator responds with 401.
	ErrUnauthorized = errors.New("unauthorized")
	// ErrNetworkExhausted is returned when retries on a coordinator call are exhausted.
	ErrNetworkExhausted = errors.New("network error: coordinator unreachable after retries")
)

// Config holds the worker configuration derived from flags and env vars.
type Config struct {
	CoordinatorURL    string        // APITEST_COORDINATOR_URL / --coordinator-url
	Token             string        // APITEST_BACKEND_TOKEN / --token
	JobID             string        // --job (job_<hex>)
	Org               string        // --org (slug or org_<hex>)
	WorkerID          string        // --worker-id (defaults to "wkr_" + hostname + pid + nanos)
	Concurrency       int           // --concurrency (default 1)
	HeartbeatInterval time.Duration // --heartbeat-interval (default 15s)
}

// Validate returns an error if any required Config field is missing.
func (c *Config) Validate() error {
	if c.CoordinatorURL == "" {
		return ErrCoordinatorURLMissing
	}
	if c.Token == "" {
		return ErrTokenMissing
	}
	if c.JobID == "" {
		return errors.New("--job is required")
	}
	if c.Org == "" {
		return errors.New("--org is required")
	}
	if c.Concurrency < 1 {
		return errors.New("--concurrency must be >= 1")
	}
	return nil
}

// WorkerRequest is the per-request HTTP spec inside a shard's RequestsJson.
type WorkerRequest struct {
	Name    string            `json:"name,omitempty"`
	Method  string            `json:"method"`
	URL     string            `json:"url"`
	Headers map[string]string `json:"headers,omitempty"`
	Body    any               `json:"body,omitempty"`
}

// ShardResponse mirrors the backend CoordinatorShardDto.
type ShardResponse struct {
	ShardID        string `json:"shard_id"`
	JobID          string `json:"job_id"`
	ShardIndex     int    `json:"shard_index"`
	State          string `json:"state"`
	AssignedWorker string `json:"assigned_worker,omitempty"`
	RequestsJson   string `json:"requests_json"`
}

// SubmitItem mirrors backend SubmitResultRequest.Items[].
type SubmitItem struct {
	Name       string `json:"name"`
	Status     string `json:"status"` // "pass" | "fail" | "error"
	DurationMs int64  `json:"duration_ms"`
	Message    string `json:"message,omitempty"`
}

// SubmitResultBody is the JSON body for POST /shards/{id}/result.
type SubmitResultBody struct {
	WorkerID   string       `json:"worker_id"`
	PassCount  int          `json:"pass_count"`
	FailCount  int          `json:"fail_count"`
	DurationMs int64        `json:"duration_ms"`
	Items      []SubmitItem `json:"items"`
}

// ClaimRequestBody is the JSON body for POST /jobs/{id}/claim.
type ClaimRequestBody struct {
	WorkerID     string   `json:"worker_id"`
	Capabilities []string `json:"capabilities,omitempty"`
}

// HeartbeatBody is the JSON body for POST /shards/{id}/heartbeat.
type HeartbeatBody struct {
	WorkerID string `json:"worker_id"`
}
