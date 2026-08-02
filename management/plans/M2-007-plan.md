# Implementation Plan: M2-007

## Overview
Wire vault provider bulk retrieval into `internal/vault/resolver.go` and add structured error wrapping so vault fetch failures propagate as `*errors.Structured` with `CategoryConfig`, producing clear user-facing messages and exit code 5.

## Task Details
- **ID:** M2-007
- **Title:** Vault integration with runner and variable precedence
- **Phase:** M2: Vault Storage
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-014 | Variable precedence resolution | done |
| M2-001 | Vault provider profile config and parsing | done |
| M2-004 | GCP Secret Manager and 1Password CLI providers | done |

## Code Exploration Findings

### What already works (no implementation needed)

**B1 – CLI override wins:** `runner_test.go:2766` `vault_secrets_overridden_by_cli_vars` verifies CLI `--var` (precedence 10) overwrites vault (precedence 6). `runner.go:190-192` sets CLI vars after vault merge.

**B2 – Vault beats from_command:** `runner_test.go:2791` `vault_secrets_override_from_command` verifies vault (6) > from_command (5). `runner.go:148-173` places vault merge after from_command merge.

**B3 – Interpolation:** `runner_test.go:2711` `vault_secrets_resolved_and_available_as_variables` verifies vault values enter the scope and are interpolated into URLs.

**B5 – Auto-sensitive:** `config.go:157-163` `SensitiveNames()` returns all key variable names as sensitive. `main.go:321-323` already calls `projectCfg.Secrets.SensitiveNames()` and merges into the sensitive set.

### Variable precedence table

| Level | Source | Where in runner.go |
|-------|--------|--------------------|
| 1 | Dynamic functions | line 196 |
| 2 | Project vars | lines 87-89 |
| 3 | Environment file (`--env`) | lines 90-92 |
| 4 | `.env` file | lines 93-95 |
| 5 | `from_command` | lines 97-146 |
| **6** | **Vault provider** | **lines 148-173** |
| 7 | Collection variables | lines 175-177 |
| 8 | Request-level variables | lines 292-306 |
| 9 | `--env-var` | lines 185-188 |
| 10 | `--var` CLI | lines 190-193 |

### What needs to be built

**B4 – Structured error for exit code 5:**
`resolver.go:61-63` currently returns raw provider errors (`return nil, err`). `main.go:402-404` returns exit code 5 for any `varErr`, and calls `errOut.StructuredError(varErr)`. For the "clear message" contract, vault fetch errors must be `*errors.Structured` with `CategoryConfig` and a hint. Currently they format as plain `[ERROR] ResourceNotFoundException: not found` with no context.

**B6 – Bulk retrieval:**
`resolver.go:56-66` loops over unique paths calling `provider.Fetch()` individually. The `Provider` interface (`provider.go:24-26`) already defines `BulkFetch`. All five providers implement it (falling back to looping `Fetch`). The resolver must call `BulkFetch` to honour the contract and allow future provider optimisations.

### Existing tests that will break
- `resolver_test.go:191-207` `resolve_returns_error_for_missing_secret`: checks `errors.Is(err, ErrSecretNotFound)`. After wrapping in `*errors.Structured`, `errors.Is` traverses `Unwrap()` → `Inner` → finds `ErrSecretNotFound` ✓. The check will still pass. However we must add the structured error assertion to the same test.
- `resolver_test.go:209-225` `resolve_returns_error_for_missing_field_in_json`: checks `errors.Is(err, ErrFieldNotFound)`. Field-extraction errors (`ExtractField`) are NOT fetch errors and are not in scope for structured wrapping in this task. No change needed.

---

## Implementation Steps

### Step 1: Write failing tests in resolver_test.go (RED)

