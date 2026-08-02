Review code for task: $ARGUMENTS

Pipeline: `/plan` → `/execute` → **`/review`** → `/improve` (if FAIL) → `/review` → `/verify`

**Assume the code is wrong until proven otherwise. Do not rubber-stamp.**

---

## Step 0: Pre-audit Gate (build + tests must be green)

A static audit of red code is worthless. Run the Go gate first — if it fails, the review is already a FAIL and there is no point doing the rest of the audit.

```bash
./scripts/ci-local.sh --go
```

This runs `go build`, `go test`, `go test -race`, coverage, `golangci-lint`, and `./smoke/run.sh`. It skips the docker stack — that is `/verify`'s job.

**If the gate fails:**

1. Write `management/reviews/<TASK-ID>-review.md` with verdict **FAIL** and a single Critical finding:

   ```markdown
   ## Verdict: FAIL

   ## Findings
   | # | Severity | Category | File | Line | Finding | Recommendation |
   |---|----------|----------|------|------|---------|---------------|
   | 1 | Critical | Pre-audit Gate | — | — | `./scripts/ci-local.sh --go` failed at step `<step>` | Fix the failing gate before any static audit can be meaningful |
   ```

2. Paste the last ~40 lines of gate output into the report under a `## Gate Output` heading so `/improve` knows what broke.
3. Commit the review (same commit pattern as the FAIL path below) and tell the user to run `/improve <TASK-ID>`.
4. **Do NOT continue to Step 1.** Skip the rest of the static audit.

If the gate passes, continue.

---

## Step 1: Identify Changed Files

Run now:
```bash
git diff --name-only main...HEAD
```

If no remote tracking:
```bash
git diff --name-only main..HEAD
```

Read EVERY changed source file in full. Do NOT skip test files.

---

## Step 2: Re-read Standards

**MANDATORY: Read the Go Development Standards section in CLAUDE.md now. Do NOT work from memory.**

```
Read CLAUDE.md → "Go Development Standards" section
```

Also read the task definition:
```
Read management/tasks/<TASK-ID>.yaml
```

And the plan:
```
Read management/plans/<TASK-ID>-plan.md
```

---

## Step 3: Error Handling Audit

Find every error return and `fmt.Errorf` call in changed files.

### Go Error Classification

**Expected failures (MUST return errors, never panic):**
- Invalid YAML / malformed collection files
- Bad file paths (not found, permission denied)
- Variable resolution failures (undefined variable, circular reference)
- HTTP failures (timeout, DNS, connection refused)
- Assertion failures (expected vs actual mismatch)
- File I/O errors (read, write, create)
- Invalid user input (bad flags, missing arguments)

**Truly exceptional (panic is acceptable):**
- nil pointer on a programming bug (should not reach user)
- Impossible states that indicate a code defect

For each error in changed code, verify:
- [ ] Wrapped with `fmt.Errorf("context: %w", err)` — NOT `fmt.Errorf("context: %v", err)`
- [ ] Context message describes WHERE, not just WHAT
- [ ] Sentinel errors (`var ErrXxx = errors.New(...)`) used for errors callers need to match with `errors.Is()`
- [ ] No swallowed errors (error returned but never checked by caller)
- [ ] No `panic()` for expected failures

---

## Step 4: Input Validation Audit

For every public function accepting external input in changed files:

- What happens with `nil`? → Should return error, not panic
- What happens with empty string `""`? → Defined behaviour?
- What happens with malformed input? → Clear error message?

**ApiTool-specific input examples:**
- Empty collection file (0 bytes)
- YAML with unknown fields
- Variable reference to undefined variable `${nonexistent}`
- URL with no scheme
- Negative timeout value
- Request with no method specified

---

## Step 5: Naming Audit

