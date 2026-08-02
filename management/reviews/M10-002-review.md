# Code Review: M10-002

**Task:** agent skill shipment: SKILL.md content, executable-spec playbook test, docs, CHANGELOG, IMPROVEMENT.md W5 status
**Reviewer:** AI
**Date:** 2026-04-25
**Branch:** feature/M10-002-agent-skill-shipment

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error-returning production code in changed files. Test helpers use `t.Fatal` for file-write errors (correct pattern). |
| Input Validation | PASS | No new public functions accepting external input. |
| Naming | PASS | All exported symbols have doc comments. `writePlaybookNamedCollection` avoids collision with unrelated helpers in other test files. No stuttering. |
| Code Organization | PASS | All new code confined to `cmd/apitest/` (test file) and `templates/` (template content). No `internal/` package boundaries crossed. |
| Correctness | PASS | Previous finding #1 (snapshot test was version-sensitive at line 127) resolved: two `bytes.ReplaceAll` calls now cover both the version-comment line and the bold `**version**` form in the Notes section. Golden file verified stable at both positions. |
| Test Quality | PASS | Previous findings #2 and #3 resolved: all three NDJSON helpers now log read errors via `t.Logf` before returning false; MANUAL.md §4.9 corrected to "The following are created or modified in addition to the standard scaffold:" with per-item annotations. |

## Previous Findings — Resolution Verified

| # | Severity | Finding (iteration 1) | Resolution | Verified |
|---|----------|-----------------------|------------|---------|
| 1 | High | `TestSkillClaude_SkillFileSnapshot` only replaced the version-comment form; Notes section `**0.1.0-dev**` survived in golden, breaking on every version bump. | Added `bytes.ReplaceAll(got, []byte("**"+version+"**"), []byte("**v0.0.0-test**"))`. Golden line 127 now reads `**v0.0.0-test**`. | `TestSkillClaude_SkillFileSnapshot` passes; golden verified at lines 6 and 127. |
| 2 | Medium | NDJSON helpers silently discarded `os.ReadFile` errors, masking missing-file failures as "event not found". | `t.Logf("...: events file missing or unreadable: %v", err)` added before each `return false` in all three helpers. | Confirmed present at lines 297, 325, 355. |
| 3 | Medium | MANUAL.md §4.9 said "Five files are created in addition to the bare scaffold" — incorrect count. | Replaced with descriptive prose; each artifact now annotated `(new file)` or `(modified)`. | Verified at `docs/MANUAL.md` line 1506. |

## Spec / Behavior Coverage

| Behavior | Tests / Observable | Status |
|----------|--------------------|--------|
| SKILL.md: frontmatter, version comment, H1, seven H2 sections | `TestSkillClaude_SkillFileSnapshot`; `grep -cE` observable returns 7 | PASS |
| Failure playbook covers exit codes 0,1,2,3,4,5,6,9 (8 rows) | 10 sub-tests in `TestSkillClaude_PlaybookMatchesBinary`; `grep -E '^\| [0-9] '` returns 8 | PASS |
| `responses/run.md` canonical landing, correlation IDs, narration discipline | Content present in SKILL.md at lines 58, 73–75, 79 | PASS |
| Each sub-test: scenario + run + exit-code assert + artifact assert | All 10 sub-tests pass; artifacts asserted per-playbook-row | PASS |
| Exit 9 uses grace-expired fixture (LAST_VALIDATION_OVERRIDE=31d) | `testPlaybookExit9` matches existing `TestRunCmd_GraceExpired_ExitsNine` pattern | PASS |
| SPECIFICATION.md `#### --skill <name> flag` subsection | Present at line 3020; TOC entry at line 142 | PASS |
| MANUAL.md `### 4.9 Driving apitest with Claude Code` with worked example | Present at line 1492; TOC entry at line 48 | PASS |
| CHANGELOG.md `[Unreleased]` `- Added: --skill claude` entry | Present; `awk/grep` observable matched | PASS |
| IMPROVEMENT.md §3 status header flipped to Shipped | Line 3: "Shipped — W1…W5 complete (2026-04-25)" | PASS |
| IMPROVEMENT.md §5 W5 block references M10-001 PR #128 + M10-002 PR #pending | Line 279 confirmed | PASS |
| No live "W5 pending" or "blocked on W4" in tracked docs | `git grep` returns exit 1 (zero matches) | PASS |

## Test Coverage
- Coverage: 81.4% (cmd/apitest) — exceeds the 80% requirement.
- All 10 sub-tests of `TestSkillClaude_PlaybookMatchesBinary` pass.
- `TestSkillClaude_SkillFileSnapshot` passes with stable golden.
- `./scripts/ci-local.sh --go` passes: build, test, race, coverage, lint, smoke.

## Summary

All three findings from the iteration-1 review have been correctly resolved. The snapshot test is now version-stable at both substitution points, NDJSON helpers log file-read errors for easier CI diagnosis, and MANUAL.md §4.9 accurately describes the scaffold output. The implementation is complete, all observables from the task YAML pass, and the full test suite is green at 81.4% coverage.
