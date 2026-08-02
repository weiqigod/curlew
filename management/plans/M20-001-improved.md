# Improvement Report: M20-001

**Task:** --locale flag, config precedence + fallback chain, ERR_LOCALE_UNKNOWN, shipped end-to-end with de-DE
**Date:** 2026-06-12
**Review:** management/reviews/M20-001-review.md

## Resolved Findings (Iteration 1)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | No integration test covers the exec subcommand locale path end-to-end (`TestExec_Locale_DeDE_DrawsGermanName` and `TestExecCmd_Locale_UnknownCode` were specified in the plan but not implemented) | Added `TestExecCmd_Locale_DeDE_DrawsGermanName` (uses `captureExecCmd` + `--seed 42 --locale de-DE --dry-run --format json`; asserts URL is resolved to a German name, not an en-US surname, and token `$faker.fullName` is not left raw) and `TestExecCmd_Locale_UnknownCode_ErrLocaleUnknown` (asserts non-zero exit and "Supported locales" in stderr) | ✓ tests pass |
| 2 | Medium | `hints_init.go` line 103 hardcodes the 15-locale list as a string literal, duplicating `supportedLocales` in `locale.go` | Replaced the literal with `strings.Join(supportedLocales, ", ")` (added `"strings"` import) | ✓ tests pass |
| 3 | Medium | `TestRun_LocalePrecedence_FlagBeatsProject` tests CLI flag beating **collection** `config.locale`, not project config (no `apitest.yaml` present); name and comment are misleading | Renamed to `TestRun_LocalePrecedence_FlagBeatsCollection`; added separate `TestRun_LocalePrecedence_FlagBeatsProjectConfig` that creates a temp `apitest.yaml` with `config.locale: en-US` alongside the collection and verifies the flag wins | ✓ tests pass |
| 4 | Low | Verbose "resolved locale" diagnostic uses `flags.locale` (raw, un-normalized value); `--locale de_DE` would display as `de_DE` not `de-DE` | Added `NormalizeLocale` exported function to `internal/variable/dynamic.go`; updated `localeSource` in `cmd/apitest/main.go` to call `variable.NormalizeLocale(flags.locale)` before emitting the diagnostic | ✓ tests pass |
| 5 | Low | `TestParseRunArgs_Locale` table case `"locale missing value"` passes `["--locale", "file.yaml"]`; `"file.yaml"` is consumed as locale value and the error is "missing collection file path" — not the `--locale requires a value` code path the name implies | Removed the misleading case; added a comment pointing to `TestParseRunArgs_LocaleMissingValue` which already covers the actual missing-value error path | ✓ tests pass |

## Resolved Findings (Iteration 2)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Behavior 7 ("collection config honored when flag absent") had no integration-level test. `TestResolveLocale_Precedence` verifies the precedence function at unit level, but no binary-level test ran a collection with `config:\n  locale: de-DE` absent a `--locale` flag and asserted faker output from the de-DE pool. | Added `TestRun_CollectionLocale_HonoredWhenFlagAbsent` to `cmd/apitest/run_test.go`: spins up an httptest server that captures the request path, writes a collection with `config.locale: de-DE` and `{{$faker.fullName}}` in the URL, runs with `--seed 42 -v` (no `--locale` flag), and asserts: exit 0; verbose diagnostic names "collection config" as source; captured URL path contains no en-US-only surname. | ✓ tests pass |

## Resolved Findings (Iteration 3)

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | `run` subcommand emitted the locale fallback warning unconditionally to stderr; `vars.Diagnostics` is always wired to `stderr`, so the warning fired even without `-v`, violating Behaviour 5 and Observable #5. `exec` correctly gates on `opts.Verbosity >= VerbosityVerbose`. | Added `LocaleVerbose bool` to `runner.VarSources`. In `runner.go`, changed `WithLocaleWarning` gate from `vars.Diagnostics != nil` to `vars.Diagnostics != nil && vars.LocaleVerbose`. In `main.go`, set `LocaleVerbose: verbosity >= output.VerbosityVerbose` at the `runner.Run` call site. | ✓ tests pass |
| 2 | Medium | No CLI-layer integration test for Behaviour 5's verbosity gating (fallback warning absent without `-v`, present with `-v`). | Added `TestRun_Locale_FallbackChainEnGB` to `cmd/apitest/run_test.go` with two subtests: `warning_absent_without_-v` and `warning_present_with_-v`. | ✓ tests pass |
| 3 | Low | No test asserting `--locale` appears in help text (Behaviour 8). A future refactor of `printHelpTo` could silently drop the flag from help output. | Added `TestHelpText_ContainsLocaleFlag` to `cmd/apitest/main_test.go` asserting `--locale` appears at least twice in `printHelpTo` output (once under Exec Options, once under Run Options). | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS (55 packages) |
| `golangci-lint run` | PASS (0 issues) |
| Coverage `internal/variable` | 97.2% |
| Coverage `internal/config` | 90.5% |
| Coverage `internal/runner` | 84.8% |
| Coverage `cmd/apitest` | 79.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| `b662e891` | test(cmd): add exec locale integration tests | Iter1 #1 |
| `62bbea4e` | fix(variable): eliminate hardcoded locale list in hints_init.go | Iter1 #2 |
| `8bb2129e` | test(cmd): rename and expand locale precedence tests | Iter1 #3 |
| `90d0b835` | fix(cmd): normalize locale in verbose diagnostic line | Iter1 #4 |
| `862808df` | test(cmd): remove misleading locale-missing-value table case | Iter1 #5 |
| `7f29d9a8` | test(run): add TestRun_CollectionLocale_HonoredWhenFlagAbsent | Iter2 #1 |
| `2d178414` | fix(runner): gate locale fallback warning on LocaleVerbose in run path | Iter3 #1 |
| `cee1b8fb` | test(run): add TestRun_Locale_FallbackChainEnGB for verbosity gating | Iter3 #2 |
| `822dcdfe` | test(main): add TestHelpText_ContainsLocaleFlag for Behaviour 8 coverage | Iter3 #3 |

## Summary
9/9 findings resolved across 3 iterations. 0 deferred.
