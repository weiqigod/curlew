Create an implementation plan for task: $ARGUMENTS

Pipeline: `/plan` → `/execute` → `/review` → `/improve` (if FAIL) → `/review` → `/verify`

---

## Step 0: Verify Branch

Run now:
```bash
git branch --show-current
```

**STOP** if on `main`. Create the feature branch:
```bash
git checkout -b feature/<TASK-ID>-description
```

---

## Step 1: Load Task

Read `management/tasks/<TASK-ID>.yaml` now. Do NOT work from memory.

Extract these fields:
- `observable` — what you can run to verify
- `behaviors` — testable behavior statements
- `scope` — what needs to be built
- `definition_of_done` — completion criteria
- `dependencies` — prerequisite tasks

**STOP** if status is not `backlog`. Say:
> "Task <TASK-ID> status is `<status>`, expected `backlog`. Cannot plan a task that is already planned/in progress."

---

## Step 2: Verify Dependencies

For each task in `dependencies`, check its status in `management/backlog.yaml`.

**STOP** if any dependency is not `done`. Say:
> "Dependency <DEP-ID> is `<status>`. All dependencies must be `done` before planning. Complete <DEP-ID> first."

---

## Step 3: Explore and Reason (Plan Agent)

**This is the most important step. Do not rush it.**

Use the **Agent tool** with `subagent_type: "Plan"` and `model: "opus"` to perform deep code exploration and architectural reasoning in a dedicated context.

Pass the Plan agent a prompt containing:

1. **Task details** — the `behaviors`, `scope`, `observable`, and `definition_of_done` extracted in Step 1
2. **Project context** — instruct the agent to read `docs/SPECIFICATION.md` and `docs/DEVELOPMENT_PHILOSOPHY.md`
3. **Exploration instructions** — the agent must:
   - Read every file that will be touched (fully, not skimmed)
   - Trace imports in both directions (what does it import? what imports it?)
   - Read every related `_test.go` file and note test helpers/fixtures
   - Check `cmd/curlew/main.go` for wiring and global state
   - Identify all exported functions/types that will change signatures
   - Search for all call sites of changed exports across `internal/` and `cmd/`
   - Identify which existing tests will break and why
4. **Reasoning instructions** — the agent must:
   - Draft an implementation approach
   - Challenge it with edge cases from the specification
   - Iterate — revise the approach if challenges reveal problems
   - Determine step ordering — smallest blast radius first, with a one-sentence rationale for each step's position
   - Propose Go function signatures and `internal/` package boundaries
   - Name table-driven test cases upfront
   - Identify risks, edge cases, and mitigations
5. **Output format** — the agent must return:
   - A summary of code exploration findings (dependencies, contracts, state)
   - The proposed implementation approach with ordered steps
   - For each step: files to modify, before/after code snippets, tests to write first
   - Impact on existing tests
   - Risks and edge cases with mitigations
   - Proposed Go function signatures

Use the Plan agent's output as the basis for Step 4.

---

## Step 4: Write Plan

Create `management/plans/<TASK-ID>-plan.md` using this template:

```markdown
# Implementation Plan: <TASK-ID>

## Overview
<1-2 sentences describing what this task delivers>

## Task Details
- **ID:** <TASK-ID>
- **Title:** <title>
- **Phase:** <phase>
- **Priority:** <priority>
- **Complexity:** <complexity>

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| <dep> | <title> | done |

## Implementation Steps

### Step 1: <Title>
**Rationale:** <why this step comes first>

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/pkg/file.go` | create/modify | <what changes> |
| `internal/pkg/file_test.go` | create/modify | <what changes> |

#### Current Code
```go
// existing code that will change (copy exact lines)
```

#### New Code
```go
// what it will look like after changes
```

#### Tests to Write FIRST (RED phase)

```go
func TestFunctionName(t *testing.T) {
    tests := []struct {
        name    string
        // fields
    }{
        {"descriptive case name", /* ... */},
        {"edge case - empty input", /* ... */},
        {"error case - invalid format", /* ... */},
    }
    // ...
}
```

#### Impact on Existing Tests
- <test name> — will break because <reason>, fix by <action>
- No existing tests affected

### Step 2: <Title>
<same sub-template>

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/pkg/file_test.go` | `TestXxx` | breaks | update assertion |
| — | — | none | — |

## Risks and Edge Cases
- **Risk:** <description> → **Mitigation:** <approach>
- **Edge case:** <description> → **Handling:** <approach>

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
<exact command from task YAML observable field>
```
```

---

## Step 5: Commit Plan

```bash
git add management/plans/<TASK-ID>-plan.md
git commit -m "$(cat <<'EOF'
docs(plan): add implementation plan for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

---

## Step 6: Update Status

In `management/backlog.yaml`, set the task status to `planned`. Add:
- `planned_date: <today YYYY-MM-DD>`
- `branch: feature/<TASK-ID>-description`

Commit:
```bash
git add management/backlog.yaml
git commit -m "$(cat <<'EOF'
chore(task): mark <TASK-ID> as planned

Refs: <TASK-ID>
EOF
)"
```

---

## Step 7: Output

Summarize:
1. Key architectural decisions made
2. Number of implementation steps
3. Files to be created/modified (count)
4. Tests to be written (count)
5. Open questions or risks

Confirm: "Plan committed. Run `/execute <TASK-ID>` to begin TDD implementation."

---

## Quality Checklist

- [ ] Every file to be modified has been fully read (not skimmed)
- [ ] Every affected `_test.go` file has been read
- [ ] All call sites of changed interfaces identified
- [ ] Before/after code snippets for non-trivial changes
- [ ] Impact on existing tests explicitly listed
- [ ] Edge cases and risks identified with mitigations
- [ ] Tests specified BEFORE implementation (TDD)
- [ ] Go function signatures include `context.Context` where appropriate
- [ ] Error wrapping uses `fmt.Errorf("context: %w", err)` pattern
- [ ] Sentinel errors defined for well-known failure modes
- [ ] No stuttering in names (`parser.Config` not `parser.ParserConfig`)
- [ ] Table-driven test cases named upfront
- [ ] Steps ordered by blast radius (smallest first)
- [ ] Observable verification command is concrete and runnable
- [ ] Plan file matches the template structure
- [ ] Plan committed to the feature branch
- [ ] Backlog status updated to `planned`
