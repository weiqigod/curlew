# Verification Report: M1-014

**Task:** Request-level variables, --env-var, full precedence
**Verified by:** AI
**Date:** 2026-03-12
**Branch:** feature/M1-014-request-vars-env-var-precedence
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 10 packages, all pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | 93.3% | Meets >= 80% threshold |

## Observable Output

```
$ TEST_API_KEY=my-secret curlew run precedence-test.yaml --env-var TEST_API_KEY --var shared_var=cli-level
Collection: Precedence Test
  ✓ Check precedence  200  502ms
  ✓ Check request vars dont leak  200  717ms
2 request(s): 2 passed, 0 failed (1219ms)

$ curlew run test.yaml --env-var MISSING_VAR_DOES_NOT_EXIST
[ERROR] environment variable not set: "MISSING_VAR_DOES_NOT_EXIST" is not set in the environment
Exit: 1
```

Expected: Request-level vars override collection, don't leak to next request. --env-var imports OS env. Missing env var produces error.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Request-level value used (precedence 8) | `TestRun_request_level_variables_override_collection` | PASS |
| 2 | --env-var imports OS env (precedence 9) | `TestParseEnvVarFlag/bare_name_set`, `TestRun_env_var_overrides_collection` | PASS |
| 3 | --env-var missing var errors | `TestParseEnvVarFlag/bare_name_unset`, `TestParseEnvVarFlag/mapped_name_unset` | PASS |
| 4 | Higher precedence wins: CLI(10) > env-var(9) > request(8) > extract(7) > .env(4) > env(3) > collection(2) | `TestRun_full_precedence_chain`, `TestRun_cli_var_overrides_env_var`, `TestRun_env_var_overrides_request_level` | PASS |
| 5 | Full precedence chain integration test | `TestRun_full_precedence_chain_with_request_and_envvar` | PASS |
| 6 | Request vars reference collection vars (interpolation chain) | `TestRun_request_level_variables_reference_collection_vars` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` all pass | PASS |
| 2 | Observable output works | CLI output matches expectations | PASS |
| 3 | Test coverage >= 80% | 93.3% overall | PASS |
| 4 | No build warnings or lint errors | `golangci-lint run` — 0 issues | PASS |
| 5 | Help text updated | `--env-var` in help output | PASS |
| 6 | Smoke test updated | 3 new smoke tests for --env-var | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — `%w` wrapping, sentinel errors |
| Naming conventions | PASS — Effective Go, no stuttering |
| Code organization | PASS — clean package boundaries |
| Test quality | PASS — table-driven, integration tests |

Review PASS trusted (management/reviews/M1-014-review.md), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| 2c63ab0 | docs(plan): add implementation plan for M1-014 |
| df6eb1b | chore(task): mark M1-014 as planned |
| b57535d | chore(task): mark M1-014 as in_progress |
| e1f6020 | feat(parser): add Variables field to RequestItem |
| d9d0698 | feat(variable): add ParseEnvVarFlag and sentinel errors |
| 94acec3 | feat(variable): add WithOverrides method to Scope |
| b10f969 | refactor(runner): introduce VarSources struct for Run signature |
| 7c9ed21 | feat(runner): implement request-level variables and env-var precedence |
| 0d2c7f1 | refactor(variable): use strings.TrimPrefix per staticcheck |
| dbd5b07 | feat(cli): add --env-var flag for OS environment variable import |
| 9c6ab18 | test(smoke): add --env-var smoke tests |
| d17e640 | chore(task): mark M1-014 as review |
| 08c80e6 | docs(review): add review with findings for M1-014 |
| 74fc69b | fix(variable): validate empty varName and envName in ParseEnvVarFlag |
| 13ee380 | docs(review): add improvement report for M1-014 |
| 5c1846c | docs(review): add passing review for M1-014 |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/main.go` | modified | +44/-0 |
| `cmd/curlew/main_test.go` | modified | +242/-0 |
| `internal/parser/collection.go` | modified | +1/-0 |
| `internal/parser/parser_test.go` | modified | +44/-0 |
| `internal/parser/testdata/with_request_variables.yaml` | added | +9/-0 |
| `internal/parser/testdata/with_request_variables_empty.yaml` | added | +7/-0 |
| `internal/runner/runner.go` | modified | +46/-0 |
| `internal/runner/runner_test.go` | modified | +374/-0 |
| `internal/variable/variable.go` | modified | +59/-0 |
| `internal/variable/variable_test.go` | modified | +108/-0 |
| `smoke/run.sh` | modified | +38/-0 |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge
