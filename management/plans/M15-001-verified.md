# Verification Report: M15-001

**Task:** plugin_loading Enterprise gate enforced at CURLEW_PLUGINS load sites
**Verified by:** AI
**Date:** 2026-05-07
**Branch:** feature/M15-001-plugin-loading-gate
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected (ci-local.sh) |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean, including new free-tier reject block |
| Coverage (internal/auth) | 89.6% | Meets >= 80% threshold |
| Coverage (cmd/curlew) | 81.5% | Meets >= 80% threshold |
| Coverage (total) | 81.8% | Meets >= 80% threshold |

## Observable Output

```
# Free tier rejects CURLEW_PLUGINS (exit 6):
$ CURLEW_TIER=free CURLEW_PLUGINS=./cmd/curlew/testdata/plugins/hello ./curlew plugins list 2>&1; echo "exit=$?"
✗ Feature requires upgrade

  Plugin loading via CURLEW_PLUGINS requires Enterprise tier

  Your current tier: free
  Required tier:     enterprise

  Register for a free trial:
  https://apitesttool.com/register

  Upgrade:
  https://apitesttool.com/upgrade

  Workaround: Use built-in signers (signing: aws-sigv4, oauth1) or dynamic functions ({{$webhookSign.*}}, {{$jwtDecode*}}) for the common cases — see docs/MANUAL.md §6.8 and §3.7. These are available at every tier.

exit=6

# Empty CURLEW_PLUGINS at free tier (header-only table, exit 0):
$ CURLEW_TIER=free ./curlew plugins list; echo "exit=$?"
NAME  VERSION  HOOKS
exit=0
```

Expected: registered Description ("Plugin loading via CURLEW_PLUGINS requires Enterprise tier") and Workaround in stderr, exit 6 for free tier. Header-only table + exit 0 when env unset.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `plugin_loading` entry with RequiredTier=TierEnterprise, non-empty Description and Workaround | `TestDefaultRegistry_PluginLoading` | PASS |
| 2 | CURLEW_PLUGINS unset → no gate check, (nil, noop, nil) returned | `TestRun_PluginGate_NoCheckWhenEnvUnset`, `TestPluginsList_GateSkippedWhenEnvUnset` | PASS |
| 3 | CURLEW_PLUGINS set + Free/Solo/Professional/Team → buildHookDispatcher returns *GateError before LoadForRun | `TestRun_PluginGate_RejectsBelowEnterprise` (spawned==0) | PASS |
| 4 | CURLEW_PLUGINS set + Enterprise → host.LoadForRun invoked, dispatcher returned | `TestRun_HookPlugin_OnRequestReceivesRequest` et al. | PASS |
| 5 | CURLEW_PLUGINS set + Free/Solo/Professional/Team → pluginsListCmdOut exits 6 before host.Load | `TestPluginsList_GateRejectsBelowEnterprise` (spawned==0) | PASS |
| 6 | CURLEW_PLUGINS set + Enterprise → host.Load invoked, table rendered | `TestPluginsList_GateAllowsEnterprise` | PASS |
| 7 | Gate renders Description + Workaround; user-facing message is registered text | `TestPluginsList_GateRejectsBelowEnterprise` asserts "Enterprise" and "built-in signers" | PASS |
| 8 | docs/MANUAL.md plugin tier-gate paragraph verified to match enforcement | MANUAL.md explicitly states CURLEW_PLUGINS, Enterprise tier, exit 6 | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | `go test ./internal/auth/... ./cmd/curlew/... -run "Plugin\|Gate"` — 8 test functions, all PASS | PASS |
| 2 | `go test ./...` passes | ci-local.sh: all packages ok | PASS |
| 3 | `go test -cover ./internal/auth/... ./cmd/curlew/... >= 80%` | 89.6% + 81.5% (total 81.8%) | PASS |
| 4 | `golangci-lint run` passes with 0 issues | ci-local.sh lint gate clean | PASS |
| 5 | `./smoke/run.sh` passes | ci-local.sh smoke gate clean | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | `internal/auth/registry.go` has `plugin_loading: TierEnterprise` with non-empty Description and Workaround | Lines 143–148 confirmed | PASS |
| 8 | `internal/auth/registry_test.go` updated with four-tier matrix | `TestDefaultRegistry_PluginLoading` + `TestDefaultRegistry_PluginLoading_TierMatrix` | PASS |
| 9 | `cmd/curlew/plugins.go` gates both `buildHookDispatcher` and `pluginsListCmdOut` | Lines 44–47 and 119–131 | PASS |
| 10 | Tests cover: unset env at any tier; set env at Free/Solo/Professional/Team (exit 6); set env at Enterprise (loaded) | All gate tests PASS | PASS |
| 11 | `docs/MANUAL.md` plugin tier-gate paragraph verified and sharpened | Exit code and CURLEW_PLUGINS explicitly stated | PASS |
| 12 | `smoke/run.sh` extended with free-tier reject block | M15-001 block in smoke verified by ci-local.sh | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS — `errors.As` correctly guarded at both call sites; `%w` wrapping in `fmt.Errorf("plugin: %w", err)`; defensive fallback branch for non-GateError |
| Naming conventions | PASS — No stuttering; exported symbols have doc comments |
| Code organization | PASS — `internal/auth` boundary respected; `plugins.go` owns enforcement |
| Test quality | PASS — Spawner-never-invoked assertion at both call sites; table-driven four-tier matrix |
| Import hygiene | PASS — `errors` and `auth` imports correctly grouped |

