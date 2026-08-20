# Code Review: M28-003

**Task:** The usage synopsis, and seven test stubs whose reason expired
**Reviewer:** AI
**Date:** 2026-08-20
**Branch:** feature/M28-003-usage-synopsis-and-expired-stubs
**Iteration:** 2 (re-review after `/improve`)

## Verdict: PASS

## Findings

None.

Iteration 1 reported one Low-severity finding: `cmd/curlew/help_parity_test.go`'s
`flagSurface` doc comment pointed to `docs/PRODUCT_ROADMAP.md` for detail on the
deferred watch-synopsis follow-up (twelve missing flags), but that file never
mentioned it. Commit `bfe946a` repointed the comment at `CHANGELOG.md` (where
the note actually lives, in the M28-003 `### Fixed` entry) and inlined the
flag count so the pointer only needs to carry the *why*. Verified independently
this pass:

- `git show bfe946a` — the comment now reads: `"the watch synopsis (main.go's
  watchCmdOut) is the known next candidate, omitting twelve flags parseRunArgs
  accepts; see the run usage synopsis entry in CHANGELOG.md for why it was left
  out of scope."`
- `grep -n "watch" CHANGELOG.md` confirms line 25 carries the referenced
  sentence: `"A third surface — curlew watch's own inline synopsis, which omits
  twelve flags — was found and is deliberately left alone; the task scoped
  other help surfaces out."` The pointer now resolves to real content.
- `grep -rn "PRODUCT_ROADMAP.md" cmd/curlew/help_parity_test.go
  internal/variable/` — no hits. No stale roadmap pointer remains anywhere in
  the changed files.
- Re-verified the underlying "twelve flags" claim against source, not just
  against the prior review's word: `watchCmdOut`'s inline synopsis
  (`cmd/curlew/main.go:1516`) lists `--env, --var, --format, --clear, --color,
  --no-color, -v, -vv, -q` — of `parseRunArgs`'s 20 accepted tokens, 8 appear
  (`--clear` is watch-only). The 12 absent are exactly
  `--allow-sensitive, --confirm-large-dataset, --dry-run, --env-var, --events,
  --locale, --only, --parallel, --quiet, --report, --seed,
  --show-dependencies`. The number in the fixed comment is correct, not just
  copied forward.

No new findings surfaced on this pass. Full re-audit performed rather than a
narrow check of the one prior finding (see below).

Severity levels:
- **Critical**: Will cause bugs, data loss, or security issues
- **High**: Violates project standards, will cause problems
- **Medium**: Code quality issue, should fix
- **Low**: Style/convention, minor improvement

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new production error paths. Test helpers use `t.Fatalf`/`t.Errorf` correctly; `walkFlagLiterals` fails loudly (never silently) when a named function is missing or unquoting fails. |
| Input Validation | PASS | `acceptedArgTokensIn` floor-guards at `< 10` tokens; `walkFlagLiterals` `t.Fatal`s on a renamed/missing target function. No silent empty-pass path anywhere in the new machinery. |
| Naming | PASS | No stuttering (`flagSurface`, not `FlagSurfaceStruct`); unexported symbols scoped correctly; every new function/type/var carries a doc comment (`walkFlagLiterals`, `acceptedArgTokensIn`, `flagSurface`, `assertSurfaceDocumentsFlags`, `runParseErrorStderr`, `runSynopsisExceptions`, `localeAwareFuncs`, `fakerFuncNames`, `evalAt`). |
| Code Organization | PASS | `acceptedFlags`'s signature preserved exactly (confirmed `doc_prose_test.go` still calls it unchanged); no orphaned imports after the seven-stub deletion (`gofmt -l`, `go vet`, `golangci-lint` all clean); locale-neutrality coverage placed in a new file matching the package's existing `locale*_test.go` grouping rather than bloating the 5400-line `dynamic_test.go` further. |
| Correctness | PASS | Re-verified independently, not taken from the prior review or the plan: (1) reproduced the mutation-guard proof myself — inserted `case "--zzz-mutation-probe": f.dryRun = f.dryRun` into `parseRunArgs`, confirmed both `TestUsage_synopsis_lists_every_accepted_flag` and `TestHelp_documents_every_accepted_flag` fail naming exactly `--zzz-mutation-probe`, reverted, confirmed `git diff --stat` empty and both tests green again; (2) traced `runParseErrorStderr`'s "no side effects" claim myself through `runWithWriters` → `runCmdWithWriters` → `runCmdInner`'s early return at the `parseErr != nil` branch — confirmed no file I/O runs before that return; (3) confirmed no unconditional `t.Skip` remains in `dynamic_test.go` (`grep -n "t.Skip(" internal/variable/dynamic_test.go` returns nothing). |
| Test Quality | PASS | RED/GREEN/mutation sequence reproduced live this pass. `TestFaker_localeNeutralFunctionsIgnoreLocale` and `TestFaker_localeAwareFunctionsVaryByLocale` are true converses of each other (one would rot silently without the other, and both explicitly `t.Fatal` on an empty result set rather than passing vacuously). Sub-tests are named per flag/function; assertions are exact-value, not just `err != nil`. |

