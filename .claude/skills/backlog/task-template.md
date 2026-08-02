# Task Template

## YAML Schema

```yaml
id: M1-NNN                   # e.g., M1-001, M1-015
title: "Short descriptive title"
status: backlog              # backlog | planned | in_progress | review | done | blocked
priority: <1-5>              # 1 = highest
created: YYYY-MM-DD
phase: "M<N>: <Phase Name>"

dependencies:                # Task IDs that must be done first
  - <TASK-ID>

observable: |
  Concrete, runnable verification. Use actual commands:
  - `go build ./cmd/curlew && ./curlew <args>` with expected output
  - `go test ./internal/<package>/...` with expected pass count
  - Real CLI invocations with sample files

behaviors:
  - "Given <condition>, when <action>, then <result>"
  - "Each behavior maps to one or more test cases"
  - "4-10 behaviors per task"

scope: |
  What needs to be built. Reference specific Go packages, types,
  interfaces. 2-5 sentences.

definition_of_done:
  - "All behavior tests pass"
  - "Observable output works as specified"
  - "Test coverage >= 80%"
  - "No build warnings or lint errors"
  - "Help text updated (if user-facing)"
  - "Smoke test updated (if new capability)"

complexity: low              # low | medium | high
estimated_effort: "2-3 hours"  # low: 1-3h, medium: 3-6h, high: 6-12h
track: go-cli                # go-cli | backend | web (required for M4+, omit for M1–M3)
```

M1–M3 tasks omit `track:`; they are implicitly `go-cli`. M4+ tasks MUST set it explicitly so the Plan agent can pick the right observable template (see Track Notes in `milestone-mapping.md`).

## Behavior Guidelines

### Good Behaviors (Specific, Testable)
- "Given a collection file with one GET request, when executed, then the response status code is printed"
- "Given an invalid YAML file, when parsed, then an error with line number is returned"
- "Given --version flag, when the binary runs, then it prints the version string and exits 0"

### Bad Behaviors (Vague, Untestable)
- "The parser works correctly" — what does "correctly" mean?
- "Errors are handled" — which errors? how?
- "Output is formatted" — what format? what content?

### Sources for Deriving Behaviors
1. **Interface methods** — each method signature suggests behaviors
2. **Type definitions** — each field suggests validation and edge cases
3. **Test scenarios in spec** — explicit examples to implement
4. **Error codes** — each error type is a behavior
5. **Edge cases** — empty input, max size, special characters, unicode

### Observable Examples by Task Type

**CLI task:** `./curlew --flag` produces specific output
**Parser task:** `go test ./internal/parser/...` passes, binary parses sample file
**HTTP task:** binary sends request to test server, prints response
**Integration task:** end-to-end scenario with fixture files in `testdata/`

### Complexity Guidelines

| Complexity | Slice Type | Behaviors | Effort |
|-----------|-----------|-----------|--------|
| low | Single function, one package | 4-6 | 1-3 hours |
| medium | Multiple functions, cross-package | 6-8 | 3-6 hours |
| high | New subsystem, integration | 8-10 | 6-12 hours |

## Common Pitfalls

### Behaviors that are not testable
- "The feature is robust" — what does robust mean operationally?
- "Handles errors gracefully" — which errors? what does graceful look like?
- "Performs well" — what latency target? measured how?

### Observables that are not runnable
- "All tests pass" — not an observable, that's the quality gate
- "The function works" — not a command
- "User can do X" — name the exact keystrokes or HTTP call

### Slices that are not vertical
- "Add type X to package Y" — that's a layer, not a slice
- "Refactor package Z" — that's maintenance, not a capability
- "Wire up logging" — that's plumbing; bundle it into a feature slice

### Behaviors must use Given/When/Then

Every entry in the `behaviors:` list must parse as:

    Given <precondition>, when <action>, then <observable outcome>

If you cannot state a precondition, promote the input to the precondition.
If you cannot state an outcome a test harness can observe, it is not a behavior.
