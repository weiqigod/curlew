# Improvement Report: M1-023

**Task:** Sensitive variable detection and redaction
**Date:** 2026-03-17
**Review:** management/reviews/M1-023-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Heuristic `IsSensitiveName` only applied to `col.Variables.Values`; variables from `.env`, `--env`, `--env-var`, `--var` with sensitive names not detected | Added `AddHeuristicNames` helper to `SensitiveSet`; expanded `main.go` to call it on all six variable sources (col, dotenv, envFile, envVar, CLI, projectCfg) | ✓ tests pass |
| 2 | Low | Missing test `TestRunCmd/sensitive_in_json_output_format` | Added `sensitive_redacted_in_json_format` subtest: runs with `--format json -v`, parses JSON, asserts `Authorization` header = `[REDACTED]` | ✓ tests pass |
| 3 | Low | Missing test `TestRunCmd/heuristic_token_var_redacted` | Added `heuristic_token_var_redacted` subtest: collection with `token: "secret_token_val"` and header `token: "{{token}}"`, asserts value is redacted in `-vv` output | ✓ tests pass |
| 4 | Low | Missing behavior 7 guard test: sensitive value not leaked in error messages | Added `error_message_does_not_leak_sensitive_value` subtest: collection with `password: "super_secret_password_val"` and undefined variable reference, asserts password value absent from stderr | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 92.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| bb093ed | test(variable): add failing test for dotenv heuristic sensitivity detection | #1 (RED) |
| 0f9a2da | feat(output): expand heuristic sensitivity detection to all variable sources | #1, #2, #3, #4 |

## Summary

4/4 findings resolved. 0 deferred.

**Finding #1 (Medium)** was the only code bug: `AddHeuristicNames` was added to `SensitiveSet` as a clean helper, and `main.go` now calls it on all six variable maps (collection, dotenv, environment file, `--env-var`, `--var`, project config). All other findings were test gaps filled with integration subtests inside `TestRunCmd_SensitiveRedaction`.
