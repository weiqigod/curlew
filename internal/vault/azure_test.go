package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestAzureProvider(t *testing.T) {
	t.Run("name_returns_azure_key_vault", func(t *testing.T) {
		p := NewAzureProvider("my-vault", nil)
		if p.Name() != ProviderAzure {
			t.Errorf("got %q, want %q", p.Name(), ProviderAzure)
		}
	})

	t.Run("fetch_single_secret_success", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "my-secret-value", nil
		}
		p := NewAzureProvider("my-vault", exec)
		val, err := p.Fetch(context.Background(), "my-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != "my-secret-value" {
			t.Errorf("got %q, want %q", val, "my-secret-value")
		}
	})

	t.Run("fetch_returns_json_secret_for_field_extraction", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return `{"host":"db.example.com","password":"s3cret"}`, nil
		}
		p := NewAzureProvider("my-vault", exec)
		val, err := p.Fetch(context.Background(), "database-prod")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if val != `{"host":"db.example.com","password":"s3cret"}` {
			t.Errorf("got %q, want JSON secret", val)
		}
	})

	t.Run("fetch_nonexistent_secret_returns_not_found", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("SecretNotFound: secret not found")
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "nonexistent")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_resource_not_found_returns_not_found", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("ResourceNotFound: the resource was not found")
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "missing")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Errorf("expected ErrSecretNotFound, got %v", err)
		}
	})

	t.Run("fetch_invalid_credentials_returns_auth_error", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("AADSTS70001: Application with identifier was not found")
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected ErrProviderAuth, got %v", err)
		}
		if !strings.Contains(err.Error(), "az login") {
			t.Errorf("expected auth hint in error, got %q", err.Error())
		}
	})

	t.Run("fetch_not_logged_in_returns_auth_error", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("Please run 'az login' to setup account")
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("bulk_fetch_calls_fetch_per_path", func(t *testing.T) {
		calls := 0
		exec := func(ctx context.Context, command string) (string, error) {
			calls++
			if strings.Contains(command, "'key1'") {
				return "val1", nil
			}
			if strings.Contains(command, "'key2'") {
				return "val2", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewAzureProvider("my-vault", exec)
		results, err := p.BulkFetch(context.Background(), []string{"key1", "key2"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if calls != 2 {
			t.Errorf("expected 2 calls, got %d", calls)
		}
		if results["key1"] != "val1" {
			t.Errorf("key1 = %q, want %q", results["key1"], "val1")
		}
		if results["key2"] != "val2" {
			t.Errorf("key2 = %q, want %q", results["key2"], "val2")
		}
	})

	t.Run("bulk_fetch_returns_all_results", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "'a'") {
				return "va", nil
			}
			if strings.Contains(command, "'b'") {
				return "vb", nil
			}
			if strings.Contains(command, "'c'") {
				return "vc", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewAzureProvider("my-vault", exec)
		results, err := p.BulkFetch(context.Background(), []string{"a", "b", "c"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 3 {
			t.Errorf("expected 3 results, got %d", len(results))
		}
	})

	t.Run("validate_config_success", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "az account show") {
				return `{"id":"sub-123"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		p := NewAzureProvider("my-vault", exec)
		err := p.ValidateConfig()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("validate_config_not_logged_in", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("Please run 'az login' to setup account")
		}
		p := NewAzureProvider("my-vault", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrProviderAuth) {
			t.Errorf("expected ErrProviderAuth, got %v", err)
		}
	})

	t.Run("validate_config_cli_not_found", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("az: command not found")
		}
		p := NewAzureProvider("my-vault", exec)
		err := p.ValidateConfig()
		if err == nil {
			t.Fatal("expected error, got nil")
		}
	})

	t.Run("fetch_command_format_is_correct", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "val", nil
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "my-secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		expected := "az keyvault secret show --name 'my-secret' --vault-name 'my-vault' --query value -o tsv"
		if captured != expected {
			t.Errorf("command:\n  got  %q\n  want %q", captured, expected)
		}
	})

	t.Run("fetch_quotes_path_with_spaces", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "val", nil
		}
		p := NewAzureProvider("my-vault", exec)
		_, err := p.Fetch(context.Background(), "my secret name")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'my secret name'") {
			t.Errorf("expected quoted path, got %q", captured)
		}
	})

	t.Run("fetch_quotes_vault_name_with_special_chars", func(t *testing.T) {
		var captured string
		exec := func(ctx context.Context, command string) (string, error) {
			captured = command
			return "val", nil
		}
		p := NewAzureProvider("vault's-name", exec)
		_, err := p.Fetch(context.Background(), "secret")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !strings.Contains(captured, "'vault'\\''s-name'") {
			t.Errorf("expected escaped vault name, got %q", captured)
		}
	})
}