Branch A: Review PASS (Iteration 3) trusted, spot-check clean — error handling, naming, and test quality all verified.

## Commits

| Hash | Message |
|------|---------|
| e774577a | docs(plan): add implementation plan for M15-001 |
| 7877e8b9 | chore(task): mark M15-001 as planned |
| 45fa533b | chore(task): mark M15-001 as in_progress |
| ee7ea636 | test(auth): add failing tests for plugin_loading registry entry |
| 6bd66703 | feat(auth): register plugin_loading feature gate at Enterprise tier |
| 79303a65 | test(cli): add failing tests for plugin_loading gate in pluginsListCmdOut |
| 9d182326 | feat(cli): gate pluginsListCmdOut behind plugin_loading Enterprise check |
| c6844bc8 | test(cli): add failing tests for plugin_loading gate in buildHookDispatcher |
| 95e68b8e | feat(cli): gate buildHookDispatcher and run callsite behind plugin_loading check |
| 8a54f2c3 | test(cli): add CURLEW_TIER=enterprise to existing plugin happy-path tests |
| ccf6fe1d | docs(manual): sharpen plugin tier-gate paragraph with exit code and enforcement note |
| d5cbd2a6 | chore(smoke): add CURLEW_TIER=enterprise to plugin sections, add free-tier reject test |
| 6cf5f469 | chore(task): mark M15-001 as review |
| ec54108a | docs(review): add review with findings for M15-001 |
| 378799df | docs(review): add improvement report for M15-001 |
| 5cd3341f | fix(plugins): resolve review findings #1 and #2 |
| 6bee44f1 | docs(review): add iteration 2 review for M15-001 |
| 0be59f23 | docs(review): add improvement report for M15-001 iteration 2 |
| a2f0e423 | fix(plugins): guard errors.As return value in pluginsListCmdOut gate |
| aebff9e3 | docs(review): add passing review for M15-001 |

## Files Changed

| File | Action | Notes |
|------|--------|-------|
| `internal/auth/registry.go` | modified | Added `plugin_loading` FeatureDefinition at TierEnterprise |
| `internal/auth/registry_test.go` | modified | Added `TestDefaultRegistry_PluginLoading` and `TestDefaultRegistry_PluginLoading_TierMatrix` |
| `cmd/curlew/plugins.go` | modified | Gate in `buildHookDispatcher` (lines 44–47) and `pluginsListCmdOut` (lines 119–131) |
| `cmd/curlew/plugins_test.go` | modified | 5 new gate tests; 11 existing tests patched with `CURLEW_TIER=enterprise` |
| `cmd/curlew/main.go` | modified | `errors.As` mapping for gate error at `buildHookDispatcher` callsite (~line 1241) |
| `cmd/curlew/discovery_run.go` | modified | `writeGateForFormat` used in run-path gate rendering |
| `docs/MANUAL.md` | modified | Plugin tier-gate paragraph sharpened with exit code and enforcement note |
| `smoke/run.sh` | modified | `CURLEW_TIER=enterprise` added to 4 happy-path invocations; free-tier reject block added |

## Issues Found

None.

## Recommendation

PASS — ready for PR and merge.
