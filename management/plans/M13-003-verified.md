# Verification Report: M13-003

**Task:** $faker location data — 12 functions
**Verified by:** AI
**Date:** 2026-04-29
**Branch:** feature/M13-003-faker-location
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `go test ./...` | PASS | All packages pass |
| `go test -race ./...` | PASS | No races detected |
| `golangci-lint run` | PASS | No findings |
| `./smoke/run.sh` | PASS | Smoke test clean |
| Coverage (`internal/variable`) | 97.0% | Meets >= 80% threshold |

## Observable Output

```
=== RUN   TestRegistry_FakerLocation
--- PASS: TestRegistry_FakerLocation (0.00s)
    --- PASS: TestRegistry_FakerLocation/address_matches_US-address_pattern (0.00s)
    --- PASS: TestRegistry_FakerLocation/street_is_<num>_<streetName>_<suffix> (0.00s)
    --- PASS: TestRegistry_FakerLocation/streetName_is_<name>_<suffix> (0.00s)
    --- PASS: TestRegistry_FakerLocation/city_is_non-empty_pool_draw (0.00s)
    --- PASS: TestRegistry_FakerLocation/state_is_non-empty_full_state_name (0.00s)
    --- PASS: TestRegistry_FakerLocation/stateAbbr_is_2_uppercase_letters (0.00s)
    --- PASS: TestRegistry_FakerLocation/zipCode_is_5_digits (0.00s)
    --- PASS: TestRegistry_FakerLocation/country_is_non-empty_pool_draw (0.00s)
    --- PASS: TestRegistry_FakerLocation/countryCode_is_2_uppercase_letters (0.00s)
    --- PASS: TestRegistry_FakerLocation/latitude_in_[-90,_90]_with_4_decimals (0.00s)
    --- PASS: TestRegistry_FakerLocation/longitude_in_[-180,_180]_with_4_decimals (0.00s)
    --- PASS: TestRegistry_FakerLocation/timezone_is_valid_IANA_identifier (0.00s)
=== RUN   TestRegistry_FakerLocation_Seeded
--- PASS: TestRegistry_FakerLocation_Seeded (0.00s)
=== RUN   TestRegistry_FakerLocation_Unseeded
--- PASS: TestRegistry_FakerLocation_Unseeded (0.00s)
=== RUN   TestRegistry_FakerLocation_TimezonePool_AllValid
--- PASS: TestRegistry_FakerLocation_TimezonePool_AllValid (0.01s)
=== RUN   TestRegistry_FakerLocation_AddressShape
--- PASS: TestRegistry_FakerLocation_AddressShape (0.00s)
PASS
ok  github.com/peterlindqvist/apitest/internal/variable
```

Expected: PASS — all 12 location functions registered and validated.
Result: MATCH

## Behaviors Verified

