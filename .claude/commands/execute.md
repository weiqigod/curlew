Execute TDD implementation for task: $ARGUMENTS

Pipeline: `/plan` → **`/execute`** → `/review` → `/improve` (if FAIL) → `/review` → `/verify`

---

## Step 0: Verify Branch

Run now:
```bash
git branch --show-current
```

**STOP** if on `main`. Say:
> "You are on `main`. Switch to the task's feature branch: `git checkout feature/<TASK-ID>-description`"

**STOP** if no feature branch exists for this task. Say:
> "No feature branch found. Run `/plan <TASK-ID>` first to create the plan and branch."

---

## Step 1: Pre-flight Checks

Read these files now. Do NOT work from memory:

1. `management/tasks/<TASK-ID>.yaml`
2. `management/plans/<TASK-ID>-plan.md`
3. `management/backlog.yaml`

**STOP** if task status is not `planned` or `in_progress`. Say:
> "Task <TASK-ID> status is `<status>`. Expected `planned` or `in_progress`. Run `/plan <TASK-ID>` first."

**STOP** if plan file does not exist. Say:
> "No plan found at `management/plans/<TASK-ID>-plan.md`. Run `/plan <TASK-ID>` first."

Verify all dependency tasks are `done` in backlog.yaml.

**STOP** if any dependency is not `done`. Say:
> "Dependency <DEP-ID> is `<status>`. All dependencies must be `done` before execution."

---

## Step 2: Internalize the Plan

**Do not write any code until this step is complete.**

Read the plan thoroughly. Then read every file listed in the plan's "Files to Modify" tables — the actual files on disk, not just the plan's snippets.

### Reconciliation Check

Compare current code against the plan's "Current Code" snapshots. If code has changed since planning:

**STOP** and say:
> "Code has changed since the plan was written. The following files differ: <list>. Updating plan to reflect current state."

Update the plan file, commit:
```bash
git add management/plans/<TASK-ID>-plan.md
git commit -m "$(cat <<'EOF'
docs(plan): reconcile plan with current code for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

---

## Step 3: Update Status

If task status is `planned`, update to `in_progress` in `management/backlog.yaml`:

```bash
git add management/backlog.yaml
git commit -m "$(cat <<'EOF'
chore(task): mark <TASK-ID> as in_progress

Refs: <TASK-ID>
EOF
)"
```

---

## Step 4: TDD Implementation Loop

For each step in the plan, execute the full RED → GREEN → REFACTOR cycle:

### RED Phase

1. Write failing tests using `testing.T` and `t.Run()` subtests
2. Use table-driven tests with the exact test case names from the plan
3. Run:
   ```bash
   go test -v -run TestFunctionName ./internal/package/
   ```
4. **Verify the test fails for the RIGHT reason.** Read the failure output. The test should fail because the feature is not yet implemented, NOT because of a syntax error, import issue, or wrong test setup.

**STOP** if the test fails for the wrong reason. Fix the test first.

5. Commit:
   ```bash
   git add <test_files>
   git commit -m "$(cat <<'EOF'
   test(scope): add failing tests for <feature>

   Refs: <TASK-ID>
   EOF
   )"
   ```

### GREEN Phase

1. Implement the **minimum code** to make the failing tests pass. No more, no less.

2. **Go Standards Checklist** — After writing implementation code, verify ALL of these:

   - [ ] Errors returned, not panicked
   - [ ] Errors wrapped with `fmt.Errorf("context: %w", err)` for traceable chains
   - [ ] Sentinel errors (`var ErrXxx = errors.New(...)`) for well-known failure modes
   - [ ] No stuttering: `parser.Config` not `parser.ParserConfig`
   - [ ] Effective Go naming: short names in tight scopes, descriptive at package level
   - [ ] Doc comments on all exported symbols
   - [ ] `context.Context` as first parameter where appropriate
   - [ ] `defer` for cleanup (files, connections, locks)
   - [ ] No goroutine leaks — every goroutine has a termination path
   - [ ] No unprotected shared mutable state — channels or scoped mutexes
   - [ ] No unnecessary allocations — `strings.Builder` for string building, `make([]T, 0, capacity)` for known sizes
   - [ ] Minimal external dependencies — standard library first

3. Run:
   ```bash
   go test ./...
   ```
   **All tests must pass.** Not just the new tests — ALL tests.

4. Commit:
   ```bash
   git add <implementation_files>
   git commit -m "$(cat <<'EOF'
   feat(scope): implement <feature>

   Refs: <TASK-ID>
   EOF
   )"
   ```

### REFACTOR Phase

Review the code just written against this checklist:

- [ ] **Duplication** — Any copy-pasted logic that should be extracted?
- [ ] **Single responsibility** — Does each function do one thing?
- [ ] **Naming** — Do names communicate intent? Would a reader unfamiliar with the code understand?
- [ ] **Allocations** — Use `strings.Builder` for string concatenation, `make([]T, 0, capacity)` for slices with known capacity
- [ ] **Data structure fit** — `map[T]struct{}` for sets, appropriate container for the access pattern
- [ ] **Magic values** — Any unexplained constants that should be named?
- [ ] **Exported surface** — Is anything exported that should be internal?

If ANY changes are needed:
1. Refactor
2. Run `go test ./...` — all tests must still pass
3. Commit:
   ```bash
   git add <refactored_files>
   git commit -m "$(cat <<'EOF'
   refactor(scope): <what changed>

   Refs: <TASK-ID>
   EOF
   )"
   ```

If no changes needed, skip this commit.

---

## Step 5: Step-level Gate

After EACH RED+GREEN+REFACTOR cycle, run the full quality gate:

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
```

