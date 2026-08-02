# Improvement Report: M9-001

**Task:** request_slug: derive per-request identifier and emit in events schema v1.2
**Date:** 2026-04-25
**Review:** management/reviews/M9-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `TestEvents_v12_RequestSlug` listed in DoD but did not exist anywhere in the codebase | Added comprehensive `TestEvents_v12_RequestSlug` to `internal/output/events/emitter_test.go` with 5 sub-tests: schema_version 1.2 on both events, slug present on request.start across 5 name derivation cases, slug present on request.end paired with start, omitempty for empty slug on both events, multiple paired requests with distinct slugs | ✓ tests pass |
| 2 | High | `TestEmitter_GoldenRunHappy` and `TestEmitter_GoldenRunFailedAssertion` did not set `RequestSlug` on `RequestEndInput`, causing golden files to omit slug on `request.end` | Added `RequestSlug: "create-user"` and `RequestSlug: "get-user"` to `RequestEndInput` calls in both golden helpers; regenerated golden files with `UPDATE_GOLDEN=1` | ✓ tests pass |
| 3 | High | `TestRunner_RequestSlugAllEmitSites` was missing "parallel main waves", "data-driven parallel", and "websocket main" sub-tests | Added three sub-tests: "parallel main waves" (3 independent items via parallel executor), "data-driven parallel" (data-driven with `parallel: true`, verifies iteration slug prefix and start/end match), "websocket main" (single WS request via fake dialer) | ✓ tests pass |
| 4 | Low | Two doc-comments in `schema_test.go` (lines 94, 161) said "v1.1 JSON Schema" but the helper now compiles v1.2 | Updated both comments to say "v1.2 JSON Schema" | ✓ build clean |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage `internal/parser` | 90.1% |
| Coverage `internal/runner` | 85.1% |
| Coverage `internal/output/events` | 96.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 016514e | fix(events): add RequestSlug to golden test helpers and fix comments | #2, #4 |
| 822019d | test(events): add TestEvents_v12_RequestSlug | #1 |
| ec71fb5 | test(runner): add parallel, data-driven parallel, and websocket sub-tests to TestRunner_RequestSlugAllEmitSites | #3 |

## Summary

4/4 findings resolved. 0 deferred.
