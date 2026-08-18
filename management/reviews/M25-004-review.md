# Code Review: M25-004

**Task:** A binary built from source knows which version it is
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M25-004-buildinfo-version (iteration 3)

## Verdict: PASS

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS**, `rc=0`, ends `=== ci-local PASS ===`.
Ran to completion this session (build, test, race, coverage, lint, smoke, and
the `readme_install`/`release_artifacts`-tagged dogfood/redaction/openapi/
crosscheck steps against a live `mudflat` instance) before any static
reading began, per the gate's own instructions.

`git tag -l` printed exactly `v0.1.0` before this session's first command and
after its last. `git status --porcelain` is clean now. Every mutation made
during this iteration (detailed below) was reverted and independently
re-confirmed clean before moving to the next step, not only at the end.

---

## Part A — Is iteration 2's fix genuinely, *permanently* resolved?

Iteration 2's finding: `README.md`'s version paragraph made an unqualified,
present-tense claim about `go install`-at-a-tag behavior that was false for
the only tag that exists. The fix (912cf06) reworded the paragraph to:

> A binary reports a real version when it was built from a release tag — the
> release build injects it at link time. A source build or `go install` at
> that tag reports the same version too, but only once that tag's own source
> already carries the fallback in `cmd/curlew/version.go`. `v0.1.0` predates
> it, so a source build or `go install` at `v0.1.0` reports `0.1.0-dev`
> either way. Built from an untagged commit, every path reports `0.1.0-dev`.

Checked each clause against reality, independently re-measured rather than
trusting the improvement report's account:

**Sentence 1** ("the release build injects it at link time"). True and
structurally guaranteed: `.goreleaser.yaml:47` is
`-X main.version={{ .Version }}`, and `TestVersion_build_info_form_matches_goreleaser`
pins that the template is `{{ .Version }}` (which strips the leading `v`),
not `{{ .Tag }}`.

