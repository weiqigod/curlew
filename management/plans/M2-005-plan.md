# Implementation Plan: M2-005

## Overview
Implement Azure Key Vault and HashiCorp Vault providers that fetch secrets via their respective CLI tools (`az`, `vault`), following the established AWS provider pattern and reusing the existing Provider interface, config validation, caching, and field extraction infrastructure.

## Task Details
- **ID:** M2-005
- **Title:** Azure Key Vault and HashiCorp Vault providers
- **Phase:** M2: Vault Storage
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-004 | Vault provider interface and AWS Secrets Manager provider | done |

## Implementation Steps

### Step 1: Azure Key Vault Provider

**Rationale:** Simpler of the two providers -- no multi-step auth flow. Directly mirrors the AWS pattern. Zero blast radius since nothing references it yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/azure.go` | create | Azure Key Vault provider implementation |
| `internal/vault/azure_test.go` | create | Full test suite with mocked CLI |

#### Azure CLI Commands

Fetch a secret:
```
az keyvault secret show --name <secret-name> --vault-name <vault-name> --query value -o tsv
```

Validate credentials:
```
az account show
```

#### New Code

```go
package vault

import (
	"context"
	"fmt"
	"strings"
)

const azureAuthHint = "Run 'az login' or check Azure CLI credentials. See https://learn.microsoft.com/en-us/cli/azure/authenticate-azure-cli"

var azureAuthErrorPatterns = []string{
	"AADSTS",
	"Please run 'az login'",
	"az login",
	"not logged in",
	"InvalidAuthenticationToken",
}

// AzureProvider fetches secrets from Azure Key Vault via the Azure CLI.
type AzureProvider struct {
	vaultName string
	execute   CommandExecutor
}

