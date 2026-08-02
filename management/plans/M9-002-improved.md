# Improvement Report: M9-002 (Iteration 7)

**Task:** markdown formatter: --format markdown with sentinel splice, JSON body, run.md index
**Date:** 2026-04-25
**Review:** management/reviews/M9-002-review.md

## Resolved Findings (Iteration 7)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Parallel data-driven `RequestResult` literals do not populate `RequestID` or `RequestSlug`. The conversion loop at runner.go:2504 left these as zero values, contradicting the field's documented contract ("Always populated for non-skipped requests"). The sequential data-driven path (lines 2159, 2191) correctly set the fields; the parallel path did not. | Added `RequestID string` and `RequestSlug string` fields to `datadriven.IterationResult`. Populated both fields in the `execFn` for both the error path (`errIR`) and the success path (`ir`). Updated the conversion loop at runner.go:2504 to propagate `ir.RequestID` and `ir.RequestSlug` onto `RequestResult`. Added assertions in `TestRun_DataDriven_ParallelExecution` to guard the contract. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/output/markdown` | 87.7% |
| Coverage `internal/runner` | 85.1% |
| Coverage `cmd/curlew` | 80.9% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| ee0fafc | fix(runner): propagate RequestID/RequestSlug through parallel data-driven path | #1 |

## Summary

1/1 findings resolved. 0 deferred.

- Critical: 0 (none in this review)
- High: 0 (none in this review)
- Medium: 1 fixed (#1: parallel data-driven RequestResult had empty RequestID/RequestSlug)
- Low: 0 (none in this review)

## Prior Iterations

- Iteration 6 resolved 3 findings (parseSentinels multi-pair handling, dead return nil, env name branch coverage)
- Iteration 5 resolved 4 findings (skipped literals missing RequestSlug, dead nolint + unreachable code, deprecated os.IsNotExist, writeAtomic error paths)
- Iteration 4 resolved 4 findings (otherRunID fixture length, malformed-sentinel sub-cases, partial dead-default fix, reserved-slug doc comment)
- Iteration 3 resolved 1 finding (unused actionFreshWrite constant — see commit ee818fb)
- Iteration 2 resolved 3 findings (run_id coordination, unused param, dead switch case — see commit 629e5ab)
- Iteration 1 resolved 8 findings (see git history: 8ce05c2, 3cbc190)
