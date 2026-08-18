# Improvement Report: M27-001

**Task:** The prose register - checkable claims that are sentences, not rows
**Date:** 2026-08-18
**Review:** management/reviews/M27-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `CHANGELOG.md:219` stated the shrink-only table register lived at `internal/docs/table-execution-baseline.txt`; the file actually lives at `docs/table-execution-baseline.txt`, and the same changelog entry gets the path right twice more (lines 252, 727), making line 219 an internal inconsistency, not just a filesystem error. | Corrected the path at `CHANGELOG.md:219` from `internal/docs/table-execution-baseline.txt` to `docs/table-execution-baseline.txt`, matching the other two references in the same entry. Structural doc fix — no behaviour change, no TDD required. | ✓ `grep -n "table-execution-baseline.txt" CHANGELOG.md` now shows all three lines (219, 252, 727) agreeing on `docs/table-execution-baseline.txt`. |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS (all packages ok, including `internal/docs` and `cmd/curlew`) |
| `golangci-lint run` | PASS (0 issues) |
| `internal/docs` coverage | 81.1% of statements |
| Observable: `TestProse_inventory_is_complete` | PASS — 103 prose claims: 14 executed, 3 exempt (cap 12), 86 owed |
| Observable: `TestProse_register_cannot_grow` | PASS — all 7 subtests (clean + 3 mutation directions + exempt + unresolved-executor) |
| Regression: `TestDocTables_everyTableIsExecutedOrDeclaredProse` | PASS — 77 tables: 72 executed, 5 declared prose (cap 8), 0 owed |
| Regression: `cmd/curlew` `TestProse_*` | PASS — all 4 tests (command/flag/env-var name checks, ignore-marker count) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `76393ba` | fix(docs): correct table-execution-baseline.txt path in CHANGELOG | #1 |

## Summary
1/1 findings resolved. 0 deferred.
