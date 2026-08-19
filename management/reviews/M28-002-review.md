# Code Review: M28-002

**Task:** The front-door files, and a quickstart that was executed
**Reviewer:** AI
**Date:** 2026-08-19
**Branch:** feature/M28-002-front-door-files-and-quickstart

## Pre-audit Gate

`./scripts/ci-local.sh --go` passes, including the two new named steps
("front-door files (M28-002)" and "README quickstart, executed (M28-002)").
Static audit proceeded.

## Verdict: FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Medium | Code Organization | `internal/docs/frontdoor.go` | 73, 83 | `GateMode.Accepts` and `GateMode.String` are exported but never called anywhere — not by production code, not by any test (`go tool cover -func` shows 0.0% for both). `AuditGate` (same file, lines 268-276) reimplements `Accepts`'s alias-matching loop inline instead of calling `m.Accepts(a)`, so the method exists purely as unexercised surface area. Violates the Code Organization standard ("No unused imports, variables, or functions"; "Exported surface is minimal — only export what other packages need"). | Either have `AuditGate` call `m.Accepts` and add a test for `String`, or delete both methods until something needs them. |
| 2 | Medium | Test Coverage / Spec Compliance | `.github/ISSUE_TEMPLATE/bug_report.yml`, `management/plans/M28-002-plan.md:412-418` | — | The plan explicitly commits: the bug template's `exit-code` dropdown is held to `exitcodes.Set(exitcodes.Reachable(...))` via `reachableExitCodes(t)` (`cmd/curlew/skill_exit_codes_test.go:100`), "so the observable's test stays about file presence and content." No such test exists — `grep -rln "bug_report" --include=*.go` under the repo returns only `internal/docs/frontdoor.go`, which checks the three field *names* (`IssueFields`) but never the exit-code *values*. Measured: the dropdown's codes (`0,1,2,3,4,5,130`) do match the live reachable set today, so nothing is broken now — but a future exit code added to or removed from the binary would leave `bug_report.yml` silently stale with nothing in the gate to catch it, which is exactly the drift class this task (and M21-001) exists to close. | Add the committed containment test in `cmd/curlew`, or if it was deliberately descoped, strike the claim from the plan so it stops reading as a delivered guarantee. |
| 3 | Low | Code Organization | `internal/docs/frontdoor.go` | 53-54, 62-70 (`internal/docs/hints_init.go`) | `ErrNoFrontDoorFiles` is declared and registered with `apierrors.RegisterPackage`, but is never returned by any function in the package. The "registry emptied out" guard the sentinel's own doc comment describes is instead implemented as a raw `len(docs.FrontDoor) < 6` check inline in `TestRepo_front_door_files_are_present` (`internal/docs/frontdoor_test.go:503-505`), so the sentinel has no producer anywhere in the codebase — a dead error that exists only to satisfy the sentinel-registration coverage test. | Wire it into `AuditFrontDoor`/`ReadFrontDoor` for the `len(files) == 0` case, or remove the sentinel and its registration. |
| 4 | Low | Code Organization | `internal/docs/frontdoor.go` | 60 | `GateMode.Line` is populated by `GateModes` (`Line: i + 1`) but never read anywhere — not in `String()`, not in an error message, not in any test assertion. Every other line-carrying type in this package (`Row`, `ProseRef`, etc.) surfaces its `Line` field through a `String()`/error method for an actionable message; `GateMode` collects the same provenance and discards it. | Use `Line` in an error/format path (e.g. `String()` or the audit's failure messages), or drop the field until something needs it. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | Sentinels wrapped correctly with `%w`; one sentinel (`ErrNoFrontDoorFiles`) is registered but never produced (finding 3). |
| Input Validation | PASS | `GateModes`, `Checklist`, `ModuleOwnerRepo`, `quickstartExtract` all handle empty/malformed input via sentinel errors, proven by mutation tests. |
| Naming | PASS | No stuttering; exported symbols carry doc comments; package/interface naming follows Effective Go. |
| Code Organization | FAIL | Findings 1, 3, 4 — unused exported methods, an unused sentinel, and an unused struct field, all newly introduced by this change. |
| Correctness | PASS | Gate parsing verified against the live `scripts/ci-local.sh` case statement (heredoc/here-string skip confirmed correct on real input); quickstart extraction and normalization verified by mutation; vacuity guard manually re-verified by renaming `## Quickstart` in README.md and confirming the test fails with `errNoQuickstartSection`, then restoring the file. |
| Test Quality | PASS (with finding 2) | Table-driven, MUTATION-labeled cases throughout; `go test -race` passes; coverage 86.7% (`internal/docs`) / 81.1% (`cmd/curlew`) on the touched packages. Finding 2 is a gap between a specific, written plan commitment and what was actually tested. |

## Test Coverage
- `internal/docs`: 86.7% of statements (target ≥80%, met)
- `cmd/curlew`: 81.1% of statements (target ≥80%, met)
- Uncovered in touched code: `GateMode.Accepts` (0%), `GateMode.String` (0%), `ReadFrontDoor`'s context-cancellation and non-`ErrNotExist` read-error branches (partial)

## Summary
The core deliverable is solid: `./scripts/ci-local.sh --go` is green including the
two new gate steps, all four task behaviors have attributable tests, the
quickstart's vacuity guard was manually re-verified by mutation (rename the
heading, watch it fail, restore the file), and the front-door files
(`CONTRIBUTING.md`, `SECURITY.md`, issue/PR templates) are accurate and held to
their sources (`ci-local.sh`, `go.mod`, `CLAUDE.md`) by executed tests rather than
prose. The findings above are all in the newly-added `internal/docs/frontdoor.go`
support code, not the observable behavior: two unused exported members and one
unused sentinel error that should be wired up or removed, plus one written plan
commitment (exit-code dropdown held to `reachableExitCodes`) that was never
implemented and has no test protecting it from future drift.
