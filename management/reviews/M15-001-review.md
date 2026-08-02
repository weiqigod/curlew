# Code Review: M15-001 (Iteration 3)

**Task:** plugin_loading Enterprise gate enforced at CURLEW_PLUGINS load sites
**Reviewer:** AI
**Date:** 2026-05-07
**Branch:** feature/M15-001-plugin-loading-gate

## Verdict: PASS

## Findings

No findings.

## Iteration 2 Finding Status

| Finding | Previous Verdict | Iteration 3 Status |
|---------|-----------------|-------------------|
| Medium: `errors.As` return value discarded in `pluginsListCmdOut`, leaving nil dereference unguarded | OPEN | **RESOLVED** — `plugins.go:122–129` now uses the correct `if errors.As(gateErr, &ge) { ... }` guard, matching the `main.go:1242` pattern exactly. `ge` is only dereferenced inside the `if` branch; the `else` branch prints `gateErr` directly. No nil dereference is possible. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `errors.As` correctly guarded in both `pluginsListCmdOut` and `main.go`. `buildHookDispatcher` returns gate error unwrapped so callsite can `errors.As` it. Defensive fallback branch matches the established codebase pattern (`main.go:1250–1255`). |
| Input Validation | PASS | Gate fires only when `env != ""`, preserving the existing empty-env contract. Help and unknown-flag paths return before the gate check. |
| Naming | PASS | No stuttering. All exported symbols in changed files have doc comments. |
| Code Organization | PASS | `internal/auth` boundary respected. `plugins.go` owns enforcement. No circular dependencies. `discovery_run.go` uses `FeatureGate` renderer for the terminal path. |
| Correctness | PASS | Four-tier matrix enforced at both `pluginsListCmdOut` and `buildHookDispatcher`. Gate fires before `host.Load`/`host.LoadForRun` (verified by spawner-never-invoked check in tests). `writeGateForFormat` used for run-path rendering, matching the established gate-error flow. |
| Test Quality | PASS | All 8 behaviors covered. Four-tier matrix tested at both call sites. Workaround text (`"built-in signers"`) asserted at CLI level. Spawner-never-invoked check in both `plugins list` and `run` paths. Existing happy-path tests patched with `CURLEW_TIER=enterprise`. `smoke/run.sh` extended with free-tier rejection block. |

## Test Coverage

- `internal/auth`: 89.6%
- `cmd/curlew`: 81.5%
- Both exceed the 80% threshold. ✓

## Behavior Coverage

| Behavior | Test |
|----------|------|
| `plugin_loading` registered in DefaultRegistry with TierEnterprise, non-empty Description and Workaround | `TestDefaultRegistry_PluginLoading` |
| Four-tier matrix: Free/Solo/Professional/Team reject; Enterprise allows | `TestDefaultRegistry_PluginLoading_TierMatrix` |
| CURLEW_PLUGINS unset → no gate check, (nil, noop, nil) returned | `TestRun_PluginGate_NoCheckWhenEnvUnset`, `TestPluginsList_GateSkippedWhenEnvUnset` |
| CURLEW_PLUGINS set + non-Enterprise tier → buildHookDispatcher returns *GateError before LoadForRun | `TestRun_PluginGate_RejectsBelowEnterprise` (spawned==0) |
| CURLEW_PLUGINS set + Enterprise → host.LoadForRun invoked, dispatcher returned | `TestRun_HookPlugin_OnRequestReceivesRequest` et al. |
| CURLEW_PLUGINS set + non-Enterprise tier → pluginsListCmdOut exits 6 before host.Load | `TestPluginsList_GateRejectsBelowEnterprise` (spawned==0) |
| CURLEW_PLUGINS set + Enterprise → host.Load invoked, table rendered | `TestPluginsList_GateAllowsEnterprise` |
| Gate renders Description + Workaround to user | `TestPluginsList_GateRejectsBelowEnterprise` and `TestRun_PluginGate_RejectsBelowEnterprise` assert `"Enterprise"` and `"built-in signers"` in stderr |
| docs/MANUAL.md matches enforcement | MANUAL.md line 3373 explicitly states CURLEW_PLUGINS, Enterprise tier, exit 6 |

## Summary

All three iterations of findings are now resolved. The implementation correctly registers `plugin_loading` at `TierEnterprise` in `DefaultRegistry`, gates both `buildHookDispatcher` and `pluginsListCmdOut` behind `auth.CheckFeature` when `CURLEW_PLUGINS` is non-empty, and maps the resulting `*GateError` through the established exit-6 rendering paths. The codebase gate pattern — `if errors.As(...) { render; return 6 }` with a defensive fallback — is consistent across all three call sites (`plugins.go`, `main.go`, `discovery_run.go`). Test coverage exceeds 80% in all affected packages. CI gate passes.
