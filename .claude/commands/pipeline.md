# Pipeline: Full Task Lifecycle

Run the complete task lifecycle automatically: plan → execute → review → improve (if needed) → verify.

Accepts a task ID (`/pipeline M20-001`), `next` (highest-priority ready task), or `all`
(walk the entire backlog task-by-task until exhausted or a task fails).

Each phase runs in an isolated Agent context to keep the main context lean. Communication
between phases happens through files in `management/` — the same files the individual
commands already produce.

**No human gates.** The pipeline auto-continues through all phases. The review → improve
loop runs up to 10 iterations before giving up.

## Instructions

For task: $ARGUMENTS

### Step 1: Resolve Task

**If `$ARGUMENTS` is a task ID** (e.g., `M2-015`):

1. Read `management/tasks/<TASK-ID>.yaml` — verify it exists
2. Read `management/backlog.yaml` — find the task entry, note its `status` and `title`
3. Read the task's `dependencies` from the YAML file. For each dependency, verify its
   status is `done` in `management/backlog.yaml`. If any dependency is not `done`:
   **STOP** — report which dependencies are incomplete.

**If `$ARGUMENTS` is `next`:**

1. Read `management/backlog.yaml`
2. Collect all tasks with `status: backlog` across all capabilities
3. For each candidate, read `management/tasks/<ID>.yaml` to get its `dependencies` list
4. Filter to tasks where every dependency has `status: done` in backlog.yaml
5. Among qualifying tasks, pick the one with the lowest `priority` number
6. If no tasks qualify: **STOP** — "No tasks ready. All backlog tasks have unmet dependencies."

**If `$ARGUMENTS` is `all`:**

Walk the entire backlog, one task at a time, until it is exhausted:

1. Ensure the working tree is clean and on `main` (`git checkout main && git pull`).
   If the tree is dirty: **STOP** — report the uncommitted changes.
2. Resolve a task using the `next` procedure above (re-read `management/backlog.yaml`
   fresh each iteration — completing a task may have unblocked its dependents).
3. If no tasks qualify:
   - If no tasks with `status: backlog` remain at all → report "Backlog complete." and
     emit the walk summary (below). **DONE.**
   - Otherwise → report which tasks remain and which unmet dependencies block them. **STOP.**
4. Run the full lifecycle (Steps 2–4 below) for the resolved task.
5. If the task result is **SUCCESS**: record it, return to step 1 of this loop.
6. If the task result is **FAILED** at any phase: **STOP the entire walk immediately.**
   Do not start further tasks — a failed task may leave the tree or backlog in a state
   that corrupts subsequent runs. Report which task and phase failed, then emit the
   walk summary.

After the walk ends (complete, blocked, or failed), emit a walk summary:

```
## Backlog Walk Summary

| # | Task | Result | Review iterations | PR |
|---|------|--------|-------------------|-----|
| 1 | M20-001 | SUCCESS | 1 | #238 |
| 2 | M20-002 | FAILED at Execute | — | — |

Remaining in backlog: <count> (<IDs, or "none">)
```

Report: `## Pipeline: <TASK-ID> — <title>`

### Step 2: Determine Starting Phase

Check the task's current `status` in `management/backlog.yaml`:

| Status | Start at | Notes |
|--------|----------|-------|
| `backlog` | Phase A (Plan) | Full lifecycle |
| `planned` | Phase B (Execute) | Plan exists, skip to implementation |
| `in_progress` | Phase B (Execute) | Resume implementation |
| `review` | Phase C (Review) | Code written, needs review |
| `done` | **STOP** | Report "Task already complete." |
| `blocked` | **STOP** | Report "Task is blocked." |

### Step 3: Run Phases

Execute phases sequentially. Each phase uses the **Agent tool** to spawn a **general-purpose
agent** that invokes the corresponding skill via the **Skill tool**. The agent runs in the
shared working directory (**not** a worktree) so git state carries between phases.

**Do not use `isolation: "worktree"`** — phases must share the same branch and commits.

---

#### Phase A: Plan

Spawn a general-purpose Agent with **model: opus**:

```
You are running the PLAN phase of an automated pipeline for task <TASK-ID>.

Use the Skill tool to invoke: /plan <TASK-ID>

Follow all instructions from the skill completely. Important adjustments for pipeline mode:
- Do NOT ask the user for confirmation or approval at any point
- If there are open questions in the Plan agent's analysis, use your best judgment to
  resolve them and document your decisions in the plan
- Do NOT output "next steps" suggestions at the end — just complete your work
- Complete all steps including committing the plan and updating task status to `planned`

When done, state: "Plan phase complete for <TASK-ID>."
```

**After agent returns:**

1. Verify `management/plans/<TASK-ID>-plan.md` exists (use Glob)
2. Read `management/backlog.yaml` — verify task status is now `planned`
3. If either check fails: **STOP** — "Plan phase failed. Review agent output above."

---

#### Phase B: Execute

