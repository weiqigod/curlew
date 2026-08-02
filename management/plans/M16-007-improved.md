# Improvement Report: M16-007 (iteration 4)

**Task:** Trial activation endpoint with CLI subcommand and Claims struct extension
**Date:** 2026-05-10
**Review:** management/reviews/M16-007-review.md

## Resolved Findings (iteration 4)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `licenseDebugOut` did not expose `trial_state` or `features[]` in its JSON output map. Task observable explicitly states these fields must be visible via `apitest license --debug` after trial activation. | Added `claimSliceFn` helper; added `"trial_state"` and `"features"` keys to the debug output map in `licenseDebugOut`. Added `TestLicenseDebug_ExposesTrialStateAndFeatures` which mints a JWT with `trial_state=active` and `features=["vault_provider_profiles"]`, runs `--debug`, parses JSON output, and asserts both fields are present with correct values. | ✓ tests pass |

## Resolved Findings (iteration 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `licenseTrialStartOut`: the `feature == ""` guard at line 585 was untested — `apitest license trial start ""` reaches the guard but no test exercised it | Added `TestLicenseTrialStartOut_empty_feature_returns_1` — calls `captureRun("license","trial","start","")`, asserts exit 1 and "feature name cannot be empty" in stderr | ✓ tests pass |
| 2 | Low | `licenseTrialStartExitForError`: the else-branch (409 TRIAL_ALREADY_CONSUMED with no `previous_grant` extension) was untested — only 409 responses with `previous_grant` were covered | Added `TestLicenseTrialStartOut_already_consumed_no_previous_grant_returns_5` using an inline httptest server that omits `previous_grant`; asserts exit 5, "Trial already consumed for feature" in stderr, absence of "previously granted" | ✓ tests pass |
| 3 | Low | `licenseTrialStartExitForError`: the `ErrServerError` branch (backend returns plain HTTP 500) was untested; `"server_500"` behavior existed in `newTrialServer` but no test called it | Added `TestLicenseTrialStartOut_server_error_returns_6` using `newTrialServer(t, "server_500", "")`; asserts exit 6 and "5xx" in stderr | ✓ tests pass |
| 4 | Low | `licenseTrialStartExitForError`: the `default:` catch-all branch was untested — any unrecognised future error type would silently have untested behaviour | Added `TestLicenseTrialStartExitForError_default_returns_1` that calls `licenseTrialStartExitForError` directly with `fmt.Errorf("unexpected internal error")`; asserts exit 1 and "trial activation failed" in stderr | ✓ tests pass |

## Resolved Findings (iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `CheckFeatureWithClaims` defined but never called in production — vault_provider_profiles gate in `runner.go` still called `CheckFeature` (no claims); Behavior 6 was dead code | Added `TrialClaims auth.TrialChecker` field to `VarSources`; changed `buildScope` (runner.go:1007) to call `auth.CheckFeatureWithClaims(reg, "vault_provider_profiles", tier, vars.TrialClaims)`; added `TestRun_VaultGate/active_trial_overrides_tier_gate_for_vault` | ✓ tests pass |
| 2 | Low | `licenseTrialCmdOut` default-branch (unknown subcommand) and `start` with missing-arg branches untested | Added `TestLicenseTrialCmdOut_unknown_subcommand_returns_1` and `TestLicenseTrialCmdOut_start_missing_feature_returns_1`; `licenseTrialCmdOut` now at 100% coverage | ✓ tests pass |
| 3 | Low | `parsePreviousGrant` json.Unmarshal error path untested | Added `TestStartTrial_409_malformed_previous_grant_returns_nil_grant` — injects `"previous_grant": "not-an-object"` and asserts `PreviousGrant()` returns nil without panic | ✓ tests pass |
| 4 | Low | `licenseTrialStartExitForError` — `ErrRefreshExpired` (exit 4), `ErrRefreshReused` (exit 5), `ErrDeviceMismatch` (exit 7) branches untested | Added `TestLicenseTrialStartOut_refresh_expired_returns_4`, `_refresh_reused_returns_5`, `_device_mismatch_returns_7` using `newRefreshCodeTrialServer` helper | ✓ tests pass |
| 5 | Low | DoD item 7 (smoke test for `apitest license trial --help`) unimplemented | Added `=== License Trial Help (M16-007) ===` section to `smoke/run.sh` | ✓ verified |

## Resolved Findings (iteration 1 — carried forward)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Behavior 8 (previous-grant date in 409) not implemented | Extended `ProblemDetails.Extensions`; added `TrialPreviousGrant` + `parsePreviousGrant`; `trialSentinelError.PreviousGrant()` exposed | ✓ |
| 2 | High | already_consumed test didn't assert previous-grant date | Updated test to assert `"previously granted"` substring | ✓ |
| 3 | Medium | `trialSentinelError.Error()` at 0% coverage | Added `TestStartTrial_409_error_string_format` | ✓ |
| 4 | Medium | Missing C# JWT claim tests (Behaviors 4 and 7) | Added two `TrialsEndpointsTests` verifying `features[]` + `trial_state=active` | ✓ |
| 5 | Medium | `ResolveUserContextAsync` duplicated across endpoints | Extracted to `Auth/Shared/UserContext.cs` | ✓ |
| 6 | Low | Missing slug validation (`^[a-z0-9_]+$`) | Added `featureSlugRE` guard + test | ✓ |
| 7 | Low | `userId is null` returned 400 instead of 401 | Changed to `HttpResults.Unauthorized()` | ✓ |

## Out of Scope (Deferred)

No findings deferred. All findings from all iterations resolved.

## Quality Gate (iteration 3)

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`cmd/apitest`) | 81.5% |
| Coverage (`licenseTrialStartExitForError`) | 100% (was 73.7%) |

### Coverage by package (iteration 3)

| Package | Coverage |
|---------|----------|
| `internal/license` | 88.7% |
| `internal/auth` | 89.8% |
| `internal/backend` | 84.8% |
| `cmd/apitest` | 81.5% |
| `internal/runner` | 84.8% |

## Fix Commits (iteration 4)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2bd5bd4b | fix(license): expose trial_state and features[] in --debug output | #1 |

## Fix Commits (iteration 3)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| bbe78079 | test(license): add missing coverage for 4 trial exit branches | #1, #2, #3, #4 |

## Fix Commits (iteration 2)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f4e7f164 | fix(runner): wire CheckFeatureWithClaims at vault_provider_profiles gate | #1 |
| c02bb63a | test(license,backend): add missing coverage for trial error branches | #2, #3, #4 |
| a012024f | test(smoke): add License Trial Help smoke check (M16-007) | #5 |

## Quality Gate (iteration 4)

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (`cmd/apitest`) | 81.5% |
| Coverage (`internal/license`) | 88.7% |
| Coverage (`internal/auth`) | 89.8% |
| Coverage (`internal/backend`) | 84.8% |

## Summary

17/17 findings resolved across all four iterations. 0 deferred.
