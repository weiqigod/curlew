# Implementation Plan: M2-004

## Overview

Define the vault `Provider` interface and implement the AWS Secrets Manager provider. Add a caching layer with TTL, JSON field extraction (`#field` syntax), and wire vault secret resolution into the runner's variable pipeline at precedence 6.

## Task Details
- **ID:** M2-004
- **Title:** Vault provider interface and AWS Secrets Manager provider
- **Phase:** M2: Vault Storage
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-002 | Vault provider profile configuration and parsing | done |

## Implementation Steps

### Step 1: Provider Interface

**Rationale:** Zero blast radius — defines contracts that nothing depends on yet. Must come first since all subsequent steps implement or consume this interface.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/provider.go` | create | Provider interface, CommandExecutor type, sentinel errors |
| `internal/vault/provider_test.go` | create | Compile-time interface satisfaction check |

#### New Code

```go
package vault

import (
	"context"
	"errors"
)

// ErrSecretNotFound is returned when a requested secret path does not exist.
var ErrSecretNotFound = errors.New("secret not found")

// ErrProviderAuth is returned when provider authentication fails.
var ErrProviderAuth = errors.New("vault provider authentication failed")

// Provider defines the interface for vault secret providers.
// Implementations shell out to CLI tools rather than using SDK libraries.
type Provider interface {
	// Name returns the provider identifier (e.g., "aws-secrets-manager").
	Name() string

	// Fetch retrieves a single secret value by path.
	Fetch(ctx context.Context, path string) (string, error)

	// BulkFetch retrieves multiple secrets by path.
	// Falls back to individual Fetch calls if bulk is not natively supported.
	BulkFetch(ctx context.Context, paths []string) (map[string]string, error)

	// ValidateConfig checks prerequisites (CLI tools, credentials).
	ValidateConfig() error
}

