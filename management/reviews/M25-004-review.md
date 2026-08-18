# Code Review: M25-004

**Task:** A binary built from source knows which version it is
**Reviewer:** AI
**Date:** 2026-08-18
**Branch:** feature/M25-004-buildinfo-version (iteration 1)

## Verdict: FAIL

## Pre-audit Gate

`./scripts/ci-local.sh --go` — **PASS**, run twice, `rc=0` both times.

- `go build ./cmd/curlew`: ok
- `go test $(go_pkgs)`: ok, `cmd/curlew` 103.973s
- `go test -race $(go_pkgs)`: ok, `cmd/curlew` 110.951s
- `go coverage`: `cmd/curlew` 81.1%, total 86.2%
- `golangci-lint run`: 0 issues
- `smoke/run.sh`: ok
- goreleaser `check` + snapshot build + `TestRelease_*` (release_artifacts, tagged): all PASS

Additionally ran (not part of `--go`, but relevant to Finding 1 and to CLAUDE.md's completeness bar):
`go test -tags readme_install -run '^TestReadme_install_commands_execute$' ./cmd/curlew/ -count=1 -v` → PASS, 6.06s.

The static audit below proceeds because the gate is green — but green tests are necessary, not sufficient; Finding 1 is a case of a green test whose own stated justification does not hold.

## Findings

| # | Severity | Category | File | Line | Finding | Recommendation |
|---|----------|----------|------|------|---------|---------------|
| 1 | Medium | Test Quality / Documentation Accuracy | `cmd/curlew/readme_install_exec_test.go` | 25-34, 101-114, 120-133 (mirrored in `CHANGELOG.md`) | Comments added by this task — three separate places in this file, plus a `CHANGELOG.md` "Added" entry — assert that once a post-fix tag exists, README's "Install from source" (`go install …@latest`) and "Clone and build" (`git clone && go build`) blocks will "ALSO report a released version," which is why an attribution check (`strings.Contains(b.body, "gh release download")`) was added to keep the test meaningful. This claim is false for the README as it exists on this branch: neither block's script invokes the installed/built binary at all. Verified by extracting and running both blocks verbatim in isolation: `GOPRIVATE='github.com/weiqigod/*' go install github.com/weiqigod/curlew/cmd/curlew@latest` produced **zero bytes** of combined stdout+stderr on success; `git clone https://github.com/weiqigod/curlew && cd curlew && go build ./cmd/curlew` produced only git's own `Cloning into 'curlew'...` line. Only the "Download a release binary" block ends with `./curlew --version`. So `readmeReleasedVersionRE` (`^curlew [0-9]+\.[0-9]+\.[0-9]+$`) can **only ever** match block 1's output — not because of tag state, but because it is the only block that runs the binary at all — both before and after this task. The attribution logic this task added is therefore defending a branch that cannot execute given the README's actual content, and the plan's own claim (M25-004-plan.md D7: "The real path becomes covered by `TestReadme_install_commands_execute` the moment a post-fix tag is cut") does not hold: that test will keep exercising exactly the one path it already exercised pre-M25-004 (the download archive, which was never broken — `-X main.version` always worked). The only place behavior 2 ("a binary installed with `go install <module>@<tag>`... reports that tag") is actually exercised end-to-end is `TestVersion_tagged_build_reports_the_tag`'s synthetic fixture, which is honestly labelled as a proxy (D7) — the README-execution test is not the second, user-facing proof the comments claim it to be. | Add a version-check line to the end of the "Install from source" and "Clone and build" README blocks — e.g. `` $(go env GOPATH)/bin/curlew --version `` for the install block and `./curlew --version` for the clone-and-build block, mirroring the download block's own pattern. This both gives a human reader a way to confirm their install worked (arguably the more valuable fix, and in the spirit of this task's own goal) and makes the existing attribution check start testing something real once a tag is cut. If the blocks are deliberately being kept minimal instead, correct the three comments in this file and the `CHANGELOG.md` entry, since as written they assert a specific, checkable future test behavior that will not occur. |

## Standards Compliance

| Category | Status | Notes |
|----------|--------|-------|
| Error Handling | PASS | `resolveVersion`/`buildInfoVersion` follow the codebase's existing `(value, ok bool)` idiom (matching `debug.ReadBuildInfo` itself); no swallowed errors; the resolver is documented as never panicking and never returning `""`, and the table tests exercise every stated branch. |
| Input Validation | PASS | Every edge case enumerated in the task's scope is tested and independently reverified: `ok==false`, nil info, `""`, `"(devel)"`, all three canonical Go pseudo-version forms, `+dirty`, `+incompatible`. Independently re-verified with a standalone 25-case probe (13 reject / 12 accept, including `v10.20.30`, `v1.0.0-beta.2`, `v0.2.0-rc.1` named in the review brief) — all correct in both directions. |
| Naming | PASS | No stuttering, doc comments on every symbol in the new file (none are exported, since `package main`, but the project's comment density convention is followed regardless), package-level names read clearly (`resolvedVersion` vs `version` vs `defaultVersion` is self-documenting). |
| Code Organization | PASS | `cmd/curlew/version.go` is a clean, single-purpose new file; `.golangci.yml` correctly left untouched (no new build tag was needed, per plan D9, confirmed empty diff); no import-boundary or circular-dependency issues. |
| Correctness | PASS | Verified, not merely read: (1) reverted `resolveVersion` to a stub → `TestVersion_falls_back_to_build_info` goes RED (15 of 16 subtests fail) and `TestVersion_tagged_build_reports_the_tag`'s clean leg goes RED while its dirty leg correctly stays green; (2) removed the fixture's `git tag` step → fails loudly (`git describe --tags --exact-match HEAD: exit status 128`), never silently passes; (3) injected a fake 8th read of the raw `version` symbol into `main.go` → `TestVersion_only_version_go_reads_the_raw_symbol` catches it; (4) deleted `-X main.version={{ .Version }}` from `.goreleaser.yaml` under a trap and ran the real `goreleaser build --snapshot --clean --single-target` → artifact reports exactly `curlew 0.1.0-dev`, confirming `scripts/ci-local.sh`'s guard comment (line ~320) is accurate, and additionally confirmed a second, independent guard (`TestRelease_version_matches_the_tag`'s native-binary-execution leg, plus `ci-local.sh`'s own `-snapshot`-suffix regex check) would still catch a broken `-X` even in the narrow theoretical case where HEAD sits exactly at an existing tag, since the fallback path can never coincidentally produce a `-snapshot`-suffixed string; (5) built a real tagged fixture (`v0.77.0`, outside the source tree, real repo's tags left untouched — verified `git tag -l` prints exactly `v0.1.0` throughout) and confirmed all **seven** production surfaces agree: `--version`, `--help`'s `Version:` line, `info --format json`, the run-events NDJSON `curlew_version` field, the scaffolded `SKILL.md` comment, and the UI server's `/api/v1/meta` — all reported `0.77.0`, going beyond what the automated `TestVersion_all_surfaces_agree` covers (which only checks 3 of 7 surfaces, and only on an untagged/`(devel)` test binary). |
| Test Quality | FAIL | See Finding 1. Otherwise strong: table-driven, two-sided (mutation-tested in both directions above), exact-equality assertions replacing the old vacuous prefix check, an AST-walk guard rather than a grep, a `t.Fatal`-only fixture with no silent-skip paths. |

## Test Coverage

- `cmd/curlew` package: 81.1% (repo total 86.2%) — both exceed the 80% DoD floor.
- `cmd/curlew/version.go` function coverage: `resolveVersion` 100%, `usableBuildVersion` 100%, `trimVersionPrefix` 100%, `buildInfoVersion` 75% (the untested branch is `ok==true but info==nil`, a defensive case `debug.ReadBuildInfo` cannot actually produce — acceptable).
- Missing coverage: none material. The one gap found is not a coverage percentage problem — it's a claim, in comments and CHANGELOG.md, about what a passing test proves, that does not match what the test actually exercises.

## DoD / Behavior Cross-check

All nine `definition_of_done` items and all five `behaviors` in `management/tasks/M25-004.yaml` have a corresponding test, and each was spot-checked against real output rather than taken on faith (see Correctness row above). Behavior 2 (`go install <module>@<tag>`) is honestly documented in the plan as unreachable today (only tag is `v0.1.0`, predating the fix) and is covered by a well-designed, clearly-labelled proxy fixture instead — that part of the design is sound. The gap is narrower than behavior 2 itself: it's specifically that the README's own executable documentation was claimed (new, in this task) to become a *second* proof of the same behavior once a tag exists, and that claim doesn't hold given what the README blocks actually run.

## Summary

The core mechanism — `resolveVersion`, the pseudo-version rejection, the AST guard, the tagged-build fixture, and agreement across all seven production call sites — is implemented carefully and holds up under adversarial mutation testing well beyond what the existing automated suite exercises; I found no defect in any of it after actively trying to break each piece. The one finding is narrower and lower-stakes: this task, while touching M25-003's README-execution test for an unrelated reason (attribution), added new comments and a CHANGELOG entry making a specific, checkable claim about that test's future behavior that is false given the README's actual command blocks, which never invoke `--version` outside the download path. Fixing it is small — either extend two README blocks with a version check (the more valuable option, and arguably closer to this task's own intent) or correct the four places that overstate what the test proves.
