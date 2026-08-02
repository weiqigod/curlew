# Improvement Report: M1-004

**Task:** Assert on status code
**Date:** 2026-03-10
**Review:** management/reviews/M1-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `UnmarshalYAML` error paths untested (invalid scalar, invalid sequence, unsupported node type) | Added 3 test fixtures and 3 test cases covering all error branches. `UnmarshalYAML` now at 100% coverage. | ✓ tests pass |
| 2 | Low | Misleading `echo "Exit code: $?"` in smoke test under `set -e` | Replaced with `&& ... || ...` pattern consistent with rest of script | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 94.2% (up from 92.6%) |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| d725467 | test(parser): add tests for UnmarshalYAML error paths | #1 |
| 9089bc7 | fix(cli): fix misleading exit code echo in smoke test | #2 |

## Summary

2/2 findings resolved. 0 deferred.
