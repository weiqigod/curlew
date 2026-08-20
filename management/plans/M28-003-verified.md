# Verification Report: M28-003

**Task:** The usage synopsis, and seven test stubs whose reason expired
**Verified by:** AI
**Date:** 2026-08-20
**Branch:** feature/M28-003-usage-synopsis-and-expired-stubs
**Verdict:** PASS

## Test Results

| Suite | Result | Details |
|-------|--------|---------|
| `./scripts/ci-local.sh` | PASS | Auto-scoped to `go=1` (only Go + management files changed); build, backlog integrity, fuzz corpora, repo-layout/front-door/README-quickstart guards, `go test`, `go test -race`, coverage, `golangci-lint`, telemetry/signing-key guards, smoke suite, release/goreleaser checks, and the mudflat dogfood/gaps/redaction/openapi/crosscheck/ledger harnesses — all green, ending `=== ci-local PASS ===` |
| `go build ./cmd/curlew` | PASS | Clean build, no warnings |
| `go vet ./cmd/curlew/... ./internal/variable/...` | PASS | Clean |
| `gofmt -l` on all 4 changed Go files | PASS | No output (already formatted) |
| Coverage `cmd/curlew` | 81.1% | Meets >= 80% threshold |
| Coverage `internal/variable` | 97.7% | Meets >= 80% threshold |

## Observable Output

**Observable 1** — `go test ./cmd/curlew/ -run TestUsage_synopsis_lists_every_accepted_flag -v`: all 20 sub-tests (one per accepted flag/token: `--allow-sensitive`, `--color`, `--confirm-large-dataset`, `--dry-run`, `--env`, `--env-var`, `--events`, `--format`, `--locale`, `--no-color`, `--only`, `--parallel`, `--quiet`, `--report`, `--seed`, `--show-dependencies`, `--var`, `-q`, `-v`, `-vv`) PASS.

**Observable 2** — `go test ./internal/variable/ -run LocaleDeferred -v`: `testing: warning: no tests to run`, exit 0 — no test matches that name pattern any more, confirming no skip remains under the old reason.

