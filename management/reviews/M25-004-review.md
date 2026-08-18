# Code Review: M25-004

**Task:** A binary built from source knows which version it is
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M25-004-buildinfo-version (iteration 2)

## Verdict: FAIL

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS**, `rc=0`.

- `go build ./cmd/curlew`: ok
- `go test $(go_pkgs)`, `go test -race $(go_pkgs)`: ok
- coverage: `cmd/curlew` 81.1%, total 86.2% — re-measured directly this session
  (`go test -coverprofile=... $(go_pkgs)` then `go tool cover -func=...`),
  byte-for-byte match with iteration 1's baseline and with the improvement
  report's claim
- `golangci-lint run`: 0 issues — re-run standalone this session with
  `--build-tags readme_install,release_artifacts` against `./cmd/curlew/...`,
  confirmed 0 issues independently of the full gate
- `smoke/run.sh`, goreleaser `check` + snapshot build + `TestRelease_*`
  (release_artifacts, tagged): all PASS
- The `readme_install`-tagged step (`TestReadme_install_commands_execute`,
  `TestReadme_documents_a_binary_download`, `TestReadme_install_blocks_extraction`)
  is confirmed unconditional in `scripts/ci-local.sh` — it sits outside every
  `if` block in the script (verified by reading the surrounding control flow),
  guarded only by a `command -v gh` probe that fails loudly, not a changed-file
  scope gate — so it necessarily ran and passed as part of the `rc=0` above.

Diff scope re-confirmed against the prior iteration's review commit:
`git diff 47a7133..HEAD --stat` shows only `CHANGELOG.md`,
`cmd/curlew/readme_install_exec_test.go`, `cmd/curlew/readme_install_test.go`,
`management/plans/M25-004-improved.md`, and `management/plans/M25-004-plan.md`
changed since iteration 1 — exactly commits `7652b24` and `ef058fe`, nothing
else touched.

`git tag -l` printed exactly `v0.1.0` before and after every command run this
session, including the mutation tests below. `git status --porcelain` is clean.

---

## Part A/B/C — Verification of Iteration 1's Finding (by measurement)

Iteration 1's sole finding: comments in `readme_install_exec_test.go` and a
`CHANGELOG.md` entry falsely claimed the "Install from source" and "Clone and
build" README blocks would report a released version once a post-fix tag
exists, when neither block invokes `--version` at all, at any tag. The improve
phase chose to correct the prose (not extend the README blocks) and added a
hermetic subtest. Verified directly rather than by trusting the improvement
report's own account:

**A. The new subtest is genuinely two-sided, not decoration.** Ran
`TestReadme_documents_a_binary_download` as a baseline (green, 8/8 subtests
pass). Then, under a `trap 'git checkout -- README.md' EXIT INT TERM`:

- Added `` $(go env GOPATH)/bin/curlew --version `` to the "Install from
  source" block (a non-download block). The `"only the download block runs
  the built binary"` subtest went **RED**: `block invokes --version=true, is
  the download block=false`. Reverted; re-ran; back to **GREEN**.
- Separately, removed `./curlew --version` from the actual download block
  (mirror direction of the same biconditional). Went **RED**:
  `block invokes --version=false, is the download block=true`. Reverted;
  re-ran; back to **GREEN**.

Both directions of the check are live. `git status --porcelain` was clean
after each revert.

**B. Audited the new subtest for vacuity.** It iterates
`readmeSectionBlocks(doc, "Install")` with no explicit floor guard of its
own, so a renamed/emptied `## Install` heading would make it silently execute
zero iterations and report a vacuous pass. This is not a live gap in
practice: (1) its sibling subtest `"install section exists and is non-empty"`
in the same test function fails loudly (`t.Fatal`) if the heading disappears,
and (2) the tagged `TestReadme_install_commands_execute` — run
unconditionally by `ci-local.sh`, confirmed above — has its own explicit
`len(blocks) == 0` / `len(blocks) < 3` guards against the identical
extraction call on the identical document, so the same structural change
that would make this subtest vacuous necessarily fails a sibling test in the
same gate run. Block reordering does not affect it either, since the check
is per-block (`runsVersion != isDownloadBlock`), not positional. The
attribution substrings (`"--version"`, `"gh release download"`) match the
same substrings `readme_install_exec_test.go` itself keys on, and neither
appears incidentally in the other two blocks' current bodies. Not flagged as
a finding — redundant coverage elsewhere means no scenario silently escapes
the gate — but noted as a design observation: the subtest would be more
robust with its own explicit floor check rather than relying on a sibling.

