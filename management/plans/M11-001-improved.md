# Improvement Report: M11-001

**Task:** JUnit XML output ungating (remove Professional-tier feature gate)
**Date:** 2026-04-25
**Review:** management/reviews/M11-001-review.md

## Resolved Findings (iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `IMPROVEMENT.md` line 34: JUnit XML table row Notes column still read "Standard testsuites/testsuite/testcase. Professional-tier gated." — inaccurate after ungating, even though the §2.4 bullet at line 59 was correctly annotated with "Shipped (M11-001)". | Changed the Notes cell to "Standard testsuites/testsuite/testcase. Free tier (ungated in M11-001)." | ✓ tests pass |

## Resolved Findings (iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `testdata/agent-harness/feature-gate-denied/env.txt` line 1: comment read "Force the free tier so the junit_xml gate fires." but `junit_xml` was removed; fixture now triggers `parallel_execution` via `--parallel 2`. | Updated comment to: "Force the free tier so the parallel_execution gate fires (--parallel 2). Migrated from --format junit in M11-001 when JUnit was ungated." | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 81.6% (`cmd/apitest`), 86.9% total |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 84691c0 | fix(docs): update IMPROVEMENT.md JUnit table row to reflect ungating | iteration 1 #1 |
| 7862996 | fix(testdata): update stale junit_xml comment in feature-gate-denied env.txt | iteration 2 #1 |

## Summary
2/2 findings resolved across 2 review iterations. 0 deferred.
