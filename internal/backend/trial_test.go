package backend_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/backend"
)

// trialSuccessResponse writes a 200 activation response.
func trialSuccessResponse(w http.ResponseWriter, feature string) {
	now := time.Now().UTC()
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]interface{}{
		"feature":    feature,
		"kind":       "ondemand",
		"granted_at": now.Format(time.RFC3339),
		"expires_at": now.Add(7 * 24 * time.Hour).Format(time.RFC3339),
		"tokens": map[string]string{
			"license_jwt":   "lj-new",
			"access_token":  "at-new",
			"refresh_token": "rt-new",
		},
	})
}

func TestStartTrial_success_returns_activation_and_tokens(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/trials/vault_provider_profiles" {
			trialSuccessResponse(w, "vault_provider_profiles")
			return
		}
		http.NotFound(w, r)
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	act, err := client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err != nil {
		t.Fatalf("StartTrial() unexpected error: %v", err)
	}
	if act.Feature != "vault_provider_profiles" {
		t.Errorf("Feature = %q, want vault_provider_profiles", act.Feature)
	}
	if act.Kind != "ondemand" {
		t.Errorf("Kind = %q, want ondemand", act.Kind)
	}
	if act.Tokens.LicenseJWT != "lj-new" {
		t.Errorf("LicenseJWT = %q, want lj-new", act.Tokens.LicenseJWT)
	}
	if act.Tokens.RefreshToken != "rt-new" {
		t.Errorf("RefreshToken = %q, want rt-new", act.Tokens.RefreshToken)
	}
}

func TestStartTrial_409_TrialAlreadyConsumed_returns_sentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type":   "https://api.apitool.dev/errors/trial-already-consumed",
			"title":  "Trial already consumed",
			"status": 409,
			"code":   "TRIAL_ALREADY_CONSUMED",
			"previous_grant": map[string]interface{}{
				"kind":       "full_initial",
				"granted_at": time.Now().Format(time.RFC3339),
				"expires_at": time.Now().Add(14 * 24 * time.Hour).Format(time.RFC3339),
			},
		})
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, backend.ErrTrialAlreadyConsumed) {
		t.Errorf("expected errors.Is(ErrTrialAlreadyConsumed), got %v", err)
	}
	// ProblemDetails must be accessible.
	var pd *backend.ProblemDetails
	if !errors.As(err, &pd) {
		t.Errorf("errors.As(*ProblemDetails) = false; want true")
	} else if pd.Code != "TRIAL_ALREADY_CONSUMED" {
		t.Errorf("ProblemDetails.Code = %q; want TRIAL_ALREADY_CONSUMED", pd.Code)
	}
}

func TestStartTrial_404_TrialFeatureUnknown_returns_sentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusNotFound)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type":   "https://api.apitool.dev/errors/feature-unknown",
			"title":  "Feature unknown",
			"status": 404,
			"code":   "TRIAL_FEATURE_UNKNOWN",
		})
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "made_up_feature", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, backend.ErrTrialFeatureUnknown) {
		t.Errorf("expected errors.Is(ErrTrialFeatureUnknown), got %v", err)
	}
}

func TestStartTrial_path_includes_feature_slug(t *testing.T) {
	var seenPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seenPath = r.URL.Path
		trialSuccessResponse(w, "vault_provider_profiles")
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if seenPath != "/api/v1/trials/vault_provider_profiles" {
		t.Errorf("path = %q, want /api/v1/trials/vault_provider_profiles", seenPath)
	}
}

func TestStartTrial_request_body_carries_refresh_token_and_device_id(t *testing.T) {
	var body map[string]string
	var authorization string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authorization = r.Header.Get("Authorization")
		_ = json.NewDecoder(r.Body).Decode(&body)
		trialSuccessResponse(w, "vault_provider_profiles")
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, _ = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-secret", "dev-xyz", "at-secret")
	if body["refresh_token"] != "rt-secret" {
		t.Errorf("refresh_token = %q, want rt-secret", body["refresh_token"])
	}
	if body["device_id"] != "dev-xyz" {
		t.Errorf("device_id = %q, want dev-xyz", body["device_id"])
	}
	if authorization != "Bearer at-secret" {
		t.Errorf("Authorization = %q, want Bearer at-secret", authorization)
	}
}

func TestStartTrial_network_failure_returns_ErrNetworkFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	srv.Close() // close immediately to force network failure

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, backend.ErrNetworkFailure) {
		t.Errorf("expected errors.Is(ErrNetworkFailure), got %v", err)
	}
}

