package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"
)

const (
	defaultMaxRetries  = 3
	defaultBackoffBase = 200 * time.Millisecond
)

// Client posts coordinator API calls and returns parsed responses.
type Client struct {
	BaseURL     string
	Token       string
	HTTPClient  *http.Client  // nil = http.DefaultClient
	MaxRetries  int           // 0 = defaultMaxRetries
	BackoffBase time.Duration // 0 = defaultBackoffBase
	LogWriter   io.Writer     // nil = os.Stderr; receives per-retry warning lines
}

// logWriter returns the configured log destination or os.Stderr.
func (c *Client) logWriter() io.Writer {
	if c.LogWriter != nil {
		return c.LogWriter
	}
	return os.Stderr
}

// Claim attempts to claim a shard for the given job. Returns (nil, nil) when
// the coordinator has no shards available (HTTP 204).
func (c *Client) Claim(ctx context.Context, org, jobID string, body *ClaimRequestBody) (*ShardResponse, error) {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/coordinator/jobs/%s/claim", c.BaseURL, org, jobID)
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling claim request: %w", err)
	}

	respBody, status, err := c.doWithRetry(ctx, "POST", url, data)
	if err != nil {
		return nil, err
	}
	if status == 204 {
		return nil, nil
	}

	var shard ShardResponse
	if err := json.Unmarshal(respBody, &shard); err != nil {
		return nil, fmt.Errorf("parsing claim response: %w", err)
	}
	return &shard, nil
}

// SubmitResult posts the per-shard outcomes. Returns nil on 202 Accepted.
// Retries network failures with exponential backoff.
func (c *Client) SubmitResult(ctx context.Context, org, jobID, shardID string, body *SubmitResultBody) error {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/coordinator/jobs/%s/shards/%s/result", c.BaseURL, org, jobID, shardID)
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling submit request: %w", err)
	}

	_, _, err = c.doWithRetry(ctx, "POST", url, data)
	return err
}

// Heartbeat extends the shard's lease. Returns nil on 204 No Content.
func (c *Client) Heartbeat(ctx context.Context, org, jobID, shardID string, body *HeartbeatBody) error {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/coordinator/jobs/%s/shards/%s/heartbeat", c.BaseURL, org, jobID, shardID)
	data, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshalling heartbeat request: %w", err)
	}

	_, _, err = c.doWithRetry(ctx, "POST", url, data)
	return err
}

// doWithRetry sends an HTTP request and retries on 5xx or connection errors.
// Returns (responseBody, statusCode, error).
// Maps 401 → ErrUnauthorized (no retry).
// Maps other 4xx → non-retriable error.
// Maps 5xx + network errors → retry with exponential backoff, logging a warning per attempt.
// After all retries exhausted → ErrNetworkExhausted.
func (c *Client) doWithRetry(ctx context.Context, method, url string, body []byte) ([]byte, int, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	maxRetries := c.MaxRetries
	if maxRetries == 0 {
		maxRetries = defaultMaxRetries
	}
	backoffBase := c.BackoffBase
	if backoffBase == 0 {
		backoffBase = defaultBackoffBase
	}

	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			backoff := backoffBase * (1 << uint(attempt-1))
			_, _ = fmt.Fprintf(c.logWriter(), "warning: retrying %s (attempt %d/%d): %v\n", url, attempt, maxRetries, lastErr)
			select {
			case <-ctx.Done():
				return nil, 0, ctx.Err()
			case <-time.After(backoff):
			}
		}

		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, 0, fmt.Errorf("creating request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+c.Token)

		resp, err := httpClient.Do(req)
		if err != nil {
			lastErr = err
			continue // retry on connection error
		}
		respBody, readErr := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if readErr != nil {
			lastErr = readErr
			continue
		}

		if resp.StatusCode == 401 {
			return nil, 401, ErrUnauthorized
		}
		if resp.StatusCode >= 500 {
			lastErr = fmt.Errorf("backend returned HTTP %d", resp.StatusCode)
			continue // retry on 5xx
		}
		if resp.StatusCode >= 400 {
			return nil, resp.StatusCode, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, string(respBody))
		}
		return respBody, resp.StatusCode, nil
	}
	return nil, 0, fmt.Errorf("%w: %v", ErrNetworkExhausted, lastErr)
}
