# Code Review: M27-001

**Task:** The prose register - checkable claims that are sentences, not rows
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-001-prose-register
**Iteration:** 2 (re-review after `/improve`)

## Verdict: PASS

## Findings

None.

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Iteration 1 Finding: Verified Fixed

Iteration 1 raised one Medium finding: `CHANGELOG.md:219` stated the table register
lives at `internal/docs/table-execution-baseline.txt`, inconsistent with the two
correct references to `docs/table-execution-baseline.txt` three and five hundred
lines later in the same entry. Commit `76393ba` ("fix(docs): correct
table-execution-baseline.txt path in CHANGELOG") changed line 219 to
`docs/table-execution-baseline.txt`. Re-verified this iteration:

```
$ grep -n "table-execution-baseline.txt" CHANGELOG.md
219:  `docs/table-execution-baseline.txt` only ever tracked tables.
252:  Same shrink-only contract as `docs/table-execution-baseline.txt` and
727:  `docs/table-execution-baseline.txt`.
```

All three references now agree, and the path matches the file on disk
(`ls docs/table-execution-baseline.txt` succeeds). No other path or line-number
claim in the same CHANGELOG entry was found to be wrong (see re-audit below).

## Pre-audit Gate

`./scripts/ci-local.sh --go` run twice this iteration (once to confirm green,
once with full log capture): build, `go test`, `go test -race`, per-package
coverage, `golangci-lint`, `smoke/run.sh`, and the `testapi` dogfood/gaps/
redaction/openapi/crosscheck harnesses all passed. Terminal tail: `=== ci-local
PASS ===`. `internal/docs` coverage measured directly: 81.1% of statements,
matching the improvement report's figure.

## Re-audit of the Whole Change (not just the iteration-1 fix)

This iteration re-read every changed file in full (`internal/docs/prose.go`,
`internal/docs/proseclaims.go`, `internal/docs/prose_test.go`, the seven payoff
test files, `docs/prose-claim-baseline.txt`, the `CHANGELOG.md` entry, and the
management docs) rather than trusting the prior pass, and additionally verified
by direct action rather than by reading:

- **Observable commands from the task**, run directly:
  `go test ./internal/docs/ -run TestProse_inventory_is_complete -v` →
  `103 prose claims: 14 executed, 3 exempt (cap 12), 86 owed`.
  `go test ./internal/docs/ -run TestProse_register_cannot_grow -v` → all 7
  subtests pass (clean ×2, new-debt, stale-debt, both-at-once, exempt,
  unresolved-executor). Both match the task's stated observable and the
  improvement report's numbers.
- **All three shrink-only guard directions, mutated against the real files
  myself** (not the synthetic fixtures in the test — the actual
  `docs/prose-claim-baseline.txt`, `docs/CLI_SPECIFICATION.md`, and
  `docs/MANUAL.md`), each reverted and confirmed clean via `git status --short`
  afterward:
  - Deleted a real still-owed baseline line → correctly reported as 1 new-debt
    claim (`MANUAL.md:114`), test fails.
  - Appended a bogus, never-executed-looking baseline line → correctly
    reported as 1 stale-debt entry, test fails.
  - Appended 10 extra `prose-not-executable` markers to
    `CLI_SPECIFICATION.md` (3 real + 10 = 13, cap 12) → correctly fails with
    "13 prose-not-executable markers, cap is 12".
  - Appended a brand-new checkable sentence to `MANUAL.md`
    (`The mutation probe always writes a canary value to
    \`--mutation-probe-file\`.`) with no executor and no register line →
    correctly reported as 1 new-debt claim at the correct line number.
- **Regression guards**, run directly: `TestDocTables_everyTableIsExecutedOrDeclaredProse`
  still passes (`77 tables: 72 executed, 5 declared prose (cap 8), 0 owed`);
  `cmd/curlew`'s `TestProse_everyNamedCommandExists` /
  `TestProse_everyNamedFlagIsAccepted` /
  `TestProse_everyNamedEnvironmentVariableIsRead` /
  `TestProse_ignoreMarkersAreFewAndDeliberate` all pass with the ignore-marker
  count unchanged (`MANUAL.md:3205, CLI_SPECIFICATION.md:2063`) — confirming
  the new `prose-not-executable` markers do not collide with the existing
  `ignore-names` machinery or its 15%-suppression guard.
- **Marker/table adjacency risk called out in the plan**: grepped both
  documents for every `doc-check:` marker line. No `prose-not-executable`
  marker sits between a `table-not-executable` marker and its table, so the
  risk the plan flagged (a stray marker spending the wrong pending-exempt
  slot) does not occur in the actual documents.
- **Payoff-tranche `docs.Prose(...)` calls spot-checked against document
  text**: confirmed `"is the same hex value across"`,
  `"governs ANSI escape sequences"`,
  `"cannot put escape codes into a payload a consumer has to parse"`, and
  `"exits 0 when every test passed"` all appear verbatim at the claimed
  locations; confirmed `"correlate the JSONL entry with the same identifiers"`
  and `"fan out from any fragment back to the whole run"` are real but
  hard-wrapped across source lines (`MANUAL.md:2269-2270` and
  `MANUAL.md:2318-2319` respectively) — exactly the defect-8 archetype the
  task exists to catch, and exactly why the reflow stage is load-bearing.
  All seven payoff test files (`cmd/curlew/main_test.go`,
  `markdown_correlation_test.go`, `redaction_response_test.go`, `run_test.go`,
  `stream_discipline_matrix_test.go`, `ui_test.go`,
  `internal/variable/doc_functions_test.go`, `dynamic_test.go`) write each
  claim as its own direct `docs.Prose("DOC.md", "substring")` call — never a
  loop over a slice — consistent with the AST-walk constraint the plan's
  Deviations section documents.
- **`gofmt -l` and `go vet`** on every changed Go file: clean.
- **File-path claims in `CHANGELOG.md`** re-checked against the filesystem:
  `docs/table-execution-baseline.txt`, `testapi/harness/redaction-known-leaks.txt`,
  and `docs/prose-claim-baseline.txt` all exist at the stated paths.
- **`docs/prose-claim-baseline.txt` internal consistency**: header states
  "Started at 86 of 103"; counted 86 tab-separated data lines in the file
  directly (`grep -c $'\t'`), matching.
- **`management/backlog.yaml`** diff: `M27-001` status moved
  `backlog` → `review` with `branch: feature/M27-001-prose-register`, matching
  `management/tasks/M27-001.yaml`'s `status: review`.

No new issues were found in this broader re-audit; the fix from iteration 1
did not introduce any regression, and the rest of the change holds up under
independent, hands-on verification rather than re-reading the code alone.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `ProseClaims`/`Prose` wrap underlying errors with `%w`; `Prose` returns constructed errors (no match / ambiguous match) rather than guessing; `ExtractProse`/`AuditProse` are pure and error-free by design; no swallowed errors, no panics on expected failures. |
| Input Validation | PASS | `ProseInventory` propagates `ReadDoc`'s error for a missing document; `Prose` errors on 0 or >1 substring matches; `ProseClaims` silently (and correctly, per its doc comment) skips non-literal call arguments rather than mis-resolving them — verified this skip behaviour directly via `TestProseClaims`'s "no string args" case and the "leading period with no preceding text" edge case in `TestExtractProse`. |
| Naming | PASS | No stuttering (`docs.ProseRef`, `docs.ProseClaim`, `docs.ProseAudit`); vocabulary mirrors the existing `Table`/`TableRef`/`Claims` machinery; every exported type, func, method and const carries a doc comment; unexported helpers use short, scope-appropriate names. |
| Code Organization | PASS | Confined to `internal/docs`, consistent with the table-register machinery it mirrors; `ExtractProse`/`AuditProse` are pure (text/slices in, no filesystem), which is what makes the mutation-based guard tests — and my own hands-on mutation of the real files — possible; no reaching into other packages' internals. |
| Correctness | PASS | Independently re-verified, not re-read: observable commands match stated output; all three shrink-only guard directions confirmed against the real files (not just the synthetic test fixtures) and cleanly reverted; regression suites (table register, `cmd/curlew` name-checks) unaffected; marker/table adjacency risk checked directly and absent; payoff-tranche substrings confirmed present in the actual, sometimes-hard-wrapped document text. |
| Test Quality | PASS | `TestExtractProse` is table-driven with 25 cases covering shapes, referents, declines, marker scope, reflow, semicolon-splitting, abbreviation/initials guards, and an explicit "does not crash the guard" edge case with an inline comment explaining why. `TestProse_register_cannot_grow` proves both shrink-only directions by mutation against synthetic documents; the zero-claims direction is checked inline in `TestProse_inventory_is_complete`, mirroring the accepted precedent in `internal/docs/inventory_test.go`'s table-register test (same shape, same lack of a dedicated synthetic subtest for that one direction), and the plan's Deviations section documents this choice explicitly. |

## Test Coverage
- `internal/docs` package: **81.1%** of statements (measured this iteration via `go test -coverprofile` + `go tool cover -func`).
- Every function in `prose.go`: **100%**.
- `proseclaims.go`: `ProseClaims` 88.9%, `Prose` 92.9% — the uncovered slivers are defensive AST-walk branches (e.g. a parse-error path), nothing load-bearing to this task's contract.
- Not blocking: `AuditProse`'s "executor substring matches >1 claim" branch is exercised only indirectly (via `docs.Prose`'s own direct test of that case) rather than by a dedicated `AuditProse`-level subtest in `TestProse_register_cannot_grow`.

## Summary
The single iteration-1 finding — a self-inconsistent file path in the `CHANGELOG.md` entry — is confirmed fixed in commit `76393ba`, and all three of its former references to `table-execution-baseline.txt` now agree with the file's actual location. This iteration re-audited the change as a whole rather than only the diff of the fix: I independently re-ran the pre-audit gate, mutated the real baseline and real documents in all three shrink-only directions (new debt, stale debt, marker-cap) and reverted cleanly each time, confirmed the regression guards and the marker/table-adjacency risk the plan called out, and spot-checked payoff-tranche claims against the actual (sometimes hard-wrapped) document text, including the exact defect-8 archetype sentence this task exists to catch. No new issues surfaced. The extraction core, source-derived executor linkage, and shrink-only audit are well-built and independently verified to enforce what they claim.