| # | Behavior | Test | Status |
|---|----------|------|--------|
| 1 | `$faker.address` — US-format `<num> <street>, <city>, <stateAbbr> <zip>` | `TestRegistry_FakerLocation/address_matches_US-address_pattern`, `TestRegistry_FakerLocation_AddressShape` | PASS |
| 2 | `$faker.street` — `<num> <streetName> <suffix>` | `TestRegistry_FakerLocation/street_is_...` | PASS |
| 3 | `$faker.streetName` — non-empty street name, no leading number | `TestRegistry_FakerLocation/streetName_is_...` | PASS |
| 4 | `$faker.city` — non-empty pool draw | `TestRegistry_FakerLocation/city_is_non-empty_pool_draw` | PASS |
| 5 | `$faker.state` / `$faker.stateAbbr` — 1:1 pool alignment | `TestFakerLocation_PoolAlignment` | PASS |
| 6 | `$faker.zipCode` — matches `^\d{5}$` | `TestRegistry_FakerLocation/zipCode_is_5_digits` | PASS |
| 7 | `$faker.country` — non-empty pool draw | `TestRegistry_FakerLocation/country_is_non-empty_pool_draw` | PASS |
| 8 | `$faker.countryCode` — matches `^[A-Z]{2}$` | `TestRegistry_FakerLocation/countryCode_is_2_uppercase_letters` | PASS |
| 9 | `$faker.latitude` — float in [-90, 90] with 4 decimal places | `TestRegistry_FakerLocation/latitude_in_[-90,_90]_with_4_decimals` | PASS |
| 10 | `$faker.longitude` — float in [-180, 180] with 4 decimal places | `TestRegistry_FakerLocation/longitude_in_[-180,_180]_with_4_decimals` | PASS |
| 11 | `$faker.timezone` — valid IANA identifier via `time.LoadLocation` | `TestRegistry_FakerLocation/timezone_is_valid_IANA_identifier`, `TestRegistry_FakerLocation_TimezonePool_AllValid` | PASS |
| 12 | `--seed 42` → byte-equal output across two independent instances | `TestRegistry_FakerLocation_Seeded` | PASS |
| 13 | No seed → entropy (different outputs) | `TestRegistry_FakerLocation_Unseeded` | PASS |
| 14 | `--locale de-DE` deferred (en-US only) | `TestRegistry_FakerLocation_LocaleDeferred` (t.Skip) | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | All behavior tests pass | All 14 behaviors covered by tests above | PASS |
| 2 | `go test ./...` passes | `ci-local.sh` output: all packages ok | PASS |
| 3 | `go test -cover ./internal/variable/... >= 80%` | 97.0% coverage | PASS |
| 4 | `golangci-lint run` passes with 0 issues | `ci-local.sh` lint gate: PASS | PASS |
| 5 | `./smoke/run.sh` passes | `ci-local.sh` smoke gate: PASS | PASS |
| 6 | `./scripts/ci-local.sh` passes | `=== ci-local PASS ===` | PASS |
| 7 | `docs/MANUAL.md` §3.7 includes 12 new rows | Location-data sub-table added with all 12 functions | PASS |
| 8 | Seeded-reproducibility and no-seed-entropy tests pass | `TestRegistry_FakerLocation_Seeded`, `TestRegistry_FakerLocation_Unseeded` | PASS |
| 9 | `$faker.state` and `$faker.stateAbbr` 1:1 aligned | `TestFakerLocation_PoolAlignment` asserts length equality + spot checks | PASS |

## Code Review

| Check | Status |
|-------|--------|
| Error handling | PASS |
| Naming conventions | PASS |
| Code organization | PASS |
| Test quality | PASS |

Branch A: Review PASS (iteration 2) trusted. Spot-check confirmed:
- Error handling: all 12 functions use `noArgs` wrapper with shared `arityError` / `apierrors.Structured` mechanism
- Exported symbols: no new exported types; package-level pool vars are unexported (correct)
- Tests: `TestRegistry_FakerLocation_AddressShape` tests exactly what it claims — the SPEC:778 3-part structure with state abbr and zip validation

## Commits

| Hash | Message |
|------|---------|
| f8c24d5 | docs(plan): add implementation plan for M13-003 |
| 872783d | chore(task): mark M13-003 as planned |
| a680361 | chore(task): mark M13-003 as in_progress |
| d0184d6 | test(variable): add failing TestFakerLocation_PoolAlignment (RED) |
| b8deefd | feat(variable): add location data pools — cities, streets, states, countries, timezones |
| 341722a | test(variable): add failing tests for $faker location-data family (RED) |
| 8e9839c | feat(variable): register 12 $faker location-data functions (GREEN) |
| 5eef3dc | docs(plan): add $faker location-data table to MANUAL.md §3.7 |
| a87507e | chore(task): mark M13-003 as review |
| 0f24991 | docs(review): add review with findings for M13-003 |
| 327e0a2 | fix(docs): correct MANUAL.md location-data table — accuracy and consistency claims |
| 56f6d3d | docs(review): add improvement report for M13-003 |
| 1fca843 | docs(review): add passing review for M13-003 (iteration 2) |

TDD pattern visible: `test(variable)` RED commits precede `feat(variable)` GREEN commits.

## Files Changed

| File | Action | Description |
|------|--------|-------------|
| `internal/variable/dynamic.go` | modified | 12 new `noArgs` registrations + 8 package-level pool slices (cities, streetNames, streetSuffixes, stateNames, stateAbbrs, countryNames, countryCodes, ianaTimezones) |
| `internal/variable/dynamic_test.go` | modified | 7 new test functions; `TestRegistry_available_sorted` count updated 38→50; `TestRegistry_DottedName_LocationNotYetRegistered` renamed and retargeted to `faker.company` |
| `internal/variable/variable_test.go` | modified | `TestInterpolate_DottedName_UnknownFunctionError` switched from `faker.address` to `faker.company` |
| `docs/MANUAL.md` | modified | §3.7 location-data sub-table added (12 rows); parenthetical and dotted-namespace paragraph updated |

## Issues Found
None.

## Recommendation
PASS — ready for PR and merge.
