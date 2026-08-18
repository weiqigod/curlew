# Improvement Report: M25-004

**Task:** A binary built from source knows which version it is
**Date:** 2026-08-18
**Review:** management/reviews/M25-004-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | Three comments in `cmd/curlew/readme_install_exec_test.go` (~25-34, 101-114, 120-133) and a `CHANGELOG.md` "Added" entry claimed that once a post-fix tag exists, README's "Install from source" and "Clone and build" blocks would ALSO report a released version — the stated justification for the attribution check (`gh release download` in body) this task added. False: neither block invokes `--version` at all, at any tag, so the released-version regex can only ever match the download block's output, independently of tag state. | Corrected all four places to state the true, tag-independent reason. Added a hermetic subtest to `TestReadme_documents_a_binary_download` (`readme_install_test.go`), "only the download block runs the built binary", so the assumption is enforced structurally instead of resting only in prose. Added an `E4` deviation record to `management/plans/M25-004-plan.md`, since Step 7 / D7 is where the false claim originated and the same repository convention (E1–E3) already existed for recording this class of correction. | Ran both README blocks verbatim in isolation (proved zero output / git-only output, not read from the review); mutation-tested the new subtest (temporarily broke the invariant → red, reverted → green); full quality gate green; `git tag -l` unchanged |

## Repair Option Chosen

The review offered two repairs and left the choice to the implementer: (a) add a `--version` check to the two README blocks so the original claim becomes true, or (b) correct the four places to describe reality. **Chose (b)**, after evaluating (a) concretely rather than by inspection alone (a Plan agent was used to reason through the trade-off before any file was touched, per the `/improve` skill's Phase 2):

- The reviewer's own suggested line for the install-from-source block, `` $(go env GOPATH)/bin/curlew --version ``, was reproduced verbatim under the test's actual harness this session and **fails today** — `rc=127`, "No such file or directory" — not just hypothetically at a future tag. The test redirects `GOBIN` to a per-block temp dir specifically so `go install` cannot overwrite the developer's `~/go/bin/curlew` (`readme_install_exec_test.go:98-103`); `go env GOPATH` does not follow that redirection, so the two diverge under the harness.
- Even setting that aside, a correctly-written version check on the "Clone and build" block would not reliably demonstrate anything: that block clones the default branch, not a tag, and `main` is measurably already past the latest tag in steady state (`git describe --tags main` = `v0.1.0-2-gf8584df`, confirmed fresh this session, not merely cited). A checkout that isn't exactly at a tag stamps a Go pseudo-version, which this task's own resolver correctly rejects — so the block would keep printing the compile-time default in ordinary operation, matching the released-version regex only in the narrow window between a tag and the next commit to `main`.
- Naively applying (a) — appending the version lines without also reworking the attribution loop's error branch — would have planted a latent failure: the first time a future tag proved M25-004's own fix works through `go install <module>@<tag>`, the unmodified attribution check would error on that very success, because the install-from-source block's body does not and structurally cannot contain `gh release download`. That is a worse outcome than the finding being fixed.

Given (a)'s literal form breaks today and its safe form would require reworking runtime test logic that cannot be fully verified against a real future tag without violating the release-freeze constraint (no new tags in this repo), (b) was the deliberate, evidence-based choice: it fully resolves the finding, is provably correct now, and carries no risk of a latent future failure. It was strengthened beyond a pure prose fix by making the underlying assumption executable (the new hermetic subtest), so the corrected comments are no longer just another unchecked claim.

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

One adjacent observation surfaced while investigating this finding and is deliberately not folded in: `README.md:123-127`'s prose ("a source build or `go install` at that same tag derives it from the module version instead") is the mirror image of this finding — true only once a post-fix tag exists, and false today for the only tag that exists (`v0.1.0`, whose source predates this fix; `go install …@v0.1.0` measurably prints `curlew 0.1.0-dev`, per `management/tasks/M25-004.yaml:31-34`). Left alone because the review did not name it as a finding, matching this task's own `E3` precedent of not correcting prose whose premise turns out, on inspection, to already be accurate for the case it actually describes.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go build -tags readme_install ./cmd/curlew/...` | PASS |
| `go vet ./cmd/curlew/...` (default and `-tags readme_install`) | PASS |
| `go test ./...` (via `go_pkgs`, matching `ci-local.sh`'s node_modules exclusion) | PASS |
| `go test -tags readme_install -run '^TestReadme_install_commands_execute$'` | PASS (6.67s) |
| `go test -tags readme_install` full 3-test group (`readme_tests_re`) | PASS, count still 3 |
| `golangci-lint run` (both `release_artifacts` and `readme_install` tags active, per `.golangci.yml`) | PASS, 0 issues |
| `./scripts/ci-local.sh --go` (full authoritative gate, including the `release_artifacts` and `readme_install` steps) | PASS |
| Coverage | 86.2% total / 81.1% `cmd/curlew` — unchanged from the review's own baseline (no new production code; only test code was added) |
| `git tag -l` | `v0.1.0` before and after, unchanged |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 7652b24 | fix(cli): correct the README-exec test's false next-tag claim | #1 |

## Summary
1/1 findings resolved. 0 deferred.
