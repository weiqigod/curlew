package telemetry

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// Client posts telemetry events to the ingest endpoint. Dedicated to
// internal/telemetry (does NOT reuse internal/backend) per v4-11.
type Client struct {
	httpClient *http.Client
	endpoint   string
}

// ClientOptions configures a Client.
type ClientOptions struct {
	// Endpoint is the URL to POST events to.
	// Defaults to defaultEndpoint when empty.
	Endpoint string
	// Timeout is the per-request deadline. Defaults to 2 seconds.
	Timeout time.Duration
	// HTTPClient overrides the default http.Client (for testing).
	HTTPClient *http.Client
}

// NewClient builds a Client from opts.
func NewClient(opts ClientOptions) *Client {
	ep := opts.Endpoint
	if ep == "" {
		ep = defaultEndpoint
	}
	timeout := opts.Timeout
	if timeout <= 0 {
		timeout = 2 * time.Second
	}
	hc := opts.HTTPClient
	if hc == nil {
		hc = &http.Client{Timeout: timeout}
	}
	return &Client{httpClient: hc, endpoint: ep}
}

// Emit posts one event. Errors are returned so callers can record them in the
// ring buffer; callers MUST NOT surface them to the user (per v4-9 :11479).
// Returns nil on a 200 OK, 202 Accepted, or 429 Too Many Requests.
func (c *Client) Emit(ctx context.Context, installID, eventType string, payload map[string]any) error {
	if payload == nil {
		payload = map[string]any{}
	}
	body, err := json.Marshal(map[string]any{
		"install_id":    installID,
		"event_type":    eventType,
		"event_payload": payload,
	})
	if err != nil {
		return fmt.Errorf("telemetry: marshal: %w", err)
	}
	idemKey, err := newUUIDv4()
	if err != nil {
		return fmt.Errorf("telemetry: gen idempotency key: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.endpoint, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("telemetry: build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", idemKey)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("telemetry: post: %w", err)
	}
	defer resp.Body.Close() //nolint:errcheck
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusAccepted, http.StatusTooManyRequests:
		// 200, 201, 202 = success; 429 = non-fatal per spec :11479.
		return nil
	}
	return fmt.Errorf("telemetry: unexpected status %d", resp.StatusCode)
}