**C. The corrected prose is true.** Ran both non-download blocks verbatim,
independently of the test suite:
- Install-from-source block (`GOPRIVATE=... go install .../curlew@latest`
  into an isolated `GOBIN`): **zero bytes** of combined stdout+stderr on
  success. Matches the comment's claim exactly.
- Clone-and-build block (`git clone ... && cd curlew && go build
  ./cmd/curlew`): output was exactly `Cloning into 'curlew'...` — git's own
  line, nothing else. Matches the comment's claim exactly.

Also spot-checked the CHANGELOG.md numeric claims introduced by this task:
`TestVersion_falls_back_to_build_info` has exactly 17 subtests (counted via
`go test -v`, matching the "17 rows" claim), and the coverage/lint figures
above match what the improvement report recorded.

**Conclusion: iteration 1's finding is genuinely resolved.** The fix is
correct, the new subtest is a real check in both directions, and the
corrected prose holds up under direct execution.

---

## Part D — Full Review of the Diff

Read every changed file's full diff: `cmd/curlew/version.go` and
`version_test.go` (unchanged since iteration 1, which already re-verified the
resolver's mechanics under mutation — not re-audited here per instructions),
`main.go`, `main_test.go`, `ui.go`, `release_artifacts_test.go`,
`skill_playbook_test.go` (all mechanical `version`→`resolvedVersion` or
`version`→`defaultVersion` symbol repoints, each checked for using the
*correct* one of the two — `release_artifacts_test.go` correctly kept
`defaultVersion`, not `resolvedVersion`, in its comparison against a
`--snapshot` archive's parsed filename, since `resolvedVersion` would there
reflect the *test binary's own* build info rather than the thing under test;
this distinction is right), `CHANGELOG.md`, `README.md`,
`management/backlog.yaml`, `management/tasks/M25-004.yaml`,
`management/plans/M25-004-plan.md`, `management/plans/M25-004-improved.md`,
and the `scripts/ci-local.sh` guard-message hunk. This surfaced one new
finding, in a file iteration 1's finding did not examine.

### Finding: README.md's new "at that same tag" paragraph is false today, for the one tag that exists

`README.md:123-127` (added by this branch — confirmed via
`git diff main...HEAD -- README.md`, not pre-existing) reads:

> A binary reports a real version when it was built from a release tag — the
> release build injects it at link time, and a source build or `go install`
> at that same tag derives it from the module version instead (see
> `cmd/curlew/version.go`). Built from an untagged commit, either path
> reports `0.1.0-dev`.

This is an unqualified, present-tense claim, sitting directly below the
download block and directly above "### Install from source" — the section a
reader would check next. It is false for the repository's actual, current
state. Verified directly this session, independent of the improvement
report's own account:

```
go install .../curlew@latest   -> curlew 0.1.0-dev
go install .../curlew@v0.1.0   -> curlew 0.1.0-dev   (v0.1.0 is the only tag)
git clone (main) && go build   -> curlew 0.1.0-dev
```

`@latest` and `@v0.1.0` were installed separately, into separate clean
`GOBIN`s, and both report the placeholder — because `v0.1.0`'s source
predates this fix, `go install` "at that same tag" does **not** derive a
real version today, contradicting the paragraph's own words.

This is not a matter of interpretation. This exact task's own plan already
says so, in D7, written before the improve phase touched this area: *"the
only tag is `v0.1.0`, whose source predates this fix, so an installed v0.1.0
binary will report `0.1.0-dev` **forever and correctly**"* — the plan and the
shipped README directly contradict each other about the same fact. The
improvement report itself independently noticed this exact paragraph while
investigating iteration 1's finding (its "Out of Scope" section: *"true only
once a post-fix tag exists, and false today for the only tag that exists...
`go install …@v0.1.0` measurably prints `curlew 0.1.0-dev`"*) and chose to
leave it, citing "this task's own `E3` precedent of not correcting prose
whose premise turns out, on inspection, to already be accurate for the case
it actually describes." That citation does not support the decision made:
E3 (`docs/MANUAL.md:180`) was a case where an *already-true* claim was nearly
"corrected" into a false one and left alone because it was found accurate.
This is the reverse — a claim already known, in the same paragraph of the
same report, to be false for the only case a reader can currently test
against, deliberately left uncorrected.

Since release state is frozen (no new tag may be cut, in this review or in
`/improve`), there is no way to make the literal words true today. That is a
reason to change the wording, not a reason to leave a known-false claim in
the project's primary entry-point document — precisely the class of
self-honesty regression M25 exists to close (per `CLAUDE.md`: "`--version`
reports `0.1.0-dev` everywhere" is named as one of the four biggest measured
gaps this milestone addresses). A new user following this README's own
"Install from source" instructions verbatim, immediately after reading this
paragraph, gets the placeholder it says they will not get.

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | High | Documentation Accuracy | `README.md` | 123-127 | New paragraph (added by this branch) claims "a source build or `go install` at that same tag derives it from the module version instead" as present-tense fact. False for the only tag that exists: `go install .../curlew@v0.1.0` (and `@latest`, which resolves to it) both measurably print `curlew 0.1.0-dev` this session, matching the plan's own D7 ("will report `0.1.0-dev` forever and correctly"). The improvement report noticed this and left it, citing an E3 precedent that does not apply (E3 was prose found to be *already true*; this prose is known to be false). | Rewrite to state what is actually true today rather than what will become true at some future tag: e.g. state that the tag-derived path takes effect starting with the first release cut *after* this fix, and that the repository's current tag (`v0.1.0`) predates it, so today only the downloaded release archive reports a real version. Do not phrase it as a general present-tense mechanism claim without that qualifier while the one real tag contradicts it. |

No other findings. The rest of the diff — the resolver itself, the AST guard,
the tagged fixture, the seven production call-site repoints, the two test
files' symbol choices, the CHANGELOG's other claims, `ci-local.sh`'s updated
guard comment — held up under the checks performed this iteration and under
iteration 1's already-completed mutation testing, which this iteration did
not repeat per the operating instructions.

---

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new production error paths this iteration (test files and prose only). Iteration 1's assessment of `version.go` stands, unchanged since. |
| Input Validation | PASS | Unchanged since iteration 1 (`version.go`/`version_test.go` not touched by the improve phase). |
| Naming | PASS | New subtest name (`"only the download block runs the built binary"`) and local identifiers (`runsVersion`, `isDownloadBlock`) are clear and accurate. |
| Code Organization | PASS | The new subtest is correctly placed in the untagged `readme_install_test.go`, consistent with that file's own stated rationale for what lives untagged vs. behind `//go:build readme_install`. |
| Correctness | PASS | The fix is correct and verified two-sided in both directions (Part A). The mechanical symbol repoints in `main.go`/`ui.go`/`main_test.go`/`skill_playbook_test.go`/`release_artifacts_test.go` are all the *right* choice of `resolvedVersion` vs. `defaultVersion` for their respective purposes, checked individually. |
| Test Quality | PASS | The new subtest is a real, two-sided check (Part A), correctly scoped (Part B), with only a minor non-blocking defense-in-depth observation (no independent floor guard, but redundantly covered by sibling tests). |
| Documentation Accuracy | FAIL | See finding above. A new, currently-false, user-facing claim was introduced by this task and knowingly left in place. |

## Test Coverage

- `cmd/curlew` package: 81.1% (repo total 86.2%) — re-measured directly this
  session, matches iteration 1's baseline exactly (no new production code
  this iteration; only test code and prose changed).
