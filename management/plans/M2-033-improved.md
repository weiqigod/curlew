# Improvement Report: M2-033

**Task:** WebSocket message buffering, expect patterns, and variable extraction
**Date:** 2026-04-10
**Review:** management/reviews/M2-033-review.md

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `data, _ := json.Marshal(arr)` silently discards the marshal error in `aggregateExtraction` (executor.go:385), violating the project "no swallowed errors" standard. | Changed to `data, marshalErr := json.Marshal(arr)` with an explicit `if marshalErr != nil { return nil }` guard, consistent with the project error-handling convention. | tests pass |

## Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Plan promised an explicit regression test asserting `message_raw` is NOT JSON-serialised, but `TestExecute_SendRawMessage` only sent `"PING"` (also valid JSON) without verifying non-serialisation | Added `TestExecute_SendRawMessageNotJSONEncoded` which sends `{"raw":"value"}` via `message_raw` and asserts the bytes received are identical to the input, not double-encoded as `"{\"raw\":\"value\"}"` | tests pass |
| 2 | Low | `TestParseFile_WebSocketSendMutualExclusion` table only covered `message + message_template`; the `message + message_raw` and `message_raw + message_template` combinations were untested | Extended the table with two new entries (`message_and_raw` and `raw_and_template`), added corresponding testdata fixtures `websocket_conflict_msg_raw.yaml` and `websocket_conflict_raw_template.yaml` | tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/websocket`) | 84.8% |
| Coverage (`internal/websocket/templates`) | 88.9% |
| Coverage (`internal/parser`) | 89.0% |
| Coverage (`internal/runner`) | 86.7% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 2aa7d5e | fix(websocket): handle json.Marshal error in aggregateExtraction | Iter 2 #1 |
| 9f58305 | test(websocket): add missing regression and mutual-exclusion tests | Iter 1 #1, #2 |

## Summary

3/3 total findings resolved across 2 iterations. 0 deferred.
