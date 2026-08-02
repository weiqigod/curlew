# Verification Report: M2-010

**Task:** Per-request auth profile reference
**Verified by:** AI
**Date:** 2026-03-29
**Branch:** feature/M2-010-per-request-auth-profile
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/apitest` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | 15 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All scenarios pass (exit 0) |
| Coverage (total) | 91.6% | Meets >= 80% threshold |
| Coverage (`internal/parser`) | 91.0% | |
| Coverage (`internal/runner`) | 91.3% | |
| `resolveAuthProfile` | 100.0% | |
| `hasHeaderCaseInsensitive` | 100.0% | |

## Observable Output

```
Collection: per-request-auth-test
[ERROR] request "Authenticated Request": auth profile not found: "admin_token"; no auth profiles configured (add auth_profiles: to apitest.yaml)
```

Expected: `auth:` field parsed; clear error when no auth profiles configured.
Result: MATCH — `auth:` field is parsed and runner produces a clear, actionable error.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Given a request with `auth: admin_token`, when executed, then Authorization header is injected | `TestRun_PerRequestAuth/auth_injects_bearer_header` | PASS |
| 2 | Given two requests referencing different auth profiles, each uses its own credentials | `TestRun_PerRequestAuth/two_requests_use_different_profiles` | PASS |
| 3 | Given a request with `auth:` referencing a non-existent profile, a clear error lists available profiles | `TestRun_PerRequestAuth/nonexistent_profile_returns_error_listing_available`, `TestResolveAuthProfile/nonexistent_profile_lists_available_names` | PASS |
| 4 | Given a request with both `auth:` and explicit Authorization header, the explicit header takes precedence | `TestRun_PerRequestAuth/explicit_Authorization_header_takes_precedence`, `TestRun_PerRequestAuth/case-insensitive_explicit_header_takes_precedence` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./...` — all 15 packages pass | PASS |
| 2 | Observable output works as specified | `apitest run` with `auth:` field produces clear error | PASS |
| 3 | Test coverage >= 80% | 91.6% total | PASS |
| 4 | No build warnings or lint errors | Clean build + `golangci-lint run` 0 issues | PASS |
| 5 | Help text updated | N/A — no new CLI flags | PASS |
| 6 | Smoke test updated | Per-request auth scenario added to `smoke/run.sh` | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — all errors wrapped with `%w`; `ErrAuthProfileNotFound` sentinel used |
| Naming conventions | PASS — no stuttering; doc comments on all exported/unexported functions |
| Code organization | PASS — `internal/` boundaries respected; auth injection scoped to runner package |
| Test quality | PASS — table-driven throughout; all 4 behaviors covered; edge cases explicit |

Branch A: Review PASS trusted (verdict PASS in `management/reviews/M2-010-review.md`), spot-check clean.

## Commits

| Hash | Message |
|------|---------|
| e352b5a | docs(plan): add implementation plan for M2-010 |
| d187307 | chore(task): mark M2-010 as planned |
| 29e9723 | chore(task): mark M2-010 as in_progress |
| c6a8409 | test(parser): add failing tests for auth field on RequestItem |
| 951d4d2 | feat(parser): add Auth field to RequestItem |
| 85e332c | refactor(parser): fix gofumpt alignment for Auth field |
| e0c5855 | test(parser): add failing tests for auth propagation in external refs |
| 18af046 | feat(parser): propagate auth field from reference site in external refs |
| 821306d | test(runner): add failing tests for resolveAuthProfile and hasHeaderCaseInsensitive |
| 70c2d43 | feat(runner): add resolveAuthProfile and hasHeaderCaseInsensitive |
| 16444f5 | test(runner): add failing tests for per-request auth injection |
| 01d0868 | feat(runner): inject auth profile header in executePhase |
| e5c3de8 | refactor(runner): fix gofumpt formatting in runner_test.go |
| 070f1db | chore(task): mark M2-010 as review |
| 59ae566 | docs(review): add review with findings for M2-010 |
| 70ee2be | fix(smoke,docs): add smoke test and CHANGELOG entry for M2-010 |
| 4336131 | docs(review): add improvement report for M2-010 |
| db2628f | docs(review): add passing review for M2-010 |

## Files Changed

| File | Action |
|------|--------|
| `internal/parser/collection.go` | modified — `Auth string` field added to `RequestItem` |
| `internal/parser/external.go` | modified — auth propagation from reference site |
| `internal/parser/parser_test.go` | modified — `TestParseFile_AuthField` |
| `internal/parser/external_test.go` | modified — `TestParseFile_ExternalRef_Auth*` |
| `internal/parser/testdata/with_auth.yaml` | created |
| `internal/parser/testdata/ref_with_auth.yaml` | created |
| `internal/parser/testdata/ref_without_auth.yaml` | created |
| `internal/runner/runner.go` | modified — `ErrAuthProfileNotFound`, `resolveAuthProfile`, `hasHeaderCaseInsensitive`, injection in `executePhase` |
| `internal/runner/runner_test.go` | modified — `TestResolveAuthProfile`, `TestHasHeaderCaseInsensitive`, `TestRun_PerRequestAuth` |
| `smoke/run.sh` | modified — per-request auth scenario added |
| `CHANGELOG.md` | modified — M2-010 entry added |

## Issues Found

None. Note: `management/tasks/M2-010.yaml` had stale `status: backlog` (not updated by the `chore(task): mark M2-010 as review` commit). Fixed as part of this verification step.

## Recommendation

PASS — ready for PR and merge.