Every exported symbol in changed files checked against Effective Go:
- [ ] No stuttering: `parser.Config` not `parser.ParserConfig`
- [ ] Short names in tight scopes (`i`, `err`, `ctx`), descriptive at package level
- [ ] Doc comments on ALL exported types, functions, methods, constants
- [ ] Package names are lowercase, single-word, no underscores
- [ ] Interface names: single-method interfaces use `-er` suffix (e.g., `Reader`, `Resolver`)

---

## Step 6: Code Organization Audit

- [ ] `internal/` package boundaries respected — no reaching into another package's internals
- [ ] Single responsibility per package
- [ ] No circular dependencies between packages
- [ ] `defer` used for cleanup (file handles, HTTP response bodies, locks)
- [ ] No unused imports, variables, or functions
- [ ] Exported surface is minimal — only export what other packages need

---

## Step 7: Correctness Audit

- [ ] Edge cases handled: nil, empty, zero, negative, max values
- [ ] Resource cleanup: files closed, response bodies closed, connections released
- [ ] `context.Context` propagated correctly through call chains
- [ ] No goroutine leaks — every goroutine has a clear termination path
- [ ] `go test -race ./...` would pass (no data races)
- [ ] Boundary conditions: off-by-one, empty collections, single-element collections

---

## Step 8: Test Quality Audit

For every test file in changed files, verify:

1. Are error paths covered? (not just happy path)
2. Are edge cases tested? (empty, nil, boundary values)
3. Are assertions specific? (not just `if err != nil`, but checking the actual error)
4. Are table-driven tests used where there are multiple similar cases?
5. Do subtests use `t.Run()` with descriptive names?
6. Is there an integration test that exercises the real binary via `os/exec`?
7. Are `testdata/` fixtures used for file-based test input?
8. Are ALL behaviors from the task YAML covered by at least one test?

---

## Step 9: Spec Compliance Check

Read the task's `behaviors` list from the YAML. For each behavior:
- Identify the test(s) that verify it
- Verify the implementation matches the specification

If any behavior lacks test coverage, flag it.

---

## Step 10: Write Report

Create `management/reviews/<TASK-ID>-review.md` using this template:

```markdown
# Code Review: <TASK-ID>

**Task:** <title>
**Reviewer:** AI
**Date:** <YYYY-MM-DD>
**Branch:** feature/<TASK-ID>-description

## Verdict: PASS / FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Critical | Error Handling | `internal/pkg/file.go` | 42 | Error swallowed without return | Return wrapped error |
| 2 | High | ... | ... | ... | ... | ... |

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS/FAIL | <notes> |
| Input Validation | PASS/FAIL | <notes> |
| Naming | PASS/FAIL | <notes> |
| Code Organization | PASS/FAIL | <notes> |
| Correctness | PASS/FAIL | <notes> |
| Test Quality | PASS/FAIL | <notes> |

## Test Coverage
- Coverage: <X>%
- Missing coverage: <areas>

## Summary
<2-3 sentences on overall code quality>
```

### All-or-Nothing Rule

**The review FAILS if there are ANY findings at any severity level.**

This is intentional. Even Low-severity findings should be fixed before marking the task as done. The `/improve` command exists to efficiently process all findings.

---

## Output

### On PASS (zero findings)

```
## Review: PASS ✓

No findings. Code meets all standards.

Standards compliance: all categories PASS.
Test coverage: <X>%.

→ Run `/verify <TASK-ID>` to complete the task.
```

Commit the review:
```bash
git add management/reviews/<TASK-ID>-review.md
git commit -m "$(cat <<'EOF'
docs(review): add passing review for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```

### On FAIL (any findings)

```
## Review: FAIL ✗

Found <N> issues: <critical> critical, <high> high, <medium> medium, <low> low.

Top issues:
1. <most critical finding>
2. <second most critical>
3. <third>

→ Run `/improve <TASK-ID>` to fix findings, then `/review <TASK-ID>` again.
```

Commit the review:
```bash
git add management/reviews/<TASK-ID>-review.md
git commit -m "$(cat <<'EOF'
docs(review): add review with findings for <TASK-ID>

Refs: <TASK-ID>
EOF
)"
```
