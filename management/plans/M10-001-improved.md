# Improvement Report: M10-001

**Task:** agent skill scaffolding: init --skill flag, embedded SKILL.md template, output: + .gitignore extension
**Date:** 2026-04-25
**Review:** management/reviews/M10-001-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | 5 DoD test function names in `internal/scaffold/scaffold_test.go` did not match the `definition_of_done` names in the task YAML. `TestInit_SkillClaude_VersionInTemplate` was also missing entirely (behavior split across other tests but no dedicated function by that name). | Renamed the 5 existing tests to match DoD names: `CreatesSkillFile` → `CopiesFile`, `DefaultsMarkdownFormat` → `DefaultsMarkdown`, `AppendsEventsForEveryFormat` → `ExtendsOutputBlock`, `GitignoreContainsApitestDir` → `ExtendsGitignore`, `OutputJSON_KeepsJSONReportAndAddsEvents` → `OutputOverride`. Added `TestInit_SkillClaude_VersionInTemplate` to `cmd/apitest/main_test.go` as a binary-level check that `{{apitest_version}}` is fully substituted and the version comment reflects the live binary version constant. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/scaffold`) | 86.6% |
| Coverage (`cmd/apitest`) | 81.3% |
| Coverage (`templates`) | 90.9% |
| Coverage (overall) | 86.8% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 6c681b8 | fix(scaffold,cli): rename 5 DoD test names + add TestInit_SkillClaude_VersionInTemplate | #1 |

## Summary
1/1 findings resolved. 0 deferred.
