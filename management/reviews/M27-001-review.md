# Code Review: M27-001

**Task:** The prose register - checkable claims that are sentences, not rows
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M27-001-prose-register

## Verdict: FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Medium | Documentation Accuracy | `CHANGELOG.md` | 219 | The new changelog entry states `internal/docs/table-execution-baseline.txt only ever tracked tables`, but the file actually lives at `docs/table-execution-baseline.txt` (confirmed: `find . -iname "*table-execution-baseline*"` returns only `./docs/table-execution-baseline.txt`). The same entry gets the path right twice more, at lines 252 and 727 (`docs/table-execution-baseline.txt`), so this is an internal inconsistency, not just an error relative to the filesystem. | Fix the path at line 219 to `docs/table-execution-baseline.txt` to match the correct references elsewhere in the same entry. |

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `ProseClaims` and `Prose` wrap errors correctly (`fmt.Errorf` with `%w` where an underlying error exists, plain `fmt.Errorf` for newly-constructed no-match/ambiguous errors); `ExtractProse`/`AuditProse` are pure and correctly error-free; no swallowed errors, no panics on expected failures. |
| Input Validation | PASS | `ProseInventory`/`ReadDoc` return a wrapped error for a missing document (`TestProseInventory_missingDocument`); `Prose` errors on zero or >1 matches rather than guessing; `ProseClaims` silently (and correctly, per the doc comment) skips call sites whose arguments aren't literals rather than mis-resolving them. |
| Naming | PASS | No stuttering (`docs.ProseRef`, not `docs.DocsProseRef`); naming mirrors the existing `Table`/`TableRef`/`Claims` vocabulary exactly (`Prose`/`ProseRef`/`ProseClaims`); every exported type, func, method and const has a doc comment; unexported helpers use short, scope-appropriate names. |
| Code Organization | PASS | Everything lives in `internal/docs`, consistent with the existing table-register machinery it mirrors; `AuditProse`/`ExtractProse` are pure (take slices/strings, not the filesystem), which is what makes the mutation-based guard tests possible; no reaching into other packages' internals; no unused imports/vars. |
| Correctness | PASS, with one doc-accuracy defect (see Findings) | Verified independently, not just by reading: `go test ./internal/docs/ -run TestProse_inventory_is_complete -v` and `-run TestProse_register_cannot_grow -v` both pass and match the task's `observable` output (`103 prose claims: 14 executed, 3 exempt (cap 12), 86 owed`). I mutated the real baseline file three ways and reverted each time (`git status` clean throughout): deleting a still-owed line correctly fails as new debt; appending a bogus already-resolved line correctly fails as stale debt; pushing the marker count in `CLI_SPECIFICATION.md` past the cap of 12 correctly fails. Spot-checked several of the 14 payoff-tranche `docs.Prose(...)` calls against the actual (sometimes hard-wrapped) document text — all resolve to the intended sentence. `go build ./...`, `gofmt -l`, and `go vet` are all clean on the changed packages; `go test -race ./internal/docs/...` and the regression guards (`TestDocTables_everyTableIsExecutedOrDeclaredProse`, `cmd/curlew`'s `TestProse_*`) all pass unaffected. |
| Test Quality | PASS | `TestExtractProse` is table-driven with 25+ cases covering shapes, referents, declines, marker scope, reflow, semicolon-splitting, abbreviation/initials guards, and an explicit empty-prefix edge case with a comment explaining why it can't crash the guard. `TestProse_register_cannot_grow` proves both shrink-only directions by mutation against synthetic documents, independent of the real files. The zero-claims direction (behavior 5 / DoD "guards verified by mutation in all three directions") is checked inline in `TestProse_inventory_is_complete` rather than via a separate synthetic-document subtest — this exactly mirrors the accepted precedent in `internal/docs/inventory_test.go`'s `TestDocTables_everyTableIsExecutedOrDeclaredProse` (same `t.Fatal` on `len(refs) == 0`/`len(claims) == 0` shape, also with no dedicated mutation subtest), and the plan's Deviations section calls this out explicitly, so it is not treated as a gap here. |

## Test Coverage
- `go test -coverprofile=... ./internal/docs/...`: **81.1%** of statements package-wide; every function in `prose.go` is at 100%, `proseclaims.go`'s `ProseClaims`/`Prose` are at 88.9%/92.9%.
- Missing coverage: the small uncovered slivers in `proseclaims.go` are defensive branches in the AST walk (e.g. a parse-error path); nothing load-bearing to this task's contract is unexercised. `AuditProse`'s "executor substring matches >1 claim" branch is only exercised indirectly (via the sibling `docs.Prose` reader's own direct test of that case, `TestProse_readerErrors/substring_matches_two_or_more_claims`) rather than by a dedicated `AuditProse`-level case in `TestProse_register_cannot_grow` — noted, not blocking.

## Summary
The extraction core, source-derived executor linkage, and shrink-only audit are well-built and independently verified here to actually enforce what they claim: I mutated the real baseline and the real document three separate ways and watched each guard fire correctly, then reverted cleanly. Design choices that look like shortcuts on first read (inline zero-claims check, truncation-not-emphasis-stripping in `Key()`) are each deliberate, documented in the plan's Deviations section, and match an established precedent already shipped in this codebase. The one finding is a factual slip in the `CHANGELOG.md` prose itself — `internal/docs/table-execution-baseline.txt` where it should read `docs/table-execution-baseline.txt` — self-inconsistent with two correct references three lines and 500 lines later in the same file. Per the All-or-Nothing rule this fails the review even though it is a single, mechanical, low-risk fix.
