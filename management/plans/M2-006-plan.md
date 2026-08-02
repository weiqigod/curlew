# Implementation Plan: M2-006

## Overview
Implement GCP Secret Manager and 1Password CLI providers that follow the existing Provider interface and
CLI-executor pattern established by M2-004/M2-005, then wire both into the `NewProvider` factory.

## Task Details
- **ID:** M2-006
- **Title:** GCP Secret Manager and 1Password CLI providers
- **Phase:** M2: Vault Storage
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-004 | Vault provider interface and AWS Secrets Manager | done |

---

## Implementation Steps

### Step 1: GCP Secret Manager provider (RED → GREEN)
**Rationale:** GCP structurally mirrors Azure (one required config field: `project`, single CLI tool, single command format). Smaller blast radius than 1Password's dual-mode fetch logic. Writing tests first ensures the command format is locked before any implementation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/gcp_test.go` | create | Table-driven tests for GCPProvider (RED phase) |
| `internal/vault/gcp.go` | create | GCPProvider implementation (GREEN phase) |

#### Current Code
```go
// resolver.go — NewProvider switch, relevant excerpt
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
```

#### New Code (`internal/vault/gcp.go`)
```go
package vault

import (
    "context"
    "fmt"
    "strings"
)

const gcpAuthHint = "Run 'gcloud auth login' or check GCP credentials. See https://cloud.google.com/sdk/gcloud/reference/auth/login"

var gcpAuthErrorPatterns = []string{
    "UNAUTHENTICATED",
    "PERMISSION_DENIED",
    "gcloud auth login",
    "not authorized",
}

// GCPProvider fetches secrets from GCP Secret Manager via the gcloud CLI.
type GCPProvider struct {
    project string
    execute CommandExecutor
}

// NewGCPProvider creates a provider that shells out to the gcloud CLI.
func NewGCPProvider(project string, exec CommandExecutor) *GCPProvider {
    return &GCPProvider{project: project, execute: exec}
}

func (p *GCPProvider) Name() string { return ProviderGCP }

func (p *GCPProvider) Fetch(ctx context.Context, path string) (string, error) {
    cmd := fmt.Sprintf(
        "gcloud secrets versions access latest --secret=%s --project=%s",
        shellQuote(path), shellQuote(p.project),
    )
    out, err := p.execute(ctx, cmd)
    if err != nil {
        return "", p.classifyError(err, path)
    }
    return out, nil
}

func (p *GCPProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
    results := make(map[string]string, len(paths))
    for _, path := range paths {
        val, err := p.Fetch(ctx, path)
        if err != nil {
            return nil, err
        }
        results[path] = val
    }
    return results, nil
}

func (p *GCPProvider) ValidateConfig() error {
    _, err := p.execute(context.Background(), "gcloud config get-value project")
    if err != nil {
        return p.classifyError(err, "")
    }
    return nil
}

func (p *GCPProvider) classifyError(err error, path string) error {
    msg := err.Error()
    if strings.Contains(msg, "NOT_FOUND") || strings.Contains(msg, "not found") {
        return fmt.Errorf("%w: %s", ErrSecretNotFound, path)
    }
    for _, pattern := range gcpAuthErrorPatterns {
        if strings.Contains(msg, pattern) {
            return fmt.Errorf("%w: %s. %s", ErrProviderAuth, msg, gcpAuthHint)
        }
    }
    return fmt.Errorf("gcp secret manager: %w", err)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestGCPProvider(t *testing.T) {
    t.Run("name_returns_gcp_secret_manager", ...)
    t.Run("fetch_single_secret_success", ...)
    t.Run("fetch_command_format_is_correct", ...)      // assert exact gcloud command
    t.Run("fetch_quotes_path_with_spaces", ...)
    t.Run("fetch_quotes_project_with_special_chars", ...)
    t.Run("fetch_not_found_returns_sentinel", ...)     // NOT_FOUND → ErrSecretNotFound
    t.Run("fetch_unauthenticated_returns_auth_error", ...)
    t.Run("fetch_permission_denied_returns_auth_error", ...)
    t.Run("fetch_generic_error_wraps_original", ...)
    t.Run("fetch_returns_json_for_field_extraction", ...)  // behavior 3
    t.Run("bulk_fetch_calls_fetch_per_path", ...)
    t.Run("bulk_fetch_returns_all_results", ...)
    t.Run("bulk_fetch_stops_on_first_error", ...)
    t.Run("validate_config_success", ...)
    t.Run("validate_config_cli_not_found_returns_auth_error", ...)
    t.Run("validate_config_unauthenticated_returns_auth_error", ...)
}
```

#### Impact on Existing Tests
- No existing tests affected (purely additive new file).

---

### Step 2: 1Password provider (RED → GREEN)
**Rationale:** More complex than GCP due to dual-mode fetch (`op://` URIs vs plain item names) and the required "CLI not installed" error behavior. Isolating this in its own step limits the blast radius from the dual-mode logic.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/onepassword_test.go` | create | Table-driven tests for OnePasswordProvider (RED phase) |
| `internal/vault/onepassword.go` | create | OnePasswordProvider implementation (GREEN phase) |

#### New Code (`internal/vault/onepassword.go`)
```go
package vault

