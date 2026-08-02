package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestHashiCorpProvider(t *testing.T) {
	tokenAuth := AuthConfig{Method: "token", Token: "hvs.test-token"}
	approleAuth := AuthConfig{Method: "approle", RoleID: "role-123", SecretID: "secret-456"}

	t.Run("name_returns_hashicorp_vault", func(t *testing.T) {
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, nil)
		if p.Name() != ProviderHashiCorp {
			t.Errorf("got %q, want %q", p.Name(), ProviderHashiCorp)
		}
	})

	t.Run("fetch_single_secret_token_auth_success", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return `{"data":{"data":{"password":"s3cret","host":"db.example.com"},"metadata":{}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		val, err := p.Fetch(context.Background(), "secret/data/db")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Should return re-serialized .data.data as JSON
		if !strings.Contains(val, "password") || !strings.Contains(val, "s3cret") {
			t.Errorf("expected JSON with password, got %q", val)
		}
	})

	t.Run("fetch_extracts_data_data_from_kv_json", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return `{"data":{"data":{"key":"value"},"metadata":{"version":1}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		val, err := p.Fetch(context.Background(), "secret/data/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != `{"key":"value"}` {
			t.Errorf("got %q, want %q", val, `{"key":"value"}`)
		}
	})

	t.Run("fetch_nonexistent_secret_returns_not_found", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("No value found at secret/data/nonexistent")
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_invalid_token_returns_auth_error", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("permission denied")
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected ErrProviderAuth, got %v", err)
		}
		if !strings.Contains(err.Error(), "Vault token") {
			t.Errorf("expected auth hint in error, got %q", err.Error())
		}
	})

	t.Run("fetch_network_error_returns_clear_message", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("dial tcp 127.0.0.1:8200: connection refused")
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Vault server") {
			t.Errorf("expected network hint in error, got %q", err.Error())
		}
	})

	t.Run("fetch_connection_refused_shows_network_hint", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("connection refused")
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "hashicorp.com") {
			t.Errorf("expected hashicorp docs link in error, got %q", err.Error())
		}
	})

	t.Run("fetch_approle_auth_success", func(t *testing.T) {
		callCount := 0
		exec := func(ctx context.Context, command string) (string, error) {
			callCount++
			if strings.Contains(command, "auth/approle/login") {
				return `{"auth":{"client_token":"hvs.approle-token"}}`, nil
			}
			if strings.Contains(command, "vault kv get") {
				// Verify the token from approle login is used
				if !strings.Contains(command, "VAULT_TOKEN='hvs.approle-token'") {
					t.Errorf("expected approle token in command, got %q", command)
				}
				return `{"data":{"data":{"secret":"approle-value"},"metadata":{}}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewHashiCorpProvider("https://vault:8200", approleAuth, exec)
		val, err := p.Fetch(context.Background(), "secret/data/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(val, "approle-value") {
			t.Errorf("expected approle-value in result, got %q", val)
		}
	})

	t.Run("fetch_approle_caches_token_across_calls", func(t *testing.T) {
		loginCalls := 0
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "auth/approle/login") {
				loginCalls++
				return `{"auth":{"client_token":"hvs.cached-token"}}`, nil
			}
			if strings.Contains(command, "vault kv get") {
				return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewHashiCorpProvider("https://vault:8200", approleAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/a")
		if err != nil {
			t.Fatalf("first fetch: %v", err)
		}
		_, err = p.Fetch(context.Background(), "secret/data/b")
		if err != nil {
			t.Fatalf("second fetch: %v", err)
		}
		if loginCalls != 1 {
			t.Errorf("expected 1 approle login, got %d", loginCalls)
		}
	})

	t.Run("fetch_approle_invalid_credentials_returns_auth_error", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("invalid role or secret")
		}
		p := NewHashiCorpProvider("https://vault:8200", approleAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("bulk_fetch_calls_fetch_per_path", func(t *testing.T) {
		fetchCalls := 0
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "vault kv get") {
				fetchCalls++
				return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		results, err := p.BulkFetch(context.Background(), []string{"a", "b"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fetchCalls != 2 {
			t.Errorf("expected 2 fetch calls, got %d", fetchCalls)
		}
		if len(results) != 2 {
			t.Errorf("expected 2 results, got %d", len(results))
		}
	})

	t.Run("validate_config_token_auth_success", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "vault token lookup") {
				return `{"data":{"id":"hvs.test"}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		err := p.ValidateConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_approle_auth_success", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "auth/approle/login") {
				return `{"auth":{"client_token":"hvs.approle"}}`, nil
			}
			if strings.Contains(command, "vault token lookup") {
				return `{"data":{"id":"hvs.approle"}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewHashiCorpProvider("https://vault:8200", approleAuth, exec)
		err := p.ValidateConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_unreachable_server", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("dial tcp: no such host")
		}
		p := NewHashiCorpProvider("https://vault.invalid:8200", tokenAuth, exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "Vault server") {
			t.Errorf("expected network hint, got %q", err.Error())
		}
	})

	t.Run("fetch_command_includes_vault_addr", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "VAULT_ADDR='https://vault:8200'") {
			t.Errorf("expected VAULT_ADDR in command, got %q", captured)
		}
	})

	t.Run("fetch_command_includes_vault_token", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "VAULT_TOKEN='hvs.test-token'") {
			t.Errorf("expected VAULT_TOKEN in command, got %q", captured)
		}
	})

	t.Run("fetch_quotes_path_with_spaces", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/my path")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'secret/data/my path'") {
			t.Errorf("expected quoted path, got %q", captured)
		}
	})

	t.Run("approle_login_command_format_is_correct", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "auth/approle/login") {
				captured = command
				return `{"auth":{"client_token":"hvs.tok"}}`, nil
			}
			return `{"data":{"data":{"k":"v"},"metadata":{}}}`, nil
		}
		p := NewHashiCorpProvider("https://vault:8200", approleAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "VAULT_ADDR='https://vault:8200'") {
			t.Errorf("expected VAULT_ADDR in login command, got %q", captured)
		}
		if !strings.Contains(captured, "vault write -format=json auth/approle/login") {
			t.Errorf("expected vault write auth/approle/login, got %q", captured)
		}
		if !strings.Contains(captured, "role_id='role-123'") {
			t.Errorf("expected role_id in login command, got %q", captured)
		}
		if !strings.Contains(captured, "secret_id='secret-456'") {
			t.Errorf("expected secret_id in login command, got %q", captured)
		}
	})

	t.Run("fetch_network_error_wraps_original_error", func(t *testing.T) {
		origErr := fmt.Errorf("dial tcp 127.0.0.1:8200: connection refused")
		exec := func(ctx context.Context, command string) (string, error) {
			return "", origErr
		}
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, exec)
		_, err := p.Fetch(context.Background(), "secret/data/test")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		unwrapped := errors.Unwrap(err)
		if unwrapped == nil {
			t.Fatal("expected wrapped error, got nil from Unwrap")
		}
		if unwrapped.Error() != origErr.Error() {
			t.Errorf("unwrapped error = %q, want %q", unwrapped.Error(), origErr.Error())
		}
	})

	t.Run("extract_secret_data_invalid_json", func(t *testing.T) {
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, nil)
		_, err := p.extractSecretData("not valid json at all")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse response") {
			t.Errorf("expected 'failed to parse response' in error, got %q", err.Error())
		}
	})

	t.Run("extract_secret_data_missing_data_field", func(t *testing.T) {
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, nil)
		_, err := p.extractSecretData(`{"auth":{"token":"x"}}`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "response missing 'data' field") {
			t.Errorf("expected missing data field error, got %q", err.Error())
		}
	})

	t.Run("extract_secret_data_data_field_not_object", func(t *testing.T) {
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, nil)
		_, err := p.extractSecretData(`{"data":"not-an-object"}`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "failed to parse data field") {
			t.Errorf("expected 'failed to parse data field' in error, got %q", err.Error())
		}
	})

	t.Run("extract_secret_data_missing_inner_data_field", func(t *testing.T) {
		p := NewHashiCorpProvider("https://vault:8200", tokenAuth, nil)
		_, err := p.extractSecretData(`{"data":{"metadata":{"version":1}}}`)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), "response missing 'data.data' field") {
			t.Errorf("expected missing data.data field error, got %q", err.Error())
		}
	})
}
