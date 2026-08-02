# Verification Report: M6-003

**Task:** Redact sensitive values from request and response bodies
**Verified by:** AI
**Date:** 2026-04-21
**Branch:** feature/M6-003-body-redaction
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass (cached + fresh) |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage `internal/variable` | 96.7% | Meets >= 80% threshold |
| Coverage `cmd/apitest` | 81.7% | Meets >= 80% threshold |
| Coverage total | 86.7% | Meets >= 80% threshold |

## Observable Output

```
=== RUN   TestRedact_BodyAcrossFormats
=== RUN   TestRedact_BodyAcrossFormats/terminal_vv
=== RUN   TestRedact_BodyAcrossFormats/json_format
=== RUN   TestRedact_BodyAcrossFormats/tap_format
=== RUN   TestRedact_BodyAcrossFormats/junit_format
=== RUN   TestRedact_BodyAcrossFormats/allow_sensitive_shows_secret
--- PASS: TestRedact_BodyAcrossFormats (0.01s)
    --- PASS: TestRedact_BodyAcrossFormats/terminal_vv (0.00s)
    --- PASS: TestRedact_BodyAcrossFormats/json_format (0.00s)
    --- PASS: TestRedact_BodyAcrossFormats/tap_format (0.00s)
    --- PASS: TestRedact_BodyAcrossFormats/junit_format (0.00s)
    --- PASS: TestRedact_BodyAcrossFormats/allow_sensitive_shows_secret (0.00s)
PASS
ok      github.com/peterlindqvist/apitest/cmd/apitest   0.351s
```

Expected: PASS with all subtests passing and secret "hunter2" absent from non-allow-sensitive runs
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Request body: sensitive value replaced with redaction marker before any formatter | `TestRedact_BodyAcrossFormats/terminal_vv` | PASS |
| 2 | Response body: echoed sensitive value replaced before any formatter | `TestRedact_BodyAcrossFormats/{terminal_vv,json_format,tap_format,junit_format}` | PASS |
| 3 | `--allow-sensitive` skips redaction (opt-out preserved) | `TestRedact_BodyAcrossFormats/allow_sensitive_shows_secret` | PASS |
| 4 | JSON structure preserved; only string leaves rewritten | `TestRedactBody/{json_string_object,json_string_nested,map_string_any}` | PASS |
| 5 | Non-JSON text: substring replaced; partial-token not replaced | `TestRedactBody/{plain_text_substring,plain_text_partial_not_replaced}` | PASS |
| 6 | Existing header-redaction tests unchanged | `TestRunCmd_SensitiveRedaction` (8 subtests) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 6 behaviors pass as shown above | PASS |
| 2 | `go test ./...` passes | CI gate output: all packages PASS | PASS |
| 3 | `golangci-lint run` passes with 0 issues | CI gate: 0 issues | PASS |
| 4 | `internal/variable` coverage >= 80% | 96.7% | PASS |
| 5 | End-to-end test greps every format's output for secret | `TestRedact_BodyAcrossFormats` covers terminal, json, tap, junit, allow-sensitive | PASS |
| 6 | Smoke test passes | `./smoke/run.sh` clean in CI gate | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — pure transform functions; no swallowed errors; nil guards on all public receivers |
| Input validation | PASS — nil body, empty string, empty []byte, nil SensitiveSet, zero-value SensitiveSet, empty values all handled |
| Naming conventions | PASS — no stuttering; `RedactBody`, `AddValue`, `Values` all have doc comments; private helpers use short descriptive names |
| Code organization | PASS — `internal/variable/redact.go` single-purpose; `addSensitiveValues` unexported, local to main.go |
| Correctness | PASS — longest-first ordering prevents prefix collisions; JSON round-trip preserves structure; `--allow-sensitive` opt-out preserved |
| Test quality | PASS — table-driven tests; nil/zero-value guard paths exercised; all 6 behaviors covered |

Branch A: Review PASS trusted (Iteration 2 re-review), spot-check clean:
- Error handling site: `RedactBody` has nil guards and returns body unchanged (no errors to wrap — correct)
- Exported symbol: `RedactBody` has full multi-line doc comment (lines 9-19 of redact.go)
- Test quality: `TestRedactBody` is table-driven with 15 cases covering all body types and edge cases

## Commits

| Hash | Message |
|------|---------|
| 230d863 | docs(review): add passing review for M6-003 |
| de5f767 | docs(review): add improvement report for M6-003 |
| 862a7bc | fix(variable): resolve all M6-003 review findings |
| 7dee72e | docs(review): add review with findings for M6-003 |
| be02567 | chore(task): mark M6-003 as review |
| 7f4ef65 | feat(cli): wire RedactBody for request and response bodies in run command |
| 8056d57 | test(cli): add failing E2E test for body redaction across formats |
| 528291d | feat(variable): implement RedactBody with JSON-walk and plain-text fallback |
| 0173ad6 | test(variable): add failing tests for RedactBody |
| 2fb8a61 | feat(variable): add AddValue/Values to SensitiveSet for body redaction |
| 4d86ce9 | test(variable): add failing tests for SensitiveSet value tracking |
| bb38717 | chore(task): mark M6-003 as in_progress |
| 43bdec3 | chore(task): mark M6-003 as planned |
| 1c704c5 | docs(plan): add implementation plan for M6-003 |

TDD pattern visible: `test(...)` commits precede corresponding `feat(...)` commits throughout.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/variable/sensitive.go` | modified | Added `values` field, `AddValue`, `Values`, updated `Merge` |
| `internal/variable/sensitive_test.go` | modified | Added `TestSensitiveSet_ValueTracking`, `TestSensitiveSet_MergeValues`, `TestSensitiveSet_ZeroValue`, `TestSensitiveSet_NilSafe` extensions |
| `internal/variable/redact.go` | created | `RedactBody` with JSON-walk and plain-text fallback |
| `internal/variable/redact_test.go` | created | Table-driven tests: `TestRedactBody`, `TestRedactBody_EmptySet`, `TestRedactBody_NilSet`, `TestRedactBody_LongestValueFirst` |
| `cmd/apitest/main.go` | modified | `addSensitiveValues` helper, `AddValue` population loop, `RedactBody` calls for request and response bodies |
| `cmd/apitest/main_test.go` | modified | `TestRedact_BodyAcrossFormats` with 5 subtests |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
