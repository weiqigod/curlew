Verify and complete task: $ARGUMENTS

Pipeline: `/plan` → `/execute` → `/review` → `/improve` (if FAIL) → `/review` → **`/verify`**

This is the final verification gate. On **PASS**: creates PR and merges. On **FAIL**: keeps status at `review`.

---

## Step 0: Verify Branch

Run now:
```bash
git branch --show-current
```

**STOP** if on `main`. Say:
> "You are on `main`. Switch to the task's feature branch."

Check all changes are committed:
```bash
git status
```

**STOP** if there are uncommitted changes. Say:
> "Uncommitted changes detected. Commit or stash before verification."

---

## Step 1: Load Context

Read ALL of these files now. Do NOT work from memory:

1. `management/tasks/<TASK-ID>.yaml`
2. `management/plans/<TASK-ID>-plan.md`
3. `management/reviews/<TASK-ID>-review.md` (if exists)
4. `management/plans/<TASK-ID>-improved.md` (if exists)
5. `management/backlog.yaml`

**STOP** if task status is not `review`. Say:
> "Task <TASK-ID> status is `<status>`. Expected `review`. Complete the review cycle first."

---

## Step 2: Run All Quality Checks

Run the local CI gate. This mirrors `.github/workflows/e2e-m4.yml` and `e2e-m5.yml` — the Go gate always runs, and the .NET, web, and Playwright E2E gates run automatically when `src/ApiTool.Backend/**`, `web/**`, or `docker-compose.test.yml` / `scripts/test-stack.sh` have changed on this branch.

```bash
./scripts/ci-local.sh
```

Flags, if the auto-scope is wrong:
- `./scripts/ci-local.sh --full` — force every gate (backend + web + E2E, spins up the docker stack)
- `./scripts/ci-local.sh --go` — Go gate only, skip the docker stack

Record coverage from the script's `go coverage` step. Coverage must be >= 80%.

**STOP** if `ci-local.sh` exits non-zero or coverage is below 80%. The failing gate is the last `=== ... ===` heading printed. Jump to **On FAIL** output.

Rationale: `/review` is static-only and does not run tests. Running the full CI gate here — rather than the Go-only subset — catches .NET, web, and integration-environment regressions before they hit the GitHub runner and cost a ~5-minute round-trip per attempt.

---

## Step 3: Verify Observable

Read the `observable` field from the task YAML. Execute the exact observable scenario:

1. Build the binary (if not already built):
   ```bash
   go build -o ./curlew ./cmd/curlew
   ```
2. Run the observable command from the task YAML
3. Verify the output matches expectations

**STOP** if observable verification fails. Record the expected vs actual output. Jump to **On FAIL**.

---

## Step 4: Verify Behaviors

For each `behavior` in the task YAML:
1. Identify the test(s) that verify this behavior
2. Run the specific test:
   ```bash
   go test -v -run TestName ./internal/package/
   ```
3. Confirm the test passes and actually exercises the behavior (not just checking for no error)

Record a table:

| Behavior | Test(s) | Status |
|----------|---------|--------|
| <behavior text> | `TestXxx` | PASS/FAIL/MISSING |

**STOP** if any behavior is FAIL or MISSING. Jump to **On FAIL**.

---

## Step 5: Verify Definition of Done

For each item in `definition_of_done` from the task YAML:
1. Verify with concrete evidence (test output, file existence, command output)
2. Record the evidence

| DoD Item | Evidence | Status |
|----------|----------|--------|
| All behavior tests pass | `go test ./...` output | PASS/FAIL |
| Observable output works | Command output matches | PASS/FAIL |
| Test coverage >= 80% | `go tool cover` shows X% | PASS/FAIL |
| No build warnings | Clean `go build` output | PASS/FAIL |

**STOP** if any DoD item fails. Jump to **On FAIL**.

---

## Step 6: Verify Plan Completion

Read the plan. For each implementation step:
- Is it marked complete?
- Were the files created/modified as listed?
- Were any deviations documented?

---

## Step 7: Code Review Check

### Branch A: Review PASS Exists

If `management/reviews/<TASK-ID>-review.md` exists with verdict **PASS**:

Trust the review but spot-check 2-3 items:
1. Pick a random error handling site — verify `%w` wrapping
2. Pick a random exported symbol — verify doc comment exists
3. Pick a random test — verify it tests what it claims to test

If spot-check reveals issues, switch to Branch B.

### Branch B: No PASS Review (or spot-check failed)

Perform a condensed code review against Go standards:

```bash
git diff --name-only main...HEAD
```

Read every changed file. Check:

- [ ] Errors returned, not panicked
- [ ] Errors wrapped with `fmt.Errorf("context: %w", err)`
- [ ] Sentinel errors for well-known failure modes
- [ ] No stuttering in names
- [ ] Effective Go naming conventions
- [ ] Doc comments on all exports
- [ ] `context.Context` as first parameter where appropriate
- [ ] `defer` for cleanup
- [ ] No goroutine leaks
- [ ] No unprotected shared mutable state
- [ ] Table-driven tests
- [ ] Integration tests via `os/exec`

If ANY standard is violated, record it. Jump to **On FAIL**.

---

## Step 8: Verify Commits

