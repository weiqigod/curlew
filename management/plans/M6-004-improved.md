# Improvement Report: M6-004 (Iteration 3)

**Task:** Events emitter package with NDJSON event types
**Date:** 2026-04-21
**Review:** management/reviews/M6-004-review.md

## Resolved Findings (Iteration 1 — 6 findings)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `EmitRunStart` used `e.atMs()` instead of hardcoded `0`, violating schema contract that `RunStart` is always `at_ms=0` | Hardcoded `AtMs: 0` in `EmitRunStart` with doc comment | ✓ tests pass |
| 2 | High | Single `body_encoding` field was semantically ambiguous when request body is text and response body is binary | Replaced with per-body `request_body_encoding` and `response_body_encoding` fields in struct and schema | ✓ tests pass |
| 3 | Medium | `truncateBody` returned `size=len(raw)` for non-truncated binary bodies, emitting `body_size` when not truncated (schema violation) | Return `size=0` for non-truncated binary bodies so `omitempty` suppresses the field | ✓ tests pass |
| 4 | Medium | `TestEmitter_RequestEnd_RegisteredSentinelHint` used direct `*Structured` error, bypassing `ClassifyError` sentinel-lookup path | Rewrote to use `fmt.Errorf("context: %w", parser.ErrInvalidYAML)` so `ClassifyError` walks the chain | ✓ tests pass |
| 5 | Low | `TestEmitter_BodyTruncation_OverLimit` only tested response body truncation; request body path uncovered | Added `request_body` sub-test verifying `request_body_truncated=true` and `request_body_size` | ✓ tests pass |
| 6 | Low | `BodyLimit()` exported but never called in tests (0% coverage) | Added `TestEmitter_BodyLimit` with sub-tests for default and override values | ✓ tests pass |

## Resolved Findings (Iteration 2 — 1 finding)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 7 | Medium | `EmitRunStart` passed `nil` cliArgs directly into the `RunStart` struct. A nil `[]string` marshals as JSON `null` rather than `[]`, violating the JSON Schema's `"type":"array"` constraint on `cli_args`. No test exercised the nil path. | Added `if cliArgs == nil { cliArgs = []string{} }` before constructing the struct. Added `TestEmitter_RunStart_NilCLIArgs` which calls `EmitRunStart(nil, "", "")` and asserts `cli_args` is a JSON array (not null). | ✓ tests pass |

## Resolved Findings (Iteration 3 — 3 findings)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 8 | Medium | `Options.CurlewVersion` not validated in `NewEmitter`; empty string silently produces schema-invalid `run.start` events | Added guard `if opts.CurlewVersion == "" { return nil, errors.New("events: Options.CurlewVersion is required") }` in `NewEmitter`. Updated `TestEmitter_RunID_Default` and `TestEmitter_BodyLimit` to pass a valid version. Added `TestEmitter_EmptyCurlewVersion` confirming the error. | ✓ tests pass |
| 9 | Low | `EmitRunEnd` called `e.atMs()` twice — for `AtMs` and `DurationMs` — risking different values if the clock advanced between calls | Captured `atMs := e.atMs()` once before constructing the struct; used for both fields. | ✓ tests pass |
| 10 | Low | Binary-body-over-limit truncation path in `truncateBody` not exercised; `truncateBody` at 83.3% coverage | Added `TestEmitter_BodyTruncation_BinaryOverLimit` with a 4096-byte NUL-containing body asserting `response_body_encoding=base64`, `response_body_truncated=true`, `response_body_size=4096`. `truncateBody` is now 100.0%. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/output/events`) | 95.7% |

## Fix Commits (All Iterations)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `1624d00` | fix(events): hardcode at_ms=0 in EmitRunStart | #1 |
| `0631b86` | fix(events): per-body encoding fields and fix binary size for non-truncated | #2, #3 |
| `43dc882` | test(events): use real parser sentinel in TestEmitter_RequestEnd_RegisteredSentinelHint | #4 |
| `8b6f078` | test(events): cover request body truncation and BodyLimit accessor | #5, #6 |
| `7d10884` | chore(events): fix gofumpt struct field alignment | lint gate |
| `1e4e8ef` | fix(events): normalize nil cliArgs to empty slice in EmitRunStart | #7 |
| `cb70d1c` | fix(events): validate CurlewVersion in NewEmitter | #8 |
| `659c1e2` | fix(events): capture atMs once in EmitRunEnd | #9 |
| `d5b3782` | test(events): add binary-body-over-limit truncation test | #10 |

## Summary

10/10 findings resolved across 3 iterations. 0 deferred.