func TestStartTrial_5xx_returns_ErrServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("internal server error"))
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, backend.ErrServerError) {
		t.Errorf("expected errors.Is(ErrServerError), got %v", err)
	}
}

func TestStartTrial_409_error_string_format(t *testing.T) {
	// Covers trialSentinelError.Error() to satisfy coverage requirement (finding #3).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type":   "https://api.apitool.dev/errors/trial-already-consumed",
			"title":  "Trial already consumed",
			"status": 409,
			"code":   "TRIAL_ALREADY_CONSUMED",
			"previous_grant": map[string]interface{}{
				"kind":       "full_initial",
				"granted_at": time.Now().Add(-14 * 24 * time.Hour).Format(time.RFC3339),
				"expires_at": time.Now().Format(time.RFC3339),
			},
		})
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	// Error() must not panic and must include the sentinel description + title.
	errStr := err.Error()
	if errStr == "" {
		t.Error("Error() returned empty string")
	}
	if !strings.Contains(errStr, "trial already consumed") {
		t.Errorf("Error() = %q; want to contain 'trial already consumed'", errStr)
	}
	if !strings.Contains(errStr, "Trial already consumed") {
		t.Errorf("Error() = %q; want to contain 'Trial already consumed' (title)", errStr)
	}
}

func TestStartTrial_409_malformed_previous_grant_returns_nil_grant(t *testing.T) {
	// Finding #3: parsePreviousGrant must return nil (not panic) when the
	// previous_grant extension field contains invalid JSON. The TRIAL_ALREADY_CONSUMED
	// sentinel is still returned; only the previous-grant accessor returns nil.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		// Inject syntactically invalid JSON for the previous_grant field.
		_, _ = w.Write([]byte(`{
			"type":  "https://api.apitool.dev/errors/trial-already-consumed",
			"title": "Trial already consumed",
			"status": 409,
			"code":  "TRIAL_ALREADY_CONSUMED",
			"previous_grant": "not-an-object"
		}`))
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, backend.ErrTrialAlreadyConsumed) {
		t.Errorf("expected errors.Is(ErrTrialAlreadyConsumed), got %v", err)
	}
	// parsePreviousGrant returns nil for malformed JSON — PreviousGrant() must be nil.
	type previousGranter interface {
		PreviousGrant() *backend.TrialPreviousGrant
	}
	var pg previousGranter
	if errors.As(err, &pg) {
		if pg.PreviousGrant() != nil {
			t.Errorf("PreviousGrant() = non-nil for malformed JSON; want nil")
		}
	}
}

func TestStartTrial_409_previous_grant_is_accessible(t *testing.T) {
	// Verify that PreviousGrant() returns parsed data from the 409 extension.
	grantedAt := time.Now().Add(-14 * 24 * time.Hour).UTC().Truncate(time.Second)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusConflict)
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"type":   "https://api.apitool.dev/errors/trial-already-consumed",
			"title":  "Trial already consumed",
			"status": 409,
			"code":   "TRIAL_ALREADY_CONSUMED",
			"previous_grant": map[string]interface{}{
				"kind":       "full_initial",
				"granted_at": grantedAt.Format(time.RFC3339),
				"expires_at": time.Now().Format(time.RFC3339),
			},
		})
	}))
	defer srv.Close()

	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.StartTrial(context.Background(), "vault_provider_profiles", "rt-old", "dev-id", "at-old")
	if err == nil {
		t.Fatal("expected error, got nil")
	}

	type previousGranter interface {
		PreviousGrant() *backend.TrialPreviousGrant
	}
	var pg previousGranter
	if !errors.As(err, &pg) {
		t.Fatalf("errors.As previousGranter = false; got %T", err)
	}
	if pg.PreviousGrant() == nil {
		t.Fatal("PreviousGrant() = nil; expected non-nil for 409 TRIAL_ALREADY_CONSUMED")
	}
	if pg.PreviousGrant().Kind != "full_initial" {
		t.Errorf("PreviousGrant().Kind = %q; want full_initial", pg.PreviousGrant().Kind)
	}
	// GrantedAt should round-trip (within 1 second tolerance for RFC3339).
	diff := pg.PreviousGrant().GrantedAt.Sub(grantedAt)
	if diff < 0 {
		diff = -diff
	}
	if diff > time.Second {
		t.Errorf("PreviousGrant().GrantedAt = %v; want ~%v (diff %v)", pg.PreviousGrant().GrantedAt, grantedAt, diff)
	}
}
