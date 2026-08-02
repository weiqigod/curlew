# Verification Report: M10-001

**Task:** agent skill scaffolding: init --skill flag, embedded SKILL.md template, output: + .gitignore extension
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M10-001-init-skill-flag
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected (via ci-local.sh) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (`internal/scaffold`) | 86.6% | Meets >= 80% threshold |
| Coverage (`cmd/apitest`) | 81.3% | Meets >= 80% threshold |
| Coverage (`templates`) | 90.9% | Meets >= 80% threshold |
| Coverage (overall new packages) | 81.5% | Meets >= 80% threshold |

## Observable Output

```
# init --skill claude scaffolds SKILL.md
$ apitest init --skill claude && test -f .claude/skills/apitest/SKILL.md && echo "skill scaffolded"
skill scaffolded

# Default output block
$ grep -A 5 '^output:' apitest.yaml
output:
  format: markdown
  report: responses/
  events: .apitest/run.ndjson
  verbosity: normal

# .gitignore extended
$ grep -E '^\.apitest/$' .gitignore
.apitest/

# Version comment in SKILL.md
$ grep 'apitest-skill: claude v1.0 (apitest ' .claude/skills/apitest/SKILL.md
<!-- apitest-skill: claude v1.0 (apitest 0.1.0-dev) -->

# --skill claude --output json adds both format and events
$ apitest init --skill claude --output json && grep -A 5 '^output:' apitest.yaml | grep -E 'format: json|events: \.apitest/run\.ndjson'
  format: json
  events: .apitest/run.ndjson

# Bare init has no events
$ apitest init && grep events apitest.yaml || echo "no events reference"
no events reference

# Unknown skill exits 3 and names claude
$ apitest init --skill madeup 2>err.log; echo $?; grep -E 'claude' err.log
3
Error: unknown --skill value "madeup" (supported: claude)

# --help documents --skill flag
$ apitest init --help | grep -E '^\s*--skill'
  --skill <name>          Scaffold an agent skill template at .claude/skills/apitest/SKILL.md.

# Pre-existing responses/ preserved
$ mkdir responses && echo "preexisting" > responses/keep.md && apitest init --skill claude && test -f responses/keep.md && echo "responses preserved"
responses preserved

# Schema parity (test-level — validates against project-v1.json)
$ go test -run TestSchema_scaffolded_all_output_formats_validate/skill_claude ./internal/schema/
--- PASS: TestSchema_scaffolded_all_output_formats_validate/skill_claude (0.00s)
```

