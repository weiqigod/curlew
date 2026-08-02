# Code Review: M1-017

**Task:** Global project config (apitest.yaml)
**Reviewer:** AI
**Date:** 2026-03-14
**Branch:** feature/M1-017-global-project-config

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`; `ErrInvalidProjectConfig` sentinel used correctly; no swallowed errors; `ParseProjectConfig` wraps file-not-found and YAML parse failures cleanly. |
| Input Validation | PASS | `FindProjectRoot` terminates at filesystem root (parent == dir check); nil `Project` map in `VarSources` is safe (iterating nil map is no-op in Go); empty/invalid paths fall through to OS error with context. |
| Naming | PASS | No stuttering; all exported symbols have doc comments; `projectFile` unexported helper needs none; all names are clear and idiomatic. |
| Code Organization | PASS | `internal/config` boundary respected; `flatten()` reused from same package without export; `defer` not needed (uses `os.ReadFile`); imports clean. |
| Correctness | PASS | Precedence chain (Project 2 < EnvFile 3 < DotEnv 4 < Collection 7 < EnvVar 9 < CLI 10) correctly implemented in `runner.go:70-81`; walk-up safely terminates at filesystem root; `LoadProjectConfig` handles `.yaml`/`.yml` fallback correctly; `.env` relocation to project root when found works correctly. |
| Test Quality | PASS | All five behaviors covered at unit and integration level; `TestRun_ProjectVariables` covers all six precedence cases including `dotenv overrides project`; `TestFindProjectRoot` grandparent case asserts correct root path; `TestLoadProjectConfig` asserts actual root path value; integration tests use real binary via `os/exec`; smoke test covers walk-up scenario and help text. |

## Test Coverage

- `internal/config/project.go`: **100%** (all three exported functions)
- `internal/runner/runner.go`: **90.9%**
- `cmd/apitest/main.go`: **84.6%**
- Overall project: **92.4%**

All five task behaviors have test coverage at unit and integration level. Smoke test includes project config walk-up and help text verification.

## Summary

The implementation is correct, clean, and complete. All three findings from the prior review (missing `dotenv overrides project` precedence test, unasserted grandparent path in `TestFindProjectRoot`, unasserted root path in `TestLoadProjectConfig`) have been resolved in commit 668fdfc. No new findings.
