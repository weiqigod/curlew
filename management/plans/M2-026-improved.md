# Improvement Report: M2-026

**Task:** JUnit XML output format
**Date:** 2026-04-08
**Review:** management/reviews/M2-026-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Feature gate check for `--format junit` happens after parse/loading error bailouts. Free-tier user with invalid file gets JUnit XML error output (exit 3) instead of feature gate rejection (exit 6). | Moved feature gate check to before `parser.ParseFile`, immediately after format validation. Added `TestRunCmd_format_junit_feature_gate_before_parse` to verify Free tier with invalid file returns exit 6. | tests pass |
| 3 | Low | When `--format junit` is gated at Free tier, gate error is sent as plain text to stderr. Inconsistent with `--parallel` gate which writes JUnit XML. | Changed gate rejection to output JUnit XML via `writeJUnitError(os.Stdout, gateErr)` instead of `errOut.StructuredError(gateErr)`, consistent with the `--parallel` gate pattern. | tests pass |
| 2 | Low | Missing integration test for skipped request behavior in JUnit output. | Added `TestRunCmd_format_junit_skipped` using a required setup that fails (unreachable server), causing main request to be skipped. Verifies `<skipped>` element appears in JUnit XML output and the specific testcase has the skipped element. | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 89.6% (total), buildJUnitOutput 100%, writeJUnitError 100%, WriteJUnitXML 75% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 0f1f607 | fix(junit): move feature gate check before parsing and output JUnit XML for gate error | #1, #3 |
| d6f645e | test(junit): add integration test for skipped request in JUnit output | #2 |

## Summary
3/3 findings resolved. 0 deferred.