Spawn a general-purpose Agent with **model: sonnet**:

```
You are running the EXECUTE phase of an automated pipeline for task <TASK-ID>.

Use the Skill tool to invoke: /execute <TASK-ID>

Follow all instructions from the skill completely. Important adjustments for pipeline mode:
- Do NOT ask the user for confirmation at any point
- If the plan needs reconciliation with current code, reconcile and continue
- If the plan has a large correction needed, make your best judgment call,
  update the plan file with the deviation, and continue
- Do NOT output "next steps" suggestions at the end — just complete your work
- Complete all steps including the final review and updating task status to `review`

When done, state: "Execute phase complete for <TASK-ID>."
```

**After agent returns:**

1. Run `go build ./cmd/curlew` — verify it succeeds
2. Run `go test ./...` — verify all tests pass
3. If build or tests fail: **STOP** — "Execute phase produced broken build/tests. Manual intervention needed."
4. Read `management/backlog.yaml` — verify task status is now `review`

---

#### Phase C: Review + Improve Loop

Initialize: `review_iteration = 0`

**Loop:**

1. Increment `review_iteration`

2. **If `review_iteration > 10`:** **STOP** — report:
   ```
   Review failed after 10 iterations. Manual intervention needed.
   Latest findings in: management/reviews/<TASK-ID>-review.md
   ```
   Read and display the latest review findings summary.

3. **Review phase** — Spawn a general-purpose Agent with **model: sonnet**:

   ```
   You are running the REVIEW phase (iteration <review_iteration>) of an automated
   pipeline for task <TASK-ID>.

   Use the Skill tool to invoke: /review <TASK-ID>

   Follow all instructions from the skill completely. Important adjustments for pipeline mode:
   - Do NOT ask the user for anything
   - Do NOT output "next steps" suggestions — just complete your work and commit the report
   - Be thorough but fair — find real issues, not style nitpicks

   When done, state: "Review phase complete. Result: PASS" or "Result: FAIL"
   ```

4. **Check verdict:**
   Read `management/reviews/<TASK-ID>-review.md`.
   Find the `## Verdict:` heading — extract `PASS` or `FAIL` from it.

5. **If PASS:** Report `Review passed on iteration <review_iteration>.` → proceed to Phase D.

6. **If FAIL:**

   Report: `Review iteration <review_iteration> found issues. Running improve...`

   **Improve phase** — Spawn a general-purpose Agent with **model: sonnet**:

   ```
   You are running the IMPROVE phase (iteration <review_iteration> of max 10) of an
   automated pipeline for task <TASK-ID>.

   Use the Skill tool to invoke: /improve <TASK-ID>

   Follow all instructions from the skill completely. Important adjustments for pipeline mode:
   - Do NOT ask the user for anything
   - Do NOT output "next steps" suggestions — just complete your work
   - Focus on resolving ALL findings from the review report
   - Complete all steps including committing fixes and the improvement report

   When done, state: "Improve phase complete for <TASK-ID>."
   ```

   After improve agent returns → continue loop (back to step 1).

---

#### Phase D: Verify

Spawn a general-purpose Agent with **model: sonnet**:

```
You are running the VERIFY phase of an automated pipeline for task <TASK-ID>.

Use the Skill tool to invoke: /verify <TASK-ID>

Follow all instructions from the skill completely. Important adjustments for pipeline mode:
- Do NOT ask the user for confirmation at any point
- Complete all steps including creating the PR, waiting for CI, and merging
- If verification fails, report what failed — do NOT retry

When done, state: "Verify phase complete. Status: PASSED" or "Status: FAILED"
```

**After agent returns:**

1. Read `management/plans/<TASK-ID>-verified.md`
2. Check the `**Verdict:**` line for `PASS` or `FAIL`
3. If `FAIL`: Report the issues from the verification report. **STOP.**

---

### Step 4: Report

After all phases complete (or on failure at any phase):

```
## Pipeline Complete: <TASK-ID>

**Title:** <title>
**Result:** SUCCESS | FAILED at <phase name>
**Branch:** <branch name>
**Review iterations:** <count>
**PR:** <URL if created>

### Phases

| Phase | Status |
|-------|--------|
| Plan | DONE / SKIPPED / FAILED |
| Execute | DONE / SKIPPED / FAILED |
| Review | PASS (iteration N) / FAILED (iteration N) |
| Verify | PASSED / FAILED / NOT REACHED |

### Files Produced

- Plan: management/plans/<TASK-ID>-plan.md
- Review: management/reviews/<TASK-ID>-review.md
- Improvement: management/plans/<TASK-ID>-improved.md (if applicable)
- Verification: management/plans/<TASK-ID>-verified.md (if applicable)
```

If the pipeline succeeded, also identify the next available task:
Read `management/backlog.yaml`, find the highest-priority `backlog` task with all
dependencies `done`, and report: `Next task: <ID> — <title>. Run /pipeline <ID>, /pipeline next, or /pipeline all`