## Test Coverage
- `cmd/curlew`: 81.1–81.2% (`go test -coverprofile`, reproduced this pass)
- `internal/variable`: 97.6–97.7% (`go test -coverprofile`, reproduced this pass)
- Missing coverage: none introduced by this change; both figures match the packages' pre-existing baselines and the prior review's measurement.

## Verification performed (reproduced independently this pass)

- `./scripts/ci-local.sh --go` — full pass: build, test, test -race, coverage, lint, smoke, and the dogfood/gaps/redaction/openapi/crosscheck/ledger harnesses all green.
- `go test ./cmd/curlew/ -run TestUsage_synopsis_lists_every_accepted_flag -v` — all 20 sub-tests PASS.
- `go test ./internal/variable/ -run LocaleDeferred -v` — `testing: warning: no tests to run`, exit 0 (nothing skips on the expired reason).
- `go test ./internal/variable/ -run 'TestFaker_localeNeutralFunctionsIgnoreLocale|TestFaker_localeAwareFunctionsVaryByLocale' -v` — all sub-tests PASS.
- Mutation proof, reproduced live end-to-end (not re-read from the improvement report): probe inserted → both parity tests fail naming `--zzz-mutation-probe` → probe reverted → `git diff --stat` empty, both tests green.
- `go build ./cmd/curlew && go vet ./cmd/curlew/... ./internal/variable/...` — clean.
- `go test -race ./cmd/curlew/... ./internal/variable/...` — PASS, no data races.
- `~/go/bin/golangci-lint run ./cmd/curlew/... ./internal/variable/...` — 0 issues.
- `gofmt -l` on all four touched Go files — no output.
- `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v` — PASS; 244 tasks, only M28-003 (review) and M29-001 (backlog) open, consistent with `management/backlog.yaml` and `management/tasks/M28-003.yaml` both showing `status: review`.
- Diff-read every changed file in full: `cmd/curlew/help_parity_test.go`, `cmd/curlew/main.go`, `internal/variable/dynamic_test.go`, `internal/variable/locale_neutrality_test.go`, `CHANGELOG.md`, `management/backlog.yaml`, `management/tasks/M28-003.yaml`.

## Summary
The prior Low-severity finding (a dangling `docs/PRODUCT_ROADMAP.md` pointer in a doc comment) is fixed: commit `bfe946a` repoints it at `CHANGELOG.md`, and the destination sentence exists exactly where the comment now says it does. A full re-audit — not just a check of the one prior finding — turned up nothing new: the mutation guard was reproduced live and fires on exactly the right flag, all seven expired `t.Skip` stubs are gone with no unconditional skip left in the file, the locale-neutral/locale-aware test pair is a genuine converse pair rather than one-sided coverage, and `ci-local.sh --go` plus race/vet/lint/gofmt all pass cleanly. Zero findings.
