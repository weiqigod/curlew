package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/peterlindqvist/apitest/internal/backend"
)

func TestBackendVaultFetcher_RefreshesAccessTokenAfterUnauthorized(t *testing.T) {
	var calls, refreshes int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			w.Header().Set("Content-Type", "application/problem+json")
			w.WriteHeader(http.StatusUnauthorized)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"type": "https://api.apitool.dev/errors/unauthorized", "title": "Unauthorized",
				"status": 401, "code": "unauthorized",
			})
			return
		}
		if got := r.Header.Get("Authorization"); got != "Bearer fresh-access" {
			t.Errorf("retry Authorization = %q; want refreshed access token", got)
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"template": "team_secrets: {}\n", "version": 7,
		})
	}))
	t.Cleanup(srv.Close)

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	fetcher := &backendVaultFetcher{
		client: client,
		refreshAccessToken: func(context.Context) (string, error) {
			refreshes++
			return "fresh-access", nil
		},
	}

	result, err := fetcher.GetTeamVault(context.Background(), "org-test", "expired-access")
	if err != nil {
		t.Fatalf("GetTeamVault: %v", err)
	}
	if result.Version != 7 {
		t.Fatalf("version = %d; want 7", result.Version)
	}
	if calls != 2 || refreshes != 1 {
		t.Fatalf("calls=%d refreshes=%d; want 2 and 1", calls, refreshes)
	}
}
