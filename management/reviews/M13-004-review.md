# Code Review: M13-004

**Task:** $faker company data — 5 functions
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M13-004-faker-company-data

## Verdict: PASS

## Findings

No findings.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths introduced. All five registrations are `noArgs` closures — arity errors are handled by the existing `noArgs` wrapper (pre-existing). No new error sites. |
| Input Validation | PASS | All pool access is guarded by `intn(rng, len(pool))`; pools are all non-empty (verified by `TestFakerCompany_PoolsNonEmpty`). No panics possible on nil/empty. |
| Naming | PASS | All new symbols are unexported (`companies`, `companySuffixes`, `jobTitles`, `departments`, `catchPhraseAdjectives`, `catchPhraseNouns`, `catchPhraseGerunds`). All have doc comments. The `companyPoolContains` helper in tests avoids collision with any existing `contains` helper. No stuttering. |
| Code Organization | PASS | All changes confined to `internal/variable/`. Pool variables placed at file scope after the M13-003 pools. Registrations appended within the existing `register()` body after `faker.timezone`. No package-boundary violations. |
| Correctness | PASS | `catchPhrase` noun pool uses hyphenated single-tokens (e.g. `"core-competencies"`, `"best-practices"`) — no internal spaces that would break the 3-part split validator. Seed determinism is correct: three `intn` calls in fixed source order produce stable output under `--seed`. |
| Test Quality | PASS | All 8 behaviors are covered. Table-driven `TestRegistry_FakerCompany`. Separate seeded (`TestRegistry_FakerCompany_Seeded`) and unseeded (`TestRegistry_FakerCompany_Unseeded`) tests. Shape test (`TestRegistry_FakerCompany_CatchPhraseShape`) verifies pool membership of all three tokens under seed 42. Pool integrity tests (`TestFakerCompany_PoolsNonEmpty`, `TestFakerCompany_SuffixSet`) guard against drift. Locale deferred via `t.Skip` stub. Existing "not-yet-registered" tests correctly migrated from `faker.company` to `faker.url`. |

## Test Coverage
- Coverage: 97.1% (`internal/variable` package)
- Missing coverage: none — all new code paths are exercised

## Behavior Coverage

| Behavior | Test(s) |
|----------|---------|
| 1. `$faker.company` non-empty pool draw | `TestRegistry_FakerCompany/company_is_non-empty_pool_draw` |
| 2. `$faker.companySuffix` from canonical set | `TestRegistry_FakerCompany/companySuffix_in_canonical_set`, `TestFakerCompany_SuffixSet` |
| 3. `$faker.jobTitle` non-empty pool draw | `TestRegistry_FakerCompany/jobTitle_is_non-empty_pool_draw` |
| 4. `$faker.department` non-empty pool draw | `TestRegistry_FakerCompany/department_is_non-empty_pool_draw` |
| 5. `$faker.catchPhrase` three-word composition | `TestRegistry_FakerCompany/catchPhrase_is_three_space-separated_parts`, `TestRegistry_FakerCompany_CatchPhraseShape` |
| 6. Seed reproducibility | `TestRegistry_FakerCompany_Seeded` |
| 7. Unseeded entropy | `TestRegistry_FakerCompany_Unseeded` |
| 8. Locale deferred | `TestRegistry_FakerCompany_LocaleDeferred` (t.Skip stub) |

## Summary

This is a clean, additive slice that follows the M13-002 and M13-003 registration patterns exactly. The implementation is correct — the `catchPhrase` noun pool uses hyphenated single-token entries to avoid breaking the 3-part space-split validator, an important correctness decision. The `companyPoolContains` helper is correctly renamed to avoid future name collision. All 8 behaviors have tests, coverage is 97.1%, the pre-audit gate passes cleanly, and `golangci-lint` reports 0 issues.
