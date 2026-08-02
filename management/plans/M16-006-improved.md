# Improvement Report: M16-006

**Task:** ITrialStateResolver and LicenseTokenIssuer wiring with tier-upgrade preemption
**Date:** 2026-05-10
**Review:** management/reviews/M16-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TrialSeederService.cs:63-68` — `DbUpdateException` race-condition catch swallowed silently, no log entry | Added `log.LogDebug("trial_seed_race_discarded user_id={UserId} — concurrent seed already committed", userId)` inside the catch block before `return 0` | ✓ tests pass |
| 2 | Low | `DatabaseTrialStateResolver.cs:30` — Redundant `Kind != PreemptedBySubscription` filter with no explanation | Added a clarifying belt-and-suspenders comment explaining that `ExpiresAt = now()` at preemption already excludes preempted rows, but the explicit Kind guard makes intent unambiguous at read-time | ✓ tests pass |
| 3 | Low | `AdminBootstrapTests.cs:61-79` — Primary happy-path test `RunAsync_empty_db_creates_admin_user_and_default_org` does not assert trial row count | Added `(await scope.Db.Trials.CountAsync()).Should().Be(TrialFeatures.All.Count)` assertion to the test; confirmed it passes with `EnsureCreatedAsync()` path | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (Go) | 87.3% overall; C# backend tests: 29/29 pass |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 84d686b9 | fix(trials): address review findings #1, #2, #3 for M16-006 | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
