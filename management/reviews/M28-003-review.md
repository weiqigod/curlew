# Code Review: M28-003

**Task:** The usage synopsis, and seven test stubs whose reason expired
**Reviewer:** AI
**Date:** 2026-08-20
**Branch:** feature/M28-003-usage-synopsis-and-expired-stubs

## Verdict: FAIL

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Low | Documentation Accuracy | `cmd/curlew/help_parity_test.go` | 218 | The `flagSurface` doc comment says "the watch synopsis (main.go's watchCmdOut) is the known next candidate; see docs/PRODUCT_ROADMAP.md" — but `docs/PRODUCT_ROADMAP.md` contains no mention of the watch synopsis, `watchCmdOut`, or the twelve missing flags anywhere in the file (verified: `grep -n "watch\|twelve\|watchCmdOut" docs/PRODUCT_ROADMAP.md` returns nothing). A reader following the pointer for more context finds none. The actual note lives in `CHANGELOG.md`'s M28-003 entry, not the roadmap. | Point the comment at `CHANGELOG.md` (where the twelve-flag finding is actually recorded), or drop the file pointer and let the comment stand on its own, or add a corresponding line to `docs/PRODUCT_ROADMAP.md`'s M28-003 bullet. |

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new production error paths; test helpers correctly use `t.Fatalf`/`t.Errorf`, never swallow. |
| Input Validation | PASS | `walkFlagLiterals` fails loudly (`t.Fatalf`) when a named function is missing or the token floor isn't met — no silent empty-pass path. |
| Naming | PASS | No stuttering; unexported test/package symbols named per Effective Go; all new functions and types carry doc comments. |
| Code Organization | PASS | Clean extraction (`acceptedFlags` signature preserved for `doc_prose_test.go`); no orphaned imports after the seven stub deletions; no circular deps. |
| Correctness | PASS | Verified independently: mutation-tested the guard myself (inserted `--zzz-mutation-probe` into `parseRunArgs`, confirmed both parity tests fail naming it, reverted, confirmed green); confirmed `NewRegistry` performs no RNG draws during construction so the byte-identity assertion is sound; cross-checked the 8-function `localeAwareFuncs` set against every `r.locale.*` read site in `dynamic.go` — exact match; confirmed `runParseErrorStderr`'s "no side effects" claim by tracing `runWithWriters` → `runCmdWithWriters` → `runCmdInner`'s early return on `parseErr != nil`. |
| Test Quality | PASS | RED/GREEN/mutation sequence all reproduced live; sub-tests named per flag/function; assertions are specific (exact string/byte comparison with descriptive failure messages); locale-neutral and locale-aware sets are mutually exhaustive and were confirmed against the actual `dynamic.go` source rather than trusted from the plan. |

## Test Coverage
- `cmd/curlew`: 81.1% (go test -coverprofile)
- `internal/variable`: 97.7% (go test -coverprofile)
- Missing coverage: none introduced by this change; both figures are consistent with the packages' existing baselines.

## Verification performed (reproduced independently, not taken on faith)

- `./scripts/ci-local.sh --go` — full pass (build, test, race, lint, smoke, dogfood/gaps/redaction/openapi/crosscheck/ledger harnesses).
- `go test ./cmd/curlew/ -run TestUsage_synopsis_lists_every_accepted_flag -v` — all 20 sub-tests (`--allow-sensitive`, `--color`, `--confirm-large-dataset`, `--dry-run`, `--env`, `--env-var`, `--events`, `--format`, `--locale`, `--no-color`, `--only`, `--parallel`, `--quiet`, `--report`, `--seed`, `--show-dependencies`, `--var`, `-q`, `-v`, `-vv`) PASS.
- `go test ./internal/variable/ -run LocaleDeferred -v` — `testing: warning: no tests to run`, exit 0.
- `go test ./internal/variable/ -run 'TestFaker_localeNeutralFunctionsIgnoreLocale|TestFaker_localeAwareFunctionsVaryByLocale' -v` — all sub-tests PASS (33 neutral functions × 15 locales; 8 aware functions × 3 candidate locales × 5 seeds).
- Mutation proof (DoD item), reproduced live: inserted `case "--zzz-mutation-probe": f.dryRun = f.dryRun` into `parseRunArgs`; both `TestUsage_synopsis_lists_every_accepted_flag` and `TestHelp_documents_every_accepted_flag` failed, each naming `--zzz-mutation-probe` with the correct fix-hint message; reverted the probe; both tests passed again; `git status`/`git diff` confirmed the working tree matched the committed `main.go` exactly after revert.
- `go test -race ./cmd/curlew/... ./internal/variable/...` — PASS, no data races.
- `go vet ./cmd/curlew/... ./internal/variable/...` — clean.
- `~/go/bin/golangci-lint run ./cmd/curlew/... ./internal/variable/...` — 0 issues.
- `gofmt -l` on all four touched Go files — no output (all formatted).
- Cross-checked the plan's "twelve flags missing from the watch synopsis" claim directly against source: watch's inline synopsis lists 8 of `parseRunArgs`'s 20 tokens plus its own `--clear`; the 12 absent are exactly `--allow-sensitive`, `--show-dependencies`, `--dry-run`, `--parallel`, `--confirm-large-dataset`, `--quiet`, `--env-var`, `--seed`, `--report`, `--events`, `--only`, `--locale`.

## Summary
The implementation is careful and unusually well-verified: the AST-walk refactor is clean, the mutation guard genuinely fires on the exact flag added (reproduced independently, not taken from the plan's transcript), the locale-neutral/locale-aware classification was checked against the real `dynamic.go` source rather than trusted, and `ci-local.sh --go` plus race/vet/lint all pass cleanly. The single finding is a Low-severity dangling documentation pointer — a code comment sends the reader to `docs/PRODUCT_ROADMAP.md` for detail on the deferred watch-synopsis follow-up that isn't actually there (it's in `CHANGELOG.md` instead). Per this project's own standard for doc-drift issues, and since the All-or-Nothing review policy treats any finding as a fail, this is reported and blocks a clean PASS despite the substance of the change being sound.
