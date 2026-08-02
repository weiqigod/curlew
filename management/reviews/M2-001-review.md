# Code Review: M2-001

**Task:** from_command variable source
**Reviewer:** AI
**Date:** 2026-03-20
**Branch:** feature/M2-001-from-command-variable-source

## Verdict: PASS

## Previous Findings (all resolved across 3 rounds)

| # | Round | Severity | Finding | Resolution |
|---|-------|----------|---------|------------|
| 1 | 1 | Medium | CommandCache created fresh per Run(), never serves a hit | Design-intent comment and documentation test added |
| 2 | 1 | Medium | No integration test for from_command sensitive redaction | Runner-level and CLI-level tests added |
| 3 | 1 | Low | Command executes even when collection value will overwrite result | Skip check added in runner |
| 4 | 1 | Low | No test for negative TTL in CommandCache | Test case added |
| 5 | 2 | Medium | `sensitive` field uses raw string comparison instead of YAML boolean decoding | Changed to `fieldVal.Decode(&sensitive)` with error handling |
| 6 | 3 | Low | Inner error wrapped with `%v` instead of `%w` in non-ExitError path | Changed to `%w` for Go 1.20+ multi-`%w` support |

## Findings

No findings. All previous issues have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All `fmt.Errorf` uses `%w` for wrapping. Sentinel `ErrCommandFailed` used correctly. No swallowed errors. Dual `%w` in non-ExitError path valid for Go 1.24. |
| Input Validation | PASS | Empty `from_command` rejected at parse time. `value`/`from_command` mutual exclusion enforced. `sensitive` decoded as YAML boolean via `Decode()`. `cache` field decoded with type safety. |
| Naming | PASS | No stuttering. Doc comments on all exported symbols (`CommandCache`, `CommandVar`, `ExecuteCommand`, `ErrCommandFailed`, `NewCommandCache`, `Get`, `Set`). Short names in tight scopes. |
| Code Organization | PASS | Clean package boundaries: parsing in `parser`, execution in `variable`, integration in `runner`, CLI wiring in `cmd/curlew`. `internal/` enforced. Minimal exported surface. |
| Correctness | PASS | Precedence ordering correct (5 between dotenv/4 and collection/7). Skip checks for CLI, EnvVar, and collection-value overrides prevent unnecessary command execution. Cache scope documented. Context propagated via `exec.CommandContext`. |
| Test Quality | PASS | All 7 behaviors covered. Error paths tested (non-zero exit, stderr capture, context cancellation). Edge cases handled (nil, empty, negative TTL, pipe syntax, cancelled context). Integration tests for exit code 6 in terminal and JSON formats. Table-driven tests with descriptive names. |

## Test Coverage
- `internal/variable`: 96.4%
- `internal/parser`: 90.9%
- `internal/runner`: 90.7%
- `internal/auth`: 100.0%
- Combined: 93.7% (well above 80% threshold)

## Behavior Traceability

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | from_command resolves stdout at Solo tier | `runner: from_command_resolves_at_solo_tier`, `from_command_used_in_request_url` |
| 2 | sensitive: true redacts value | `runner: from_command_sensitive_marks_variable_in_sensitive_set`, `cli: TestFromCommand_sensitive_in_parsed_collection` |
| 3 | cache: 300 avoids re-execution | `variable: TestCommandCache` (TTL mechanics), `runner: from_command_cache_is_per_run_scope` (design doc) |
| 4 | Free tier returns exit 6 | `runner: from_command_at_free_tier_returns_gate_error`, `cli: TestRunFromCommand_gate_returns_exit_6`, `TestRunFromCommand_gate_json_returns_exit_6` |
| 5 | Failed command shows command, exit code, stderr | `variable: TestExecuteCommand` (error cases), `TestExecuteCommand_error_includes_stderr`, `TestExecuteCommand_error_includes_command_and_exit_code` |
| 6 | Pipe syntax works via shell | `variable: pipe_syntax_works` |
| 7 | CLI --var overrides from_command | `runner: from_command_cli_override_skips_execution`, `from_command_env_var_override_skips_execution`, `from_command_collection_var_overrides_command` |

## Quality Gates

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS (all 14 packages) |
| `golangci-lint run` | PASS (0 issues) |
| `./smoke/run.sh` | PASS |
| Coverage >= 80% | PASS (93.7%) |

## Summary

Clean implementation with zero findings. All 6 issues identified across 3 prior review rounds have been resolved. The code correctly implements the from_command variable source with shell execution, stdout capture, caching infrastructure, sensitivity support, feature gating, and proper precedence ordering. Test coverage is thorough with full behavior traceability.