**Manual reproduction of the user-facing scenario**, built binary and ran directly:
```
$ ./curlew run foo.yaml --bogus
Usage: curlew run <collection-file> [--env <name>] [--env-var VAR ...] [--var key=value ...] [--seed <number>] [--locale <code>] [--format <type>] [--report <file>] [--events <file>] [--only "<name>"] [--show-dependencies] [--dry-run] [--parallel] [--confirm-large-dataset] [--allow-sensitive] [--color <when>] [--no-color] [-v] [-vv] [-q|--quiet]
[ERROR] unknown flag: --bogus
EXIT=1
```
Expected: synopsis lists `--allow-sensitive`, `--events`, `--locale`, and `--quiet` (the four flags found missing during planning, one more than the task's original three). Result: MATCH.

## Behaviors Verified

| # | Behavior | Test(s) | Status |
|---|----------|---------|--------|
| 1 | Given every flag the run parser accepts, when the usage synopsis is printed, then each appears in it. | `TestUsage_synopsis_lists_every_accepted_flag` | PASS |
| 2 | Given a flag added to the run parser without a synopsis entry, when the test suite runs, then it fails. | Reproduced live: inserted `case "--zzz-mutation-probe":` into `parseRunArgs` (`cmd/curlew/main.go`) — both `TestUsage_synopsis_lists_every_accepted_flag` and `TestHelp_documents_every_accepted_flag` failed, each naming `--zzz-mutation-probe` exactly. Reverted; `git diff --stat` confirmed empty and both tests green again. | PASS |
| 3 | Given a test that skips unconditionally, when the suite runs, then either the skip reason still holds or the test is removed. | `grep -c "t.Skip(" internal/variable/dynamic_test.go` → 0. Replacement coverage: `TestFaker_localeNeutralFunctionsIgnoreLocale` and `TestFaker_localeAwareFunctionsVaryByLocale` in new `internal/variable/locale_neutrality_test.go`, both run and PASS (all sub-tests). | PASS |

## Definition of Done

| # | Item | Evidence | Status |
|---|------|----------|--------|
| 1 | The run usage synopsis asserted to list every flag the parser accepts | `TestUsage_synopsis_lists_every_accepted_flag` covers all 20 tokens `acceptedArgTokensIn` extracts from `parseRunArgs` via AST walk | PASS |
| 2 | The M23-001 parity test extended rather than duplicated | `help_parity_test.go`: `assertSurfaceDocumentsFlags` is shared by both `helpSurface` (existing M23-001 test) and the new `runSynopsisSurface`; `walkFlagLiterals`/`acceptedArgTokensIn` are the shared AST-walk machinery | PASS |
| 3 | Guard verified by mutation — add a flag, confirm failure, restore | Reproduced independently this pass (see Behavior 2 above); `git diff --stat` empty after restore | PASS |
| 4 | All seven `LocaleDeferred` stubs either deleted or replaced by real tests | `grep -c "t.Skip(" internal/variable/dynamic_test.go` → 0; six replaced by `locale_neutrality_test.go`'s two converse tests, one (personal-data) deleted as already covered 5x by existing locale tests per the plan's finding #2 | PASS |
| 5 | No unconditional `t.Skip` remains whose stated reason is untrue | Confirmed no `t.Skip(` in `dynamic_test.go`; plan documented the two remaining guarded skips elsewhere in the repo (schema/redaction) are conditional, not in scope | PASS |
| 6 | `./scripts/ci-local.sh --go` passes | Ran `./scripts/ci-local.sh` (auto-scoped to go-only since no backend/web/stack files changed) — PASS, ending `=== ci-local PASS ===` | PASS |
| 7 | CHANGELOG.md updated | `CHANGELOG.md` `[Unreleased]` → `### Fixed` carries two detailed bullets (synopsis fix, stub deletion), added in commit `b590f89` | PASS |

## Code Review

Branch A: review PASS exists (`management/reviews/M28-003-review.md`, iteration 2, zero findings). Trusted, with independent spot-checks performed this pass rather than accepted at face value:

| Check | Status | Note |
|-------|--------|------|
| Parity-test extension (not duplication) | PASS | Read `help_parity_test.go` directly — `assertSurfaceDocumentsFlags(t, s flagSurface)` is one function driving both `helpSurface` and `runSynopsisSurface` |
| Mutation guard | PASS | Reproduced live end-to-end this pass, not taken from the review's word (see Behavior 2) |
| Locale replacement tests are genuine converses | PASS | Ran both `TestFaker_localeNeutralFunctionsIgnoreLocale` and `TestFaker_localeAwareFunctionsVaryByLocale` directly — all sub-tests PASS, exercising distinct function families (neutral vs. aware) |
| `gofmt` / `go vet` clean on all 4 changed files | PASS | Reproduced this pass |
| No stray diff after mutation-probe edit/revert cycle | PASS | `git diff --stat` empty |

## Commits

14 commits on the branch, all with `Refs: M28-003` trailers and conventional `type(scope): description` subjects. TDD sequence visible: `test(cli): add failing test for run synopsis flag parity` (0f8d9c5) precedes `feat(cli): list every accepted flag in the run usage synopsis` (a360099); `test(variable): cover the per-family locale-neutrality docs promise` (9c1e6f3) precedes `fix(variable): delete seven t.Skip stubs whose reason expired` (e407314).

| Hash | Message |
|------|---------|
| `147a253` | docs(plan): add implementation plan for M28-003 |
| `3b7ef48` | chore(task): mark M28-003 as planned |
| `253dcbd` | chore(task): mark M28-003 as in_progress |
| `61703a1` | refactor(cli): hoist run usage synopsis to a named const |
| `0f8d9c5` | test(cli): add failing test for run synopsis flag parity |
| `a360099` | feat(cli): list every accepted flag in the run usage synopsis |
| `9c1e6f3` | test(variable): cover the per-family locale-neutrality docs promise |
| `e407314` | fix(variable): delete seven t.Skip stubs whose reason expired |
| `b590f89` | docs(changelog): record the synopsis fix and stub deletion |
| `8b53d95` | chore(task): mark M28-003 as review |
| `6be371a` | docs(review): add review with findings for M28-003 |
| `bfe946a` | fix(cmd/curlew): point flagSurface comment at where the note lives |
| `dda13ec` | docs(review): add improvement report for M28-003 |
| `8341bbd` | docs(review): add passing review for M28-003 |

## Files Changed

| File | Action | Lines +/- |
|------|--------|-----------|
| `cmd/curlew/help_parity_test.go` | modified | +252/-... (net, extends M23-001 parity machinery to a second surface) |
| `cmd/curlew/main.go` | modified | +13/-... (hoists synopsis to `runUsageSynopsis` const, adds 4 missing flags) |
| `internal/variable/dynamic_test.go` | modified | -58 net (deletes 7 `t.Skip` stubs) |
| `internal/variable/locale_neutrality_test.go` | added | +138 (new file: locale-neutral vs. locale-aware converse tests) |
| `CHANGELOG.md` | modified | +32 |

Total: 5 files changed, 393 insertions(+), 100 deletions(-).

## Issues Found

None. One observation, not a defect: the CHANGELOG's two M28-003 bullets do not inline `(M28-003)` the way roughly half of existing entries do (`grep` shows 332 lines across the file citing a task ID inline, but plenty of entries — including this one — rely on the commit's `Refs:` trailer instead). This is pre-existing stylistic inconsistency in the file, not something this task's DoD requires ("CHANGELOG.md updated" — satisfied), and not worth reopening review over.

## Recommendation

PASS — ready for PR and merge.
