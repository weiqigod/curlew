# Improvement Report: M10-002

**Task:** agent skill shipment: SKILL.md content, executable-spec playbook test, docs, CHANGELOG, IMPROVEMENT.md W5 status
**Date:** 2026-04-25
**Review:** management/reviews/M10-002-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `TestSkillClaude_SkillFileSnapshot` only replaced `"curlew 0.1.0-dev"` (version comment line), missing the bold form `**0.1.0-dev**` in the Notes section. Golden file line 127 stored the hardcoded version, breaking on every version bump. | Added a second `bytes.ReplaceAll` for `"**"+version+"**"` → `"**v0.0.0-test**"`. Regenerated the golden with `CURLEW_UPDATE_SNAPSHOTS=1`. Golden now has `v0.0.0-test` on both line 6 and line 127. | ✓ tests pass |
| 2 | Medium | `ndjsonContainsRunEnd`, `ndjsonContainsRunError`, and `ndjsonContainsRequestEndNetworkError` silently returned `false` when `os.ReadFile` failed, masking missing-file failures as "event not found" failures. | Added `t.Logf("...: events file missing or unreadable: %v", err)` before each `return false` in all three helpers. | ✓ tests pass |
| 3 | Medium | MANUAL.md §4.9 line 1506 stated "Five files are created in addition to the bare scaffold" — factually incorrect (one new file plus two modifications to existing files). | Replaced with: "The following are created or modified in addition to the standard scaffold:" with `(new file)` / `(modified)` annotations on each bullet. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Coverage (cmd/curlew) | 81.4% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| fbfea98 | fix(cli,docs): resolve M10-002 review findings #1, #2, #3 | #1, #2, #3 |

## Summary

3/3 findings resolved. 0 deferred.
