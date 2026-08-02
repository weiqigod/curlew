package vault

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	apierrors "github.com/weiqigod/curlew/internal/errors"
)

// mockProvider is a test double that implements Provider and records BulkFetch call count.
type mockProvider struct {
	providerName   string
	bulkFetchCount int
	data           map[string]string
}

func (m *mockProvider) Name() string { return m.providerName }

func (m *mockProvider) Fetch(ctx context.Context, path string) (string, error) {
	val, ok := m.data[path]
	if !ok {
		return "", fmt.Errorf("%w: %s", ErrSecretNotFound, path)
	}
	return val, nil
}

func (m *mockProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
	m.bulkFetchCount++
	result := make(map[string]string)
	for _, path := range paths {
		val, ok := m.data[path]
		if !ok {
			return nil, fmt.Errorf("%w: %s", ErrSecretNotFound, path)
		}
		result[path] = val
	}
	return result, nil
}

func (m *mockProvider) ValidateConfig() error { return nil }

func TestNewProvider(t *testing.T) {
	noop := func(ctx context.Context, command string) (string, error) {
		return "", nil
	}

	t.Run("aws_secrets_manager_returns_aws_provider", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys:     map[string]string{"k": "v"},
		}
		p, err := NewProvider(cfg, noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != ProviderAWS {
			t.Errorf("got %q, want %q", p.Name(), ProviderAWS)
		}
	})

	t.Run("unknown_provider_returns_error", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider: "unknown-provider",
			Keys:     map[string]string{"k": "v"},
		}
		_, err := NewProvider(cfg, noop)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrUnknownProvider) {
			t.Fatalf("expected ErrUnknownProvider, got %v", err)
		}
	})

	t.Run("nil_config_returns_error", func(t *testing.T) {
		_, err := NewProvider(nil, noop)
		if err == nil {
			t.Fatal("expected error for nil config, got nil")
		}
		if !errors.Is(err, ErrMissingRequiredField) {
			t.Fatalf("expected ErrMissingRequiredField, got %v", err)
		}
	})

	t.Run("azure_key_vault_returns_azure_provider", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider:  ProviderAzure,
			VaultName: "my-vault",
			Keys:      map[string]string{"k": "v"},
		}
		p, err := NewProvider(cfg, noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != ProviderAzure {
			t.Errorf("got %q, want %q", p.Name(), ProviderAzure)
		}
	})

	t.Run("hashicorp_vault_returns_hashicorp_provider", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider: ProviderHashiCorp,
			Address:  "https://vault:8200",
			Auth:     AuthConfig{Method: "token", Token: "tok"},
			Keys:     map[string]string{"k": "v"},
		}
		p, err := NewProvider(cfg, noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != ProviderHashiCorp {
			t.Errorf("got %q, want %q", p.Name(), ProviderHashiCorp)
		}
	})

	t.Run("gcp_secret_manager_returns_gcp_provider", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider: ProviderGCP,
			Project:  "my-project",
			Keys:     map[string]string{"k": "v"},
		}
		p, err := NewProvider(cfg, noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != ProviderGCP {
			t.Errorf("got %q, want %q", p.Name(), ProviderGCP)
		}
	})

	t.Run("1password_returns_onepassword_provider", func(t *testing.T) {
		cfg := &SecretsConfig{
			Provider: Provider1Password,
			Keys:     map[string]string{"k": "v"},
		}
		p, err := NewProvider(cfg, noop)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if p.Name() != Provider1Password {
			t.Errorf("got %q, want %q", p.Name(), Provider1Password)
		}
	})
}

