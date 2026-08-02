# Code Review: M2-033

**Task:** WebSocket message buffering, expect patterns, and variable extraction
**Reviewer:** AI
**Date:** 2026-04-10
**Branch:** feature/M2-033-websocket-buffering-patterns
**Iteration:** 3 (post-improve, iteration 2 finding verified resolved)

## Verdict: PASS

## Findings

No findings. All prior findings from iterations 1 and 2 have been resolved.

## Resolved from Iteration 2

The sole finding from iteration 2 was correctly resolved:
- `data, _ := json.Marshal(arr)` changed to `data, marshalErr := json.Marshal(arr); if marshalErr != nil { return nil }` in `aggregateExtraction` (executor.go:385-388). The error is now named and checked; golangci-lint passes cleanly.

## Resolved from Iteration 1

Both findings from iteration 1 remain resolved:
1. `TestExecute_SendRawMessageNotJSONEncoded` verifies that a literal JSON object sent via `message_raw` arrives byte-for-byte identical to the input without double-encoding.
2. `TestParseFile_WebSocketSendMutualExclusion` table covers all three mutual-exclusion combinations: `message+template`, `message+raw`, and `raw+template`, each backed by a testdata fixture.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. `marshalErr` in `aggregateExtraction` is checked (named variable, guarded return). Sentinel errors used throughout (`ErrDialFailed`, `ErrNoSteps`, `ErrExpectTimeout`, `ErrExtractFailed`, `ErrSendFailed`). No swallowed errors. |
| Input Validation | PASS | Parser validates mutual exclusions (message/message_raw/message_template, message/any_of), rejects negative count, rejects variables without message_template. Nil request and empty steps handled at the top of Execute. |
| Naming | PASS | No stuttering. All exported symbols have doc comments (`Execute`, `Result`, `StepResult`, `LoadTemplate`, `ErrTemplateNotFound`). Package names are lowercase single-word. |
| Code Organization | PASS | `internal/websocket/templates/` mirrors `internal/graphql/files/` convention. Buffer owned exclusively by the executor step-loop goroutine. `internal/` boundaries respected throughout. Runner change is minimal (one line). |
| Correctness | PASS | Buffer FIFO semantics correct; copy-on-push prevents mutation aliasing. Count accumulation and JSON-array aggregation for count>1 correct. Scope isolation for step variables verified (`WithOverrides`/child scope). Warning fires exactly once. No data races (`go test -race` passes). |
| Test Quality | PASS | All 7 behaviors covered. Happy path, error path, boundary conditions (count=1 backward compat, partial timeout, interleaved noise), integration (runner-level warning propagation via `TestRun_WebSocketBufferWarningPropagates`). Testdata fixtures used for parser validation. |

## Test Coverage

- `internal/websocket`: 84.8%
- `internal/websocket/templates`: 88.9%
- `internal/parser`: 89.0%
- `internal/runner`: 86.7%

All packages exceed the 80% threshold.

## Behavior Coverage

| Behavior | Test(s) | Status |
|----------|---------|--------|
| Buffer checked first (instant match) | `TestExecute_BufferFIFO`, `TestExecute_BufferCarriesFramesAcrossSteps` | PASS |
| `any_of:` multi-pattern matching | `TestExecute_ExpectAnyOfMatchesFirst`, `TestExecute_ExpectAnyOfMatchesSecond`, `TestExecute_ExpectAnyOfNoneMatchTimesOut` | PASS |
| `count: N` collect array with extraction | `TestExecute_ExpectCountCollectsArray`, `TestExecute_ExpectCountPartialTimesOut`, `TestExecute_ExpectCountWithAnyOf` | PASS |
| Variable extraction available to subsequent steps | `TestExecute_ExpectExtractsVariable`, `TestExecute_ExtractedVariableAvailableInLaterStep` | PASS |
| Buffer warning at 100+ messages | `TestExecute_BufferWarningSurfaced`, `TestRun_WebSocketBufferWarningPropagates` | PASS |
| `message_template:` external file | `TestExecute_SendMessageTemplate`, `TestExecute_SendMessageTemplate_UsesScopeVars`, `TestExecute_SendMessageTemplate_StepVarsDoNotLeak`, `TestParseFile_WebSocketMessageTemplate` | PASS |
| `message_raw:` plain text (not JSON-serialized) | `TestExecute_SendRawMessage`, `TestExecute_SendRawMessageNotJSONEncoded` | PASS |

## Quality Gates

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `go test -race ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |

## Summary

The implementation is architecturally sound and all three findings from the two prior review iterations have been correctly resolved. The `json.Marshal` error in `aggregateExtraction` is now explicitly named and guarded (lint-clean). All 7 task behaviors are covered by tests, coverage exceeds 80% in every changed package, and the race detector is clean.
