# Code Review: M10-001

**Task:** agent skill scaffolding: init --skill flag, embedded SKILL.md template, output: + .gitignore extension
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M10-001-init-skill-flag
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. `ErrUnknownSkill` sentinel defined in `templates` package for CLI branching. `ErrProjectExists` reused for project-already-exists path. `installSkill` wraps all sub-errors with descriptive context. No swallowed errors. No panics for expected failures. |
| Input Validation | PASS | Unknown `--skill` value exits 3, stderr names the supported set and echoes the rejected value. Unknown `--output` validation unchanged from M9-005. `--skill` with no value exits 1. Empty entries slice passed to `ensureGitignore` is only reachable via the zero-value `Options{}` path (always has `.env`), and would create an empty gitignore rather than panic — no-op scenario not reachable from production paths. |
| Naming | PASS | All exported symbols have doc comments (`ErrUnknownSkill`, `SupportedSkills`, `SkillRelativePath`, `Render`, `IsSupportedSkill`, `Options.SkillName`, `Options.ApitestVersion`). No stuttering. Package `templates` is lowercase single-word. `SkillRelativePath` correctly uses `_` parameter since v1 path is uniform; documented intent. |
| Code Organization | PASS | Clean dependency layering: `templates` (no internal imports) → `internal/scaffold` → `cmd/apitest`. Single responsibility per package. No circular deps. `cmd/apitest` remains an entry-point-only package. |
| Correctness | PASS | `outputBlock` splice correctly finds `verbosity: normal` insertion point in all 6 format branches; defensive fallback at end is unreachable by construction. `ensureGitignore` new-file path writes correctly for all reachable callers. `installSkill` skips write on pre-existing file (returns `nil`) matching `ErrProjectExists` semantics for `apitest.yaml`. Plan mentioned `ErrSkillExists` sentinel but implementation omits it — `installSkill` is only called from `Init` which has no need to branch on that sentinel; acceptable divergence. `formatForBlock` coupling is internal to scaffold, not CLI. |
| Test Quality | PASS | All 9 behaviors covered. All 11 DoD-named test functions present and passing. Integration test (`TestInit_SkillClaude_FullPipeline`) exercises the real binary via `captureRun`. Regression tests for bare-init byte-equality (`TestInit_BareInit_ByteIdenticalToM9005`, `TestInit_DefaultUnchanged`). Schema parity test (`TestSchema_scaffolded_all_output_formats_validate/skill_claude`). Table-driven tests used where multiple similar cases exist (`ExtendsOutputBlock`, `TestIsSupportedSkill`). Error paths tested: unknown skill, missing flag value, pre-existing files. `testdata/` fixture used for schema fixture. |

## Test Coverage

- `templates`: 90.9% — 1 uncovered statement is the `ReadFile` error path inside `case "claude"` (unreachable since the embed is compile-time; acceptable)
- `internal/scaffold`: 86.6% — uncovered paths are `MkdirAll` failure inside `installSkill` (no test for unwritable skill directory) and the defensive `outputBlock` end-append fallback (unreachable by construction); both acceptable
- `cmd/apitest`: 81.3%
- All three packages exceed the 80% threshold required by the DoD

## DoD Verification

| DoD Item | Test | Status |
|----------|------|--------|
| TestInit_SkillClaude_CopiesFile | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_DefaultsMarkdown | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_ExtendsOutputBlock | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_ExtendsGitignore | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_PreservesExistingResponsesDir | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_OutputOverride | `internal/scaffold/scaffold_test.go` | PASS |
| TestInit_SkillClaude_VersionInTemplate | `cmd/apitest/main_test.go` | PASS |
| TestInit_SkillFlag_DocumentsInHelp | `cmd/apitest/main_test.go` | PASS |
| TestInit_SkillUnknown_Exit3 | `cmd/apitest/main_test.go` | PASS |
| TestInit_SkillClaude_FullPipeline | `cmd/apitest/main_test.go` | PASS |
| TestSchema_scaffolded_all_output_formats_validate (extended) | `internal/schema/validate_test.go` | PASS |
| Regression: bare init byte-identical | `cmd/apitest/main_test.go` + `internal/scaffold/scaffold_test.go` | PASS |
| Regression: existing M9-005 tests pass | `go test ./...` | PASS |
| go test ./... passes | `./scripts/ci-local.sh --go` | PASS |
| Coverage >= 80% (scaffold, cmd/apitest, templates) | 86.6% / 81.3% / 90.9% | PASS |
| golangci-lint run passes 0 issues | `./scripts/ci-local.sh --go` | PASS |
| ./smoke/run.sh passes | `./scripts/ci-local.sh --go` | PASS |

## Summary

The iteration-2 code is clean and correct. The sole finding from iteration 1 (5 test function names not matching DoD names, plus missing `TestInit_SkillClaude_VersionInTemplate`) has been resolved: all 11 DoD-required test functions are present with exact names and all pass. Error handling, naming, code organization, and correctness all meet project standards. Coverage meets or exceeds 80% in all three new packages.
