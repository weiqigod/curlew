# Verification Report: M5-013

**Task:** go-cli: offline JWT verification and grace-period state machine
**Verified by:** AI
**Date:** 2026-04-19
**Branch:** feature/M5-013-offline-license
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go build ./cmd/apitest` | PASS | Clean build, no warnings |
| `go test ./...` | PASS | All packages pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All smoke tests pass including M5-013 offline license section |
| Coverage `internal/license` | 87.8% | Meets >= 80% threshold |
| Coverage `internal/license/jwks` | 92.0% | Meets >= 80% threshold |
| Coverage `cmd/apitest` | 83.3% | Meets >= 80% threshold |
| Coverage total | 87.1% | Meets >= 80% threshold |

## Observable Output

```
--- Observable 1: OFFLINE VALID ---
Validating license offline...
Key source: embedded JWKS (kid=apitest-2025-01)
Tier: enterprise
State: VALID
Exit: 0

--- Observable 2: GRACE PERIOD day 25 ---
Validating license offline...
Key source: embedded JWKS (kid=apitest-2025-01)
Tier: enterprise
State: GRACE_PERIOD
Warning: 5 days until grace period expires
Exit: 0
```

Expected: stdout contains "State: VALID" and exit 0 for offline valid; stderr contains "5 days until grace period expires" for day 25.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | APITEST_OFFLINE=1 + cached JWT → embedded JWKS verification, State: VALID, exit 0 | `TestLicenseValidate_Offline_Valid`, `TestValidator_OfflineValid` | PASS |
| 2 | kid not in embedded → check cached JWKS | `TestKeyResolver_CachedFallback`, `TestValidator_KidInCachedJWKS` | PASS |
| 3 | kid in neither + offline → exit 6 "key_not_found" | `TestLicenseValidate_UnknownKid`, `TestKeyResolver_OfflineFails` | PASS |
| 4 | >24h elapsed → GRACE_PERIOD, tier features available | `TestValidator_OfflineGracePeriod`, `TestEvaluate` | PASS |
| 5 | GRACE_PERIOD days 21-29 → warning to stderr | `TestLicenseValidate_Offline_GracePeriod_Day25` | PASS |
| 6 | 30 days elapsed → GRACE_EXPIRED, tier features gated exit 9 | `TestRunCmd_GraceExpired_ExitsNine`, `TestExecCmd_GraceExpired_ExitsNine`, `TestLicenseValidate_Offline_GraceExpired_Day31` | PASS |
| 7 | Reconnect success → VALID, timestamps updated | `TestStore_MarkValidated` | PASS |
| 8 | Tampered signature → exit 6 "signature_invalid" | `TestLicenseValidate_TamperedToken`, `TestVerifyRS256/tampered_signature_fails` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 8 behaviors tested; `go test ./...` all PASS | PASS |
| 2 | Observable output works as specified | Offline VALID and GRACE_PERIOD day 25 output match spec exactly | PASS |
| 3 | Test coverage >= 80% | `internal/license`: 87.8%, `internal/license/jwks`: 92.0%, `cmd/apitest`: 83.3%, total: 87.1% | PASS |
| 4 | No build warnings or lint errors | `go build` clean, `golangci-lint run` 0 issues | PASS |
| 5 | Help text for `apitest license --validate` documents offline mode and grace-period states | `printLicenseHelp()` present in `cmd/apitest/license.go`, visible in `--help` output | PASS |
| 6 | Smoke test covers offline validation, grace-period warning, and grace-expired paths | `smoke/run.sh` M5-013 section: PASS offline VALID, grace warning at day 25, grace expired at day 31 | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling (fmt.Errorf %w) | PASS — all error wrapping uses %w in production code |
| Sentinel errors | PASS — ErrKeyNotFound, ErrSignatureInvalid, ErrTokenMalformed, ErrNoLicense, ErrOffline exported |
| Naming conventions | PASS — no stuttering, doc comments on all exports |
| Code organization | PASS — package boundaries respected, single responsibility |
| Test quality | PASS — table-driven tests, t.Run, white-box signature mutation tests |
| alg=none rejection | PASS — verified by TestVerifyRS256/alg=none_rejected |
| GRACE_EXPIRED exit 9 gating | PASS — checkGraceExpired() wired into runCmd and execCmd |

Branch A: Review PASS (iteration 3) trusted, spot-check clean — `%w` error wrapping confirmed in resolver.go, doc comments confirmed on all exports in validator.go, table-driven tests confirmed in state_test.go.

## Commits

| Hash | Message |
|------|---------|
| 8ac61e9 | docs(review): add passing review for M5-013 |
| 1839261 | docs(review): add iteration 2 improvement report for M5-013 |
| cf9d4e5 | fix(license): gate premium commands with exit 9 on GRACE_EXPIRED |
| 0f3939b | docs(review): add iteration 2 review with findings for M5-013 |
| a4fce87 | docs(review): add improvement report for M5-013 |
| 72d3c04 | fix(smoke): use APITEST_LAST_VALIDATION_OVERRIDE=1h for offline VALID test |
| 2be374a | fix(license): make Token.signed and Token.signature unexported fields |
| 079b985 | fix(license): use %w instead of %v in all error wrapping calls |
| 512814a | docs(review): add review with findings for M5-013 |
| 691ac9d | chore(task): mark M5-013 as review |
| d0033ff | refactor(license): fix lint issues and format code |
| ff72323 | feat(smoke): add offline license validation smoke tests (Step 8) |
| f4f0c12 | feat(cli): implement apitest license --validate subcommand (Step 7) |
| d442b9d | test(cli): add failing tests for license --validate subcommand |
| 170c4ac | feat(license): implement Validator orchestration (Step 6) |
| b967a94 | test(license): add failing tests for validator orchestration |
| f4f5c6e | feat(license): implement license store with env override (Step 5) |
| 7bdd09b | test(license): add failing tests for license store and duration parsing |
| 2528f33 | feat(license): implement grace-period state machine (Step 4) |
| f3ca52f | test(license): add failing tests for grace-period state machine |
| bbfa6c0 | feat(license): implement key lookup chain embedded->cached->online->fail (Step 3) |
| 21e514d | test(license): add failing tests for key lookup chain |
| 4c92aab | feat(license): implement JWT parsing and RS256 verification (Step 2) |
| d8ff9f1 | test(license): add failing tests for JWT parsing and RS256 verification |
| d53f716 | feat(license): implement JWKS parsing and RSA key lookup (Step 1) |
| b8b3e01 | test(license): add failing tests for JWKS parsing and RSA key lookup |
| 8b0e87e | chore(task): mark M5-013 as in_progress |
| 83d17b4 | chore(task): mark M5-013 as planned |
| 579e2d0 | docs(plan): add implementation plan for M5-013 |

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/license.go` | added — license subcommand, checkGraceExpired |
| `cmd/apitest/license_test.go` | added — integration tests for license --validate |
| `cmd/apitest/main.go` | modified — grace expired gating in runCmd/execCmd |
| `internal/license/jwks/jwks.go` | added — JWKS parsing and RSA key lookup |
| `internal/license/jwks/jwks_test.go` | added |
| `internal/license/jwt.go` | added — JWT parsing and RS256 verification |
| `internal/license/jwt_fixtures_test.go` | added |
| `internal/license/jwt_internal_test.go` | added |
| `internal/license/jwt_test.go` | added |
| `internal/license/keys/embed.go` | added — go:embed for JWKS |
| `internal/license/keys/jwks.json` | added — embedded test JWKS |
| `internal/license/keys/testdata/` | added — generate.go, test fixtures |
| `internal/license/resolver.go` | added — key lookup chain (embedded->cached->online->fail) |
| `internal/license/resolver_test.go` | added |
| `internal/license/state.go` | added — grace-period state machine |
| `internal/license/state_test.go` | added |
| `internal/license/store.go` | added — license JSON store with env override |
| `internal/license/store_test.go` | added |
| `internal/license/validator.go` | added — Validator orchestration |
| `internal/license/validator_test.go` | added |
| `smoke/run.sh` | modified — M5-013 offline license smoke tests |
| `management/` | added — plan, review, improvement, task files |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. All 8 spec behaviors implemented and tested. Coverage exceeds 80% across all packages. Lint clean. Smoke tests pass including all offline license paths. Three review iterations completed with all findings resolved.
