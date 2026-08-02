# Verification Report: M1-016

**Task:** Setup and teardown sections
**Verified by:** AI
**Date:** 2026-03-14
**Branch:** feature/M1-016-setup-teardown
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | 10 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All cases pass, setup/teardown headers verified |
| Coverage total | 92.3% | Meets >= 80% threshold |
| `internal/runner` | 90.8% | Above threshold |
| `internal/parser` | 91.5% | Above threshold |
| `internal/output` | 100.0% | Above threshold |
| `cmd/curlew` | 85.2% | Above threshold |

## Observable Output

```
=== Smoke Test (setup/teardown section) ===

--- Running collection with setup and teardown (expect pass, headers printed) ---
PASS: Setup: header printed
PASS: Teardown: header printed
PASS: 3 total requests counted
Collection: Setup Teardown Smoke Test

Setup:
  ✓ Setup Request  200  440ms
  ✓ Main Request  200  105ms

Teardown:
  ✓ Teardown Request  200  128ms

3 request(s): 3 passed, 0 failed (675ms)
Pass: exit code 0

--- Teardown runs when main fails (exit code ignores teardown) ---
PASS: Teardown: header printed on main failure
PASS: exit code 1 (main assertion failure)
```

Expected: Setup: header → setup results → main results → Teardown: header → teardown results
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given a collection with setup: section, when executed, then setup requests run before main requests | `TestRun_phases/setup_runs_before_main` | PASS |
| 2 | Given a collection with teardown: section, when executed, then teardown runs after main requests | `TestRun_phases/teardown_runs_after_main` | PASS |
| 3 | Given a main request failure, when teardown exists, then teardown still executes | `TestRun_phases/teardown_runs_when_main_fails` | PASS |
| 4 | Given required: true on a setup request, when setup fails, then main requests are skipped but teardown runs | `TestRun_phases/required_setup_failure_skips_main_runs_teardown` | PASS |
| 5 | Given required: false (default) on a setup request, when setup fails, then main requests still execute | `TestRun_phases/non-required_setup_failure_continues_main` | PASS |
| 6 | Given setup requests with extract:, when variables are extracted, then they are available in main requests and teardown | `TestRun_phases/setup_extract_available_in_main_and_teardown` | PASS |
| 7 | Given teardown failure, when reported, then it is included in output but does not change main pass/fail status | `TestRun_phases/teardown_failure_does_not_change_main_pass/fail` | PASS |

All 7 behaviors verified. Additional CLI integration tests: `TestCLIIntegration_setup_teardown_execution_order`, `TestCLIIntegration_setup_teardown_runs_after_main_failure`.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `TestRun_phases` — 8 sub-tests all PASS | PASS |
| 2 | Observable output works as specified | Smoke test: Setup:/Teardown: headers printed, correct order | PASS |
| 3 | Test coverage >= 80% | 92.3% total; all packages above 80% | PASS |
| 4 | No build warnings or lint errors | `go build` clean; `golangci-lint` 0 issues | PASS |
| 5 | Help text updated (if user-facing) | No new user-facing flags; existing help unchanged | PASS |
| 6 | Smoke test updated (if new capability) | Two new smoke cases added to `smoke/run.sh` | PASS |

## Code Review

Branch A: Review PASS trusted (verdict PASS, no findings). Spot-check performed:

| Check | Status |
|-------|--------|
| Error handling (`%w` wrapping) | PASS — `executePhase` wraps errors; all `fmt.Errorf` calls use `%w` |
| Exported symbols have doc comments | PASS — `Phase`, `PhaseSetup/Main/Teardown`, `executePhase`, `Run`, `PrintSectionHeader` all documented |
| Test quality | PASS — `TestRun_phases` uses table-driven tests with mock `ExecuteFunc`; tests assert phase tags, skip flags, and variable propagation |
| Naming conventions | PASS — `Phase`, `IsRequired()`, `TeardownErrors` — idiomatic, no stuttering |
| Error handling | PASS |
| Code organization | PASS |

## Commits

| Hash | Message |
|------|---------|
| 8f757cf | docs(review): add passing review for M1-016 |
| 1c09543 | docs(review): add improvement report for M1-016 |
| da94bb5 | test(cli): add integration tests for setup/teardown behaviour |
| 5531ecc | docs(review): add improvement report for M1-016 |
| 5db4570 | docs(review): add improvement report for M1-016 |
| b2a2b94 | fix(cli): allow setup/teardown to run when main requests is empty |
| 58c8864 | fix(parser): validate url field in setup and teardown sections |
| 4dbe4d3 | docs(review): add review with findings for M1-016 |
| f00ab0c | chore(task): mark M1-016 as review |
| d9d7793 | chore(smoke): add setup/teardown smoke tests and update CHANGELOG |
| 90d9144 | feat(cli): phase-aware rendering and teardown-excluded exit code |
| 7f1e530 | feat(output): add PrintSectionHeader for phase section display |
| fea83cf | test(output): add failing test for PrintSectionHeader |
| 3076415 | feat(runner): implement three-phase setup/main/teardown execution |
| 1b8ac60 | test(runner): add failing tests for three-phase execution |
| 0ef987f | feat(runner): add Phase type/constants and TeardownErrors to Summary |
| 20770b3 | feat(parser): apply external ref resolution and method validation to setup/teardown |
| f2cc28c | test(parser): add failing tests for setup/teardown resolution and validation |
| 6d4eb5e | feat(parser): add Setup, Teardown, Required fields to Collection/RequestItem |
| de9a403 | test(parser): add failing tests for setup/teardown/required fields |
| faedb6b | chore(task): mark M1-016 as in_progress |
| 5beb94f | chore(task): mark M1-016 as planned |
| 3607623 | docs(plan): add implementation plan for M1-016 |

TDD pattern visible: `test(...)` commits precede corresponding `feat(...)` commits throughout.

## Files Changed

| File | Action |
|------|--------|
| `internal/parser/collection.go` | modified — `Setup`, `Teardown` on `Collection`; `Required` on `RequestItem`; `IsRequired()` |
| `internal/parser/parser.go` | modified — three-section loop for validation/resolution |
| `internal/parser/parser_test.go` | modified — new tests for setup/teardown parsing |
| `internal/parser/testdata/with_setup*.yaml` | created — 6 test fixtures |
| `internal/parser/testdata/with_teardown*.yaml` | created — 3 test fixtures |
| `internal/runner/runner.go` | modified — `Phase` type, `executePhase`, three-phase `Run` |
| `internal/runner/runner_test.go` | modified — `TestRun_phases` (8 sub-tests) |
| `internal/output/terminal.go` | modified — `PrintSectionHeader` |
| `internal/output/terminal_test.go` | modified — `TestPrintSectionHeader` |
| `cmd/curlew/main.go` | modified — phase-aware rendering, teardown-excluded exit code |
| `cmd/curlew/main_test.go` | modified — 2 CLI integration tests |
| `smoke/run.sh` | modified — 2 new smoke cases |
| `CHANGELOG.md` | modified — M1-016 entry |
| `management/backlog.yaml` | modified — status update |
| `management/plans/M1-016-plan.md` | created |
| `management/plans/M1-016-improved.md` | created |
| `management/reviews/M1-016-review.md` | created |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
