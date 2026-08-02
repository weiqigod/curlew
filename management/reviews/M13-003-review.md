# Code Review: M13-003

**Task:** $faker location data — 12 functions
**Reviewer:** AI
**Date:** 2026-04-29
**Branch:** feature/M13-003-faker-location
**Iteration:** 2 (post-improve)

## Verdict: PASS

## Findings

No findings. All issues from iteration 1 have been resolved.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | All 12 new functions use the `noArgs` wrapper which delegates to the shared `arityError` / `apierrors.Structured` mechanism. No new error paths introduced. |
| Input Validation | PASS | Location functions accept no arguments; arity rejection is handled by `noArgs`. Pool access is always `intn(rng, len(pool))` — bounds-safe by construction. |
| Naming | PASS | Function names follow the established `faker.*` convention from M13-002. No new exported types. Pool variable names (`stateNames`, `stateAbbrs`, `countryNames`, `countryCodes`, `ianaTimezones`, `cities`, `streetNames`, `streetSuffixes`) are descriptive and follow project conventions. |
| Code Organization | PASS | Pools added at file scope after existing pools. Registrations appended in `register()` after the SSN block. No circular imports, no new packages. |
| Correctness | PASS | Latitude/longitude mapping `(2v-1)*scale` correctly centres on 0. `strconv.FormatFloat(..., 'f', 4, 64)` produces exactly 4 decimal places. State/country parallel slices are length-equal (50+50, 30+30). Building numbers use `intn(rng, 9999)+1` → [1, 9999] (≤4 digits), consistent with the `\d{1,4}` regex in tests. All timezone pool entries verified loadable by `time.LoadLocation`. |
| Test Quality | PASS | Table-driven test covers all 12 functions. `TestFakerLocation_PoolAlignment` asserts pool length equality plus 3 index-pinned spot checks. `TestRegistry_FakerLocation_Seeded` proves seed determinism. `TestRegistry_FakerLocation_Unseeded` proves entropy. `TestRegistry_FakerLocation_TimezonePool_AllValid` calls `time.LoadLocation` on every entry. `TestRegistry_FakerLocation_AddressShape` verifies the SPEC:778 3-part structure. Existing tests updated correctly (`TestRegistry_available_sorted` count 38→50; dotted-name test renamed and retargeted to `faker.company`). All 14 behaviors covered. |

## Resolved Findings (from iteration 1)

| # | Severity | Finding | Resolution |
|---|----------|---------|------------|
| 1 | Medium | `docs/MANUAL.md` lines 1103–1104: misleading claim that "abbr and city are mutually consistent" in a `$faker.address` call — city and stateAbbr are independent pool draws | Fixed: replaced with accurate wording "`$faker.state` and `$faker.stateAbbr` render the matching full name and abbreviation for the same pool index — e.g. Illinois/IL" |
| 2 | Low | Column header "Example (seed 42)" contained illustrative values, not actual seed=42 outputs | Fixed: column renamed to "Example"; values replaced with verified seed=42 outputs (`8960 Meadow Rd, Manchester, CT 88423`, etc.) |

## Test Coverage
- Coverage: 97.0% (`internal/variable`)
- Missing coverage: None relevant to the new location functions.

## Spec and Behavior Compliance

All 14 behaviors from `management/tasks/M13-003.yaml` are covered by tests:

| Behavior | Test(s) |
|----------|---------|
| 1. `$faker.address` — US-format `<num> <street>, <city>, <stateAbbr> <zip>` | `TestRegistry_FakerLocation`, `TestRegistry_FakerLocation_AddressShape` |
| 2. `$faker.street` — `<num> <streetName> <suffix>` | `TestRegistry_FakerLocation` |
| 3. `$faker.streetName` — non-empty, no leading number | `TestRegistry_FakerLocation` |
| 4. `$faker.city` — non-empty pool draw | `TestRegistry_FakerLocation` |
| 5. State/stateAbbr 1:1 pool alignment | `TestFakerLocation_PoolAlignment` |
| 6. `$faker.zipCode` — `^\d{5}$` | `TestRegistry_FakerLocation` |
| 7. `$faker.country` — non-empty pool draw | `TestRegistry_FakerLocation` |
| 8. `$faker.countryCode` — `^[A-Z]{2}$` | `TestRegistry_FakerLocation` |
| 9. `$faker.latitude` — [-90, 90] 4 decimals | `TestRegistry_FakerLocation` |
| 10. `$faker.longitude` — [-180, 180] 4 decimals | `TestRegistry_FakerLocation` |
| 11. `$faker.timezone` — valid IANA identifier | `TestRegistry_FakerLocation`, `TestRegistry_FakerLocation_TimezonePool_AllValid` |
| 12. `--seed 42` → byte-equal across runs | `TestRegistry_FakerLocation_Seeded` |
| 13. No seed → entropy | `TestRegistry_FakerLocation_Unseeded` |
| 14. `--locale de-DE` deferred | `TestRegistry_FakerLocation_LocaleDeferred` (t.Skip) |

## Gate Output

```
=== ci-local PASS ===
```

Pre-audit gate passed: build, test, race, lint, smoke all green. Coverage 97.0%.

## Summary

All findings from iteration 1 have been resolved: the misleading city/state consistency claim in MANUAL.md was corrected and the example column header was fixed. The implementation is correct and complete: 12 location functions registered with proper pool alignment, deterministic seeding, and comprehensive test coverage across all 14 specified behaviors.