Expected: All checks match. Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | --skill flag with enum {claude}; unknown value exits 3 | `TestInit_SkillUnknown_Exit3`, `TestIsSupportedSkill` | PASS |
| 2 | Bare init byte-identical to M9-005 | `TestInit_BareInit_ByteIdenticalToM9005`, `TestInit_DefaultUnchanged` | PASS |
| 3 | --skill claude defaults markdown + events; --output flag honored | `TestInit_SkillClaude_DefaultsMarkdown`, `TestInit_SkillClaude_ExtendsOutputBlock`, `TestInit_SkillClaude_OutputOverride` | PASS |
| 4 | .gitignore extended with .apitest/; bare init still only .env | `TestInit_SkillClaude_ExtendsGitignore`, `TestEnsureGitignoreMultipleEntries` | PASS |
| 5 | Embedded SKILL.md rendered with {{apitest_version}} substituted | `TestInit_SkillClaude_VersionInTemplate`, `TestRender_SkillClaude_SubstitutesVersion` | PASS |
| 6 | --help documents --skill flag with enum | `TestInit_SkillFlag_DocumentsInHelp` | PASS |
| 7 | Pre-existing responses/ not touched | `TestInit_SkillClaude_PreservesExistingResponsesDir` | PASS |
| 8 | Placeholder SKILL.md ships with version comment as stable marker | `TestInit_SkillClaude_VersionInTemplate` | PASS |
| 9 | Schema parity: skill_claude scaffold validates against project-v1.json | `TestSchema_scaffolded_all_output_formats_validate/skill_claude` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` passes all packages | PASS |
| 2 | TestInit_SkillClaude_CopiesFile | scaffold_test.go — PASS | PASS |
| 3 | TestInit_SkillClaude_DefaultsMarkdown | scaffold_test.go — PASS | PASS |
| 4 | TestInit_SkillClaude_ExtendsOutputBlock | scaffold_test.go (6 subtests) — PASS | PASS |
| 5 | TestInit_SkillClaude_ExtendsGitignore | scaffold_test.go — PASS | PASS |
| 6 | TestInit_SkillClaude_PreservesExistingResponsesDir | scaffold_test.go — PASS | PASS |
| 7 | TestInit_SkillClaude_OutputOverride | scaffold_test.go — PASS | PASS |
| 8 | TestInit_SkillClaude_VersionInTemplate | main_test.go — PASS | PASS |
| 9 | TestInit_SkillFlag_DocumentsInHelp | main_test.go — PASS | PASS |
| 10 | TestInit_SkillUnknown_Exit3 | main_test.go — PASS | PASS |
| 11 | TestInit_SkillClaude_FullPipeline | main_test.go — PASS | PASS |
| 12 | TestSchema_scaffolded_all_output_formats_validate extended | schema/validate_test.go — PASS | PASS |
| 13 | Regression: bare init byte-identical | main_test.go + scaffold_test.go — PASS | PASS |
| 14 | Regression: existing M9-005 tests pass | `go test ./...` — PASS | PASS |
| 15 | go test ./... passes | ci-local.sh — PASS | PASS |
| 16 | Coverage >= 80% (scaffold, cmd/apitest, templates) | 86.6% / 81.3% / 90.9% | PASS |
| 17 | golangci-lint run passes 0 issues | ci-local.sh — PASS | PASS |
| 18 | ./smoke/run.sh passes | ci-local.sh — PASS | PASS |
| 19 | ./scripts/ci-local.sh passes | ci-local PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS trusted (iteration 2, post-improve). Spot-check clean:
- Error handling: `fmt.Errorf("reading embedded claude SKILL.md: %w", err)` — correct `%w` wrapping
- Exported symbols: `ErrUnknownSkill`, `SupportedSkills`, `SkillRelativePath`, `Render`, `IsSupportedSkill` all have doc comments
- Tests: `TestInit_SkillClaude_VersionInTemplate` exercises real binary, asserts version comment and no unreplaced tokens

## Commits

| Hash | Message |
|------|---------|
| c6dd0ea | docs(review): add passing review for M10-001 |
| 3c5f271 | docs(review): add improvement report for M10-001 |
| 6c681b8 | fix(scaffold,cli): rename 5 DoD test names + add TestInit_SkillClaude_VersionInTemplate |
| ce3575a | docs(review): add review with findings for M10-001 |
| f6360ff | chore(task): mark M10-001 as review |
| f5c45e7 | docs(plan): update plan with deviations for M10-001 |
| ae6e5f6 | feat(schema): add output_project_skill_claude.yaml fixture and extend parity test |
| ac63072 | test(schema): add failing test for skill_claude schema fixture |
| 508a948 | feat(cli): add --skill flag to init subcommand |
| 847108f | test(cli): add failing tests for --skill flag CLI wiring |
| 03c5642 | feat(scaffold): add SkillName/ApitestVersion to Options, installSkill helper |
| a6b815f | test(scaffold): add failing tests for skill scaffolding |
| 7440a64 | feat(scaffold): refactor ensureGitignore to accept a slice of entries |
| 361ebe5 | test(scaffold): add failing tests for multi-entry ensureGitignore |
| 80dabb9 | test(templates): add failing tests for templates package |
| ae28312 | chore(task): mark M10-001 as in_progress |
| b29a394 | chore(task): mark M10-001 as planned |
| 6c9f8cf | docs(plan): add implementation plan for M10-001 |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/apitest/main.go` | modified | +74/-5 |
| `cmd/apitest/main_test.go` | modified | +152/-0 |
| `internal/scaffold/scaffold.go` | modified | +76/-14 |
| `internal/scaffold/scaffold_test.go` | modified | +244/-6 |
| `internal/schema/testdata/output_project_skill_claude.yaml` | added | +8 |
| `internal/schema/validate_test.go` | modified | +19/-0 |
| `templates/skills/claude/apitest/SKILL.md` | added | +17 |
| `templates/templates.go` | added | +58 |
| `templates/templates_test.go` | added | +70 |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