- Missing coverage: none material.

## DoD / Behavior Cross-check

Unchanged from iteration 1 for the nine `definition_of_done` items and five
`behaviors` — no production code changed since that assessment. One
additional cross-check specific to this iteration: behavior 2 (`go install
<module>@<tag>` reports that tag) is correctly and explicitly documented as
*unreachable today* in the plan's own D7 — that part of this task's
internal record-keeping is honest. The finding above is that the *shipped,
user-facing* README does not carry the same honesty: it asserts behavior 2
already works via the exact install path it documents, which the plan itself
says is not yet true.

## Summary

The core mechanism, already found sound in iteration 1, remains sound —
nothing in the improve-phase commits touched it, and this iteration's
independent measurements (two-directional mutation testing of the new
subtest, direct re-execution of both non-download README blocks, direct
re-installation at both `@latest` and `@v0.1.0`, re-measured coverage and
lint) all corroborate iteration 1's findings and the improvement report's
own account of its fix. Iteration 1's finding is genuinely closed: the
corrected comments and CHANGELOG entry are true, verified by running the
commands they describe, and the new subtest that backs the assumption is a
real, two-sided check rather than decoration.

The full-diff pass this iteration found one new issue in a part of the diff
iteration 1's finding did not cover: a paragraph this task added to
`README.md` makes a present-tense claim about `go install`-at-a-tag behavior
that is false for the only tag that currently exists, contradicted by this
task's own plan document, noticed during the improve phase, and left in
place on reasoning that does not hold up. It is a small, well-isolated prose
fix — the same size and shape as the fix iteration 1 already required — but
it is real, it is user-facing (more so than iteration 1's test-comment
finding), and it was not resolved.