func TestResolve(t *testing.T) {
	t.Run("resolve_simple_keys_no_field", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "prod/api-key") {
				return "secret123", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys:     map[string]string{"api_key": "prod/api-key"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["api_key"] != "secret123" {
			t.Errorf("got %q, want %q", result.Variables["api_key"], "secret123")
		}
	})

	t.Run("resolve_with_field_extraction", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "prod/db") {
				return `{"password":"s3cret","host":"db.example.com"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys:     map[string]string{"db_pass": "prod/db#password"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["db_pass"] != "s3cret" {
			t.Errorf("got %q, want %q", result.Variables["db_pass"], "s3cret")
		}
	})

	t.Run("resolve_multiple_fields_from_same_secret_single_fetch", func(t *testing.T) {
		fetchCount := 0
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "prod/db") {
				fetchCount++
				return `{"password":"s3cret","host":"db.example.com"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys: map[string]string{
				"db_pass": "prod/db#password",
				"db_host": "prod/db#host",
			},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if fetchCount != 1 {
			t.Errorf("expected 1 fetch call (deduplication), got %d", fetchCount)
		}
		if result.Variables["db_pass"] != "s3cret" {
			t.Errorf("db_pass = %q, want %q", result.Variables["db_pass"], "s3cret")
		}
		if result.Variables["db_host"] != "db.example.com" {
			t.Errorf("db_host = %q, want %q", result.Variables["db_host"], "db.example.com")
		}
	})

	t.Run("resolve_returns_error_for_missing_secret", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("ResourceNotFoundException: not found")
		}
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys:     map[string]string{"k": "nonexistent"},
		}
		_, err := Resolve(context.Background(), cfg, exec)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrSecretNotFound) {
			t.Fatalf("expected ErrSecretNotFound in chain, got %v", err)
		}
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *errors.Structured wrapper, got %T: %v", err, err)
		}
		if se.Category != apierrors.CategoryConfig {
			t.Errorf("category = %q, want %q", se.Category, apierrors.CategoryConfig)
		}
	})

	t.Run("resolve_uses_bulk_fetch_for_multiple_distinct_paths", func(t *testing.T) {
		mock := &mockProvider{
			providerName: "mock",
			data: map[string]string{
				"secret-a": "value-a",
				"secret-b": "value-b",
			},
		}
		refs := []KeyRef{
			{VarName: "key_a", Path: "secret-a"},
			{VarName: "key_b", Path: "secret-b"},
		}
		result, err := resolveSecrets(context.Background(), mock, refs)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if mock.bulkFetchCount != 1 {
			t.Errorf("BulkFetch called %d times, want 1", mock.bulkFetchCount)
		}
		if result.Variables["key_a"] != "value-a" {
			t.Errorf("key_a = %q, want %q", result.Variables["key_a"], "value-a")
		}
		if result.Variables["key_b"] != "value-b" {
			t.Errorf("key_b = %q, want %q", result.Variables["key_b"], "value-b")
		}
	})

	t.Run("resolve_returns_error_for_missing_field_in_json", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return `{"a":"b"}`, nil
		}
		cfg := &SecretsConfig{
			Provider: ProviderAWS,
			Region:   "us-east-1",
			Keys:     map[string]string{"k": "path#nonexistent"},
		}
		_, err := Resolve(context.Background(), cfg, exec)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrFieldNotFound) {
			t.Fatalf("expected ErrFieldNotFound, got %v", err)
		}
	})

	t.Run("resolve_unknown_provider_returns_error", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", nil
		}
		cfg := &SecretsConfig{
			Provider: "unknown",
			Keys:     map[string]string{"k": "v"},
		}
		_, err := Resolve(context.Background(), cfg, exec)
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrUnknownProvider) {
			t.Fatalf("expected ErrUnknownProvider, got %v", err)
		}
	})

	t.Run("resolve_nil_config_returns_empty", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			return "", nil
		}
		result, err := Resolve(context.Background(), nil, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(result.Variables) != 0 {
			t.Errorf("expected empty variables, got %v", result.Variables)
		}
	})

	t.Run("resolve_azure_simple_key", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "az keyvault secret show") && strings.Contains(command, "'my-api-key'") {
				return "azure-secret-value", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider:  ProviderAzure,
			VaultName: "my-vault",
			Keys:      map[string]string{"api_key": "my-api-key"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["api_key"] != "azure-secret-value" {
			t.Errorf("got %q, want %q", result.Variables["api_key"], "azure-secret-value")
		}
	})

	t.Run("resolve_azure_with_field_extraction", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "'db-config'") {
				return `{"host":"db.example.com","password":"s3cret"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider:  ProviderAzure,
			VaultName: "my-vault",
			Keys:      map[string]string{"db_host": "db-config#host"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["db_host"] != "db.example.com" {
			t.Errorf("got %q, want %q", result.Variables["db_host"], "db.example.com")
		}
	})

	t.Run("resolve_hashicorp_token_auth", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "vault kv get") {
				return `{"data":{"data":{"password":"hc-secret"},"metadata":{}}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderHashiCorp,
			Address:  "https://vault:8200",
			Auth:     AuthConfig{Method: "token", Token: "hvs.test"},
			Keys:     map[string]string{"db_pass": "secret/data/db#password"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["db_pass"] != "hc-secret" {
			t.Errorf("got %q, want %q", result.Variables["db_pass"], "hc-secret")
		}
	})

	t.Run("resolve_hashicorp_approle_auth", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "auth/approle/login") {
				return `{"auth":{"client_token":"hvs.approle-tok"}}`, nil
			}
			if strings.Contains(command, "vault kv get") {
				return `{"data":{"data":{"api_key":"approle-val"},"metadata":{}}}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderHashiCorp,
			Address:  "https://vault:8200",
			Auth:     AuthConfig{Method: "approle", RoleID: "role", SecretID: "secret"},
			Keys:     map[string]string{"key": "secret/data/api#api_key"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["key"] != "approle-val" {
			t.Errorf("got %q, want %q", result.Variables["key"], "approle-val")
		}
	})

	t.Run("resolve_gcp_simple_key", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "gcloud secrets versions access") && strings.Contains(command, "'prod-secret'") {
				return "gcp-secret-value", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderGCP,
			Project:  "my-project",
			Keys:     map[string]string{"api_key": "prod-secret"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["api_key"] != "gcp-secret-value" {
			t.Errorf("got %q, want %q", result.Variables["api_key"], "gcp-secret-value")
		}
	})

	t.Run("resolve_gcp_with_field_extraction", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "gcloud secrets versions access") && strings.Contains(command, "'db-creds'") {
				return `{"password":"s3cret","host":"db.example.com"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: ProviderGCP,
			Project:  "my-project",
			Keys:     map[string]string{"db_pass": "db-creds#password"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["db_pass"] != "s3cret" {
			t.Errorf("got %q, want %q", result.Variables["db_pass"], "s3cret")
		}
	})

	t.Run("resolve_1password_op_uri", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "op read") && strings.Contains(command, "op://Personal/item/password") {
				return "op-secret-value", nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: Provider1Password,
			Keys:     map[string]string{"api_key": "op://Personal/item/password"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["api_key"] != "op-secret-value" {
			t.Errorf("got %q, want %q", result.Variables["api_key"], "op-secret-value")
		}
	})

	t.Run("resolve_1password_item_with_field_extraction", func(t *testing.T) {
		exec := func(ctx context.Context, command string) (string, error) {
			if strings.Contains(command, "op item get") && strings.Contains(command, "'my-item'") {
				return `{"password":"1p-secret","username":"admin"}`, nil
			}
			return "", fmt.Errorf("unexpected: %s", command)
		}
		cfg := &SecretsConfig{
			Provider: Provider1Password,
			Keys:     map[string]string{"db_pass": "my-item#password"},
		}
		result, err := Resolve(context.Background(), cfg, exec)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.Variables["db_pass"] != "1p-secret" {
			t.Errorf("got %q, want %q", result.Variables["db_pass"], "1p-secret")
		}
	})
}
