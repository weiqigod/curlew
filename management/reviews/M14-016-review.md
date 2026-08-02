# Code Review: M14-016

**Task:** Backend: IGitHubAppKeyProvider + RS256 App-JWT signing
**Reviewer:** AI
**Date:** 2026-05-06
**Branch:** feature/M14-016-github-app-key-provider
**Iteration:** 4 (post-improve × 3)

## Verdict: PASS

## Findings

No findings.

## Previous Findings — Verification

All findings from iterations 1, 2, and 3 are confirmed resolved:

| Iteration | # | Severity | Fixed? | Evidence |
|-----------|---|----------|--------|---------|
| 1 | 1 | Critical | YES | `Program.cs` lines 224–240: `if (keyMode != "kms")` guard correctly registers `IKmsClient` for `ghAppMode=kms` only when not already registered. |
| 1 | 2 | High | YES | `builder.Logging.ClearProviders()` is called at line 253, followed by `AddDebug()` and `AddEventSourceLogger()`, then the `BearerTokenRedactor`-wrapped console provider. The default unredacted provider is removed before the wrapped provider is registered. |
| 1 | 3 | Medium | YES | `InternalGitHubAppEndpointsTests.cs` line 28: `doc.RootElement.GetProperty("iss").GetInt64().Should().Be(12345)` assertion present. |
| 1 | 4 | Medium | YES | `FakeKmsClient` implements `IDisposable`, disposes both `_localKey` and `_localRsaKey`. All callsites use `using var`. |
| 2 | 1 | High | YES | `builder.Logging.ClearProviders()` called at line 253 before the wrapped provider is registered. The `BearerTokenRedactor` now correctly replaces the default console sink. |
| 3 | 1 | Low | YES | The duplicate 18-line comment block in `Program.cs` explaining `ClearProviders()` rationale is removed. Only one cohesive comment block exists (lines 244–252). Verified with `grep` — no duplicate passages found. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `GitHubAppJwtSigningException` wraps all signing failures with a fixed message that excludes PEM paths and KMS resource names. Inner exception preserved for telemetry. `InvalidAppIdMismatchException` is excluded from the catch-when filter in both providers via `when (ex is not InvalidAppIdMismatchException)`. `GoogleKmsClient.AsymmetricSignRsaPkcs1Sha256Async` propagates `InvalidOperationException` on missing `signature` field — correctly surfaced without swallowing. |
| Input Validation | PASS | AppId mismatch checked before any I/O in both providers. `GitHubAppOptions` validator requires non-empty key path/id when `AppId != 0`. `ValidateOnStart()` ensures misconfiguration fails at startup, not at first request. |
| Naming | PASS | No stuttering. All exported types have doc comments. `AppJwtBuilder` is `internal static` — correct visibility for a shared helper not exported from the namespace. `IGitHubAppKeyProvider` follows interface naming convention. Record and exception types are clearly named. |
| Code Organization | PASS | `IGitHubAppKeyProvider` and `IKeyProvider` are in separate namespaces with no shared base class — type-system separation enforced by construction. Package boundaries respected. `InternalAccessFilter` is reused from `Licensing.Keys` namespace. `InternalAccessFilter` correctly bypasses auth in Testing environment, enabling the integration test without needing secret headers. |
| Correctness | PASS | `ClearProviders()` called before registering wrapped console provider — behavior #6 fulfilled. `RSA.Create()` wrapped in `using var` — key material disposed after each sign call. Payload claim order is deterministic (`SortedDictionary`). Base64url character set in `BearerPattern` covers all JWT token characters (A-Za-z0-9 plus `.`, `-`, `_`). `ghs_` pattern requires 36+ alphanumeric chars — boundary test verified: negative case has 27 chars after prefix (not matched), positive case has 36 (matched). `iss` claim stored as `long` in `SortedDictionary<string, object>` and serialized as a JSON number by `System.Text.Json` — tests parse with `GetInt64()` confirming numeric serialization. |
| Test Quality | PASS | All 7 task behaviors covered by at least one test. Both happy-path and error-path covered for File and KMS providers. `FakeKmsClient` properly disposes both EC and RSA crypto keys. `ghs_` boundary test covers both sides of the 36-char threshold. Integration test (`InternalGitHubAppEndpointsTests`) asserts on all six response fields including `iss=12345`. `BackendFactory` generates a real RSA PEM in a temp dir for integration tests. `FileGitHubAppKeyProviderTests` verifies RSA signature round-trip with direct `VerifyData` call (correctly avoiding `JwtSecurityTokenHandler` which expects string `iss`, whereas GitHub mandates numeric `iss`). |

## Test Coverage
- Coverage: Go gate is N/A (C# task). `./scripts/ci-local.sh --go` passes (Go tests unaffected).
- All 7 task behaviors covered at unit level (≥8 tests per DoD).
- Integration test covers the `/internal/github-app/jwt-self-test` happy path with full field assertion.
- All definition-of-done items verified:
  - [x] All behavior tests pass (≥8)
  - [x] HTTP probe behavior covered by `InternalGitHubAppEndpointsTests`
  - [x] `deploy/self-hosted/README.md` GitHub App Registration runbook present (5 steps + KMS mode section)
  - [x] GHES non-support documented in both provider file headers and README
  - [x] Redaction filter unit-tested with boundary cases for both Bearer and ghs_ patterns
  - [x] `docs/SPECIFICATION.md:8350–8388` cited in `IGitHubAppKeyProvider.cs` header

## Summary

All three improve iterations have been effective. The core RS256 signing logic, exception hierarchy, DI wiring, logging redaction, and test coverage are all sound. The iteration-3 duplicate comment finding is confirmed resolved — `Program.cs` now contains exactly one clean comment block explaining `ClearProviders()`. No new issues were introduced during any improve pass. The implementation is ready for `/verify`.