import (
    "context"
    "fmt"
    "strings"
)

const onePasswordAuthHint = "Sign in to 1Password CLI: run 'op signin'. See https://developer.1password.com/docs/cli/sign-in-manually/"
const onePasswordInstallHint = "Install the 1Password CLI: https://developer.1password.com/docs/cli/get-started/"

var onePasswordAuthErrorPatterns = []string{
    "not currently signed in",
    "not signed in",
    "sign in",
    "session expired",
    "authorization",
}

var onePasswordNotFoundPatterns = []string{
    "no item named",
    "isn't an item",
    "not found",
}

// OnePasswordProvider fetches secrets from 1Password via the op CLI.
type OnePasswordProvider struct {
    execute CommandExecutor
}

// NewOnePasswordProvider creates a provider that shells out to the op CLI.
func NewOnePasswordProvider(exec CommandExecutor) *OnePasswordProvider {
    return &OnePasswordProvider{execute: exec}
}

func (p *OnePasswordProvider) Name() string { return Provider1Password }

func (p *OnePasswordProvider) Fetch(ctx context.Context, path string) (string, error) {
    var cmd string
    if strings.HasPrefix(path, "op://") {
        cmd = fmt.Sprintf("op read %s", shellQuote(path))
    } else {
        cmd = fmt.Sprintf("op item get %s --format json", shellQuote(path))
    }
    out, err := p.execute(ctx, cmd)
    if err != nil {
        return "", p.classifyError(err, path)
    }
    return out, nil
}

func (p *OnePasswordProvider) BulkFetch(ctx context.Context, paths []string) (map[string]string, error) {
    results := make(map[string]string, len(paths))
    for _, path := range paths {
        val, err := p.Fetch(ctx, path)
        if err != nil {
            return nil, err
        }
        results[path] = val
    }
    return results, nil
}

func (p *OnePasswordProvider) ValidateConfig() error {
    _, err := p.execute(context.Background(), "op whoami")
    if err != nil {
        return p.classifyError(err, "")
    }
    return nil
}

