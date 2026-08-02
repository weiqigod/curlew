# Improvement Report: M16-017

**Task:** team_vaults table, endpoints, manifest validator, and web vault-config page
**Date:** 2026-05-12
**Review:** management/reviews/M16-017-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `JsonDocument.Parse(json)` in `VaultConfigService.UpsertAsync` allocated a pooled `IDisposable` that was never disposed, leaking `ArrayPool<byte>` buffers for the lifetime of the GC object. | Wrapped in a `using` block (`using (var jsonDoc = JsonDocument.Parse(json)) { … }`) so the pooled buffer is returned immediately after `VaultManifestValidator.Validate` extracts its string paths. | ✓ tests pass |
| 2 | Medium | Behavior #6 ("in warn mode, suspicious YAML is saved with 200 + warnings") had no endpoint-level integration test — only the service-level test covered it. | Added `Put_returns_200_with_warnings_in_warn_mode` to `VaultConfigEndpointsTests.cs`. Uses `WithWebHostBuilder` + `ConfigureAppConfiguration` to set `ValidatorMode=warn`, then sends the suspicious YAML and asserts `200 OK` with `warnings[0]` present in the response body. | ✓ tests pass |
| 3 | Low | Plan specified a dedicated `VaultConfigYamlConverterTests.cs` with round-trip edge cases, but the file was not created during execution. Edge cases (empty YAML, whitespace, scalars, integers, deeply nested, multiline, list, and malformed YAML throwing `YamlException`) were untested. | Created `src/ApiTool.Backend.Tests/VaultConfig/VaultConfigYamlConverterTests.cs` with 8 targeted tests covering all identified edge cases. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `dotnet build src/ApiTool.Backend` | PASS |
| `dotnet test src/ApiTool.Backend.Tests` | PASS (1790 passed, 0 failed, 8 skipped) |
| `golangci-lint run` | PASS (0 issues) |
| VaultConfig coverage (`VaultConfigService`) | 100% |
| VaultConfig coverage (`VaultConfigYamlConverter`) | 100% |
| VaultConfig coverage (`VaultConfigEndpoints`) | ~94% |
| VaultConfig coverage (`VaultManifestValidator`) | ~84% |
| Overall VaultConfig suite | 58 tests, all pass |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `bb3b4362` | fix(vault-config): dispose JsonDocument after manifest validation | #1 |
| `976ff39f` | test(vault-config): add endpoint integration test for warn-mode PUT | #2 |
| `d42a4f86` | test(vault-config): add VaultConfigYamlConverterTests with edge cases | #3 |

## Summary

3/3 findings resolved. 0 deferred. All VaultConfig tests pass (58 tests). Full backend suite passes (1790 tests). Lint is clean.
