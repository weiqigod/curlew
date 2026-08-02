# Workflow Hardening Roadmap

## Why this exists

The existing workflow (slash commands, task YAML, vertical slices, DoD contracts) is well-designed but relies on discipline: "remember to validate", "remember to update status", "remember to flag a plan error instead of patching silently." Discipline decays. This roadmap replaces discipline with mechanism and designs explicit detection for Claude's failure modes.

## Principles

1. **Mechanism over discipline** — every "remember to X" is a bug; make the system refuse invalid state.
2. **Catch Claude's lies cheaply** — assume Claude will write fake tests, claim completion falsely, hallucinate APIs, and reinvent existing utilities. Design detection in.
3. **Fix inputs before adding output filters** — context engineering (packages index, session state) before static analyzers.
4. **Kill theater** — if a solo dev won't read it weekly, don't build it.

## Priority order (1 day/week budget)

### 1. Task YAML schema + CI validation
- `management/schemas/task.schema.json` (JSON Schema)
- CI step (GitHub Action) validates every `management/tasks/*.yaml` on PR
- `.claude/settings.json` `PostToolUse` hook runs the same validator on edits in-session
- **Value:** Claude cannot write malformed task files; schema drift caught immediately.
- **Cost:** ~2 hours.

### 2. Incident capture: `management/lessons/`
- New directory, one markdown file per correctable Claude mistake: *what it did, what was right, what rule would have caught it*
- `/plan` and `/review` slash commands load the index at start
- Compound-interest mechanism: the workflow gets smarter over time
- **Value:** the single biggest long-term leverage — every correction becomes permanent context.
- **Cost:** ~1 hour setup, then ongoing when incidents occur. *Build this early or the lessons evaporate.*

### 3. Session continuity: `WORKING_STATE.md`
- Written at end of session, read at start by every slash command
- Fields: active task ID, branch, last command, last failing test, last checkpoint, intended next step
- `/resume` command diffs checkpoint against working tree, reports what's stale
- `Stop` hook writes the update automatically when possible
- **Value:** eliminates the biggest solo+Claude failure mode — context amnesia across sessions. Re-deriving state from git log is lossy and Claude guesses wrong.
- **Cost:** ~3 hours.

