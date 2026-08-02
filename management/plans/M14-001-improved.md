# Improvement Report: M14-001 (Iteration 3)

**Task:** Backend: signing_keys table + IKeyProvider with File and GoogleKMS providers
**Date:** 2026-05-04
**Review:** management/reviews/M14-001-review.md

## Iteration 1 Resolved Findings (all verified fixed in iteration 2 review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `EfSigningKeyStore.UpdateStatusAsync` never sets `PromotedAt` when transitioning to `current`; only the bootstrap path stamped it | Added `if (newStatus == KeyStatus.Current) row.PromotedAt = clock.GetUtcNow().UtcDateTime;` branch in `UpdateStatusAsync`; added `UpdateStatusAsync_sets_PromotedAt_when_transitioning_to_current` test in `EfSigningKeyStoreTests` | ✓ confirmed in iteration 2 review |
| 2 | High | `POST_internal_keys_rotate_emergency_returns_new_kid_distinct_from_old` test never asserted `newKid != oldKid`; test would pass even if rotation returned the same kid | Rewrote the test: seeds a `KeyStatus.Next` row via DI (`IServiceProvider`) before rotating so the rotator actually promotes a different key; added `newKid.Should().NotBe(oldKid)` and `newKid.Should().Be(nextKid)` assertions | ✓ confirmed in iteration 2 review |
| 3 | Medium | `GetKeyForSigningAsync` was `public` on `FileKeyProvider` but absent from `IKeyProvider`; callers injecting `IKeyProvider` could not call it; kid-format validation was not enforced through the interface contract | Added `GetKeyForSigningAsync(string kid, CancellationToken ct = default)` to `IKeyProvider` interface with full XML-doc; implemented it in `GoogleKmsKeyProvider` (validates kid, returns KMS resource name); `FileKeyProvider` already had the implementation, signature harmonised with interface | ✓ confirmed in iteration 2 review |
| 4 | Medium | `Program.cs` registered `GoogleKmsClient` without calling `AddHttpClient("kms")`; default client had no timeout, risking indefinite hangs on KMS network failure | Added `builder.Services.AddHttpClient("kms", c => c.Timeout = TimeSpan.FromSeconds(10));` inside the `keyMode == "kms"` branch, before the singleton registration | ✓ confirmed in iteration 2 review |
| 5 | Medium | `InternalAccessFilter` compared `X-Internal-Secret` header with `string == string`, enabling a remote timing oracle | Replaced with `CryptographicOperations.FixedTimeEquals(Encoding.UTF8.GetBytes(headerVal), Encoding.UTF8.GetBytes(secret))` | ✓ confirmed in iteration 2 review |
| 6 | Medium | No test for 404-on-non-loopback behavior of `InternalAccessFilter` | Created `InternalAccessFilterTests.cs` with 5 unit tests: non-loopback 404 in Production, loopback allow, correct secret allow, wrong secret deny, Testing environment bypass | ✓ confirmed in iteration 2 review |
| 7 | Low | `ReadPrivatePemAsync` could recurse infinitely when `BootstrapAsync` succeeds at DB insert but fails to write the PEM file (e.g., disk full) | Added `bool isRetry = false` parameter; on second call with `isRetry: true` throws `InvalidOperationException` instead of recursing again | ✓ confirmed in iteration 2 review |

## Iteration 2 Resolved Findings (all verified fixed in iteration 3 review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `RotateAsync(emergency: true)` with no staged `next` key silently returned the compromised kid unchanged — no revocation, no new key | Added emergency branch in the `next is null` path: revokes the current key via `UpdateStatusAsync(current.Kid, Revoked, reason, ...)` then calls `provider.GetActiveKidAsync` which bootstraps a fresh key pair since there is no longer a `current` row. Updated class doc to accurately describe the no-next emergency path. | ✓ confirmed in iteration 3 review |
| 2 | High | `GoogleKmsKeyProvider.SignAsync` used null-forgiving operator `current.KmsKeyId!` — would throw `NullReferenceException` if a file-backed row (KmsKeyId=null) was present on a mode-switch restart | Added explicit null guard: `if (current.KmsKeyId is null) throw new InvalidOperationException(...)` with a diagnostic message citing the mode-switch scenario and remediation steps before the `AsymmetricSignAsync` call. | ✓ confirmed in iteration 3 review |

## Iteration 3 Resolved Findings (all verified fixed in iteration 4 review)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `GetKeyForSigningAsync` was a 4th method on `IKeyProvider` exceeding the spec-mandated 3-method surface (spec line 8048-8054). It was never called from production code, leaking internal key-material retrieval onto the public interface. | Removed `GetKeyForSigningAsync` from `IKeyProvider` interface. The method is retained on `FileKeyProvider` (concrete type) — the test calls it on the concrete type directly (the helper already returned `FileKeyProvider`). No production caller is affected. | ✓ confirmed in iteration 4 review |
| 2 | Low | `ISigningKeyRotator` doc comment falsely stated "After rotation a new `next` key is provisioned." The implementation does not provision a new `next` key after rotation. | Removed the false sentence from the method-level `<para>` block in `ISigningKeyRotator.cs`. | ✓ partially resolved (class-level `<summary>` retained the claim — fixed in iteration 4) |
| 3 | Low | `FileKeyProvider.GetKeyForSigningAsync` was missing `= default` on its `CancellationToken ct` parameter, inconsistent with the rest of the codebase. | Added `= default` to `CancellationToken ct = default` in the method signature. | ✓ confirmed in iteration 4 review |

## Iteration 4 Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | `ISigningKeyRotator` class-level `<summary>` at line 6 still contained "and provisions a new `<c>next</c>` key." — the false claim was removed from the method-level `<para>` in iteration 3 but the identical text remained in the class summary. `SigningKeyRotator.RotateAsync` does not provision a new `next` key. | Removed "and provisions a new `<c>next</c>` key." from the class `<summary>`. Summary now reads: "Orchestrates the key rotation lifecycle: promotes `<c>next</c>` to `<c>current</c>`, demotes the old `<c>current</c>` to `<c>verifying</c>` (or `<c>revoked</c>` in emergency)." | ✓ tests pass; `go test ./...` PASS; `golangci-lint` 0 issues |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (662 passed, 0 failed) |
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Go coverage | 87.7% |
| Backend coverage | 92.45% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `cfabfbbe` | fix(keys): set PromotedAt when UpdateStatusAsync transitions to current | iter1 #1 |
| `63b2da3b` | fix(keys): resolve review findings #2-#7 | iter1 #2, #3, #4, #5, #6, #7 |
| `34c76d97` | fix(keys): emergency rotation no-op + KMS null KmsKeyId guard | iter2 #1, #2 |
| `2d4038ed` | fix(keys): resolve review findings #1, #2, #3 for M14-001 | iter3 #1, #2, #3 |
| `b7075585` | fix(keys): remove false 'provisions next key' claim from ISigningKeyRotator summary | iter4 #1 |

## Summary

13/13 findings resolved across 4 review iterations. 0 deferred.

Iteration 4 completed the incomplete iteration-3 fix for the `ISigningKeyRotator` false documentation claim: the class-level `<summary>` now accurately describes the rotation lifecycle without claiming a new `next` key is provisioned post-rotation.
