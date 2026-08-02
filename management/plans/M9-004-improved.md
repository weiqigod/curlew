# Improvement Report: M9-004

**Task:** markdown parallel + data-driven integration: wave grouping, per-iteration files
**Date:** 2026-04-25
**Review:** management/reviews/M9-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestRun_MarkdownFormat_ParallelWaves` used a flat 3-request collection with no extract dependencies, so all requests landed in wave 0 (one wave, not three). Did not verify three distinct wave headers or per-request wave_index values. | Replaced the flat collection with a chained A→B→C collection: A extracts `token`, B uses `{{token}}` and extracts `user_id`, C uses `{{user_id}}`. This forces three distinct waves. Assertions now verify `## Wave 0`, `## Wave 1`, `## Wave 2` all present in ascending order in run.md, and each of `a.md`, `b.md`, `c.md` carries `wave_index: 0`, `wave_index: 1`, `wave_index: 2` respectively. | ✓ tests pass |
| 2 | Low | The `## Sequential` section inside `renderRunMDByWave` (reached when `IsParallel=true` but some entries have `WaveIndex=-1`) was uncovered by any test. Function coverage was 75.0%. | Added `TestMarkdown_RunMD_WaveGrouping_MixedSequential` to `internal/output/markdown/run_md_test.go`. The test uses `IsParallel=true` with two waved entries (WaveIndex 0, 1) and one data-driven aggregate entry (WaveIndex -1). Asserts `## Wave 0`, `## Wave 1`, and `## Sequential` sections all present in the expected order, with `seed-users` appearing after `## Sequential`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (markdown package) | 92.3% |
| Coverage (runner package) | 85.1% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 21c4a67 | fix(test): strengthen parallel wave + sequential branch coverage | #1, #2 |

## Summary
2/2 findings resolved. 0 deferred.