### 4. Plan-amendment protocol
- When `/execute` discovers the plan is wrong, it MUST emit `management/plans/<ID>-plan-amendment.md` and flip task status back to `planned`
- No silent reconciliation. No "I patched it mid-flight."
- `/verify` refuses to pass a task if the amendment history is inconsistent with the commits
- **Must land before structured claim verification (#7)** — without a legal "I was wrong" exit, Claude routes around verification by phrasing claims more carefully.
- **Cost:** ~2 hours (update three slash-command prompts + one hook).

### 5. Tautological-test detector
- AST walker (Go) flags test functions that:
  - Contain no assertion call
  - Contain only trivially-true assertions (`assert.True(t, true)`, `assert.Equal(t, x, x)`)
  - Mock every dependency with no real code path exercised
- CI fails on findings
- **Value:** catches Claude's most common completion-theater pattern.
- **Cost:** ~4 hours. Skip mutation testing for now — too slow per PR and too much output to triage solo.

### 6. Packages index: `docs/claude/packages.md`
- Auto-generated from `internal/` directory: package name, one-line purpose, exported symbols
- CI hook regenerates on merges touching `internal/`
- Referenced from `CLAUDE.md` so Claude reads it before proposing new code
- **Value:** prevents reinvention. Claude will reuse what it can see.
- **Cost:** ~2 hours.

### 7. Structured completion report + deterministic verification
- `/verify` requires Claude to emit `management/plans/<ID>-completion.yaml`:
  - `tests_run: [list]`
  - `files_changed: [list]`
  - `coverage_delta: number`
  - `claims: [list of boolean-verifiable statements]`
- A `Stop` hook re-executes the claimed commands and diffs results against the report
- Do NOT attempt natural-language claim parsing — that's a rabbit hole. Structured only.
- **Cost:** ~4 hours.

### 8. Claude-register linter for Go code
- Custom `semgrep` or `go-critic` rules targeting Claude tells:
  - Doc comments that paraphrase the signature (`// GetUser gets a user`)
  - Defensive nil-checks on non-nilable values
  - Premature interfaces with exactly one implementation
  - Over-wrapped errors (`fmt.Errorf("failed to X: %w", err)` where context adds nothing)
  - Dead `TODO` / `FIXME` from abandoned half-implementations
- Run in CI; emit warnings, fail only on severe patterns
- **Value:** higher ROI than adding a generic analyzer because it targets patterns Claude produces specifically.
- **Cost:** ~6 hours. Start with three rules, grow the set from `management/lessons/`.

## Also worth considering (lower priority)

- **Slash-command hardening:** each command declares its *allowed tool surface* (`/plan`: read-only; `/review`: read-only + comment; `/execute`: write within scope). Enforce via `settings.json` per-command restrictions, not prompt discipline. Add preconditions ("fail if no task ID passed", "fail if task not in `planned`").
- **`spec_refs: [§4.2]` field on task YAML** — CI cross-checks: every task has a spec_ref, every spec section is covered by at least one task.
- **Native Go fuzz on parsers only** — config loader, variable interpolation, assertion expressions. Skip fuzzing elsewhere.
- **Pre-commit hook with `go test -run Changed` + `gitleaks`** — via `lefthook`. Conventional-commit format on `commit-msg`.
- **Pinned toolchain via `mise.toml`** — Go, golangci-lint, lefthook, gitleaks. One `mise install` to bring any machine to parity, fixing the current `~/go/bin/golangci-lint`-is-my-laptop fragility.

## Non-goals (do not build)

- **Weekly pipeline dashboards / metrics markdown** — solo devs don't read dashboards they generate for themselves. Replace with a `make stats` on demand if ever needed.
- **GitHub branch protection on this solo repo** — ceremony. A pre-push hook does the same work.
- **Plan line-count complexity budgets** — Claude games any line-count threshold. If a budget is needed, measure behaviors changed via AST diff, otherwise skip.
- **Five overlapping static analyzers** (nilaway + go-critic + semgrep + staticcheck + unused) — cargo cult. Pick one or two and actually triage their output.
- **Stale-task detector** — `git log --since` handles this.
- **Canary tasks with planted bugs** — clever but not worth the maintenance for a solo dev.
- **Reusable workflow extraction to a template repo** — premature. Wait until the workflow has stabilized for 3+ months.
- **External tracker sync (GitHub Issues ↔ `management/tasks/`)** — only if Issues are actually used. Currently they aren't.
- **Splitting `CLAUDE.md` into always-loaded vs on-demand files** — only if `CLAUDE.md` exceeds ~2k tokens. Measure before splitting.

## Sequencing dependencies

- **#1 (schema) before any slash-command rework** — don't harden prompts that read unvalidated YAML.
- **#2 (lessons) before #8 (register linter)** — the linter's rules come from captured lessons.
- **#3 (WORKING_STATE) before #4 (amendments)** — amendments need checkpoint context to be meaningful.
- **#4 (amendments) before #7 (claim verification)** — without a legal "I was wrong" exit, verification creates adversarial Claude, not honest Claude.
- **#6 (packages index) before #8 (register linter)** — fix context input before filtering output.

## Verification per item

- #1: commit a malformed task YAML → CI rejects; edit a task file in-session → `PostToolUse` hook rejects.
- #2: create a lesson file, run `/plan` → plan output references the lesson in its reasoning.
- #3: start a session, do work, end session; new session's `/status` reads `WORKING_STATE.md` and reports the right active task.
- #4: trigger a plan error during `/execute` → amendment file appears, status flips to `planned`, commit history shows the loop.
- #5: write a tautological test → CI fails; delete a real assertion → CI fails.
- #6: rename an `internal/` package → packages index regenerates on merge.
- #7: Claude claims "all tests pass" in `completion.yaml` but one actually fails → `Stop` hook catches the mismatch and blocks task closure.
- #8: write Go code with a one-impl interface → linter warns; write a paraphrasing doc comment → linter warns.
