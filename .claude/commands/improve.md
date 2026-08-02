Fix review findings for task: $ARGUMENTS

Pipeline: `/plan` → `/execute` → `/review` → **`/improve`** (if FAIL) → `/review` → `/verify`

**Do NOT change task status. Only `/verify` transitions a task to `done`.**

---

## Step 0: Verify Branch

Run now:
```bash
git branch --show-current
```

**STOP** if on `main`. Say:
> "You are on `main`. Switch to the task's feature branch."

---

## Step 1: Load Context

Read ALL of these files now. Do NOT work from memory:

1. `management/tasks/<TASK-ID>.yaml`
2. `management/plans/<TASK-ID>-plan.md`
3. `management/reviews/<TASK-ID>-review.md`

### STOP Conditions

**STOP** if review file does not exist. Say:
> "No review found at `management/reviews/<TASK-ID>-review.md`. Run `/review <TASK-ID>` first."

**STOP** if review verdict is PASS. Say:
> "Review already passed. No findings to fix. Run `/verify <TASK-ID>` instead."

**STOP** if task status is not `review` or `in_progress`. Say:
> "Task <TASK-ID> status is `<status>`. Expected `review` or `in_progress`."

---

## Step 2: Read Changed Files

Run:
```bash
git diff --name-only main...HEAD
```

Read EVERY changed source file in full. Do NOT work from memory.

---

## Phase 1: Triage (Steps 3-4)

### Step 3: Extract Findings

From the review report, build a ledger of all findings:

| # | Severity | File | Line | Finding | In Scope? |
|---|----------|------|------|---------|-----------|
| 1 | Critical | ... | ... | ... | Yes/No |

### Step 4: Scope Check

For each finding, determine scope:

- **In-scope** (MUST fix): Finding is in a file changed by this task
- **Pre-existing** (MAY defer): Finding is in code that existed before this task

In-scope findings MUST be fixed. Pre-existing findings MAY be deferred with written rationale.

---

## Phase 2: Plan Fixes (Step 5 — Plan Agent)

### Step 5: Reason Through Fixes

**Do not write any code until this step is complete.**

Use the **Agent tool** with `subagent_type: "Plan"` and `model: "opus"` to reason through the fix strategy in a dedicated context.

Pass the Plan agent a prompt containing:

1. **Findings ledger** — the full triage table from Step 3, with scope decisions from Step 4
2. **Current code** — instruct the agent to read every file listed in the findings ledger
3. **Task context** — the task YAML, original plan, and review report paths so the agent can read them
4. **Reasoning instructions** — the agent must:
   - For each in-scope finding, propose a concrete fix approach
   - Identify interactions between findings (e.g., fixing #1 may resolve #3, or fixing #2 may conflict with #4)
   - Determine fix ordering — which fixes should come first to avoid rework
   - For each fix, decide: does it change observable behaviour (TDD required) or is it structural (direct fix)?
   - Identify any fixes that risk breaking existing tests and how to handle them
   - Propose specific code changes (before/after snippets) for non-trivial fixes
   - Flag any findings where the review may be wrong or the suggested fix is suboptimal
5. **Output format** — the agent must return:
   - Ordered list of fixes with rationale for the ordering
   - For each fix: approach, TDD decision, files affected, before/after code snippets
   - Interactions and dependencies between fixes
   - Risks or concerns about any proposed fix

Use the Plan agent's output to guide Phase 3.

---

## Phase 3: Fix (Step 6)

### Step 6: Severity-ordered Processing

Process findings in strict order: **Critical → High → Medium → Low**

For EACH finding:

#### 6a. Read Before Edit

Read the file containing the finding. Understand the context around the problematic code.

#### 6b. TDD Decision

- **If the finding changes observable behaviour** (bug fix, missing error handling, new edge case):
  → TDD required. Write/update a failing test FIRST, then fix.
  ```bash
  # Write test
  go test -v -run TestName ./internal/package/
  # Verify it fails for the right reason
  # Fix the code
  go test -v -run TestName ./internal/package/
  # Verify it passes
  ```

- **If the finding is structural** (naming, wrapping style, code organization, doc comments):
  → No TDD needed. Apply the fix directly.

#### 6c. Quality Gate After EACH Fix

Run after EVERY individual fix, not just at the end:

```bash
go build ./cmd/apitest && go test ./... && ~/go/bin/golangci-lint run
```

**STOP** if any command fails. Fix before continuing to the next finding.

#### 6d. Commit Each Fix

```bash
git add <fixed_files>
git commit -m "$(cat <<'EOF'
fix(scope): <what was fixed>

Review finding #<N>: <brief description>

Refs: <TASK-ID>
EOF
)"
```

Alternatively, if multiple related findings affect the same file, they may be combined into one commit with a clear message listing all resolved findings.

---

## Phase 4: Report (Steps 7-9)

### Step 7: Run Final Quality Gate

```bash
go build ./cmd/apitest
go test ./...
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
~/go/bin/golangci-lint run
```

### Step 8: Generate Report

Create `management/plans/<TASK-ID>-improved.md` using this template:

```markdown
# Improvement Report: <TASK-ID>

**Task:** <title>
**Date:** <YYYY-MM-DD>
**Review:** management/reviews/<TASK-ID>-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | <finding> | <what was done> | ✓ tests pass |
| 2 | High | <finding> | <what was done> | ✓ tests pass |

## Out of Scope (Deferred)

| # | Severity | Finding | Rationale |
|---|----------|---------|-----------|
| — | — | — | — |

(If none: "No findings deferred. All findings resolved.")

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS/FAIL |
| `go test ./...` | PASS/FAIL |
| `golangci-lint run` | PASS/FAIL |
| Coverage | <X>% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| <hash> | <message> | #1, #2 |
| <hash> | <message> | #3 |

## Summary
<resolved>/<total> findings resolved. <deferred> deferred.
```

### Step 9: Commit Report

```bash
git add management/plans/<TASK-ID>-improved.md
git commit -m "$(cat <<'EOF'
docs(review): add improvement report for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

---

## Output

```
## Improvement Complete

Resolved: <N>/<total> findings
- Critical: <n> fixed
- High: <n> fixed
- Medium: <n> fixed
- Low: <n> fixed
Deferred: <n> (with rationale in report)

Quality gate: PASS/FAIL
Coverage: <X>%

→ Run `/review <TASK-ID>` to verify all findings are resolved.
```

**Do NOT suggest `/verify` directly.** The review must pass first. The cycle is:
`/improve` → `/review` → (if PASS) → `/verify`