**STOP** if any command fails. Fix before continuing to the next step.

Review your own diff:
```bash
git diff HEAD~N..HEAD
```
(where N is the number of commits in this cycle)

Cross-reference against the plan's test impact table. If an existing test broke and the plan didn't predict it, update the plan.

---

## Step 6: Track Progress

After each completed step, note which plan steps are done and any deviations. If deviations are significant, update the plan file and commit:

```bash
git add management/plans/<TASK-ID>-plan.md
git commit -m "$(cat <<'EOF'
docs(plan): update plan with deviations for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

---

## Step 7: Final Review

After all plan steps are implemented:

1. **Full diff review:**
   ```bash
   git log --oneline main..HEAD
   git diff main..HEAD
   ```

2. **Standards spot-check.** Search for common issues:
   ```bash
   grep -rn "panic(" internal/ cmd/
   ```
   Every `panic(` must be justified (truly unrecoverable). Search for missing `%w`:
   ```bash
   grep -rn 'Errorf.*[^%]w' internal/ cmd/
   ```

3. **Definition of Done check** — Read each item from the task YAML and verify.

4. **Observable verification** — Run the binary with the task's observable command. Verify output matches expectations.

5. **Integration test** — If the plan specifies an `os/exec`-based integration test, run it:
   ```bash
   go test -v -run TestIntegration ./...
   ```

---

## Step 8: Completion

Run full quality gate:
```bash
go build ./cmd/curlew
go test ./...
go test -coverprofile=coverage.out ./... && go tool cover -func=coverage.out
~/go/bin/golangci-lint run
./smoke/run.sh
```

Coverage must be >= 80%.

Update task status to `review` in `management/backlog.yaml`:
```bash
git add management/backlog.yaml
git commit -m "$(cat <<'EOF'
chore(task): mark <TASK-ID> as review

Refs: <TASK-ID>
EOF
)"
```

Push:
```bash
git push -u origin feature/<TASK-ID>-description
```

Output: "Implementation complete. Run `/review <TASK-ID>` for code review."

---

## Commit Scopes (Curlew-specific)

| Scope | Package / Area |
|-------|---------------|
| `cli` | `cmd/curlew/`, CLI wiring |
| `parser` | `internal/parser/` |
| `http` | `internal/http/` |
| `variable` | `internal/variable/` |
| `assertion` | `internal/assertion/` |
| `output` | `internal/output/` |
| `ai` | `internal/ai/` |
| `auth` | `internal/auth/` |
| `config` | `internal/config/` |
| `plan` | `management/plans/` |
| `verify` | `management/plans/*-verified.md` |
| `review` | `management/reviews/` |
| `task` | `management/tasks/`, `management/backlog.yaml` |

---

## When the Plan is Wrong

Plans are written before implementation and may be wrong. Handle deviations by severity:

### Small Correction
A test case name change, minor signature adjustment, trivial difference.
- Update the plan inline
- Continue implementation
- No special commit needed

### Medium Correction
A missing step, different approach for one component, unexpected test impact.
- Update the plan with a "Deviations" section explaining what changed and why
- Commit the plan update
- Continue implementation

### Large Correction
The approach is fundamentally wrong, a major assumption was invalid, multiple steps need rethinking.
- **STOP.** Do not continue writing code.
- Update the plan with findings
- Commit the plan update:
  ```bash
  git add management/plans/<TASK-ID>-plan.md
  git commit -m "$(cat <<'EOF'
  docs(plan): flag major deviation for <TASK-ID>

  Refs: <TASK-ID>
  EOF
  )"
  ```
- Say: "Major deviation found. The plan needs to be revised. Suggest re-running `/plan <TASK-ID>` to address: <summary of issues>."
