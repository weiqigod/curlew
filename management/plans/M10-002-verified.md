# Verification Report: M10-002

**Task:** agent skill shipment: SKILL.md content, executable-spec playbook test, docs, CHANGELOG, IMPROVEMENT.md W5 status
**Verified by:** AI
**Date:** 2026-04-25
**Branch:** feature/M10-002-agent-skill-shipment
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` (via ci-local) | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (cmd/apitest) | 81.4% | Meets >= 80% threshold |

## Observable Output

```
# SKILL.md has the real content (frontmatter + sections), not the placeholder.
$ awk '/^---$/{c++} c==2{exit} {print}' .claude/skills/apitest/SKILL.md | grep -E '^description: '
description: Run ApiTool collections and interpret results. Use when the user asks to run, test, hit, exercise, or check an HTTP API; ...

$ grep -cE '^## (When invoked|Invocation|Where results land|How to narrate|Failure playbook|What to edit|Notes)$' .claude/skills/apitest/SKILL.md
7

$ grep -E '^\| [0-9] ' .claude/skills/apitest/SKILL.md | wc -l | tr -d ' '
8

$ go test -run 'TestSkillClaude_PlaybookMatchesBinary' -v ./cmd/apitest/...
--- PASS: TestSkillClaude_PlaybookMatchesBinary (0.69s)
    --- PASS: .../exit_0_all_passed
    --- PASS: .../exit_1_assertion_failed
    --- PASS: .../exit_2_guard_rail
    --- PASS: .../exit_3_parse_error
    --- PASS: .../exit_3_config_error
    --- PASS: .../exit_3_only_no_match
    --- PASS: .../exit_4_network_error
    --- PASS: .../exit_5_undefined_variable
    --- PASS: .../exit_6_feature_gate
    --- PASS: .../exit_9_grace_expired

$ grep -c 'init --skill' docs/SPECIFICATION.md
1

$ grep -c 'Driving apitest with Claude Code' docs/MANUAL.md
2

$ awk '/^## \[Unreleased\]/,/^## \[0/' CHANGELOG.md | grep -E '^- Added.*--skill claude'
- Added: --skill claude flag for apitest init scaffolds Claude Code agent skill...

$ head -5 IMPROVEMENT.md | grep -E 'W1.*W5.*[Cc]omplete|Shipped'
**Status:** Shipped — W1, W2, W3 complete (2026-04-24), W4 complete (2026-04-25), W5 complete (2026-04-25)

