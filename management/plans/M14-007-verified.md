# Verification Report: M14-007

**Task:** CLI: JWKS fetch + cache for offline License JWT verification
**Verified by:** AI
**Date:** 2026-05-05
**Branch:** feature/M14-007-jwks-fetch-cache
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass, 0 failures |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | M14-007 smoke block: 4/4 checks pass |
| Coverage (`internal/license`) | 88.3% | Above 80% threshold |
| Coverage (`internal/license/jwks`) | 89.3% | Above 80% threshold |
| Coverage (`internal/license/export`) | 80.8% | At 80% threshold |
| Coverage (`internal/backend`) | 84.6% | Above 80% threshold |
| Coverage (`cmd/apitest`) | 81.5% | Above 80% threshold |
| Coverage (total) | 87.3% | Above 80% threshold |

## Observable Output

The smoke test block `=== License Validate — JWKS cache (M14-007) ===` in `smoke/run.sh` exercises the full observable:

```
=== License Validate — JWKS cache (M14-007) ===
PASS: license --validate exits 0 with jwks=embedded (primary kid)
stub-backend started (pid 43960)
PASS: license --validate exits 0 with jwks=online (secondary kid via fetcher)
PASS: jwks_cache.json written after online fetch
stub-backend stopped
PASS: license --validate exits 0 with jwks=cached (offline after warm-up)
```

Expected: exit 0 with `jwks=embedded`, then `jwks=online` (with backend), then `jwks=cached` (backend stopped).
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | Empty cache + reachable backend → fetch /api/v1/.well-known/jwks.json, persist, verify | `TestValidator_OnlineFetcherWiring`, `TestJWKSClient_FetchSuccess`, smoke `jwks=online` | PASS |
| 2 | jwks_cache.json contains required kid → no network call | `TestJWKSClient_CacheWarmShortCircuits`, smoke `jwks=cached` (after stub stopped) | PASS |
| 3 | kid absent from all sources + backend unreachable → ErrKeyNotFound, exit 6 | `TestLicenseValidate_BackendUnreachable_UnknownKid` | PASS |
| 4 | alg not in {ES256} allowlist → ErrInvalidAlgorithm without key lookup | `TestVerifyJWT/alg=RS256_rejected_by_allowlist`, `alg=ES384_rejected_by_allowlist`, `alg=none_rejected`, `alg=HS256_rejected` | PASS |
| 5 | typ != "license+jwt" → ErrInvalidType | `TestVerifyJWT/typ=JWT_rejected_(License_JWT_requires_license+jwt)` | PASS |
| 6 | VerifyRS256 renamed to VerifyJWT(token, jwks), old name removed | No `VerifyRS256` in codebase; `TestVerifyJWT` is the canonical test | PASS |
| 7 | Cache-Control max-age=3600 honored as soft TTL; stale cache still consulted when backend unreachable | `TestJWKSClient_CacheStaleRefetches`, `TestJWKSClient_DefaultMaxAgeWhenMissing`, `TestParseMaxAge` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | `go test ./internal/license/... passes with >=12 tests` | 149 test function runs across 3 packages (license, jwks, export) | PASS |
| 2 | Live binary verifies JWT against cache after backend stopped | Smoke block: `PASS: license --validate exits 0 with jwks=cached (offline after warm-up)` | PASS |
| 3 | VerifyRS256 export removed; no compile aliases remain | `grep -r VerifyRS256 internal/ cmd/` returns nothing | PASS |
| 4 | Help text for `license --validate` mentions cache path | `printLicenseHelpTo` includes JWKS lookup order line with `~/.config/apitesttool/jwks_cache.json` | PASS |
| 5 | docs/SPECIFICATION.md:7990 + alg-agnostic clause cited in jwt.go header | SPEC reference comment present in `jwt.go` | PASS |
| 6 | Smoke test confirms `license --validate` exits 0 against fixture JWT after online warm-up | Smoke block passes | PASS |

## Code Review

Review PASS trusted (3rd iteration, `management/reviews/M14-007-review.md`). Spot-check performed:

