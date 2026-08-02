# Code Review: M15-003

**Task:** Retier from_command from Solo to Free across registry, tests, and MANUAL.md
**Reviewer:** AI
**Date:** 2026-05-07
**Branch:** feature/M15-003-from-command-free-tier

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths introduced; purely a registry tier-rank change |
| Input Validation | PASS | No new public inputs; gate logic unchanged |
| Naming | PASS | New test names follow project conventions; no stutter, doc comments present |
| Code Organization | PASS | Change correctly scoped: registry, tests, doc sweep, smoke — no layer violations |
| Correctness | PASS | Stale `runner.go:946` comment fixed to say "Free tier — available at all tiers"; no remaining Solo framing anywhere in the changed code |
| Test Quality | PASS | `TestDefaultRegistry_FromCommand` now screens both `Description` and `Workaround` for `"$9/month"` and `"Solo"`; tier-matrix covers all 5 tiers; CLI gate tests use `httptest.NewServer` for hermetic HTTP; runner test inverted to assert resolution, not gate error |

## Test Coverage

- `internal/auth`: 89.6% (above 80% floor)
- `internal/variable`: 97.4%
- `internal/runner`: 84.8%

## Behavior Coverage (from task YAML)

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | `from_command` entry has `RequiredTier=TierFree` with refreshed Description/Workaround | `TestDefaultRegistry_FromCommand` | PASS |
| 2 | Free-tier license: command shells out, variable resolves, no GateError | `runner_test.go/from_command_at_free_tier_resolves`, `TestRunFromCommand_at_free_tier_no_longer_gated` | PASS |
| 3 | Solo/Professional/Team/Enterprise: behavior unchanged (still allowed) | `TestDefaultRegistry_FromCommand_TierMatrix` (all five tiers), `runner_test.go/from_command_resolves_at_solo_tier` | PASS |
| 4 | `internal/variable/command.go`: no implementation changes required | `command.go` unchanged (verified by `git diff`) | PASS |
| 5 | `docs/MANUAL.md`: all `[Solo]` references for `from_command` updated | Zero matches on `grep -n "from_command.*\[Solo\]\|from_command.*Solo tier"` | PASS |
| 6 | TOC slug consistent with section heading | TOC `#63-from_command-variables` matches heading `### 6.3 \`from_command\` variables` | PASS |
| 7 | `registry_test.go` tier-matrix asserts `from_command` allowed at all five tiers | `TestDefaultRegistry_FromCommand_TierMatrix` | PASS |
| 8 | Backwards compatibility: Solo+ collections continue to work | `from_command_resolves_at_solo_tier` subtest; `TierMatrix` "solo allows" row | PASS |

## Summary

Both findings from the first review (stale `runner.go` comment and missing `Workaround` string checks in the registry test) have been correctly resolved. The implementation is clean: the registry entry is at `TierFree`, the six MANUAL.md sites are updated with no remaining `[Solo]` adjacency, the tier-matrix table row is removed per the "unlisted = Free" convention, and all tests pass cleanly with coverage above the 80% floor.