**Sentence 2** ("A source build or `go install` at that tag reports the same
version too, but only once that tag's own source already carries the
fallback"). This is a general conditional, not a claim about any specific
tag, so "is it still true after the next tag is cut" is the right question
to ask of it. Reasoned through explicitly: if a future tag (say `v0.2.0`) is
cut from a commit that includes `cmd/curlew/version.go`, a source build or
`go install` exactly at that tag stamps `Main.Version = "v0.2.0"` (a clean
tag, not a pseudo-version — confirmed by `TestVersion_tagged_build_reports_the_tag`'s
"clean checkout at the tag" subtest, which does exactly this end-to-end with
a real tag and a real build), `resolveVersion` accepts it
(`usableBuildVersion` matches `releaseVersionRE`, fails `pseudoVersionRE`),
and `trimVersionPrefix` strips the `v` to `"0.2.0"` — byte-identical to what
goreleaser's `-X` would inject for the same release. The sentence is true
today and stays true for every tag cut after this fix, precisely because it
is phrased as a conditional rather than a claim about current tag state.

**Sentence 3** ("`v0.1.0` predates it, so a source build or `go install` at
`v0.1.0` reports `0.1.0-dev` either way"). Independently re-verified, without
relying on the new test or the improvement report:
```
$ git show v0.1.0:cmd/curlew/version.go
fatal: path 'cmd/curlew/version.go' exists on disk, but not in 'v0.1.0'
$ git show v0.1.0:cmd/curlew/main.go | grep -n 'version ='
52:var version = "0.1.0-dev"
```
At `v0.1.0`, `main.go` hardcodes `version` with no fallback logic at all —
there is no code path by which a source build or `go install` at that exact
tag could report anything other than `0.1.0-dev`, deductively, without
needing a network `go install` call against the private repo to confirm it.
`v0.1.0` is an immutable historical tag, so this stays true forever short of
the tag being force-recreated at a different commit — a scenario
`TestVersion_v0_1_0_predates_the_fallback` (below) would catch.

**Sentence 4** ("Built from an untagged commit, every path reports
`0.1.0-dev`") — the specific probe the task instructions asked for. This
needed real measurement, not inspection, because sentence 1 brings goreleaser's
release-build mechanism into the paragraph's scope, and that mechanism *can*
run against an untagged commit in one documented mode: a local snapshot
build. Measured directly this session, a real `goreleaser build --snapshot`
against this repository's actual current HEAD (which is several commits past
`v0.1.0`, i.e. genuinely untagged):
```
$ goreleaser build --snapshot --clean --single-target
  ...
    • building snapshot...                           version=0.1.1-snapshot
$ ./dist/curlew_darwin_arm64_v8.0/curlew --version
curlew 0.1.1-snapshot
```
Read literally and broadly, "every path" is false: this is a real, sanctioned
command (`.goreleaser.yaml`'s own header comment documents
`goreleaser build --snapshot --clean --single-target` as a "local check"),
run from an untagged commit, and it does not report `0.1.0-dev`. This is not
a hypothetical edge case — `cmd/curlew/release_artifacts_test.go`'s
pre-existing `TestRelease_version_matches_the_tag` already runs a real
`goreleaser release --snapshot --clean` and asserts the resulting version
both is *not* `defaultVersion` and *does* match `^curlew [0-9]+\.[0-9]+\.[0-9]+-snapshot$`
— i.e. the project's own test suite has asserted the opposite of a literal
"every path" reading since before this task existed. `scripts/ci-local.sh`'s
own M25-004-authored comment on this exact check states the tension
explicitly: "a snapshot build's own checkout is untagged for versioning
purposes ... so a broken -X still resolves to this same literal, and the
-snapshot-suffix check just below is a second, independent guard."

Having measured the contradiction, the question the task posed is whether
"path" is adequately scoped by context to exclude it. It is. `README.md`
establishes "path" as section-local terminology *before* this paragraph, at
the top of `## Install`: line 103, unchanged by this task, reads "The
repository is private, so every path below needs GitHub access" — "path"
there unambiguously means "one of the three subsections under `## Install`".
The pre-task version of this same paragraph (visible via
`git diff main...HEAD -- README.md`) used the identical convention: "This is
the **only path** that produces a binary reporting its real version."
Neither usage, before or after this task, was ever intended to reach outside
the three reader-facing install methods into internal release tooling that
is never itself called "a path" anywhere in `README.md` and is not one of
the three `## Install` subsections. Under that reading — which is the
document's own established one, not a charitable rescue invented for this
review — sentence 4 is true today and stays true after any future tag: none
of the three real install paths can, at any point, exercise goreleaser's
snapshot machinery, because a reader of this README never runs `goreleaser`
themselves.

**Conclusion: iteration 2's finding is genuinely and permanently resolved.**
The rewritten paragraph's four clauses all hold, now and after the next tag.
The "every path" phrase was investigated as a specific, real discrepancy
(confirmed by direct measurement, not assumed) but resolves in the README's
favor once weighed against the document's own pre-existing, unchanged
definition of "path" — this is judged a correct, defensible claim rather
than a manufactured pass; the reasoning above is the evidence either way.

---

## Part B — Audit of what improve iteration 2 added (commits 912cf06, 86602d9)

`TestVersion_v0_1_0_predates_the_fallback` (`cmd/curlew/version_test.go:664-678`)
is new production-adjacent test code, audited independently rather than by
trusting the improvement report's claims:

**Two-sidedness, verified by mutation (not by re-reading the report's
claim).** Under `sed -i` against a working copy, changed the queried path
from `cmd/curlew/version.go` to `cmd/curlew/main.go` (known to exist at
`v0.1.0`):
```
--- FAIL: TestVersion_v0_1_0_predates_the_fallback (0.03s)
    version_test.go:675: cmd/curlew/version.go exists at tag v0.1.0 ...
```
Reverted via `git checkout -- cmd/curlew/version_test.go`; re-ran; back to
`PASS`. The check is live in both directions.

**Behavior with no `.git`, with git absent from `PATH`, in a shallow clone,
and in a worktree** — all four probed directly this session, not assumed
from the code:

- *No `.git` anywhere in the directory tree* (proxy for a module-cache
  extraction): built a standalone test binary (`go test -c`) and ran it with
  `cwd` set to an empty scratch directory confirmed outside any git repo.
  Result: `git rev-parse v0.1.0 failed (exit status 128) ... fatal: not a
  git repository (or any of the parent directories): .git` — fails loudly,
  `CombinedOutput`'s text makes the real cause legible even though the
  comment's own framing anticipates a narrower cause (a shallow clone).
- *`git` absent from `PATH`*: ran the same compiled binary with
  `PATH` pointed at an empty directory. Result: `git rev-parse v0.1.0 failed
  (exec: "git": executable file not found in $PATH) ...` — distinctly
  legible, not confusable with the shallow-clone case.
- *Genuine shallow clone*: `git clone --depth 1 file:///.../curlew
  <scratch>` (a `file://` URL is required — a same-host path clone silently
  ignores `--depth`, confirmed by git's own warning on the first attempt).
  Confirmed `git rev-parse --is-shallow-repository` = `true` and `git tag -l`
  = empty in the clone. Ran the package's tests there: the guard fires
  exactly as the code comment predicts — `git rev-parse v0.1.0 failed (exit
  status 1) -- this checkout cannot resolve tag v0.1.0 (a shallow clone
  would look like this) ...`. This is the scenario the guard was explicitly
  built for, and it holds: without it, a shallow clone lacking the tag
  entirely would make `git cat-file -e v0.1.0:cmd/curlew/version.go` fail
  for a *different* reason (unresolvable ref, not "path absent at that
  ref") and the test's `err == nil` check would treat that failure as a
  silent pass — exactly the vacuous-pass class of bug this file's other
  fixtures are careful to guard against (`buildTaggedFixture`'s own
  guards, D8 in the plan). The guard prevents that class of false pass here
  too.
- *Worktree*: `git worktree add --detach <scratch> HEAD`, ran
  `go test ./cmd/curlew/ -run TestVersion_v0_1_0_predates_the_fallback` from
  inside it. `PASS` — worktrees share refs with the main repository, so
  tag resolution works unmodified. Removed via `git worktree remove --force`
  afterward; `git worktree list` confirmed only the main worktree remains.

All four scenarios fail loudly for a diagnosable reason (or pass correctly,
for the worktree case) — none silently mis-report success, and none produces
a message a future reader would misdiagnose as a different root cause than
the actual one shown in the command's own output.

**Does it belong in `cmd/curlew`'s test suite, given it tests a git fact
rather than a binary behaviour?** Yes, on the merits and on precedent
already established in this exact file before this iteration:
`TestVersion_build_info_form_matches_goreleaser` (iteration 1) already pins
a fact about `.goreleaser.yaml`'s YAML content as an executable check in
this same file, and `readme_install_test.go`/`readme_install_exec_test.go`
pin facts about `README.md`'s prose structure the same way. The new test is
the same pattern applied to a git fact instead of a YAML or Markdown one,
placed in the file that already owns the version/README relationship it is
about. It is also honestly scoped in its own doc comment: it "does not
re-check that README.md's wording still matches the fact — only that the
fact itself still holds," which is an accurate description of what it
checks.

**Conclusion: the new test is sound.** Genuinely two-sided, fails loudly and
legibly under every adverse environment probed, and consistent with this
file's existing conventions.

---

## Part C — Full review of the whole diff

Read every changed file's full diff (`CHANGELOG.md`, `README.md`,
`cmd/curlew/main.go`, `main_test.go`, `readme_install_exec_test.go`,
`readme_install_test.go`, `release_artifacts_test.go`,
`skill_playbook_test.go`, `ui.go`, `version.go`, `version_test.go`,
`management/backlog.yaml`, `management/tasks/M25-004.yaml`,
`scripts/ci-local.sh`) plus the plan's design-decision records D0–D10 and
deviation records E1–E5 in `management/plans/M25-004-plan.md`.

- **Correctness.** `version.go`'s resolver logic is unchanged since
  iteration 1 (already mutation-tested in prior iterations, not repeated
  here per the operating instructions). The seven production call sites
  listed in the plan's D5 (`main.go:75,574,929,2632,2725,3374`, `ui.go:227`)
  all correctly use `resolvedVersion`; the two test-side comparisons that
  need the raw compile-time default (`release_artifacts_test.go`'s
  `TestRelease_version_matches_the_tag`) correctly use `defaultVersion`, not
  `resolvedVersion` — using `resolvedVersion` there would have compared a
  snapshot archive's version against *this test binary's own* build info
  rather than against the placeholder, which would silently pass for the
  wrong reason.
- **Error handling.** No new production error paths this iteration beyond
  what iterations 1–2 already reviewed; `version.go`'s `resolveVersion`
  still never panics and never returns `""`, by construction.
- **Naming.** `resolvedVersion`/`defaultVersion`/`resolveVersion`/
  `buildInfoVersion`/`usableBuildVersion`/`trimVersionPrefix` are
  unambiguous and non-stuttering; the new subtest names
  ("only the download block runs the built binary") and locals
  (`runsVersion`, `isDownloadBlock`) read clearly.
- **Dead code.** None found. `golangci-lint run` (part of the Step 0 gate)
  reported 0 issues, which includes the `unused` check that already caught
  a real instance of this during `/execute` (E2 in the plan).
  `scripts/ci-local.sh`'s hardcoded `"curlew 0.1.0-dev"` literal is a
  deliberate, commented trade-off (deriving it from Go source in shell was
  judged not worth the machinery for one string) rather than drift — it
  still matches `defaultVersion` today.
- **Test quality.** Table-driven where appropriate
  (`TestVersion_falls_back_to_build_info`, 17 rows covering both canonical
  pseudo-version forms plus the third the original table omitted, dirty
  variants, `+incompatible`, a bare module path, and empty-injection);
  descriptive `t.Run` subtests throughout; a real end-to-end fixture
  (`TestVersion_tagged_build_reports_the_tag`) that builds a real binary
  from a real tag rather than only asserting against synthetic inputs; the
  AST guard (`TestVersion_only_version_go_reads_the_raw_symbol`) protects
  against regressions none of the other tests would catch, since every test
  binary's own build info is `(devel)` regardless of which symbol a hypothetical
  eighth site read.
- **Docs accuracy.** `README.md`'s paragraph — the subject of Part A — holds
  up under close reading. `CHANGELOG.md`'s new entries accurately describe
  what shipped (spot-checked: `TestVersion_falls_back_to_build_info`
  genuinely has 17 subtests, counted via `go test -v`).
- **Deviation records (E1–E5) vs. what was actually done.** E1's regex fix
  matches the current `pseudoVersionRE` pattern exactly
  (`[.-][0-9]{14}-[0-9a-f]{12}$`, handling both canonical forms). E3's claim
  about `docs/MANUAL.md` having no release-archive path was checked against
  the actual file structure and holds (confirmed no `git diff` touches that
  file). E4's claim that only the download block invokes `--version` was
  independently re-verified against the current `README.md` (lines 113–142):
  "Install from source" ends at `go install`, "Clone and build" ends at
  `go build`, neither has a `--version` line — accurate. E5's account of
  what was wrong and why is accurate and independently re-derived in Part A
  above rather than merely cross-checked against the record.

No findings survived this pass.

---

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | No new production error paths; `resolveVersion` still never panics, never returns `""`. |
| Input Validation | PASS | `resolveVersion`/`usableBuildVersion` enumerate rejection cases by pattern (D2), not by the one case observed; unchanged since iteration 1. |
| Naming | PASS | No stuttering; new identifiers are clear and accurately named. |
| Code Organization | PASS | New test lives in `version_test.go`, consistent with this file's existing precedent for pinning non-Go facts (`.goreleaser.yaml` content, README structure). |
| Correctness | PASS | Verified independently this iteration: mutation-tested the new test, re-derived the `v0.1.0`-predates-the-fallback fact from first principles (`git show`), measured a real goreleaser snapshot build, probed four adverse environments for the new git-shelling test. |
| Test Quality | PASS | Table-driven, real end-to-end fixture, AST guard, two-sided mutation-verified new test, legible failures under no-git/no-repo/shallow-clone/worktree. |
| Documentation Accuracy | PASS | Every clause of `README.md`'s version paragraph checked against reality and against future-tag stability; holds under the document's own established terminology. |

## Test Coverage

- `cmd/curlew` package: 81.1% (repo total 86.2%) — unchanged since iteration
  1's baseline; no new production code this iteration or last, only test
  code and prose.
- Missing coverage: none material.

## DoD / Behavior Cross-check

All nine `definition_of_done` items and five `behaviors` in
`management/tasks/M25-004.yaml` are satisfied, unchanged in substance since
iteration 1's assessment (no production code changed since). `go install
<module>@<tag>` (behavior 2's literal path) remains honestly documented as
untestable in this repository today (plan D7) rather than silently
unaddressed — the fixture-based end-to-end test is an explicitly-disclosed
faithful proxy, not a claim that `go install` itself ran.

## Summary

Three independent lines of investigation this iteration — clause-by-clause
verification of the reworded README paragraph (including a real
`goreleaser build --snapshot` measurement to test the "every path" phrase
against the release-build mechanism sentence 1 introduces), mutation and
adverse-environment testing of the new `TestVersion_v0_1_0_predates_the_fallback`,
and a full-diff pass for correctness/naming/dead-code/test-quality/docs —
found nothing that survives scrutiny. The "every path" phrase was the
closest call: a real, measured discrepancy exists between the literal words
and what a `goreleaser` snapshot build reports, but the document's own
pre-existing (and unchanged by this task) convention for the word "path" —
established at `README.md:103` and in this exact paragraph's own pre-task
wording — scopes it to the three reader-facing `## Install` methods, none of
which can ever, at any future tag, exercise that mechanism. Judged a correct
claim under the document's own terms rather than a manufactured pass.

The core mechanism (unchanged since iteration 1, not re-audited here per the
operating instructions) remains sound. Both prior findings are closed for
good: iteration 1's by a hermetic, two-sided subtest; iteration 2's by a
reworded paragraph whose every clause — including the one this iteration
was specifically asked to stress-test — holds up under direct measurement
and reasoning about future tags, not just present tag state.
