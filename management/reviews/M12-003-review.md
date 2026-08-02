# Code Review: M12-003

**Task:** $urlEncode and $jsonEncode dynamic functions
**Reviewer:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-003-url-json-encode

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned, none swallowed. `json.Marshal` blank-identifier is intentional and documented — `json.Marshal` of a Go string never errors; golangci-lint errcheck reports 0 issues. `fmt.Errorf("$%s: %w", name, err)` wrapping in `Evaluate` preserves the chain. |
| Input Validation | PASS | Both functions accept any string (no runtime failure mode). Arity validation is handled by the existing `oneArg`/`arityError` wiring, which returns structured `DYNFN_ARITY` errors. Empty string, unicode, and edge-case inputs are tested. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. Inline comments explain the `b, _ := json.Marshal(s)` blank identifier and the query-component vs path-segment distinction. |
| Code Organization | PASS | Two registrations added at the end of the `register()` block, consistent with the M12-002 pattern. No package boundary violations. `encoding/json` and `net/url` are stdlib — no new external dependencies. |
| Correctness | PASS | `url.QueryEscape` and `json.Marshal(string)` are both correct for the stated contract. The count assertion in `TestRegistry_available_sorted` is bumped from 17 to 19 to match the two new registrations. The `b, _ := json.Marshal(s)` blank-identifier is safe because `json.Marshal` of a `string` value never returns a non-nil error. |
| Test Quality | PASS | Table-driven tests with `t.Run` subtests. All 7 behaviors from the task YAML are covered. `TestRegistry_JsonEncode` includes a `json.Unmarshal` round-trip sanity check on every case. `TestRegistry_UrlEncode_NestedVar` covers behavior 6 (nested template interpolation). `TestRegistry_UrlJsonEncode_arity_errors` covers behavior 5 for both functions. |

## Behavior Coverage

| # | Behavior | Test(s) |
|---|----------|---------|
| 1 | `$urlEncode` → `net/url.QueryEscape` (space→+, &→%26, /→%2F, UTF-8 percent-encoded) | `TestRegistry_UrlEncode` (8 table cases) |
| 2 | `$jsonEncode` → RFC 8259 JSON-string literal including surrounding double quotes | `TestRegistry_JsonEncode` ("plain ascii" case + round-trip check) |
| 3 | Quote and backslash escaped per RFC 8259 | `TestRegistry_JsonEncode` ("contains quote", "contains backslash" cases) |
| 4 | Literal newline → `\n` in output; result is valid JSON | `TestRegistry_JsonEncode` ("contains newline" case + json.Unmarshal round-trip) |
| 5 | Arity mismatch returns structured `DYNFN_ARITY` error naming the function | `TestRegistry_UrlJsonEncode_arity_errors` (4 cases: 0/2 args for each function) |
| 6 | Nested template `{{$urlEncode('{{query}}')}}` resolves correctly | `TestRegistry_UrlEncode_NestedVar` |
| 7 | Empty `$jsonEncode('')` → literal `""` (3 chars) | `TestRegistry_JsonEncode` ("empty string" case) |

## Test Coverage
- Coverage: 96.2% (`go test -cover ./internal/variable/...`)
- No missing coverage areas related to this task.

## Summary

The implementation is a clean, minimal extension of the M12-001/M12-002 wiring: two new entries in `register()`, reusing `oneArg` unchanged, with appropriate inline documentation. All 7 task behaviors are covered by table-driven tests with descriptive subtest names, including edge cases (empty string, unicode, nested template interpolation, and arity errors for both zero and two arguments). The MANUAL.md update correctly adds two table rows and a clarifying prose paragraph, and removes `$urlEncode` from the "forthcoming" parenthetical. No issues found at any severity level.
