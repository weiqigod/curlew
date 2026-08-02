# Code Review: M16-007

**Task:** Trial activation endpoint with CLI subcommand and Claims struct extension
**Reviewer:** AI
**Date:** 2026-05-10
**Branch:** feature/M16-007-trial-activation-endpoint
**Iteration:** 5 (post-improve iteration 4)

## Verdict: PASS

## Findings

No findings. All previous findings have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; sentinel errors defined; `parsePreviousGrant` defensive on malformed JSON; `trialSentinelError.Unwrap()` exposes both sentinel and `*ProblemDetails` for `errors.Is`/`errors.As`; `translateRefreshError` reused for symmetric AUTH_* sentinel routing |
| Input Validation | PASS | Empty slug and URL-unsafe slugs rejected client-side before any I/O (`featureSlugRE`); nil body + missing device_id/refresh_token returns 400 on backend; nil claims handled in `CheckFeatureWithClaims` |
| Naming | PASS | No stuttering; `TrialChecker` uses `-er` suffix; all exported symbols have doc comments; package names correct; `TrialPreviousGrant` and `TrialActivation` well-named |
| Code Organization | PASS | `UserContext.ResolveAsync` extracted to shared file; `TrialChecker` interface decouples `internal/auth` from `internal/license`; compile-time interface check in `trial_interface_test.go`; `TrialClaims auth.TrialChecker` field on `VarSources` cleanly wired to the `vault_provider_profiles` gate |
| Correctness | PASS | `CheckFeatureWithClaims` wired at runner.go vault gate; AD1 explicitly documents that deeper plumb-through (other features, `vaultCmdOut`) is a follow-up task; C# race handled via `DbUpdateException` re-read; `parsePreviousGrant` returns nil gracefully on malformed extension; `--debug` now exposes `trial_state` and `features` per task observable |
| Test Quality | PASS | All 9 behaviors from task YAML covered; all branches of `licenseTrialStartExitForError` tested (codes 1–7); `TestLicenseDebug_ExposesTrialStateAndFeatures` verifies observable; `TestStartTrial_409_malformed_previous_grant_returns_nil_grant` covers defensive path; coverage ≥80% across all changed packages |

## Previous Findings Resolution

Finding from iteration 4 resolved:
- Finding 1 (Medium): `licenseDebugOut` now exposes `trial_state` (line 491) and `features` (line 490) in its JSON output map using `claimStringFn`/`claimSliceFn`; `TestLicenseDebug_ExposesTrialStateAndFeatures` asserts both fields are present with correct values ✓

## Test Coverage

- `internal/license/jwt.go` (IsTrialActiveFor, TrialState/TrialExpiry fields): 88.7% package — PASS
- `internal/auth/gate.go` (CheckFeatureWithClaims, TrialChecker): 89.8% package — PASS
- `internal/backend/trial.go`: all paths covered including Error(), PreviousGrant(), malformed extension, sentinel routing — 84.8% package — PASS
- `cmd/curlew/license.go`: all trial branches tested including refresh-taxonomy exit codes (4, 5, 7), empty-feature guard, server 5xx, default catch-all — 81.5% package — PASS
- `internal/runner/runner.go`: active_trial_overrides_tier_gate_for_vault test covers wired-through gate — PASS
- Overall: all packages ≥80% — PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` passed:
- `go build`: OK
- `go test ./...`: all packages OK
- `go test -race ./...`: no races detected
- `golangci-lint run`: 0 issues
- `./smoke/run.sh`: PASS including "License Trial Help (M16-007)" smoke check

## Summary

The implementation is complete and correct. All nine behaviors from the task YAML are covered by passing tests. The iteration-4 finding (missing `trial_state` and `features` in `--debug` output) has been resolved — the debug map now includes both fields and a dedicated test verifies them. The `TrialChecker` interface cleanly decouples `internal/auth` from `internal/license` while preserving all existing call sites, and the one wired-through gate site (`vault_provider_profiles` in runner.go) demonstrates end-to-end trial bypass. Coverage is at or above 80% across all changed packages and the full CI gate passes clean.
