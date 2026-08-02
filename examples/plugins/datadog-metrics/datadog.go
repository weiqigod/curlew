package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
)

// ddSeries is the minimal Datadog v2 series-submission payload.
type ddSeries struct {
	Series []ddMetric `json:"series"`
}

// ddMetric represents a single metric in the Datadog v2 series API.
type ddMetric struct {
	Metric string    `json:"metric"`
	Type   int       `json:"type"` // 1 = count, 3 = gauge
	Points []ddPoint `json:"points"`
	Tags   []string  `json:"tags,omitempty"`
}

// ddPoint is a single (timestamp, value) pair.
type ddPoint struct {
	Timestamp int64   `json:"timestamp"`
	Value     float64 `json:"value"`
}

// submitMetric posts a single metric to the Datadog /api/v2/series endpoint.
// Returns nil on 2xx; wraps the HTTP status on non-2xx; wraps transport errors
// with fmt.Errorf("submit: %w", err).
func submitMetric(ctx context.Context, client *http.Client, baseURL, apiKey string, m ddMetric) error {
	body, err := json.Marshal(ddSeries{Series: []ddMetric{m}})
	if err != nil {
		return fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, baseURL+"/api/v2/series", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("DD-API-KEY", apiKey)
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("submit: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("datadog http %d", resp.StatusCode)
	}
	return nil
}
