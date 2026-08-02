package prcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

const (
	maxRetries    = 2
	retryInterval = 200 * time.Millisecond
)

// Client posts results and PR check status to the backend.
type Client struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client // nil = http.DefaultClient
}

// UploadResults POSTs the results payload and returns the result ID.
func (c *Client) UploadResults(ctx context.Context, org string, payload *ResultsPayload) (*IngestResponse, error) {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/results", c.BaseURL, org)
	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshalling results: %w", err)
	}
	respBody, err := c.doWithRetry(ctx, "POST", url, body)
	if err != nil {
		return nil, err
	}
	var resp IngestResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing ingest response: %w", err)
	}
	return &resp, nil
}

// PostPrCheck POSTs the PR check status payload.
func (c *Client) PostPrCheck(ctx context.Context, org string, payload *PrCheckPayload) error {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/pr-checks", c.BaseURL, org)
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshalling pr-check: %w", err)
	}
	_, err = c.doWithRetry(ctx, "POST", url, body)
	return err
}

// doWithRetry sends an HTTP request with up to maxRetries retries on
// connection errors. Returns ErrUnauthorized on 401, ErrNetworkFailure
// after all retries are exhausted.
func (c *Client) doWithRetry(ctx context.Context, method, url string, body []byte) ([]byte, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	var lastErr error
	for attempt := 0; attempt <= maxRetries; attempt++ {
		if attempt > 0 {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(retryInterval):
			}
		}
		req, err := http.NewRequestWithContext(ctx, method, url, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("creating request: %w", err)
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
			return nil, ErrUnauthorized
		}
		if resp.StatusCode >= 200 && resp.StatusCode < 300 {
			return respBody, nil
		}
		return nil, fmt.Errorf("backend returned HTTP %d: %s", resp.StatusCode, string(respBody))
	}
	return nil, fmt.Errorf("%w: %v", ErrNetworkFailure, lastErr)
}