func NewAzureProvider(vaultName string, exec CommandExecutor) *AzureProvider
func (p *AzureProvider) Name() string
func (p *AzureProvider) Fetch(ctx context.Context, path string) (string, error)
func (p *AzureProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error)
func (p *AzureProvider) ValidateConfig() error
func (p *AzureProvider) classifyError(err error, path string) error
```

**Fetch builds:** `az keyvault secret show --name '<path>' --vault-name '<vaultName>' --query value -o tsv`

**Error classification:**
- `"SecretNotFound"` or `"ResourceNotFound"` -> `ErrSecretNotFound`
- Matches `azureAuthErrorPatterns` -> `ErrProviderAuth` with `azureAuthHint`
- Fallthrough -> `"azure key vault: %w"`

**ValidateConfig:** Runs `az account show`, classifies errors.

**BulkFetch:** Iterates calling `Fetch` per path (no bulk CLI command).

#### Tests to Write FIRST (RED phase)

```go
func TestAzureProvider(t *testing.T) {
	tests := []struct {
		name string
		// ...
	}{
		{"fetch_single_secret_success"},
		{"fetch_returns_json_secret_for_field_extraction"},
		{"fetch_nonexistent_secret_returns_not_found"},
		{"fetch_invalid_credentials_returns_auth_error"},
		{"fetch_not_logged_in_returns_auth_error"},
		{"bulk_fetch_calls_fetch_per_path"},
		{"bulk_fetch_returns_all_results"},
		{"name_returns_azure_key_vault"},
		{"validate_config_success"},
		{"validate_config_not_logged_in"},
		{"validate_config_cli_not_found"},
		{"fetch_quotes_path_with_spaces"},
		{"fetch_quotes_vault_name_with_special_chars"},
		{"fetch_command_format_is_correct"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected. New file pair.

---

### Step 2: HashiCorp Vault Provider

**Rationale:** More complex due to AppRole auth (two-step: login to get token, then use token for `vault kv get`). Still zero blast radius since not wired into factory yet.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/hashicorp.go` | create | HashiCorp Vault provider implementation |
| `internal/vault/hashicorp_test.go` | create | Full test suite with mocked CLI |

#### HashiCorp Vault CLI Commands

Token auth -- fetch:
```
VAULT_ADDR=<address> VAULT_TOKEN=<token> vault kv get -format=json <path>
```

AppRole auth -- login:
```
VAULT_ADDR=<address> vault write -format=json auth/approle/login role_id=<role_id> secret_id=<secret_id>
```
Returns JSON with `.auth.client_token`.

Validate credentials (token):
```
VAULT_ADDR=<address> VAULT_TOKEN=<token> vault token lookup -format=json
```

#### Design: Handling `vault kv get` JSON Output

The `vault kv get -format=json <path>` command returns:
```json
{
  "data": {
    "data": {
      "password": "s3cret",
      "host": "db.example.com"
    },
    "metadata": { ... }
  }
}
```

The provider parses this JSON, extracts `.data.data`, and re-serializes as compact JSON. The existing `ExtractField` then handles `#field` references if present.

#### New Code

```go
package vault

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
)

const hashicorpAuthHint = "Check your Vault token or AppRole credentials. See https://developer.hashicorp.com/vault/docs/auth"
const hashicorpNetworkHint = "Check that the Vault server is reachable at the configured address. See https://developer.hashicorp.com/vault/docs/configuration/listener/tcp"

var hashicorpAuthErrorPatterns = []string{
	"permission denied",
	"missing client token",
	"invalid role",
	"invalid secret",
}

var hashicorpNetworkErrorPatterns = []string{
	"connection refused",
	"no such host",
	"dial tcp",
	"i/o timeout",
	"certificate",
	"tls:",
}

// HashiCorpProvider fetches secrets from HashiCorp Vault via the vault CLI.
type HashiCorpProvider struct {
	address string
	auth    AuthConfig
	token   string          // resolved auth token (set on first use)
	execute CommandExecutor
}

func NewHashiCorpProvider(address string, auth AuthConfig, exec CommandExecutor) *HashiCorpProvider
func (p *HashiCorpProvider) Name() string
func (p *HashiCorpProvider) Fetch(ctx context.Context, path string) (string, error)
func (p *HashiCorpProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error)
func (p *HashiCorpProvider) ValidateConfig() error
func (p *HashiCorpProvider) ensureToken(ctx context.Context) error
func (p *HashiCorpProvider) approleLogin(ctx context.Context) (string, error)
func (p *HashiCorpProvider) classifyError(err error, path string) error
func (p *HashiCorpProvider) extractSecretData(jsonOutput string) (string, error)
```

**`ensureToken` flow:**
- If `p.token` is already set, return immediately
- If `p.auth.Method == "approle"`, call `p.approleLogin(ctx)`, store result in `p.token`

**`approleLogin`:** Runs the `vault write auth/approle/login` command, parses `.auth.client_token` from JSON output.

**`Fetch`:** Calls `ensureToken`, then runs `vault kv get -format=json`, passes output through `extractSecretData`.

**`extractSecretData`:** Parses full JSON, navigates to `.data.data`, re-serializes as compact JSON.

**Error classification:**
- Matches `hashicorpNetworkErrorPatterns` -> descriptive error with `hashicorpNetworkHint`
- `"No value found"` or `"secret not found"` -> `ErrSecretNotFound`
- Matches `hashicorpAuthErrorPatterns` -> `ErrProviderAuth` with `hashicorpAuthHint`
- Fallthrough -> `"hashicorp vault: %w"`

#### Tests to Write FIRST (RED phase)

```go
func TestHashiCorpProvider(t *testing.T) {
	tests := []struct {
		name string
		// ...
	}{
		{"fetch_single_secret_token_auth_success"},
		{"fetch_extracts_data_data_from_kv_json"},
		{"fetch_nonexistent_secret_returns_not_found"},
		{"fetch_invalid_token_returns_auth_error"},
		{"fetch_network_error_returns_clear_message"},
		{"fetch_connection_refused_shows_network_hint"},
		{"fetch_approle_auth_success"},
		{"fetch_approle_caches_token_across_calls"},
		{"fetch_approle_invalid_credentials_returns_auth_error"},
		{"bulk_fetch_calls_fetch_per_path"},
		{"name_returns_hashicorp_vault"},
		{"validate_config_token_auth_success"},
		{"validate_config_approle_auth_success"},
		{"validate_config_unreachable_server"},
		{"fetch_quotes_path_with_spaces"},
		{"fetch_command_includes_vault_addr"},
		{"fetch_command_includes_vault_token"},
		{"approle_login_command_format_is_correct"},
	}
}
```

**Key test: `fetch_approle_caches_token_across_calls`** -- verifies that calling `Fetch` twice with AppRole auth only triggers the login command once.

#### Impact on Existing Tests
- No existing tests affected. New file pair.

---

### Step 3: Wire Both Providers into Resolver

**Rationale:** Smallest integration change -- two new cases in a switch statement. Once done, end-to-end resolution works for both providers.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver.go` | modify | Add Azure and HashiCorp cases to `NewProvider` |

#### Current Code

```go
func NewProvider(cfg *SecretsConfig, exec CommandExecutor) (Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: nil secrets config", ErrMissingRequiredField)
	}
	switch cfg.Provider {
	case ProviderAWS:
		return NewAWSProvider(cfg.Region, exec), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, cfg.Provider)
	}
}
```

#### New Code

```go
func NewProvider(cfg *SecretsConfig, exec CommandExecutor) (Provider, error) {
	if cfg == nil {
		return nil, fmt.Errorf("%w: nil secrets config", ErrMissingRequiredField)
	}
	switch cfg.Provider {
	case ProviderAWS:
		return NewAWSProvider(cfg.Region, exec), nil
	case ProviderAzure:
		return NewAzureProvider(cfg.VaultName, exec), nil
	case ProviderHashiCorp:
		return NewHashiCorpProvider(cfg.Address, cfg.Auth, exec), nil
	default:
		return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, cfg.Provider)
	}
}
```

#### Tests to Write FIRST (RED phase)

```go
// In resolver_test.go, add to TestNewProvider:

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
```

#### Impact on Existing Tests
- No existing tests broken. `unknown_provider_returns_error` and `aws_secrets_manager_returns_aws_provider` still pass.

---

### Step 4: Update Provider Interface Checks and Resolver Integration Tests

**Rationale:** Compile-time interface satisfaction checks and end-to-end resolver tests for the new providers.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/provider_test.go` | modify | Add compile-time checks for Azure and HashiCorp |
| `internal/vault/resolver_test.go` | modify | Add Resolve integration tests for both providers |

