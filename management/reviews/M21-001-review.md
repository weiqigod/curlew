# Code Review: M21-001

**Task:** Collection/project JSON Schema completeness: 9 parser fields missing, editors flag valid collections
**Reviewer:** AI
**Date:** 2026-08-04
**Branch:** fix/M21-001-schema-completeness

## Verdict: PASS (second pass)

First pass raised four findings; all four are fixed in `99cf9e2`. Finding #4
turned out to be substantive rather than cosmetic — see below.

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS** (exit 0: build, `go test`, race, coverage,
lint, smoke).

## First-pass findings and resolutions

| # | Severity | Finding | Resolution |
|---|----------|---------|-----------|
| 1 | Medium | `TestParser_protocol_hint_lists_only_accepted_values` reduced to "the hint must not contain the word `https`", so any *other* rejected protocol reintroduced into the hint would pass. | Now checks a set of plausible-but-rejected values (`https`, `http2`, `ws`, `wss`, `grpc`, `tcp`, `rest`, `soap`), each skipped automatically if it ever becomes supported. |
| 2 | Low | `collectionSchemaBytes()` / `projectSchemaBytes()` were one-line wrappers over exported package vars. | Removed; call sites use `schema.CollectionSchema` / `schema.ProjectSchema` directly. |
| 3 | Low | `collectionAround` mutated its argument in place *and* returned a wrapper, making the mutation invisible at call sites. | Builds and returns a copy; the caller's map is never touched. |
| 4 | Low → **substantive** | Behaviour 4's HTTP resolution of `$id` was unverifiable and unrecorded. | New `scripts/check-schema-urls.sh`. Running it revealed the behaviour as written **cannot be satisfied**: see below. |

## Finding #4: behaviour 4 was based on a false premise

Behaviour 4 says the `$id` should "return the schema rather than 404 — the org
segment matches the actual remote (`weiqigod`), not `peterlindqvist`". Fixing the
org segment was necessary but **not sufficient**: `gh repo view` reports
`"isPrivate": true`, and `raw.githubusercontent.com` returns 404 for a private
repository regardless of the org. Both URLs 404 today, and would have 404'd with
the old org too — the org was a second, independent defect, not the cause.

This was flagged as a risk in the plan and is now resolved the way the plan
proposed: the `$id` values are still corrected (they are identifiers, and they
become live if the repository is ever made public), but MANUAL §1.5 no longer
offers a URL that does not work. It now tells out-of-tree users to emit the
schema from their own binary (`curlew schema > .curlew/schemas/collection-v1.json`),
which is strictly better advice — the schema they get is the one their installed
binary enforces — and states plainly that the published URL resolves only while
the repository is public.

`scripts/check-schema-urls.sh` makes the claim checkable rather than assumed. It
is deliberately **not** wired into `ci-local.sh`: it needs network, and a private
repo would make the gate fail permanently for a condition that is not a code
defect.

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new error paths. The four extracted comparisons preserve their `apierrors.Structured` returns, sentinels and categories exactly; hints are now derived from the lists they describe. |
| Input Validation | PASS | `inClosedSet` treats `""` as unset, matching each original `x != "" && x != …` guard. WebSocket `action` uses bare `slices.Contains` because the original `switch`/`default` rejected `""` — preserved and covered. |
| Naming | PASS | Four new exported vars, all with doc comments, no stuttering, following the existing `output.SupportedFormats` precedent. |
| Code Organization | PASS | No new package dependencies. `internal/parser` does not import `internal/schema`, so the parity test in package `schema_test` creates no cycle. |
| Correctness | PASS | Behaviour-preserving refactor confirmed by the pre-existing parser suite plus the new closed-set tests. No concurrency, resources or context plumbing touched. |
| Test Quality | PASS | Table-driven throughout, `t.Run` subtests, explicit `error case -` rows, negative controls for every new schema definition. |

## Verification Independence

The schema change is confirmed from three directions that share no code path:

1. **Go tests** — reflection parity against the parser structs, plus
   `santhosh-tekuri/jsonschema` validation of seven fixtures.
2. **An independent validator** — Python `jsonschema` `Draft202012Validator`
   accepts the task's own observable document with 0 errors, and still rejects
   six typo cases, proving `additionalProperties: false` was not weakened to buy
   the completeness.
3. **The real parser** — `TestSchema_dod_fixture_parses` runs the
   all-nine-fields fixture through `parser.ParseFile` and asserts the decoded
   values, so schema and parser agree on a *document*, not just a name list.

## Behaviour Coverage

| # | Behaviour | Covering test | Status |
|---|-----------|---------------|--------|
| 1 | Collection using the nine fields validates cleanly | `TestSchema_accepts` (7 fixtures), `TestSchema_examples`, observable 2 | ✅ |
| 2 | Every yaml-tagged parser field is described | `TestSchema_parser_fields_are_described`, plus converse and table-completeness guards | ✅ |
| 3 | Committed files match `curlew schema` output | `TestSchema_published_path_matches_embed`, `TestSchema_project_published_path_matches_embed` | ⚠️ tautological — see below |
| 4 | `$id` resolves rather than 404s | `TestSchema_ids_match_module_path` + `scripts/check-schema-urls.sh` | ⚠️ premise false — documented above |
| 5 | MANUAL §1.5 caveat corrected | prose; no automated guard | ✅ (manual) |

**On behaviour 3.** `curlew schema` writes the `go:embed`ed bytes of the very
file being diffed, and `go test` recompiles the embed from that file, so the
check cannot fail. It was already satisfied before this task. It is left in place
because the DoD names it, and it is *not* the recurrence guard — that is
`internal/schema/parity_test.go`. Stated explicitly so the sign-off is honest
rather than implied.

## Test Coverage

- Total: **85.0%** (threshold 80%)
- New test files: `internal/schema/parity_test.go`,
  `internal/schema/astsource_test.go`, `internal/parser/closedsets_test.go`
- Missing coverage: none introduced. The schema files are data, not statements.

## Pre-existing issue found while running the gate (not caused by this task)

`TestTAPOutput_ParallelSpeedup` (`cmd/curlew/main_test.go:6892`) is flaky. Verified
on `main` in a clean worktree with none of this branch's changes: **2 failures in
8 consecutive runs**. It compares `speedup_factor` between two separate wall-clock
runs, so machine load makes the values legitimately diverge. Out of scope here and
filed separately; recorded so this branch is not blamed for an intermittent red CI.

## Summary

The defect is fixed and made non-recurring: reflection parity in both directions,
table-completeness so a new nested struct cannot slip past, `go/ast` guards
pinning the two tagless decoders, and enum pinning across five closed sets. Scope
grew beyond the nine fields by four verified defects of the same class —
`$defs.request` requiring `method`, `project-v1.json` omitting `config:`, an error
hint offering a protocol the parser rejects, and a published schema URL that
404s — each fixed with a test or a script that checks it. All first-pass findings
are resolved.
