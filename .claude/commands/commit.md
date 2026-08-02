Prepare and create a commit for the current changes.

Context (if provided): $ARGUMENTS

Pipeline: `/plan` → `/execute` → `/review` → `/improve` (if FAIL) → `/review` → `/verify`

---

## Step 0: Verify Branch

Run now:
```bash
git branch --show-current
```

**STOP** if output is `main` or `master`. Say:
> "You are on `main`. All development MUST happen on feature branches. Create a branch first:
> `git checkout -b feature/<TASK-ID>-description`"

Branch naming conventions:
- `feature/<TASK-ID>-description` — New features
- `fix/<TASK-ID>-description` — Bug fixes
- `refactor/<TASK-ID>-description` — Code improvements

---

## Step 1: Review Changes

Run these three commands now. Read the output carefully — do NOT skip this step:

```bash
git status
```

```bash
git diff
```

```bash
git diff --cached
```

Understand every change. Identify:
- Which files are staged vs unstaged
- Whether changes belong to one logical unit or should be split
- Whether any unrelated changes have crept in

**STOP** if there are no changes to commit. Say:
> "No changes detected. Nothing to commit."

---

## Step 2: Pre-commit Quality Gate

Run ALL three commands. ALL must pass before committing:

```bash
go build ./cmd/curlew
```

```bash
go test ./...
```

```bash
~/go/bin/golangci-lint run
```

**STOP** if any command fails. Fix the issue first, then return to Step 1.

---

## Step 3: Stage Changes

Stage only the files that belong to this logical change. Do NOT stage unrelated files.

```bash
git add <file1> <file2> ...
```

Never blindly `git add -A` or `git add .` — review what you are staging.

---

## Step 4: Create Commit

### Commit Message Format

```
type(scope): brief description

- Detail 1
- Detail 2

Refs: TASK-ID
```

### Rules

- **Subject line:** imperative mood, no capitalisation after type, no trailing period, under 50 characters
- **Body:** explain WHY, not WHAT (the diff shows what)
- **Footer:** include `Refs: <TASK-ID>` when working on a task

### Types

| Type | When |
|------|------|
| `feat` | New feature or capability |
| `fix` | Bug fix |
| `refactor` | Code restructuring, no behaviour change |
| `test` | Adding or updating tests only |
| `docs` | Documentation, plans, reviews |
| `chore` | Tooling, config, CI, dependencies |

### Commit Scopes (Curlew-specific)

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
| `commands` | `.claude/commands/` |

### Project-specific Examples

```
test(parser): add failing tests for YAML collection parsing
feat(parser): implement YAML collection file parser
fix(http): handle timeout error wrapping with %w
refactor(variable): extract interpolation into helper
docs(plan): add implementation plan for PRS-001
chore(cli): update golangci-lint configuration
test(assertion): add table-driven tests for status code checks
```

---

## Step 5: TDD Commit Pattern

When following the TDD cycle, commits should appear in this order:

1. **RED** — `test(scope): add failing tests for <feature>`
   Tests written, confirmed failing for the right reason.

2. **GREEN** — `feat(scope): implement <feature>`
   Minimum code to make tests pass. All tests green.

3. **REFACTOR** — `refactor(scope): <what changed>`
   Only if refactoring was done. Tests still green.

Each commit in the sequence MUST build and pass all tests (except the RED commit, where only the NEW tests fail).

---

## Step 6: Execute Commit

```bash
git commit -m "$(cat <<'EOF'
type(scope): brief description

- Detail 1
- Detail 2

Refs: TASK-ID
EOF
)"
```

After committing, verify:
```bash
git log --oneline -3
```

---

## Pre-commit Checklist

Before every commit, mentally verify:

- [ ] On feature branch (not main)
- [ ] `go build ./cmd/curlew` succeeds
- [ ] `go test ./...` passes
- [ ] `~/go/bin/golangci-lint run` passes
- [ ] Only related changes are staged
- [ ] Commit message uses conventional format
- [ ] Scope matches the package being changed
- [ ] Subject line is imperative mood, under 50 chars
- [ ] Task reference included if working on a task
- [ ] One logical change per commit