func (p *OnePasswordProvider) classifyError(err error, path string) error {
    msg := err.Error()
    // CLI not installed — behavior 4: surface install URL
    if strings.Contains(msg, "command not found") || strings.Contains(msg, "op: not found") {
        return fmt.Errorf("%w: op CLI not installed. %s", ErrProviderAuth, onePasswordInstallHint)
    }
    for _, pattern := range onePasswordNotFoundPatterns {
        if strings.Contains(msg, pattern) {
            return fmt.Errorf("%w: %s", ErrSecretNotFound, path)
        }
    }
    for _, pattern := range onePasswordAuthErrorPatterns {
        if strings.Contains(msg, pattern) {
            return fmt.Errorf("%w: %s. %s", ErrProviderAuth, msg, onePasswordAuthHint)
        }
    }
    return fmt.Errorf("1password: %w", err)
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestOnePasswordProvider(t *testing.T) {
    t.Run("name_returns_1password", ...)
    t.Run("fetch_op_uri_uses_op_read", ...)            // behavior: op:// → op read
    t.Run("fetch_plain_name_uses_op_item_get", ...)    // plain name → op item get --format json
    t.Run("fetch_op_uri_command_format_is_correct", ...)
    t.Run("fetch_plain_name_command_format_is_correct", ...)
    t.Run("fetch_quotes_path_with_spaces", ...)
    t.Run("fetch_not_found_returns_sentinel", ...)     // "no item named" → ErrSecretNotFound
    t.Run("fetch_not_signed_in_returns_auth_error", ...)
    t.Run("fetch_session_expired_returns_auth_error", ...)
    t.Run("fetch_cli_not_installed_returns_auth_error_with_install_url", ...)  // behavior 4
    t.Run("fetch_generic_error_wraps_original", ...)
    t.Run("bulk_fetch_calls_fetch_per_path", ...)
    t.Run("bulk_fetch_returns_all_results", ...)
    t.Run("bulk_fetch_stops_on_first_error", ...)
    t.Run("validate_config_success", ...)              // op whoami succeeds
    t.Run("validate_config_cli_not_installed_suggests_install_url", ...)  // behavior 4
    t.Run("validate_config_not_signed_in_returns_auth_error", ...)
}
```

#### Impact on Existing Tests
- No existing tests affected (purely additive new file).

---

### Step 3: Wire both providers into `NewProvider` factory
**Rationale:** Now that both providers compile and pass tests, adding them to the factory is a two-line change with minimal risk. Done after providers exist so the compiler validates the calls.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver.go` | modify | Add two case branches |

#### Current Code
```go
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
```

#### New Code
```go
switch cfg.Provider {
case ProviderAWS:
    return NewAWSProvider(cfg.Region, exec), nil
case ProviderAzure:
    return NewAzureProvider(cfg.VaultName, exec), nil
case ProviderHashiCorp:
    return NewHashiCorpProvider(cfg.Address, cfg.Auth, exec), nil
case ProviderGCP:
    return NewGCPProvider(cfg.Project, exec), nil
case Provider1Password:
    return NewOnePasswordProvider(exec), nil
default:
    return nil, fmt.Errorf("%w: %q", ErrUnknownProvider, cfg.Provider)
}
```

#### Impact on Existing Tests
- `TestNewProvider/unknown_provider_returns_error` remains valid; GCP and 1Password no longer fall through to default.

---

### Step 4: Compile-time interface checks
**Rationale:** Enforces at compile time that both new types fully satisfy `Provider`. Follows existing project convention.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/provider_test.go` | modify | Add two compile-time guard lines |

#### Current Code
```go
// Compile-time check that HashiCorpProvider satisfies Provider.
var _ Provider = (*HashiCorpProvider)(nil)
```

#### New Code (append after existing checks)
```go
// Compile-time check that GCPProvider satisfies Provider.
var _ Provider = (*GCPProvider)(nil)

// Compile-time check that OnePasswordProvider satisfies Provider.
var _ Provider = (*OnePasswordProvider)(nil)
```

#### Impact on Existing Tests
- No impact; additive lines.

---

### Step 5: Integration tests in `resolver_test.go`
**Rationale:** Validates the complete Resolve path for both new providers through the factory. These tests exercise `NewProvider` + `Fetch` together, confirming the wiring and field extraction work end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver_test.go` | modify | Add GCP and 1Password test cases to TestNewProvider and TestResolve |

#### New Test Cases for `TestNewProvider`

```go
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
```

#### New Test Cases for `TestResolve`

```go
t.Run("resolve_gcp_simple_key", func(t *testing.T) {
    // asserts gcloud secrets versions access command is called with secret name and project
})

t.Run("resolve_gcp_with_field_extraction", func(t *testing.T) {
    // returns JSON secret; #field extracts the named field
})

t.Run("resolve_1password_op_uri", func(t *testing.T) {
    // op:// path triggers op read
})

t.Run("resolve_1password_item_with_field_extraction", func(t *testing.T) {
    // plain item name triggers op item get --format json; #field extracts field
})
```

#### Impact on Existing Tests
- All existing `TestNewProvider` and `TestResolve` cases continue to pass.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/vault/resolver_test.go` | `TestNewProvider` | extended | add GCP and 1Password cases |
| `internal/vault/resolver_test.go` | `TestResolve` | extended | add GCP and 1Password cases |
| `internal/vault/provider_test.go` | (compile checks) | extended | add two guard lines |
| All other existing tests | — | none | — |

---

## Risks and Edge Cases

- **Risk: `shellQuote` defined in `aws.go`** — All providers use it because they share the `vault` package. This is the established pattern for Azure and HashiCorp; maintain it.
  → **Mitigation:** No change needed; consistent with existing providers.

- **Risk: GCP `NOT_FOUND` vs `not found` message variance** — `gcloud` may emit different casing or wording across SDK versions.
  → **Mitigation:** Check for both `"NOT_FOUND"` and `"not found"` (case-sensitive) in `classifyError`.

- **Risk: 1Password dual-mode fetch confusion** — A user storing an `op://` URI in a vault key alongside a `#field` extractor would receive the raw value from `op read` (already field-resolved), causing `ExtractField` to fail because the value is not JSON.
  → **Mitigation:** Document in test names that `op://` URIs should be self-contained (include the field segment). Tests will cover this edge case.

- **Risk: `op whoami` exit code variance** — Older `op` CLI versions may behave differently. Prefer `op whoami` over `op --version` since it tests auth state, not just installation.
  → **Mitigation:** `classifyError` checks "command not found" first (not installed) then auth patterns. Generic fallback wraps unexpected errors.

- **Risk: 1Password `op item get --format json` output structure** — The `op` CLI returns a deeply nested JSON structure (with `fields` array), not a flat `{"key": "value"}` map. The `#field` extractor (`ExtractField`) expects top-level string values.
  → **Mitigation:** Tests will document that plain item-name paths are best used when the secret itself contains raw JSON. For structured 1Password items, users should use `op://` URIs (which return just the field value via `op read`).

- **Edge case: GCP secret path with slashes** — `shellQuote` handles slashes correctly (they don't need escaping in single-quoted strings).
  → **Handling:** Covered by `shellQuote`; no special logic needed.

---

## Verification

```bash
go build ./cmd/apitest
go test ./internal/vault/...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Configure GCP and 1Password vault profiles in apitest.yaml.
# Run go test ./internal/vault/... with mocked CLI calls for both providers.
go test ./internal/vault/... -v -run TestGCPProvider
go test ./internal/vault/... -v -run TestOnePasswordProvider
go test ./internal/vault/... -v -run TestNewProvider/gcp
go test ./internal/vault/... -v -run TestNewProvider/1password
```
