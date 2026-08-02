# Code Review: M1-026

**Task:** AI exec command
**Reviewer:** AI
**Date:** 2026-03-19
**Branch:** feature/M1-026-ai-exec-command
**Review round:** 3 (post-improve from round 2)

## Verdict: PASS

## Findings

No findings. All issues from rounds 1 and 2 have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w` and descriptive context. `AppendJSONL` uses named return for deferred close-error propagation. Sentinel errors used where callers need `errors.Is()` matching. No swallowed errors. |
| Input Validation | PASS | Empty stdin, invalid JSON, missing URL, conflicting flags (`--stdin` + URL), missing flag values all produce clear error messages. 10MB `io.LimitReader` prevents memory exhaustion. Unknown format rejected. |
| Naming | PASS | No stuttering. `ExecOptions`, `stdinRequest`, `JSONLEntry` follow Effective Go. Doc comments on all exported symbols. Short names in tight scopes. |
| Code Organization | PASS | `internal/output/jsonl.go` is a clean, focused addition. `execCmd` follows established patterns from `runCmd`. No circular dependencies. `defer` used for file cleanup. Minimal exported surface. |
| Correctness | PASS | Dry-run JSON now initializes `Assertions` to `[]` (round 2 fix verified). Method defaults, `-X` override, variable interpolation, context propagation all correct. `StatusCode`/`DurationMs` use `omitempty` for dry-run entries. Race detector passes. |
| Test Quality | PASS | Table-driven tests with descriptive `t.Run` names. Integration tests exercise real binary with stdin pipe. JSONL tests verify create, append, valid JSON, newline termination, omitempty. 13 `execCmd` test cases covering happy and error paths. `TestExecCmd_dryRunJsonSchema` regression test covers the round 2 fix. |

## Test Coverage
- Package coverage: 84.9% for `cmd/apitest`, 91.2% for `internal/output`
- `AppendJSONL`: 85.7%
- `parseExecArgs`: 91.1%
- `parseStdinRequest`: 92.9%
- `execCmd`: 66.7% (some error branches like log-write failures and variable interpolation errors are hard to trigger in tests; overall package exceeds 80% threshold)
- All tests pass with `-race`

## Behavior Coverage

| # | Behavior | Tests |
|---|----------|-------|
| 1 | exec --stdin with piped JSON | `TestExecCmd/stdin_json_executes_request`, `TestExecIntegration/stdin_json_via_binary` |
| 2 | exec with inline URL/method | `TestExecCmd/inline_url_executes_request`, `TestExecIntegration/inline_url_via_binary` |
| 3 | --dry-run shows details, no HTTP | `TestExecCmd/dry_run_*`, `TestExecIntegration/dry_run_via_binary` |
| 4 | --log appends JSONL entry | `TestExecCmd_logFileContent`, `TestExecCmd_dryRunLogEntry`, `TestExecIntegration/log_file_created_via_binary` |
| 5 | --non-interactive suppresses prompts | `TestExecCmd/non_interactive_suppresses_prompts` (trivially satisfied — exec has no interactive prompts; flag accepted for AI-agent signaling) |
| 6 | --format json matches run schema | `TestExecCmd_formatJsonSchema`, `TestExecCmd_dryRunJsonSchema` |
| 7 | Invalid JSON -> clear error | `TestParseStdinRequest/invalid_json_returns_error`, `TestExecCmd/invalid_stdin_json_returns_error`, `TestExecIntegration/invalid_json_via_binary` |

## Previous Findings (All Resolved)

| Round | # | Severity | Finding | Status |
|-------|---|----------|---------|--------|
| 1 | 1 | Medium | `AppendJSONL` deferred close swallowed errors (no named return) | Fixed: named return `(err error)` |
| 1 | 2 | Low | `StatusCode`/`DurationMs` lacked `omitempty` — dry-run showed zeros | Fixed: `omitempty` added |
| 2 | 1 | Medium | Dry-run JSON `Assertions` field was nil (`null` in JSON, not `[]`) | Fixed: initialized to `make([]output.JSONAssertion, 0)` |

## Summary

The exec command implementation is solid. All 3 findings from rounds 1 and 2 have been correctly resolved. Error handling follows project conventions, input validation is thorough, tests cover all 7 specified behaviors, and coverage exceeds the 80% threshold. Race detector passes. Code is ready for verification.
