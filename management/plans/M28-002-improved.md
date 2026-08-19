# Improvement Report: M28-002

**Task:** The front-door files, and a quickstart that was executed
**Date:** 2026-08-19
**Review:** management/reviews/M28-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `GateMode.Accepts` and `GateMode.String` were exported but never called; `AuditGate` reimplemented `Accepts`'s alias-matching loop inline instead of calling it. | `AuditGate` now calls `m.Accepts(w)` for each invoked word instead of its own inline loop, so `Accepts` is exercised through the existing `TestAuditGate` table. `String()` was rewritten to render `scripts/ci-local.sh:LINE <aliases>`, matching the file:line convention `Row` and `ProseRef` already use elsewhere in `internal/docs`, and a new `TestGateMode_String` proves it directly. Removed a resulting dead `invokedSet` map in `AuditGate` that the `Accepts` refactor made unused. | ✓ `go tool cover -func` shows `Accepts` and `String` at 100.0% (was 0.0%); `TestAuditGate` and `TestGateMode_String` pass; `golangci-lint run` clean |
| 2 | Medium | The plan committed to a test holding `bug_report.yml`'s exit-code dropdown to `exitcodes.Reachable(...)` via `reachableExitCodes(t)`, "so the observable's test stays about file presence and content," but no such test existed. | Added `cmd/curlew/bug_report_exit_codes_test.go`: parses the dropdown's `options` out of the real `.github/ISSUE_TEMPLATE/bug_report.yml`, and asserts every code in `exitcodes.Set(reachableExitCodes(t))` has a matching numeric option (containment, not equality, so the dropdown's `"other / not sure"` escape valve stays legal). Also asserts that escape-valve option exists. | ✓ `TestBugReportTemplate_exitCodeDropdownCoversReachableCodes` passes; verified by mutation — removing the `"130"` option from the real file made the test fail with the expected message, then the file was restored (confirmed clean via `git status`/`diff`) |
| 3 | Low | `ErrNoFrontDoorFiles` was declared and registered but never produced by any function; the "registry emptied out" guard its own doc comment describes was instead a raw `len(docs.FrontDoor) < 6` check inline in the observable test. | `ReadFrontDoor` now returns `ErrNoFrontDoorFiles` when handed an empty (nil or zero-length) file registry, giving the sentinel a real producer. The pre-existing `len(docs.FrontDoor) < 6` floor in `TestRepo_front_door_files_are_present` was left in place (it guards a different thing — the registry *shrinking*, not being fully emptied) and a new `TestReadFrontDoor_emptyRegistry` proves the sentinel path directly. | ✓ `TestReadFrontDoor_emptyRegistry` (both nil and empty-slice cases) passes; `errors.Is` confirms the sentinel |
| 4 | Low | `GateMode.Line` was populated (`Line: i + 1` in `GateModes`) but never read anywhere. | Folded into the finding-1 fix: `GateMode.String()` now formats `Line` into `scripts/ci-local.sh:LINE <aliases>`, giving it a real consumer, consistent with how `Row.String()` and `ProseRef.String()` surface their own `Line` fields in the same package. | ✓ `TestGateMode_String` asserts the line number appears in the rendered string for both a single-alias and an aliased mode |

## Out of Scope (Deferred)

No findings deferred. All four findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS (54 packages `ok`, none failing) |
| `go test -race ./internal/docs/... ./cmd/curlew/...` | PASS |
| `golangci-lint run ./...` | PASS (0 issues) |
| `./scripts/ci-local.sh --go` | PASS (`=== ci-local PASS ===`, exit 0), including the two named M28-002 gate steps |
| Coverage — `internal/docs` | 87.5% (was 86.7%) |
| Coverage — `cmd/curlew` | 81.2% (was 81.1%) |
| Coverage — repo total | 84.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `f42502d` | fix(docs): wire up GateMode's unused surface and the empty-registry sentinel | #1, #3, #4 |
| `54f89a2` | test(cmd/curlew): hold bug_report.yml's exit-code dropdown to reachableExitCodes | #2 |

## Summary
4/4 findings resolved. 0 deferred.