| Check | Finding | Status |
|-------|---------|--------|
| Random error handling site (`jwt.go:150`) | `fmt.Errorf("%w: kid=%s", ErrKeyNotFound, kid)` — correct `%w` wrapping | PASS |
| Random exported symbol (`JWKSClient.FetchJWKS` in `jwks_client.go:54-63`) | Full doc comment present describing offline behaviour and TTL logic | PASS |
| Random test (`TestVerifyJWT` in `jwt_internal_test.go`) | Table-driven, 8 cases; verifies actual sentinel error types not just `err != nil` | PASS |

## Commits

| Hash | Message |
|------|---------|
| `bf8117df` | docs(plan): add implementation plan for M14-007 |
| `d9bce70c` | chore(task): mark M14-007 as planned |
| `e200b121` | chore(task): mark M14-007 as in_progress |
| `92ae1c1e` | test(license): regenerate fixtures as ES256 (dev-es256-202605-a3f4d2/b8c1e7) |
| `67d5be7f` | test(license): migrate all test fixtures from RS256 to ES256 |
| `cc7db341` | feat(license): replace VerifyRS256 with algorithm-agnostic VerifyJWT + ES256 dispatcher |
| `231f9f19` | feat(backend): JWKSClient HTTP fetcher with Cache-Control soft-TTL + disk cache |
| `c6acc6c4` | feat(cli): wire JWKSClient into license --validate + jwks=<source> stdout suffix |
| `a1cd7fec` | feat(cli): update stub_server to mint ES256 + serve JWKS endpoint (M14-007) |
| `52c71d20` | feat(cli): add JWKS cache smoke test + sample-license.json for M14-007 |
| `7637a5da` | fix(cli): stub_server serves combined JWKS (primary + secondary kid) for M14-007 |
| `714fb63e` | refactor(license): update stale kid references to dev-es256-202605-a3f4d2 |
| `d0e465ca` | chore(task): mark M14-007 as review |
| `aed2dda3` | fix(license): resolve all four M14-007 review findings |
| `5025c6c8` | fix(license): map ErrOffline → exit 6 for backend-unreachable unknown-kid path |
| `872d72f5` | docs(review): add passing review for M14-007 |

TDD pattern visible: `test(license)` commits precede `feat(license)` commits.

## Files Changed

| File | Action |
|------|--------|
| `internal/license/jwt.go` | Modified — `VerifyRS256` → `VerifyJWT`; ES256 dispatcher; sentinel errors |
| `internal/license/jwt_internal_test.go` | Modified — `TestVerifyJWT` with 8 cases |
| `internal/license/jwt_fixtures_test.go` | Modified — ECDSA helpers |
| `internal/license/jwt_test.go` | Modified — ECDSA minters |
| `internal/license/resolver.go` | Modified — returns `*jwks.Set` instead of `*rsa.PublicKey` |
| `internal/license/resolver_test.go` | Modified — new kid constants |
| `internal/license/validator.go` | Modified — `NewValidatorWithFetcher` added |
| `internal/license/validator_test.go` | Modified — `TestValidator_OnlineFetcherWiring` added |
| `internal/license/jwks/jwks.go` | Modified — `LookupES256` added |
| `internal/license/jwks/jwks_test.go` | Modified — `TestLookupES256*` added |
| `internal/license/keys/jwks.json` | Regenerated — ES256 key, kid `dev-es256-202605-a3f4d2` |
| `internal/license/keys/testdata/generate.go` | Rewritten — ECDSA P-256 |
| `internal/license/keys/testdata/private.pem` | Regenerated — EC private key |
| `internal/license/keys/testdata/private2.pem` | Regenerated — EC private key |
| `internal/license/keys/testdata/jwks_extra.json` | Regenerated — secondary EC kid |
| `internal/license/keys/testdata/license.json` | Regenerated — ES256 JWT, far-future exp |
| `internal/backend/jwks_client.go` | Created — `JWKSClient` with Cache-Control soft-TTL |
| `internal/backend/jwks_client_test.go` | Created — 8 unit tests |
| `cmd/apitest/license.go` | Modified — `NewValidatorWithFetcher` wiring; `jwks=<source>` stdout suffix; help text |
| `cmd/apitest/license_test.go` | Modified — `TestLicenseValidate_BackendUnreachable_UnknownKid` added |
| `smoke/run.sh` | Modified — M14-007 JWKS cache smoke block |
| `testdata/m14/sample-license.json` | Created — secondary-kid ES256 fixture |
| `testdata/m14/stub_server.go` | Modified — ES256 minting + JWKS endpoint |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
