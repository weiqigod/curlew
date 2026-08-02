# Improvement Report: M2-006

**Task:** GCP Secret Manager and 1Password CLI providers
**Date:** 2026-03-27
**Review:** management/reviews/M2-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `"command not found"` in `gcpAuthErrorPatterns` caused misleading auth hint when gcloud was not installed | Removed `"command not found"` from the auth pattern slice; added `gcpInstallHint` const; added early-return check in `classifyError` before the auth loop — mirrors the 1Password provider pattern | ✓ tests pass |
| 2 | Low | `validate_config_cli_not_found_returns_auth_error` missing install URL assertion | Added `strings.Contains(err.Error(), "https://cloud.google.com/sdk/docs/install")` assertion to the test (written as failing RED test before fixing gcp.go) | ✓ tests pass |
| 3 | Low | No binary-level vault list test for GCP and 1Password (behavior 5 gap) | Added `TestVaultCmd_solo_tier_gcp` and `TestVaultCmd_solo_tier_1password` to `cmd/curlew/main_test.go` asserting provider name and key count appear in `vault list` output | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 95.6% (vault package), 91.4% total |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `7553065` | fix(vault): distinguish gcloud CLI not installed from auth failure | #1, #2 |
| `0e18371` | test(vault): add binary-level vault list tests for GCP and 1Password | #3 |

## Summary
3/3 findings resolved. 0 deferred.
