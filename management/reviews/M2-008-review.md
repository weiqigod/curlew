# Code Review: M2-008

**Task:** Auth profile configuration and execution
**Reviewer:** AI
**Date:** 2026-03-27
**Branch:** feature/M2-008-auth-profile-config-execution

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Errors wrapped with `%w`; sentinel errors used and matchable via `errors.Is`. `%w: %w` double-wrap in `profile.go:61` is valid Go 1.20+ and intentionally wraps both `ErrProfileFailed` and the underlying cause. `%s` for inner YAML decode error in `config/project.go:71` is consistent with pre-existing file pattern and callers only need `ErrInvalidProjectConfig`. No swallowed errors. |
| Input Validation | PASS | `ParseProjectConfig` validates `type` and `collection` fields with clear errors. `ExecuteProfiles` handles nil/empty profiles (returns empty result, not panic). |
| Naming | PASS | No stuttering. All exported types, functions, and constants have doc comments. Package names lowercase and single-word. |
| Code Organization | PASS | Package boundaries respected (`internal/auth`, `internal/config`, `internal/runner`, `internal/variable` cleanly separated). No circular dependencies. `defer` not needed (no file handles opened directly by changed code). |
| Correctness | PASS | Recursion prevention correct (`AuthProfiles = nil`, `ProjectRoot = ""`, `AuthExecuteFunc = nil` in `RunForExtraction`). Scope-diff approach correctly captures new/changed variables. Sensitive marking applied to all auth profile variables. Guard rail counter not incremented for auth collections (uses separate counter inside `RunForExtraction` → `runPhases`). Gate check fires before auth execution. |
| Test Quality | PASS | `TestRunForExtraction` covers all extraction paths (file-not-found, success scope-diff, network failure, assertion failure, recursive prevention). `TestRun_AuthProfiles` covers all runner integration paths including gate blocking, variable injection, failure path, and guard rail counter isolation. `TestRunCmd_AuthProfileFailure_exitCode5` covers exit code 5 end-to-end. |

## Test Coverage

| Package | Coverage |
|---------|----------|
| `internal/auth` | 100% |
| `internal/config` | 96.5% |
| `internal/runner` | 90.2% |
| `internal/variable` | 96.4% |

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | Login collection runs before main requests | `TestRun_AuthProfiles/"auth profile variables available in main requests"` (auth executor called before HTTP exec) |
| 2 | Variables from auth profile available in main requests | `TestRun_AuthProfiles/"auth profile variables available in main requests"` (capturedURL assertion) |
| 3 | Auth profile failure → exit code 5, main requests skipped | `TestRunCmd_AuthProfileFailure_exitCode5`, `TestRun_AuthProfiles/"auth profile failure returns error"` |
| 4 | Auth profile variables treated as pre-execution (no inter-request deps) | Satisfied by design: variables injected via `scope.Set()` before all phases run |
| 5 | Auth profile variables automatically marked sensitive | `TestExecuteProfiles/"all returned variables are marked sensitive"` |
| 6 | Auth profile requests do not count toward 1,000-request guard rail | `TestRun_AuthProfiles/"auth profile requests do not increment main guard rail counter"` (`summary.RequestsExecuted == 1`) |

## Summary

All three findings from the first review are resolved: `TestRunForExtraction` now covers the full extraction function with five subtests; `ErrProfileNotFound` has been removed; `TestRunCmd_AuthProfileFailure_exitCode5` covers the exit-5 path end-to-end. The implementation is architecturally sound — auth profiles execute before all phases, extracted variables flow into the main scope as pre-execution variables, all returned variables are marked sensitive, the guard rail counter is isolated, and recursion is prevented by clearing `AuthProfiles` and `AuthExecuteFunc` in the inner run. No new findings.