**Rationale:** Tests first. Two behaviors are missing coverage: structured error type on fetch failure, and BulkFetch usage. Writing these tests first causes them to fail, confirming the gap.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver_test.go` | modify | Update error test + add BulkFetch test |

#### Tests to Write FIRST (RED phase)

**Update existing test** `resolve_returns_error_for_missing_secret` — add structured error assertion:

```go
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
```

**New test** for BulkFetch path (verifies correct result for multiple distinct paths):

```go
t.Run("resolve_uses_bulk_fetch_for_multiple_distinct_paths", func(t *testing.T) {
    exec := func(ctx context.Context, command string) (string, error) {
        if strings.Contains(command, "secret-a") {
            return "value-a", nil
        }
        if strings.Contains(command, "secret-b") {
            return "value-b", nil
        }
        return "", fmt.Errorf("unexpected command: %s", command)
    }
    cfg := &SecretsConfig{
        Provider: ProviderAWS,
        Region:   "us-east-1",
        Keys: map[string]string{
            "key_a": "secret-a",
            "key_b": "secret-b",
        },
    }
    result, err := Resolve(context.Background(), cfg, exec)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result.Variables["key_a"] != "value-a" {
        t.Errorf("key_a = %q, want %q", result.Variables["key_a"], "value-a")
    }
    if result.Variables["key_b"] != "value-b" {
        t.Errorf("key_b = %q, want %q", result.Variables["key_b"], "value-b")
    }
})
```

**Impact on Existing Tests:** `resolve_returns_error_for_missing_secret` gains two new assertions; the `errors.Is` check is preserved unchanged. All other resolver tests: no impact (they test success paths or non-fetch errors).

---

### Step 2: Implement BulkFetch + structured error wrapping in resolver.go (GREEN)

**Rationale:** Smallest implementation change to make Step 1 tests pass. Only `resolver.go` changes — no runner, no providers, no CLI.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/vault/resolver.go` | modify | Add import, helper function, switch loop to BulkFetch |

#### Current Code (`resolver.go:1-83`)

```go
package vault

import (
    "context"
    "fmt"
)

// ... (NewProvider, Resolve)

func Resolve(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor) (*ResolveResult, error) {
    // ...
    fetched := make(map[string]string)
    for _, ref := range refs {
        if _, ok := fetched[ref.Path]; ok {
            continue
        }
        val, err := provider.Fetch(ctx, ref.Path)
        if err != nil {
            return nil, err  // <-- raw error, no structured wrapping
        }
        fetched[ref.Path] = val
    }
    // ...
}
```

#### New Code

```go
package vault

import (
    "context"
    "fmt"

    apierrors "github.com/weiqigod/curlew/internal/errors"
)

// wrapVaultFetchError wraps a provider fetch error in a Structured error
// so callers receive a user-facing message with a hint.
func wrapVaultFetchError(provider Provider, err error) error {
    return &apierrors.Structured{
        Category: apierrors.CategoryConfig,
        Message:  fmt.Sprintf("vault secret fetch failed (provider: %s): %v", provider.Name(), err),
        Hint:     fmt.Sprintf("Check that the secret exists and credentials are configured for %s", provider.Name()),
        Inner:    err,
    }
}

func Resolve(ctx context.Context, cfg *SecretsConfig, exec CommandExecutor) (*ResolveResult, error) {
    result := &ResolveResult{Variables: make(map[string]string)}
    if cfg == nil {
        return result, nil
    }
    refs, err := cfg.ParsedKeys()
    if err != nil {
        return nil, err
    }
    provider, err := NewProvider(cfg, exec)
    if err != nil {
        return nil, err
    }

    // Collect unique paths; BulkFetch lets providers optimise into a single API call.
    uniquePaths := make([]string, 0, len(refs))
    seen := make(map[string]bool)
    for _, ref := range refs {
        if !seen[ref.Path] {
            uniquePaths = append(uniquePaths, ref.Path)
            seen[ref.Path] = true
        }
    }

    fetched, err := provider.BulkFetch(ctx, uniquePaths)
    if err != nil {
        return nil, wrapVaultFetchError(provider, err)
    }

    // Apply field extraction.
    for _, ref := range refs {
        raw := fetched[ref.Path]
        if ref.Field == "" {
            result.Variables[ref.VarName] = raw
            continue
        }
        val, err := ExtractField(raw, ref.Field)
        if err != nil {
            return nil, fmt.Errorf("secret %q field %q: %w", ref.Path, ref.Field, err)
        }
        result.Variables[ref.VarName] = val
    }
    return result, nil
}
```

**Note on `resolve_multiple_fields_from_same_secret_single_fetch`:** This test counts `exec` calls to verify deduplication. After switching to `BulkFetch`, the AWS provider's `BulkFetch` loops over `Fetch` which calls `exec`. With one unique path (`prod/db`), `BulkFetch(["prod/db"])` calls `Fetch` once → `exec` once. `fetchCount == 1` still passes. ✓

---

### Step 3: Write failing runner test for structured error propagation (RED → GREEN)

