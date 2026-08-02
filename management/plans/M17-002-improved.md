# Improvement Report: M17-002

**Task:** AWS Signature Version 4 signer (`type: aws-sigv4`)
**Date:** 2026-04-29
**Review:** management/reviews/M17-002-review.md
**Iteration:** 5

## Resolved Findings — Iteration 5

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `TestSigV4_MissingField_StructuredError` did not assert `se.Category`. Task behavior 5 requires "a structured **CategoryInput** error" — the production code sets `Category: apierrors.CategoryInput` correctly, but a regression changing the category to `CategoryConfig` would not be caught by any test. | Added `if se.Category != apierrors.CategoryInput { t.Errorf(...) }` inside the `t.Run` body. | ✓ tests pass |
| 2 | Medium | `session_token` sensitive-propagation path (lines 156–158 in `awssigv4.go`) had 0% test coverage. If the `sensitiveSourceFromSet["session_token"]` lookup were broken, no test would detect it. | Added `TestSigV4_SessionTokenSensitivePropagation` mirroring `TestSigV4_SecretKeySensitivePropagation`: scope has `aws_token` in `SensitiveSet`; params use `session_token: "{{aws_token}}"`. Assert both that `sensSet.Values()` contains the resolved token and that `x-amz-security-token` was injected. | ✓ tests pass |
| 3 | Low | `uriEncode`'s percent-encoding else branch (lines 332–334 in `awssigv4.go`) had 0% coverage. All existing tests used only RFC 3986 unreserved characters. | Added `TestUriEncode_PercentEncoding` calling `uriEncode` directly with space (`hello world` → `hello%20world`), slash (`path/value` → `path%2Fvalue`), all-unreserved (`a-z_0.9~`), and compound special chars (`a+b=c&d` → `a%2Bb%3Dc%26d`). | ✓ tests pass |
| 4 | Low | DoD requires byte-exact checks at each intermediate stage (canonical request, string-to-sign, Authorization). `canonical-request.txt` and `string-to-sign.txt` fixtures were shipped but never asserted. | Added canonical-request assertions (using a pre-Sign copy of the request to avoid Authorization header pollution) in `TestSigV4_GetVanilla` and `TestSigV4_PostXWWWFormURLEncoded`. Added string-to-sign assertion in `TestSigV4_GetVanilla` against `string-to-sign.txt` fixture. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All iteration-5 findings resolved.

## Previous Iterations (carried forward)

### Resolved Findings — Iteration 4

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `serializeBody` swallowed `json.Marshal` error via `_, _` discard. When marshal fails (channel field, cyclic ref), `out` is nil and the function returned nil — mapping to the well-known empty-body hash, producing a silently wrong Authorization header. | Changed `serializeBody` signature from `[]byte` to `([]byte, error)`. Propagated error from `Sign` via `fmt.Errorf("aws-sigv4: serializing body: %w", err)`. Added regression test `TestSigV4_SerializeBodyError` using a `chan int` body. | ✓ tests pass |
| 2 | Low | `testing.go` (non-test file) compiled `ResetBuiltinsForTest` into the production binary. Exported test-only symbol expanded the production API surface with no production caller. | Deleted `testing.go`. Added general-purpose `Unregister(name string)` to `builtins.go` (legitimate for both production deregistration and test cleanup). Updated `signing_integration_test.go` to call `signer.Unregister("builtin-test")` instead. | ✓ tests pass |

### Out of Scope (Deferred) — Iteration 4

| # | Severity | Finding | Rationale |
|---|----------|---------|-----------|
| 3 | Low | `TestSigV4_PostHeaderKeyCase` name says "Post" but tests a GET vector (`get-header-key-case`). Review recommended renaming to `TestSigV4_GetHeaderKeyCase`. | **Rejected — name is contractually required by the task observable.** `management/tasks/M17-002.yaml` observable explicitly mandates `go test -run 'TestSigV4_PostHeaderKeyCase'`. Renaming would violate the task contract. |

### Resolved Findings — Iteration 3

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `VarSources.Signer` doc comment in `runner.go` said "signer.NewBuiltinRegistry() is used" (empty registry) but the actual fallback on line 494 uses `signer.NewWithBuiltins()` (preloaded with all init-registered signers). Comment was left stale when M17-002 changed the fallback. | Updated comment to: "When nil, `signer.NewWithBuiltins()` is used — a registry preloaded with all signers registered at init time (e.g. aws-sigv4, oauth1)." Structural fix; no TDD required. | ✓ build passes |
| 2 | Medium | `resolveParams` swallowed interpolation errors silently by falling back to the raw literal string. When `scope.Interpolate("{{undefined_var}}")` returned an error, the code used `"{{undefined_var}}"` as the resolved value, passing required-field re-validation and producing a malformed Authorization header with no error returned to the caller. | Changed `resolveParams` signature to `(map[string]string, error)`. On interpolation failure, returns `fmt.Errorf("aws-sigv4: param %q interpolation failed: %w", k, err)`. Sign propagates the error. Added `TestSigV4_InterpolationError` test (TDD RED → GREEN). | ✓ tests pass |

