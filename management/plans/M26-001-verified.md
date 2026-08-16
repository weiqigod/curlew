# Verification Report: M26-001

**Task:** The shipped agent skill stops describing a licensing system that was deleted
**Verified by:** AI
**Date:** 2026-08-16
**Branch:** feature/M26-001-agent-skill-truth
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` (auto-scoped) | PASS | Diff touches only `cmd/curlew/`, `internal/`, `templates/`, `management/`, `CHANGELOG.md` — no `src/ApiTool.Backend/`, `web/`, `ui/`, or docker-compose files, so the script correctly auto-scoped to the Go gate only (`go=1 backend=0 web=0 ui=0 e2e=0`). `set -euo pipefail` means the dogfood/openapi/redaction/crosscheck output only appears if every earlier step (build, backlog integrity, stream-discipline, `go test`, `go test -race`, coverage, `golangci-lint`, telemetry-import guard, smoke, goreleaser check + snapshot) already succeeded; script exited 0 ending in `=== ci-local PASS ===`. |
| `go test ./...` (independent re-run after mutation restore) | PASS | 53 packages ok, 0 failed |
| `go test -race` | PASS | Ran inside `ci-local.sh`; no races reported |
| `golangci-lint run` | PASS | Ran inside `ci-local.sh`; 0 issues |
| `./smoke/run.sh` | PASS | Ran inside `ci-local.sh` |
| Coverage | 86.2% repo-wide; 87.4% `internal/exitcodes`; 81.1% `cmd/curlew` | Independently re-measured via `go tool cover -func=coverage.out` and `go test -cover ./internal/exitcodes/... ./cmd/curlew/...` against the fresh `coverage.out` from this run — matches the review's figures exactly. Meets >= 80% threshold. |

## Observable Output

```
$ go test ./cmd/curlew/ -run TestSkill_exit_codes_are_reachable -v
--- PASS: TestSkill_exit_codes_are_reachable (0.01s)
    --- PASS: .../SKILL.md/failure_playbook_table
    --- PASS: .../exit-codes.md/master_table
    --- PASS: .../exit-codes.md/how_to_read_list
    --- PASS: .../failure-playbook.md/per_code_headings
PASS

$ go test ./cmd/curlew/ -run TestSkill_names_only_real_commands -v
--- PASS: TestSkill_names_only_real_commands (0.01s)
PASS

$ ./curlew init /tmp/probe --skill agent
Project initialized successfully!
$ grep -ril "license\|feature gate\|tier" /tmp/probe/.claude/skills/
(exit 1 — no matches; 11 files scaffolded under .claude/skills/curlew/)
```

Expected: all three observable commands from the task YAML pass as written.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Every exit code documented in the skill is reachable from `cmd/curlew` | `TestSkill_exit_codes_are_reachable` (forward loop, all 4 statements) | PASS |
| 2 | A code the binary can return but the skill omits fails the test, in both directions | `TestSkill_exit_codes_are_reachable` (reverse loop over the union of all 4 statements) | PASS |
| 3 | Every `curlew <subcommand>` named in the skill exists in the CLI's dispatcher | `TestSkill_names_only_real_commands`, `TestCLI_dispatched_and_advertised_commands_agree` | PASS |
| 4 | A new exit code added to `cmd/curlew` without a skill entry fails the suite | `TestSkill_exit_codes_are_reachable` (reverse loop; independently re-verified by mutation, see below) | PASS |
| 5 | The scaffolded skill mentions no licensing, feature gates, or tiers | `TestSkill_scaffold_has_no_licensing_surface`, `TestSkill_licensingPattern_matchesWhatItIsFor` | PASS |

**Behavior 4 independently re-verified by mutation** (not just re-read from the review): added
`case flags.vus < 0: return 7` to `cmd/curlew/perf.go`'s `perfCmdOut` (dynamically unreachable —
`parsePerfArgs` already rejects negative `--vus` earlier — but statically reachable, which is what
the AST walk sees). Re-ran `TestSkill_exit_codes_are_reachable`: failed with exactly
`cmd/curlew can return exit 7 (perf.go:142, in perfCmdOut) but no skill file documents it — an
agent hitting this code has no playbook`. Restored via `git checkout -- cmd/curlew/perf.go`;
`git status --porcelain` confirmed empty immediately after.

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | Licensing, tier, and feature-gate references removed from all three (actually four — `output-formats.md` too) skill files | `grep -rniE "licen[cs]e\|feature.?gate\|tier\|subscription" templates/skills/agent/curlew/` → 0 matches | PASS |
| 2 | Exit codes in the skill asserted equal to the codes the binary can return, both directions | `TestSkill_exit_codes_are_reachable` | PASS |
| 3 | Every curlew subcommand named in the skill asserted to exist | `TestSkill_names_only_real_commands` | PASS |
| 4 | Guards verified by mutation — add a fictional code, confirm failure, restore | Independently re-ran the M3 mutation myself (see Behaviors #4); review independently reproduced M1/M2a/M2b | PASS |
| 5 | A scaffolded project contains no reference to the removed system | Observable #3, re-run against a fresh `/tmp/probe` build | PASS |
| 6 | `./scripts/ci-local.sh --go` passes | Ran plain `./scripts/ci-local.sh` (auto-scope correctly resolved to Go-only given the diff) — PASS | PASS |
| 7 | CHANGELOG.md updated | `## [Unreleased] / ### Fixed` already carries a detailed M26-001 entry (commit `ef4e845`) | PASS |
| 8 | All behavior tests pass | See Behaviors table | PASS |
| 9 | Test coverage >= 80% | 86.2% repo / 87.4% exitcodes / 81.1% cmd/curlew | PASS |
| 10 | No build warnings | `go build -o ./curlew ./cmd/curlew` clean; `golangci-lint` 0 issues | PASS |

