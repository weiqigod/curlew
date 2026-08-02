# Code Review: M4-002

**Task:** CLI consumes shared vault template at runtime
**Reviewer:** AI
**Date:** 2026-04-15
**Branch:** feature/M4-002-cli-consumes-shared-vault

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors use `%w` wrapping with context; sentinel errors defined for all failure modes (`ErrTemplateNotFound`, `ErrUnknownEnvironment`, `ErrUnknownAlias`, `ErrEnvFlagRequired`); no swallowed errors; `errors.Is`/`errors.As` used consistently in tests and callers. |
| Input Validation | PASS | Empty path returns `(nil, nil)` with documented semantics; nil map lookups safe in Go; nil inputs handled throughout. The `realTeamProviderFactory` placeholder is honest and returns a clear actionable error. |
| Naming | PASS | No stuttering; all exported symbols have doc comments; package names correct; `-er` suffix conventions followed. |
| Code Organization | PASS | `internal/` package boundaries respected; team template loading in `config`, resolver in `vault/teamtemplate`, wiring in `runner`; `defer` used for resource cleanup; no circular dependencies. `secrets.` namespace correctly excluded from parallel dependency scanner. |
| Correctness | PASS | Cache protected by `sync.Mutex`; context propagated through `BulkFetch`; feature gate checked before vault access; `ErrEnvFlagRequired` raised before any HTTP dispatch; `SharedSecretsResolved` counter tracks actual resolved count; empty-map vs nil distinction handled correctly in cache check. |
| Test Quality | PASS | All 8 behaviors covered; stderr log line tested via `TestRunCmd_TeamSecrets_LogsResolvedCount`; staging value content verified via captured URL assertion in `TestRun_TeamSecrets/staging_uses_azure_provider`; error paths, edge cases, and caching all tested. |

## Test Coverage

- `internal/vault/teamtemplate`: 91.4%
- `internal/runner`: 85.9%
- `internal/variable`: 96.3% (new `WithSecrets`, `HasSecretsNamespace`, `SecretReferences` functions covered)
- `internal/config`: 96.4%
- `internal/parallel`: 90.6% (new `secrets.` exclusion logic covered)
- `cmd/apitest`: 83.0%

All packages meet the ≥80% threshold.

## Behavior Coverage

| # | Behavior | Status |
|---|----------|--------|
| 1 | production env resolves `{{secrets.api_key}}` via stub | PASS — `TestRun_TeamSecrets/resolves_production_alias_via_stub` + `TestRunCmd_TeamSecrets_Success` |
| 2 | staging env uses staging provider and resolves to staging values | PASS — `TestRun_TeamSecrets/staging_uses_azure_provider` asserts resolved URL contains `"stub::staging"` |
| 3 | missing APITEST_TEAM_CONFIG file exits with clear error | PASS — `TestRunCmd_TeamSecrets_MissingFile` + `TestLoadTeamTemplate/missing_file` |
| 4 | unknown alias fails before any HTTP traffic | PASS — `TestRun_TeamSecrets/unknown_alias_fails_before_http` (dispatcher count checked) |
| 5 | logs "Resolved N secrets from shared template (<env>)" exactly once | PASS — `TestRunCmd_TeamSecrets_LogsResolvedCount` captures stderr via `os.Pipe` and asserts message presence and count |
| 6 | `--env` omitted fails with explicit message | PASS — `TestRunCmd_TeamSecrets_MissingEnvFlag` + `TestRun_TeamSecrets/env_flag_required_when_secret_referenced` |
| 7 | help text documents `--env` and `APITEST_TEAM_CONFIG` | PASS — `TestRunCmd_Help_MentionsTeamTemplate` |
| 8 | vault provider queried once; second call served from cache | PASS — `TestSecretsResolver_CachedOnSecondCall` |

## Definition of Done Verification

| Item | Status |
|------|--------|
| Behavior tests pass for `internal/vault/teamtemplate` and `internal/runner` | PASS — all 6+ tests passing in both packages |
| `apitest run` produces expected resolved request against local test server | PASS — `TestRunCmd_TeamSecrets_Success` uses httptest server |
| Stub provider covers `aws-secrets-manager` and `azure-key-vault` shapes | PASS — `TestStubProvider_*` tests cover both; `TestRun_TeamSecrets/staging_uses_azure_provider` exercises Azure shape |
| Error paths exercised by red-path fixtures and asserted on | PASS — missing file, unknown env, unknown alias, missing --env flag all covered |
| Help text for `apitest run` mentions `--env` + shared template | PASS — `printHelp()` documents both `--env` and `APITEST_TEAM_CONFIG`; tested by `TestRunCmd_Help_MentionsTeamTemplate` |
| `smoke/run.sh` adds an end-to-end team template scenario | PASS — M4-002 section added at end of smoke script with python3 server + `APITEST_VAULT_STUB=1` |

## Summary

The implementation is complete and all previous review findings (iterations 1 and 2) have been addressed. The `TestRunCmd_TeamSecrets_LogsResolvedCount` test correctly captures stderr via `os.Pipe` and verifies both message content and that it appears exactly once. The staging env test now asserts the resolved stub value in the URL contains `"stub::staging"`, confirming the correct provider selection. All 8 behaviors are covered, all packages meet the 80% coverage threshold, the build is clean, and golangci-lint reports zero issues.