### Resolved Findings — Iteration 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | Signed headers not visible in `RequestResult.RequestHeaders` when original request has nil headers. `ToHTTPRequest` copies the nil `Headers` reference; signer allocates a new map on the `httpexec.Request` copy; runner captures original (still nil). Smoke produced `null` for Authorization. | Sync `hr.Headers` back to `req.Headers` after exec in all five exec sites (runner.go sequential, 401-retry, data-driven sequential, data-driven parallel; parallel/executor.go). Tests: `TestRunner_Signing_HeadersPropagate_NilOriginal` and `TestExecuteWaves_SignerHeaders_PropagateWhenNilOriginal`. | ✓ tests pass, smoke passes |

### Resolved Findings — Iteration 1

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `headersToSkip` unconditionally excluded `x-amz-content-sha256` from SignedHeaders; post-x-www-form-urlencoded vector requires it included | Removed `headersToSkip` map; replaced with `hasBody bool` param to `buildCanonicalHeaderBlock` and `buildCanonicalRequest`. | ✓ tests pass |
| 2 | Critical | DoD requires byte-exact Authorization check for all three vectors; only get-vanilla was checked; fixture files never loaded | Added `loadFixture` helper; byte-exact checks for all three vectors. | ✓ tests pass |
| 3 | High | TestSigV4_MultiValueQueryParams only checked prefix, not actual canonical query string or signature | Added direct call to `canonicalQuery`; byte-exact Authorization check. | ✓ tests pass |
| 4 | High | MANUAL.md lines 3165/3167 still referenced `§6.5`/`§6.x` instead of `§6.8` | Updated both forward references in Part 10. | ✓ verified |
| 5 | Medium | Typo `deriveSigingKey` (missing `n`) | Renamed to `deriveSigningKey`. | ✓ tests pass |
| 6 | Medium | TestSigV4_BodyHashInjection did not verify body hash was included in canonical request | Added SignedHeaders assertions and pre-computed signature assertion. | ✓ tests pass |
| 7 | Low | Smoke test made live HTTP request to httpbin.org without --dry-run | Investigated: `--dry-run` does not prevent HTTP execution; kept httpbin.org pattern consistent with rest of smoke suite. | ✓ smoke passes |

## Quality Gate (Iteration 5)

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (internal/signer/awssigv4) | 96.6% |
| Coverage (internal/signer) | 92.1% |
| Coverage (internal/runner) | 84.9% |
| Coverage (internal/parallel) | 90.2% |

## Fix Commits (Iteration 5)

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| e93b17b | test(awssigv4): fix all four review findings from iteration 5 | #1, #2, #3, #4 |

## Previous Iteration Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 61c392e | fix(awssigv4): propagate json.Marshal error from serializeBody | iter4 #1 |
| 5581285 | fix(signer): remove test-only helper from production binary | iter4 #2 |
| 00cb468 | fix(runner): update VarSources.Signer doc comment to reflect NewWithBuiltins | iter3 #1 |
| d6f4e2c | fix(awssigv4): propagate interpolation errors from resolveParams | iter3 #2 |
| 7881003 | fix(runner,parallel): propagate signer-injected headers when original is nil | iter2 #1 |
| 4951b86 | fix(awssigv4): include x-amz-content-sha256 in signed headers for non-empty bodies | iter1 #1, #5 |
| 1bc01d5 | test(awssigv4): add byte-exact fixture tests for all three published test vectors | iter1 #2, #3, #6 |
| 1b0570d | docs(manual): update §6.5/§6.x forward references to §6.8 | iter1 #4 |
| 1192fdc | fix(smoke): revert aws-sigv4 smoke to httpbin.org without dry-run | iter1 #7 |

## Summary

16/17 findings resolved across 5 review iterations. 1 deferred (iter4 finding #3, contractually rejected — name mandated by task observable).

Iteration 5 addressed: `se.Category` assertion gap in missing-field test (#1, medium); session_token sensitive-propagation 0% coverage (#2, medium); uriEncode percent-encoding 0% coverage (#3, low); canonical-request and string-to-sign fixtures not asserted despite DoD requirement (#4, low). Coverage improved from 95.3% → 96.6%.