$ grep -A 1 '^### W5 — Default Claude skill' IMPROVEMENT.md | head -2
### W5 — Default Claude skill packaged with the tool
**Status:** Shipped 2026-04-25 — M10-001 (PR #128, plumbing) + M10-002 (PR #129, real SKILL.md content + executable-spec test + docs).

$ git grep -n 'W5.*[Pp]ending\|blocked on W4' -- docs/ IMPROVEMENT.md CHANGELOG.md
(exit 1 — zero matches)
```

Expected: description present, 7 sections, 8 exit-code rows, all playbook sub-tests PASS, >= 1 spec match, >= 1 manual match, changelog entry present, improvement status Shipped, zero W5-pending/blocked references.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | SKILL.md: frontmatter, version comment, H1, 7 H2 sections, 8-row playbook | `TestSkillClaude_SkillFileSnapshot` | PASS |
| 2 | Failure playbook covers exit codes 0,1,2,3,4,5,6,9 with correct artifacts | `TestSkillClaude_PlaybookMatchesBinary` (10 sub-tests) | PASS |
| 3 | responses/run.md canonical landing, correlation IDs, narration discipline | SKILL.md content observable | PASS |
| 4 | Each sub-test: scenario + run + exit-code assert + artifact assert | 10 sub-tests all PASS | PASS |
| 5 | Exit 9 uses grace-expired fixture (LAST_VALIDATION_OVERRIDE=31d) | `exit_9_grace_expired` sub-test | PASS |
| 6 | SPECIFICATION.md init --skill subsection | `grep -c 'init --skill'` = 1 | PASS |
| 7 | MANUAL.md §4.9 with worked example | `grep -c 'Driving apitest...'` = 2 | PASS |
| 8 | CHANGELOG.md Added entry | observable grep matched | PASS |
| 9 | IMPROVEMENT.md status flipped to Shipped | head-5 grep matched | PASS |
| 10 | Consistency gate (no W5 pending / blocked on W4) | git grep exits 1 | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all packages PASS | PASS |
| 2 | SKILL.md has real content: 7 H2 headers, 8-row playbook | `grep -cE` returns 7; `grep -E '^\| [0-9] '` returns 8 | PASS |
| 3 | `TestSkillClaude_PlaybookMatchesBinary` passes for all codes | 10/10 sub-tests PASS | PASS |
| 4 | `TestSkillClaude_SkillFileSnapshot` passes | PASS — golden verified stable | PASS |
| 5 | SPECIFICATION.md has init --skill subsection | `grep -c 'init --skill'` = 1 | PASS |
| 6 | MANUAL.md has §4.9 worked example | `grep -c 'Driving apitest with Claude Code'` = 2 | PASS |
| 7 | CHANGELOG.md [Unreleased] has --skill claude Added entry | observable grep matched | PASS |
| 8 | IMPROVEMENT.md §3 status flipped to Shipped | line 3 confirmed | PASS |
| 9 | IMPROVEMENT.md §5 W5 block references M10-001 + M10-002 | line 279 confirmed | PASS |
| 10 | No live W5 pending / blocked on W4 | git grep exits 1 (zero matches) | PASS |
| 11 | go test ./... passes | PASS | PASS |
| 12 | Coverage cmd/apitest >= 80% | 81.4% | PASS |
| 13 | golangci-lint run 0 issues | PASS | PASS |
| 14 | ./smoke/run.sh passes | PASS | PASS |
| 15 | ./scripts/ci-local.sh passes | ci-local PASS | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Doc comments on exports | PASS |
| Test quality | PASS |
| No stuttering | PASS |

Branch A: Review PASS (iteration 2) trusted, spot-check clean — doc comments present on both exported test functions, error returns use `t.Logf` before `return false` (correct for test helpers), no stutter.

## Commits

| Hash | Message |
|------|---------|
| 8b1e4a5 | docs(review): add passing review for M10-002 |
| b9154b1 | docs(review): add improvement report for M10-002 |
| fbfea98 | fix(cli,docs): resolve M10-002 review findings #1, #2, #3 |
| 34d8b26 | docs(review): add review with findings for M10-002 |
| 0bc9e0c | chore(task): mark M10-002 as review |
| a57362a | docs(plan): update plan with deviations for M10-002 |
| caa0a87 | feat(docs): ship SPECIFICATION §Project Initialization, MANUAL §4.9, CHANGELOG entry, IMPROVEMENT.md W5 status flip |
| 5444363 | refactor(cli): use fmt.Fprintf in writeMultiRequestCollection |
| bd96028 | feat(templates,cli): replace SKILL.md placeholder with real content + golden snapshot (GREEN) |
| 3cc38a2 | test(cli): add failing playbook tests for skill_claude (RED) |
| 774d8be | chore(task): mark M10-002 as in_progress |
| 96fe4f5 | chore(task): mark M10-002 as planned |
| 1791963 | docs(plan): add implementation plan for M10-002 |

TDD pattern visible: `test(cli)` RED at 3cc38a2 before `feat(templates,cli)` GREEN at bd96028.

## Files Changed

| File | Action |
|------|--------|
| `templates/skills/claude/apitest/SKILL.md` | replaced placeholder with real skill body |
| `cmd/apitest/skill_playbook_test.go` | created — 10-sub-test playbook test + snapshot test |
| `cmd/apitest/testdata/skill_claude_golden.md` | created — byte-stable golden snapshot |
| `docs/SPECIFICATION.md` | added Project Initialization section with init --skill subsection |
| `docs/MANUAL.md` | added §4.9 Driving apitest with Claude Code |
| `CHANGELOG.md` | added M10-002 Added entry under [Unreleased] |
| `IMPROVEMENT.md` | flipped §3 header + §5 W5 block to Shipped |
| `management/backlog.yaml` | updated M10-002 status to review |
| `management/plans/M10-002-plan.md` | added deviations section |
| `management/reviews/M10-002-review.md` | added review (iteration 2 PASS) |
| `management/plans/M10-002-improved.md` | added improvement report |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
