# Improvement Report: M6-007

**Task:** Validation harness: gate for v0.1 → v1.0 schema promotion
**Date:** 2026-04-22
**Review:** management/reviews/M6-007-review.md

## Iteration 1 — Resolved Findings (review iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `testdata/agent-harness/missing-variable/expect.yaml` omits `error.line_nonzero: true`, allowing a line-regression to pass the harness silently | Added `error.line_nonzero: true` under `must_have` to match behavior #1 and the plan's Step 2 table | ✓ tests pass |
| 2 | Low | `TestEmitter_AllKindsValidateAgainstSchema` doc comment says "v0.1 JSON Schema" after promotion | Updated comment to "v1.0 JSON Schema" | ✓ tests pass |
| 3 | Low | `TestEmitter_GoldenSchemaValidates` doc comment says "v0.1 JSON Schema" after promotion | Updated comment to "v1.0 JSON Schema" | ✓ tests pass |
| 4 | Low | `TestSchema_MarkdownExamplesValidate` doc comment references "EVENTS_SCHEMA_v0.1.md" and "v0.1 JSON Schema" after promotion | Updated both references to v1.0 | ✓ tests pass |
| 5 | Low | `valueEqual` (bool path, fmt.Sprintf fallback) and `toFloat64` (int64, float64 branches) had untested code paths | Added 4 new test cases to `TestContract_Match`: boolean exact match (pass and fail), float64 value match, and fallback string comparison | ✓ tests pass; harness package coverage 88.8% → 92.9% |

## Iteration 2 — Resolved Findings (review iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `toFloat64` `int64` branch remained untested after iteration 1 only added a `float64` test case; `toFloat64` showed 60% coverage | Added `"exact match on int64 value"` table entry to `TestContract_Match` constructing `MustHave` with `int64(200)` directly (YAML decodes small ints as `int`, so only a Go-level synthetic expectation reaches the `int64` branch); coverage for `toFloat64` rose to 80%, harness package overall 92.9% → 93.9% | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (overall) | 86.2% |
| Coverage (cmd/apitest-agent-harness) | 93.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| f3e8061 | fix(harness): resolve all review findings for M6-007 | iter-1 #1–#5 |
| f9fb81a | test(harness): add int64 branch coverage for toFloat64 | iter-2 #1 |

## Summary

6/6 total findings resolved across 2 iterations. 0 deferred.
