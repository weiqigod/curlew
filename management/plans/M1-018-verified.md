# Verification Report: M1-018

**Task:** Dynamic variable functions with seed reproducibility
**Verified by:** AI
**Date:** 2026-03-14
**Branch:** feature/M1-018-dynamic-variable-functions
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/apitest` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | 10 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All smoke scenarios pass (two bugs fixed during verification) |
| Coverage `internal/variable` | 95.8% | Meets >= 80% threshold |
| Coverage `internal/runner` | 91.2% | Meets >= 80% threshold |
| Coverage `cmd/apitest` | 85.5% | Meets >= 80% threshold |
| Coverage total | 92.9% | Meets >= 80% threshold |

## Smoke Test Fixes During Verification

Two bugs were found and fixed in `smoke/run.sh` during verification:

1. **Shell variable expansion**: `echo "--- Dynamic function {{$uuid}} ---"` — bash expanded `$uuid` as a shell variable; `set -u` caused "unbound variable" exit. Fixed by using single quotes.
2. **Timing comparison**: Seed determinism check compared full output including `(Nms)` timing fields, which naturally differ each run. Fixed by stripping timing before comparison.

Both fixes are in commit `91d3d6e`.

## Observable Output

```
Collection: Dynamic Var Test
  ✓ test dynamic vars  200  509ms

1 request(s): 1 passed, 0 failed (509ms)
```

Two runs with `--seed 42` produce identical output (timing stripped): MATCH

Expected: identical UUID and timestamp determinism for seeded runs.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | `{{$timestamp}}` inserts current Unix timestamp | `TestRegistry_Evaluate/timestamp_returns_unix_seconds`, `TestScope_Interpolate_dynamic/timestamp_replaced` | PASS |
| 2 | `{{$uuid}}` inserts a valid UUID v4 | `TestRegistry_Evaluate/uuid_returns_valid_v4`, `TestRun_dynamic_uuid_interpolated`, `TestIntegration_dynamic_functions/uuid_in_request_url_reaches_server` | PASS |
| 3 | `{{$randomInt}}` inserts a random integer | `TestRegistry_Evaluate/randomInt_in_range_0-1000` | PASS |
| 4 | `--seed 42` produces the same UUID each run | `TestRun_seed_deterministic_across_runs`, `TestIntegration_dynamic_functions/seed_42_is_deterministic_across_runs` | PASS |
| 5 | `--seed 42` deterministic for multiple functions | `TestRegistry_seeded_deterministic` | PASS |
| 6 | `{{$randomEmail}}` / `{{$randomName}}` realistic values | `TestRegistry_Evaluate/randomEmail_has_@_and_example.com`, `TestRegistry_Evaluate/randomName_has_two_space-separated_parts` | PASS |
| 7 | Unknown function error lists available functions | `TestRegistry_unknown_function_lists_available`, `TestScope_Interpolate_dynamic/unknown_function_returns_error` | PASS |
| 8 | Explicit variable wins over dynamic function | `TestScope_Interpolate_dynamic/explicit_var_overrides_function`, `TestRun_dynamic_override_by_cli_var` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — 10 packages PASS | PASS |
| 2 | Observable output works | Two `--seed 42` runs produce identical output | PASS |
| 3 | Test coverage >= 80% | Total 92.9%; variable 95.8%, runner 91.2%, cmd 85.5% | PASS |
| 4 | No build warnings or lint errors | Clean `go build`; `golangci-lint` 0 issues | PASS |
| 5 | Help text updated | `--seed <number>` visible in `apitest --help` | PASS |
| 6 | Smoke test updated | Dynamic UUID and seed determinism cases added to `smoke/run.sh` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS (management/reviews/M1-018-review.md) trusted. Spot-check confirmed:
- `Registry.Evaluate` wraps unknown-function error with `fmt.Errorf`; nil-cache guard present
- All exports (`DynFunc`, `Registry`, `NewRegistry`, `Evaluate`, `Available`, `WithDynamic`, `BeginRequest`, `EndRequest`) have doc comments
- `TestRegistry_seeded_deterministic` actually asserts identical values, not just no-error

## Commits

| Hash | Message |
|------|---------|
| 91d3d6e | fix(smoke): fix $uuid shell expansion and seed timing comparison |
| 1adc02f | docs(review): add passing review for M1-018 |
| 314af5a | docs(review): add improvement report for M1-018 |
| 30f0991 | fix(variable): add test for intn crypto/rand path (nil rng branch) |
| 25c4818 | docs(review): add improvement report for M1-018 |
| ba199dc | docs(review): add improvement report for M1-018 |
| a08068a | fix(variable): propagate registry in WithOverrides and guard nil cache |
| 5496f07 | docs(review): add review with findings for M1-018 |
| 70842b3 | chore(task): mark M1-018 as review |
| b4e0e7f | refactor(variable): group var declarations for gofumpt compliance |
| 13983c1 | test(cli): add integration tests and smoke test for dynamic functions and --seed |
| e38a4d6 | feat(runner): wire dynamic function registry into Run and interpolateRequest |
| 692cc3f | test(runner): add failing tests for dynamic function integration |
| 2b04497 | feat(cli): add --seed flag for deterministic random variable functions |
| 028b3d3 | test(cli): add failing tests for --seed flag parsing |
| 7c7ea2f | feat(variable): extend Scope with dynamic function interpolation |
| 464e0eb | test(variable): add failing tests for dynamic interpolation in Scope |
| 92e7923 | feat(variable): implement dynamic function registry with 15 built-in functions |
| 15e5ab0 | test(variable): add failing tests for dynamic function registry |

TDD pattern: all `test(...)` commits precede corresponding `feat(...)` commits.

## Files Changed

| File | Action |
|------|--------|
| `internal/variable/dynamic.go` | created |
| `internal/variable/dynamic_test.go` | created |
| `internal/variable/variable.go` | modified (dynPattern, WithDynamic, BeginRequest, EndRequest, Interpolate) |
| `internal/variable/variable_test.go` | modified (dynamic interpolation tests) |
| `internal/runner/runner.go` | modified (Seed field, registry wiring, BeginRequest/EndRequest) |
| `internal/runner/runner_test.go` | modified (dynamic function integration tests) |
| `cmd/apitest/main.go` | modified (--seed flag, parseRunArgs, runCmd, help text) |
| `cmd/apitest/main_test.go` | modified (seed parsing tests, integration tests) |
| `smoke/run.sh` | modified (dynamic UUID and seed smoke cases; two bug fixes) |
| `management/tasks/M1-018.yaml` | modified (status progression) |
| `management/plans/M1-018-plan.md` | created |
| `management/reviews/M1-018-review.md` | created |
| `management/plans/M1-018-improved.md` | created |

## Issues Found

One issue in smoke test found and fixed during verification:
- `$uuid` shell expansion bug (cosmetic — didn't affect binary)
- Timing-dependent seed comparison (caused false FAIL in smoke test)

## Recommendation

PASS — ready for PR and merge.