```bash
git log --oneline main..HEAD
```

Check:
- [ ] All commits reference the task ID (`Refs: <TASK-ID>`)
- [ ] Conventional commit format used (`type(scope): description`)
- [ ] TDD pattern visible: `test(...)` commits appear before `feat(...)` commits
- [ ] Each commit builds cleanly (no broken intermediate states)

---

## Step 9: Generate Verification Report

Create `management/plans/<TASK-ID>-verified.md`:

```markdown
# Verification Report: <TASK-ID>

**Task:** <title>
**Verified by:** AI
**Date:** <YYYY-MM-DD>
**Branch:** feature/<TASK-ID>-description
**Verdict:** PASS / FAIL

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | <N> tests, <time> |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage | <X>% | Meets >= 80% threshold |

## Observable Output

```
<actual output from running the observable command>
```

Expected: <what was expected>
Result: MATCH / MISMATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | <behavior text> | `TestXxx` | PASS |
| 2 | <behavior text> | `TestYyy` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | test output | PASS |
| 2 | Observable output works | command output | PASS |
| 3 | Test coverage >= 80% | cover report | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

(Branch A: "Review PASS trusted, spot-check clean" / Branch B: full checklist results)

## Commits

| Hash | Message |
|------|---------|
| <short hash> | <commit message> |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `internal/pkg/file.go` | modified | +50/-10 |

## Issues Found
None / <list issues>

## Recommendation
PASS — ready for PR and merge / FAIL — <what needs to happen>
```

Commit the report:
```bash
git add management/plans/<TASK-ID>-verified.md
git commit -m "$(cat <<'EOF'
docs(verify): add verification report for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

---

## Step 10: Update Changelog

Add entry under `## [Unreleased]` in `CHANGELOG.md`:

```markdown
### Added
- <what was added> (<TASK-ID>)
```

or `### Fixed`, `### Changed` as appropriate.

```bash
git add CHANGELOG.md
git commit -m "$(cat <<'EOF'
docs(changelog): add <TASK-ID> to unreleased

Refs: <TASK-ID>
EOF
)"
```

---

## Step 11: Update Task Status

Update `management/backlog.yaml`:
- Status: `done`
- Add: `completed_date: <YYYY-MM-DD>`
- Add: `verified_by: ai`

Update `management/tasks/<TASK-ID>.yaml`:
- Status: `done`

```bash
git add management/backlog.yaml management/tasks/<TASK-ID>.yaml
git commit -m "$(cat <<'EOF'
chore(task): mark <TASK-ID> as done

Refs: <TASK-ID>
EOF
)"
```

---

## Step 12: Push and Create PR

```bash
git push -u origin feature/<TASK-ID>-description
```

Create the pull request:
```bash
gh pr create --title "feat(<scope>): <task title>" --body "$(cat <<'EOF'
## Summary
- <what was implemented>
- <key design decisions>

## Task
<TASK-ID>: <title>

## Test Results
- All tests pass (`go test ./...`)
- Coverage: <X>%
- Lint clean (`golangci-lint run`)
- Smoke test passes

## Behaviors Verified
- <behavior 1>
- <behavior 2>

## Definition of Done
All items verified. See `management/plans/<TASK-ID>-verified.md` for full report.

---
EOF
)"
```

---

## Step 13: Merge

**Remote GitHub Actions is paused** (account billing limit). The local CI gate from Step 2 (`./scripts/ci-local.sh`) is the authoritative signal — it mirrors `.github/workflows/e2e-m4.yml` and `e2e-m5.yml`. Do **not** run `gh pr checks --watch`; remote checks will be red regardless of code health.

Merge directly:
```bash
gh pr merge --squash --delete-branch --admin
```

(`--admin` bypasses any branch-protection rule that requires green remote checks. Drop the flag once billing is restored.)

Update local main:
```bash
git checkout main
git pull origin main
```

> **When billing is restored:** revert this step to wait on `gh pr checks --watch` before merging, and remove `--admin`.

---

## Step 14: Ready for Next Task

Identify the next task from `management/backlog.yaml`:
- Must be `backlog` status
- Must have all dependencies `done`
- Prefer highest priority

Say: "Task <TASK-ID> complete and merged. Next candidate: <NEXT-ID> (<title>). Run `/plan <NEXT-ID>` to begin."

---

## On PASS

Full output:
```
## Verification: PASS ✓

Task: <TASK-ID> — <title>
Coverage: <X>%
Tests: <N> pass, 0 fail
Behaviors: <N>/<N> verified
DoD: <N>/<N> complete

PR created: <PR-URL>
Merged to main.

→ Next task candidate: <NEXT-ID> (<title>)
→ Run `/plan <NEXT-ID>` to continue.
```

---

## On FAIL

Do NOT update changelog, task status, or create PR.

```
## Verification: FAIL ✗

Task: <TASK-ID> — <title>

Failed checks:
- <check 1>: <reason>
- <check 2>: <reason>

Issues found:
1. <issue description>
2. <issue description>

→ Fix the issues, then run `/review <TASK-ID>` → `/improve <TASK-ID>` → `/verify <TASK-ID>`
```

Keep task status at `review`. Do NOT create a PR.
