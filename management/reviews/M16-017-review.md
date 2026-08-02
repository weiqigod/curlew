# Code Review: M16-017

**Task:** team_vaults table, endpoints, manifest validator, and web vault-config page
**Reviewer:** AI
**Date:** 2026-05-12
**Branch:** feature/M16-017-team-vaults-config
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All three issues from the iteration-1 review were resolved:

| Prior # | Severity | Resolution |
|---------|----------|-----------|
| 1 | Medium | `JsonDocument` wrapped in `using` block in `VaultConfigService.UpsertAsync` (line 85) — pooled buffers returned immediately after validation. |
| 2 | Medium | `Put_returns_200_with_warnings_in_warn_mode` endpoint integration test added to `VaultConfigEndpointsTests.cs` — exercises endpoint + `ValidatorMode=warn` configuration together via `WithWebHostBuilder`. |
| 3 | Low | `VaultConfigYamlConverterTests.cs` created with 7 edge-case tests: empty YAML, whitespace-only, scalar string, integer, deeply nested, multiline block scalar, list, and malformed YAML throwing `YamlException`. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors returned (never panicked). `YamlException` wrapped with context. Service error enum (`VaultConfigServiceError`) is distinct from tier-gate enum (`Internal.TierGates.VaultConfigError`). Audit append is transactional with `SaveChangesAsync`. |
| Input Validation | PASS | Empty/whitespace YAML body rejected with `InvalidYaml`. Permission checked before DB access. Tier gate checked before service call. `OrgId.TryParse` guards malformed org IDs at endpoint boundary. |
| Naming | PASS | No stuttering. All exported symbols have doc comments. `VaultConfigServiceError` correctly namespaced to avoid collision. `VaultManifestValidator` uses source-generated regex (C# 7+ pattern). |
| Code Organization | PASS | `internal/` boundaries respected. Thin endpoint layer delegates to service. Pure `VaultManifestValidator` (static, no DB) separated from service. Clean package under `ApiTool.Backend.VaultConfig`. Migration is auto-generated and correct. |
| Correctness | PASS | `JsonDocument` disposed immediately after validation. Walk logic correct: string-valued sensitive keys are flagged; non-string values recurse normally. ETag comparison correctly strips quotes and handles `*` wildcard. `idx_team_vaults_updated` index present. FK constraints (`Cascade` for org, `Restrict` for users) match spec. |
| Test Quality | PASS | 58+ tests across all layers: 13 service tests, 6+ validator tests, 8 YAML converter tests, 17 endpoint tests (including warn-mode), 5 vitest web unit tests, 7 Playwright E2E specs. All 10 task YAML behaviors covered. |

## Test Coverage

- Backend service: 100% (per improvement report)
- `VaultConfigYamlConverter`: 100%
- `VaultConfigEndpoints`: ~94%
- `VaultManifestValidator`: ~84%
- Web API client: 5 vitest tests — all behaviors covered
- E2E: 7 Playwright specs covering all observable behaviors

## Summary

The implementation is architecturally sound across all layers: clean EF Core migration with correct schema, TDD commits in RED-GREEN-REFACTOR order, correct ETag/304 semantics, transactional audit logging, a working reject/warn validator-mode toggle bridged via environment variable, and a minimum-viable SvelteKit vault-config page. All three findings from iteration 1 were correctly resolved. No new findings identified in this pass.
