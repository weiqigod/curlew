# Code Review: M2-004

**Task:** Vault provider interface and AWS Secrets Manager provider
**Reviewer:** AI
**Date:** 2026-03-26
**Branch:** feature/M2-004-vault-provider-aws
**Review round:** 2 (re-review after `/improve`)

## Verdict: PASS

## Findings

No findings. All three issues from the prior review have been properly resolved:

| # | Prior Finding | Fix Verified |
|---|--------------|-------------|
| 1 | (High) Unquoted shell arguments in AWS CLI commands | `shellQuote` helper uses standard POSIX single-quote escaping (`'\''`). Tests cover spaces and embedded single quotes. |
| 2 | (Medium) Redundant per-call cache in `Resolve()` | Removed cache from `Resolve()`, kept `fetched` map for within-call dedup. `Cache` type retained for future cross-call use. |
| 3 | (Low) Exported `NewProvider(nil)` panics | Nil guard added, returns `ErrMissingRequiredField`. Test covers this path. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`, sentinel errors for known failures (`ErrSecretNotFound`, `ErrProviderAuth`, `ErrFieldNotFound`, `ErrNotJSONSecret`). `classifyError` correctly maps AWS CLI error patterns. No swallowed errors. |
| Input Validation | PASS | Shell arguments quoted via `shellQuote`. Nil guard on exported `NewProvider`. `Resolve` handles nil config gracefully. |
| Naming | PASS | No stuttering (`vault.Provider`, not `vault.VaultProvider`). Doc comments on all exports. Short names in tight scopes. Follows Effective Go. |
| Code Organization | PASS | Clean package boundaries (vault does not import runner). Minimal export surface. `defer` for mutex unlock. No unused imports. `shellQuote` unexported (internal helper). |
| Correctness | PASS | Shell quoting uses correct POSIX idiom. Cache is thread-safe with `sync.Mutex`. Dedup via `fetched` map is correct regardless of map iteration order. Precedence 6 placement in runner is correct (after from_command, before collection values). Race detector clean. |
| Test Quality | PASS | Table-driven tests where appropriate. Error paths covered with `errors.Is()`. Concurrent access tested. Integration tests verify precedence ordering in runner. Descriptive `t.Run()` names throughout. |

## Test Coverage
- vault package: 96.2%
- runner package: 90.0%
- Missing coverage: `BulkFetch` empty-paths early return (trivial)

## Spec Compliance

| Behavior | Status | Tests |
|----------|--------|-------|
| Valid credentials → secrets fetched and available | PASS | `fetch_single_secret_success`, `resolve_simple_keys_no_field`, `vault_secrets_resolved_and_available_as_variables` |
| `cache_ttl` → cached value returned within TTL | PASS | `TestCache` suite (7 cases). Within-call dedup: `resolve_multiple_fields_from_same_secret_single_fetch`. Cross-call caching deferred to future multi-run (Cache type ready). |
| `refresh_on_failure` → re-fetch on auth failure | DEFERRED | Per plan: "Out of scope for M2-004 (requires HTTP assertion integration)." `Cache.Invalidate` exists for future use. |
| Structured extraction (`prod/db#password`) | PASS | `TestExtractField` (8 cases), `resolve_with_field_extraction`, `vault_field_extraction_in_runner` |
| Invalid credentials → clear error with docs link | PASS | `fetch_invalid_credentials_returns_auth_error`, `fetch_expired_credentials_returns_auth_error`. Error includes AWS CLI docs URL. |
| Provider interface → only needs Fetch/BulkFetch/Name | PASS | Interface defined, compile-time satisfaction check `var _ Provider = (*AWSProvider)(nil)` |

## Quality Gates

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `golangci-lint run` | 0 issues |
| Coverage >= 80% | 96.2% (vault), 90.0% (runner) |

## Summary

Clean, well-structured implementation. The Provider interface is minimal and extensible. Error handling follows project standards throughout with proper wrapping and sentinel errors. The shell quoting fix is correct, the cache simplification removes dead code, and the nil guard prevents misuse of the exported API. Test coverage is excellent at 96% with good edge case and integration coverage.
