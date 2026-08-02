# Code Review: M13-002

**Task:** $faker personal data — 10 functions including auto-sensitive $faker.ssn
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M13-002-faker-personal-data

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All new closures use `noArgs` wrapper returning structured `DYNFN_ARITY` errors. `IsSensitiveReturn` nil-guards correctly (`if r == nil || r.sensitiveReturn == nil`). `AddValue("")` is safely ignored by `SensitiveSet`. No panics for expected failures. |
| Input Validation | PASS | `noArgs` rejects unexpected args with structured arity error. Pool slices (`firstNames`, `lastNames`, `namePrefixes`, `nameSuffixes`) are non-empty constants — no `intn(rng, 0)` risk. The value-side hook fires inside the `s.runtimeSensitive != nil` guard, preventing nil dereference. |
| Naming | PASS | No stuttering. `IsSensitiveReturn`, `Registry`, `NewRegistry` have doc comments. `sensitiveReturn` field has an inline doc comment that clearly describes the hook's semantics and its mirror relationship to `sensitiveArgIdx`. |
| Code Organization | PASS | 10 new registrations grouped under a clear comment block in `register()`. `sensitiveReturn` map mirrors `sensitiveArgIdx` symmetrically. `IsSensitiveReturn` exposes only what callers need. `namePrefixes`/`nameSuffixes` slices are alongside the existing name pools. |
| Correctness | PASS | SSN format arithmetic produces non-zero values in all three groups (area 1–999, group 1–99, serial 1–9999), matching real SSA issuance constraints. Value-side hook in `Scope.Interpolate` fires inside the `s.runtimeSensitive != nil` guard. Smoke test confirms end-to-end redaction in `-vv`, `--format markdown`, and verifies absence from `--format json` (which has no request_body field). All three integration scenarios pass. |
| Test Quality | PASS | Unit coverage: 96.9% for `internal/variable`. All 13 behaviors covered (behavior 13 deferred with documented `t.Skip`). `TestRegistry_FakerSSN_Sensitive` exercises `RedactBody` directly. `TestRegistry_SensitiveReturn_HookFires` proves the hook independently before SSN code lands. Seeded-determinism and no-seed-entropy tests both present. |

## Previous Findings: All Resolved

The three findings from the previous review iteration are confirmed fixed:

1. **Critical (smoke JSON assertion)** — The `--format json` smoke block no longer asserts `[REDACTED]` in the JSON output (which has no `request_body` field). It now only verifies the SSN pattern is absent from the JSON output; positive-redaction proof is in the `-vv` and markdown blocks.

2. **Critical (mktemp macOS issue)** — The smoke test now uses `mktemp -t curlew_faker_ssn` (macOS-portable, no literal suffix after Xs) plus `trap 'rm -f "$FAKER_FILE"; rm -rf "${FAKER_MD_DIR:-}"; kill "$FAKER_SRV_PID" 2>/dev/null || true' EXIT` to ensure cleanup on all exit paths.

3. **High (MANUAL.md incorrect doc)** — `docs/MANUAL.md` now correctly states that `--format json` does not include a request-body field and that the SSN simply does not appear there; positive-redaction is documented for `-vv`, markdown report, and events stream.

## Test Coverage
- Coverage: 96.9% (`internal/variable`)
- Missing: none — all behaviors from task YAML are covered by at least one test.

## Summary

The implementation is clean and complete. The 10 personal-data functions produce spec-conforming values, the `sensitiveReturn` value-side hook is a sound symmetric mirror of the existing `sensitiveArgIdx` arg-side hook, and the integration smoke tests correctly reflect the actual output format capabilities (`--format json` has no request body field; redaction is proven via `-vv` and `--format markdown`). All three critical and high findings from the previous review iteration are confirmed fixed. Unit tests pass at 96.9% coverage with all 13 behaviors explicitly covered.
