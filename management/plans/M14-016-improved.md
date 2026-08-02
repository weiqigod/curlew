# Improvement Report: M14-016

**Task:** Backend: IGitHubAppKeyProvider + RS256 App-JWT signing
**Date:** 2026-05-06
**Review:** management/reviews/M14-016-review.md

## Resolved Findings — Iteration 1

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Critical | `IKmsClient` not registered when `ghAppMode=kms` and `keyMode!=kms`, causing DI startup crash | Added conditional `AddHttpClient("kms")` + `AddSingleton<IKmsClient, GoogleKmsClient>()` in `Program.cs` inside the `ghAppMode == "kms"` branch, guarded by `keyMode != "kms"` to avoid double-registration | ✓ tests pass |
| 2 | High | `BearerTokenRedactor` unit-tested but never registered in the actual logging pipeline | Added `builder.Services.AddSingleton<ILoggerProvider>(sp => new BearerTokenRedactor(new ConsoleLoggerProvider(...)))` in `Program.cs` | ✓ tests pass |
| 3 | Medium | Integration test omitted `iss` field assertion explicitly required by the task observable | Added `doc.RootElement.GetProperty("iss").GetInt64().Should().Be(12345)` to `InternalGitHubAppEndpointsTests` | ✓ tests pass |
| 4 | Medium | `FakeKmsClient` held two `IDisposable` crypto resources (`ECDsa`, `RSA`) without implementing `IDisposable` | Implemented `IDisposable` on `FakeKmsClient` disposing both keys; refactored `GoogleKmsGitHubAppKeyProviderTests` to use `using var kms` and `FakeKmsClientTests` to use `using var fake` | ✓ tests pass |

## Resolved Findings — Iteration 2

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `BearerTokenRedactor` registered via `AddSingleton<ILoggerProvider>` WITHOUT calling `ClearProviders()` first, leaving the default unredacted `ConsoleLoggerProvider` active alongside the wrapper — tokens logged twice, once in plain text | Called `builder.Logging.ClearProviders()` before the `AddSingleton<ILoggerProvider>` registration; added explicit `builder.Logging.AddDebug()` and `builder.Logging.AddEventSourceLogger()` to restore dev/trace tooling | ✓ tests pass |

## Resolved Findings — Iteration 3

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Low | Duplicated 18-line comment block in `Program.cs` (lines 244–261) explaining `ClearProviders()` rationale — two verbatim copies created a maintenance hazard | Removed the second copy; kept the first accurate explanation at lines 244–252 | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS |
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS (0 issues) |
| Tests (C#) | 1009 passed, 8 skipped (live Stripe integration, expected) |
| Targeted M14-016 tests | 18 passed, 0 failed |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `3d59d987` | fix(github): register IKmsClient and BearerTokenRedactor in Program.cs | iter-1 #1, #2 |
| `794f2072` | fix(tests): add iss assertion and dispose FakeKmsClient crypto keys | iter-1 #3, #4 |
| `98ced9e9` | fix(logging): clear default console provider before registering BearerTokenRedactor | iter-2 #1 |
| `29ec12b8` | fix(logging): remove duplicated ClearProviders comment in Program.cs | iter-3 #1 |

## Summary
6/6 findings resolved across 3 iterations. 0 deferred.
