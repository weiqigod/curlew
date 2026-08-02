package main

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestSubmitMetric_PostsToSeriesEndpoint(t *testing.T) {
	var gotPath, gotKey, gotContentType string
	var gotBody []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotKey = r.Header.Get("DD-API-KEY")
		gotContentType = r.Header.Get("Content-Type")
		gotBody, _ = io.ReadAll(r.Body)
		w.WriteHeader(202)
	}))
	defer srv.Close()

	err := submitMetric(context.Background(), http.DefaultClient, srv.URL, "secret-key", ddMetric{
		Metric: "apitest.request.duration",
		Type:   3,
		Points: []ddPoint{{Timestamp: 1700000000, Value: 142}},
		Tags:   []string{"status:200"},
	})
	if err != nil {
		t.Fatalf("submitMetric: %v", err)
	}
	if gotPath != "/api/v2/series" {
		t.Errorf("path: %q", gotPath)
	}
	if gotKey != "secret-key" {
		t.Errorf("api key: %q", gotKey)
	}
	if gotContentType != "application/json" {
		t.Errorf("content-type: %q", gotContentType)
	}

	var payload ddSeries
	if err := json.Unmarshal(gotBody, &payload); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if len(payload.Series) != 1 || payload.Series[0].Metric != "apitest.request.duration" {
		t.Errorf("body: %s", gotBody)
	}
}

func TestSubmitMetric_Non2xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(401)
	}))
	defer srv.Close()
	err := submitMetric(context.Background(), http.DefaultClient, srv.URL, "bad", ddMetric{})
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Errorf("expected 401 error, got %v", err)
	}
}

func TestSubmitMetric_TransportErrorWrapped(t *testing.T) {
	err := submitMetric(context.Background(), http.DefaultClient, "http://127.0.0.1:1", "k", ddMetric{})
	if err == nil {
		t.Errorf("expected error, got nil")
	}
	// Verify the error is wrapped (errors.Is works on the chain)
	var urlErr interface{ Unwrap() error }
	if !errors.As(err, &urlErr) && !strings.Contains(err.Error(), "submit") {
		t.Errorf("expected wrapped transport error, got %v", err)
	}
}