// CommandExecutor runs a shell command and returns stdout.
// Injected for testability (avoids real CLI calls in tests).
type CommandExecutor func(ctx context.Context, command string) (string, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestProviderInterfaceSatisfaction(t *testing.T) {
	// compile-time check that AWSProvider satisfies Provider
	var _ Provider = (*AWSProvider)(nil)
}

func TestCommandExecutorType(t *testing.T) {
	// verify variable.ExecuteCommand has compatible signature
	var exec CommandExecutor = variable.ExecuteCommand
	_ = exec
}
```

Note: The first test won't compile until Step 4 (AWSProvider). Include it as a placeholder commented out, then uncomment.

#### Impact on Existing Tests
- No existing tests affected

---

### Step 2: Cache Layer

**Rationale:** Independent of provider implementation. Small, self-contained, fully testable. The cache pattern follows `variable.CommandCache` but adds `Invalidate` for refresh-on-failure support.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/cache.go` | create | TTL cache with invalidation |
| `internal/vault/cache_test.go` | create | Full cache test suite |

#### New Code

```go
package vault

import (
	"sync"
	"time"
)

// Cache stores fetched secret values with TTL-based expiration.
type Cache struct {
	mu      sync.Mutex
	entries map[string]cacheEntry
	ttl     int // seconds; 0 = no caching
}

func NewCache(ttlSeconds int) *Cache

func (c *Cache) Get(key string) (string, bool)

func (c *Cache) Set(key, value string)

// Invalidate removes a specific key (for refresh-on-failure).
func (c *Cache) Invalidate(key string)

func (c *Cache) InvalidateAll()
```

Design notes:
- TTL of 0 means no caching (`Get` always returns miss, `Set` is no-op)
- Uses `sync.Mutex` for thread safety (same as `variable.CommandCache`)
- `cacheEntry` reuses the same `{value, expires}` pattern

#### Tests to Write FIRST (RED phase)

```go
func TestCache(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"hit_within_ttl"},
		{"miss_on_empty_cache"},
		{"expired_after_ttl"},
		{"zero_ttl_disables_caching"},
		{"invalidate_single_key"},
		{"invalidate_all_clears_everything"},
		{"concurrent_access_is_safe"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 3: JSON Field Extraction

**Rationale:** Self-contained utility. `ParseKeyRef` already splits `path#field` — this step implements the JSON parsing that consumes the `Field` value.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/extract.go` | create | JSON field extraction |
| `internal/vault/extract_test.go` | create | Extraction test suite |

#### New Code

```go
package vault

import "errors"

var ErrFieldNotFound = errors.New("field not found in secret JSON")
var ErrNotJSONSecret = errors.New("secret is not valid JSON for field extraction")

// ExtractField parses rawJSON as a JSON object and extracts fieldName.
// String values are returned directly; other types are JSON-encoded.
func ExtractField(rawJSON string, fieldName string) (string, error)
```

#### Tests to Write FIRST (RED phase)

```go
func TestExtractField(t *testing.T) {
	tests := []struct {
		name      string
		rawJSON   string
		fieldName string
		want      string
		wantErr   error
	}{
		{"extracts_string_field", `{"password":"s3cret"}`, "password", "s3cret", nil},
		{"extracts_numeric_field", `{"port":5432}`, "port", "5432", nil},
		{"extracts_boolean_field", `{"active":true}`, "active", "true", nil},
		{"extracts_nested_object_as_json", `{"cfg":{"a":1}}`, "cfg", `{"a":1}`, nil},
		{"missing_field_returns_error", `{"a":"b"}`, "c", "", ErrFieldNotFound},
		{"non_json_returns_error", `not json`, "x", "", ErrNotJSONSecret},
		{"empty_json_object", `{}`, "x", "", ErrFieldNotFound},
		{"null_field_value", `{"x":null}`, "x", "", nil},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 4: AWS Secrets Manager Provider

**Rationale:** First concrete provider. Depends on `CommandExecutor` (Step 1) and uses `ExtractField` indirectly via the resolver. All tests use injected mock executors — no real AWS calls.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/aws.go` | create | AWS provider implementation |
| `internal/vault/aws_test.go` | create | AWS provider test suite (mocked CLI) |
| `internal/vault/provider_test.go` | modify | Uncomment interface satisfaction check |

#### New Code

```go
package vault

import (
	"context"
	"fmt"
	"strings"
)

// AWSProvider fetches secrets from AWS Secrets Manager via the AWS CLI.
type AWSProvider struct {
	Region  string
	execute CommandExecutor
}

func NewAWSProvider(region string, exec CommandExecutor) *AWSProvider

func (p *AWSProvider) Name() string // returns "aws-secrets-manager"

// Fetch runs: aws secretsmanager get-secret-value --secret-id <path> --region <region> --query SecretString --output text
func (p *AWSProvider) Fetch(ctx context.Context, path string) (string, error)

// BulkFetch calls Fetch per path (AWS CLI has no bulk command).
func (p *AWSProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error)

// ValidateConfig runs: aws sts get-caller-identity --region <region>
func (p *AWSProvider) ValidateConfig() error
```

Error classification in `Fetch`:
- stderr contains "ResourceNotFoundException" → wrap `ErrSecretNotFound`
- stderr contains "InvalidClientTokenId" / "ExpiredToken" / "credentials" → wrap `ErrProviderAuth` with hint: "Run 'aws configure' or check AWS credentials. See https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html"
- Other failures → generic error with stderr content

#### Tests to Write FIRST (RED phase)

```go
func TestAWSProvider(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"fetch_single_secret_success"},
		{"fetch_returns_raw_value_untrimmed_internally"},
		{"fetch_nonexistent_secret_returns_not_found"},
		{"fetch_invalid_credentials_returns_auth_error"},
		{"fetch_expired_credentials_returns_auth_error"},
		{"bulk_fetch_calls_fetch_per_path"},
		{"bulk_fetch_returns_all_results"},
		{"name_returns_aws_secrets_manager"},
		{"validate_config_success"},
		{"validate_config_missing_cli"},
		{"validate_config_bad_credentials"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 5: Resolver (Orchestrator)

**Rationale:** Glue layer connecting Provider + Cache + ExtractField + SecretsConfig. The runner calls this single function. Must come after Steps 1-4 since it composes all prior work.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver.go` | create | Orchestrator function |
| `internal/vault/resolver_test.go` | create | End-to-end resolver tests |

#### New Code

```go
package vault

import "context"

// ResolveResult holds resolved secret values.
type ResolveResult struct {
	Variables map[string]string
}

// NewProvider creates the appropriate Provider for the given config.
func NewProvider(cfg *SecretsConfig, exec CommandExecutor) (Provider, error)

// Resolve fetches all secrets defined in cfg, applies caching and field extraction.
func Resolve(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor) (*ResolveResult, error)
```

`Resolve` algorithm:
1. Call `cfg.ParsedKeys()` to get `[]KeyRef`
2. Create `Provider` via `NewProvider(cfg, exec)`
3. Create `Cache` with `cfg.CacheTTL`
4. Deduplicate paths (multiple vars may reference same secret with different `#field`)
5. For each unique path: check cache → if miss, fetch via provider → cache result
6. For each `KeyRef`: if `Field` != "", call `ExtractField(rawValue, field)`
7. Populate `ResolveResult.Variables`

`NewProvider` dispatches on `cfg.Provider`:
- `ProviderAWS` → `NewAWSProvider(cfg.Region, exec)`
- Others → `ErrUnknownProvider` (only AWS implemented in M2-004)

#### Tests to Write FIRST (RED phase)

```go
func TestResolve(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"resolve_simple_keys_no_field"},
		{"resolve_with_field_extraction"},
		{"resolve_multiple_fields_from_same_secret_single_fetch"},
		{"resolve_uses_cache_on_second_call"},
		{"resolve_returns_error_for_missing_secret"},
		{"resolve_returns_error_for_missing_field_in_json"},
		{"resolve_unknown_provider_returns_error"},
		{"resolve_nil_config_returns_empty"},
	}
}

func TestNewProvider(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"aws_secrets_manager_returns_aws_provider"},
		{"unknown_provider_returns_error"},
	}
}
```

#### Impact on Existing Tests
- No existing tests affected

---

### Step 6: Wire into Runner

**Rationale:** Integration step — largest blast radius. After vault gate check passes, call `vault.Resolve()` and inject secrets into `merged` at precedence 6. Requires updating `VarSources` with `VaultExecutor` for testability.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add VaultExecutor to VarSources; call vault.Resolve() after gate |
| `internal/runner/runner_test.go` | modify | Update existing vault_at_solo_tier test; add new vault tests |

#### Current Code

```go
// runner.go lines 64-74
type VarSources struct {
	Project  map[string]string
	EnvFile  map[string]string
	DotEnv   map[string]string
	EnvVar   map[string]string
	CLI      map[string]string
	Seed     *int64
	Tier     auth.Tier
	Registry *auth.Registry
	Secrets  *vault.SecretsConfig
}

// runner.go lines 147-164
	// Check vault feature gate
	if vars.Secrets != nil {
		reg := vars.Registry
		if reg == nil {
			reg = auth.DefaultRegistry()
		}
		tier := vars.Tier
		if tier == "" {
			tier = auth.TierFree
		}
		if err := auth.CheckFeature(reg, "vault_provider_profiles", tier); err != nil {
			return nil, summary, err
		}
	}

	for k, v := range col.Variables.Values {
```

#### New Code

```go
// VarSources — add one field
type VarSources struct {
	Project       map[string]string
	EnvFile       map[string]string
	DotEnv        map[string]string
	EnvVar        map[string]string
	CLI           map[string]string
	Seed          *int64
	Tier          auth.Tier
	Registry      *auth.Registry
	Secrets       *vault.SecretsConfig
	VaultExecutor vault.CommandExecutor // nil = use variable.ExecuteCommand
}

// runner.go — after gate check passes, resolve vault secrets
	// Check vault feature gate
	if vars.Secrets != nil {
		reg := vars.Registry
		if reg == nil {
			reg = auth.DefaultRegistry()
		}
		tier := vars.Tier
		if tier == "" {
			tier = auth.TierFree
		}
		if err := auth.CheckFeature(reg, "vault_provider_profiles", tier); err != nil {
			return nil, summary, err
		}

		// Precedence 6: vault secrets (after from_command at 5, before collection at 7)
		vaultExec := vars.VaultExecutor
		if vaultExec == nil {
			vaultExec = variable.ExecuteCommand
		}
		result, err := vault.Resolve(ctx, vars.Secrets, vaultExec)
		if err != nil {
			return nil, summary, err
		}
		for k, v := range result.Variables {
			merged[k] = v
		}
	}

	for k, v := range col.Variables.Values {
```

#### Tests to Write FIRST (RED phase)

```go
// New tests
func TestRun_VaultResolution(t *testing.T) {
	tests := []struct {
		name string
	}{
		{"vault_secrets_resolved_and_available_as_variables"},
		{"vault_secrets_overridden_by_collection_values"},
		{"vault_secrets_overridden_by_cli_vars"},
		{"vault_secrets_override_from_command"},
		{"vault_secret_fetch_error_stops_execution"},
		{"vault_field_extraction_in_runner"},
	}
}
```

#### Impact on Existing Tests

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | `vault_at_solo_tier_no_error` | **breaks** — `Resolve()` will attempt real CLI call | Provide mock `VaultExecutor` that returns success |
| `internal/runner/runner_test.go` | `vault_at_free_tier_returns_gate_error` | none — gate rejects before `Resolve()` is called | none |
| `internal/runner/runner_test.go` | `no_vault_no_gate_check` | none — `Secrets` is nil | none |

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/runner/runner_test.go` | `vault_at_solo_tier_no_error` | breaks | Add `VaultExecutor` mock returning values for key "k" |
| All other tests | — | none | — |

## Risks and Edge Cases

- **Risk:** AWS CLI not installed → **Mitigation:** `ValidateConfig()` checks availability; `Fetch()` wraps error with install URL hint
- **Risk:** Expired/invalid AWS credentials → **Mitigation:** Pattern-match stderr for known AWS error strings, return `ErrProviderAuth` with `aws configure` hint
- **Risk:** Malformed JSON in secret value → **Mitigation:** `ExtractField` returns `ErrNotJSONSecret` with clear message
- **Risk:** Missing field in JSON secret → **Mitigation:** `ExtractField` returns `ErrFieldNotFound`
- **Risk:** Multiple vars reference same secret path with different `#field` → **Mitigation:** `Resolve()` deduplicates paths before fetching — one CLI call per unique path
- **Risk:** Concurrent cache access → **Mitigation:** `sync.Mutex` on all cache operations
- **Risk:** `refresh_on_failure` behavior → **Mitigation:** Out of scope for M2-004 (requires HTTP assertion integration). Config field is parsed and stored; actual re-fetch trigger deferred to future task
- **Edge case:** Empty secret value from AWS → Valid, variable gets empty string
- **Edge case:** Binary secret (SecretBinary not SecretString) → `--query SecretString` returns error; hint in error message
- **Edge case:** Windows compatibility → `/bin/sh -c` pattern already established in codebase (`variable.ExecuteCommand`)

## Verification

```bash
go build ./cmd/apitest
go test ./internal/vault/...
go test ./internal/runner/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# With mock/real AWS credentials configured:
# apitest.yaml:
#   secrets:
#     provider: aws-secrets-manager
#     region: us-east-1
#     keys:
#       db_password: prod/db#password
#
# Then:
apitest run tests.yaml
# Confirm secrets are fetched and available as variables
go test ./internal/vault/... -v -count=1
```
