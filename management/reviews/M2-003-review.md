# Code Review: M2-003

**Task:** Vault CLI subcommand (gated placeholder)
**Reviewer:** AI
**Date:** 2026-03-26
**Branch:** feature/M2-003-vault-cli-subcommand

## Verdict: PASS

## Findings

No findings. All 3 findings from the previous review have been resolved:

| # | Previous Finding | Resolution |
|---|-----------------|------------|
| 1 | Unused `noColor` parameter in `vaultListCmd` | Removed from signature; `noColor` remains in `vaultCmd` where it is used for gate error path |
| 2 | Missing edge case test for secrets-present-but-empty-keys | Added `TestVaultCmd_solo_tier_empty_keys` verifying config validation rejects empty keys |
| 3 | Smoke test not updated for `vault list` | Added smoke cases for `vault list` exit 6, `vault list --format json` gated JSON, and help text "vault list" |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned with context; `errors.As` correctly extracts `*auth.GateError`; JSON write errors caught; `os.Getwd` and `LoadProjectConfig` errors reported to stderr |
| Input Validation | PASS | `parseVaultArgs` handles nil/empty args, missing `--format` value, unknown args, duplicate subcommands; `cfg.Secrets == nil` checked before field access |
| Naming | PASS | Consistent with existing patterns (`WriteVaultListJSON` matches `WriteInfoJSON`); no stuttering; doc comments on all exported symbols |
| Code Organization | PASS | No dead code; vault functions grouped logically; JSON types in `internal/output`; `currentTier` var for test override is clean |
| Correctness | PASS | Map iteration sorted with `sort.Strings`; `os.Chdir` restored via `t.Cleanup`; `currentTier` override restored; no goroutine or resource leaks |
| Test Quality | PASS | All 5 specified behaviors covered; edge cases tested (empty args, no profiles, empty keys, unknown args); table-driven tests with `t.Run`; smoke tests updated |

## Test Coverage
- Coverage: 85.5% (cmd/curlew), 92.3% (internal/output), 86.7% total
- All vault code paths exercised: gate path (Free tier), dispatch path (Solo tier), no-profiles path, empty-keys validation error path

## Behavior Verification

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | `curlew vault` at Free tier → exit 6 + gate message | `TestVaultCmd_free_tier` (4 cases) |
| 2 | `curlew vault --format json` at Free tier → JSON gate error | `TestVaultCmd_free_tier` (5 cases) |
| 3 | `curlew vault list` at Solo tier → provider names + key counts | `TestVaultCmd_solo_tier` (3 cases) |
| 4 | `curlew vault list --format json` at Solo tier → JSON array | `TestVaultCmd_solo_tier` (3 cases) |
| 5 | `curlew vault list` with no profiles → helpful message | `TestVaultCmd_solo_tier_no_profiles` (2 cases) |

## Summary
Clean implementation after improvement pass. All previous findings resolved. The vault subcommand correctly gates at Free tier (preserving existing behavior), dispatches to `vault list` at Solo tier, handles no-profiles and invalid-config edge cases, and produces both terminal and JSON output with sorted keys. Code is well-tested with 27 vault-specific test cases plus smoke tests.
