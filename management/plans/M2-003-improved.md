# Improvement Report: M2-003

**Task:** Vault CLI subcommand (gated placeholder)
**Date:** 2026-03-26
**Review:** management/reviews/M2-003-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `vaultListCmd` accepts unused `noColor` parameter — dead code | Removed `noColor` parameter from `vaultListCmd` signature and call site; parameter remains in `vaultCmd` where it is used for the gate error path | ✓ tests pass |
| 2 | Low | No test for secrets-present-but-empty-keys edge case | Added `TestVaultCmd_solo_tier_empty_keys` verifying exit 1 with config error when secrets block has provider but no keys (validation rejects empty keys) | ✓ tests pass |
| 3 | Low | Smoke test not updated for `vault list` subcommand | Added smoke cases: `vault list` exit 6, `vault list --format json` gated JSON, help text contains "vault list" | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 85.5% cmd/apitest, 92.3% internal/output, 90.9% total |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 571f8ed | fix(cli): remove unused noColor parameter from vaultListCmd | #1 |
| 1d4ead3 | test(cli): add edge case test for vault list with empty keys | #2 |
| 0f5a2c7 | test(smoke): add vault list smoke test cases | #3 |

## Summary
3/3 findings resolved. 0 deferred.
