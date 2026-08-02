# Code Review: M17-001

**Task:** Signer registry foundation and `signing:` request field
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M17-001-signer-registry-foundation
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All issues from iteration 1 are resolved.

## Previous Findings — Resolved

| # | Severity | Finding | Resolution |
|---|----------|---------|-----------|
| 1 | Medium | `SetSigningExplicitNullForTest` exported from production `collection.go` | Removed. `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` rewritten to use inline YAML fixture with `signing: ~` parsed via `parser.ParseFile`. No test-only helper in production code. |
| 2 | Low | `explicitNull bool` field missing `yaml:"-"` struct tag | Fixed. Line 40 of `collection.go` now reads `explicitNull bool \`yaml:"-"\``. |
| 3 | Low | Unprotected `int` counter `fired` in test closure | Fixed. Changed to `var fired int32` with `atomic.AddInt32` / `atomic.LoadInt32`. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All errors wrapped with `%w`. Sentinel `ErrUnknownSignerType` correctly defined and tested. Signer build and Sign errors wrapped with context. No swallowed errors. |
| Input Validation | PASS | `SigningSpec.UnmarshalYAML` rejects missing type and non-mapping values. `Registry.Register` rejects duplicates. `Registry.Lookup` returns structured error with sorted available list. Explicit-null semantics correctly detected via `!!null` tag. |
| Naming | PASS | No stuttering. Doc comments on all exported symbols. `Signer` interface follows `-er` convention. No test-only helpers in production code. |
| Code Organization | PASS | `internal/signer` is clean and independent. Signing logic isolated in `runner/signing.go`. Package boundaries respected throughout. `explicitNull` field correctly tagged. |
| Correctness | PASS | Precedence resolution (per-request > collection > none; explicit null disables) correctly implemented and tested via YAML fixture parsing. Signing sits outside hooks wrap (signer fires first, then hooks see signed request). All four execution paths (sequential, sequential-DD, parallel-DD, parallel-main via PreExec) are wired. Re-exec on 401 refresh carries same signing ctx. Fast-path verified (no allocation when no signing field present). `collectionUsesSigning` walks setup/main/teardown. |
| Test Quality | PASS | All behaviour tests present. Counters use atomics. `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` now exercises the YAML decoder directly. Race detector passes. |

## Test Coverage
- `internal/signer`: **100%**
- `internal/parser`: **90.1%** (above 80% DoD threshold, no regression)
- `internal/runner`: **85.0%** (above 80% DoD threshold, no regression)
- `internal/parallel`: **90.1%** (PreExec covered by AttachesPerItemCtx + Nil_Passthrough)

## Behavior Coverage

All 11 behaviors from the task YAML are covered:

| Behavior | Test |
|----------|------|
| Register + Lookup returns factory | `TestSignerRegistry_RegisterAndLookup` |
| Lookup unknown → ErrUnknownSignerType with sorted list | `TestSignerRegistry_Lookup_UnknownType` |
| YAML `signing:` round-trips to `*SigningSpec` | `TestParser_SigningField_RequestLevel` |
| Collection default + per-request override + explicit null | `TestParser_SigningField_CollectionDefault` + `TestRunner_SignerStep_CollectionDefault_RequestNullDisables` |
| Unknown type → ErrUnknownSignerType on request Err | `TestRunner_SignerStep_UnknownTypeProducesError` |
| Signer invoked between templating and exec | `TestRunner_SignerStep_InvokedBetweenTemplatingAndExec` + `TestRunner_Signing_EndToEnd_Noop` |
| No signing → exec pointer unchanged | `TestRunner_SignerStep_NoSigning_FastPath` |
| sensitives.AddValue inside Sign registered on run set | `TestRunner_SignerStep_SensitiveValueRegistered` |
| auth/profile.go unchanged | Verified: `git diff internal/auth/profile.go` produces 0 lines |
| No clock on Signer interface | Package doc states the no-clock contract explicitly |
| docs/MANUAL.md apitest-sigv4 removed; Enterprise clarification present | Verified: grep confirms "doc-fix landed" and Enterprise paragraph carries sibling clarification |

## Definition of Done — Verified

| DoD Item | Status |
|----------|--------|
| All behavior tests pass | PASS |
| `go test ./...` passes | PASS |
| `go test -cover ./internal/signer/... >= 80%` | PASS (100%) |
| `go test -cover ./internal/parser/... >= 80%` | PASS (90.1%, no regression) |
| `go test -cover ./internal/runner/... >= 80%` | PASS (85.0%, no regression) |
| `golangci-lint run` passes with 0 issues | PASS |
| `./smoke/run.sh` passes | PASS |
| `./scripts/ci-local.sh` passes | PASS |
| `internal/auth/profile.go` unchanged | PASS |
| `signer.go` package doc states no-clock contract | PASS |
| `docs/MANUAL.md` no longer cites apitest-sigv4 | PASS |
| `docs/MANUAL.md` Enterprise plugin tier-gate clarification present | PASS |
| Smoke/integration fixture exercising end-to-end wiring | PASS (`TestRunner_Signing_EndToEnd_Noop`) |

## Summary

All three findings from iteration 1 are resolved. The implementation is correct, well-structured, and functionally complete. No new issues were introduced by the improvements. All coverage thresholds are met, the CI gate passes cleanly, and all 11 behaviors from the task YAML are covered by tests.
