package schedule

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/weiqigod/curlew/internal/backend"
)

// HTTPClient is the subset of backend.Client used by Client. Exposed
// as an interface for test stubbing without a live HTTP server.
type HTTPClient interface {
	// GetJSONOptional sends a GET request; returns (false, nil) on HTTP 204.
	GetJSONOptional(ctx context.Context, path, accessToken string, out any) (found bool, err error)
	// PostJSON sends a POST request with a JSON body.
	PostJSON(ctx context.Context, path, accessToken string, body, out any) error
}

// Client is the schedule-executor endpoint wrapper. It wraps a backend HTTP
// client and an access token, exposing the three endpoints needed by the
// schedule-pull worker: PollNextRun, Heartbeat, PostResult.
type Client struct {
	// HTTP is the underlying HTTP transport. In production this is a
	// *backend.Client; in tests it is a stub implementing HTTPClient.
	HTTP HTTPClient
	// AccessToken is the bearer token included in every request.
	AccessToken string
	// RefreshAccessToken rotates the backend session after a 401. When set, the
	// failed request is retried once with the returned token.
	RefreshAccessToken func(context.Context) (string, error)
	tokenMu            sync.RWMutex
}

// CurrentAccessToken returns the token currently used for backend requests.
func (c *Client) CurrentAccessToken() string { return c.accessToken() }

// SetAccessToken replaces the bearer token used by subsequent requests.
func (c *Client) SetAccessToken(token string) {
	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	c.AccessToken = token
}

// PollNextRun calls GET /api/v1/schedules/next-run.
// Returns ErrNoRunAvailable on HTTP 204 (no pending run).
// Returns *backend.ProblemDetails on 4xx problem responses.
// Returns backend.ErrServerError on 5xx without a problem body.
// Returns backend.ErrNetworkFailure when the backend is unreachable.
func (c *Client) PollNextRun(ctx context.Context) (*NextRunResponse, error) {
	var resp NextRunResponse
	usedToken := c.accessToken()
	found, err := c.HTTP.GetJSONOptional(ctx, "/api/v1/schedules/next-run", usedToken, &resp)
	if retry, refreshErr := c.refreshAfterUnauthorized(ctx, usedToken, err); retry {
		if refreshErr != nil {
			return nil, refreshErr
		}
		found, err = c.HTTP.GetJSONOptional(ctx, "/api/v1/schedules/next-run", c.accessToken(), &resp)
	}
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrNoRunAvailable
	}
	return &resp, nil
}

// Heartbeat calls POST /api/v1/schedules/runs/{runID}/heartbeat.
// Maps HTTP 409 with problem type "claim-reaped" or "stale-claim" to
// ErrClaimReaped. Returns nil on 200.
func (c *Client) Heartbeat(ctx context.Context, runID, claimToken string) error {
	path := fmt.Sprintf("/api/v1/schedules/runs/%s/heartbeat", runID)
	body := HeartbeatRequest{ClaimToken: claimToken}
	usedToken := c.accessToken()
	err := c.HTTP.PostJSON(ctx, path, usedToken, body, nil)
	if retry, refreshErr := c.refreshAfterUnauthorized(ctx, usedToken, err); retry {
		if refreshErr != nil {
			return refreshErr
		}
		err = c.HTTP.PostJSON(ctx, path, c.accessToken(), body, nil)
	}
	if err == nil {
		return nil
	}
	// Map 409 claim-reaped / stale-claim to our sentinel.
	var pd *backend.ProblemDetails
	if errors.As(err, &pd) {
		t := pd.Type
		if t == "claim-reaped" || t == "stale-claim" {
			return ErrClaimReaped
		}
	}
	return err
}

// PostResult calls POST /api/v1/schedules/runs/{runID}/result.
// Maps HTTP 409 with problem type "already-completed" to ErrAlreadyCompleted.
// Returns backend.ErrNetworkFailure when the backend is unreachable.
func (c *Client) PostResult(ctx context.Context, runID string, body *ResultRequest) error {
	path := fmt.Sprintf("/api/v1/schedules/runs/%s/result", runID)
	usedToken := c.accessToken()
	err := c.HTTP.PostJSON(ctx, path, usedToken, body, nil)
	if retry, refreshErr := c.refreshAfterUnauthorized(ctx, usedToken, err); retry {
		if refreshErr != nil {
			return refreshErr
		}
		err = c.HTTP.PostJSON(ctx, path, c.accessToken(), body, nil)
	}
	if err == nil {
		return nil
	}
	// Map 409 already-completed to our sentinel.
	var pd *backend.ProblemDetails
	if errors.As(err, &pd) {
		if pd.Type == "already-completed" {
			return ErrAlreadyCompleted
		}
	}
	return err
}

func (c *Client) accessToken() string {
	c.tokenMu.RLock()
	defer c.tokenMu.RUnlock()
	return c.AccessToken
}

// refreshAfterUnauthorized serializes refreshes across the poll and heartbeat
// goroutines. If another request already replaced the token, the caller can
// retry immediately without rotating the refresh-token family again.
func (c *Client) refreshAfterUnauthorized(ctx context.Context, usedToken string, requestErr error) (bool, error) {
	if c.RefreshAccessToken == nil || !isUnauthorized(requestErr) {
		return false, nil
	}

	c.tokenMu.Lock()
	defer c.tokenMu.Unlock()
	if c.AccessToken != usedToken {
		return true, nil
	}

	token, err := c.RefreshAccessToken(ctx)
	if err != nil {
		return true, fmt.Errorf("refresh backend access token: %w", err)
	}
	if token == "" {
		return true, errors.New("refresh backend access token: backend returned an empty access token")
	}
	c.AccessToken = token
	return true, nil
}

func isUnauthorized(err error) bool {
	var problem *backend.ProblemDetails
	return errors.As(err, &problem) && problem.Status == 401
}
