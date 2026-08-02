# Verification Report: M12-004

**Task:** $sha256 and $md5 dynamic functions
**Verified by:** AI
**Date:** 2026-04-28
**Branch:** feature/M12-004-sha256-md5-dynamic-functions
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (`internal/variable`) | 96.2% | Meets >= 80% threshold |

## Observable Output

```
=== RUN   TestRegistry_Sha256
--- PASS: TestRegistry_Sha256 (0.00s)
    --- PASS: TestRegistry_Sha256/hello (0.00s)
    --- PASS: TestRegistry_Sha256/empty_string (0.00s)
    --- PASS: TestRegistry_Sha256/utf-8_multibyte (0.00s)
    --- PASS: TestRegistry_Sha256/abc (0.00s)

=== RUN   TestRegistry_Md5
--- PASS: TestRegistry_Md5 (0.00s)
    --- PASS: TestRegistry_Md5/hello (0.00s)
    --- PASS: TestRegistry_Md5/empty_string (0.00s)
    --- PASS: TestRegistry_Md5/utf-8_multibyte (0.00s)
    --- PASS: TestRegistry_Md5/abc (0.00s)

go test ./...  → PASS (all packages)

curlew run collections/sample.yaml --dry-run --format json
→ passed with X-Body-Hash header rendered as hex SHA-256 of resolved body
```

Expected: PASS for sha256/md5 tests; dry-run renders header value as hex SHA-256.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `$sha256(s)` returns lowercase 64-char hex of SHA-256 | `TestRegistry_Sha256/hello`, `TestRegistry_Sha256/abc` (len+regex checks) | PASS |
| 2 | `$md5(s)` returns lowercase 32-char hex of MD5 | `TestRegistry_Md5/hello`, `TestRegistry_Md5/abc` (len+regex checks) | PASS |
| 3 | `$sha256('')` returns SHA-256 of empty string | `TestRegistry_Sha256/empty_string` | PASS |
| 4 | UTF-8 multibyte `héllo` hashed over UTF-8 byte sequence | `TestRegistry_Sha256/utf-8_multibyte` (golden vector pinned) | PASS |
| 5 | Arity errors for zero or two args (both functions) | `TestRegistry_HashFns_arity_errors` (4 subtests) | PASS |
| 6 | Cross-request determinism (pure function, per-request cache) | `TestRegistry_Sha256_caches_per_request` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test -v -run TestRegistry_Sha256\|TestRegistry_Md5\|...` — all PASS | PASS |
| 2 | `go test ./...` passes | CI gate: all packages pass | PASS |
| 3 | `go test -cover ./internal/variable/...` >= 80% | 96.2% | PASS |
| 4 | `golangci-lint run` passes with 0 issues | CI gate: golangci-lint PASS | PASS |
| 5 | `./smoke/run.sh` passes | CI gate: smoke PASS | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | `docs/MANUAL.md` §3.7 updated with `$sha256` and `$md5` rows + MD5 caveat | Two rows added to argument-bearing subtable; MD5 deprecation paragraph added; parenthetical updated to reference M12-005 onward | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — no new error paths; hashing is infallible; arity errors go through pre-existing `arityError` chain |
| Naming conventions | PASS — registry keys `"sha256"` and `"md5"` match existing lowercase convention; no new exports |
| Code organization | PASS — two registrations appended in `register()`, reusing `oneArg` from M12-002 |
| Doc comments | PASS — both registrations have inline comments; no new exported symbols introduced |
| Test quality | PASS — table-driven golden vectors, UTF-8 edge case, empty string, arity errors for zero/two args, per-request cache with call-counter wrapper |

Branch A: "Review PASS trusted, spot-check clean" — review verdict was PASS with no findings. Spot-check confirmed: error handling uses pre-existing `arityError`/`Evaluate` chain with `%w` wrapping, no new exports (no doc comment needed), arity test verifies what it claims.

## Commits

| Hash | Message |
|------|---------|
| `a3e0ba4` | docs(review): add passing review for M12-004 |
| `c97046d` | chore(task): mark M12-004 as review |
| `ac72612` | docs(manual): add $sha256 and $md5 rows to §3.7 with MD5 deprecation caveat |
| `f0e9e21` | feat(variable): $sha256 and $md5 dynamic functions |
| `8eff9f1` | test(variable): add failing tests for $sha256 and $md5 dynamic functions |
| `7d45986` | chore(task): mark M12-004 as in_progress |
| `4669b9c` | chore(task): mark M12-004 as planned |
| `76199aa` | docs(plan): add implementation plan for M12-004 |

TDD pattern visible: `test(variable)` → `feat(variable)` → `docs(manual)`.

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/variable/dynamic.go` | modified | Added `crypto/md5` and `crypto/sha256` imports; registered `sha256` and `md5` via `oneArg` |
| `internal/variable/dynamic_test.go` | modified | Added 5 new test functions; bumped `Available` count assertion 19 → 21 |
| `docs/MANUAL.md` | modified | Added two rows to argument-bearing subtable; MD5 deprecation caveat paragraph; updated trailing parenthetical |

## Issues Found
None

## Recommendation
PASS — ready for PR and merge
