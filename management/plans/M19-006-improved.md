# Improvement Report: M19-006

**Task:** Expand templates/skills/claude/curlew/ to multi-file skill
**Date:** 2026-05-17
**Review:** management/reviews/M19-006-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `fs.WalkDir` return value discarded in `Walk` without explanation; truncation silent if ReadFile fails | Added extended godoc on `Walk` explaining why the discard is intentional (embedded.FS cannot fail at runtime; SkipAll handles early termination); added inline comment at the discard site | ✓ tests pass |
| 2 | Low | `strings.CutPrefix` boolean return discarded in `Walk`; if prefix absent, `rel` silently equals full FS path | Assigned the boolean; `!ok` branch falls back to full path `p` with an explanatory comment documenting the structural invariant | ✓ tests pass |
| 3 | Low | `TestSkillClaude_MultiFile_ExpressionsDocumentsCEL` missing assertion for the decision table heading required by behavior #3 | Added `"Decision table"` to the `[]string{...}` slice in the test | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`templates`) | 87.5% |
| Coverage (`internal/scaffold`) | 87.1% |
| Coverage (`cmd/curlew`) | 81.5% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 89b15e91 | fix(templates): document WalkDir discard, guard CutPrefix bool, add decision-table assertion | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
