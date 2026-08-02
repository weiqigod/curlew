# Verification Report: M19-006

**Task:** Expand templates/skills/claude/curlew/ to multi-file skill
**Verified by:** AI
**Date:** 2026-05-17
**Branch:** feature/M19-006-multi-file-skill
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean (incl. M19-001, M19-004, M19-005 gates) |
| Coverage (`templates`) | 87.5% | Meets >= 80% threshold |
| Coverage (`internal/scaffold`) | 87.1% | Meets >= 80% threshold |
| Coverage (`cmd/curlew`) | 81.5% | Meets >= 80% threshold |

## Observable Output

```
$ go build -o ./curlew ./cmd/curlew && rm -rf /tmp/curlew-skill-check && mkdir -p /tmp/curlew-skill-check && (cd /tmp/curlew-skill-check && /Users/peterlindqvist/kod/active/Curlew/curlew init --skill claude) && ls /tmp/curlew-skill-check/.claude/skills/curlew/

Project initialized successfully!

Created:
  curlew.yaml
  .gitignore
  .env.example
  environments/dev.yaml
  collections/sample.yaml
  .claude/skills/curlew/SKILL.md

assertions.md
exit-codes.md
expressions.md
failure-playbook.md
output-formats.md
parallel.md
retry.md
signing.md
SKILL.md
variables.md
vault.md
```

Expected: root `SKILL.md` plus 10 per-topic files (`variables.md`, `output-formats.md`, `assertions.md`, `retry.md`, `parallel.md`, `vault.md`, `signing.md`, `expressions.md`, `exit-codes.md`, `failure-playbook.md`).
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `curlew init --skill claude` materialises root SKILL.md + 10 per-topic files | `TestInit_SkillClaude_WritesAllTopicFiles`, `TestSkillClaude_MultiFile_PresentAfterInit` | PASS |
| 2 | Root SKILL.md contains trigger phrases + topic index with one-line summaries | `TestSkillClaude_MultiFile_SkillMdIndexesTopicFiles`, `TestSkillClaude_MultiFile_TriggerPhrasesPreserved` | PASS |
| 3 | `expressions.md` documents `if:`, `cel:`, standard activation, disabled functions, decision table | `TestSkillClaude_MultiFile_ExpressionsDocumentsCEL` | PASS |
| 4 | `failure-playbook.md` has entries for `ERR_CEL_PARSE` and `ERR_CEL_TYPE` + `curlew validate` | `TestSkillClaude_MultiFile_PlaybookHasCELErrorCodes` | PASS |
| 5 | Embedded-FS walker writes every file recursively | `TestInit_SkillClaude_WritesAllTopicFiles`, `TestWalk_ClaudeYieldsAllTopicFiles` | PASS |
| 6 | Existing `TestSkillPlaybook` still passes against the multi-file layout | `TestSkillClaude_PlaybookMatchesBinary` (10 sub-tests), `TestSkillClaude_SkillFileSnapshot` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages pass | PASS |
| 2 | Test coverage >= 80% for new code in cmd/curlew/init.go | templates: 87.5%, scaffold: 87.1%, cmd/curlew: 81.5% | PASS |
| 3 | No build warnings or lint errors | `golangci-lint run` clean | PASS |
| 4 | `./scripts/ci-local.sh` passes | `ci-local PASS` — all gates green | PASS |
| 5 | `curlew init --skill claude` materialises multi-file skill | ls output shows SKILL.md + 10 topic files | PASS |
| 6 | Existing `TestSkillPlaybook` updated and passes | All 10 `TestSkillClaude_PlaybookMatchesBinary` sub-tests PASS | PASS |
| 7 | `failure-playbook.md` includes `ERR_CEL_PARSE` and `ERR_CEL_TYPE` | `TestSkillClaude_MultiFile_PlaybookHasCELErrorCodes` PASS | PASS |
| 8 | `docs/MANUAL.md` untouched | Not in `git diff --name-only main...HEAD` | PASS |

## Code Review

Branch A: Review PASS trusted (iteration 2 review verdict: PASS). Spot-check performed:

| Check | Status |
|-------|--------|
| Error handling — `%w` wrapping | PASS — `Walk`, `installSkill`, `Render` all use `fmt.Errorf("...: %w", err)` |
| Exported symbols have doc comments | PASS — `Walk`, `SkillRootDir`, `ErrUnknownSkill`, `SupportedSkills`, `installSkill` (unexported, has block comment) all documented |
| Test quality — `TestSkillClaude_MultiFile_PlaybookHasCELErrorCodes` exercises real behavior | PASS — reads the embedded file and asserts specific strings |

## Commits

| Hash | Message |
|------|---------|
| 6a543088 | docs(plan): add implementation plan for M19-006 |
| daf32850 | chore(task): mark M19-006 as planned |
| 94cce033 | chore(task): mark M19-006 as in_progress |
| 5abfee3b | test(templates,scaffold,cli): add failing tests for multi-file skill (M19-006) |
| 64ec6f06 | feat(templates,scaffold): expand skill to multi-file layout (M19-006) |
| 8ecbc641 | chore(task): mark M19-006 as review |
| 074e02c3 | docs(review): add review with findings for M19-006 |
| 89b15e91 | fix(templates): document WalkDir discard, guard CutPrefix bool, add decision-table assertion |
| 645a1980 | docs(review): add improvement report for M19-006 |
| e3259d11 | docs(review): add passing review for M19-006 (iteration 2) |

## Files Changed

| File | Action |
|------|--------|
| `cmd/curlew/skill_playbook_test.go` | modified — added 5 `TestSkillClaude_MultiFile_*` tests |
| `cmd/curlew/testdata/skill_claude_golden.md` | modified — snapshot regenerated for multi-file SKILL.md |
| `internal/scaffold/scaffold.go` | modified — `installSkill` now walks directory tree |
| `internal/scaffold/scaffold_test.go` | modified — added `TestInit_SkillClaude_WritesAllTopicFiles`, `TestInit_SkillClaude_PreservesExistingTopicFile` |
| `templates/templates.go` | modified — added `Walk`, `SkillRootDir`, widened embed to `all:skills/claude/curlew` |
| `templates/templates_test.go` | modified — added `TestWalk_*` tests |
| `templates/skills/claude/curlew/SKILL.md` | modified — added topic-files index section |
| `templates/skills/claude/curlew/assertions.md` | created |
| `templates/skills/claude/curlew/exit-codes.md` | created |
| `templates/skills/claude/curlew/expressions.md` | created |
| `templates/skills/claude/curlew/failure-playbook.md` | created |
| `templates/skills/claude/curlew/output-formats.md` | created |
| `templates/skills/claude/curlew/parallel.md` | created |
| `templates/skills/claude/curlew/retry.md` | created |
| `templates/skills/claude/curlew/signing.md` | created |
| `templates/skills/claude/curlew/variables.md` | created |
| `templates/skills/claude/curlew/vault.md` | created |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
