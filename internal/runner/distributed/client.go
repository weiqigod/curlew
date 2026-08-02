package distributed

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
)

// HTTPClient implements CoordinatorClient using HTTP calls.
type HTTPClient struct {
	BaseURL    string
	Token      string
	HTTPClient *http.Client // nil = http.DefaultClient
}

// CreateJob posts to POST /api/v1/organizations/{org}/coordinator/jobs.
func (c *HTTPClient) CreateJob(ctx context.Context, org string, body *CreateJobBody) (*JobResponse, error) {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/coordinator/jobs", c.BaseURL, org)
	data, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshalling create-job request: %w", err)
	}

	respBody, status, err := c.doJSON(ctx, "POST", url, data)
	if err != nil {
		return nil, err
	}
	if status != 200 && status != 201 {
		return nil, fmt.Errorf("create job: unexpected status %d: %s", status, string(respBody))
	}

	var resp JobResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing create-job response: %w", err)
	}
	return &resp, nil
}

// GetJob calls GET /api/v1/organizations/{org}/coordinator/jobs/{jobID}.
func (c *HTTPClient) GetJob(ctx context.Context, org, jobID string) (*JobResponse, error) {
	url := fmt.Sprintf("%s/api/v1/organizations/%s/coordinator/jobs/%s", c.BaseURL, org, jobID)

	respBody, status, err := c.doJSON(ctx, "GET", url, nil)
	if err != nil {
		return nil, err
	}
	if status != 200 {
		return nil, fmt.Errorf("get job: unexpected status %d: %s", status, string(respBody))
	}

	var resp JobResponse
	if err := json.Unmarshal(respBody, &resp); err != nil {
		return nil, fmt.Errorf("parsing get-job response: %w", err)
	}
	return &resp, nil
}

// doJSON performs an HTTP request with JSON body, returning the response body and status.
func (c *HTTPClient) doJSON(ctx context.Context, method, url string, body []byte) ([]byte, int, error) {
	httpClient := c.HTTPClient
	if httpClient == nil {
		httpClient = http.DefaultClient
	}

	var bodyReader io.Reader
	if body != nil {
		bodyReader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, url, bodyReader)
	if err != nil {
		return nil, 0, fmt.Errorf("creating request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	req.Header.Set("Authorization", "Bearer "+c.Token)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("sending request: %w", err)
	}
	respBody, readErr := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if readErr != nil {
		return nil, resp.StatusCode, fmt.Errorf("reading response: %w", readErr)
	}
	return respBody, resp.StatusCode, nil
}
