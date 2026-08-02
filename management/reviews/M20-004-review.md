# Code Review: M20-004

**Task:** MANUAL.md locale reference + cross-locale seed reproducibility matrix
**Reviewer:** AI
**Date:** 2026-06-12
**Branch:** feature/M20-004-locale-manual-reproducibility-matrix

## Verdict: PASS

## Findings

No findings. Code meets all standards.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new production code; test errors use `t.Fatalf` / `t.Errorf` correctly. No error swallowing. `mustResolve` propagates `resolveLocaleData` errors via `t.Fatalf`. |
| Input Validation | PASS | `mustResolve` helper propagates `resolveLocaleData` errors via `t.Fatalf`; no silent swallowing. Guard `len(supportedLocales) != 15` catches future locale additions. |
| Naming | PASS | `TestLocale_ReproducibilityMatrix`, `TestLocale_ReproducibilityMatrix_SameDrawOrder`, `mustResolve` all follow conventions. No stuttering. Doc comments on all three functions. Package name `variable` is correct. |
| Code Organization | PASS | New test file in the correct `package variable` scope. Helper `mustResolve` is test-file-local with no collision in the package. Single responsibility. |
| Correctness | PASS | Both matrix tests pass for all 15 locales. Two independent registries per locale correctly simulate two process invocations. `mustResolve` uses the real `resolveLocaleData` fallback chain so tests read the same pool as runtime. Empty-value guard in place. |
| Test Quality | PASS | Table-driven via `supportedLocales` slice; subtests named by locale code; `t.Run` used throughout; error paths fatalf-guarded; cross-locale draw-order semantics documented via code comments matching KAD-2 rationale. |

## Spec Compliance — Behaviors

| # | Behavior | Status | Notes |
|---|----------|--------|-------|
| 1 | 15-row locale reference table in MANUAL.md with locale code, Language/Region, Name-format exemplar, Phone-format exemplar | PASS | Table at MANUAL.md:1278–1294; observable #1 grep returns 19 ≥ 15. |
| 2 | Seven M13 deferral notes replaced | PASS | `grep -F 'flag is deferred from M13' docs/MANUAL.md` returns no match. Content/financial/file sections now state "locale-neutral by design" without deferral framing. |
| 3 | Fixed `--seed`, any locale, `$faker.fullName` byte-identical across two runs | PASS | `TestLocale_ReproducibilityMatrix` covers fullName, phone, city, firstName for all 15 locales with two independent registries. |
| 4 | Fixed `--seed`, `$faker.firstName` pool-membership verified per locale | PASS | `TestLocale_ReproducibilityMatrix_SameDrawOrder` asserts each result is a member of the active locale's firstNames pool. Draw-order semantics documented in comments per KAD-2. |
| 5 | MANUAL.md documents precedence chain "Default (en-US) < Project < Environment < Collection < CLI flag" | PASS | MANUAL.md:1296–1307 correctly lists all 5 levels. `Environment` level is present and explained with the caveat that the seam is not yet wired to env-file parsing. Fixed in improvement iteration. |
| 6 | `testdata/locale/reproducibility-matrix.yaml` runs green via `curlew run` with `--seed 12345` | CONDITIONAL | Fixture is structurally valid; uses canonical `assertions:` format with `body:` JSONPath map (parser-supported per collection.go BodyAssertions). Not run in hermetic CI by design (KAD-5). Requires network access to httpbin.org for live verification. |
| 7 | MANUAL.md documents ERR_LOCALE_UNKNOWN with supported-locale list | PASS | MANUAL.md:1315–1321 documents the error code, `CategoryInput` error category, and shows the hint matching `localeUnknownError` output (locale.go:181–186). |

## Test Coverage
- Coverage: 97.2% (internal/variable package)
- Missing coverage: `resolveLocaleData` line 163 (the non-unknown-locale fallback path) at 90.9% — pre-existing, not introduced by this task.

## Summary

The implementation is clean and complete. The medium-severity finding from the first iteration (precedence chain header omitting the `Environment` level) has been correctly fixed: MANUAL.md:1297 now reads "Default (`en-US`) < Project < Environment < Collection < CLI flag (`--locale`)." with the Environment bullet added to the description list and the explanatory caveat retained. All seven M13 deferral notes are replaced with substantive, factually accurate documentation. The reproducibility matrix tests are thorough, covering all 15 locales with two independent registries per locale, and the fixture follows established conventions. No new findings.
