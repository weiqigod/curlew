# Code Review: M28-002

**Task:** The front-door files, and a quickstart that was executed
**Reviewer:** AI
**Date:** 2026-08-19
**Branch:** feature/M28-002-front-door-files-and-quickstart
**Iteration:** 2 (re-review after `/improve`)

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` was run fresh for this iteration (not reused from
the improve report). It ran to completion under `set -euo pipefail` and
printed `=== ci-local PASS ===` with exit code 0, meaning every step —
`go build`, `go test`, `go test -race`, `go coverage`, `golangci-lint`,
`./smoke/run.sh`, the release/goreleaser snapshot checks, the README-install
exec test, and the full dogfood suite against mudflat — passed, including the
two M28-002-specific named steps ("front-door files (M28-002)" and "README
quickstart, executed (M28-002)"): a failure at either would have aborted the
script before it reached the final `PASS` line. `coverage.out` from this same
run was inspected directly (not taken on the improve report's word):
repo-wide statement coverage is 86.6%, and `internal/docs` / `cmd/curlew`
both sit comfortably above the 80% floor. `git status` after the run shows a
clean working tree. Static audit proceeded.

## Verification of Iteration-1 Findings

All four findings from the prior review were checked against the current code
in `internal/docs/frontdoor.go` / `hints_init.go`, not just against the
improvement report's description:

| # | Finding | Verified fix |
|---|---------|---------------|
| 1 | `GateMode.Accepts`/`String` unused | `AuditGate` (frontdoor.go:268) now calls `m.Accepts(w)` instead of an inline loop; `TestGateMode_String` (frontdoor_test.go:277) exercises `String()` directly, and it now formats `Line` in, matching finding 4 |
| 2 | Plan-committed exit-code dropdown test missing | `cmd/curlew/bug_report_exit_codes_test.go` exists, parses `bug_report.yml`'s dropdown, and asserts containment against `exitcodes.Reachable`/`reachableExitCodes` |
| 3 | `ErrNoFrontDoorFiles` never produced | `ReadFrontDoor` (frontdoor.go:462-464) now returns it for a nil/empty registry; `TestReadFrontDoor_emptyRegistry` proves both nil and empty-slice cases |
| 4 | `GateMode.Line` never read | Folded into `String()` (frontdoor.go:85-87): `scripts/ci-local.sh:LINE <aliases>`, following the same convention as `LayoutRow.String()` and `ProseRef.String()` elsewhere in the package (verified: those two are also exported, tested by a dedicated `_String` test, and not called from production code — an established, not a novel, pattern in this package) |

All four are resolved by real code changes with direct test coverage, not by
deleting the finding's premise.

## Fresh Findings (this iteration)

None. The full diff (19 files, +3256/−21) was read in full: `README.md`,
`CONTRIBUTING.md`, `SECURITY.md`, the three issue/PR templates,
`internal/docs/frontdoor.go`, `internal/docs/frontdoor_test.go`,
`internal/docs/hints_init.go`, `cmd/curlew/readme_quickstart_test.go`,
`cmd/curlew/bug_report_exit_codes_test.go`, `scripts/ci-local.sh`,
`CHANGELOG.md`. Checked and confirmed:

- The reused helpers (`readmeSectionBlocks`, `readmeRepoRoot`,
  `readmeReadFileOrFatal`, `reachableExitCodes`) are untouched by this branch
  (`git diff` against `readme_install_test.go`, `skill_exit_codes_test.go` is
  empty) — genuine reuse, not a fork.
- `ci-local.sh`'s corrected comment ("a bare `^TestReadme` prefix returns 7")
  was independently verified: `go test -tags readme_install -list
  '^TestReadme' ./cmd/curlew/` lists exactly 7 `TestReadme_*` functions.
- The vacuity guards in `quickstartExtract` (empty section, no commands, no
  output, ambiguous multiple blocks) and the observable test's independent
  floors (`< 3` commands, `< 10` non-blank output lines, must contain
  `"passed"`) are distinct and layered, matching the M22-001 concern the task
  names explicitly.
- `SECURITY.md`, `CONTRIBUTING.md`, and the PR/issue templates are held to
  their sources (`ci-local.sh`'s case statement, `go.mod`'s module path,
  `CLAUDE.md`'s checklists) by executed tests in both directions where the
  task calls for it (`AuditGate`) and one direction where it doesn't
  (`TestPullRequestTemplate_carries_the_quality_gate_checklist`, forward-only
  by design since the template may add PR-specific lines).
- `errcheck`/`gofumpt` compliance: `Serve`/`Shutdown`/`os.WriteFile` returns
  are all handled or explicitly discarded with a comment explaining why.
- `.golangci.yml`'s `build-tags` already carries `readme_install` and
  `release_artifacts` together, and no new identifier in the touched
  `package main` files collides with either tag's file set.
- One line in `SECURITY.md` (`SecurityPublicIssueRule`, 119 columns) exceeds
  the 100-column house style noted in the plan — but the string is a Go
  constant asserted verbatim by `TestSecurity_names_a_reporting_route_that_exists`
  via `strings.Contains`, so it cannot be soft-wrapped without breaking the
  containment check. Not flagged: markdownlint is not wired into any gate,
  and the deviation is forced by the test-anchored verbatim-match design
  rather than an oversight.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All sentinels wrapped correctly; every declared sentinel now has both a `RegisterPackage` entry and a real producer. |
| Input Validation | PASS | `GateModes`, `GateInvocations`, `Checklist`, `ModuleOwnerRepo`, `AuditFrontDoor`, `ReadFrontDoor`, `quickstartExtract` all handle empty/malformed/nil input via sentinel errors, proven by mutation tests. |
| Naming | PASS | No stuttering; exported symbols carry doc comments; package/interface naming follows Effective Go. |
| Code Organization | PASS | Both previously-unused exported members (`Accepts`, `String`) now have real callers/tests; no unused sentinels remain. |
| Correctness | PASS | Gate parsing verified against the live `case "$MODE" in` statement including heredoc-skip; `TestReadme_quickstart_actually_works` runs the real quickstart against an in-process mudflat server and matches output byte-for-byte (duration-normalised) — confirmed passing in this iteration's fresh gate run. |
| Test Quality | PASS | Table-driven, MUTATION-labeled throughout; the plan's exit-code dropdown commitment now has a real test; coverage exceeds the 80% floor on every touched package. |

## Test Coverage
- Repo-wide (this run's `coverage.out`): 86.6% of statements
- `internal/docs`: ~86.8% (function-average, consistent with the 87.5% statement-weighted figure in the improve report)
- `cmd/curlew`: ~84.7% (function-average, consistent with the 81.2% statement-weighted figure in the improve report)

## Summary
All four iteration-1 findings are fixed with real, tested code — verified by
reading the current source, not by trusting the improvement report's prose.
A full fresh read of the entire task diff surfaced no new issues. The
pre-audit gate (`./scripts/ci-local.sh --go`) was re-run end-to-end for this
iteration and passed, including both M28-002 gate steps and the full dogfood
suite, confirming the fixes did not regress anything else on the branch.
