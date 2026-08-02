# Improvement Report: M3-006

**Task:** openapi import: headers, request bodies, and status assertions
**Date:** 2026-04-14
**Review:** management/reviews/M3-006-review.md

## Resolved Findings (Round 1 — prior review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `grep -q "name:"` in smoke test always passes because it matches the collection `name:` field, not the request body | Replaced with `grep -q "body:" && grep -q "name: string"` | ✓ tests pass |
| 2 | Medium | Behavior 4's named `examples` (plural) map path completely untested | Added `TestImport_RequestBody_FromNamedExamples` | ✓ tests pass |
| 3 | Medium | `for _, ex := range mt.Examples` iterates a Go map non-deterministically | Sort map keys before ranging | ✓ tests pass |
| 4 | Low | Fallback content-type loop for `application/vnd.api+json` untested | Added `TestImport_RequestBody_FallbackContentType` | ✓ tests pass |
| 5 | Low | `strRef(s string)` declares unused parameter `s` | Removed the `s` parameter; updated all 9 call-sites | ✓ tests pass |

## Resolved Findings (Round 2 — current review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Content-type fallback loop iterates `map[string]*MediaType` without sorting — non-deterministic when multiple json-variant types are present | Collect keys, `sort.Strings`, iterate in order; added `TestImport_RequestBody_FallbackContentType_Deterministic` that imports the same spec 20× and asserts the lexicographically first key always wins | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/openapi`) | 96.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7cf6378 | fix(openapi): remove unused parameter from strRef helper | Round 1 #5 |
| 0049dd4 | fix(openapi): deterministic named examples + tests for untested branches | Round 1 #2, #3, #4 |
| f715f1e | fix(smoke): replace false-positive body grep with specific assertions | Round 1 #1 |
| 1d48b86 | fix(openapi): sort content-type keys before fallback iteration | Round 2 #1 |

## Summary

6/6 findings resolved across two review rounds. 0 deferred.