#### provider_test.go -- Add:

```go
// Compile-time check that AzureProvider satisfies Provider.
var _ Provider = (*AzureProvider)(nil)

// Compile-time check that HashiCorpProvider satisfies Provider.
var _ Provider = (*HashiCorpProvider)(nil)
```

#### resolver_test.go -- Add:

```go
t.Run("resolve_azure_simple_key", func(t *testing.T) {
	// Mock az CLI, verify variable resolved
})

t.Run("resolve_azure_with_field_extraction", func(t *testing.T) {
	// Mock az CLI returning JSON, verify #field extraction
})

t.Run("resolve_hashicorp_token_auth", func(t *testing.T) {
	// Mock vault CLI, verify variable resolved
})

t.Run("resolve_hashicorp_approle_auth", func(t *testing.T) {
	// Mock approle login + kv get, verify variable resolved
})
```

#### Impact on Existing Tests
- All existing tests unaffected. Additions only.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/vault/aws_test.go` | all | none | -- |
| `internal/vault/cache_test.go` | all | none | -- |
| `internal/vault/config_test.go` | all | none | -- |
| `internal/vault/extract_test.go` | all | none | -- |
| `internal/vault/yaml_test.go` | all | none | -- |
| `internal/vault/provider_test.go` | all | additive | 2 new compile-time checks |
| `internal/vault/resolver_test.go` | all | additive | new factory + resolve tests |

## Risks and Edge Cases

- **Risk: HashiCorp KV v1 vs v2 paths** -> The provider uses `vault kv get` which is KV v2. Document that v2 paths are expected. Invalid paths produce clear CLI errors.

- **Risk: AppRole token expiration mid-session** -> For single-run resolution, the token only needs to last for the fetch duration. Provider instances are re-created per run. No additional complexity needed.

- **Risk: `vault kv get` JSON structure varies** -> `.data.data` is stable across all major Vault versions. `extractSecretData` returns a clear error if the structure is missing.

- **Risk: Azure secret names with slashes** -> Azure Key Vault doesn't support slashes in secret names. The provider passes the name through; Azure CLI returns a clear error for invalid names.

- **Risk: `shellQuote` is in `aws.go`** -> Since `shellQuote` is unexported and all providers are in the same `vault` package, it's directly accessible. No refactoring needed.

- **Edge case: Empty secret value** -> `Fetch` returns `""` with no error. Empty string is a valid secret value, consistent with AWS provider.

- **Edge case: HashiCorp `.data.data` is null/empty** -> `extractSecretData` returns `"{}"`. If `#field` is used, `ExtractField` returns `ErrFieldNotFound`.

- **Edge case: CLI tool not installed** -> Command executor returns "command not found" error. Falls through to generic error with descriptive message. `ValidateConfig` provides a pre-flight check.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/vault/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Configure Azure Key Vault profile in curlew.yaml:
#   secrets:
#     provider: azure-key-vault
#     vault_name: my-vault
#     keys:
#       api_key: my-api-key

# Configure HashiCorp Vault profile in curlew.yaml:
#   secrets:
#     provider: hashicorp-vault
#     address: https://vault.example.com:8200
#     auth:
#       method: token
#       token: hvs.xxxxx
#     keys:
#       db_pass: secret/data/db#password

# Run tests with mocked CLI calls:
go test ./internal/vault/... -v -run "TestAzureProvider|TestHashiCorpProvider"
```
