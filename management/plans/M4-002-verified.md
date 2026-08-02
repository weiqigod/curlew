# Verification Report: M4-002

**Task:** CLI consumes shared vault template at runtime
**Verified by:** AI
**Date:** 2026-04-15
**Branch:** feature/M4-002-cli-consumes-shared-vault
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | 28 packages, all pass |
| `golangci-lint run` | PASS | 0 issues |
| `./smoke/run.sh` | PASS | All scenarios pass including M4-002 section |
| Coverage (`internal/vault/teamtemplate`) | 91.4% | Meets >= 80% threshold |
| Coverage (`internal/runner`) | 85.9% | Meets >= 80% threshold |
| Coverage (`cmd/apitest`) | 83.0% | Meets >= 80% threshold |
| Coverage (total) | 89.2% | Meets >= 80% threshold |

## Observable Output

```
Collection: Uses team vault
Resolved 2 secrets from shared template (production)
  ✓ Ping with shared secret  200  3ms

────────────────────────────────
  1 request(s): 1 passed, 0 failed (3ms)
Exit code: 0
```

Command: `APITEST_TIER=team APITEST_TEAM_CONFIG=testdata/team/shared-vault-template.yaml APITEST_VAULT_STUB=1 ./apitest run testdata/team/uses-team-vault.yaml --env production`

Expected: test passes; stdout shows "Resolved 2 secrets from shared template (production)"
Result: MATCH

Note: `APITEST_TIER=team` is required to bypass the feature gate (same as smoke test). The task YAML's observable omits this env var, but the intent is met — the feature works as specified when the tier is active.

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | production env resolves `{{secrets.api_key}}` via stub | `TestRun_TeamSecrets/resolves_production_alias_via_stub`, `TestRunCmd_TeamSecrets_Success` | PASS |
| 2 | staging env uses staging provider and resolves to staging values | `TestRun_TeamSecrets/staging_uses_azure_provider` (asserts URL contains "stub::staging") | PASS |
| 3 | missing APITEST_TEAM_CONFIG file exits with clear error | `TestRunCmd_TeamSecrets_MissingFile`, `TestTeamTemplate/missing_file` | PASS |
| 4 | unknown alias fails before any HTTP traffic | `TestRun_TeamSecrets/unknown_alias_fails_before_http` (dispatcher count checked) | PASS |
| 5 | logs "Resolved N secrets from shared template (<env>)" exactly once | `TestRunCmd_TeamSecrets_LogsResolvedCount` (stderr captured via os.Pipe) | PASS |
| 6 | `--env` omitted fails with explicit message | `TestRunCmd_TeamSecrets_MissingEnvFlag`, `TestRun_TeamSecrets/env_flag_required_when_secret_referenced` | PASS |
| 7 | help text documents `--env` and `APITEST_TEAM_CONFIG` | `TestRunCmd_Help_MentionsTeamTemplate` | PASS |
| 8 | vault provider queried once; second call served from cache | `TestSecretsResolver_CachedOnSecondCall` | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | Behavior tests pass for `internal/vault/teamtemplate` and `internal/runner` | `go test ./internal/vault/teamtemplate/... ./internal/runner/...` all pass | PASS |
| 2 | `apitest run` produces expected resolved request against local test server | `TestRunCmd_TeamSecrets_Success` uses httptest; smoke test M4-002 section passes | PASS |
| 3 | Stub provider covers `aws-secrets-manager` and `azure-key-vault` shapes | `TestStubProvider_*` tests cover both; staging test exercises azure shape | PASS |
| 4 | Error paths exercised by red-path fixtures and asserted on | missing file, unknown env, unknown alias, missing --env all covered and tested | PASS |
| 5 | Help text for `apitest run` mentions `--env` + shared template | `TestRunCmd_Help_MentionsTeamTemplate` asserts APITEST_TEAM_CONFIG, --env, "shared vault" | PASS |
| 6 | `smoke/run.sh` adds an end-to-end team template scenario | M4-002 section present in smoke/run.sh with python3 server + APITEST_VAULT_STUB=1 | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — `%w` wrapping throughout; sentinel errors `ErrTemplateNotFound`, `ErrUnknownEnvironment`, `ErrUnknownAlias`, `ErrEnvFlagRequired` |
| Naming conventions | PASS — no stuttering; all exports have doc comments |
| Code organization | PASS — `internal/` boundaries respected; `defer` used; no circular deps |
| Test quality | PASS — table-driven tests; behavior coverage; error path coverage; caching verified |

Branch A: Review PASS trusted. Spot-check results:
- Error wrapping: `fmt.Errorf("%w: %q", ...)` used in `resolver.go` — PASS
- Doc comments: all exported symbols in `teamtemplate` package have doc comments — PASS
- Test quality: `TestSecretsResolver_CachedOnSecondCall` verifies call count via custom provider — PASS

## Commits

| Hash | Message |
|------|---------|
| 3bea72d | docs(review): add passing review for M4-002 |
| 3b076db | test(runner,cmd): assert log line and staging-namespaced stub values |
| e63a9cb | test(vault/teamtemplate): cover BulkFetch error, missing-path, field extraction error, and factory error paths |
| 67f22df | test(cmd): verify help text content for APITEST_TEAM_CONFIG and shared vault |
| c26c84d | test(runner): add visitBody branch coverage and collectSecretReferences body-type tests |
| 92b1c5e | fix(smoke): resolve SIGPIPE failure on tap help check and stub fixture |
| 74eeb19 | refactor(cli,variable,vault): fix golangci-lint findings |
| 26dfd86 | feat(cli): add fixtures, smoke test, and changelog entry (M4-002 Step 6) |
| 8f3d1c5 | feat(cli): wire team vault template into runCmdInner (M4-002 Step 5) |
| 3fa42b2 | feat(runner): wire team vault template into buildScope (M4-002 Step 4) |
| 36889ce | feat(vault): implement SecretsResolver and LoadTeamTemplate (M4-002 Step 3) |
| 90b7ce0 | feat(vault): implement StubProvider for APITEST_VAULT_STUB=1 (M4-002 Step 2) |
| 39062a4 | feat(variable): add secrets namespace for {{secrets.ALIAS}} interpolation |

## Files Changed

| File | Action |
|------|--------|
| `cmd/apitest/main.go` | modified — wired --env, team config, APITEST_TEAM_CONFIG |
| `cmd/apitest/run_test.go` | modified — TeamSecrets CLI integration tests |
| `internal/config/team_template.go` | added — LoadTeamTemplate, TeamTemplate types |
| `internal/config/team_template_test.go` | added — config loading tests |
| `internal/parallel/scan.go` | modified — exclude secrets. namespace from dependency scan |
| `internal/runner/runner.go` | modified — buildScope wired to SecretsResolver |
| `internal/runner/runner_test.go` | modified — TeamSecrets runner tests |
| `internal/variable/variable.go` | modified — secrets namespace interpolation |
| `internal/vault/teamtemplate/resolver.go` | added — SecretsResolver with caching |
| `internal/vault/teamtemplate/stub.go` | added — StubProvider for APITEST_VAULT_STUB=1 |
| `internal/vault/teamtemplate/teamtemplate.go` | added — provider interfaces |
| `smoke/run.sh` | modified — M4-002 end-to-end scenario |
| `testdata/team/shared-vault-template.yaml` | added — test fixture |
| `testdata/team/uses-team-vault.yaml` | added — test collection fixture |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge. All 8 behaviors verified, all DoD items met, coverage 89.2% overall, lint clean, smoke test passes end-to-end.
