package backend

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Sentinel errors for transport-level failures. Callers Is-check these
// before falling through to ProblemDetails decoding.
var (
	// ErrNetworkFailure is returned when the backend is unreachable. Mapped
	// to CLI exit code 3 by the apitest license --refresh handler.
	ErrNetworkFailure = errors.New("backend: network failure")
	// ErrServerError is returned when the backend responds with 5xx after
	// problem-details decoding fails (i.e., 5xx without a proper RFC 7807
	// body). Mapped to CLI exit code 6.
	ErrServerError = errors.New("backend: server error")
)

// Client is the apitest CLI's HTTP client for the apitool backend. The
// zero value is not usable; construct via NewClient. Safe for concurrent
// use — every method creates a fresh *http.Request and reads the response
// body to completion before returning.
type Client struct {
	baseURL    string
	httpClient *http.Client
	userAgent  string
}

// Options configure a Client.
type Options struct {
	BaseURL    string        // required, e.g. "https://api.apitool.dev"
	HTTPClient *http.Client  // nil = http.Client{Timeout: 30s}
	UserAgent  string        // empty = "apitest-cli"
	Timeout    time.Duration // applied to default http.Client when HTTPClient is nil
}

// NewClient returns a configured Client. Returns an error if BaseURL is empty.
func NewClient(opts Options) (*Client, error) {
	if opts.BaseURL == "" {
		return nil, errors.New("backend: BaseURL is required")
	}
	hc := opts.HTTPClient
	if hc == nil {
		timeout := opts.Timeout
		if timeout == 0 {
			timeout = 30 * time.Second
		}
		hc = &http.Client{Timeout: timeout}
	}
	ua := opts.UserAgent
	if ua == "" {
		ua = "apitest-cli"
	}
	return &Client{baseURL: opts.BaseURL, httpClient: hc, userAgent: ua}, nil
}

// GetJSON sends a GET request to path with an optional Bearer token and
// decodes a 2xx JSON response into out. On non-2xx responses, attempts to
// decode application/problem+json — the resulting *ProblemDetails is
// wrapped in the returned error so callers can errors.As it.
func (c *Client) GetJSON(ctx context.Context, path, accessToken string, out any) error {
	req, err := c.buildRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return c.do(req, out)
}

// GetJSONOptional sends a GET request like GetJSON, but returns (false, nil)
// when the server responds with HTTP 204 (no content) instead of attempting to
// decode an empty body. On 2xx responses with a body it returns (true, nil) and
// decodes into out. On non-2xx responses it follows the same error-mapping as
// GetJSON.
func (c *Client) GetJSONOptional(ctx context.Context, path, accessToken string, out any) (found bool, err error) {
	req, err := c.buildRequest(ctx, http.MethodGet, path, nil)
	if err != nil {
		return false, err
	}
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, fmt.Errorf("%w: %v", ErrNetworkFailure, err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, resp.Body)
		return false, nil
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		if out == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			return true, nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return false, fmt.Errorf("decode response: %w", err)
		}
		return true, nil
	}
	// Non-2xx: try problem+json first.
	pd, perr := DecodeProblem(resp)
	if perr == nil {
		return false, pd
	}
	if !errors.Is(perr, ErrNotProblem) {
		return false, perr
	}
	if resp.StatusCode >= 500 {
		return false, fmt.Errorf("%w: HTTP %d", ErrServerError, resp.StatusCode)
	}
	return false, fmt.Errorf("backend: HTTP %d", resp.StatusCode)
}

// PostJSON sends a POST with a JSON body. The accessToken is set as a
// Bearer header iff non-empty; refresh tokens MUST be passed inside body
// (never as accessToken) per SPECIFICATION.md:8195-8202.
func (c *Client) PostJSON(ctx context.Context, path, accessToken string, body, out any) error {
	var buf bytes.Buffer
	if body != nil {
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return fmt.Errorf("encode body: %w", err)
		}
	}
	req, err := c.buildRequest(ctx, http.MethodPost, path, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	if accessToken != "" {
		req.Header.Set("Authorization", "Bearer "+accessToken)
	}
	return c.do(req, out)
}

// BuildRefreshRequest is exported for the reflection-based behaviour-test
// (#6) that asserts refresh tokens never appear in headers. It returns a
// fully-formed *http.Request whose body contains the refresh-token JSON
// payload, with NO Authorization header — the request body is the only
// place the refresh_token is carried per SPECIFICATION.md:8195-8202.
func (c *Client) BuildRefreshRequest(ctx context.Context, refreshToken, deviceID string) (*http.Request, error) {
	payload := map[string]string{"refresh_token": refreshToken, "device_id": deviceID}
	var buf bytes.Buffer
	if err := json.NewEncoder(&buf).Encode(payload); err != nil {
		return nil, fmt.Errorf("encode refresh payload: %w", err)
	}
	req, err := c.buildRequest(ctx, http.MethodPost, "/api/v1/auth/refresh", &buf)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Deliberately no Authorization header. Refresh tokens are body-only.
	return req, nil
}

// do executes req, decodes a 2xx body into out (when non-nil), and on
// non-2xx attempts ProblemDetails decoding before falling through to
// ErrServerError. Networking errors become ErrNetworkFailure.
func (c *Client) do(req *http.Request, out any) error {
	req.Header.Set("User-Agent", c.userAgent)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrNetworkFailure, err)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		defer func() { _ = resp.Body.Close() }()
		if out == nil {
			_, _ = io.Copy(io.Discard, resp.Body)
			return nil
		}
		if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
			return fmt.Errorf("decode response: %w", err)
		}
		return nil
	}
	// Non-2xx: try problem+json first.
	pd, perr := DecodeProblem(resp)
	if perr == nil {
		return pd // *ProblemDetails implements error
	}
	if !errors.Is(perr, ErrNotProblem) {
		return perr // malformed problem+json
	}
	if resp.StatusCode >= 500 {
		return fmt.Errorf("%w: HTTP %d", ErrServerError, resp.StatusCode)
	}
	return fmt.Errorf("backend: HTTP %d", resp.StatusCode)
}

func (c *Client) buildRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	url := c.baseURL + path
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	return req, nil
}