## Code Review

Branch A: `management/reviews/M26-001-review.md` verdict is **PASS**, no findings, only two
non-blocking observations (a subsumed regex alternative; an untested-but-verified-correct cycle
guard). Trusted, and spot-checked:

| Check | Status | Evidence |
|-------|--------|----------|
| Error handling (`%w` wrapping, sentinels) | PASS | `internal/exitcodes/reachable.go`: `ErrRootNotFound`/`ErrNoSources` are sentinels; every wrap site uses `fmt.Errorf("...: %w", err)` |
| Doc comments on exports | PASS | `Code`, `Reachable`, `Set`, `Find`, `MaxDepth`, both sentinels all carry rationale-bearing doc comments; package doc explains the composite-literal exclusion design |
| Test quality (tests what it claims) | PASS | `testdata/mapliteral/main.go` genuinely mirrors `discovery_run.go`'s dead `6: 6, // feature gate` map entry (key AND value both 6); `reachable_test.go` asserts `Reachable` returns `{0,1}`, i.e. confirms exclusion |

No standards violations found; Branch B (full checklist) not required.

## Commits

16 commits, all carrying a `Refs: M26-001` trailer (verified programmatically over the full
range), conventional `type(scope): description` format, clear TDD ordering:

| Hash | Message |
|------|---------|
| `7a13491` | docs(plan): add implementation plan for M26-001 |
| `c8b5526` | chore(task): mark M26-001 as planned |
| `5c333c8` | chore(task): mark M26-001 as in_progress |
| `b6e6822` | test(exitcodes): add failing tests for the reachable-exit-code AST walk (RED) |
| `fa9839d` | feat(exitcodes): implement the reachable-exit-code AST walk (Strategy C) (GREEN) |
| `7d61db6` | refactor(exitcodes): extract the shared statement-flattening traversal |
| `2e6a7d7` | test(cli): add failing tests holding the skill's exit-code contract to the binary (RED) |
| `bdd440a` | test(cli): add failing test holding the skill's commands to the dispatcher (RED) |
| `54ca361` | test(cli): add failing word-anchored scan for the removed licensing system (RED) |
| `a7385c2` | fix(cli): strip the removed licensing system from the shipped agent skill (GREEN) |
| `dfea8b5` | test(cli): regenerate the SKILL.md golden snapshot |
| `303b9fb` | test(cli): hold output-formats.md's format table to output.SupportedFormats |
| `e876570` | test(exitcodes): cover the full-value-forward and unresolvable-call branches |
| `ef4e845` | docs(changelog): record the agent skill licensing removal for M26-001 |
| `37259eb` | chore(task): mark M26-001 as review |
| `b9c0d45` | docs(review): add passing review for M26-001 |

Build-integrity spot-check: checked out commit `b6e6822` (first RED commit) into an isolated
`git worktree`; `go build ./...` succeeded cleanly (production code unaffected) while
`go vet ./internal/exitcodes/...` correctly failed on `undefined: ErrRootNotFound` — the expected
and correct TDD RED state, not a broken build. Worktree removed after.

## Files Changed

24 files, +1657/-26:

| File | Action | Lines +/- |
|------|--------|-----------|
| `internal/exitcodes/reachable.go` | created | +406 |
| `cmd/curlew/skill_exit_codes_test.go` | created | +386 |
| `cmd/curlew/skill_commands_test.go` | created | +285 |
| `internal/exitcodes/reachable_test.go` | created | +138 |
| `cmd/curlew/skill_hygiene_test.go` | created | +105 |
| `cmd/curlew/skill_output_formats_test.go` | created | +68 |
| `CHANGELOG.md` | modified | +67 |
| `internal/exitcodes/hints_init.go` | created | +27 |
| `internal/exitcodes/testdata/**` (7 fixtures) | created | +153 |
| `templates/skills/agent/curlew/failure-playbook.md` | modified | +17/-… |
| `templates/skills/agent/curlew/exit-codes.md` | modified | +7/-… |
| `management/backlog.yaml`, `management/tasks/M26-001.yaml` | modified | task lifecycle |
| `templates/skills/agent/curlew/{SKILL,output-formats}.md`, `cmd/curlew/testdata/skill_agent_golden.md`, `internal/errors/coverage_test.go` | modified | small |

`cmd/curlew/discovery_run.go` (M26-002's territory, home of the dead `exitCodeSeverity` map) does
**not** appear in this diff — confirmed via `git diff main...HEAD -- cmd/curlew/discovery_run.go`
(empty) and `git log -1 -- cmd/curlew/discovery_run.go` (last touched by an unrelated pre-existing
commit, `cdaecc3`).

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