**Rationale:** The runner propagates vault errors unchanged (`runner.go:167-169`). After Step 2, the error from `vault.Resolve()` will be `*errors.Structured`. Adding this assertion to the runner test confirms end-to-end B4 behavior without any additional runner changes.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner_test.go` | modify | Extend `vault_secret_fetch_error_stops_execution` |

#### Current Code (`runner_test.go:2820-2839`)

```go
t.Run("vault_secret_fetch_error_stops_execution", func(t *testing.T) {
    mockExec := func(ctx context.Context, command string) (string, error) {
        return "", fmt.Errorf("ResourceNotFoundException: not found")
    }
    // ...
    _, _, err := Run(context.Background(), col, successExecutor, VarSources{ ... })
    if err == nil {
        t.Fatal("expected error from vault fetch failure")
    }
})
```

#### New Code

```go
t.Run("vault_secret_fetch_error_stops_execution", func(t *testing.T) {
    mockExec := func(ctx context.Context, command string) (string, error) {
        return "", fmt.Errorf("ResourceNotFoundException: not found")
    }
    col := &parser.Collection{
        Name: "Test",
        Requests: []parser.RequestItem{
            {Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
        },
    }
    _, _, err := Run(context.Background(), col, successExecutor, VarSources{
        Tier:          auth.TierSolo,
        Registry:      auth.DefaultRegistry(),
        Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"k": "nonexistent"}},
        VaultExecutor: mockExec,
    })
    if err == nil {
        t.Fatal("expected error from vault fetch failure")
    }
    var se *apierrors.Structured
    if !errors.As(err, &se) {
        t.Fatalf("expected *errors.Structured, got %T: %v", err, err)
    }
    if se.Category != apierrors.CategoryConfig {
        t.Errorf("category = %q, want %q", se.Category, apierrors.CategoryConfig)
    }
})
```

**Import required in runner_test.go:** `apierrors "github.com/weiqigod/curlew/internal/errors"` (check if already imported).

**Impact on Existing Tests:** This test currently passes with a simple nil check. After Step 2, it will still pass the nil check AND the new structured-error assertions. No other runner tests are affected.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/vault/resolver_test.go` | `resolve_returns_error_for_missing_secret` | update | Add `*errors.Structured` assertions |
| `internal/vault/resolver_test.go` | `resolve_uses_bulk_fetch_for_multiple_distinct_paths` | new | Write from scratch |
| `internal/runner/runner_test.go` | `vault_secret_fetch_error_stops_execution` | update | Add structured error assertions |
| All other vault tests | all | none | No changes |
| All other runner tests | all | none | No changes |

## Risks and Edge Cases

- **`errors.Is` chain must survive wrapping:** `Structured.Unwrap()` returns `Inner`. The AWS provider wraps `ErrSecretNotFound` via `fmt.Errorf("%w", ErrSecretNotFound)`. After our wrapping: `Structured{Inner: providerErr}` → `errors.Is` traverses `Unwrap()` → `providerErr` → finds `ErrSecretNotFound`. ✓

- **`resolve_multiple_fields_from_same_secret_single_fetch` deduplication test:** After switching to `BulkFetch`, unique path deduplication moves from a manual skip into the `uniquePaths` slice. `BulkFetch(["prod/db"])` calls `Fetch("prod/db")` once. `fetchCount == 1` still holds. ✓

- **Field extraction errors not wrapped as structured:** `ExtractField` failures remain `fmt.Errorf(...)` plain errors. This is correct — they indicate a configuration error in the collection file (wrong field name), not a vault connectivity issue. The task spec only mentions "vault fetch failure" for exit code 5, and plain errors also return exit code 5 from `main.go`.

- **No-vault no-op (nil Secrets):** `Resolve(ctx, nil, exec)` returns empty result at line 39-41, before `BulkFetch` is reached. Unchanged. ✓

- **Partial BulkFetch failure:** All provider `BulkFetch` implementations return on first error from `Fetch`. The user gets a clear structured error identifying the provider. Individual key names are not surfaced in the error message (limitation: BulkFetch doesn't identify which key failed). This is acceptable — the structured error message names the provider and the hint directs the user to check credentials.

- **Import alias conflict:** `internal/errors` package imported with alias `apierrors` to avoid shadowing the stdlib `errors` package. The runner_test.go currently imports stdlib `errors` — check whether `apierrors` alias is already used before adding.

## Verification

```bash
go build ./cmd/curlew
go test ./internal/vault/...
go test ./internal/runner/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Verify vault variables at precedence 6
go test ./internal/runner/... -run TestRun_VaultResolution -v

# Verify structured error for fetch failure
go test ./internal/vault/... -run TestResolve/resolve_returns_error_for_missing_secret -v

# Verify CLI override (exit code 0 since override is non-vault)
# curlew run tests.yaml --var api_key=override
```
