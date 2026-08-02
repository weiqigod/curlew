package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestGCPProvider(t *testing.T) {
	t.Run("name_returns_gcp_secret_manager", func(t *testing.T) {
		p := NewGCPProvider("my-project", nil)
		if got := p.Name(); got != ProviderGCP {
			t.Errorf("got %q, want %q", got, ProviderGCP)
		}
	})

	t.Run("fetch_single_secret_success", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {"my-secret-value", nil},
		})
		p := NewGCPProvider("my-project", exec)
		got, err := p.Fetch(context.Background(), "my-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got != "my-secret-value" {
			t.Errorf("got %q, want %q", got, "my-secret-value")
		}
	})

	t.Run("fetch_command_format_is_correct", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "prod-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := "gcloud secrets versions access latest --secret='prod-secret' --project='my-project'"
		if captured != want {
			t.Errorf("got command %q, want %q", captured, want)
		}
	})

	t.Run("fetch_quotes_path_with_spaces", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "my secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'my secret'") {
			t.Errorf("expected quoted path in command, got: %s", captured)
		}
	})

	t.Run("fetch_quotes_project_with_special_chars", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "value", nil
		}
		p := NewGCPProvider("my project-123", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'my project-123'") {
			t.Errorf("expected quoted project in command, got: %s", captured)
		}
	})

	t.Run("fetch_not_found_returns_sentinel", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {"", fmt.Errorf("ERROR: (gcloud.secrets.versions.access) NOT_FOUND: Secret [projects/my-project/secrets/missing] not found")},
		})
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_unauthenticated_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {"", fmt.Errorf("ERROR: (gcloud.secrets.versions.access) UNAUTHENTICATED: Request had invalid authentication credentials")},
		})
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("fetch_permission_denied_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {"", fmt.Errorf("ERROR: (gcloud.secrets.versions.access) PERMISSION_DENIED: Permission denied on resource project my-project")},
		})
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("fetch_generic_error_wraps_original", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {"", fmt.Errorf("some unexpected gcloud error")},
		})
		p := NewGCPProvider("my-project", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if errors.Is(err, ErrSecretNotFound) || errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected wrapped generic error, got sentinel: %v", err)
		}
		if !strings.Contains(err.Error(), "gcp secret manager") {
			t.Errorf("expected 'gcp secret manager' prefix in error, got: %v", err)
		}
	})

	t.Run("fetch_returns_json_for_field_extraction", func(t *testing.T) {
		jsonSecret := `{"password":"s3cr3t","username":"admin"}`
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud secrets versions access": {jsonSecret, nil},
		})
		p := NewGCPProvider("my-project", exec)
		got, err := p.Fetch(context.Background(), "db-creds")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Provider returns the raw value; field extraction is done by Resolve.
		if got != jsonSecret {
			t.Errorf("got %q, want %q", got, jsonSecret)
		}
	})

	t.Run("bulk_fetch_calls_fetch_per_path", func(t *testing.T) {
		callCount := 0
		exec := func(ctx context.Context, command string) (string, error) {
			callCount++
			if strings.Contains(command, "secret-a") {
				return "value-a", nil
			}
			if strings.Contains(command, "secret-b") {
				return "value-b", nil
			}
			return "", fmt.Errorf("unexpected command: %s", command)
		}
		p := NewGCPProvider("my-project", exec)
		got, err := p.BulkFetch(context.Background(), []string{"secret-a", "secret-b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if callCount != 2 {
			t.Errorf("expected 2 calls, got %d", callCount)
		}
		if got["secret-a"] != "value-a" || got["secret-b"] != "value-b" {
			t.Errorf("unexpected results: %v", got)
		}
	})

	t.Run("bulk_fetch_returns_all_results", func(t *testing.T) {
		exec := func(_ context.Context, _ string) (string, error) {
			return "val", nil
		}
		p := NewGCPProvider("my-project", exec)
		got, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 3 {
			t.Errorf("expected 3 results, got %d", len(got))
		}
	})

	t.Run("bulk_fetch_stops_on_first_error", func(t *testing.T) {
		exec := func(_ context.Context, command string) (string, error) {
			if strings.Contains(command, "'b'") {
				return "", fmt.Errorf("NOT_FOUND: secret b not found")
			}
			return "val", nil
		}
		p := NewGCPProvider("my-project", exec)
		_, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("validate_config_success", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud config get-value project": {"my-project", nil},
		})
		p := NewGCPProvider("my-project", exec)
		if err := p.ValidateConfig(); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_cli_not_found_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud config get-value project": {"", fmt.Errorf("command not found: gcloud")},
		})
		p := NewGCPProvider("my-project", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
		if !strings.Contains(err.Error(), "https://cloud.google.com/sdk/docs/install") {
			t.Errorf("expected install URL in error, got: %v", err)
		}
	})

	t.Run("validate_config_unauthenticated_returns_auth_error", func(t *testing.T) {
		exec := mockExecutor(map[string]struct {
			output string
			err    error
		}{
			"gcloud config get-value project": {"", fmt.Errorf("UNAUTHENTICATED: not logged in")},
		})
		p := NewGCPProvider("my-project", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Fatalf("expected ErrProviderAuth, got %v", err)
		}
	})
}
