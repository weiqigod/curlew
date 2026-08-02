# Verification Report: M1-017

**Task:** Global project config (curlew.yaml)
**Verified by:** AI
**Date:** 2026-03-14
**Branch:** feature/M1-017-global-project-config
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All smoke tests pass including project config walk-up and help text |
| Coverage | 92.4% | Meets >= 80% threshold |

## Observable Output

```
Collection: Demo
  ✓ Get  200  444ms

1 request(s): 1 passed, 0 failed (444ms)
```

Setup: `curlew.yaml` in `/tmp/curlew-demo/` with `base_url: https://httpbin.org`; collection in `/tmp/curlew-demo/requests/` using `{{base_url}}/get`.
Expected: request succeeds, no "undefined variable" errors.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given curlew.yaml in project root with variables: block, when collection runs, then variables are loaded at precedence 2 | `TestRun_ProjectVariables/project_variables_available_in_interpolation`, `TestRunCmd_ProjectConfig/project_variables_resolved_in_collection` | PASS |
| 2 | Given curlew.yaml variables and collection variables with same key, then collection wins as more specific | `TestRun_ProjectVariables/collection_overrides_project`, `TestRunCmd_ProjectConfig/project_variables_overridden_by_collection_variables` | PASS |
| 3 | Given no curlew.yaml, when collection runs, then no error (file is optional) | `TestLoadProjectConfig/no_project_root_returns_empty_config_no_error`, `TestRunCmd_ProjectConfig/no_curlew.yaml_runs_without_error` | PASS |
| 4 | Given curlew.yaml with project_name: field, when loaded, then it is available for output context | `TestParseProjectConfig/valid_with_project_name_and_variables` | PASS |
| 5 | Given curlew.yaml in a parent directory, when collection is in subdirectory, then project config is found by walking up | `TestFindProjectRoot/found_in_grandparent_directory`, `TestRunCmd_ProjectConfig/project_config_in_parent_directory_(walk-up)`, smoke test | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages PASS | PASS |
| 2 | Observable output works as specified | Collection with `{{base_url}}` resolves via project config | PASS |
| 3 | Test coverage >= 80% | 92.4% overall; `internal/config/project.go` 100% | PASS |
| 4 | No build warnings or lint errors | Clean `go build`; `golangci-lint` 0 issues | PASS |
| 5 | Help text updated | `curlew.yaml` listed in Auto-loaded section | PASS |
| 6 | Smoke test updated | Walk-up scenario and help text check added | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — all errors wrapped with `%w`; `ErrInvalidProjectConfig` sentinel used correctly |
| Naming conventions | PASS — no stuttering; all exports have doc comments |
| Code organization | PASS — `internal/config` boundary respected; `flatten()` reused from same package |
| Test quality | PASS — table-driven tests; integration tests via `os/exec`; precedence cases all covered |

Branch A: Review PASS (dated 2026-03-14) trusted. Spot-check clean: error wrapping (`%w`), exported doc comments, and `FindProjectRoot` test all verified.

## Commits

| Hash | Message |
|------|---------|
| b8712bc | docs(review): add passing review for M1-017 |
| 5c007fd | docs(review): add improvement report for M1-017 |
| 668fdfc | test(runner,config): strengthen assertions in project variable tests |
| 3b31a35 | docs(review): add review with findings for M1-017 |
| 5f3c2a4 | chore(task): mark M1-017 as review |
| 0152136 | refactor(config): check error return from f.Close in project_test |
| 47d827d | feat(cli): update help text and smoke test for project config |
| 2bea7f5 | feat(cli): wire LoadProjectConfig into runCmd; shift .env dir to project root |
| c4ec852 | test(cli): add failing integration tests for project config wiring |
| 028cefc | feat(runner): add Project field to VarSources at precedence 2 |
| 0593410 | test(runner): add failing tests for Project variable precedence in VarSources |
| b5c35a0 | feat(config): implement ProjectConfig parsing and root discovery |
| 455c5ad | test(config): add failing tests for ProjectConfig parsing and root discovery |

TDD pattern visible: `test(...)` commits precede `feat(...)` commits throughout.

## Files Changed

| File | Action |
|------|--------|
| `internal/config/project.go` | created |
| `internal/config/project_test.go` | created |
| `internal/runner/runner.go` | modified — Project field in VarSources |
| `internal/runner/runner_test.go` | modified — TestRun_ProjectVariables |
| `cmd/curlew/main.go` | modified — LoadProjectConfig wiring, help text |
| `cmd/curlew/main_test.go` | modified — TestRunCmd_ProjectConfig |
| `smoke/run.sh` | modified — project config walk-up smoke test |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
