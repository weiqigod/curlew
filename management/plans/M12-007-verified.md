# Verification Report: M12-007

**Task:** $formatDate and $parseDate using Go reference-time layouts
**Verified by:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-007-format-parse-date
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages, 0 fail |
| `go test -race ./...` (via ci-local.sh) | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 96.6% | Meets >= 80% threshold |

## Observable Output

```
=== RUN   TestRegistry_ParseDate
--- PASS: TestRegistry_ParseDate (0.00s)
    --- PASS: TestRegistry_ParseDate/dd/mm/yyyy_hh:mm:ss (0.00s)
=== RUN   TestRegistry_FormatDate_FromUnix
--- PASS: TestRegistry_FormatDate_FromUnix (0.00s)
=== RUN   TestRegistry_FormatDate_FromIso
--- PASS: TestRegistry_FormatDate_FromIso (0.00s)
    --- PASS: TestRegistry_FormatDate_FromIso/dd/mm/yyyy (0.00s)
=== RUN   TestRegistry_FormatDate_Now
--- PASS: TestRegistry_FormatDate_Now (0.00s)
PASS
ok  	github.com/peterlindqvist/apitest/internal/variable	0.375s
```

Expected: All four observable tests pass.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `$formatDate` with digit string parses as Unix seconds | `TestRegistry_FormatDate_FromUnix` | PASS |
| 2 | `$formatDate` with RFC3339 string reformats with layout | `TestRegistry_FormatDate_FromIso` | PASS |
| 3 | Unrecognisable input returns structured error naming input shape | `TestRegistry_FormatDate_BadInput` | PASS |
| 4 | `$parseDate` returns `t.UTC().Format("2006-01-02T15:04:05Z")` | `TestRegistry_ParseDate` | PASS |
| 5 | Bogus layout returns layout verbatim (layout-as-template) | `TestRegistry_FormatDate_LayoutAsTemplate` | PASS |
| 6 | Non-UTC zone-tagged input to `$parseDate` converted to UTC | `TestRegistry_ParseDate_NonUTCZone` | PASS |
| 7 | Arity 0/1/3 returns `DYNFN_ARITY` error naming arity 2 | `TestRegistry_FormatDate_ParseDate_arity_errors` | PASS |
| 8 | Empty input returns structured error naming empty input | `TestRegistry_FormatDate_EmptyInput`, `TestRegistry_ParseDate_EmptyInput` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 13 tests PASS | PASS |
| 2 | `go test ./...` passes | All packages pass | PASS |
| 3 | `go test -cover ./internal/variable/... >= 80%` | 96.6% | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint step clean | PASS |
| 5 | `./smoke/run.sh` passes | ci-local.sh smoke step clean | PASS |
| 6 | `./scripts/ci-local.sh` passes | Exit 0, `=== ci-local PASS ===` | PASS |
| 7 | `docs/MANUAL.md §3.7` documents both functions, layouts, and footgun | Lines 1062-1157 verified | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Doc comments on exports | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (management/reviews/M12-007-review.md verdict PASS). Spot-check:
- `parseDateToUTC`: `Inner: err` wrapping present, doc comment present.
- `formatDate`: doc comment present at line 361.
- `TestRegistry_FormatDate_BadInput`: table-driven, verifies `Code`, `Message` content, `Hint`, and truncation — tests what it claims.

## Commits

| Hash | Message |
|------|---------|
| d67aaa2 | docs(review): add passing review for M12-007 |
| 858b2bf | chore(task): mark M12-007 as review |
| 4e78472 | docs(plan): update MANUAL.md §3.7 with $formatDate, $parseDate, Go reference-time layouts |
| 9d9293e | feat(variable): implement $formatDate with isAllDigits helper (Step 2 GREEN) |
| 1c2e032 | test(variable): add failing tests for $formatDate (Step 2 RED) |
| 6c927ee | feat(variable): implement $parseDate with parseDateToUTC helper (Step 1 GREEN) |
| afeb186 | test(variable): add failing tests for $parseDate (Step 1 RED) |
| bea4be8 | chore(task): mark M12-007 as in_progress |
| 26ee41b | chore(task): mark M12-007 as planned |
| eb46fd5 | docs(plan): add implementation plan for M12-007 |

TDD pattern visible: `test(...)` commits precede `feat(...)` commits for both step 1 and step 2.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/variable/dynamic.go` | modified | Added `$formatDate`, `$parseDate` registrations + `formatDate`, `parseDateToUTC`, `isAllDigits` helpers |
| `internal/variable/dynamic_test.go` | modified | 13 new tests; `Available()` count bumped to 26 |
| `docs/MANUAL.md` | modified | §3.7 updated: 2 table rows + "Go reference-time layouts" subsection |
| `management/tasks/M12-007.yaml` | modified | Status tracking |
| `management/backlog.yaml` | modified | Status tracking |
| `management/plans/M12-007-plan.md` | added | Implementation plan |
| `management/reviews/M12-007-review.md` | added | Code review report (PASS) |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
