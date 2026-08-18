# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/).

## [Unreleased]

### Fixed
- **Two of the README's three install paths reported a version that
  identified nothing.** `go install <module>@latest` and `git clone && go
  build` both printed `curlew 0.1.0-dev` regardless of what was actually
  checked out — installing the published `v0.1.0` tag through `go install`
  still printed the development placeholder (measured 2026-08-16, M25-004
  plan). Only `gh release download` ever reported a real version, because
  nothing else in the tree derived one from the build itself:
  `debug.ReadBuildInfo` appeared zero times across `cmd/`, `internal/` and
  `testapi/`. A bug report filed from either of the other two paths carried
  a version string that identified nothing, and every project scaffolded by
  `curlew init --skill agent` from a source build embedded the placeholder
  in `SKILL.md` forever.

  `cmd/curlew/version.go` (new) now falls back to
  `debug.ReadBuildInfo().Main.Version` whenever `-X main.version` did not
  inject anything; the ldflag still wins whenever it did
  (`TestVersion_is_injectable_at_link_time` passes unmodified). The fallback
  accepts only a real release or pre-release version and rejects everything
  else: `ok == false`, a nil build-info, an empty string, `"(devel)"`, any Go
  pseudo-version, and anything carrying `+`-prefixed build metadata
  (`+dirty`, `+incompatible`). Pseudo-versions were the case that mattered
  most: on go1.25.5 a plain `go build` does not report the obviously-fake
  `(devel)`, it reports a pseudo-version — measured freshly this session as
  `v0.1.1-0.20260817162458-f8584df3c908` from a clean clone of `main` — a
  patch release that was never cut and that sorts *above* the real `v0.1.0`
  under semver, which would have been worse than the placeholder it
  replaced. All seven production reads of the version, not only the three
  most visible ones, now go through one resolved value (`resolvedVersion`),
  and a new AST-walking test fails the build if an eighth surface ever reads
  the pre-fallback symbol directly, so `--version`, `--help`,
  `info --format json` and a scaffolded `SKILL.md` cannot drift apart from
  each other or from the events file for the same run.

  A follow-up correction to this same task's own README paragraph (found in
  review, after the attribution fix above had already landed): it claimed,
  present tense and unqualified, that "a source build or `go install` at
  that same tag derives it from the module version instead." False for the
  only tag that exists — `v0.1.0`'s source predates this fix
  (`cmd/curlew/version.go` does not exist at that tag), so `go install
  .../curlew@v0.1.0` and `@latest` both still print `curlew 0.1.0-dev`,
  measured directly. The plan's own D7 already said as much ("will report
  `0.1.0-dev` forever and correctly"); the README contradicted its own
  task's plan. Reworded to state the mechanism generally — a source build or
  `go install` reports a tag's real version only once that tag's own source
  carries the fallback — and to name `v0.1.0` as the concrete tag that does
  not, rather than asserting a present-tense claim the one real tag
  contradicts. That phrasing needs no future edit: it is true for `v0.1.0`
  today and stays true for every tag cut hereafter, unlike the wording it
  replaced. `TestVersion_v0_1_0_predates_the_fallback` (new) pins the
  underlying git fact — `git cat-file -e v0.1.0:cmd/curlew/version.go`
  fails — as an executable check rather than leaving it only as prose,
  mirroring the attribution fix's own hermetic subtest above.

- **The unit tests that justified an extraction were never run by anything.**
  `ci-local.sh`'s `--check-signing-keys` mode delegates to
  `scripts/check-signing-keys.sh` under the comment "Delegated to
  check-signing-keys.sh for unit-testability" — and M18-009 duly wrote
  `scripts/check-signing-keys_test.sh`, six tests covering all three exit-code
  paths with a stubbed `psql`. Nothing ever invoked it. Not `ci-local.sh`, not
  any workflow in `.github/workflows/`; the only occurrences of its name in the
  repository were inside the file itself and in its own verification report,
  which recorded "6/6 bash unit tests pass" as of the day it was written and
  bound nothing thereafter.

  That is the shape this project keeps naming as the thing worse than no
  checker: a test file present in a directory listing, cited in a report, and
  never executed. The extraction bought testability and then never spent it.

  The tests run in the Go gate now, so `--go` and every workflow that shells out
  to it cover them. They were confirmed to pass before wiring, and confirmed to
  be load-bearing after: inverting the failure branch of
  `check-signing-keys.sh` so a NULL `kms_key_id` row wrongly exits 0 turns the
  new step red and stops the run, which is the only evidence that distinguishes
  a gate step from a decoration. They stub `psql` on `PATH`, so the step needs
  no database and costs about 0.1s.

- **The README's only install command has never worked, for anyone.**
  `README.md`'s `## Install` section offered exactly one command —
  `go install github.com/weiqigod/curlew/cmd/curlew@latest` — and nothing had
  ever run it. Verified directly: it exits 1, because the repository is
  private and the public checksum database (`sum.golang.org`) cannot read
  the module (`404 Not Found`), and git's non-interactive credential helper
  then fails outright (`fatal: could not read Username for
  'https://github.com': terminal prompts disabled`). The section is
  rewritten around three paths, each now executed for real by a new test: a
  `gh release download` of the release archive (no Go toolchain needed, and
  the only path that reports a released version rather than `0.1.0-dev`),
  `go install` with `GOPRIVATE` set, and clone-and-build. A new paragraph
  states plainly that every path needs GitHub access, since the repository
  being private means none of them are anonymous — a plain `curl` against a
  release asset returns 404 (established in M25-002).

- **The Homebrew tap two shipped security documents described does not
  exist.** `docs/security/info-sec-policy.md:18` and
  `docs/security/pentest-2026-Q2.md:45` both listed a Homebrew tap as part
  of the CLI's distribution chain. No tap exists and `.goreleaser.yaml` has
  no `brews:` block. Both are corrected, pointing at a new decision record
  (below) rather than silence.

### Added
- **The version resolver is proven by five layers of test, not asserted
  once.** Pure table tests (`TestVersion_falls_back_to_build_info`, 17 rows)
  cover the resolver's contract with no I/O; a `-buildvcs=false` build
  deterministically reports `(devel)` and is asserted byte-for-byte against
  the default, replacing `TestVersion_default_when_not_injected`'s old
  assertion, which had passed identically for the placeholder, a real
  version, and `(devel)` alike; a wiring test crosses the process boundary
  and reproduces `--version` from the binary's own `debug/buildinfo` read; a
  real tag, a `git init`-from-working-tree fixture (never `git clone`, which
  would reflect HEAD instead of uncommitted work, or be shallow and lose Go's
  tag stamping; never `git worktree`, which would tag this repository for
  real) and a real build prove the end-to-end path that `go install
  <module>@<tag>` cannot exercise here, since the repository's only tag
  (`v0.1.0`) predates this fix; and a three-surface agreement test holds
  `--version`, `--help` and `info --format json` to one string for the same
  run.

  Two of Go's three canonical pseudo-version forms were caught only by
  testing them directly during implementation, not by inspection: an initial
  regex matched the untagged-repo form (`v0.0.0-<timestamp>-<hash>`) but not
  the more common commit-after-a-release-tag form
  (`vX.Y.(Z+1)-0.<timestamp>-<hash>`), which inserts a `.` rather than a `-`
  immediately before the timestamp — caught by the table test built to cover
  exactly this case. Both forms are now rejected, along with the third
  (pre-release-base) form the original table did not name.

  `TestReadme_install_commands_execute` (`cmd/curlew/readme_install_exec_test.go`)
  now attributes a released-version output specifically to the block whose
  body contains `gh release download`, rather than to any block producing
  one. Of the README's three install blocks, only that one runs the binary
  it produces — "Install from source" ends at `go install` and "clone and
  build" ends at `go build`, and neither invokes `--version` — so a match
  can only ever come from the download block, at any tag: verified by
  running the other two blocks in isolation and observing no version output
  from either. `TestReadme_documents_a_binary_download` now checks that same
  assumption directly (`readme_install_test.go`), so it is enforced rather
  than only asserted in a comment.

- **Every README install command is now executed by a test, not merely
  read.** `TestReadme_install_commands_execute`
  (`cmd/curlew/readme_install_exec_test.go`) extracts every fenced bash/sh
  block under `## Install` and runs each as a whole script (`bash -euo
  pipefail`) in its own temp directory, so a `cd curlew` on one line affects
  the `go build` on the next — exactly what a reader copying the block would
  experience. Two vacuity guards run before any command does: zero blocks (a
  renamed or deleted heading) and fewer than three (the measured floor on
  this tree) both fail the test rather than silently executing nothing. A
  fence tagged anything other than `bash`/`sh` under `## Install` is also a
  failure, not a silent skip — there is no way to park a broken command in a
  fence tagged `text` and exempt it. Behind `//go:build readme_install`, the
  same pattern M25-002 established for `release_artifacts`: it costs several
  seconds and needs the public internet, an authenticated `gh`, and git
  credentials for a private repo, none of which the three routine `go test`
  passes in `ci-local.sh` should acquire. `scripts/ci-local.sh` names it as
  an unconditional step of its own — probing for `gh` and failing loudly if
  it is absent, then a `-list` vacuity guard using an anchored test-name
  alternation rather than a bare `^TestReadme` prefix, which would also
  catch `help_parity_test.go`'s pre-existing
  `TestReadme_lists_every_command_the_CLI_advertises` (measured: the prefix
  returns 4 matches, the alternation exactly 3).

  `TestReadme_documents_a_binary_download`
  (`cmd/curlew/readme_install_test.go`, untagged and hermetic) holds the
  section's prose to two source-of-truth files instead of hand-typed
  strings: the documented archive name and extension (including the Windows
  `.zip` override) are rendered from `.goreleaser.yaml`'s
  `archives[0].name_template` and asserted against the README text, and the
  `go install` line and the download URL's `owner/repo` are asserted against
  `go.mod`'s module path. The extractor (`readmeSectionBlocks`,
  `readmeSectionText`) is a fresh, purpose-built markdown walker rather than
  a reuse of `skill_commands_test.go`'s `codeSpansWithLines`, which discards
  the fence language and would not distinguish an executable block from one
  tagged `text`.

  Verified in this session by three mutations, each applied and reverted
  with the working tree confirmed clean afterward, plus two direct checks.
  Reverting `README.md` to its pre-fix content (two command blocks, not
  three) fails the exec test's floor guard before it attempts to run
  anything: `found 2 command block(s) under '## Install'; measured 3 on
  this tree`. Changing the `go install` line's module path to
  `github.com/wrong/curlew` fails only the "go install line matches the
  go.mod module path" subtest; changing `.goreleaser.yaml`'s
  `name_template` separator fails only the "documented archive name matches
  name_template" subtest — every other subtest stays green in each case,
  showing the checks are independent rather than one masking the others.
  Separately: running the real (fixed) README executes all three blocks in
  6.55s–8.43s across two runs, and the download block's output matched a
  released version (`curlew X.Y.Z`, no `-dev`/`-snapshot` suffix) — proving
  the download-and-run claim rather than assuming it. And
  `management/tasks/M25-003.yaml`'s own observable command needed the same
  correction M25-002's did: run without `-tags readme_install`, it exits 0
  with `[no tests to run]` — the identical vacuity.

- **The package-manager decision is recorded rather than silent.**
  `docs/TECH_CHOICES.md` gains a `### Distribution` section under `## CLI —
  Go`: no Homebrew tap yet, and the reason is disqualifying rather than
  cautious — a tap's formula fetches its release asset anonymously, and this
  repository is private, so an unauthenticated request for the v0.1.0 asset
  returns HTTP 404 (established in M25-002). Revisit when the repository is
  public and a second release exists. The stale "Release and versioning
  strategy… cadence is TBD" bullet under "Decisions Not Yet Made" is also
  refreshed: the mechanism is decided and executed (v0.1.0 was tagged
  2026-08-16); only cadence remains open.

### Added
- **A debt register for checkable claims stated in sentences, not rows.**
  Defect 8 — a markdown correlation ID reading `id=-iter-0` while the events
  stream emitted `req-1` — sat directly beside a table that had already been
  executed. It survived because the promise it broke was a sentence, and
  `docs/table-execution-baseline.txt` only ever tracked tables.
  `internal/docs/prose.go` (new) extracts checkable claims from `MANUAL.md`
  and `CLI_SPECIFICATION.md`: a block filter drops fenced code, tables,
  headings, blockquotes and HTML comments; hard-wrapped paragraphs are
  reflowed to one line (load-bearing — the defect-8 sentence at
  `MANUAL.md:2315` wraps across five source lines); sentences are split on
  `.`/`!`/`?`/`;`, guarded against abbreviations, bare initials, mid-number
  decimals and punctuation inside an open backtick span; and a sentence
  becomes a claim only when it matches one of four shapes (`modal`,
  `same-as`, `written-appears`, `given-curlew`) **and** names a concrete
  referent — a flag, a `CURLEW_*` variable, an exit code, a filename, a
  `curlew ...` command, or any backticked code span.

  103 claims were extracted (`MANUAL.md` 70, `CLI_SPECIFICATION.md` 33).
  Three are exempt by a `<!-- doc-check: prose-not-executable <why> -->`
  marker (cap 12, same governance as the table register's cap of 8): the
  specification's own completeness statement about itself, a narrative line
  about the request-file format's generality, and rationale recounting a
  past documentation mistake — none states anything the binary can be asked
  to demonstrate. Fourteen are executed in this same change via a new
  `docs.Prose(doc, substring)` reader — resolved from test sources by a new
  `docs.ProseClaims` AST walk, exactly the way `docs.Claims` derives the
  table register, so the link from test to claim is never a hand-kept list —
  prioritising the defect-8 family (`TestMarkdown_correlationIDsSurviveEveryRetentionPolicy`
  now names the three correlation-ID sentences it already proved), redaction
  invariants (the HMAC redaction-trigger and `[REDACTED]`-propagation
  claims), colour/stream discipline (`TestStreamDisciplineMatrix` adopts
  four claims it already enforced), exit codes (`pr-check`'s 0/1/2
  contract), and determinism under `--seed`
  (`TestDocTables_seededExamplesReproduce` adopts two). The remaining 86
  claims are debt, listed in `docs/prose-claim-baseline.txt` with the count
  stated in its header.

  Same shrink-only contract as `docs/table-execution-baseline.txt` and
  `testapi/harness/redaction-known-leaks.txt`: `TestProse_inventory_is_complete`
  fails the build on new debt (an unexecuted claim missing from the
  register), on stale debt (a registered claim that is now executed and
  whose line was not deleted), and on zero claims extracted from either
  document — the M22-001 lesson that an empty result must never read as a
  clean register. `TestProse_register_cannot_grow` proves the new-debt and
  stale-debt guards by mutation against synthetic documents, independent of
  the real files' current content. `AuditProse` takes slices and a map
  rather than reading the filesystem specifically so that proof is possible.

  Precision measured directly against the two real documents rather than
  estimated: of 103 extracted claims, only the 3 now under a marker read as
  false positives on inspection — the extractor's recall is deliberately
  narrower than its precision (a sentence using "not" rather than "never",
  for instance, is left uncaught), which is the same trade-off the task
  description asked for: "a narrow extractor that catches real claims beats
  a broad one whose register nobody empties."

## [0.1.0] — 2026-08-16

The first released build. Every prior version of this tool reported
`0.1.0-dev`, because no tag existed and no archive had ever been produced.

### Fixed
- **The shipped agent skill stops describing a licensing system that was
  deleted.** `curlew init --skill agent` writes `.claude/skills/curlew/`
  into a user's own repository, where an agent reads it and acts on it
  literally. Four of its files still described the five-tier licensing
  system removed on 2026-08-03: `SKILL.md`'s failure playbook and
  `exit-codes.md`'s master table and numbered list each documented exit 6
  ("Feature gate denied") and exit 9 ("License grace period expired"),
  `failure-playbook.md` carried a full two-section remediation ending in
  "suggest running `curlew license --validate`" — a command that does not
  exist — and `output-formats.md` claimed HTML output "requires
  Professional tier" while `internal/output/config.go` lists it
  unconditionally. That fourth file was not named in the task that started
  this work; it surfaced only once the fix was held to a source-derived
  check rather than to the task's own hand-written file list.

  The binary's actual exit-code set is `{0, 1, 2, 3, 4, 5, 130}`, not
  `{0, 1, 2, 3, 4, 5, 6, 9}`: the skill invented two codes and omitted one.
  130 (128+SIGINT, `cmd/curlew/perf.go`'s `perfCmdOut`, a `curlew perf` run
  cancelled by Ctrl+C) is not a new claim — `docs/MANUAL.md` and
  `docs/CLI_SPECIFICATION.md` already published it, and
  `TestPerfCmd_ContextCancelExitCode130` already covered it behaviourally —
  the skill was the sole surface still missing it. All four exit-code
  statements now list `{0, 1, 2, 3, 4, 5, 130}`, including a new Exit 130
  row/section explaining that the run was cancelled, not failed, and that
  the partial summary already on stdout should be reported as partial.

  Held to the binary going forward by a new `internal/exitcodes` package
  rather than by a hand-maintained list: it parses `cmd/curlew`, roots a
  call graph at `runWithWriters`, and collects the integer literals in
  return statements reached only through a return-position call or a local
  identifier resolved back to one (`code := runErrorExitCode(varErr);
  return code, summary`, the pattern a naive call-graph walk misses).
  `cmd/curlew/discovery_run.go`'s dead `6: 6, // feature gate` map entry
  (left by the same removed system, out of scope here as M26-002) is
  excluded structurally rather than by an exception list: a composite
  literal's key and value are never a return statement. Four new CLI tests
  read the skill's contract from a freshly scaffolded project (not from
  `templates/` directly) and check it against that derived set in both
  directions, against each other, and against `runWithWriters`'s dispatch
  switch for every `curlew <subcommand>` the skill names in a code span —
  scoping extraction to markdown code rather than prose removes the need
  for `doc_prose_test.go`'s 60-entry `nonCommandWords` blocklist, since a
  prose sentence like "driving curlew with an agent" is never wrapped in
  backticks. A fifth new test holds `output-formats.md`'s format table to
  `output.SupportedFormats` the same way `doc_name_tables_test.go` already
  holds the manual and the CLI spec.

  Verified by mutating three ways and confirming each restores cleanly
  afterward. Inventing a code (`| 7 | ERR_FICTION | ... |` added to
  `exit-codes.md`'s master table) fires two failures at once: `exit-codes.md
  (master table) documents exit 7, which cmd/curlew cannot return
  (reachable: [0 1 2 3 4 5 130])` and the four-statement agreement check.
  Omitting a real code shows the two guards are independent: deleting only
  `exit-codes.md`'s 130 row (leaving it in the other three files) trips the
  agreement check alone (`exit-codes.md (master table) lists [0 1 2 3 4 5];
  SKILL.md (failure playbook) lists [0 1 2 3 4 5 130]`); deleting it from
  all four files makes the four statements agree with each other again but
  still fails the binary comparison (`cmd/curlew can return exit 130
  (perf.go:142, in perfCmdOut) but no skill file documents it`). Adding a
  statically-reachable, dynamically-unreachable `case flags.vus < 0: return
  7` to `perfCmdOut` (guarded by a negative `--vus` that `parsePerfArgs`
  already rejects, so no behavioural test changes outcome) proves the
  AST walk itself reaches `perfCmdOut`, not merely that 130 came from
  somewhere else: `cmd/curlew can return exit 7 (perf.go:142, in
  perfCmdOut) but no skill file documents it`.

- **Every exit-code surface is now held to one source-derived set, and the
  last structural trace of the licensing strip is gone.**
  `cmd/curlew/discovery_run.go` ranked exit `6` ("feature gate") in
  `exitCodeSeverity` even though nothing in the binary can return it — the
  map's own comment claimed an ordering ending "< 6" that no branch could
  reach. `docs/UI_SPECIFICATION.md` published exit `9` ("license grace
  period expired") in its §2.4 table and described a startup step and a
  dispatch-wiring bullet both calling a `checkGraceExpiredTo` that
  `grep -rn 'checkGraceExpired' --include='*.go' .` finds nowhere — the same
  licensing residue M26-001 removed from the skill, one document further
  out, and not one of the four surfaces the task named.

  All four *named* surfaces — `docs/CLI_SPECIFICATION.md` §17,
  `docs/MANUAL.md` §4.3 and appendix D, and the agent skill — already
  agreed with the binary's actual set, `{0, 1, 2, 3, 4, 5, 130}`, when
  measured on this branch. Their new guard, `TestExitCodes_all_surfaces_agree`,
  was therefore green on arrival for all four; the mutation harness below is
  the only evidence it can fail at all. It reuses `internal/exitcodes`
  (M26-001) rather than building a second extractor, and
  `documentedExitCodes` (`doc_exit_codes_test.go`) rather than a second
  table reader — the one signature change is a column-name parameter,
  since `docs/UI_SPECIFICATION.md`'s per-command table heads its code
  column "Exit", not "Code". That table and `docs/plugins.md`'s are two
  more per-command surfaces, asserted by containment only: a table naming a
  subset of the contract is correct by construction, not incomplete.

  Three more guards close paths the four-surface check does not reach.
  `TestExitCodes_no_unreachable_mapping` asserts every key in
  `exitCodeSeverity` is reachable — 130 stays deliberately unranked, since
  it is raised inside a single `perf` run and never reaches the worst-wins
  fold across discovered collections, and forcing it a rank would be
  inventing an exit code's meaning.
  `TestExitCodes_skillProseNamesOnlyReachableCodes` sweeps all eleven
  scaffolded skill files for a prose "code N" mention — `assertions.md`,
  `variables.md`, and `vault.md` each make one — rather than trusting the
  four table/list/heading statements M26-001 already pinned.
  `TestExitCodes_everyExitCodeSectionIsRegistered` sweeps `docs/` (rooted at
  `docs.Dir`, not a repo-wide glob, since a second checkout under
  `.claude/worktrees/` would otherwise contribute a phantom copy of every
  document) for exit-code section headings and fails on any the registry
  does not classify — the analogue of `internal/schema/parity_test.go`'s
  parser-struct coverage test, and the reason a sixth surface can't repeat
  what happened here. `docs/SPECIFICATION.md`'s exit-code table, including
  its own `6` row, and `docs/history/IMPROVEMENT.md` stay excluded, each
  with its reason recorded in code rather than a commit message: the former
  is platform-scoped by its own 2026-08-04 scope note, the latter is an
  archived record.

  Verified by six mutations, each applied and reverted under a
  `trap ... EXIT INT TERM` with `git status --porcelain` confirmed empty
  afterward. Making the binary return a new code (`case flags.vus < 0:
  return 7` in `perfCmdOut` — statically reachable, dynamically
  unreachable, since `parsePerfArgs` already rejects negative `--vus`)
  fails all four full-contract surfaces at once, each naming `perf.go:142`
  by file and line, plus M26-001's own skill guard. Inventing exit `7` in
  `docs/CLI_SPECIFICATION.md` §17 fails that surface's own equality check
  and the pre-existing pairwise `TestDocTables_theThreeExitCodeTablesAgree`
  (`CLI_SPECIFICATION.md ... lists [0 1 2 3 4 5 7 130]; MANUAL.md ... lists
  [0 1 2 3 4 5 130]`). Dropping the real `130` row from all three doc
  tables at once is the case that matters most: pairwise agreement between
  the documents passes — they now agree with each other, having all gone
  wrong together — but `TestExitCodes_all_surfaces_agree` still fails on
  each of the three independently, because the reference is the binary,
  not a peer document. Inventing exit `7` in the skill's master table fails
  the new skill subtest plus both of M26-001's own guards. Adding a `` `6`
  feature gate `` clause to `CLI_SPECIFICATION.md` §21's prose sentence
  reproduces the original defect shape and is caught by the new
  per-command reader. Appending an unregistered `### Z. Exit codes for
  something new` section to `docs/MANUAL.md` is caught by the coverage
  guard: `docs/MANUAL.md:3950 "### Z. Exit codes for something new"
  publishes exit codes but no surface in exitCodeSurfaces() reads it`.

  Two gaps found and deliberately left alone, each recorded rather than
  silently skipped: `docs/CLI_SPECIFICATION.md` §20 omits exit `5` from its
  `ui` prose sentence though `cmd/curlew/ui.go:149` returns it (an
  omission, not a false claim, so containment still passes); and
  `docs/MANUAL.md`'s `pr-check` sentence outside §4.3 is phrased without
  the `Exit codes: ` anchor the per-command reader requires. Both are
  noted as follow-up candidates in `management/plans/M26-002-plan.md`
  rather than fixed here, since fixing either would need an unproven
  per-command reachable-code walk this task does not build.

### Added
- **The gate now builds all six release targets and opens what they
  produced, not just one binary for the host running it.** M25-001 (previous
  entry) proved *a* release build works; it single-targeted the host
  platform, so LICENSE/NOTICE presence, the version injection, and every
  binary's format were never checked for the other five targets a user might
  actually download. `ci-local.sh --go` gains one more step, after the
  existing single-target checks and deliberately after the `release_bin_count`
  guard that requires exactly one file named `curlew` under `dist/` — a
  six-target release leaves four (`linux`×2, `darwin`×2) plus two named
  `curlew.exe`, so ordering matters here, not just presence.

  Two mutations shaped what gets asserted and why a config-derived check
  alone is not enough. **Deleting `- NOTICE` from `.goreleaser.yaml`'s
  `archives[].files` list** — dropping the file Apache-2.0 §4(d) requires to
  travel with the distribution — produces six *valid* archives at exit 0,
  simply missing NOTICE; a check that derives its expected-file list from the
  same config that produced the archive agrees with itself and passes. The
  new step therefore checks every archive against **two** independent
  sources: `.goreleaser.yaml` itself (catches a target or file goreleaser
  silently failed to include despite declaring it) and a hardcoded floor of
  six targets and six required files that does not shrink when the config
  does (catches exactly the NOTICE mutation, with a message naming
  Apache-2.0 explicitly). **Deleting the `NOTICE` file entirely** (rather
  than un-declaring it) makes goreleaser fail loudly on its own — but leaves
  behind 32-byte `.tar.gz` and 22-byte `.zip` stubs in `dist/` that Go's
  `archive/tar` and `archive/zip` read as *zero entries with no error*. Every
  archive's entry count is asserted positively for exactly this reason: a
  test that only checked "did it open without an error" would have passed on
  six archives containing nothing.

  Per archive: extension and binary entry name match the target
  (`curlew`/`curlew.exe`), the binary entry's mode is `0755` (a `0644` binary
  extracts unrunnable and nothing else would have noticed), `debug/buildinfo`
  confirms `GOOS`/`GOARCH`/`CGO_ENABLED=0` match the filename, and a
  `debug/elf`/`debug/macho`/`debug/pe` parser — chosen by the *expected* GOOS,
  so a windows archive holding an ELF binary fails on the parse rather than a
  generic magic-byte error — confirms the machine field matches the GOARCH.
  Linux binaries are additionally checked for the *absence* of an ELF
  `.interp` section, which is what "requires no dynamic linker" actually
  means; this is deliberately not a `Type == ET_EXEC` check, since a
  statically linked binary built with a future `-buildmode=pie` default would
  legitimately be `ET_DYN` with no `.interp`, and a `Type` check would then
  fail on a harmless build-mode change while the binary stayed exactly as
  static as before. Each archive **file's** own filesystem mtime (never an
  entry's — those are pinned to the commit timestamp by `mod_timestamp` for
  reproducibility and would not reflect a fresh run) is checked against the
  moment the test invoked goreleaser, guarding against a stale `dist/`
  somehow surviving `--clean`.

  The version chain is checked at every hop — archive filenames,
  `checksums.txt`, and every binary's `buildinfo` `-ldflags` — against
  `cmd/curlew/main.go`'s `version` symbol directly (never the literal
  `"0.1.0-dev"`, so the comparison cannot drift if the default ever changes;
  the literal already matches the semver shape regex anyway, which is why the
  regex alone was never the assertion). The `-X main.version=` flag is
  matched as an exact, case-sensitive literal — never a substring or
  case-insensitive test — because the linker silently drops `-X` for a
  symbol that does not exist: `-X main.Version=` (wrong case) would leave
  every binary reporting the development default while `buildinfo` still
  faithfully echoed back the flag text it was given, and a loose match would
  have called that "injected" anyway.

  `buildinfo` alone is blind to one regression: `version` changing from a
  `var` to a `const`, which the linker also silently ignores while
  `buildinfo` keeps recording the flag it was never able to apply. Only
  executing a binary catches that, and only one of the six can run on any
  given host — this gate's darwin/arm64 runner executes the darwin/arm64
  archive; a Linux CI runner would execute a different one. Rosetta makes
  darwin/amd64 *also* runnable here, and that capability is deliberately
  unused: a check that passes only when a translation layer happens to be
  installed is a conditional pass, not a property of the artifact. The other
  five targets stay covered by `buildinfo` and the format/machine parse
  above — stated honestly as five-of-six-by-static-inspection,
  one-of-six-by-execution, rather than implying full execution coverage that
  doesn't exist on a single host.

  `checksums.txt` is re-hashed with `crypto/sha256` rather than trusted, and
  checked in both directions — every archive has a line, every line names an
  archive that exists — with cardinality asserted **first and independently**
  of either direction: a release that produced zero archives and no
  `checksums.txt` would otherwise satisfy both directions vacuously, having
  had nothing to range over.

  Cost is cache-sensitive: measured standalone at 6.5s with a warm Go build
  cache (this host, after repeated same-day invocations); the six-target
  `goreleaser release --snapshot --clean` alone measured 28s earlier the same
  day on a colder cache. Behind `//go:build release_artifacts` so it stays
  out of the three routine `go test` passes `ci-local.sh` already runs
  (plain, `-race`, `-coverprofile`) — tripling either figure for no added
  coverage, since none of what this drives is concurrent code this package
  owns. The tag is not an escape hatch: `ci-local.sh` names the step
  unconditionally on every `--go` gate, the same way the M25-001 release
  steps are unconditional. `.golangci.yml` gained a `run.build-tags` entry so
  the tagged file is still linted — proved, not assumed, by a real lint pass
  that caught five genuine `errcheck` violations and one `gofumpt` issue in
  the file on first write (fixed), followed by a planted-and-removed
  synthetic violation confirmed caught and the tree confirmed clean
  afterward.

  No tag was created and nothing was published. `git tag -l` stays empty;
  cutting `v0.1.0` remains a deliberately deferred, human-reviewed step (see
  `management/plans/M25-002-plan.md`).

- **The release build is now executed on every gate, not trusted.**
  `.goreleaser.yaml` shipped in #14 and had never been run once: no tag
  existed, `goreleaser` was not installed, and `ci-local.sh` never mentioned
  it. Every property the config claimed — static `CGO_ENABLED=0` builds, `-X
  main.version` injection, LICENSE and NOTICE carried into every archive for
  Apache-2.0 §4(a)/§4(d) — was a claim about a build nobody had performed.

  `ci-local.sh --go` now runs four steps between `smoke` and the dogfood gate:
  confirm `goreleaser` is installed and at least major version 2 (parsed off
  the `GitVersion:` line, not an unanchored semver grep — which also matches
  the reported Go version), `goreleaser check`, a single-target `--snapshot`
  build for the host, and — the step that actually matters — execute the built
  binary and assert on its `--version` output. Measured cost is ~2.1s (`check`
  0.07s, the snapshot build ~2s), so the step runs unconditionally. A missing
  `goreleaser` fails the gate with the install command rather than skipping
  the step and reporting PASS — the same false-clear M22-001 exists to
  prevent, applied to the release path.

  First-ever execution of `.goreleaser.yaml` produced a real artifact:
  `curlew 0.0.1-snapshot` at `dist/curlew_darwin_arm64_v8.0/curlew`.

  Verified by mutating the config three ways and confirming each restores
  cleanly afterward: an invalid `goos` value is caught by `goreleaser check`;
  deleting the `-X main.version` ldflag entirely passes `check` and `build`
  and is only caught by the `--version` assertion — the failure the task
  exists to prevent, since a binary silently reporting the `0.1.0-dev` default
  is exactly what would otherwise ship. A third mutation (an undefined
  template variable in `ldflags`) fails at the **build** step itself, as
  originally expected: `goreleaser build --snapshot --clean --single-target`
  exits 1 with `map has no entry for key "NoSuchVar"` and leaves `dist/` with
  zero files — it does not link a binary, silently or otherwise. `goreleaser
  check` alone is confirmed **not** sufficient on its own: it does not catch
  either template problem, only structural ones such as the invalid `goos`
  value above.

  **Correction to an earlier revision of this entry.** It previously stated
  the opposite of the paragraph above — that `goreleaser build --snapshot`
  "accepts [the undefined template variable] silently and links a binary
  carrying the same `0.1.0-dev` default instead." That claim is false and
  does not reproduce. `management/plans/M25-001-plan.md` retracted the same
  claim during `/execute`, after it failed to reproduce across repeated runs,
  but this file was not corrected to match until now — and nothing automated
  would have caught the drift, since CHANGELOG.md is deliberately excluded
  from this project's doc-prose and doc-table checks. Recorded as a
  correction rather than silently overwritten: a task whose whole argument is
  that unexecuted claims are worthless cannot itself carry a changelog entry
  that quietly rewrites its own measurement.

  `.github/workflows/go.yml` and `release.yml` — the two workflows that
  delegate to `ci-local.sh --go` — now install `goreleaser` first;
  `release.yml` uses `goreleaser-action`'s `install-only` mode rather than
  `go install`, since it pins Go 1.24 while goreleaser v2.17.1 requires
  Go >= 1.26.5.

  Nothing here publishes: no tag, no `goreleaser release`, no push. Cutting
  `v0.1.0` is M25-002.

- **`docs/PRODUCT_ROADMAP.md`, and M25–M29 in the backlog (12 tasks).** M1–M24
  closed the backlog for the fourth time. Each of the four campaigns —
  feature-gating stripped, backend stripped, post-strip drift closed,
  documentation tables executed — worked the same way: inventory what has not
  been checked, make the inventory a build-failing register, empty it. The last
  one found eight defects, including a run reporting `3 passed, 0 failed` and
  exiting 0 on a suite with a failing assertion.

  The method had never been pointed at the *product* surface, and an audit on
  2026-08-14 found the results were roughly what that predicts:

  - **No release has ever been built.** `.goreleaser.yaml` is 94 careful lines
    — static builds, `-X main.version` injection, LICENSE and NOTICE carried
    into every archive for Apache-2.0 §4(a) and §4(d) — and nothing has ever
    executed it. `goreleaser` is not installed, `ci-local.sh` never invokes it,
    there is no tag, and `--version` reports `0.1.0-dev` on every machine that
    has ever run the binary. (M25)
  - **The shipped agent skill is wrong.** `curlew init --skill agent` writes
    `.claude/skills/curlew/` into the user's own repository, where Claude Code
    and Copilot read it. Three of its files still document the deleted
    five-tier licensing system: exit 6 "feature gate denied", exit 9 "license
    grace period expired", and `curlew license --validate`. The binary returns
    0–5 and has no `license` command. An agent obeys that file literally. (M26)
  - **Zero fuzz targets and zero property-based tests**, across a YAML parser,
    a variable interpolator, a JSONPath implementation and a CEL evaluator —
    every one of them fed by input the project did not author. (M27)
  - **`src/` is 139,627 lines of C# across 866 files** that the shipped CLI
    does not call — larger than the entire Go CLI at 134,144 — plus a 186-file
    dashboard. More than half the repository, unexplained in the README. (M28)
  - **Nothing runs on push.** All eight workflows are `workflow_dispatch`-only,
    so every "the gate is green" claim means "green when someone last ran it".
    (M29)

  The manual and the CLI specification were audited in the same pass and are
  accurate: removed features appear only in explicit "not in this CLI" and
  migration sections, which is correct.

- **`--color={auto|always|never}`.** `--no-color` could turn colour off and
  auto-detection could turn it on when safe, but there was no way to say "on
  anyway" — piping a run to `less -R`, or capturing coloured output in CI, both
  need colour on a writer that is not a TTY.

  Accepted by `run`, `watch`, `validate` and `exec`, in both `--color always`
  and `--color=always` forms. `--no-color` is retained and means exactly
  `--color=never`. An invalid value is rejected with the alternatives named.

  An explicit `--color` takes precedence over `NO_COLOR`: the variable is a
  standing preference, the flag is a decision for this invocation, and the more
  specific one wins — as in `git`, `grep` and `ripgrep`. Under `auto`, a
  non-empty `NO_COLOR` decides.

  `--color=always` **cannot** put escape codes into a machine format's payload.
  `json`, `tap`, `junit`, `markdown` and `html` never construct a terminal
  printer for stdout, and a test pins that for all three of the first formats so
  a consumer piping JSON can always parse it.

  Collection discovery now forwards the mode to each discovered collection
  rather than only being able to forward "off".

  This was documented before it existed: the manual described the flag with
  three worked examples against a binary that answered `unknown flag:
  --color=never`. The documentation was corrected first, then the flag built.

- **Prose is now held to the binary by the names it uses.** A sentence cannot be
  executed, but almost every prose claim worth making names something concrete —
  a command, a flag, an environment variable — and a name is checkable even when
  the sentence is not. Every `curlew <command>`, every `--flag` on a line naming
  curlew, and every `CURLEW_*` variable in the manual and the specification must
  now exist in the binary or be read by the source.

  This targets drift the project has actually suffered rather than a
  hypothetical: the licensing and backend strips removed `curlew license`,
  `curlew login`, `curlew worker`, `--workers`, `--report-upload` and every
  `CURLEW_BACKEND_*` variable, and M21-002 was four manual surfaces still
  describing the removed backend, found by reading months later.

  Sections that name removed things on purpose carry an explicit
  `<!-- doc-check: ignore-names -->` marker. A marker cannot outlive the next
  heading, so none can blanket a document, and their total is capped so the
  checks cannot be hollowed out a section at a time.

  It found a live defect at once: the manual documented
  `--color={auto|always|never}` with three worked examples and called
  `--no-color` an alias for `--color=never`. **No `--color` flag exists** — the
  binary answers `unknown flag: --color=never`. The section was corrected first,
  and both halves of the discrepancy were then closed the other way, by building
  what the document described — see the `--color` entry above and the `NO_COLOR`
  entry below.

- **`help_parity_test.go` now sees flags accepted outside a switch.** It derived
  the accepted set from `case "--flag":` clauses alone, so `--clear` on
  `curlew watch` — accepted by an `if` — was invisible to it.

- **The documents' behavioural tables are now executed, not just written.** A
  document states things about the binary in three ways — examples, tables and
  prose — and only examples were checked. A table is not a snippet: no parser
  will ever reject one, so a table can promise behaviour the binary does not
  have and the build stays green. That is what §11C.2 was, in two documents, for
  as long as it existed.

  `internal/docs` reads a markdown table so tests can run what it claims. The
  GraphQL outcome × mode matrix is executed cell by cell against the runner —
  and the two documents must agree with each other first, since they describe
  one binary. The body and header operator catalogues are held to the evaluator
  in both directions, with the implemented set read out of the source via
  `go/ast` rather than restated in the test, because a restated list is a second
  thing to forget to update. The WebSocket step-field table is held to the
  parser's own map.

  Each check was verified by a canary rather than trusted because it passes:
  reverting the §11C.2 fix fails twelve matrix cells by name, adding an
  undocumented operator fails the parity test, and drifting the step-field table
  fails with both lists printed.

  Turning the checks on immediately found one more: both documents claimed
  "Thirteen operators" above a table listing **fourteen**. The count is now
  checked against the table it introduces.

### Changed
- **A WebSocket step field that its action ignores is now a parse error.** Each
  action reads only its own fields — `wait` reads `duration_ms`, `expect` reads
  `timeout_ms` — and the parser accepted either for any action, decoding into a
  field the executor never consults. A `wait` given a `timeout_ms` paused for
  zero milliseconds and said nothing about it, which is how the example in
  `CLI_SPECIFICATION` §12.3 survived being written down twice. The error names
  the field, the action, and where that field does belong; unknown fields are
  rejected too.

  This is what made the documentation checkable. An example can parse cleanly
  and still describe behaviour the binary does not have, so extending the
  example test to the specification would not have caught it on its own — the
  parser had to start refusing the mistake first. Verified with a canary:
  reintroducing the defect fails the test with the file, the line and the fix.

- **Every table in both documents is now accounted for.** "Tables are executed"
  has been true since #28 only of the three tables someone had wired; there were
  77, and 70 of them stated something about the binary that no test ran. A count
  of executed tables cannot close that gap — only a count of *unexecuted* ones
  can, so `internal/docs` now takes an inventory of every table in the manual and
  the specification and requires each to be executed, declared unexecutable in
  the document with a capped marker, or listed in
  `docs/table-execution-baseline.txt`.

  The baseline is a debt register, not an exemption list, and it can only shrink:
  an unexecuted table missing from it fails the build, and a listed table that
  has become executed fails until the line is deleted. Same contract as
  `testapi/harness/redaction-known-leaks.txt`, which started at thirteen lines
  and emptied itself. **This one is empty too.** It started at 70 and every
  entry has been paid off; empty means the next table anyone adds must arrive
  with a test or a marker, because there is no longer a list to append to.

  Which tables a test reads is derived from the test sources by `go/ast` rather
  than from a hand-kept list, because a list of "tables we execute" is one more
  document about the binary and would drift like the ones it guards.

  Now executed, 72 of 77 — the other five carry a marker, against a cap of
  eight. The last fifteen were the ones no reading could settle: the ten-rung
  precedence ladder, proven pair by pair with the same variable defined from
  both rungs; the phase and protocol tables, proven by a server that counts how
  many requests are in flight at once, because sequential and parallel produce
  the same output in the same order; every file-reference rule, run with a
  decoy of the same name in the working directory so that resolving from the
  wrong place finds a file rather than nothing; §11.4's four rejected
  constructions, each paired with a control that must run; the glob tokens,
  proven by what they must *not* match; §18.8's 3.1 translations, warnings
  included; the iteration variables, `store_results`, the body-file
  Content-Type tables, the CEL sites, and the markdown rendering matrix.

  Already executed before that, 53 of 77: the eleven flag tables (85 mentions, every one accepted
  by a parser), the twelve dynamic-function tables in both directions, the three
  exit-code tables, both output-format tables and both `output:` block tables,
  the telemetry subcommands, the limits table against the constants the binary
  enforces, the three backoff tables (formulae evaluated, worked sequences
  reproduced, jitter bounds computed), the locale table in both directions, the
  date-layout table rendered, the interpolation forms matched against the
  engine's own patterns, both WebSocket action tables against the parser's field
  map, both plugin-hook tables against the payload structs, the signer types
  against the registry, and the vault providers against their constants.

  Nothing remains in the register.

  It found fourteen live defects, below.

### Fixed
- **A retention policy was deciding which of a result's fields exist.**
  `filterDataDrivenResults` rebuilt `RequestResult` field by field, as a
  whitelist, so every field added to the struct after the filter was written was
  silently absent from a `summary` or `failed_only` result — nine fields under
  the first, eleven under the second.

  Two of them mattered. `AssertionResults` is the entry below: losing it
  reported a failing run as passing. `RequestID` and `RequestSlug` are this one:
  the markdown report's correlation sentinel came out as `id=-iter-0` while the
  events stream for the same run still named `req-1`, so §4.1a's promise that
  the two link a specific event line to a specific markdown file was false under
  two of the three policies — and false in the worst available direction, since
  the event names a file and the file cannot say which event it belongs to.

  `stripResponseDetail` inverts the whitelist: copy the result, clear the
  response and the per-attempt record of obtaining it. A field added tomorrow
  survives by default. `Warnings`, `RetryWarnings` and `RetryCount` come back
  with it — a count is not detail, and "aggregate counts only" is what the row
  asks for. A reflection guard pins the rule rather than the field list, which
  is what makes the whitelist unnecessary rather than merely longer.

- **`store_results: summary` turned a failing run green.** §10.3 heads its
  column "Retained": a policy decides what a finished run keeps, never what the
  run was. `summary` dropped the assertion results outright, and the summary
  counter reads exactly that field — so three iterations, one of them failing
  its status assertion, were reported as "3 passed, 0 failed" and the run
  exited 0.

  The CLI's own out-of-memory hint recommends the flag ("set `store_results:
  summary|failed_only` to bound what the run retains"), so the advice for a
  large suite was also the way to stop noticing it was broken. `failed_only`
  was unaffected: it keeps failing iterations whole, verdict included.

  Each iteration's verdict now survives the filter without its per-assertion
  detail, which is what "aggregate counts only" means.

- **`{{_count}}` was documented in both documents and never existed.** §10.2 and
  §5.5 name three built-in iteration variables; the injector defined `_index`,
  `_total`, `_iteration` and `_row_number`. A collection written from either
  table died with `undefined variable "_count"` before its first request.

  `_count` now exists. `_iteration` and `_row_number` keep working and are
  documented as the aliases they are, rather than left for a reader to
  encounter in someone else's collection.

- **A YAML body file went out with no `Content-Type` at all.** Detection
  delegates to the host's MIME database, and the host has no entry:
  `application/yaml` was registered in 2024 (RFC 9512), later than the system
  `mime.types` files most machines ship. So `body_file: payload.yaml` sent its
  body unlabelled while the manual promised `application/yaml`, and a server
  requiring the header rejected a request whose error said nothing about
  content types.

  The gap is filled without taking the mapping away from the host — the
  registration runs only where the host is silent — and §5.1.2 now says so
  instead of claiming the mapping is purely the platform's.

- **§11.4's four rejected constructions exited 5, and one did not happen.**
  Every row gives exit code 3, and §17 puts these under "parse or configuration
  error … dependency cycle, variable collision" — the code that tells CI the
  tests never started rather than that a variable is missing. Three of the four
  exited 5.

  The fourth was not rejected at all. A `depends_on:` naming an item in another
  phase parses, and the wave planner dropped the edge silently on the reasoning
  that phases are ordered anyway — but skip propagation is same-phase (§3.3),
  so the line a user wrote as a safety link did nothing: the item ran even when
  the setup item it named had failed. It is now rejected under `--parallel`,
  where the planner is the thing that cannot honour it. A name that resolves
  nowhere is still skipped, because that is `--only` having removed the item.

  The analysis is also hoisted ahead of the setup phase. §3.1 says validation
  errors abort before any HTTP traffic, and a rejected collection was running
  its setup — creating whatever setup creates — before anyone looked at the
  graph.

- **Three cells of §4.1a's content-type matrix did not describe the
  formatter.** It dumps the first 512 bytes of a binary body, not 256, and the
  HEAD and empty-body markers are `_(HEAD — no body)_` and `_(empty body)_`
  rather than the shorter forms the table gave. The markdown report is
  described in the manual as the record you check into git, so the table is a
  promise about a file a reader may never regenerate.

- **The manual's JSONPath tutorial taught two expressions the engine rejects.**
  §2.3 opens "if you don't [know JSONPath], this section is all you need" and
  then teaches `$.items[*].sku` (wildcard across an array) and `$..sku`
  (recursive descent). Both are rejected as invalid paths — and the worked
  example three lines below used the wildcard too, so a reader copying the
  tutorial's own assertion got a failure.

  The section now says plainly that neither is implemented and what to write
  instead. Both remain unimplemented: adding wildcard and recursive descent to
  the JSONPath engine is a feature, not a documentation fix, and changes what an
  assertion receives. A test pins the paragraph in both directions — if either
  form is built later, it fails rather than leaving the manual wrong the other
  way round.

  Found by evaluating §2.3's table against the sample body printed above it.

- **`{{name|default:value}}` now substitutes the default.** The syntax was
  documented in three places — §6.1 as an interpolation form, §9.x and §11.5 as
  what lets a dependent run when its producer succeeded but the JSONPath missed
  — and was half-built. `internal/parallel`'s scanner stripped the pipe to find
  the dependency name, and the runner used it to decide skip-versus-run. Nothing
  ever substituted the value.

  So the dependent *ran*, exactly as §11.5's second row promises, and sent

      http://host/{{user_id|default:FALLBACK}}

  to the server, placeholder and all. That is worse than the exit 5 an
  unresolvable reference produces: a failure shaped like a success, on the wire.

  Found by executing §11.5's failure-and-skip table.

- **§6.1 documented a dotted reference form that does not exist.** A plain
  reference name is `[a-zA-Z_][a-zA-Z0-9_]*`; dots are not part of it, so
  `{{user.id}}` was passed through as literal text with no error. The row is
  gone and the prose beneath now says which dotted forms are real — `$`-prefixed
  functions and `secrets.` aliases, each resolved by its own namespace.

- **The manual's `output:` block table omitted `markdown`**, the same format its
  §4.1 table was missing. Two tables, two documents, one absent feature.

- **The large-dataset guard exits 2, not 5.** All three exit-code tables
  document a tripped safety guard as exit 2, and the specification's CI column
  reads "Fail — fix the invocation". The guard refused correctly and exited 5 —
  the code reserved for variable resolution — because it returned a bare error
  and every bare error ending a run became a 5.

  The distinction is the entire reason the codes are separate: a pipeline
  branching on 5 goes looking for a missing variable, when the fix is to pass
  `--confirm-large-dataset`. `runner.ErrLargeDataset` now identifies the guard,
  carried by an error type that keeps the original advice as its message rather
  than prefixing a sentinel to a complete sentence.

  Found by executing the exit-code tables instead of reading them: exit 2 was
  documented three times and produced by nothing.

- **The manual's `--seed 42` examples were not what that seed produces.** The
  faker table's "Example (seed 42)" column is a reproducibility promise, and all
  ten rows were wrong — `{{$faker.firstName}}` under seed 42 is `Tom`, not
  `Carol`. The column reproduced under no reading: not per-function, not read
  across the table in order.

  Regenerated from the binary, and the section now states what the seed
  guarantees. The rows are independent draws, so `fullName` gives `Tom Edwards`
  while `lastName` alone gives `Bell`; within a run the same function with the
  same arguments returns the same value, so a reused placeholder is stable.

- **The manual's output-format table omitted `markdown`** and announced "Five
  output formats" above a binary that supports six. The specification's table
  had all six, and §4.1a documents markdown reports at length — the primary
  table a reader consults for `--format` was the one place it was missing.

- **`NO_COLOR` now takes effect on a non-empty value only**, per
  [no-color.org](https://no-color.org): the variable disables colour "when
  present and not an empty string (regardless of its value)". curlew disabled on
  presence alone, so `NO_COLOR= curlew run …` — the conventional way to clear an
  inherited preference for one command — turned colour off instead of leaving
  the TTY check to decide. `NO_COLOR=0` and `NO_COLOR=false` still disable, which
  is the convention rather than an oversight: the value is not read.

  Both documents' environment-variable tables had said "any non-empty value"
  since the variable shipped in M1-019. The row was right and the binary was
  wrong for that entire time, and nothing noticed because nothing read the row.
  It is now read and run, so the table and the switch statement cannot disagree
  again.

  Worth recording for what it says about the limits of the prose check added
  below: the false sentence named `NO_COLOR`, a variable that exists and is read,
  so checking names could never have reached the claim it made about it.

- **Both documents' YAML examples are now executed by a test**, not only the
  manual's. The specification is written in fragments — a bare `request:`
  mapping, a bare `assertions:` mapping — so a checker that accepted only whole
  collections found nothing in it; fragments are wrapped into a collection
  before parsing, because a snippet the reader is expected to paste under a
  request must be valid there. 22 examples in the manual and 7 in the
  specification are checked on every run.

  It immediately found one in each: a duplicate `status:` key, used to show two
  alternatives in a single block, which is invalid YAML a reader would copy.

- **All ten Phase 3 dogfooding defects, and an eleventh found while fixing
  them.** Each reproduction moved rather than being deleted — a gap that closes
  leaves behind the test that proves it stayed closed.

  **A non-JSON GraphQL response no longer aborts the run.** A gateway 500 with
  an HTML error page discarded every result in the collection, reported
  `"requests": []` beside a summary counting six passes, and exited 5 — the code
  reserved for variable resolution. It now fails that request and continues.
  `error_handling` also governs the full-failure outcome, as the manual's matrix
  always said; it was handled before the mode was read, so `warn` and `ignore`
  behaved exactly like `fail`.

  **A WebSocket heartbeat now survives an idle step**, which is what a heartbeat
  is for. It failed precisely when idle, reporting a server that answered every
  ping as dead. Reading during the wait was not enough on its own: a gorilla read
  error is permanent, so ending a wait with a read timeout poisons the connection
  and every later step inherits the stale error. Reads therefore moved to a
  single pump that never sets a deadline and stays inside `ReadMessage` for the
  life of the connection, which is where control frames are dispatched. Messages
  arriving during a wait are buffered rather than dropped, and an orderly close
  ends a wait successfully while a broken connection fails it.

  **A refused upgrade reports its status and body.** gorilla hands back the
  response — having already read the body into it — and the dialer was discarding
  it with `conn, _, err :=`, so 426, 401, 403 and 500 were one string.

  **The reported duration covers the body read.** Measured around `Do`, it
  stopped when the headers arrived: a one-second stream reported 0ms and
  `timing.max_duration_ms: 50` passed. curlew measured the right number and
  reported the wrong one.

  **A body that is not JSON is assertable at its root.** Every operator answered
  "not valid JSON" against an event stream, and `cel:` — the documented escape
  hatch — did not help either, because the body was left nil. HTML, CSV, XML,
  plain text, NDJSON and SSE were assertable only by status and headers.
  Deliberately not permissive: deeper paths and structural operators still fail,
  and a gaps entry holds that boundary.

  **OpenAPI 3.1 documents import**, rather than being validated against 3.0 rules
  after declaring 3.1. Translating them into the 3.0 spelling of the same meaning
  was preferred to a 3.1-native dependency, since the import reads only paths,
  parameters, bodies and response codes. **And the import now runs as
  generated**: a path parameter became `{{code}}` with no variable and no
  default, so any document with a path parameter produced a collection that
  exited 5 at run time.

  **Two documentation defects**, each caught by a new structural test rather than
  by reading: the manual's WebSocket examples put `websocket:` one level out and
  could not parse, and a counted `extract:` yields a JSON array that neither
  document mentioned while the manual's own example implied the opposite. A
  third, found while fixing those: the specification gave `wait` a `timeout_ms`
  it ignores, so the documented example paused for no time at all.

### Added
- **Mudflat Phase 3: WebSocket, GraphQL, streaming, TLS — and ten more defects.**
  Twenty-one endpoints across five families, 102 dogfood assertions, and three
  more harnesses in `ci-local.sh`.

  **The WebSocket frame layer is written from RFC 6455**, not taken from
  gorilla — which is what curlew's client uses, so a mudflat built on gorilla
  would agree with it by construction about masking, fragmentation, control
  frames and close codes. The package's own tests read those frames back *with*
  gorilla, so a second implementation checks the bytes, and the handshake accept
  value is pinned against RFC 6455 §1.3's published example. Writing the frames
  is also what buys the endpoints: a library will not send an empty continuation
  frame, decline to answer a ping, or close with a code it dislikes.

  **The TLS matrix is deliberately reduced from nine ports to one.** The
  specification required one thing to be measured before it was built, and it
  was: `SSL_CERT_FILE` does not override the platform verifier on go1.25.5
  darwin/arm64 — not with `SSL_CERT_DIR`, not with
  `GODEBUG=x509usefallbackroots=1` — while the same listener and certificate are
  trusted by a client that sets `RootCAs` explicitly. With curlew exposing no CA
  option either, chain building fails before expiry, hostname or intermediate are
  ever examined, so an expired leaf, a self-signed leaf and a wrong-hostname leaf
  all produce one identical error. Eight endpoints a client cannot tell apart are
  what the anti-bloat rule deletes.

  The measurement sharpened the HTTP/2 gap rather than confirming it: Go's
  default transport offers `h2` in ALPN, so a client that reaches the TLS
  listener negotiates HTTP/2 without asking — verified with `curl --cacert`,
  which reports HTTP/2 over TLS 1.3. What curlew cannot reach is h2c
  specifically.

  Ten defects, none fixed, each an executable reproduction. The ones that change
  how curlew is used today: a non-JSON GraphQL response aborts the whole run and
  discards every result in it; `timing.max_duration_ms` cannot fail on a slow
  body because the reported duration stops at the headers; a response body that
  is not JSON cannot be asserted on by `body:` *or* `cel:`; a WebSocket heartbeat
  reports a healthy peer as dead whenever no step is reading; and the OpenAPI
  importer rejects ordinary 3.1 documents although both documents promise 3.x.

  Five results came out affirmative: close codes are surfaced, curlew answers
  protocol pings, a genuinely dead peer is detected, a TLS failure is legible and
  exits 4, and the OpenAPI round trip passes 8 of 8 against the server that
  served the document.

  One of the harnesses was itself broken. `gaps.sh` read a `passed` field that
  does not exist in the JSON output — the per-request outcome is a status string
  — so every request looked failed and the one thing it exists to catch, an
  unexpected PASS, could never fire. It had been reporting a vacuous pass since
  Phase 2; the fix is verified with a canary request that passes on purpose.

- **Mudflat Phase 2: the adversarial layer, signature verification, and proof of
  parallelism.** Phase 1 gave curlew a server it did not write. Phase 2 gives it
  one that fights back, and answers two questions that had never been answered.

  **`internal/signer` is correct.** It had emitted AWS SigV4 and OAuth 1.0a
  signatures for its entire life without one ever being checked by anything —
  not a suspected defect, an absence of evidence. mudflat now recomputes both.
  The verifier is written from the AWS documentation and RFC 5849 rather than
  from `internal/signer`, because a verifier derived from the implementation it
  verifies agrees with it perfectly and proves nothing; its SigV4 chain is
  pinned against AWS's published `get-vanilla` vector. Both schemes pass,
  including SigV4 over a query string and over a body.

  **`--parallel` is genuinely concurrent.** `/s/{sid}/barrier/{n}` releases only
  when n requests are in flight at once. With the flag: four released, "Waves: 1,
  Max parallelism: 4". Without it: 408, `arrived: 1`. That is a positive proof;
  the wall-clock comparison it replaces is the kind of test this repository
  already had to delete as flaky (M21-003).

  **The raw layer** is a second listener that uses no HTTP library at all — 16
  endpoints writing literal bytes, because `net/http` will not emit a
  `Content-Length` that disagrees with its body, a chunk size that is not hex,
  or a NUL in a header value. 15 golden transcripts, hand-reviewed against
  RFC 9110 and 9112 clause by clause in `testapi/golden/README.md`.
  `testapi/harness/crosscheck.sh` confirms with curl that the malformations are
  real rather than Go being strict: curl independently rejects the duplicate
  `Content-Length`, the non-hex chunk size, the missing status line, the NUL and
  the colon-less header line, and accepts the well-formed control case.

  Three harnesses now run in `ci-local.sh`, each asserting something no
  collection can express: that expected failures still fail (and that an
  unexpected *pass* is itself a failure), that no secret reached an output
  artefact, and that curl reads the raw layer the same way. 74 dogfood
  assertions.

- **Mudflat, a dedicated test API — curlew is now dogfooded (Phase 1).** Every one of
  curlew's 1,963 tests that executes a request used to terminate at a server curlew's own
  suite had written: 23 files construct `httptest.NewServer`, and the smoke suite runs a
  local echo fixture. A Go test server and a Go HTTP client agree by construction about
  header canonicalisation, body framing, chunking and HTTP/2, so every bug living in that
  shared reading was invisible to the entire suite.

  `testapi/` adds a server curlew did not write — 31 endpoints across six families —
  plus the dogfood suite that runs against it. `./scripts/ci-local.sh` now runs
  `curlew run 'testapi/collections/*.yaml'` as a gate step, so dogfooding happens on
  every change rather than when someone remembers. 53 assertions, all passing.

  The design is specified in `docs/TESTAPI_SPECIFICATION.md`. The two pieces worth
  knowing: the echo envelope reports headers as ordered pairs with the client's original
  casing and the body as base64 that is never decoded — each refusing a convenience that
  would hide the bug class the endpoint exists to find — and a parity test derives both
  sides of its comparison from the artefacts themselves, so an endpoint nobody calls or a
  URL that hits nothing fails the build.

  **What the first run found.** Three defects, all now fixed — see the Fixed
  section below. The requests that reproduced them have moved into the passing
  collections, which is where a closed gap belongs.

  **What Phase 2 found.** Two more defects, both the same shape as the first
  three: the specification documents behaviour the binary does not have. Neither
  is fixed; both are held as executable reproductions.

  1. **The object form of `extract:` does not parse.** `CLI_SPECIFICATION` §8
     opens with `api_key: { path: "$.key", sensitive: true }`, and the parser
     rejects it — `cannot unmarshal !!map into string`, because `Extract` is
     `map[string]string`. That object form is the only way to declare
     sensitivity explicitly; without it a value is sensitive only if its name
     happens to match the §6.5 heuristic, and the name comes from the API under
     test, not from the collection author.
  2. **Redaction covers the request but not the response.** The same sensitive
     value is replaced where curlew sent it and printed verbatim where the
     server returned it:

     ```
     > Authorization: [REDACTED]
     ✗ body $.authorization equals: expected …, got Bearer SENTINELVALUE123
     ```

     §6.5 lists eight output surfaces and does not restrict redaction to
     request-side occurrences. An API that echoes a token, a `Set-Cookie`
     carrying a session, or a redirect with a token in its query puts the secret
     straight into a CI log. 13 concrete leaks across the JSON, Markdown and
     event-stream outputs are recorded in
     `testapi/harness/redaction-known-leaks.txt`; the harness fails on any leak
     outside that baseline, and *also* fails when a baseline entry stops
     leaking, so a fix forces the line out instead of leaving a standing excuse.

- **A CI workflow for the Go CLI** (`.github/workflows/go.yml`). There has never been one:
  the seven existing workflows cover the .NET backend, the web dashboard, email templates
  and releases, and none of them builds or tests the CLI. The new job runs
  `./scripts/ci-local.sh --go` rather than restating its steps, so the workflow and the
  local gate cannot drift into disagreeing about what "green" means.

  It follows the repository convention of `workflow_dispatch:` only, with the
  `push`/`pull_request` block present but commented out — enabling it is a cost decision
  about GitHub Actions billing, not a code change.

- **`assertion.result` now says where the assertion is written.** v1.5 let a failing
  assertion name its request; it still could not say which line defines it. On a request
  carrying a dozen assertions, an agent wanting to open the YAML at the failure had to
  search the file for a JSONPath or header name and guess which occurrence was the right
  one.

  The event now carries `source_file` and `source_line`, and the line is the one a
  developer would edit — the operator key (`equals:`, `exists:`, …) for body and header
  assertions, the `status:` value, the `max_duration_ms:` key, the `schema:` key, and the
  list entry for a `cel:` expression. Pointing at the enclosing request would have been
  cheap and useless.

  This needed positions the parser was discarding. `BodyAssertion`, `HeaderAssertion`,
  `CELAssertion` and `StatusCodes` now record their YAML node's line; `TimingAssertion` and
  the `assertions:` block gained unmarshalers to reach `max_duration_ms:` and `schema:`.
  A binary-level test pins all six kinds to an anchor string in the collection, so a
  regression names the wrong line rather than merely omitting one.

### Fixed
- **Secrets returned by the server are now redacted.** A sensitive value was
  replaced where curlew *sent* it and printed verbatim where the server *sent it
  back*. Both lines below came from one run:

  ```
  > Authorization: [REDACTED]
  ✗ body $.authorization equals: expected …, got Bearer SENTINELVALUE123
  ```

  `docs/CLI_SPECIFICATION.md` §6.5 lists eight output surfaces and never
  restricted redaction to the request side. An API that echoes a token, a
  `Set-Cookie` carrying a session, or a redirect with a token in its query put
  the secret straight into a CI log. Thirteen concrete leaks were measured across
  JSON, Markdown and the event stream.

  It was four defects wearing one coat, and all four are fixed:

  - An extracted value was never registered as a sensitive *value*. A token
    pulled out of a response was therefore redacted nowhere, by either route to
    sensitivity. `variable.MarkExtractedSensitive` now runs at every extraction
    site in `internal/runner` and `internal/parallel`, before any event carrying
    the value is emitted — so the response body that produced the token is
    redacted too, not just later requests that use it.
  - An assertion's `expected` and `actual` strings were never redacted at all.
    That is the one surface whose entire job is to print the value that did not
    match, which makes it the likeliest place for a secret to appear.
  - Response headers were never redacted, and `Set-Cookie` was not treated as
    inherently sensitive although `Cookie` was — the same credential, travelling
    the other way.
  - A secret that lived only in a URL query string survived every pass, because
    no body ever carried it.

  `internal/runservice/redact.go` now owns the whole scrub and runs once before
  formatting, so every format is covered by one code path rather than eight. The
  runner accepts its runtime sensitive set from the caller when something is
  watching the run: the `--events` sink redacts each event as it is emitted, so a
  value discovered mid-run has to be known before the event carrying it is
  written. `--allow-sensitive` is unaffected.

  Found by dogfooding against mudflat's `/leak/*` family
  (`docs/TESTAPI_SPECIFICATION.md` §11B.2). `testapi/harness/redaction.sh` now
  sweeps nine artefact directories with an empty baseline, and
  `testapi/harness/redaction-actual.yaml` — every assertion wrong on purpose —
  closes the hole where terminal, TAP, JUnit and JSONL looked clean only because
  nothing had ever been printed to them.

- **The object form of `extract:` now parses.** `docs/CLI_SPECIFICATION.md` §8
  opens with it:

  ```yaml
  extract:
    user_id: "$.id"
    api_key:
      path: "$.key"
      sensitive: true
  ```

  `parser.RequestItem` declared `Extract` as `map[string]string`, so only the
  string form existed and the object form failed with `cannot unmarshal !!map
  into string`. It is the only way to declare an extracted value sensitive on
  purpose; without it, sensitivity rested entirely on whether the name happened
  to match the §6.5 heuristic — and the name is whatever the API under test calls
  the field.

  Both forms decode into the same map, so nothing downstream changed. An object
  with no `path`, or with an unknown key, is now a parse error: a misspelled
  `sensitiv: true` that parsed silently would leave a value unredacted while its
  author believed the opposite. The published JSON Schema accepts both forms, so
  editors no longer flag a valid collection.

- **Assertion expected values now interpolate.** `equals: "{{var}}"` compared the
  response against the literal template text — for collection variables and for
  values extracted earlier in the run, on both the header and body paths. The same
  variable interpolated correctly everywhere else, so a request could resolve
  `{{first_id}}` into its URL, fetch exactly the right resource, and then compare
  `$.id` against the twelve characters `{{first_id}}`.

  `requtil.ToHeaderInputs` and `ToBodyInputs` take the scope and interpolate.
  Body values go through `InterpolateBody`, which walks strings wherever they
  appear — including inside the map an operator like `in_range` takes — and
  leaves every other type alone, so an expected integer is never routed through a
  string round trip. An unresolvable reference is now an error, the same as at
  every other interpolation site.

- **Header `exists: false` asserts absence.** It behaved identically to
  `exists: true`, so an absent header reported "expected exists, got header not
  present" — the very condition the assertion had asked for — and
  "this response must not carry `Set-Cookie`" was unexpressible. `docs/CLI_SPECIFICATION.md`
  §7.2 has always documented the operator as "Presence (`true`) or absence
  (`false`)".

  The body path had the same hole and is fixed with it: `exists: false` and
  `not_exists: true` are now two spellings of one intent instead of disagreeing.
  A value that is not a recognisable boolean keeps the historical meaning —
  assert presence — so no collection that was passing can start failing.

- **Response-body failures are classified correctly.** Everything that went wrong
  while reading a body was reported as `network error: reading response body`,
  and none of it was classified as a `*errors.NetworkError`. The label was wrong
  in one direction and the retry classification in the other:

  - a lying `Content-Encoding` was *called* a network failure, though no retry
    can fix it;
  - and a genuine transport failure mid-body — a connection dying partway
    through a response — was never classified as one, so
    `retry_on.network_errors` silently covered only the failures that happened
    *before* the body started.

  `httpexec.classifyBodyError` now splits the two. A decode failure returns the
  new `ErrDecode` sentinel with a message naming the header responsible, and no
  retry rule matches it. Everything else becomes a `*errors.NetworkError`, so
  `retry_on.network_errors` fires for a truncated body, which it never did.

- **CLAUDE.md claimed `ci-local.sh` "mirrors the GitHub workflows".** It did not: no
  workflow built or tested the Go CLI, and every workflow has auto-triggers disabled
  pending Actions billing, so pushing a branch verified nothing. The quality-gate section
  now says plainly that `ci-local.sh` is currently the only gate, and `ci-local.sh`'s own
  header — which claimed to mirror the two `e2e-*` workflows — says the same.

- **`ci-local.sh --help` silently truncated its own output.** It printed a hardcoded line
  range (`sed -n '2,22p'`) of the header comment, so growing the header past line 22 cut
  off the exit-code table without any error. It now prints the leading comment block by
  tracking where the block ends.

- **A failing assertion could not say which request it belonged to.** `assertion.result`
  carried only `request_id` — which the schema defines as an opaque pairing key, and which
  is minted positionally (`req-1`, `req-2`, …) in execution order. `request.start` and
  `request.end` have carried the stable, name-derived `request_slug` since v1.2;
  `assertion.result` did not.

  So the one event an agent reads first could not be read alone. A consumer had to buffer
  the whole stream, collect every `request.start`, and join on `request_id` before it could
  name the failure — and the resulting table was run-local. Inserting a request at the top
  of a collection shifts every id after it, so a stored "failure at `req-3`" points at a
  different request on the next run. A consumer tailing a live stream (`curlew watch`)
  maintained that join by hand.

  `assertion.result` now carries `request_slug`. One line names the request, the assertion,
  and expected vs actual, and the slug is also the `responses/<slug>.md` filename — so the
  agent knows which file to open without reading anything else. `request_id` is unchanged
  and remains the pairing key.

  A stream-level test asserts the invariant across all three execution paths (serial,
  parallel waves, data-driven iterations), each of which builds its own assertion events.
- **Assertion results claimed a structure they did not have** (M24-001). `assertion.Result`
  carried a single `Type` field doing two incompatible jobs — a machine-readable
  discriminator and a human-readable label — and the label won. For every kind except
  `status` the field was built as a composite, `fmt.Sprintf("body %s %s", path, operator)`,
  producing values like `body $.user.name equals`.

  Every published events JSON Schema since v1.0 has declared that field as
  `enum: ["status","body","header","schema"]`, so every body, header, schema and CEL
  assertion violated the schema, and `timing` and `graphql_error` were never in the enum at
  all. Of the four declared members exactly one (`status`) was ever emitted verbatim — an
  agent following the documented contract and switching on `type == "body"` matched nothing.

  The existing schema tests did not catch it because every golden fixture and every direct
  `EmitAssertionResult` call used `"status"`: the single conforming value. A corpus that
  exercises only the passing case is not a guard.

  `Type` is now the discriminator alone, with `Target` (JSONPath, header name, schema path,
  or `assertions[N]`) and `Operator` carrying the parts that vary. `Label()` reassembles the
  historical phrase, so terminal, HTML, TAP, markdown, pr-check and UI output are unchanged —
  pinned by a test asserting the exact pre-change string for all seven kinds.
  `extractJSONOperator`, which recovered the operator by taking the last whitespace-delimited
  word and returned `"status"` as the operator of a status assertion, is deleted.

  Two guards were added and mutation-verified: assertion types must be compile-time constants
  (a computed discriminator now fails the build), and the emitted vocabulary is held to the
  published enum in both directions, so neither an undeclared type nor a declared-but-unemitted
  one survives. The walk covers the whole tree rather than `internal/assertion` alone — the
  runner builds an `assertion.Result` of its own, and a package-scoped walk had already
  declared the vocabulary complete without it.

- **Three flags reached users that `curlew --help` never mentioned** (M23-001). `--events`
  was implemented, working, and advertised in the README, but absent from help — a user
  reading the README got no confirmation from the tool that the flag existed.
  `--format markdown` was accepted, and the CLI's own "unknown output format" error message
  listed it as supported, but the Run Options `--format` line did not. And
  `curlew schema --project`, which emits the `curlew.yaml` project schema instead of the
  collection schema, was documented nowhere at all.

  All three arrived the same way: a `case "--flag":` added to a parser with no matching
  help line. Nothing failed, because nothing was checking. Three parity tests now derive
  the truth from the code rather than a hand-maintained list — accepted flags from an AST
  walk of the package's case clauses, accepted formats from the CLI's own error message,
  and advertised commands from the help output itself. Each was mutation-verified to fail
  when its guard is removed.

- **`init --skill agent` scaffolded a lesser project than `--skill claude`.** The scaffolder
  keyed the `output:` extensions on `opts.SkillName == "claude"` while keying the `.gitignore`
  extension on `opts.SkillName != ""`, contradicting its own documented contract ("when
  non-empty"). Any name other than `claude` got the skill files but not `format: markdown`,
  not `events: .curlew/run.ndjson`. Found by the new test asserting every accepted `--skill`
  value scaffolds a byte-identical project.

### Changed
- **Events schema v1.6.** `assertion.result` gains the optional `source_file` and
  `source_line`. Additive over v1.5. Published at `docs/events-schema/v1.6.json` with the
  per-kind line table in `docs/EVENTS_SCHEMA_v1.6.md`.

- **Events schema v1.5.** `assertion.result` gains the optional `request_slug`. Additive
  over v1.4; a v1.4 consumer is unaffected. Published at `docs/events-schema/v1.5.json`
  with the field reference and rationale in `docs/EVENTS_SCHEMA_v1.5.md`.

- **CEL assertion results share one identity.** `evalCELAssertion` built five separate
  `Result` literals, one per return path. They are now one `id` value reused via `with(…)`,
  the same shape body assertions took in the v1.4 fix — five places to keep in step was how
  the source pointer would have gone missing on the compile-error branch.

- **The agent skill is no longer presented as Claude-only.** `curlew init --skill claude`
  scaffolds a standard Agent Skill — `SKILL.md` with `name`/`description` frontmatter plus
  per-topic reference files — into `.claude/skills/curlew/`. That directory is a project
  skill directory for GitHub Copilot as well as Claude Code, so the artifact already worked
  with both; only curlew's naming and documentation said otherwise. The flag value `claude`,
  the help text, the README, `docs/MANUAL.md` §4.9 and `docs/CLI_SPECIFICATION.md` §18.4 all
  implied a vendor lock that does not exist, and a code comment described the enum as "the
  extension point for copilot/cursor in later slices" — an extension that was never needed.

  `agent` is now the canonical `--skill` value and leads the enum, since that order is what
  `--help` and the rejected-value error print. `claude` remains accepted as a compatibility
  alias; a test holds every accepted name to the same payload and the same destination, so
  the two cannot drift apart. Docs now name both agents. No collection, project file, or
  scaffolded skill needs to change.

- **Events schema v1.4** (M24-001). `assertion.result` gains `target` and `operator`
  (optional) and `label` (required), and `type` now emits the discriminator the schema has
  declared since v1.0. This is a conformance fix rather than a v2.0 semantic change: no
  published schema version ever permitted the composite, so the emission was non-conformant
  rather than contractual — the same reasoning v1.3 applied to the `wave_index` `minimum`
  correction. Migration is mechanical: read `label` wherever you read `type`. The enum is
  widened to the seven kinds that can actually be produced. `--format json` gains the same
  `target`/`operator`/`label` fields, and the UI renders `type` as its chip and `label` as
  the description.

- **The README now describes the whole tool** (M23-001). It had covered roughly half of it:
  retry with backoff, rate limiting, CEL `if:` conditionals, redaction-on-by-default,
  `--only`, `--events`, agent-skill scaffolding, binary request bodies, and four of the
  fourteen commands were all missing. Features are now grouped, and a command table lists
  every command. `TestReadme_lists_every_command_the_CLI_advertises` keeps it honest.

  Two claims drafted for the new section were corrected against the runner before landing:
  `required:` is honoured only in the `setup:` phase (`executePhase` receives
  `checkRequired=false` for main and teardown), and `options.stop_on_failure` halts only
  the main phase — `teardown:` still runs.

### Added
- **The copyright holder is named: Peter Lindqvist.** The notices previously read
  `weiqigod`, taken from the git identity on every commit here. A copyright notice naming a
  pseudonymous handle is weak — it does not identify a legal person who can assert or
  transfer the right. Four lines change: `NOTICE` and the three proprietary LICENSE files.

  The Go module path `github.com/weiqigod/curlew` and the schema `$id` URLs are
  **deliberately unchanged** and verified so: those encode the GitHub organisation, which is
  a different thing from the copyright holder. Renaming them would break every import path
  and re-break the `$id` URLs corrected in M21-001. The root LICENSE is likewise untouched
  (sha256 unchanged) — the Apache appendix placeholders stay as-is by convention, with the
  actual holder named in `NOTICE`.

- **The Apache-2.0 license now states its scope, carving out the commercial components.**
  The repository has carried a root Apache-2.0 `LICENSE` since commit `3d3d956`, but with
  **no statement of what it covers** — so by default it applied to the whole tree,
  including `src/`, the .NET backend holding Stripe subscriptions, a billing portal and
  proration logic. Anyone with repository access could lawfully take that code and operate
  a competing service. Permissively licensing the monetizable asset was almost certainly
  not the intent of a commit described as adding a "public README".

  The CLI stays Apache-2.0: it is the product distributed as a binary and it wants
  adoption. `src/`, `web/` and `deploy/` each gain a LICENSE reserving all rights —
  `deploy/` because it holds only self-hosting configuration for the backend and is
  useless without a component nobody is licensed to run. `NOTICE` states the split, and
  the root LICENSE text itself is unchanged.

  The `NOTICE` scope section is written as an **exclusion** list, not an inclusion list —
  enumerating covered directories invites omission, and the first draft did exactly that,
  silently failing to mention `deploy/`, `site/`, `examples/`, `sample/`, `scripts/` and
  `testdata/`.

  The existing LICENSE was verified byte-identical to the canonical Apache-2.0 text
  (sha256 `cfc7749b…23d30`, cross-checked against two independent vendored copies) and left
  untouched. No dependency constrains the choice: every direct dependency is MIT,
  Apache-2.0 or BSD, and nothing in the tree is copyleft.

- **Release archives now ship `LICENSE` and `NOTICE`.** Apache-2.0 §4(a) requires the
  License to accompany any distribution and §4(d) requires the NOTICE to travel with it.
  The archives added in M22-006 carried README, CHANGELOG and docs but neither, so every
  artifact produced before this change would have been non-compliant. Verified by
  extracting a built archive rather than by reading the config.

- **Release pipeline: link-time version injection and cross-platform artifacts.**
  `cmd/curlew/main.go` declared `const version = "0.1.0-dev"`. The Go linker **silently
  ignores `-X` for constants** — `go build -ldflags "-X main.version=1.2.3"` exits 0 and
  produces a binary still claiming `0.1.0-dev`. Any release pipeline built on that would
  have shipped every binary mislabelled, with no step failing to warn. `version` is now a
  `var`, pinned by a test that builds the real binary with the flag and asserts what it
  reports, so a revert to `const` fails the suite rather than the release.

  The injected value was verified to reach `--version`, `--help` and `info --format json`
  (machine-readable provenance for a results consumer). It does not reach plain-text
  `info`, which reports no version at all — an initial test asserted otherwise on an
  assumption; the test was corrected rather than the CLI changed to satisfy it.

  Adds `.goreleaser.yaml` building six targets (linux/darwin/windows × amd64/arm64) with
  `CGO_ENABLED=0`, archives carrying README, CHANGELOG, MANUAL and CLI_SPECIFICATION, and a
  checksums file. Validated with `goreleaser check` and proven by a full local snapshot
  build: the linux artifact was confirmed **statically linked** with `file(1)`, and the
  extracted darwin binary confirmed to report the injected version. Releases are drafts so
  a human reviews notes and artifacts before anything is public.

  Adds a `Release` workflow, `workflow_dispatch` only to match every other workflow here
  while Actions billing is paused (tag trigger documented in a comment). It runs the Go
  gate before publishing anything.

### Removed
- **All backend and login functionality removed from the CLI.** `curlew` is now entirely
  local: no account, no authentication, and no network calls beyond the HTTP requests a
  collection defines.
  - Commands gone: `curlew login` (device-code flow), `curlew worker` (distributed
    coordinator + schedule pull), and the hidden `CURLEW_INTERNAL` probe command.
  - `run` flags gone: `--report-upload`, `--org`, `--pr`, `--repo`, `--triggered-by`,
    `--git-sha`, `--workers`, `--coordinator-url`, `--refresh-vault`.
  - Environment variables gone: `CURLEW_BACKEND_URL`, `CURLEW_BACKEND_TOKEN`,
    `CURLEW_COORDINATOR_URL`, `CURLEW_TELEMETRY_ENDPOINT`. This also retires the dead
    `https://api.apitool.dev` default that survived the rebrand — `curlew login` had been
    making a live DNS lookup against a host that serves nothing.
  - Packages deleted: `internal/backend` (device code, refresh-token rotation, keychain
    and encrypted-file storage, team-vault fetch), `internal/worker`,
    `internal/runner/distributed`, `internal/prcheck/client.go`,
    `internal/vault/teamtemplate/cache.go`, `internal/telemetry/client.go`.
  - The `src/` .NET backend and `web/` dashboard remain in the repository, untouched. The
    CLI no longer talks to them.
- **The last reference to the deleted team-vault cache.** `curlew run` still stat'd
  `~/.config/curlew/team_vault.json` to decide whether a missing env file was tolerable —
  a file nothing has written since the cache was deleted. On a machine upgrading from an
  older build, that leftover file silently suppressed `environment not found` for
  `--env`. Only `CURLEW_TEAM_CONFIG` grants that tolerance now.

### Changed
- **`curlew pr-check` is a local CI gate.** It reads a results file, reports the verdict,
  and exits 1 when the run contained failures. `--summary <file>` writes the verdict as
  JSON for a CI step to consume; `--dry-run` prints it instead. The `--org`, `--pr`, and
  `--repo` flags are gone along with the upload. `--results` now accepts the output of
  `curlew run --format json` in addition to the older flat payload shape — previously
  nothing in the CLI produced a file `pr-check` could read, because `--report-upload`
  built its payload in memory and never round-tripped through disk.
- **`curlew telemetry` records to a local file.** Events are appended as NDJSON to
  `~/.config/curlew/telemetry.ndjson` (override with `CURLEW_TELEMETRY_FILE`) and are
  never transmitted. `delete-request` is renamed `delete` — with no backend to request
  anything from, it simply removes the install id, state, and collected events.
- **Shared vault templates load from a local file only.** `CURLEW_TEAM_CONFIG` still
  works; the backend cache, its TTL/stale-revalidate logic, and `--refresh-vault` are gone.
- **`curlew run` rejects unknown flags.** A dash-prefixed argument was previously
  swallowed as an extra positional and silently ignored, so a removed flag such as
  `--report-upload` would have become a no-op. It now fails with `unknown flag: …`.
- The CLI-driven Playwright E2E specs (`full-pipeline`, `enterprise-full`,
  `m14-revenue-loop`, `m16-happy-path`, `m18-compliance`) seeded the backend by shelling
  out to removed CLI commands. They now seed over HTTP and run again in
  `scripts/ci-local.sh` (see "E2E convergence specs seed over HTTP" below).
  `scripts/m16-e2e.sh` and `scripts/m18-e2e.sh` are deleted outright.

- **Licensing and tier gating removed from the CLI.** The five-tier model (Free/Solo/Professional/Team/Enterprise), license JWTs, feature gates, trials, upgrade URLs, the `curlew license` command, exit codes 6 (`feature_gated`) and 9 (grace expired), and the `CURLEW_TIER` / `CURLEW_LICENSE_BUNDLE` / `CURLEW_LAST_VALIDATION_OVERRIDE` environment variables are gone. Every CLI feature is now unconditionally available. `curlew login` remains for backend-connected features (team-vault fetch, scheduled runs, `pr-check`). Historical entries below describe the gating as it existed at the time.

### Fixed
- **`curlew telemetry --help` prints the config directory it actually uses.** Four lines
  hardcoded `~/.config/curlew/...`, which is the Linux answer — `os.UserConfigDir` resolves
  to `~/Library/Application Support` on macOS and `%AppData%` on Windows. A macOS user
  following the help to find or delete their `install_id` would look in a directory curlew
  never writes to.

  The help now resolves the path through `appdir.ResolveConfigDir` rather than restating a
  literal, which is correct on every platform without enumerating them and additionally
  shows an active `CURLEW_CONFIG_DIR` override — something static text could never do.
  Help must never fail, so an unresolvable directory degrades to a placeholder.

  The same claim appeared in `docs/MANUAL.md` (2 places) and `docs/CLI_SPECIFICATION.md`
  (3 places); those cannot print a runtime value, so they now state the resolution
  explicitly. Fixing the help alone would have left the CLI's own reference contradicting
  its own binary. `management/plans/`, the investigations, `docs/security/` and
  `docs/SPECIFICATION.md` are dated records and were left alone, the same boundary M21-004
  drew.

- **`internal/appdir` no longer documents removed backend functionality; 0% → 83.3%.** The
  package comment said the config directory was "used for device registration, backend
  session tokens, and telemetry state". The first two went in the 2026-08-03 backend strip.
  Checked against the callers rather than assumed: the only consumers are two call sites in
  `cmd/curlew/telemetry.go`, and the only files written are `install_id`, `telemetry.json`
  and `telemetry.ndjson`. Same drift class as M21-002.

  Two further comment inaccuracies were corrected in the same file: `ConfigEnv` was
  documented as overriding `~/.config/curlew`, which is Linux-specific (`os.UserConfigDir`
  is `~/Library/Application Support` on macOS), and the precedence list omitted that the
  override must be **non-empty** and is used **verbatim** — both now the tested contract.

  The package had no tests despite a precedence rule the whole `cmd/curlew` suite depends
  on: `CURLEW_CONFIG_DIR` is how those tests keep state out of the developer's real config
  directory. The uncovered remainder is the `os.UserConfigDir()` failure path. Since the
  behaviour was already correct these are characterisation tests, so mutation stands in for
  RED — each of the four guards was verified by breaking the implementation.

- **`internal/requtil` covered directly: 76.8% → 96.8%.** Three exported converters sat at
  0% — `ToHeaderInputs`, `ToBodyInputs` and `ToHTTPRequest`. None was dead code; all three
  are called from the sequential runner, the parallel executor and the WebSocket executor,
  so they were exercised indirectly and worked, but nothing pinned their behaviour.

  Two behaviours are now pinned rather than merely executed. The assertion converters are
  deliberately **asymmetric**: `ToHeaderInputs` stringifies via `fmt.Sprint` because
  `assertion.HeaderInput.Value` is a `string`, while `ToBodyInputs` must not, because
  `assertion.BodyInput.Value` is `any` and operators like `greater_than` compare on the
  typed value. Making the two "consistent" is the obvious wrong edit and now fails a test.

  `ToHTTPRequest` is a hand-maintained field copy that currently sets all five fields of
  `httpexec.Request`; a sixth added later would reach the executor silently zero — the same
  divergence class as the TAP/JSON speedup metadata in M21-003. The new guard walks the
  destination struct by reflection, so a field added there fails on the day it is added
  rather than the day someone notices requests losing it.

  Also covers the previously unreached interpolation error branches (each failure must name
  its own field: `URL:`, `headers:`, `query params:`, `body_file:`, `body:`) and the
  `injectContentType` no-detected-type branch. All six new guards verified by mutation.

- **`ci-local.sh` gates the backlog as a named, early step.** The consistency test already
  ran inside `go test $(go_pkgs)`, so this adds visibility and failure attribution rather
  than new enforcement — stated plainly so nobody later concludes the backlog was
  previously ungated. The step sits immediately after `go build`, prints the task and
  capability counts it actually read (a gate that reports what it read is harder to
  misread than one that prints only `PASS`), and fails before `go test -race`, `go
  coverage` and `golangci-lint` are paid for. Measured: an unlisted task file exits 1
  naming the file, with none of the later steps run.

- **Three task files were not parseable YAML, and nothing had ever noticed.**
  `management/tasks/M21-001.yaml`, `M21-003.yaml` and `M21-004.yaml` each carried a
  definition-of-done line beginning with a backtick, which cannot open a plain YAML
  scalar. Every check that had ever "verified" the backlog was line-oriented
  (`grep -h '^status:' management/tasks/*.yaml`), so it counted matching text in files no
  parser could read and reported them as done. The lines are now quoted, and
  `internal/backlog` parses every task file so this fails the build instead.

- **`internal/backlog`: a traversal of `management/` that cannot report a false clear.**
  Ad-hoc traversals of `backlog.yaml` had failed three times in one session, always the
  same way: a wrong key path yields an empty result, and an empty result reads as good
  news ("backlog exhausted, none open"). The index is easy to get wrong because a task
  entry is *either* a bare id string *or* a mapping carrying its own status — 218 of 221
  entries are mappings, 3 are bare strings.

  `backlog.Load` reconciles `backlog.yaml` against `management/tasks/*.yaml` and returns an
  error, never an empty success, on any structural surprise: an unknown or misspelled key
  (the index is decoded with `KnownFields(true)`), a capability with no tasks or no
  milestone, a task entry of an unexpected shape, a duplicate id, a task file that does not
  parse, an id present in one source but not the other, a status outside the documented
  lifecycle, and a missing status. Where both sources state a status they must agree, so
  editing one alone cannot produce a clean result.

  Each of the eleven guards was verified by mutation rather than by passing: neutering any
  one of them fails a named test. Five of those tests initially passed against a neutered
  guard for the wrong reason — a different check fired first and its message happened to
  contain the asserted substring — and were retargeted at fixtures that isolate the guard
  under test.

- **The published schemas no longer advertise licensing tiers.** Four `description` strings
  still named the tier that used to gate a feature: `rate_limit_rps`, `include` and
  `assertions.schema` each said "(Professional tier)", and `ui.history.enabled` said "Solo
  tier and above". MANUAL §1.5 wires both schemas into the user's editor, so hovering
  `rate_limit_rps` in VS Code advertised a paid tier of a product that has none and no way
  to buy one. Same defect class as M21-001 — a schema description that does not match the
  binary — and closed the same way: `TestSchema_descriptions_are_tier_free` walks every
  description in both files and fails on any that names a tier, naming the offending
  property by path.
- **`docs/DEVELOPMENT_PHILOSOPHY.md` no longer prescribes deleted mechanisms.** CLAUDE.md
  lists it under Key Documentation as active guidance, and two of its sections described
  removed systems as current practice. "The feature-gate seam" told developers to park a
  specified-but-unbuilt feature behind a gate reporting that it requires a higher tier;
  there is no gate, no entitlement check and no exit code for one. It is now "No middle
  state": a feature is either invisible or it works, and wanting to ship a deferred
  interface is the signal to cut the slice smaller. "The Backend Boundary", which called
  the CLI/backend network boundary the most significant in the process and required every
  slice to span it, is now "No Backend Boundary" — the CLI makes no backend calls, so
  "runnable" permanently means build the binary and run it, and the file-based equivalents
  (`pr-check`, `telemetry`, `CURLEW_TEAM_CONFIG`) are the pattern to follow.
- **CLAUDE.md's Current Status matched neither the backlog nor the tree.** It described M21
  as open with two tasks. All four are done and the backlog is exhausted again. (M21-004)
- **`docs/MANUAL.md` no longer describes the removed backend.** Four surfaces survived the
  backend strip because that pass targeted code, and survived the CLI_SPECIFICATION
  extraction because that produced a new document rather than editing this one:
  - §1.1's installation check told a new user to expect `worker` in `curlew --help`. The
    listed commands now match the binary exactly — `worker` gone, `ui` and `telemetry`
    added — so the manual's own first-run verification step passes.
  - §4.3, the exit-code master table the manual calls "the single source", and its §11D
    duplicate both carried `10 | Worker unauthorized`. Nothing returns 10. Both now list
    0–5 and 130 only, and both document the 1-versus-2 usage-error split verified in
    `docs/CLI_SPECIFICATION.md` §17 rather than the flat "usage error" claim they carried.
  - §11A still described `curlew pr-check` as "upload results and post a PR status check",
    contradicting §8.1 in the same document. It now describes the local results-file gate,
    with its actual exit codes and flags.
  - The §7 migration table, which correctly documents `curlew worker` and `--report-upload`
    as removed, is untouched.
- **`docs/MANUAL.md`'s table of contents was substantially wrong.** Four entries pointed at
  sections deleted with the backend — "8.1 Report upload", "8.2 PR checks", "8.3 Web
  dashboard", "9.1 Distributed execution" — and the Part 8 header still read "Team
  Features". Five sections added since the TOC was last touched (1.5, 3.6.1, 4.2b, 6.8,
  6.9) were missing from it entirely. The TOC now matches the document exactly, 63 entries
  to 63 headings, verified in both directions.
  - Internal anchor links went from eight broken to zero. Four were the stale TOC entries
    above; the other four were `#faker-locales-m20`, which pointed at a bold paragraph
    carrying a `{#id}` attribute — kramdown syntax that GitHub-flavored Markdown does not
    support, so the anchor never existed. Promoted to a real `####` heading, whose natural
    slug is the one the links already used.
  - Three cross-references cited `SPECIFICATION.md` by line number for CLI behaviour. That
    document is now scoped to the backend and dashboard, and the line numbers had rotted
    (`:812` is encoding and hashing, not `$faker.color`). The Conclusion also pointed
    readers there as "the source of truth for subtler behavior edge cases" and recommended
    scaling out to distributed execution. Both now point at `docs/CLI_SPECIFICATION.md`.
    (M21-002)
- **`TestTAPOutput_ParallelSpeedup` was flaky and is now deterministic.** It ran the same
  collection twice — once with `--format tap --parallel`, once with `--format json
  --parallel` — and asserted the two `speedup_factor` values agreed within 0.15.
  `speedup_factor` is `sum(wave durations) / total duration`, both wall-clock measured, so
  two executions legitimately disagree: over 25 identical runs that collection produced 0.7
  once, 0.9 once and 1.0 twenty-three times. A 0.30 spread against a 0.15 tolerance fails
  roughly one run in twelve, matching the 2-in-8 measured on `main`. The test's own doc
  comment stated the intent as "byte-equal to the JSON formatter's value **for the same
  run**", but it was never the same run — one `curlew run` emits one format, so same-run
  parity is not observable through the CLI.
  - Replaced by three deterministic tests: `TestBuildParallelMetadata` pins the formula
    against fixed durations including both rounding boundaries and every zero case;
    `TestParallelMetadata_formatters_agree` feeds one summary through both real
    constructors and both formatters and compares what comes back;
    `TestParallelMetadata_call_sites_agree` keeps an end-to-end check on `wave_count` and
    `max_parallelism`, which are properties of the dependency graph rather than the clock.
    `TestTAPOutput_ParallelSpeedup` keeps every assertion that was already deterministic —
    block presence, keys, ordering, and the one-decimal form.
  - `newParallelTAP` and `newParallelExecutionJSON` replace the three-field copy that was
    duplicated at both output call sites. Without them a divergence at one call site was
    only detectable by running the binary twice, which is precisely the flaky comparison
    being removed. Verified by mutation: dropping the rounding from
    `buildParallelMetadata`, altering either constructor, and inverting the formula each
    fail a test. (M21-003)
- **The collection and project JSON Schemas now describe every field the parser accepts.**
  `schemas/collection-v1.json` declared `additionalProperties: false` on `requestItem` and
  `request` while omitting nine fields the parser binds — `if`, `depends_on` and `signing`
  on a request item; `protocol`, `graphql` and `websocket` on a request; `cel` on
  assertions; and top-level `config` and `signing`. Because MANUAL §1.5 tells users to wire
  the schema into VS Code, every collection using conditional execution, `depends_on`,
  GraphQL, WebSocket, CEL assertions or request signing got error squiggles on valid YAML —
  the exact failure the schema exists to prevent. New `$defs`: `config`, `signing`,
  `celAssertions`, `graphql`, `websocket`, `websocketStep`, `websocketReconnect`,
  `websocketHeartbeat`.
  - Two adjacent defects of the same class are fixed alongside. `$defs.request` required
    `method`, which the parser defaults three ways (`GET`, `POST` for graphql, `WS` for
    websocket) — the WebSocket example in CLI_SPECIFICATION §12.3 was itself flagged. And
    `schemas/project-v1.json` omitted the `config:` block that `internal/config` binds, so
    a project setting a locale was flagged too.
  - Both files' `$id` moved from `raw.githubusercontent.com/peterlindqvist/curlew`, which
    404s, to `weiqigod/curlew`. The collection schema's `$id` had no test, which is how it
    drifted silently; both are now pinned.
  - New `internal/schema/parity_test.go` walks the parser structs by reflection and fails
    in both directions — a parser field with no schema entry, and a schema property the
    parser would ignore — with a table-completeness test so a newly added nested struct
    cannot slip past, and `go/ast` guards pinning the two hand-rolled key switches
    (`WebSocketStep.UnmarshalYAML`, `SensitiveVars.parseObjectVar`) to their source.
    `TestSchema_dod_fixture_parses` runs the all-nine-fields fixture through the real
    parser, so the two agree on a document and not merely on a list of names.
  - The four closed value sets the parser validates against (`protocol`, WebSocket
    `action`, `reconnect.backoff`, `graphql.error_handling`) were inline string
    comparisons duplicated as schema enums and quoted again in error hints, with nothing
    holding the copies together. They are now exported lists in `internal/parser`, with
    the hints built from them and the schema enums pinned to them. That surfaced a
    user-facing defect: the hint for `ErrUnsupportedProtocol` read "use a supported
    protocol: http, https, graphql, or websocket" — the parser rejects `https`, so a user
    who followed the hint hit the same error again.
  - MANUAL §1.5's "What isn't covered yet" paragraph was stale in both directions — all
    five things it named were already covered, and it named none of the nine real gaps —
    and is rewritten to state what the schema covers and what it deliberately leaves to
    the parser. CLI_SPECIFICATION Appendix A's divergence note is replaced, and two claims
    it made are corrected: `method` is optional, and `graphql.error_handling` accepts
    `ignore` as well as `fail` and `warn`. (M21-001)
- **The 53 failing web E2E specs never tested anything; replaced with component tests.**
  Fifteen Playwright specs stubbed backend calls with `context.route('**/api/v1/…')` and
  asserted the stub rendered. Every page they targeted loads its data in a SvelteKit
  `+page.server.ts` `load`, which runs in the web container — Playwright's browser-level
  interception never saw those requests, so the real backend's data rendered instead and
  the assertions failed. A probe confirmed it: zero browser-level `/api/v1/**` requests
  during a page load that rendered a fully populated table. Running against a host
  `npm run dev` was verified not to help — SSR is SSR wherever node runs.
  - The specs are replaced by `*.test.ts` component tests (`@testing-library/svelte`,
    rendering `+page.svelte` against a `data` prop) and `*.server.test.ts` tests that
    drive each `load`/`action`/`+server` directly with an injected `fetch`. Edge cases
    that were impractical to seed — 402 tier gates, 409 conflicts, 429 rate limits, 422
    weak-password and suspicious-value responses — are now covered rather than mocked at
    a layer that did nothing.
  - `npm run test:unit` goes from 350 to 533 tests and still finishes in ~7s, and it is
    already part of the web CI gate — so this coverage is gated for the first time.
  - The five convergence specs (`full-pipeline`, `enterprise-full`, `m14-revenue-loop`,
    `m16-happy-path`, `m18-compliance`) are untouched and still pass.
  - Also removed `account-data-{export,delete}.spec.ts` from `src/routes/`: they asserted
    API-client behaviour already covered by `src/lib/api/*.test.ts`, plus `?raw`
    source-greps standing in for the component rendering that now has real tests.
- **E2E convergence specs seed over HTTP; the E2E gate runs again.** The five specs now
  drive the same REST endpoints the CLI used to call (`POST /organizations/{id}/results`,
  `POST /pr-checks`, `POST /telemetry/events`, the auth and export endpoints), mint their
  own dev token, and skip themselves when the backend is unreachable. They need no
  `curlew` binary, no `CURLEW_BACKEND_TOKEN`, and no shell orchestrator.
  `web/tests/e2e/helpers/cli.ts` is replaced by `helpers/seed.ts`.
- **The docker-compose test stack could not start.** The rebrand renamed the CLI but not
  the backend's `ApiTool:` configuration root, leaving the stack broken in five separate
  places. All now fixed:
  - `docker-compose.test.yml` set `CURLEW__APP__WEBAPPURL`, which the backend ignores; it
    died on startup validation demanding `APITOOL__APP__WEBAPPURL`.
  - `scripts/test-token.sh` minted JWTs with issuer/audience `curlew-dev` while the
    backend validates `apitool-dev`, so every seed script got 401.
  - `scripts/ci-local.sh` scoped its backend and E2E gates on `src/Curlew.Backend`, a path
    that does not exist — backend changes silently skipped both gates.
  - `GitHub__ApiBase` never bound (the env provider maps it to an unread key), so the
    backend called the real api.github.com instead of the github-mock sidecar.
  - No GitLab KEK was configured, and `POST /api/v1/pr-checks` resolves the GitLab poster
    for every request — so every pr-check upload returned 500.
- **Check runs never posted from the test stack.** The GitHub App key/app id were
  unconfigured, and `/internal/test/seed-m14` wrote `repo_set` as a bare string array
  while `CheckRunPoster.IsRepoCovered` reads it as objects — the resulting exception was
  swallowed into a silent "queued". pr-checks now reach `posted` with a check-run id.
- **Billing receipt emails threw on the alpine image.** The runtime image ships without
  ICU, so `CultureInfo.GetCultureInfo("en-US")` failed in globalization-invariant mode and
  took down the `invoice.payment_succeeded` handler. The image now installs `icu-libs`.
  The test stack also configures the Stripe webhook secret and points the live gateway at
  stripe-mock, without which replayed webhooks were rejected unverified.
- **The vault-config page told users to run a command that no longer exists.** Its
  "Generate CLI snippet" panel emitted `curlew license --refresh` and described refreshing
  a cached template "on a worker node" — three things the CLI no longer has (no `license`
  command, no vault cache, no workers). The snippet is now the actual consumption path:
  save the template to a file, point `CURLEW_TEAM_CONFIG` at it, and pass `--env` to
  select the environment backing the collection's `{{secrets.X}}` tokens.

### Added
- **`docs/CLI_SPECIFICATION.md` — the CLI now has its own specification (v1).** The
  rename and the backend separation left one document describing two products;
  `docs/SPECIFICATION.md` still opened with the pre-rename title and a five-tier
  business model. The CLI half is now extracted into a standalone spec covering scope
  and the locality guarantee, design principles, execution model, project layout, every
  file format, the variable system, assertions, extraction, retry, data-driven testing,
  parallel execution, protocols, secrets, signing, plugins, output, exit codes, the
  command reference, the UI, `perf`, telemetry, CI, limits, and a 14-point conformance
  checklist.
  - Content was verified against the binary, not carried over on trust. Corrections
    made in the process: the exit-code table drops `6` (feature gate) and `10` (worker
    unauthorized) and records that usage errors split between `1` and `2` depending on
    the subcommand; Content-Type detection is `mime.TypeByExtension`, not the fixed
    table the platform spec listed; and multipart uploads, `raw_file`, `save_to`,
    `--fixed-time`, the 1,000-request ceiling, gRPC and SSE are recorded in Appendix B
    as designed-but-never-implemented rather than documented as features.
  - `docs/SPECIFICATION.md` is retitled "Curlew Platform Specification" and scoped to
    the `src/` backend and `web/` dashboard, which still implement it.
- **M20-004: MANUAL.md locale reference + cross-locale seed reproducibility matrix.** (M20-004)
  - `docs/MANUAL.md` faker section rewritten: 15-row locale reference table (all codes, language/region, name-format exemplar, phone-format exemplar), 5-level precedence chain (Default < Project < Environment < Collection < CLI flag), fallback chain (`en-GB → en → en-US`), and `ERR_LOCALE_UNKNOWN` reference.
  - Seven M13 deferral notes removed; replaced with live `--locale` documentation.
  - `TestLocale_ReproducibilityMatrix`: table-driven test over all 15 locales × fixed seed, asserting byte-identical output across two independent registries for `$faker.fullName`, `$faker.phone`, `$faker.city`, `$faker.firstName`.
  - `TestLocale_ReproducibilityMatrix_SameDrawOrder`: documents SPEC:1023-1026 seed/locale position-invariant draw guarantee; pool-membership assertion per locale.
  - `testdata/locale/reproducibility-matrix.yaml`: worked CLI example demonstrating de-DE reproducibility end-to-end.

- **M20-003: CJK + Cyrillic locale pools (ja-JP, zh-CN, ko-KR, ru-RU).** (M20-003)
  - Four non-Latin-script locale data tables added to the faker `localeData` lookup.
  - `familyNameFirst bool` seam added to `localeData`; `$faker.fullName` renders family-name-first for ja-JP/zh-CN/ko-KR and given-name-first for ru-RU — en-US and all Latin locales are zero-value no-ops.
  - Native-script name pools (kanji/kana, Han, Hangul, Cyrillic) confirmed by Unicode-range assertion per script block.
  - Phone formats per SPEC:973-987: `+81 d-dddd-dddd` (ja-JP), `+86 dd dddd dddd` (zh-CN), `+82 d-dddd-dddd` (ko-KR), `+7 ddd ddd-dd-dd` (ru-RU).
  - UTF-8 round-trip through JSON confirmed for every pool entry (all four locales).
  - Seed/locale position-invariant test (`TestRegistry_Locale_SeedPositionInvariant`) extended to all 15 locales.

- **M20-002: Nine Latin-script European locale pools (en-GB, fr-FR, es-ES, it-IT, pt-BR, nl-NL, pl-PL, sv-SE, tr-TR).** (M20-002)
  - Each locale ships its own first/last name pool, city pool, and a phone formatter producing the spec-documented number shape.
  - en-GB now resolves to its own native pool; the M20-001 fallback path (`en-GB → en → en-US`) is bypassed and no fallback warning is emitted.
  - tr-TR Turkish codepoints (ı U+0131, İ U+0130, ş, ç, ö, ü, ğ) stored verbatim and confirmed valid UTF-8.
  - Seed/locale position-invariant test extended to cover all 11 locales.
  - Pool-shape invariant test (non-empty, no empty/whitespace entries, valid UTF-8, locale-appropriate phone regex) passes for all nine locales.

- **M20-001: `--locale` flag, config precedence + fallback chain, `ERR_LOCALE_UNKNOWN`, de-DE shipped end-to-end.**
  - `--locale <code>` flag on both `curlew run` and `curlew exec`; selects the faker pool for all `$faker.*` functions.
  - Precedence chain (lowest to highest): Default (en-US) → project `config.locale` → environment (seam, wired in M20) → collection `config.locale` → `--locale` flag. Collection and project `config:` blocks parse the new `locale:` field.
  - Fallback chain (SPEC:989): `en-GB → en → en-US`; verbose `-v` warning when a fallback occurs.
  - `ERR_LOCALE_UNKNOWN` structured error for unsupported locale codes; lists all 15 supported locales in the hint.
  - de-DE first/last name, city, and phone (+49 format) pools shipped as the first non-en-US locale.
  - Seed/locale position-invariance (SPEC:1023-1026): same seed selects the same zero-based pool index regardless of locale.
  - en-US output is byte-identical to pre-M20 (backward compat).
  - `exec` subcommand now always wires the dynamic registry so `$faker.*` resolves without `--var` flags.

- `curlew ui` — the local web UI (runner & inspector) specified in
  `docs/UI_SPECIFICATION.md` v1.0, implemented end to end. A loopback-only HTTP
  server embedded in the single binary serves a Svelte SPA plus a JSON API:
  live runs stream over WebSocket as verbatim events-schema objects with
  gapless replay-from-event-id; REST is authoritative for full redacted
  bodies/headers/timing and wave structure; runs are inspectable mid-run.
  Includes the run view (compact/columns/lanes), the response inspector
  (Body/Headers/Assertions/Timing/Request/Error tabs, JSON tree, binary
  panel), run comparison with a client-side Myers diff (Solo tier via the new
  `ui_run_history` feature), the `.curlew/ui/` persisted history store with
  retention and a self-ignoring `.gitignore`, batch runs across collections
  (Professional `test_discovery` gate, additive `runner.VarSources.RequestIDPrefix`),
  per-start session token + Host/Origin validation, always-on redaction (no
  `--allow-sensitive`), a file watcher driving live tree refresh, the `ui:`
  config block in `curlew.yaml` (+ project JSON schema), open-in-editor, and
  Playwright e2e against the real binary. New packages: `internal/uiserver`,
  `internal/runservice` (the load-and-run pipeline extracted from
  `cmd/curlew`, including the moved events `EmitterSink` and the
  sensitive-set builders), and the `ui/` SPA workspace.
- Events schema v1.3 (additive): `request.end` gains an optional `timing`
  object — connection-phase breakdown in integer microseconds measured via
  `net/http/httptrace` (`dns_us`, `connect_us`, `tls_us`, `ttfb_us`,
  `download_us`, `total_us`, `connection_reused`, `attempts`) — benefiting
  plain `curlew run --events` users independent of the UI.
  `docs/EVENTS_SCHEMA_v1.3.md` + `docs/events-schema/v1.3.json`; v1.2
  artifacts retained as historical anchors.

### Fixed
- `curlew validate` no longer warns that a variable "may not be defined at runtime"
  when it is defined in the project config. The undefined-variable heuristic now loads
  `curlew.yaml` (or `curlew.yml`) by walking up from the collection's directory, the
  same way `curlew run` does, so a freshly scaffolded project validates cleanly instead
  of warning about its own `base_url`. The warning hint lists the project config among
  the definition sources. An unparsable `curlew.yaml` is ignored rather than failing
  collection validation.
- License and backend authentication are separated correctly: release builds derive
  product tier only from verified license JWTs, while API access tokens are stored in
  the OS keychain (or encrypted-file fallback), refreshed on 401, and accepted by the
  backend's ES256 bearer validation without breaking HS256 web sessions.
- Team-vault provider profiles now reach interactive and scheduled execution, including
  AWS/Azure provider construction, environment selection, cache refresh, and access-token
  rotation for long-running schedule workers.
- Organization API responses now carry the authoritative subscription tier; dashboard
  guards and navigation distinguish Team-or-higher from Enterprise-only features.
- The custom-role editor's PATCH request now has a matching owner-only backend endpoint
  with create-equivalent validation, duplicate-name protection, and audit logging.
- Dashboard production builds now use a compatible Svelte 5 toolchain, and the local CI
  web gate runs type checking, lint, unit tests, and the production build.
- Locale precedence is consistently Default → project → environment → collection → CLI
  in both the direct CLI and extracted runservice paths. Environment files accept a
  `config.locale` block, and project-root environment lookup works for scaffold layouts.
- Parallel-execution tests no longer assert wall-clock bounds (70–120ms),
  which flaked under CI load: the five concurrency tests in
  `internal/parallel`, `internal/runner`, and `cmd/curlew` now prove
  parallelism via an in-flight rendezvous probe — at least two requests must
  be mid-flight simultaneously, which sequential execution can never produce
  regardless of machine speed. The tests also got faster (the 40–50ms sleeps
  are gone).
- Smoke gate is now hermetic: `smoke/run.sh` starts a local
  httpbin-compatible fixture server (`smoke/fixtures/httpbin_server.py`,
  127.0.0.1:9190) and every executing check targets it instead of the public
  httpbin.org — a slow/unavailable httpbin had failed the CI gate twice on
  network weather alone. `sample/hello.yaml` and the `init` scaffold defaults
  still point at httpbin.org as user-facing documentation; the smoke run
  executes a URL-rewritten copy (and overrides the scaffold's `base_url` via
  `--var`). The `from_command` check's network-tolerance SKIP branch is now a
  hard failure, since a local fixture leaves no weather to tolerate.
- Retry: `retry_on.status_ranges` now accepts the `"Nxx"` class shorthand
  (`"5xx"` → 500–599, case-insensitive) that `docs/MANUAL.md` §5.4 has always
  documented — previously only `"min-max"` parsed and the shorthand was
  silently skipped, so a manual-following config got no retries. Unparsable
  ranges are no longer silently ignored either: `curlew validate` reports
  them as errors at every site (collection/section/request, `retry_on` and
  `do_not_retry_on`).
- Events schema: the published JSON Schema constrained `request.end.wave_index`
  to `minimum: 0`, but sequential runs have always emitted `-1` — every
  sequential stream violated the published schema. v1.3 corrects the
  constraint to `minimum: -1` (schema fix only; emission unchanged).
- Events schema docs: `phase` was documented as `setup|test|teardown` while the
  runner emits `setup|main|teardown`; the v1.3 document matches the emitter.
- `--env` resolution falls back to the project root when `environments/` is
  not found next to the collection file in both the extracted run pipeline
  (`internal/runservice`) and the direct CLI path. This matches the scaffolded
  layout produced by `curlew init`.
- Docs: `docs/UI_SPECIFICATION.md` v1.0 — complete specification for `curlew ui`, a
  localhost runner-and-inspector web UI embedded in the Go binary. Covers the command
  surface, REST + WebSocket API contract, run orchestration (incl. the `internal/runservice`
  extraction plan), the events-schema v1.3 delta (httptrace phase timing), the `.curlew/ui/`
  run-history store (Solo tier via new `ui_run_history` feature), the security model, the
  full SPA behavioral spec (Svelte 4 + Vite in a new `ui/` package), and an 11-slice
  milestone decomposition. Adversarially verified against the codebase. The reviewed design
  prototype (design-tool export) is committed at `docs/design/curlew-ui/`; its
  `at-tokens.css` is the design-token ground truth. Specification only — no implementation
  ships with this entry; folds into SPECIFICATION.md v4.5 when the milestone is scheduled.
- CI: structural guard against silent smoke-test failures. `smoke/run.sh` now defines a
  `fail()` helper (FAIL line + optional context lines + `exit 1`, EXIT-trap friendly) and
  119 single-line FAIL branches were migrated to it; multi-line branches that need extra
  cleanup keep their explicit `exit 1`. `scripts/ci-local.sh` gained a `smoke-fail-lint`
  step (before the smoke step) that flags any `echo "FAIL...` in `smoke/run.sh` without an
  `exit 1` on the same line or within the next 6 lines — the pattern that previously let
  eight failed assertions report PASS.

### Fixed
- Smoke: the `--format junit` check still asserted the Professional-tier gate that
  M11-001 removed, so it printed `FAIL: ... got: <?xml ...` on every run — and the
  failure never propagated because its FAIL branch (and seven others: watch clean-exit,
  watch `--format json` ×2, `--parallel`, `data_driven`, `--format html` ×2) did not
  `exit 1`, letting `ci-local.sh` report PASS over a failed assertion. The junit check
  now asserts JUnit XML is emitted at Free tier, and all eight FAIL branches fail the
  run. `smoke/run.sh` also now forces a deterministic Free-tier environment up front
  (exports `CURLEW_TIER=free`, points `CURLEW_CONFIG_DIR` at an isolated empty temp
  dir, clears `CURLEW_LICENSE_BUNDLE`) so results no longer depend on the developer's
  license cache — e.g. a grace-expired `license.json` aborting runs with exit 9 — and
  the license/telemetry sections restore that isolated default instead of unsetting
  `CURLEW_CONFIG_DIR` back to the real machine config dir.

### Added
- E2E: M18 compliance convergence proof — M18 closes as a unit; this is the launch-blocking
  signal per `project_milestone_release_model.md` (M18-012, v4-1 through v4-15).
  New Playwright spec `web/tests/e2e/m18-compliance.spec.ts` plus shell orchestrator
  `scripts/m18-e2e.sh` drive the full M18 happy path against the stack brought up by
  `./scripts/ci-local.sh --full`. The scenario proves all 12 M18 capabilities wire together
  end-to-end: (1) register via seed-refresh; (2) email verification; (3) `curlew telemetry
  enable` — install_id created with mode 0600; (4) `curlew run` emits `run.completed`
  telemetry event confirmed via new `GET /api/v1/internal/test-hooks/list-telemetry-events`
  hook; (5) data export bundle — signed-URL JSON carries InExport table set; (6) deletion
  request → cancel mid-window → re-initiate; (7) backdate `pending_deletion_at` via new
  `POST /api/v1/internal/test-hooks/backdate-deletion-request` hook + run finalizer;
  (8–9) audit-log rows anonymised (`actor_email` matches `deleted-user-[0-9a-f]{8}`) and
  `user.anonymised` row present; (10) Enterprise JSONL streaming export is `chunked +
  application/x-ndjson`; (11) vault-config cleartext round-trip verified (M18-009 envelope
  proof); (12) `curlew telemetry delete-request` removes local install_id file and posts
  `telemetry.delete_request` marker. Two new Dev/Testing-only backend endpoints:
  `GET /api/v1/internal/test-hooks/list-telemetry-events?install_id=<uuid>` (returns
  `{ count, rows }` for cross-cluster telemetry assertions) and
  `POST /api/v1/internal/test-hooks/backdate-deletion-request` (sets
  `users.pending_deletion_at = now() - DaysAgo` to skip the 30-day wall-clock wait in CI).
  New test fixture `testdata/m18/e2e-collection.yaml`. CI workflow
  `.github/workflows/m18-e2e.yml` (workflow_dispatch-only). `scripts/ci-local.sh --full`
  extended with `step "m18 compliance e2e"` after the M16 step. Architectural notes:
  telemetry_events is keyed by install_id (not user_id by design) so the GDPR export bundle
  cross-cluster proof runs via the list hook rather than through the GDPR bundle manifest
  (documented in plan Architectural Decision 5). No separate fake-KMS container needed —
  FileTeamVaultKeyProvider is the "self-hosted profile" KMS shim (Decision 3). SQLite
  `quote()` satisfies the raw-column ciphertext check without psql (Decision 4). (M18-012,
  v4-1 through v4-15)
- Docs: Compliance artefacts (2/2) — vendor inventory + customer/internal DFDs +
  pen-test orchestration (M18-011, v4-14, v4-15). `docs/security/vendor-inventory.md`
  enumerates Stripe, SendGrid, Google Cloud KMS, AWS, GitHub Apps, GitLab in a
  seven-column table (Vendor | Service | Data shared | Retention | Breach
  notification SLA | Vendor SOC 2 status | DPA on file). `docs/security/data-flow-customer.md`
  and `docs/security/data-flow-internal.md` document every customer-PII and
  internal-secrets path with embedded Mermaid diagrams; the internal DFD
  reflects the M18-009 envelope-encryption surfaces (`team_vaults`,
  `schedules.env_vars`, KMS-wrapped signing keys). `docs/security/pentest-2026-Q2.md`
  is the first-engagement artefact (Cure53, engagement window 2026-06-01 to
  2026-06-30) carrying Scope, Methodology, Findings, Remediation log, and
  Sign-off. `docs/COMPLIANCE.md` updated: forward references promoted to live
  cross-links and "Pen-test Cadence" section names 2027-Q2 as the next target
  window with Engineering as the budget-line owner. (M18-011, v4-14, v4-15)
- Docs: Compliance artefact umbrella + policy templates + data-classification matrix
  (M18-010, v4-14). `docs/COMPLIANCE.md` rewritten as the umbrella TOC with full
  document inventory and forward references to M18-011 (vendor inventory, DFDs, and
  pen-test artefacts). Four new policy artefacts: `docs/security/info-sec-policy.md`
  (scope, acceptable use, access control, change management, vendor management,
  incident response, business continuity); `docs/security/access-review-policy.md`
  (quarterly cadence, Owner/Admin reviewer, all OrganizationMember rows + all
  CustomRole assignments + Security Auditor role population reviewed, signed-off CSV
  evidence at `docs/security/access-reviews/YYYY-Q.csv`, separation-of-duties
  rationale for `audit_log.view`-only auditor role per v4-3);
  `docs/security/incident-response-runbook.md` (SEV-1/2/3/4 taxonomy, on-call rota
  expectation, customer-facing and internal comms templates, post-incident review
  within 5 business days, sample incident timeline);
  `docs/security/data-classification-matrix.md` (Public/Internal/Confidential/
  Restricted four-tier mapping of all 36 schema-appendix tables with rationale and
  encryption-at-rest posture; M18-009 envelope encryption referenced for
  `team_vaults.template_jsonb` and `schedules.env_vars`). New
  `DataClassificationMatrixDocTests` xUnit class (DocConsistency trait) asserts the
  matrix document exists, carries the four classification tiers, references M18-009
  envelope encryption, every table in `data-inventory.md` appears in the matrix
  (inventory ⊆ matrix), all 36 schema-appendix tables are classified, and
  `docs/COMPLIANCE.md` cross-links to the matrix. (M18-010, v4-14)
- Backend: Encryption-at-rest v2 — AES-256-GCM envelope encryption for `team_vaults.template_jsonb` and `schedules.env_vars` (M18-009, v4-12, v4-13). New `EnvelopeCodec` promoted to `ApiTool.Backend.Crypto` as the shared wire format (identical to `GitLabEnvelopeCodec`; no behaviour change). `ITeamVaultKeyProvider` (file/KMS variants: `FileTeamVaultKeyProvider`, `GoogleKmsTeamVaultKeyProvider`) wraps per-row DEK with a KEK and stores `template_jsonb_ciphertext + template_jsonb_kid` on `team_vaults`. `IScheduleEnvKeyProvider` (same pattern) stores `env_vars_ciphertext + env_vars_kid` on `schedules`. EF migration `AddEncryptedTeamVaultAndScheduleEnvVars` adds all four nullable ciphertext/kid columns (reversible; expand-contract pattern). `TeamVaultBackfillHost : IHostedService` encrypts existing plaintext rows on first boot (batch 100, verification-then-abort guard, non-blocking). `VaultConfigService` encrypts on write, decrypts on read; validator runs against cleartext before encryption (closes v3-10 deferral). `ScheduleExecutorService.ClaimNextAsync` decrypts env_vars and populates `NextRunResponse.EnvVars`; null ciphertext returns empty dict (closes spec `:11057` deferral). `scripts/check-signing-keys.sh` CI lint fails SaaS builds when any `signing_keys.kms_key_id IS NULL`; `scripts/ci-local.sh --check-signing-keys` delegates to this script (v4-13). The YAML's `dek_ciphertext` column is realised as `*_kid` (TEXT ≤ 500 chars) because the wrapped-DEK bytes are already embedded inside the envelope blob per `EnvelopeCodec.Pack` — documented as planned deviation. (M18-009, v4-12, v4-13)
- CLI: Telemetry Phase 3 emitter + `curlew telemetry` subcommand (M18-008, v4-8, v4-11). New `internal/telemetry/` package with a dedicated HTTP client (no Bearer auth, does not import `internal/backend`). Persistent `install_id` (UUIDv4, mode 0600) at `~/.config/curlew/install_id`; consent file at `~/.config/curlew/telemetry.json`. Per-event `Idempotency-Key` UUID header. Subcommand `curlew telemetry {enable,disable,status,reset-id,export,delete-request}` registered in `runWithWriters`. `delete-request` POSTs `event_type=telemetry.delete_request` before removing local files (spec :6657). `curlew run` emits a fire-and-forget `run.completed` event (2s bounded) with `install_id`, `session_id`, `duration_ms`, `collection_size`, `exit_code`. `docs/SPECIFICATION.md` updated with v4-8 supersession callout. (M18-008, v4-8, v4-11)
- Backend: Telemetry Phase 3 ingest pipeline (M18-007, v4-8, v4-9, v4-10). Anonymous `POST /api/v1/telemetry/events` endpoint — no authentication required per v4-9 opt-in consent model. Body ≤ 64 KB (413 if exceeded); `Idempotency-Key` header required; per-`install_id` rate limit 60/min (429). Idempotency dedup via pre-check + UNIQUE constraint on `idempotency_key` — replay returns 202 no-op without re-inserting. EF migration `AddTelemetryTables` creates `telemetry_events` (idempotency key UNIQUE, jsonb payload, received_at index, install_id+received_at composite index) and `telemetry_daily_aggregates` (composite PK `(day, event_type)`). Both tables are reversible. `TelemetryAggregatorHost : BackgroundService` ticks daily at 04:00 UTC, aggregates closed UTC days from `telemetry_events` into `telemetry_daily_aggregates` (one row per `(day, event_type)` with count, distinct-install-count, and numeric sums/min/max/avg/n for a whitelist of payload keys: `duration_ms`, `collection_size`, `request_count`, `failure_count`, `success_count`). Re-runs are idempotent — already-aggregated days are skipped. `TelemetryPurgeHost : BackgroundService` ticks daily, hard-deletes `telemetry_events` rows older than 90 days; daily aggregates in `telemetry_daily_aggregates` persist indefinitely. Two Dev/Testing-only internal test hooks for deterministic e2e testing: `POST /api/v1/internal/test-hooks/run-telemetry-aggregator` and `POST /api/v1/internal/test-hooks/purge-telemetry-events`. OpenAPI documents the `Idempotency-Key` header, 202/413/429 response shapes, and anonymous auth. Wire-shape contract recorded in `docs/SPECIFICATION.md § Telemetry Phase 3 Implementation Pipeline` for M18-008 CLI emitter. (M18-007, v4-8, v4-9, v4-10)
- Backend: Concrete `UserAnonymiser : IUserAnonymiser` replacing the M18-005 stub. Per spec v4-6: every audit-log row attributed to the deleted user has `actor_id` set to NULL and `actor_email` set to `deleted-user-{first8(sha256(user_id||org_id))}` — a deterministic, irreversible token distinct per (user, org). Hard-deletes rows in the InDeletionHard set (`refresh_tokens`, `email_verification_tokens`, `password_reset_tokens`, `deletion_reauth_tokens`, `organization_members`). Nulls `CreatedBy`/`UpdatedBy` columns on `custom_roles`, `schedules`, `coordinator_jobs`, `team_vaults`, `notification_rules`. Scrubs `users.email`, `users.password_hash`, clears `users.email_verified` and `users.is_admin`; sets `users.anonymised_at`. Emits one `user.anonymised` audit-of-audit row per affected org with the per-org token in payload (`actor_id = null`, `actor_email = token`). (M18-006, v4-6)
- Backend: EF migration `AnonymiseSetNullColumns` widens 7 `Guid` columns to `Guid?` for the SetNull anonymisation: `organization_audit_log.actor_id` (also renamed from `ActorId`), `custom_roles.CreatedBy`, `schedules.CreatedBy`, `coordinator_jobs.CreatedBy`, `team_vaults.CreatedBy`, `team_vaults.UpdatedBy`, `notification_rules.CreatedBy`. Reversible. (M18-006)
- Backend: `LastAdminProtectionService` extension to `POST /api/v1/users/me/deletion-requests`. A user who is sole Owner of any org with other members receives 409 `owner_cannot_leave` with body `{ blocking_orgs: [{slug, name}, ...], error_code: "OwnerCannotLeave" }`. A user who is sole Owner of an org with zero other members has that org marked `OrgStatus.PendingDeletion` in the same transaction. Owner-alongside-another-Owner is not blocking. The re-auth token is preserved (not consumed) when the request is blocked. (M18-006, v4-7)
- Web: "Delete account" panel on `/account/data` with re-auth modal and blocking-orgs preview; `/account/data/cancel-deletion` page surfaces the in-window Cancel action. (M18-006)
- Docs: `docs/security/data-inventory.md` extended with the "Anonymisation function (M18-006)" subsection — deterministic token shape, irreversibility property, audit-of-audit trail. (M18-006)
- Backend: GDPR account-deletion state machine (M18-005, v4-5). `POST /api/v1/users/me/deletion-requests` queues a 30-day cooldown deletion; requires `X-Reauth-Token` from a fresh `drto_` token (obtained via `POST /api/v1/auth/reauth` with current password). `POST .../cancel` aborts within the window and emits an `account.deletion_cancelled` audit event. `GET .../status` reports current state. `UserDeletionFinalizerHost : BackgroundService` ticks daily at 03:00 UTC, invokes `IUserAnonymiser` for users past cooldown, and enqueues `account_deletion_completed` email. `StubUserAnonymiser` (M18-005) sets `anonymised_at` and clears `pending_deletion_at`; full PII scrub ships in M18-006. `DeletionReauthService` issues and atomically consumes single-use 5-minute `drto_` tokens. EF migration `AddDeletionStateMachine` adds `users.pending_deletion_at`, `users.anonymised_at`, and `deletion_reauth_tokens` table (InExportInDeletionHard). Two new email templates: `account_deletion_initiated` (cancel URL + finalize date) and `account_deletion_completed`. Internal test-hook `POST /api/v1/internal/test-hooks/run-deletion-finalizer` drives one finalizer tick deterministically in tests. (M18-005, v4-5)
- Backend/Web: GDPR user data export (M18-004, v4-4). `POST /api/v1/users/me/export-requests` queues a per-user JSON bundle export (202); 24h per-user rate-limit returns 429 with `Retry-After` and code `export_rate_limited`. `GET /api/v1/users/me/export-requests/{id}` polls status (`queued` → `building` → `ready` | `failed` | `expired`) and returns a time-limited `signed_url` when ready. `UserExportBuilderHost : BackgroundService` claims queued rows by optimistic update, uses `UserExportBundleAssembler` (reflection-based LINQ predicate from `[GdprUserAttribution]`) to fetch rows from all 6 InExport tables per manifest, strips `password_hash`/`token_hash`, and uploads the JSON bundle via `IObjectStore`. `InMemoryObjectStore` (test/dev) stores blobs in a `ConcurrentDictionary`; `GET /api/v1/internal/object-store/{**key}` serves them in Dev+Testing. Internal test-hook `POST /api/v1/internal/test-hooks/run-export-builder` drives one builder tick deterministically in tests. SvelteKit `/account/data` page lets users request and download their export with 5s polling until a terminal status is reached. (M18-004, v4-4)
- Backend/docs: GDPR data inventory (M18-003, v4-4). Two property-level attributes — `[GdprIncluded(GdprDisposition)]` and `[GdprAnonymise(AnonymiseAs)]` — plus class-level `[GdprTable(ExcludedFromBoth | NotUserAttributable)]` annotate the 13 user-attributable EF entities (`User`, `RefreshToken`, `OrganizationMember`, `EmailVerificationToken`, `PasswordResetToken`, `NotificationRule`, `CustomRole`, `Schedule`, `CoordinatorJob`, `TeamVault`, `OrganizationAuditLogEntry`, `GithubInstallation`, `GitLabInstallation`) with one of four dispositions: `InExportInDeletionHard`, `InExportInDeletionAnonymise`, `ExcludedFromExportAnonymisedInDeletion`, `ExcludedFromBoth`. `GdprAttributeScanner.Scan(Assembly)` and the lazy `GdprAttributeScanner.Manifest` static produce a `GdprBundleManifest` consumed by M18-004 (export builder) and M18-006 (anonymiser). `docs/security/data-inventory.md` is the human-reviewable decision matrix; `docs/COMPLIANCE.md` introduces the compliance-artefact umbrella. `GdprInventoryCoverageTests` is a CI guard — any future user-attributable entity added without GDPR attributes fails the build naming the offending entity. `GdprInventoryDocConsistencyTests` asserts every row in the inventory doc matches the scanner's disposition exactly. (M18-003, v4-4)
- Backend: Audit-log RBAC + per-org retention (M18-002, v4-2, v4-3). `organizations.audit_log_retention_days` column (INTEGER NOT NULL DEFAULT 365); non-Enterprise orgs capped at 365 via `PATCH /api/v1/organizations/{orgId}` (new `audit_log_retention_days` field, returns 400 `retention_days_exceeds_cap` if > 365 without Enterprise). `AuditLogCleanupHost : BackgroundService` runs daily at the configured tick interval, iterates all orgs, and hard-deletes rows via EF Core `ExecuteDeleteAsync` per org. Emits `System.Diagnostics.Metrics` counter `audit_log_cleanup_rows_deleted_total` (tag: `org_id`) and structured log entry per org with deletion count. New `audit_log.view` and `audit_log.export` permission constants added to `Permissions.cs`; both granted to built-in Owner and Admin roles; Member unchanged. Hardcoded Owner/Admin gate at `AuditLogQueryService.cs` lifted to `RoleResolver.HasPermissionAsync` calls: paginated path requires `audit_log.view`; export path requires `audit_log.export`. Export 403 body now names the missing permission (`"Missing permission: audit_log.export."`). Internal test-only hook `POST /api/v1/internal/test-hooks/run-audit-cleanup` (Dev+Testing only, `InternalAccessFilter` guarded) triggers one cleanup tick deterministically for e2e assertions. `docs/SPECIFICATION.md` permission matrix extended with `audit_log.view` and `audit_log.export` rows and a Security Auditor template subsection. EF migration `AddOrganizationAuditLogRetentionDays` is reversible (down-migration drops the column). (M18-002, v4-2, v4-3)
- Backend: `GET /api/v1/organizations/{orgId}/audit-log?format={jsonl,csv}` now streams the full filtered audit log via `Transfer-Encoding: chunked` (`Content-Type: application/x-ndjson` or `text/csv`, `Content-Disposition: attachment; filename="audit-log-{orgId}-{yyyyMMddTHHmmss}.{ext}"`). The previous `MaxLimit=200` cap no longer applies for export-format requests; the paginated JSON path (no `format`, or `format=json`) is unchanged and still clamps at 200. `AuditLogExportTierGate` (Enterprise) gates both export formats; ineligible orgs receive RFC 7807 402 Payment Required with `current_tier`/`required_tier`/`code=audit_log_export_tier_ineligible`. Owner/Admin RBAC remains hardcoded at endpoint entry pending the `audit_log.export` permission (M18-002). Per-row streaming flushes every 100 rows; `HttpContext.RequestAborted` propagates through the EF Core enumerator so client disconnects release the DB connection. (M18-001, v4-1)
- CLI: `curlew init --skill claude` now materialises a multi-file skill payload — a root `SKILL.md` (trigger phrases + topic index) alongside 10 per-topic reference files (`variables.md`, `output-formats.md`, `assertions.md`, `retry.md`, `parallel.md`, `vault.md`, `signing.md`, `expressions.md`, `exit-codes.md`, `failure-playbook.md`). `expressions.md` documents the CEL surface (`if:`, `assertions: - cel:`, standard activation, disabled functions, decision table). `failure-playbook.md` adds entries for `ERR_CEL_PARSE` and `ERR_CEL_TYPE` pointing at `curlew validate`. The embedded-FS walker recursively writes all files; per-file skip-if-exists semantics preserve user-edited content. (M19-006)
- CLI: `curlew validate` CEL parse/type-check — walks every `if:` and `assertions: - cel:` site in the parsed collection and reports two new error codes: `ERR_CEL_PARSE` (syntactically invalid or disabled-function expression) and `ERR_CEL_TYPE` (expression compiles but returns a non-bool type; message names the actual type). Field paths follow the v4.4 spec (`setup[i].if`, `requests[i].assertions[j].cel`, etc.). Source excerpts are truncated to 200 runes with an ellipsis marker. `curlew validate` performs no HTTP requests. Smoke fixtures `cel_validate_good.yaml` and `cel_validate_bad.yaml` exercise both paths. `docs/MANUAL.md §3.10 Expression Language (CEL)` added with standard activation table, integration-site reference, operator-vs-CEL decision table, disabled-function list, and error-code reference. (M19-005)
- CLI: `assertions: cel:` list shape — CEL boolean assertions evaluated at runtime against the standard activation `{response, previous, vars, env}`. Each `- cel: "<expr>"` entry compiles once per run via the per-run `assertProgCache`, evaluates against the HTTP response, and produces one `Result` in the assertion output. On pass: recorded as passing; on fail: the literal expression source plus the resolved value of each top-level named reference (e.g. `response.body.total = 9.5`) appears in the failure message. Sensitive values in the failure message are substring-replaced with `[REDACTED]`. Type errors from `Compile` (non-bool expressions) surface as assertion failures, not panics. Mutual exclusion: a mapping-form entry that carries both `cel:` and an operator key (e.g. `eq:`) is rejected at parse time with `ErrCelAndOperatorMutuallyExclusive` (code `PARSE_CEL_OPERATOR_EXCLUSIVE`). Parallel runs with any `cel:` assertions fall back to sequential execution. `CollectTopLevelRefs` in `internal/cel/` performs source-level splitting on binary/boolean operators to enumerate named references for failure messages. (M19-004)
- CLI: `if:` field on request items — CEL boolean gate that runs before per-request templating. A falsy expression skips the request with reason `"if: false"` and prevents any `{{from_command:}}`, vault fetch, or faker seed call. `depends_on:` list on request items propagates the skip to downstream items with reason `"parent skipped: <name>"`. `previous` binding in CEL expressions refers to the last non-skipped response in the current phase. Parallel runs with any `if:` fall back to sequential execution automatically. `curlew validate` surfaces `ERR_CEL_PARSE` and `ERR_CEL_TYPE` for malformed or non-boolean `if:` expressions with 200-rune source truncation. All six output formatters render `skipped` status; terminal format uses `SKIPPED  <name>  (<reason>)` single-line style. (M19-001)
- CLI: `internal/cel/` package — CEL expression evaluator foundation wrapping `github.com/google/cel-go`. Public API: `Evaluator` interface (`Compile(src, expectType) → Program`), `Program.Eval(StandardActivation, EvalOptions)`, and `StandardActivation` binding shape (`response`, `previous`, `vars`, `env`). Structured sentinel errors `ErrCelParse` and `ErrCelType` (both reachable via `errors.Is`/`errors.As` through `*CelError`) with 200-rune source truncation and ellipsis marker. Time-of-day functions `now()` and zero-argument `timestamp()` rejected at compile time via AST walk; single-argument `timestamp(string)` remains available. `SensitiveObserver` hook invoked exactly once per referenced sensitive `vars.<name>` per `Eval` call. Both sentinels registered in `internal/errors`. (M19-002)
- E2E: `scripts/m16-e2e.sh` happy-path convergence scenario and `web/tests/e2e/m16-happy-path.spec.ts` Playwright spec covering the full M16 workflow — registration, email verification, 14-day trial JWT, schedule creation, worker claim and execution, result ingestion with `result_id`, `/results/stats` trend assertions, trial-expiry notification via `TrialExpiryNotifier` tick, on-demand trial activation, and password-reset with full refresh-token revocation. Two new internal Dev/Testing-only endpoints: `POST /internal/test/seed-near-expiry-trial` and `POST /internal/test/trial-expiry-tick`. `EmailQueueProcessor` registered in Development+fake mode for e2e audit log assertions. (M16-021)
- Web: `/org/[slug]/dashboard` SvelteKit page with five overview cards (Total Runs, Pass Rate %, Avg Duration, p50/p95 Duration), an inline-SVG daily pass-rate trend line chart, a top-10 frequently-failing-endpoints list (method, path_template, failure_count, last_seen relative time), and a recent-runs table. `?window=7d|30d|90d` selector round-trips through the URL. Free-tier orgs see an in-place upgrade prompt; `?window=<other>` renders the 400 error state without crashing. Dashboard nav link added to the org sidebar for team-tier members. (M16-020)
- CLI: `backend.Client.GetTeamVault(ctx, orgId)` transport for `GET /api/v1/organizations/{orgId}/vault-config`. Team vault cache at `~/.config/curlew/team_vault.json` with 5-minute TTL, stale-while-revalidate foreground fetch (≤2s), flock serialization against thundering-herd, and `--refresh-vault` flag on `curlew run`/`curlew worker` to bypass TTL. `internal/vault/teamtemplate.Load` overlay loader uses backend cache as base and `CURLEW_TEAM_CONFIG` as per-key overlay. Load-site gate enforces `shared_vault_templates` feature — consults trial claims before tier, exits with code 6 when below Team tier and no active trial. `curlew license --refresh` also refreshes the team vault cache in the same round-trip (non-fatal on failure). (M16-018)
- Backend: `GET /api/v1/organizations/{orgId}/results/stats` and `GET /api/v1/organizations/{orgId}/results/failures` aggregation endpoints (M16-019). On-demand SQL over the `(org_id, created_at)` index — no rollup table. `?window=7d|30d|90d` (default `30d`); any other value returns 400 `unsupported-window` (Open Decision 10). Stats response includes `window`, `window_start`, `window_end`, `totals` (runs, pass_count, fail_count, skipped_count, pass_rate, avg_duration_ms, p50_duration_ms, p95_duration_ms), and a sparse `trend[]` array of daily entries. Failures grouped by `(method, path_template)` sorted by `failure_count DESC`; default limit 10, max 50 with `Warning: 299` header and `limit_clamped` body field on clamp. `PathTemplateExtractor` collapses id-shaped segments (all-digits, RFC 4122 UUID, 32-char hex, length-≥8 hex) to `{id}` and is wired into the result-ingest pipeline. New nullable `method` (varchar 10), `request_url` (varchar 1000), `path_template` (varchar 500) columns on `result_items` (EF Core migration `AddResultItemMethodAndPathTemplate`). `UploadResultItemRequest` gains optional `Method` and `RequestUrl` fields. Both endpoints tier-gated by `DashboardTierGate.EnsureTeamOrAboveAsync` (402 on Free) and `dashboard.view` RBAC (403). (M16-019)
- Backend: `team_vaults` EF Core migration + `TeamVault` entity (org_id PK, template_yaml TEXT, template_jsonb JSONB, version BIGINT, created_by/at, updated_by/at). `VaultManifestValidator` literal-secret heuristic (`^[A-Za-z0-9+/=._-]{16,}$` under `password/secret/token/key` keys, exempting ARN/URI/path coordinates). `VaultConfigService` with `GetAsync`/`UpsertAsync`/`DeleteAsync`, version increment on upsert, `vault_config.upserted`/`vault_config.deleted` audit events, and reject/warn validator-mode toggle (`CURLEW__VAULTCONFIG__VALIDATORMODE`; default `reject`, `warn` in dev via `appsettings.Development.json`). `GET/PUT/DELETE /api/v1/organizations/{orgId}/vault-config` endpoints gated by `VaultConfigTierGate` (Team tier, 402) and `vault_config.{view,manage}` RBAC (403). ETag header on GET/PUT; 304 on conditional GET with `If-None-Match`. 422 problem-detail `vault-template-suspicious-value` with `offending_paths` extension in reject mode. YamlDotNet 17.x added for YAML→JSON conversion. `docs/api-errors.md` updated with `VAULT_CONFIG_SUSPICIOUS_VALUE`. (M16-017)
- Web: `/org/[slug]/vault-config` SvelteKit page — monospace textarea YAML editor, Save button (success toast `Updated to version N`, suspicious-value inline panel on 422), Delete with confirm, "Generate CLI snippet" button showing `curlew license --refresh`, audit log subsection (last 10 `vault_config.*` events). Page gated by `requireTeamTier`. Typed API client `vaultConfigApi` with `get` (404→null), `put` (sends raw YAML as `application/yaml`), and `remove`. (M16-017)
- Backend: `GET /api/v1/integrations/gitlab`, `POST /api/v1/integrations/gitlab`, and `DELETE /api/v1/integrations/gitlab/{id}` endpoints — replaces M16-013 stub. `POST` resolves `project_path → numeric project_id` via `IGitLabProjectLookup` (GitLab `GET /api/v4/projects/{encoded-path}`), encrypts the PAT via `IGitLabKeyProvider`, persists the `gitlab_installations` row, and returns the new installation (PAT never echoed). `DELETE` soft-deletes by setting `deleted_at`. `GET` returns only non-deleted installations with LEFT-JOIN `last_status_post_at` from `pr_checks`. All three endpoints are gated by `VaultConfigTierGate` (Team tier). `IGitLabProjectLookup` maps 401 → `pat_unauthorized`, 404 → `project_not_found`, `http://` → `insecure_base_url`, network errors → 502 `gitlab_unreachable`. (M16-016)
- Web: `/org/[slug]/integrations/gitlab` page — empty state with "Connect GitLab" button, connection-health table (project path, base URL, last status post relative time, token-revoked badge, Re-paste PAT action, Disconnect with confirm), `ConnectGitLabModal` with base URL, project-path, PAT (`type=password`), and collapsible CA bundle fields. Page is tier-gated; Free-tier orgs redirect with `team_tier_required` toast. Subnav "GitLab" link added for team-tier admins. (M16-016)
- Backend: `POST /webhooks/gitlab` inbound webhook endpoint with `X-Gitlab-Token` constant-time verification (per installation via `IGitLabKeyProvider`), `X-Gitlab-Event-UUID` idempotency on `gitlab_webhook_events`, 5-failure quarantine per event row and per source IP (in-memory tracker), `Pipeline Hook` dispatcher reconciling `pr_checks.status` by `(provider='gitlab', gitlab_installation_id, head_sha)`, `Push Hook` and `Merge Request Hook` stored for debugging only (no business logic in M16), 30-day daily retention cleanup host. **Single-secret-only in M16-015 (Open Decision 3):** multi-secret rotation via `GITLAB__WEBHOOK_SECRETS_<installation_id>` is a documented follow-up. (M16-015)
- Backend: `IGitLabCheckPoster` + `GitLabCheckPoster` posting commit statuses to GitLab's Commit Status API (`POST /api/v4/projects/{id}/statuses/{sha}`) with Private-Token PAT auth (decrypted per-request via `IGitLabKeyProvider`), lossy state mapping (timed_out→failed, neutral/skipped→success+marker), description truncation to 255 chars, per-installation Guid-keyed rate-limit tracker, HTTPS URL enforcement (overridable via `GITLAB__ALLOW_HTTP=true`), 401/PAT-revoked audit trail, and 429/rate-limit backoff. `pr_checks` table gains `provider TEXT NOT NULL DEFAULT 'github'` discriminator with CHECK constraint, nullable `gitlab_installation_id` UUID FK, and nullable `gitlab_status_id BIGINT` columns. `PrChecksUploadEndpoint` dispatches to the GitLab poster when `provider=gitlab`; missing/empty `provider` defaults to `github` for wire compat. (M16-014)
- Backend: `schedules.timezone TEXT NOT NULL DEFAULT 'UTC'` column (EF Core migration `AddScheduleTimezone`). `CreateScheduleRequest` accepts optional `timezone` (IANA identifier); validated via `TimeZoneInfo.FindSystemTimeZoneById` — invalid values return 422 problem-detail `invalid-timezone` with `valid_examples` extension. `SchedulesService.CreateAsync` and `EnqueueDueAsync` use Cronos's timezone-aware `GetNextOccurrence(now, tz, inclusive: false)` for DST-correct next-run computation. All user-facing schedule endpoints (`/api/v1/organizations/{orgId}/schedules`) are now gated by `ScheduleExecutorTierGate` (Team tier or above) — Free-tier orgs receive 402 problem-detail. (M16-012)
- Web: schedules dashboard at `/org/[slug]/schedules` — list table with name/cron/timezone/next-run/last-run/status, "New schedule" modal with cron live-preview via `cron-parser`, per-row "Run now" button, and a child route `/org/[slug]/schedules/[scheduleName]` for run history with status/duration/result links. Subnav "Schedules" link added for team-tier admins. (M16-012)

- CLI: `curlew worker --schedule-pull` mode (Team tier) polls `GET /api/v1/schedules/next-run`, resolves `file:` collection refs locally, executes them via the existing `runner.Run` engine, sends heartbeats every 30 s, and posts results to `POST /api/v1/schedules/runs/{id}/result`. `git:` refs are explicitly rejected with a clear message (deferred to a future release per Open Decision 4). Transient result-post failures use exponential backoff (up to 1 h); exhausted retries queue the payload under `~/.config/curlew/pending-uploads/<run_id>.json` and drain on the next successful poll cycle. New flags: `--backend <url>`, `--poll-interval <duration>`, `--once` (run one cycle then exit). (M16-011)
- Backend: `ScheduledRunDto` now exposes `result_id` (wire: `res_<hex>`) once a worker posts its result; null/omitted while queued, running, or for legacy rows (M16-010). `SubmitResultAsync` is now idempotent: a retry with the same `claim_token` on an already-terminal row returns 200 without re-inserting the result row, while a different `claim_token` still returns 409 `AlreadyCompleted`. ON DELETE SET NULL FK from `scheduled_runs.result_id` to `results.id` already shipped in M16-009 migration.
- Backend: `GET /api/v1/schedules/next-run`, `POST /api/v1/schedules/runs/{run_id}/heartbeat`, and `POST /api/v1/schedules/runs/{run_id}/result` endpoints for the worker-pull schedule execution protocol. `ScheduleExecutorService` handles atomic claim (status Queued→Running with UUID `claim_token` and 30-second deadline), heartbeat refresh, and result ingestion via `ResultsService`. `ScheduleExecutorTierGate` blocks Free-tier orgs (402). `ShardReaper` extended to reap `scheduled_runs` rows with `last_heartbeat_at > 5 minutes` back to Queued. `SchedulerHost.EnqueueDueAsync` gains stack-up prevention (skips new row when an in-flight run exists). Migration `20260511070846_AddScheduleExecutorClaimColumns` adds `claim_token`, `claim_deadline`, and `last_heartbeat_at` columns to `scheduled_runs`. (M16-009)
- Backend: `TrialExpiryNotifier` daily `IHostedService` fires at 09:00 UTC, queries active `full_initial`/`ondemand` trial rows expiring within 3 days or 1 day, enqueues `trial_expiring` SendGrid emails via `IEmailQueue`, and atomically sets `notified_3day_at`/`notified_1day_at` to prevent duplicate emails across reruns. Enqueue failure leaves the timestamp unset so the next tick retries. Sentinel `RunAtUtc=now` forces immediate firing (dev/test mode). Registered in `Program.cs` behind `!IsEnvironment("Testing")` guard. (M16-008)
- Backend: `POST /api/v1/trials/{feature}` on-demand trial activation endpoint. Returns 200 with re-minted token trio on first activation; 409 (`TRIAL_ALREADY_CONSUMED`) when a trial row already exists; 404 (`TRIAL_FEATURE_UNKNOWN`) for unknown slugs. `TrialsService.ActivateOnDemandAsync` handles DB insertion with race-safe `DbUpdateException` catch. `TrialActivationResult` discriminated union returned from service layer. (M16-007)
- CLI: `curlew license trial start <feature>` subcommand. Reads refresh token and device ID from cache, calls `POST /api/v1/trials/{feature}`, persists the re-minted token trio, and prints `Trial activated: <feature> (expires <RFC3339>)`. Exit code 5 on `TRIAL_ALREADY_CONSUMED`, 6 on unknown feature, 3 on network failure, 7 on missing device. (M16-007)
- CLI: `Claims.TrialState` (`string`, `omitempty`) and `Claims.TrialExpiry` (`int64`, `omitempty`) fields added to `internal/license/jwt.go`. `Claims.IsTrialActiveFor(feature)` accessor returns true iff `TrialState == "active"` and the feature appears in `Features[]`. (M16-007)
- CLI: `auth.CheckFeatureWithClaims(registry, feature, tier, claims)` — trial-aware gate that short-circuits to allowed if `claims.IsTrialActiveFor(feature)`. `auth.TrialChecker` interface decouples `internal/auth` from `internal/license`. `*license.Claims` satisfies `TrialChecker` via `IsTrialActiveFor`. (M16-007)
- Backend: License JWT now carries populated `trial_state` and `trial_expiry` claims, computed by `ITrialStateResolver` / `DatabaseTrialStateResolver` from the `trials` table. `TrialSeederService` inserts a 14-day full-initial trial row per gated feature (`TrialFeatures.All`) when a new user is created (registration via `CurrentUserAccessor`, admin bootstrap, dev-only seed endpoint). (M16-006)

### Changed
- Backend: `LicenseTokenIssuer` constructor now requires `ITrialStateResolver`. Trialing features are unioned into the JWT `features[]` claim. Callers that previously received hardcoded `trial_state = "none"` / `trial_expiry = null` now receive values from the resolver. (M16-006)
- Backend: Stripe `customer.subscription.created` webhook now preempts the org owner's active trial rows, transitioning them to `kind = preempted_by_subscription` with `expires_at = now()`. `StripeSubscriptionHandler.HandleCreatedOrUpdatedAsync` gains a `bool isCreated` parameter; only `created` events trigger preemption. (M16-006)

### Added
- Backend: `trials` table EF Core migration and `Trial` entity with `TrialKind` enum (`FullInitial`, `OnDemand`, `PreemptedBySubscription`). UNIQUE (user_id, feature) enforces one trial per feature per user. CHECK constraint on `kind` column. Partial indexes on `(expires_at)` filtered by `notified_3day_at IS NULL` and `notified_1day_at IS NULL` for the daily-cron path (M16-008). `TrialKindConverter` maps C# enum to spec-mandated snake_case strings. Migration `20260510132915_AddTrials` applies and rolls back cleanly. (M16-005)

### Changed
- CLI: `from_command` shell-out variables retiered from Solo to Free — available at every tier including Free with no feature gate. Registry entry, MANUAL.md (TOC, precedence table, mental-model, section heading, section body, tier-matrix row removed), and all tests updated. (M15-003)

### Added
- Web: SvelteKit pages for password reset and email verification — `/auth/password-reset/{request,confirm}` and `/auth/email-verification/{request,confirm}` with `+page.server.ts` form actions, `authApi` typed client for the four M16-003 endpoints, RFC 7807 `problemType` extraction on `ApiError`, zxcvbn-based weak-password inline error, enumeration-defense generic confirmations, billing-page 403 `email_not_verified` interceptor with `EmailVerifiedRequiredModal` resend flow, Playwright E2E specs (5 password-reset + 4 email-verification + 1 billing modal), and 18 page-server unit tests. (M16-004)
- Backend: password-reset and email-verification endpoints + `RequireVerifiedEmail` filter + `AuthTokenCleanupService` (M16-003). Four new endpoints: `POST /api/v1/auth/password-reset/request` (silent 200, per-email 3/24h limit enforced in service), `POST /api/v1/auth/password-reset/confirm` (zxcvbn score ≥ 3, SHA-256 token hash, transactional password update + refresh-family revocation via `RevokeAllFamiliesForUserAsync`), `POST /api/v1/auth/email-verification/resend` (rotate-on-resend, per-email 5/24h limit), `POST /api/v1/auth/email-verification/confirm` (marks `users.email_verified = true`). Per-IP ASP.NET RateLimiter policies (10/24h) added for request and resend. `RequireVerifiedEmailFilter` (`IEndpointFilter`) gates `POST /api/v1/subscriptions/checkout` and `POST /api/v1/invitations/accept` — returns 403 with `email-not-verified` problem type if caller's `users.email_verified` is false. `AuthTokenCleanupService` (`BackgroundService`) prunes consumed/revoked/expired rows in `password_reset_tokens` and `email_verification_tokens` on a 1-hour tick with 7-day retention. `ZxcvbnPasswordStrengthChecker` wraps `zxcvbn-core 7.0.92` behind `IPasswordStrengthChecker`. `AuthTokenIssuer` static helper mints `prst_*` and `evtk_*` prefixed tokens (32-byte random, base64url, SHA-256 hash for storage). `EmailNotVerifiedProblem` RFC 7807 builder (403/401). `docs/api-errors.md` updated with four new error codes: `PASSWORD_RESET_TOKEN_INVALID`, `PASSWORD_TOO_WEAK`, `EMAIL_VERIFICATION_TOKEN_INVALID`, `EMAIL_NOT_VERIFIED`. 39 new tests across 7 test files.
- Backend: `gitlab_installations` and `gitlab_webhook_events` tables, `IGitLabKeyProvider` abstraction with `FileGitLabKeyProvider` (AES-256-GCM per-row DEK, 32-byte file KEK) and `GoogleKmsGitLabKeyProvider` (KMS-wrapped DEK via `IKmsClient`), `GitLabEnvelopeCodec` wire format, DI registration via `GITLAB__KEY_PROVIDER` config switch, and stub `GET /api/v1/integrations/gitlab` returning an empty list. Migration `20260510064317_AddGitlabInstallationsAndEvents` applies and rolls back cleanly. `IKmsClient` extended with symmetric `EncryptAsync`/`DecryptAsync` for DEK wrapping. (M16-013)
- Backend: password-reset and email-verification token tables + M16 SendGrid templates (M16-002). New EF Core entities `PasswordResetToken` (`password_reset_tokens` table — id/user_id/token_hash/issued_at/expires_at/consumed_at/revoked_at/requester_ip/requester_ua) and `EmailVerificationToken` (`email_verification_tokens` table — same shape minus requester_ip/ua). New `users.email_verified BOOLEAN NOT NULL DEFAULT false` column. Single migration `AddPasswordResetAndEmailVerificationTokens`. Three indexes per table (unique on token_hash, user+issued_at lookup, partial index on expires_at where consumed_at IS NULL AND revoked_at IS NULL). `EmailTemplateInventory.Slugs` expands from six to eight: adds `password_reset` (variables: user_email, reset_url, expires_at_local, requester_ip, requester_ua) and `trial_expiring` (variables: user_email, feature, expires_at_local, upgrade_url) — both deferred from M14 per spec :9565. `templates/email/{password_reset,trial_expiring}.{mjml,json}` files added; both round-trip through the existing `EmailTemplateLoader` + `MjmlNetCompiler` + `SendGridSmtpSender` allowlist. No endpoints or services in this slice — the password-reset endpoints land in M16-003 and the trial cron in M16-008.
- Backend: `ITierGate` generic abstraction with canonical `TierGate` implementation, `TierGateProblemFactory` (RFC 7807, 402/404), three per-feature adapters (`VaultConfigTierGate`, `ScheduleExecutorTierGate`, `DashboardTierGate`), `SsoTierGate` refactored to delegate through the canonical seam, DI registration, Testing-only probe endpoints, and machine-enforced locality invariant via `TierCheckLocalityTests`. 40 new tests. (M16-001)
- Backend: SSO Enterprise tier gate at `SsoService` and `OidcService` — non-Enterprise orgs (Free, Professional, Team, or no subscription) receive HTTP 402 Payment Required on authenticated config endpoints (`GET/PUT /api/v1/organizations/{id}/sso{,/saml,/oidc}`) and HTTP 404 Not Found with `Cache-Control: no-store` on public flow endpoints (`/api/v1/sso/saml/{orgId}/login`, `…/acs`, `/api/v1/sso/oidc/{orgId}/login`, `…/callback`). Gate logic centralised in `SsoTierGate.EnsureEnterpriseAsync`; `TierIneligible` added to `SsoError`; dev seed updated to provision Enterprise subscription on the SSO fixture org. (M15-002)
- CLI: `plugin_loading` Enterprise gate enforced at both `CURLEW_PLUGINS` load sites (`buildHookDispatcher` and `pluginsListCmdOut`). Free, Solo, Professional, and Team tiers receive exit 6 with the registered Description and Workaround when `CURLEW_PLUGINS` is set; unset env prints an empty table at every tier unchanged. `docs/MANUAL.md` plugin tier-gate paragraph sharpened with explicit exit code and enforcement note. (M15-001)
- E2E: M14 revenue loop convergence slice (M14-021). `testdata/m14/e2e-collection.yaml` single-request collection used by the Playwright spec. 5-assertion `web/tests/e2e/m14-revenue-loop.spec.ts` proves the full happy path: CLI exit-0 + `check-run posted; status=success`, `/org/acme/runs` 307-redirect alias to `/results`, `/org/acme/integrations/github` new web view surfacing `posted_at` and `check_run_id` from the M14-018 poster, Stripe `invoice.payment_succeeded` replay enqueuing `billing_receipt` email (confirmed via `GET /internal/test/email-audit`), and github-mock call-log showing `POST /repos/acme/api/check-runs`. Backend: `IRecentlySentEmailLog` + `InMemoryRecentlySentEmailLog` bounded ring-buffer (200-entry default); `EmailQueueProcessor` records each successful send; `GET /internal/test/email-audit` endpoint (Dev+Testing only, mirrors `seed-refresh` precedent). Backend: `POST /internal/test/seed-m14` (Dev+Testing only) idempotently upgrades org to team tier, sets `stripe_customer_id`, and upserts a `github_installations` row; `scripts/seed-test-data.sh` extended with the `SEED_M14=1` block. `PrCheckDto` gains additive `PostedAt`/`CheckRunId` fields projected from the entity. CLI: `handleReportUpload` now prints `Uploaded result <id>; check-run posted; status=<state>` when both `--pr` and `--repo` are set. `scripts/ci-local.sh` gains `--down` mode (idempotent stack teardown; smoke-tested by `TestCiLocalDownIdempotent`). `.github/workflows/m14-e2e.yml` path-filtered CI workflow. `docs/MANUAL.md` gains §6.9 login + report-upload walkthrough.
- Web: `/integrations` route with GitHub install entry-point + install-state view (M14-020). New `+page.server.ts` + `+page.svelte` at `web/src/routes/integrations/`. Admin/owner sees a "Connect GitHub" button linking through `/api/v1/integrations/github/install-url`; member sees a read-only card with admin-only tooltip. Five install-state branches: not-installed (admin: Connect button, member: read-only), connected (status line with account slug + repo count), suspended (yellow badge + description, no Connect button), pending-claim (admin: "Claim this install" button), callback success (`?installed=true` one-time toast). Help text explains both dashboard-initiated and webhook-first install paths. 7 Playwright tests cover all 7 task behaviours. Bounded per Open Decision #10 — no billing, SSO, or account-settings UI.
- Backend: `POST /webhooks/github` + `github_webhook_events` idempotency (M14-019). New `github_webhook_events` table (EF migration `20260506121216_AddGithubWebhookEvents`) with `delivery_id UUID PRIMARY KEY`, status/attempt_count/quarantine columns mirroring the Stripe pattern. `GithubWebhookEndpoint` reads raw body before JSON parse (spec :8570), rejects legacy SHA-1-only headers (spec :8575), verifies `X-Hub-Signature-256` via `CryptographicOperations.FixedTimeEquals` (constant-time, spec :8572), supports multi-secret rotation via `CURLEW__GITHUB__WEBHOOK__SECRETS` (comma-separated, spec :8577-8584), and routes through `IGithubWebhookDispatcher`. Dispatcher handles: `installation.{created,deleted,suspend,unsuspend}`, `installation_repositories.{added,removed}`, `check_run.rerequested` (structured-log stub, `TODO(re-run-pipeline)`); unknown events accepted with 200 to stop retry storms. `GithubInstallationsService` gains `MarkSuspendedAsync`/`MarkUnsuspendedAsync`/`MarkDeletedAsync` lifecycle methods — each evicts the `IInstallationTokenCache` token; `MarkDeletedAsync` also marks open `pr_checks` state `INSTALLATION_DELETED`. `GithubWebhookStore` provides idempotent insert (ON CONFLICT DO NOTHING on Postgres / check-then-insert on SQLite), 5-failure quarantine, and 90-day processed-row cleanup. `GithubWebhookCleanupHost` runs as a daily BackgroundService (skipped in Testing). `scripts/sign-github-webhook.sh` helper. 52+ dedicated tests.
- Backend: outbound GitHub Checks API POST + pr_checks expansion + 6-state mapping (M14-018). New `POST /api/v1/pr-checks` endpoint (v4.2.1 shape): accepts `{repo, pr, state, head_sha, output}`, runs a token-leak scan (`ghs_…` regex via `TokenLeakDetector`), inserts a `pr_checks` row, and synchronously calls `CheckRunPoster.PostAsync`. `CheckRunPoster` orchestrates two-stage auth (App JWT → installation token via `InstallationTokenCache`), repo-coverage guard, suspension/deletion checks, 6-state CLI→GitHub conclusion mapping (`PrCheckConclusionMapper`), 60 000-char markdown truncation (`MarkdownSafety`), rate-limit tracking (`RateLimitTracker`), and queued-on-5xx semantics. `IInstallationTokenCache`/`InstallationTokenCache`: in-process per-installation token cache with single-flight refresh (`SemaphoreSlim`) and 5-minute early-expiry window. `IRateLimitTracker`/`RateLimitTracker`: token-bucket per installation observing `X-RateLimit-Remaining` and `Retry-After` headers. `PrChecksProblem`: RFC 7807 builder for eight `PRCHECK_*` error codes. EF Core migration `20260507130000_PrChecksV421`: additive `ALTER TABLE pr_checks` adding 13 new columns with SQL backfill. Real `GitHubInstallationsApi` replacing `StubGitHubInstallationsApi`. `github-mock` Go HTTP sidecar added to `docker-compose.test.yml` (port 18443). Named HttpClients `github-app` and `github-checks` with configurable `GitHub__ApiBase`. 36 unit tests covering all error codes and token-safety invariants.
- Backend: github_installations table + dashboard install-URL endpoint + webhook-first claim flows (M14-017). `github_installations` table with `installation_id BIGINT PRIMARY KEY`, `org_id UUID` nullable, `repo_set TEXT/JSONB`, soft-delete, suspended, and cross-tenant partial UNIQUE index `idx_github_installations_org` (`UNIQUE (org_id) WHERE deleted_at IS NULL AND org_id IS NOT NULL`). `GithubInstallationStateToken` — stateless HMAC-SHA256 signed state token (`Base64Url(JSON body).Base64Url(MAC)`) for the install-URL flow. `GET /api/v1/integrations/github/install-url` (admin/owner only, mints 15-min state token, returns GitHub App URL). `GET /api/v1/integrations/github/callback` (validates state, claims installation, 302 to `{WebAppUrl}/billing?installed=true`, 409 on duplicate). `POST /api/v1/integrations/github/claim` stub (501 until M14-018). `GithubInstallationsService` centralises webhook-first upsert (org_id=NULL), dashboard claim, repo-set set-union/set-difference, `PRCHECK_REPO_NOT_COVERED` marking. `GithubInstallationCreatedHandler` / `GithubInstallationRepositoriesHandler` — DI-registered webhook handlers (M14-019 wires them into `/webhooks/github`). `GithubInstallationReconcilerHost` — daily 24h BackgroundService reconciling `repo_set` against GitHub API. `IGitHubInstallationsApi` seam with `StubGitHubInstallationsApi` placeholder (full HTTP client in M14-018). EF Core migration `20260506093300_AddGithubInstallations`. 33 dedicated tests.
- Backend: IGitHubAppKeyProvider + RS256 App-JWT signing (M14-016). `IGitHubAppKeyProvider` interface with RS256-only signing seam (type-system-separated from ES256 `IKeyProvider`). `FileGitHubAppKeyProvider` (self-hosted, PEM at `Keys/github-app/<slug>.pem` mode 0600) and `GoogleKmsGitHubAppKeyProvider` (SaaS, RSA_SIGN_PKCS1_2048_SHA256 HSM tier). `AppJwtBuilder` mints compact JWTs with header `alg=RS256,typ=JWT` and claims `{iss=app_id, iat=now-60s, exp=iat+540s}`. `BearerTokenRedactor` structured-log filter redacts `Authorization: Bearer eyJ...` and `ghs_*` values to `<redacted>` before emission. `GET /internal/github-app/jwt-self-test` diagnostic endpoint returns all six JWT fields including `verified=true`. GHES non-support and `api.github.com` hardcode documented in provider headers and `deploy/self-hosted/README.md`. 19 tests.
- Backend: Stripe invoice + payment-method event handlers (M14-013). `StripeInvoiceHandler` handles `invoice.{payment_succeeded,payment_failed,finalized}` by re-fetching from Stripe (order-defense), upserting the local `invoices` table, updating `subscriptions.status` (Active on success, PastDue on failure), and enqueuing `billing_receipt` / `billing_payment_failed` emails. `StripePaymentMethodHandler` handles `payment_method.{attached,detached}` by upserting / soft-deleting `payment_methods` rows. New `invoices` and `payment_methods` tables (migration `20260507120000_AddInvoicesAndPaymentMethods`). `IStripeGateway` extended with `GetInvoiceAsync` + `GetPaymentMethodAsync`. Manifest-contract CI gate proves every variable name in the handler's `EmailMessage` payload matches the on-disk JSON manifest. `scripts/replay-stripe-event.sh` extended with `invoice.*` and `payment_method.*` case branches. 27 dedicated tests.
- Backend: Stripe subscription + customer event handlers (M14-012). `StripeSubscriptionHandler` handles `customer.subscription.{created,updated,deleted}` by re-fetching the current Stripe state (never trusting the event payload snapshot), upserts the subscriptions row with status/tier/period/seat-count, quarantines rows when Stripe returns 404, clears `stripe_subscription_id` on cancellation, and enqueues a `billing_subscription_canceled` email onto the `IEmailQueue` channel. `StripeCustomerUpdatedHandler` mirrors `customer.updated` email/metadata changes into `orgs.stripe_customer_email`. `StripeWebhookDispatcher` routes events to the appropriate handlers. New `Quarantined` subscription status with migration `AddSubscriptionStripeCustomerEmail`. `GetForOrgAsync` returns free tier for `Canceled` and `Quarantined` subscriptions. `scripts/replay-stripe-event.sh` helper for manual event replay. 38 dedicated tests; backend coverage 93.6%.
- Backend: 6-template inventory (MJML + manifests) + CI upload job (M14-015). Ships the six M14 transactional email templates: `email_verification`, `auth_device_code`, `billing_receipt`, `billing_payment_failed`, `billing_subscription_canceled`, `account_security_alert`. Each template is a `templates/email/<slug>.mjml` + `templates/email/<slug>.json` manifest pair. `EmailTemplateInventory` static class pins the canonical six-slug list. Parameterized `EmailTemplateInventoryManifestTests` (52 tests) validates manifest round-trips, MJML compile, variable-allowlist, placeholder-copy header, and spec-coverage for all slugs. `RunUploadTemplatesAsync` iterates `EmailTemplateInventory.Slugs` (canonical source of truth). CI workflow `.github/workflows/email-templates.yml` compiles and uploads templates to a fake SendGrid server (`scripts/fake-sendgrid.py`) and asserts six per-slug template IDs. `templates/email/README.md` documents the placeholder-copy convention, variable-allowlist rule, forbidden slugs, and spec citation.
- Backend: SendGridSmtpSender + EmailQueueProcessor + MJML compile pipeline (M14-014). Live `ISmtpSender` backed by SendGrid 9.x Dynamic Templates API. `EmailQueueProcessor` `BackgroundService` consuming `System.Threading.Channels<EmailMessage>` with exponential-backoff retry (±20% jitter), 3-attempt dead-letter path. `IMjmlCompiler`/`MjmlNetCompiler` (Mjml.Net 4.x) with Handlebars `test_data` substitution. Variable allowlist enforced at `SendTemplateAsync` before any HTTP call. `dev email-preview <slug>` CLI subcommand renders MJML to HTML on stdout without calling SendGrid. `upload-templates` CI job posts compiled HTML to SendGrid Dynamic Templates API. `IEmailDeadLetterStore`/`LoggingEmailDeadLetterStore` seam. `OperationCanceledException` correctly handled — clean shutdown does not produce false dead-letters. `email_verification` fixture template (MJML + JSON manifest). `deploy/self-hosted/README.md` documents `SENDGRID__APIKEY` + `SENDGRID__TEMPLATES__<SLUG>` env vars.
- Backend: Stripe webhook ingest pipeline — `POST /webhooks/stripe` (M14-011). HMAC-SHA256 signature verification via `Stripe.Net.EventUtility.ConstructEvent` with multi-secret rotation, idempotency via `INSERT … ON CONFLICT (event_id) DO NOTHING`, 5-failure quarantine budget, daily 90-day cleanup hosted service, and `StripeWebhookOptions` boot validation (tolerance footgun guard). New `stripe_webhook_events` table (EF migration 0015). `scripts/sign-stripe-webhook.sh` helper for manual testing.
- Backend: proration preview — `POST /api/v1/subscriptions/preview-proration` (M14-010). New `IStripeGateway.ComputeProrationAsync` method using Stripe's upcoming-invoice preview (`GET /v1/invoices/upcoming`) for server-side day-prorated math. `FakeStripeGateway` retains its full-month-delta approximation for unit tests (intentional divergence documented via XML remarks). No-op short-circuit when (tier, interval, seats) match the active subscription. `SubscriptionError.NoActiveSubscription` (409) when no subscription is on file. `invoice_upcoming_none` → 200 with `amount_due_now=0` (not an error). `StripePriceParser` extracted as a shared public helper for price-id → (tier, interval) mapping. New `deploy/self-hosted/README.md` note that proration is server-computed in live mode.
- Backend: Stripe billing portal session — `POST /api/v1/subscriptions/billing-portal` (M14-009). Live `IStripeGateway.CreatePortalSessionAsync` implementation against `Stripe.BillingPortal.SessionService`. Empty body defaults `return_url` to `App:WebAppUrl + "/billing"`. New `CURLEW__APP__WEBAPPURL` config required at boot (boot-time fail-fast via `ValidateOnStart`). New `STRIPE_UNAVAILABLE` 502 RFC 7807 mapping with preserved Stripe `request_id`. Admin or owner role required (relaxed from owner-only on the legacy `/portal` endpoint). `SubscriptionError.NoBillingSetup` (409) returned when org has no Stripe customer on file.
- Backend: live Stripe SDK wiring (Stripe.Net 43.x) + `CreateCheckoutAsync` via `price_id` (M14-008). Replaces the `StripeGateway` stub with a real implementation: creates a Stripe Customer on first checkout, then a Checkout Session with a per-call `IdempotencyKey`. `FakeStripeGateway` retained for unit tests. New `StripePriceAllowlist` validates `price_id` values. `stripe-mock` service added to `docker-compose.test.yml`; `scripts/ci-local.sh` starts it before `dotnet test` when backend files change. `CURLEW__STRIPE__MODE=live` now requires `CURLEW__STRIPE__APIKEY` at startup (validated via `ValidateOnStart`).

### Changed
- Docs: MANUAL.md gains coverage for three M7–M11 surfaces that were previously documented only in SPECIFICATION.md or in standalone schema docs. New §4.1a "Markdown response files" covers the per-request `<slug>.md` + `run.md` index layout, the `BEGIN/END curlew:response` sentinel block with `id`/`slug`/`run` attributes, the ten-section CLI-owned region, the six splice cases (`.md.new` siblings on mismatch), the content-type matrix (JSON/YAML/XML/HTML/text/binary/empty/HEAD), the 1 MiB body cap (post-redaction), the `--allow-sensitive` redaction invariant, the data-driven `<slug>/iter-N.md` + `index.md` subdirectory, and parallel runs grouping under `## Wave <N>` (M9-001..M9-005). §4.2 "Verbosity and color" is renamed to "Streams, verbosity, and color" and gains a stream-discipline subsection covering stdout/stderr segregation, the `--color={auto|always|never}` enum form, and the rule that colour is never emitted on stdout when `--format` is non-terminal regardless of TTY (M7-001..M7-005). New §4.5a "Event stream (`--events`)" summarises the six event kinds, the `run_id`/`request_id`/`request_slug` correlation triple, and cross-references the formal schema doc at `docs/EVENTS_SCHEMA_v1.2.md`. §5.7 watch-mode worked example now cross-references §4.1a. TOC updated.
- Docs: SPECIFICATION.md bumped to v4.1 (M7–M11 closeout). Stream discipline (stdout/stderr segregation + `--color={auto|always|never}` + `NO_COLOR`), JUnit free-tier note, and the `/schemas/{collection,project}-v1.json` editor-integration surface are now documented in the spec body. The two remaining inline references to `IMPROVEMENT.md` in the spec (§3.2 agent flow, §8.7 determinism tiers) are resolved — the §8.7 three-tier policy is fully inlined; the §3.2 reference points at the archived design document. `IMPROVEMENT.md` moves from the repo root to `docs/history/IMPROVEMENT.md` with an ARCHIVED banner; `docs/MANUAL.md` §4.9 cross-reference updated to the new path. Historical task records under `management/` and historical changelog entries continue to refer to `IMPROVEMENT.md` by its original name; those references resolve at the archived path.
- Changed: `--format junit` is no longer Professional-tier gated. Free-tier users can emit JUnit XML for CI integration without an upgrade. The `junit_xml` feature definition is removed from `internal/auth/registry.go`; the two `auth.CheckFeature(reg, "junit_xml", ...)` call sites in `cmd/curlew/main.go` are removed. Existing JUnit XML output (success path, failure path, parse-error path) is unchanged. The agent-harness `feature-gate-denied` fixture migrates from `--format junit` to `--parallel 2` to keep exercising the `FEATURE_GATE_DENIED` event-classification contract. Closes IMPROVEMENT.md §2.4 bullet 3. (M11-001)
- Changed: events schema v1.2 (additive from v1.1) — per-request events (`request.start`, `request.end`) gain an optional `request_slug` field; `schema_version` advances to `"1.2"` in all emitted events. Backward-compatible. (M9-001, batched into the W4 shipment with M9-002..M9-005.)
- CLI: `--only "<name>"` now prunes the setup phase to the transitive closure of `{{variable}}` references starting from the selected main requests, for a real inner-loop speedup. Setup items that have no `extract:` block (pure seeders) are always included. If the minimal-setup analyser rejects the combined DAG (cycles, duplicate producers, etc.), the runner falls back to running the full setup and emits a one-line diagnostic on stderr. Teardown is unaffected. (M8-005)
- `runCmdInner` now takes explicit `stdout, stderr io.Writer` parameters (M7-005). The previous process-global `os.Stdout`-swap hijack in `captureJSONCollection` has been removed; glob-discovered collections and future parallel runs can now execute in separate goroutines with independent output buffers. `watch.Config.RunFunc` signature changed to `func([]string, io.Writer, io.Writer) watch.RunResult`. All subcommands (`run`, `exec`, `validate`, `init`, `info`, `schema`, `watch`, `vault`, `import`, `pr-check`, `perf`, `plugins`, `worker`, `license`) now have writer-injectable `*CmdOut` variants. An AST-based anti-regression gate (`TestNoOsStdoutAssignment`) scans every `.go` file in `cmd/curlew/` and fails the test suite if any code reassigns `os.Stdout` or `os.Stderr`.

### Fixed
- `--parallel` and `--show-dependencies` now honor explicit `depends_on:` declarations
  when computing execution waves. Previously `parallel.Analyze` built dependency edges
  only from variable produce/consume relationships, so two requests linked only by
  `depends_on` (no variable flow) ran concurrently in the same wave — contradicting
  MANUAL.md §5.6. Explicit edges now participate in cycle detection, variable-collision
  suppression, wave grouping, parallel skip-on-failed-dependency propagation, and
  `--only` minimal-setup pruning (`AncestorClosure`). `depends_on` names referencing
  another phase (setup/teardown) or items filtered out by `--only` are skipped, as phase
  ordering already sequences them; self-references are ignored to match the sequential
  path. DOT output (`--show-dependencies` without `--dry-run`) labels variable-less
  explicit edges `depends_on`. New `Edge.Explicit` field in `internal/parallel`.
- `curlew <unknown-command>`, `curlew plugins <unknown-subcommand>`, `curlew perf <malformed-flag>`, `curlew worker <malformed-flag>`, and `curlew import <unknown-format>` now emit a one-line `Usage: curlew ...` synopsis to stderr instead of dumping the full help block to stdout; stdout is empty on every error path. Explicit `curlew --help` / `curlew <subcommand> --help` continue to write the full help text to stdout and exit 0. New helper `usageSynopsis(cmd string) string` in `cmd/curlew/main.go` keeps the short synopsis string in one place, and a synchronization test (`TestUsageSynopsis_MatchesPrintHelpFirstLine`) asserts each synopsis still appears verbatim in its corresponding `print*HelpTo` full-help output. Regression guard: `cmd/curlew/stream_help_test.go::TestStreamHelp` exercises every (subcommand × invocation-style) cell via `runWithWriters`. (M7-003)
- `curlew perf`, `curlew license [--validate|export]`, `curlew worker`, and `curlew exec --dry-run`: progress and confirmation text (`Load test:`, `Running...`, `Requests sent:`, `Wrote <path>`, `Validating license...`, `Exporting license bundle...`, `Included: ...`, `Claimed shard ...`, `Completed ...`, `No more shards; exiting`, `warning: submit failed ...`) now writes to stderr; stdout carries only the declared result payload (perf: `Results: requests=...`; license --validate: `Key source / Tier / State`; license export: empty; worker: empty). The stray `warning: submit failed` line in `internal/worker/run.go` that used to hit stdout has been moved to match its sibling heartbeat warning on stderr. `curlew exec --dry-run` renders the request method/URL/headers via `output.Printer.RequestDetail` instead of raw `fmt.Fprintln(os.Stdout, ...)`. Regression guard: `cmd/curlew/stream_progress_test.go::TestStreamProgress` spawns each subcommand via the built binary and asserts the stream split. New `worker.RunOptions.Stderr` field (default `os.Stderr`) adds a writer seam matching `Stdout`. (M7-002)
- `curlew run`/`watch`/`exec`: stderr printers now derive their color flag from `os.Stderr`'s own TTY state. Previously, when stdout was a TTY and stderr was a pipe, ANSI escape sequences leaked into piped stderr. A new `newStderrPrinter` helper centralizes the pattern across every call site in `cmd/curlew/`. (M7-001)
- fake-idp sidecar `/saml/sso` now accepts the SP's HTTP-Redirect binding (GET with DEFLATE+base64+url-encoded `SAMLRequest` in the query string); previously `MapPost` only accepted POST and returned 405 to every login attempt, leaving the Playwright SAML round-trip stranded at the browser navigation step (M5-021)
- `scripts/seed-enterprise.sh` now bumps the qa user to the `admin` built-in role (with the `qa-lead` custom-role overlay still applied) so the SSO-logged-in qa user can read the audit log; the `requireOrgAdmin` guard at `/org/[slug]/audit-log` was previously blocking the qa user (built-in `member`), which caused enterprise-full assertion 2 to fail with a 5s table-row timeout (M5-021)
- Audit events now persist an optional `actor_email` column (new EF migration `AddAuditLogActorEmail`); the audit-log API surfaces it as `user_email` and the web UI prefers it over the raw user GUID, so admins read meaningful actor names; SSO success/failure paths and `results.upload` populate the field at the call site (M5-021)
- Audit-log table at `/org/[slug]/audit-log` now renders a Status column showing `success` (green) or `failure (<reason>)` (red); previously the rendered row exposed neither the success flag nor the failure reason, making the failure-path assertion impossible to satisfy (M5-021)
- `MemberDto` now carries the optional `role_id` field, projected via a tuple lift in `MembersService.ListMembersAsync` (SQLite query translator rejects `Guid.ToString("N")` in the projection); the roles-settings page can now match `member.role_id` against `role.id` and render correct per-role member counts (M5-021)
- fake-idp sidecar ACS URL is now configurable via `FAKE_IDP_ACS_BASE` (default `http://backend:5000/api/v1/sso/saml`, empty/whitespace treated as unset, trailing slash stripped); `docker-compose.test.yml` overrides it to the host-reachable `http://localhost:5000/api/v1/sso/saml` so the Playwright browser on the CI runner can POST the signed SAMLResponse (previously the hardcoded docker-internal hostname caused `page.waitForURL(.../org/acme)` to time out on enterprise-full tests 1/2/7) (M5-021)
- `web/tests/e2e/helpers/auth.ts::seedAuthCookie` maps canonical seeded emails (`owner@example.com`, `qa@acme.example`) to their fixed seeded UUIDs so repeated calls produce a stable JWT `sub`; otherwise `test-token.sh`'s `uuidgen` fallback produced a new id each call and the `users.Email` UNIQUE index silently dropped the second upsert, leaving enterprise-full tests 4/5/6 authenticated as a user with no organization membership (M5-021)
- `CurrentUserAccessor.ResolveAsync` now resolves the JWT email claim under `ClaimTypes.Email` before falling back to `JwtRegisteredClaimNames.Email` (the JWT bearer middleware maps the raw `email` claim to `ClaimTypes.Email`); previously every upserted user row was saved with `Email = ""`, masked by the InMemory EF provider's lack of UNIQUE-index enforcement. Regression test: `AcceptInvitationPersistsEmailTests.Accepting_user_row_has_email_from_jwt_claim` (M5-021)

### Added
- CLI `curlew license --validate` JWKS fetch + offline cache (M14-007): algorithm-agnostic `VerifyJWT(token, jwks)` with ES256-only allowlist replaces the removed `VerifyRS256`; JWKS resolver chain (embedded → bundle → disk cache → online); `internal/backend.JWKSClient` fetches `/api/v1/.well-known/jwks.json` and persists `~/.config/curlew/jwks_cache.json` (mode 0600) with Cache-Control soft TTL; `--validate` stdout line gains `jwks=<embedded|bundle|cached|online>` suffix; help text documents the JWKS lookup order and 1h default TTL; `ErrInvalidAlgorithm`, `ErrInvalidType` both exit 6; `ErrOffline` (backend unreachable, kid not locally available) exits 6. 7 new `JWKSClient` unit tests, 8-case `TestVerifyJWT` table, `TestValidator_OnlineFetcherWiring`, and a smoke-test block proving offline-after-warm-up with the stub binary. Coverage: `internal/license` 88.3%, `internal/backend` 84.6%, `cmd/curlew` 81.5%.
- CLI `curlew license --refresh` + `curlew license --debug` (M14-006): full wiring of the two surfaces with an 8-code exit taxonomy per SPECIFICATION.md §8275–8284. `--refresh` calls `POST /api/v1/auth/refresh` via `internal/backend.RefreshTokens`, persists the rotated License JWT + access token + refresh token, and maps RFC 7807 `AUTH_*` problem codes to exit codes 0–7 (0 = success, 2 = no cache, 3 = network failure non-fatal, 4 = refresh expired, 5 = family revoked, 6 = server 5xx non-fatal, 7 = device not registered). Exit codes 3 and 6 are non-fatal — the cached License JWT remains valid for offline verification. `--debug` decodes the cached License JWT without re-verifying and prints a JSON object with `jti`, `kid`, `tier`, `exp`, `grace_until`, `refresh_failures`, `lastRefreshAttemptAt`, `cache_file`, and `last_error_type` (ProblemDetails type-URL for one-click docs navigation). New sentinels `ErrRefreshExpired`, `ErrRefreshReused`, `ErrDeviceMismatch` in `internal/backend/refresh.go` with multi-unwrap `refreshSentinelError` preserving the original `*ProblemDetails` for `errors.As`. `MANUAL.md` §9.3 gains the 8-code exit-taxonomy table. Smoke test exercises all six required exit-code paths (0, 3, 5, 6, 7, and no-cache) against the stub binary. 22 new tests; coverage: `cmd/curlew` 81.5%, `internal/backend` 84.0%, `internal/license` 88.1%.
- CLI `curlew login` subcommand (M14-005): RFC 8628 device-code flow against the curlew backend. Polls `POST /api/v1/auth/device/start` then `POST /api/v1/auth/device/poll`; prints one-time code and verification URI to stdout; optionally auto-opens the browser on TTY hosts (suppressed by `--no-browser`). Handles `authorization_pending` (retry), `slow_down` (+5s per RFC 8628 §3.5), `expired_token` (exit 4 + "Code expired" message), and `access_denied` (exit 4). On success, persists `license_jwt` via `internal/license.Store`, the API access and refresh tokens via `internal/backend.Storage` (hybrid keychain + AES-256-GCM encrypted files), and `device_id` via `internal/backend/device` (`~/.config/curlew/device.json`, mode 0600). New `testdata/m14/stub-backend.sh` wraps a Go-based stub server reusable by M14-006, M14-007, M14-021. 14 unit tests; coverage: `cmd/curlew` 81.7%, `internal/backend` 84.1%, `internal/backend/device` 84.2%.
- CLI `internal/backend` package (M14-004): Bearer-auth HTTP client, RFC 7807 `application/problem+json` decoding with typed `*ProblemDetails` (callers branch on `Code`, not `Status`), single-flight flock guard at `~/.config/curlew/refresh.lock` with 5-second timeout fallback to cache, hybrid OS-keychain + AES-256-GCM encrypted-file refresh-token storage (HKDF-SHA256 key derivation; file mode 0600; falls back to `~/.config/curlew/refresh_token.enc` when keychain is unavailable). Hidden `curlew internal backend-probe --self-test` subcommand (gated on `CURLEW_INTERNAL=1`) exercises the package end-to-end against an in-process httptest server and prints `OK: client built; lock OK; keychain available=<bool>; rfc7807 mapping OK`. 23 tests; 83.8% coverage.
- Backend JWKS endpoint (M14-003): `GET /api/v1/.well-known/jwks.json` — the public RFC 8615 well-known URI that exposes the active ES256 signing keys as a JWKS document. The endpoint is unauthenticated (`.AllowAnonymous()`), returns `Cache-Control: public, max-age=3600`, and serves only `current` + `verifying` keys (revoked keys excluded by `IKeyProvider.GetVerificationJwksAsync`). Response content type is `application/jwk-set+json` (RFC 7517 §8.5.1). The v4.1 bespoke `/api/v1/public-key` path is not registered. `deploy/self-hosted/README.md` documents that reverse proxies must allow this path through without authentication.
- Backend unified refresh endpoint (M14-002): `POST /api/v1/auth/refresh` mints all three tokens in one call — License JWT (30d, `typ=license+jwt`, 17-claim shape, `trial_state=none`), Access token (1h, `typ=at+jwt`, 9-claim shape), opaque refresh token (min(now+90d, familyRoot+365d)). RFC 9700 §4.14 rotation-on-use with family-revocation reuse detection enqueues an `account_security_alert` email via `IEmailQueue` / `System.Threading.Channels`. RFC 7807 problem-details responses for `AUTH_REFRESH_REUSED`, `AUTH_REFRESH_EXPIRED`, `AUTH_DEVICE_MISMATCH`, `AUTH_INVALID_REFRESH`. Stable error codes documented in `docs/api-errors.md`. All tokens are ES256-signed by the active `IKeyProvider` key. `scripts/test-token.sh` gains `refresh <email>` and `device <email>` modes; `POST /internal/test/seed-refresh` dev/test-only endpoint seeds refresh tokens for observable verification.
- Backend signing-key infrastructure (M14-001): `signing_keys` + `refresh_tokens` schema (EF migration `SigningKeysAndRefreshTokens`), `IKeyProvider` interface with `FileKeyProvider` (self-hosted, RFC 6979 deterministic ES256 via BouncyCastle) and `GoogleKmsKeyProvider` (SaaS, EC_SIGN_P256_SHA256 KMS), `/internal/keys/active` (GET) and `/internal/keys/rotate` (POST) endpoints, `ISigningKeyRotator` with normal and emergency rotation. Partial unique indexes enforce at most one `current` / `next` key at a time. `deploy/self-hosted/README.md` gains the FileKeyProvider key-generation procedure and emergency-rotation runbook.
- Dynamic functions: `$jwtDecodeHeader(token)` and `$jwtDecodeClaims(token)` — split a JWT on `.`, base64-url-decode (`RawURLEncoding`, no padding) the header (segment 0) or claims (segment 1), and return the decoded JSON as canonical compact JSON via `encoding/json` round-trip (keys sorted alphabetically). Signature segment (segment 2) is **never examined** — these helpers are decode-only and perform **no signature verification** (documented security caveat in MANUAL.md §3.7). Three structured `[INPUT]` error codes distinguish failure modes: `DYNFN_JWT_DECODE_BAD_FORMAT` (not exactly three `.`-separated segments), `DYNFN_JWT_DECODE_BAD_BASE64` (target segment is not valid URL-safe base64), `DYNFN_JWT_DECODE_BAD_JSON` (segment decodes but bytes are not valid JSON). Error messages include the offending input truncated to 32 chars. No `sensitiveArgIdx` registration — the token is a public bearer credential by convention; sensitive-variable propagation via M12-005 is unchanged. MANUAL.md §3.7 gains a `$jwtDecodeHeader / $jwtDecodeClaims` subsection with a table, two usage examples, error code reference, and the decode-only security caveat. (M17-005)
- Dynamic functions: 3 `$webhookSign.*` provider-signature functions — `$webhookSign.stripe`, `$webhookSign.github`, `$webhookSign.slack`. Each emits the VALUE of the provider-specific webhook-signature header shaped per the provider's published spec. `$webhookSign.stripe(body, secret[, timestamp])` returns `t=<ts>,v1=<hex>` where `<hex>` = HMAC-SHA-256(`<ts>.<body>`, secret). `$webhookSign.github(body, secret)` returns `sha256=<hex>` where `<hex>` = HMAC-SHA-256(body, secret) (x-hub-signature-256 only; SHA-1 form not produced). `$webhookSign.slack(body, secret[, timestamp])` returns `v0=<hex>` where `<hex>` = HMAC-SHA-256(`v0:<ts>:<body>`, secret). The two-arg forms of `stripe` and `slack` default the timestamp to the registry's clock seam (`time.Now().Unix()`); the three-arg form pins it. All three register arg index 1 (the secret) in `sensitiveArgIdx` so a sensitive-named source variable (`secret`, `token`, `password`, `api_key`, etc.) propagates into the run's `SensitiveSet` via the M12-005 Pass-1 plumbing — literal secrets are not auto-redacted (documented footgun in MANUAL.md §3.7). Arity errors return structured `DYNFN_ARITY` codes; `stripe`/`slack` accept 2 or 3 args, `github` exactly 2. `MANUAL.md` §3.7 gains a `$webhookSign.*` subsection with a table, per-provider examples, and the literal-secret footgun callout. (M17-004)
- OAuth 1.0a signer (`signing: {type: oauth1, ...}`): standard-library-only implementation (crypto/hmac, crypto/sha1, crypto/sha256, crypto/rand) registered at init time via `internal/signer/oauth1`. Required params: `consumer_key`, `consumer_secret`; optional: `token`, `token_secret`, `method` (default `HMAC-SHA1`; `HMAC-SHA256` also accepted; RSA-SHA1 and PLAINTEXT explicitly rejected per Open Decision #3), `realm`. Generates oauth_nonce (≥16 base32 chars from crypto/rand) and oauth_timestamp (time.Now().Unix()) at sign time; both can be overridden via spec params for test seams. Constructs the signature base string per RFC 5849 §3.4.1 (HTTP method uppercase, base URI without query, lex-sorted percent-encoded params double-encoded); signing key per RFC 5849 §3.4.2. Injects `Authorization: OAuth ...` with alphabetically-ordered, double-quoted, RFC 3986 percent-encoded key=value pairs per RFC 5849 §3.5.1. Sensitive-secret propagation: when `consumer_secret` or `token_secret` resolve from a sensitive variable, `SensitiveSet.AddValue(resolved)` is called so the value is redacted in serialised output. RFC 5849 §3.4.1.1 test vector (POST https://photos.example.net/initiate) verified byte-exactly via fixture files. `docs/MANUAL.md` §6.8 signing-types table gains the `oauth1` row with HMAC-SHA1 default, HMAC-SHA256 opt-in, and explicit RSA-SHA1/PLAINTEXT not-supported note. (M17-003)
- AWS Signature Version 4 signer (`signing: {type: aws-sigv4, ...}`): standard-library-only implementation (no AWS SDK dependency) registered at init time via `internal/signer/awssigv4`. Required params: `region`, `service`, `access_key`, `secret_key`; optional: `session_token` for STS-issued credentials. Injects `x-amz-date`, `x-amz-content-sha256`, and (when present) `x-amz-security-token` before computing the canonical request; signs all injected headers except `x-amz-content-sha256` (body hash already present as the last canonical-request field). Sensitive-secret propagation: when `secret_key` or `session_token` resolve from a sensitive variable, `SensitiveSet.AddValue(resolved)` is called so the value is redacted in serialised output — mirrors the M12-005 `$hmacSha256` pattern. Literal secrets cannot be back-traced and are not auto-redacted (documented footgun in `docs/MANUAL.md` §6.8). Multi-value query params from both the URL string and `QueryParams` map are merged, sorted by key then value, and RFC 3986 percent-encoded. `docs/MANUAL.md` gains a new §6.8 "Request signing" table with the `aws-sigv4` row and a usage example. (M17-002)
- Signer registry foundation: new `internal/signer` package with `Signer` interface, named `Registry`, `ErrUnknownSignerType` sentinel, and `NewBuiltinRegistry` constructor (empty in M17-001; `aws-sigv4` lands in M17-002, `oauth1` in M17-003). The `signing:` YAML field is added to `parser.Collection` and `parser.RequestItem` with collection-level default + per-request override + explicit-null disable semantics (mirrors `auth_profile` precedence). The runner gains a signer exec-wrap step between variable templating and HTTP dispatch; a `PreExec` hook on `parallel.Config` covers the `--parallel` wave path. `auth/profile.go` is unchanged. `docs/MANUAL.md` replaces the `curlew-sigv4` plugin example with a built-in `signing: {type: aws-sigv4, ...}` placeholder and adds a clarification that the Enterprise gate applies only to user-supplied plugins — built-in signers and dynamic functions are universally available. (M17-001)
- Dynamic functions: 4 `$faker.*` file-data functions — `$faker.fileName`, `$faker.fileExtension`, `$faker.mimeType`, `$faker.imageUrl`. `$faker.fileName` composes a lowercase base name + dotted extension matching `^[a-z0-9_-]+\.[a-z0-9]+$`. `$faker.fileExtension` returns a 2-to-4-character lowercase extension with no leading dot. `$faker.mimeType` returns a `type/subtype` string drawn from the parallel `fileExtensions` / `fileMimeTypes` pool aligned 1:1 by index. `$faker.imageUrl` is the family's only argument-bearing function — it accepts an optional `(width, height)` pair as two single-quoted positive-integer strings parsed by M12-001's `parseDynArgs` unchanged; the no-arg form returns the constant `https://picsum.photos/640/480` and the two-arg form returns `https://picsum.photos/<w>/<h>` after `strconv.Atoi`-converting and positivity-checking each. Non-integer args return `DYNFN_FAKER_IMAGEURL_BAD_INPUT`; non-positive integers return `DYNFN_FAKER_IMAGEURL_BAD_DIMENSION`; 1 or 3+ args return `DYNFN_ARITY` ("expected 0 or 2 arguments"). The three RNG-bearing functions are seed-deterministic under `--seed`; the `$faker.imageUrl` no-arg default is a constant string and intentionally identical across runs. en-US only; locale support is deferred. MANUAL.md §3.7 gains 4 new rows with both default and arg-bearing forms shown for `$faker.imageUrl`. (M13-008)
- Dynamic functions: 8 `$faker.*` financial-data functions — `$faker.price`, `$faker.currencyCode`, `$faker.currencyName`, `$faker.currencySymbol`, `$faker.creditCard`, `$faker.creditCardCVV`, `$faker.iban`, `$faker.bic`. `$faker.price` returns a 2-decimal float string in [1.00, 1000.00] by default; `$faker.price('5','50')` clamps output to [5.00, 50.00] using M12-001 parseDynArgs. `$faker.currencyCode/Name/Symbol` draw from three parallel pools aligned 1:1:1 by index for coherent seeded-RNG sequences. `$faker.creditCard` generates a Visa-like 16-digit number passing the Luhn checksum; `$faker.creditCardCVV` generates a 3-or-4 digit CVV; `$faker.iban` generates a structurally valid IBAN with the mod-97 check digits computed per ISO 13616. All three are auto-sensitive: each generated value is registered with `SensitiveSet.AddValue` at generation time and appears as `[REDACTED]` in `-vv`, `--format markdown`, and JSON request-body output, mirroring the M12-005/M13-002 pattern. `$faker.bic` generates a valid 8-or-11-character BIC/SWIFT code and is NOT auto-sensitive (bank identifier, not account). All 8 functions are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. en-US only; locale support is deferred. MANUAL.md §3.7 gains 8 new rows with [REDACTED] callout on the three auto-sensitive rows. (M13-007)
- Dynamic functions: 5 `$faker.*` content-data functions — `$faker.word`, `$faker.words`, `$faker.sentence`, `$faker.paragraph`, `$faker.text`. `$faker.word` returns a single lowercase word from the 247-entry lorem-ipsum pool. `$faker.words(count)` returns `count` space-joined words (default: 3). `$faker.sentence(wordCount)` returns a sentence with `wordCount` words (default: 6–10), first word capitalised, period-terminated. `$faker.paragraph(sentenceCount)` returns `sentenceCount` sentences joined by single spaces (default: 3–5). `$faker.text(charCount)` emits sentences until cumulative length reaches `charCount`, stopping at the next sentence boundary (default: 200 chars). All four arity-flex functions accept an optional integer-string argument parsed by M12-001's `parseDynArgs` unchanged; the registration body `strconv.Atoi`-converts `args[0]` internally, mirroring `$randomBase64`. Non-integer args return `DYNFN_FAKER_CONTENT_BAD_INPUT`; counts below 1 return `DYNFN_FAKER_CONTENT_BAD_LENGTH`. All 5 functions are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. en-US only; locale support is deferred. MANUAL.md §3.7 gains 5 new rows with both default and arg-bearing forms. (M13-006)
- Dynamic functions: 9 `$faker.*` internet-data functions — `$faker.url`, `$faker.domain`, `$faker.domainSuffix`, `$faker.ip`, `$faker.ipv6`, `$faker.mac`, `$faker.userAgent`, `$faker.color`, `$faker.hexColor`. `$faker.url` composes as `https://<root>.<tld>/<segment>` with 4320+ combinations. `$faker.ip` generates dotted-quad IPv4 with each octet in [0, 255]. `$faker.ipv6` generates full-form IPv6 (8 groups of 4 lowercase hex digits) accepted by `net.ParseIP`. `$faker.mac` produces uppercase hex MAC-48 (`^([0-9A-F]{2}:){5}[0-9A-F]{2}$`). `$faker.color` returns a CSS color *name* keyword (e.g. `red`, `blue`); this is distinct from the existing `$randomColor` which returns a `#RRGGBB` hex string — both registrations co-exist per M13 Open Decision #3. `$faker.hexColor` returns `#RRGGBB` lowercase hex. All 9 functions are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. en-US data only in M13; locale support is deferred. MANUAL.md §3.7 gains 9 new rows plus a `$faker.color`/`$randomColor` distinction note. (M13-005)
- Dynamic functions: 5 `$faker.*` company-data functions — `$faker.company`, `$faker.companySuffix`, `$faker.jobTitle`, `$faker.department`, `$faker.catchPhrase`. `$faker.companySuffix` draws from a closed canonical set `{Inc., LLC, Corp., Ltd., Co.}`. `$faker.catchPhrase` composes three independent pool draws (`<adjective> <noun> <gerund>`) joined by single spaces. All five functions are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. en-US data only in M13; locale support is deferred. MANUAL.md §3.7 gains 5 new rows. (M13-004)
- Dynamic functions: 12 `$faker.*` location-data functions — `$faker.address`, `$faker.street`, `$faker.streetName`, `$faker.city`, `$faker.state`, `$faker.stateAbbr`, `$faker.zipCode`, `$faker.country`, `$faker.countryCode`, `$faker.latitude`, `$faker.longitude`, `$faker.timezone`. All are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. State and country pools are parallel slices (1:1 aligned by index). `$faker.latitude` and `$faker.longitude` return string-encoded floats with 4 decimal places, rendering as JSON numbers when placed in a numeric position. `$faker.timezone` returns a valid IANA identifier loadable via `time.LoadLocation`. en-US data only in M13; locale support is deferred. MANUAL.md §3.7 gains 12 new rows. (M13-003)
- Dynamic functions: 10 `$faker.*` personal-data functions — `$faker.firstName`, `$faker.lastName`, `$faker.fullName`, `$faker.username`, `$faker.email`, `$faker.phone`, `$faker.phoneInternational`, `$faker.namePrefix`, `$faker.nameSuffix`, and `$faker.ssn`. All are seed-deterministic under `--seed`; without a seed they draw from `crypto/rand`. `$faker.ssn` is auto-sensitive: each generated SSN is registered as a redaction trigger for the run and appears as `[REDACTED]` in `-vv`, `--format markdown`, and events-stream output. The value-side sensitive-return hook (`Registry.IsSensitiveReturn`) is symmetric with the existing M12-005 arg-side hook. en-US data only in M13; locale support is deferred. MANUAL.md §3.7 gains 10 new rows. (M13-002)
- Dynamic functions: dotted-namespace syntax foundation for `$faker.*`. `dynPattern` now accepts function names with dot-separated namespace segments (e.g. `{{$faker.firstName}}`, `{{$faker.imageUrl(640, 480)}}`); each segment satisfies `[a-zA-Z][a-zA-Z0-9_]*` — consecutive dots, leading dots, and trailing dots are rejected. The captured name including dots flows verbatim to Registry.Evaluate as the lookup key; unknown dotted names return the standard `unknown dynamic function "faker.firstName"` error. Backward-compatible: all existing `{{$fn}}` and `{{$fn(args)}}` forms are unaffected. SPECIFICATION.md:748 typo fixed (52 → 53 faker functions). MANUAL.md §3.7 gains a "Dotted-namespace function names (M13+)" paragraph pointing forward to `$faker.*`. (M13-001)
- Dynamic functions: `$randomPassword(length)` and `$randomBase64(byteLength)` random-secret generators. `$randomPassword(n)` returns an n-character password (n >= 4) guaranteed to contain at least one ASCII uppercase, one lowercase, one digit, and one symbol from `!@#$%^&*()-_=+[]{}<>?,.`; the result is Fisher-Yates shuffled so guaranteed-class characters are not always positional. `$randomBase64(byteLength)` returns the standard-base64 encoding (RFC 4648 §4) of `byteLength` random bytes. Both functions are seed-deterministic under `--seed`; without a seed, both draw from `crypto/rand`. Structured `[INPUT]` errors for lengths < 4 (password), < 1 (base64), non-integer arguments, and arity mismatches. MANUAL.md §3.7 gains two table rows plus a prose block on the four-class guarantee, symbol set, and seed-determinism rule. (M12-008)
- Dynamic functions: `$formatDate(input, layout)` and `$parseDate(s, layout)` layout-aware date helpers. `$formatDate` accepts either a Unix-seconds integer string or an RFC3339 string and emits the time formatted with a Go reference-time layout. `$parseDate` is the inverse: parses any layout-formatted string and returns canonical ISO-8601 UTC. Non-UTC zone-tagged inputs are converted to UTC. Both use the `twoArgs` wrapper for arity-2 enforcement with `DYNFN_ARITY` errors. Structured errors for empty input (`DYNFN_FORMATDATE_EMPTY_INPUT`, `DYNFN_PARSEDATE_EMPTY_INPUT`) and unrecognised input shape (`DYNFN_FORMATDATE_BAD_INPUT`, `DYNFN_PARSEDATE_BAD_INPUT`). MANUAL.md §3.7 gains two table rows and a "Go reference-time layouts" subsection documenting the convention, common layouts, and the layout-as-template footgun. (M12-007)
- Dynamic functions: `$dateAdd(amount, unit)` and `$dateSubtract(amount, unit)` relative-time arithmetic helpers. Both take an integer string `amount` and one of seven canonical units (`second`, `minute`, `hour`, `day`, `week`, `month`, `year`). Result is ISO-8601 UTC matching `$isoTimestamp` shape. Negative amounts accepted. Arity and bad-unit errors return structured `[INPUT]` codes (`DYNFN_DATE_BAD_AMOUNT`, `DYNFN_DATE_BAD_UNIT`). Month/year offsets follow Go `time.AddDate` semantics (rollover documented in MANUAL.md §3.7). An unexported `now func() time.Time` clock seam on `Registry` (default `time.Now`) enables frozen-clock golden tests and is reusable by M12-007 `$formatDate`. (M12-006)
- Dynamic functions: `$hmacSha256(payload, key)` cryptographic signing helper. `{{$hmacSha256('The quick brown fox jumps over the lazy dog', 'key')}}` returns `f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8` (lowercase 64-char hex). When the `key` argument resolves from a sensitive variable (name matching the heuristic: contains `secret`, `token`, `password`, etc.) or from `{{secrets.X}}`, the resolved key string is automatically registered in the run's `SensitiveSet` via `AddValue` so it is redacted everywhere it would appear in serialised output. Literal keys passed inline are not auto-redacted (documented caveat in MANUAL.md §3.7). The payload argument does not trigger auto-redaction (HMAC payloads are data being signed, not credentials). `SensitiveSet` is now protected by `sync.RWMutex` for safe concurrent use in `--parallel` mode. Arity errors return structured `DYNFN_ARITY` codes. MANUAL.md §3.7 gains a `$hmacSha256` row plus a callout block on sensitive-key behaviour and the literal-key footgun caveat. (M12-005)
- Dynamic functions: `$sha256` and `$md5` hashing helpers. `{{$sha256('hello')}}` returns `2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824` (lowercase 64-char hex). `{{$md5('hello')}}` returns `5d41402abc4b2a76b9719d911017c592` (lowercase 32-char hex). Both functions operate on the UTF-8 byte sequence of their input, honour per-request memoisation, and support nested variable resolution (`{{$sha256('{{body}}')}}`). Arity errors return structured `DYNFN_ARITY` codes. `$md5` is provided for parity with legacy webhook signature schemes only — new integrations should prefer `$sha256`. MANUAL.md §3.7 argument-bearing helpers table gains two rows plus an MD5 deprecation caveat. (M12-004)
- Dynamic functions: `$urlEncode` and `$jsonEncode` string-encoding helpers. `{{$urlEncode('hello world & co')}}` returns `hello+world+%26+co` (mirrors `net/url.QueryEscape`; space→`+`, `&`→`%26`, `/`→`%2F`). `{{$jsonEncode('he said "hi"')}}` returns the RFC 8259 JSON-string literal `"he said \"hi\""` including the surrounding double quotes, suitable for direct inline in JSON request bodies. Both helpers honour per-request memoisation and support nested variable resolution. Arity errors return structured `DYNFN_ARITY` codes. MANUAL.md §3.7 gains two rows in the argument-bearing helpers table. (M12-003)
- Dynamic functions: `$base64` and `$base64Decode` encoding helpers. `{{$base64('user:pw')}}` returns standard base64 (`dXNlcjpwdw==`); `{{$base64Decode('aGVsbG8=')}}` returns the decoded UTF-8 string. `$base64Decode` rejects malformed input with a structured `[INPUT]` error containing a 32-char snippet and the underlying decoder error. Both functions honour per-request memoisation. Nested variable resolution inside arguments (`{{$base64('{{user}}:{{pass}}')}}`) supported. MANUAL.md §3.7 gains an argument-bearing helpers table. (M12-002)
- Dynamic functions: `{{$func('arg1', 'arg2')}}` parenthesized-arg syntax. Single-quoted literals with `\'`/`\\` escapes; nested `{{var}}` references inside literals are resolved before being passed to the function. All 16 existing no-arg functions (`{{$timestamp}}`, `{{$uuid}}`, etc.) migrated to the new `DynFunc(rng, args)` signature; they reject unexpected args with a structured arity error. Per-request memoization key now includes resolved args so `{{$base64('a')}}` and `{{$base64('b')}}` cache independently. Backward-compatible: `{{$func}}` (no parens) resolves identically to before. MANUAL.md §3.7 gains a subsection introducing the syntax with examples. (M12-001)
- exec --log: JSONL entries gain `run_id` (32-char lowercase hex per invocation, crypto/rand) and `request_id` (`"req-1"` for single-request exec) correlation fields. Both are populated for live exec, dry-run, and HTTP-error paths; both use omitempty so older log readers see no schema break. New `internal/output/ids` package provides the canonical run-id generator; `runner.NewRunID` and `internal/output/events.newRunID` now delegate to it so events stream, exec --log, and markdown sentinels share one implementation. MANUAL.md §4.5 example updated. Closes IMPROVEMENT.md §6 W5 bonus and the §2.4/§6 follow-up cluster opened in M11. (M11-004)
- TAP output: per-iteration data-driven test points and parallel-execution YAML diagnostic block. The TAP formatter now emits one `ok N - BaseName [i/N]` (or `not ok ...`) line per data-driven iteration with the plan count `1..N` matching the actual test-point count; the `# Data-Driven: <name> (<M> iterations)` group marker comment continues to head the iteration block. Parallel runs with `wave_count > 1` append a TAP 13 YAML diagnostic block between the last test point and the trailing summary, carrying `speedup_factor`, `wave_count`, and `max_parallelism`; `speedup_factor` is byte-equal to `parallel_execution.speedup_factor` in the JSON formatter. New helper `cmd/curlew/main.go::buildParallelMetadata` shares the speedup arithmetic between JSON and TAP paths. New struct `internal/output.ParallelTAP`; `output.WriteTAP` gains a fifth `parallel *ParallelTAP` argument (callers pass `nil` for sequential and single-wave runs). Closes IMPROVEMENT.md §2.4 bullets 1 (TAP half) and 2. (M11-003)
- JSON output: root `summary` block and per-iteration `data_driven[].iterations[]` entries. `JSONOutput` gains `Summary *SummaryJSON` (`{total, passed, failed, skipped}`) populated unconditionally — `jq '.summary.passed'` works without traversing `requests[]`. `DataDrivenJSON` gains `Iterations []DataDrivenIterationJSON` (`{name, status, duration_ms, data_columns}`) populated whenever a data-driven request ran. Iteration `name` matches the events-stream convention `BaseName [i/N]`. `iterations[].length == total_iterations` is the new invariant. Closes IMPROVEMENT.md §2.4 bullets 1 (JSON half) and 4. (M11-002)
- Added: --skill claude flag for curlew init scaffolds Claude Code agent skill at .claude/skills/curlew/SKILL.md, enables events stream by default, defaults output format to markdown. Skill content includes trigger phrases, invocation form, artifact tree, narration discipline, and an executable-spec failure playbook covering exit codes 0, 1, 2, 3, 4, 5, 6, 9 with one mock-agent-loop sub-test per code asserting the playbook matches binary behaviour. SPECIFICATION.md gains a Project Initialization subsection; MANUAL.md gains §4.9 Driving curlew with Claude Code with a worked example. (M10-002)
- CLI: `curlew init --skill <name>` flag (enum: `claude`) scaffolds `.claude/skills/curlew/SKILL.md` from an embedded Go template with `{{curlew_version}}` substituted at scaffold time. `--skill claude` defaults the `output:` block to `format: markdown`, appends `events: .curlew/run.ndjson`, and extends `.gitignore` with `.curlew/`. Bare `init` (no `--skill`) is byte-identical to its M9-005 baseline. Unknown `--skill` value exits 3 with a message naming the supported enum. (M10-001)
- Added: `--format markdown` output fully documented and discoverable — `curlew init --output markdown` scaffolds an `curlew.yaml` with `output: { format: markdown, report: responses/ }`. Other supported `--output` values (`terminal`, `json`, `tap`, `junit`, `html`) scaffold equivalent blocks pointing at default per-format paths; unknown values exit 3 with an error naming the enum. New `curlew init --help` documents the flag. SPECIFICATION.md gains a Markdown Output Format subsection; MANUAL.md §5.7 gains a VS Code split-pane worked example. Closes IMPROVEMENT.md W4 (M9-001..M9-005).
- CLI: `--format markdown` output extended with parallel wave grouping and data-driven per-iteration file layout. Parallel collections group `run.md` entries under `## Wave <N>` headers in ascending wave order; each per-request `.md` file's Timing section gains `wave_index: <N>` (or `wave_index: sequential`). Data-driven requests produce a `<report>/<slug>/` subdirectory containing one `iter-<n>.md` per iteration (splice-safe, with `request_id=req-N-iter-M` correlation IDs) plus an `index.md` manifest with summary counts and an iteration table; run.md collapses data-driven requests to a single bullet with aggregate counts. If the data source exceeds the runner row cap, `index.md` adds a truncation marker row and iter files beyond the cap are not written. All files are splice-safe: re-runs rewrite only the sentinel region, preserving agent-authored content. (M9-004)
- CLI: markdown formatter extended with full content-type coverage (JSON/YAML/XML/HTML/Text/Empty/HEAD/Binary), a 1 MiB body cap applied post-redaction, a `### Response metadata` subsection that quarantines volatile headers (Date, X-Request-Id, Set-Cookie, Etag, Server, Age) below the response signal, and a redaction invariant enforcing that `--allow-sensitive` cannot leak secrets into markdown output. (M9-003)
- CLI: `--format markdown --report <dir>` renders one `<slug>.md` per main-phase request plus a `run.md` index. Each file has a fixed 10-section order with a `<!-- BEGIN/END curlew:response id=... slug=... run=... -->` sentinel block carrying all three correlation IDs. Re-running splices only the sentinel region, preserving agent-authored content above and below verbatim. Existing files without the expected sentinel pair (malformed, no sentinels, or user-owned) receive a `.md.new` sibling and a stderr warning — the original is never overwritten. JSON response bodies are pretty-printed in a `json` fenced block; file writes are atomic (O_EXCL temp-file + rename). `--format markdown` without `--report` exits 3 at load time. Both collection and project schemas now accept `"markdown"` as a valid format enum value. (M9-002)
- Events schema v1.2: `request.start` and `request.end` events gain an optional `request_slug` field — a URL-safe identifier derived from the request name (NFKD normalized, lowercased, non-alphanumeric runs collapsed to `-`). `schema_version` promoted from `"1.1"` to `"1.2"` in all emitted events. `docs/events-schema/v1.2.json` published as a new versioned schema; `v1.1.json` and `v1.0.json` retained as historical anchors. Parser rejects request names that slugify to empty (all-punctuation, all-CJK, etc.) at load time with `PARSE_SLUG_EMPTY`. (M9-001)
- CLI: `--only "<name>"` flag on `curlew run` (and `curlew watch`) selects a single main request by name; repeatable for a union (e.g. `--only "Get user" --only "Update user"`). Setup and teardown still run in full regardless of selection. Unknown names fail immediately with exit code 3 and an "available:" list before any HTTP requests are sent. When a selected request references a variable produced by a filtered-out request, the variable-cliff error message names the filtered producer and suggests adding it to `--only` or passing the value via `--var`. Duplicate main request names are now rejected at parse time (exit 3 with line-numbered error via `PARSE_DUPLICATE_REQUEST_NAME`) — this invariant is inherited by `curlew run`, `curlew watch`, and `curlew validate` automatically. Events schema promoted from v1.0 to v1.1 (additive, backward-compatible): `run.start` gains an optional `selection` field carrying the `--only` values for the run; `schema_version` changes from `"1.0"` to `"1.1"` in every emitted event; `docs/events-schema/v1.1.json` created, `v1.0.json` retained as a historical anchor; `docs/EVENTS_SCHEMA_v1.1.md` documents the diff. New error code `ONLY_NO_MATCH` (category `input`) emitted in `run.error` when `--only` finds no matching main request. (M8-004)
- CLI: typed `output:` block accepted at both `curlew.yaml` (project default) and collection YAML (override); precedence CLI > collection > project > built-in resolved once in `runCmdInner` before formatter and events-emitter construction. New `schemas/project-v1.json` published at repo root (`$id = https://raw.githubusercontent.com/peterlindqvist/curlew/main/schemas/project-v1.json`, `title = "Curlew Project v1"`, `additionalProperties: false`); `schema.ProjectSchema` added as the byte-slice alias (mirrors M8-001 pattern). `curlew schema --project` emits the project schema on stdout; `curlew schema` (no flag) continues to emit the collection schema (backward-compatible). `curlew init --project-name <value>` now sets the project name explicitly instead of deriving it from the directory basename; `curlew init` also writes an active minimal `output:` block (`format: terminal`, `verbosity: normal`) into `curlew.yaml`, and the scaffolded file validates against the project schema (`TestSchema_scaffolded_curlew_yaml_validates`). `runFlags` gained `formatSet/reportSet/eventsSet/verbositySet` parallel bools so CLI zero-values are distinguishable from "flag not passed". The `$defs.output` subschema is byte-identical across both schema files (guarded by `TestSchema_output_defs_match`). New fixtures: `internal/schema/testdata/output_*.yaml` (one per format, per verbosity, plus project-level examples). MANUAL.md §1.5 gains the second `yaml.schemas` mapping for `schemas/project-v1.json` → `curlew.yaml`; new §3.6.1 describes the block and the precedence table. Markdown format remains excluded pending W4. (M8-003)
- CLI: collection JSON Schema now covers the full parser grammar — `requestItem.auth` (references an auth profile in `auth_profiles` block of `curlew.yaml`); `retry:` at collection, section (`setup`/`teardown`/`requests` object form), and request scope (mirrors `internal/retry.FullConfig` fields including `retry_on`/`do_not_retry_on` with `backoff_strategy` enum constrained to `exponential|linear|constant`); `requestItem.data_driven` (mirrors `internal/datadriven.Config` with `source` required, `format` enum, `store_results` enum); section phases (`setup`/`teardown`/`requests`) accept both the array form and an object form `{retry, items}`; `variables` entries accept the object form `{from_command | value, sensitive, cache}` in addition to plain scalar strings, with `from_command`/`value` declared mutually exclusive; `assertions.status` constrained to `oneOf[integer, array[integer]]` in the 100–599 range. `redhat.vscode-yaml` now surfaces autocomplete, hover docs, and inline validation for every advanced feature. One fixture per gap under `internal/schema/testdata/`, with `TestSchema_accepts` (per-gap acceptance), `TestSchema_examples` (regression walker over `internal/schema/testdata/*.yaml` and `sample/hello.yaml`), `TestSchema_rejects_malformed_status`, and `TestSchema_rejects_variable_with_both_value_and_from_command` added to `internal/schema/validate_coverage_test.go`. Closes the W1 audit gap list (IMPROVEMENT.md §5 Risks). (M8-002)
- CLI: collection JSON Schema is now published at a stable, versioned in-repo path `schemas/collection-v1.json` (with `$id` set to the raw.githubusercontent URL and title `Curlew Collection v1`). Developers editing `collections/*.yaml` can wire a `.vscode/settings.json` snippet mapping `yaml.schemas` at the file (or at the published URL for consumers using Curlew outside its source tree) to get autocomplete, hover docs, and inline validation through the `redhat.vscode-yaml` extension. The new repo-root `schemas/` package owns the `//go:embed` directive; `internal/schema/schema.go` aliases `schemas.CollectionV1` so `schema.CollectionSchema` remains the stable public symbol — `curlew schema`, `curlew validate`, and all internal callers are unchanged. New tests: `TestSchema_validates_scaffolded_sample` validates the sample.yaml produced by `curlew init` against the schema; `TestSchema_rejects_sample_missing_required_name` is a negative control; `TestSchema_published_path_matches_embed` is the drift guard between the file and the embedded bytes; `TestSchema_file_exists_at_published_path` is the cheap reachability guard. See `docs/MANUAL.md` §1.5 for the snippet. Schema-completeness for the advanced features (`auth`, `retry` at three levels, `data_driven`, section object form, variables object form, `status` oneOf union) is tracked separately under M8-002. (M8-001)
- CI regression gate `TestStreamDisciplineMatrix` (`cmd/curlew/stream_discipline_matrix_test.go`) spawning the real `curlew` binary across a matrix of (subcommand × format) cells: `run` with json/tap/junit/html/terminal formats on happy, assertion-failure, parse-error, and feature-gate-denied fixtures; plus `perf`, `license --validate`, and `exec --dry-run` cells. Each cell wires stdout and stderr to pipes and asserts: (1) stdout contains only the declared format's payload, (2) stderr carries no ANSI escape sequences on pipes, (3) no known-progress string (`Running...`, `Load test:`, `Validating license`, `Claimed shard`, `Resolved `, `Exporting license bundle`) leaks to stdout. Fixtures live under `cmd/curlew/testdata/stream-discipline/`. `scripts/ci-local.sh` now runs this test under its own named step so failures surface under a distinct header in CI logs. TTY-wired cells remain owned by `TestStderrColorFlag` (M7-001). (M7-004)
- CLI: event-stream schema promoted from v0.1 to v1.0 (M6-007): agent-diagnosability validation harness (`cmd/curlew-agent-harness/`) drives seven deliberately-broken fixture collections (`missing-variable`, `bad-yaml`, `failing-assertion`, `unreachable-host`, `auth-missing`, `feature-gate-denied`, `circular-include`) and asserts every failure event carries non-empty `category`, `code`, `file`/`line`, and an actionable `hint`; production fixes to pass all seven: `ErrUndefinedVariable` category changed from `config` to `input` with new `VAR_UNDEFINED` code; `enrichInterpErr` helper threads source location into structured errors; `RegisterTypeClassifier` enables typed errors (e.g., `*auth.GateError`) to be classified without sentinel registration; `ErrAssertionFailed` sentinel surfaces `ASSERTION_FAILED` code on `request.end outcome=failed`; `PARSE_CIRCULAR_INCLUDE` and `PARSE_CIRCULAR_FILE_REFERENCE` hints updated to use canonical action verbs; `SchemaVersion` bumped from `"0.1"` to `"1.0"` atomically; `docs/events-schema/v1.0.json` created, `v0.1.json` retained with `deprecated=true`; `docs/EVENTS_SCHEMA_v1.0.md` documents the v0.1→v1.0 diff (no breaking changes; new error codes listed); stability policy updated to require v2.0 for removals/renames (M6-007)
- CLI: `docs/EVENTS_SCHEMA_v0.1.md` agent-grade event stream reference: covers all six event kinds with schema tables and examples, stream invariants, ordering and timing guarantees, body truncation and encoding rules, error taxonomy (category → producing package → hint shape), `run.error` vs `request.end` with error distinction, stability policy (additive changes allowed in v0.x; rename/removal requires major bump; v1.0 gate is M6-007), and consumer guidance for agents; `docs/events-schema/v0.1.json` JSON Schema stays authoritative; `TestSchema_DocInSyncWithCode` uses Go reflection to assert all struct fields map 1:1 to schema properties; `TestSchema_MarkdownExamplesValidate` validates all 16 fenced code examples against the compiled schema (M6-006)
- CLI: `--events <path>` flag for the `run` subcommand writes a streaming NDJSON file containing `run.start`, per-request `request.start` / `assertion.result` / `request.end`, and `run.end` events alongside all existing `--format` outputs; request IDs are monotonic (`req-N`) across goroutines via an atomic counter; sensitive `--var`/`--env-var` values are redacted in `run.start.cli_args`; the events file is opened before any HTTP execution so unwritable paths fail early with exit 1; parallel wave emission threads `wave_index` through `internal/parallel.Config.EventSink`; `--events` is explicitly rejected on `worker`, `perf`, and `pr-check` subcommands with the message `--events is supported only on run; use --format jsonl for streaming samples`; `runner.EventSink` interface keeps `internal/runner` independent of `internal/output/events` (no import cycle) (M6-005)
- CLI: events emitter package with NDJSON event types (`internal/output/events`): defines six event kinds (`run.start`, `run.error`, `request.start`, `request.end`, `assertion.result`, `run.end`) with a stable JSON header ordering; `Emitter` serializes events atomically (mutex-guarded `io.Writer` + atomic id counter) ensuring no partial writes under concurrent use; body truncation at 2048 bytes with UTF-8 safety and base64 fallback for binary content; errors classified via `apierrors.ClassifyError`; `docs/events-schema/v0.1.json` JSON Schema validated against all emitted events; golden NDJSON fixtures for happy path, error, and failed-assertion scenarios; no runner integration (lands in M6-005) (M6-004)
- CLI: body redaction for sensitive variables (`--allow-sensitive` opt-out preserved): `variable.RedactBody` walks JSON structures and performs exact-substring replacement in plain-text bodies; `SensitiveSet` extended with `AddValue`/`Values()` to track resolved secret values; `cmd/curlew` redaction block now mutates `RequestBody` and `Result.Body` before any formatter consumes them, ensuring no sensitive value leaks into terminal `-vv`, JSON, TAP, or JUnit output (M6-003)
- CLI: `SourceFile` and `SourceLine` fields on `parser.RequestItem` and `runner.RequestResult` to carry YAML source location through the full parse-to-run pipeline (M6-002): each parsed request records the absolute path of its origin file and the 1-based YAML line number; `include:` contributors carry the included file's path; external `path:` references carry the external file path with `SourceLine = 1`; data-driven iterations copy the base item's location verbatim; parallel wave results (`RequestOutcome`) thread the location into `RequestResult` unchanged; additive-only change with no output format behaviour change.
- CLI: example plugin + developer docs (M5-019): `examples/plugins/datadog-metrics/` — a production-shaped plugin (separate Go module) that submits `curlew.request.duration` gauge metrics to Datadog v2 `/api/v2/series` on every `on_response` hook; disabled gracefully when `DATADOG_API_KEY` is absent; `DD_API_URL` override lets tests run against a local fake server with no Datadog account needed; `--help` flag prints plugin metadata and exits 0; 15 tests (httptest-backed fake Datadog server, loadConfig, handshake, end-to-end run loop, standalone binary), coverage ≥80%; `examples/plugins/README.md` index; `docs/plugins.md` expanded with Quickstart, Full example, Packaging tips, Debugging, Troubleshooting checklist sections; `smoke/run.sh` block builds and verifies the example binary (M5-019)
- CLI: plugin hook registry for request/response/result lifecycle (M5-018): `on_request` (before each HTTP request; may mutate method/url/headers/body/query_params), `on_response` (after each response; may attach annotations), `on_result` (once at run completion with pass/fail/skip counts and per-test rows); plugins declared in `CURLEW_PLUGINS` remain alive for the entire run; per-plugin 10-second timeout (timed-out plugins are terminated and dropped from all subsequent hooks); `on_request` JSON-RPC errors abort the request (marked `error`, not `fail`); two plugins chain in declared order; `internal/plugin.Channel` adds persistent JSON-RPC duplex; `internal/plugin.Host.LoadForRun` keeps channels alive after handshake; `internal/plugin/hooks` subpackage with `Registry`, `Dispatcher`, `RequestPayload`, `ResponsePayload`, `ResultPayload`; `HooksDispatcher` interface in `runner.VarSources`; `buildHookDispatcher` helper in `cmd/curlew/plugins.go`; help text includes "Plugin hooks" section; `testdata/plugins/hooklog-plugin` fixture; `docs/plugins.md` updated with full hook invocation protocol; test coverage ≥80% (M5-018)
- CLI: `curlew plugins list` subcommand and `internal/plugin` external-process plugin loader (M5-017): `CURLEW_PLUGINS` env var (colon-separated files or directories) discovers plugin executables; JSON-RPC 2.0 `curlew/hello` handshake over stdin/stdout with 5s timeout negotiates `{name, version, hooks, protocol_version}`; known hooks `on_request`, `on_response` are kept; unknown hooks print a warning and are filtered; duplicate names and missing/non-executable binaries exit 2; timeouts are non-fatal (exit 0, warning to stderr); `testdata/plugins/hello-plugin` sample fixture; `docs/plugins.md` documents the handshake schema, exit codes, and minimum Go example; test coverage ≥80%; one integration test builds the fixture under `go test` (M5-017)
- Backend: admin bootstrap and EF Core migration runner on startup (M5-016): `BACKEND_RUN_MIGRATIONS=1` applies all pending EF migrations via `MigrationRunner` (exit code 4 on failure); `BOOTSTRAP_ADMIN_EMAIL` + `BOOTSTRAP_ADMIN_PASSWORD` (>=12 chars) seed a first-boot admin user with `role=admin` in a default Enterprise-tier organization on an empty database (idempotent: `"bootstrap: admin already exists, skipping"` on restart); exit code 3 on invalid bootstrap config (password too short, invalid email, partial pair); `POST /api/v1/auth/login` endpoint for local-password auth issues a JWT with `role` claim; argon2id PHC-format password hasher (OWASP 2024 params: m=64MiB, t=3, p=4); timing-safe placeholder hash on user-miss path prevents email enumeration; `deploy/self-hosted/.env.example` documents all new vars; Npgsql provider selected at runtime via `POSTGRES_HOST`; `SchemaGuardMiddleware` returns 503 when `BACKEND_RUN_MIGRATIONS=0` and schema is absent (catches SQLite `no such table` and Npgsql `42P01` errors); note: rather than regenerating the full migration chain, an additive `AddUserBootstrapColumns` migration extends the existing chain — Postgres accepts TEXT/INTEGER column types from SQLite-authored migrations, and no shipped production databases exist yet (M5-015 deferred Postgres consumption to M5-016); 32 new tests (M5-016)
- Backend: `deploy/self-hosted/` docker-compose bundle (postgres, redis, backend, web) with persistent volumes, healthchecks, and `GET /health` endpoint reporting `{"status":"healthy","db":"connected","redis":"connected"}`; `postgres.Dockerfile` ships pg_cron-friendly extensions; `scripts/test-self-hosted.sh` smoke test gated behind `CURLEW_RUN_SELF_HOSTED=1`; `deploy/self-hosted/README.md` documents setup, env vars, ports, volumes, and upgrade path (M5-015)
- CLI: `curlew license export --output <file>` subcommand for air-gapped deployment: exports a gzipped tarball (`license.json`, `jwks.json`, `README.txt`) from the local config dir; `CURLEW_LICENSE_BUNDLE=<extracted-dir>` env var activates a read-only bundle for offline `license --validate`; bundle-age grace warning printed when bundle is older than 30 days; new `internal/license/export` package (`Write`, `Bundle`, `ParseMembers`), `internal/license/paths.go` (`ResolveConfigDir`), `ReadOnlyStore`, `WithBundleJWKS` key-resolver option, and `FromBundle` flag on `Result`; 7/7 behaviors covered, coverage ≥80% in all packages (M5-014)
- `curlew perf --output <path>` now supports `.json` and `.html` report formats in addition to the default summary-only stdout output: per-request latency samples are aggregated by a new `internal/loadgen/report` subpackage into p50/p95/p99 percentiles, error rate, throughput, and 1-second-resolution time-series buckets; the JSON report emits a machine-readable document with schema `curlew.perf.v1`; the HTML report is self-contained with a CDN Chart.js latency-vs-time line chart and a metrics summary. Unsupported extensions (e.g. `.xyz`) fail fast with exit code 2 and the message `error: unsupported report format .xyz`. Small sample sets (<100 requests) fall back to max for p99 and append a warning note to the summary line (M5-012)
- `curlew perf <file>` load-generation subcommand (Enterprise tier): fixed virtual-user pool with linear ramp-up and optional constant-RPS throughput mode; new `internal/loadgen` package with `Config`, `Summary`, `ExecuteFunc` seam, and `LoadRequestFile` YAML loader; `perf_loadgen` feature registered in `DefaultRegistry` at Enterprise tier; exit codes 0=ok, 1=failures, 2=usage, 3=request-file, 6=tier-gated, 130=SIGINT (M5-011)
- CLI: `--workers N` flag for distributed run execution (Enterprise tier): `internal/runner/shard` package with round-robin `Split()` planner; `internal/runner/distributed` package with `Run()` orchestrator that creates a coordinator job, waits for workers to join, polls shard completion, and aggregates results back into `[]runner.RequestResult` ordered by original index; `--coordinator-url` flag and `CURLEW_COORDINATOR_URL` / `CURLEW_BACKEND_TOKEN` env vars; feature-gated via new `distributed_execution` registry entry (TierEnterprise); `--workers 1` falls back to local with a warning; missing `CURLEW_COORDINATOR_URL` exits 2 with a clear message; help text documents all new flags and env vars; smoke test covers all key error paths (M5-010)
- E2E convergence slice: SSO login → audit capture → dashboard with custom role (`M5-020`): fake SAML 2.0 IdP sidecar (`scripts/fake-idp/` ASP.NET Core app) with signed assertion generation and `RelayState` GUID validation; `docker-compose.test.yml` extended with `fake-idp` service using `curl` healthcheck; `scripts/seed-enterprise.sh` seeds org, subscription, `qa-lead` custom role, qa user membership and SAML config idempotently with full HTTP status validation at every step; `testdata/enterprise/` fixtures (e2e-collection, cert/key PEM); `web/tests/e2e/enterprise-full.spec.ts` 7-assertion Playwright spec (6 happy-path + 1 failure-path bogus SAML); `web/tests/e2e/helpers/saml.ts` with `lookupOrgGuid`/`triggerSamlLogin`/`seedAuthCookie`/`runCurlew` helpers; `src/ApiTool.Backend/Results/ResultsEndpoints.cs` emits `results.upload` audit event on successful ingest; `src/ApiTool.Backend.Tests/Results/ResultsAuditTests.cs` 3-case integration test (success, permission-denied, invalid-schema); `.github/workflows/e2e-m5.yml` CI job on ubuntu-latest running Go + dotnet + Playwright checks (M5-020)
- CLI: distributed worker agent (`curlew worker`) for Enterprise tier coordinator (M5-009): `internal/worker` package with `Client` (HTTP coordinator protocol: claim/submit-result/heartbeat with exponential-backoff retry), `Run` orchestrator (claim-execute-submit loop, concurrent shard execution, heartbeat goroutine per shard), and `CoordinatorClient` interface for test stubbing; `curlew worker` subcommand with `--job`, `--org`, `--coordinator-url`, `--token`, `--concurrency`, `--heartbeat-interval` flags and env-var defaults (`CURLEW_COORDINATOR_URL`, `CURLEW_BACKEND_TOKEN`); exit codes 0=ok, 1=usage, 2=network, 10=unauthorized; 8xx sentinel errors (`ErrUnauthorized`, `ErrNetworkExhausted`, `ErrCoordinatorURLMissing`, `ErrTokenMissing`); fake coordinator `httptest.Server` for integration tests; `testdata/worker/sample-shard.json` fixture (M5-009)
- Backend: distributed worker coordinator service (M5-008): `CoordinatorJob` and `CoordinatorShard` EF entities with migration `AddCoordinator`; `CoordinatorService` implements full shard state machine (Pending → Running → Completed/Failed) with `CreateJobAsync`, `ClaimAsync` (first-claim-wins greedy), `SubmitResultAsync` (aggregates last-shard results via existing M4-004 `ResultsService.IngestAsync` path), `HeartbeatAsync`, `ReapStaleShardsAsync`; `ShardReaper` `BackgroundService` ticks every 15 seconds to reclaim shards with heartbeat older than 60 seconds; `CoordinatorEndpoints` exposes `POST /jobs` (201), `GET /jobs/{jobId}` (200), `POST /jobs/{jobId}/claim` (200/204 no_shards_available), `POST /jobs/{jobId}/shards/{shardId}/result` (202), `POST /jobs/{jobId}/shards/{shardId}/heartbeat` (204) under `/api/v1/organizations/{orgId}/coordinator`; `coordinator.worker` permission key added to all built-in roles (All/Owner/Admin/Member); Swagger lists all coordinator endpoints; 37 new tests (Coordinator filter ≥ 10 endpoint + 12 service + 3 reaper + ID helpers); 403 on non-member or wrong-worker shard submit (M5-008)
- Web: custom role editor UI (`/org/[slug]/settings/roles`): SvelteKit owner-only, enterprise-tier page listing built-in (read-only) and custom roles with per-role member counts; Create role modal with permission-matrix picker grouped by six spec categories (Member Management, Billing & Subscription, Organization Settings, Service Tokens, Test Resources, Team Dashboard); Edit custom role fires `PATCH /organizations/{id}/roles/{role_id}`; Delete custom role with 409 `role_in_use` guard surfacing `Role is assigned to N members` toast; typed `rolesApi` client (list, create, update, remove) with `encodeURIComponent`-encoded path params; `PERMISSION_CATALOGUE` (23 keys) and `PERMISSION_CATEGORIES` constants in `$lib/types/roles`; `ApiError.details` field added to capture non-reserved 409 body fields (e.g. `member_count`); `role_id?: string` added to `Member` type; `subnav-roles-link` gated on enterprise tier + owner role; 156 web unit tests (17 files) including 9 rolesApi tests, 7 roles loader tests, details round-trip; 8 Playwright E2E assertions covering all 7 behaviors; svelte-check 0 errors; ESLint clean (M5-007)
- CLI: offline JWT verification and grace-period state machine (`curlew license --validate`): embedded JWKS via `go:embed`; key lookup chain (embedded → cached → online → fail); RS256 signature verification with `alg=none` rejection; VALID/GRACE_PERIOD/GRACE_EXPIRED state machine with 24h freshness window and 30-day grace period; `Warning: N days until grace period expires` printed to stderr in GRACE_PERIOD (days 21-29); `checkGraceExpired()` gates `run` and `exec` commands with exit code 9 (`feature_gated`) in GRACE_EXPIRED state; state persisted to `~/.config/curlew/license.json`; `CURLEW_OFFLINE=1` and `CURLEW_LAST_VALIDATION_OVERRIDE` env vars; `internal/license` and `internal/license/jwks` packages; 87.8% / 92.0% coverage (M5-013)
- Backend: custom roles and granular permissions: `organization_custom_roles` table (EF migration `AddCustomRoles`); `Permissions` static catalogue (23 keys, three built-in sets owner/admin/member); `CustomRole` entity + `RoleId` wire-format helper; `RoleResolver` computes effective permission sets (custom role ∪ direct overrides, falls back to built-in on missing/cross-org role); `CustomRolesService` with `ListAsync`/`CreateAsync`/`DeleteAsync` (owner-only writes, member reads); `CustomRolesEndpoints` exposes `POST/GET/DELETE /api/v1/organizations/{id}/roles` (201/200/204, error codes `invalid_name`, `invalid_permission`, `role_name_taken`, `role_in_use`); `PATCH /organizations/{id}/members/{id}` extended with optional `role_id` field emitting `role.changed` audit event; `POST /results` gated on `results.upload` via `RoleResolver`; 46 new tests (480 total) (M5-006)
- Web: audit log viewer page (`/org/[slug]/audit-log`): SvelteKit page for enterprise-tier admins to view, filter, paginate, and export org audit log entries; table with columns `event_type`, `user`, `target`, `timestamp`, `IP`; 10-row client-side pagination; event_type dropdown filter and date-range preset picker (24h, 7d, 30d, all) updating URL params; CSV export via `/org/[slug]/audit-log/export` endpoint with RFC 4180 quoting and formula-injection mitigation, header row `event_type,user,target,timestamp,ip`; `requireAuth` → `requireEnterpriseTier` → `requireOrgAdmin` guard chain; non-admins redirected with `admin_required` toast; `subnav-audit-log-link` gated on enterprise + admin/owner; typed `auditLogApi` client; `formatAuditLogCsv` formatter; 137 web unit tests (16 files); 7 Playwright E2E assertions; svelte-check 0 errors; ESLint clean (M5-005)
- Backend: audit log capture middleware: `AuditCaptureMiddleware` enriches scoped `AuditContext` with `ip_address` and `user_agent` on every request; `IAuditWriter`/`AuditWriter` appends `OrganizationAuditLogEntry` rows via EF (callers control `SaveChangesAsync`); `AuditLogQueryService` queries audit rows with filter parameters (`event_type`, `user_id`, `from`, `to`, `limit`); `AuditLogEndpoints` exposes `GET /organizations/{id}/audit-log` with RBAC (admin/owner only, 403 for members), JSON and RFC 4180 CSV export with formula-injection mitigation, rate-limited at 30 req/min per org; `AuditLogCsvFormatter` with double-quote escaping and formula-injection prefix protection; six service refactors (`InvitationsService`, `OrganizationService`, `MembersService`, `SubscriptionsService`, `SamlHandler`, `OidcService`) now write audit rows via `IAuditWriter`; Swagger surface test confirms filter parameters exposed; 45 audit-scoped tests; 395 total tests passing; 91.1% line coverage (M5-004)
- Web: SSO configuration UI (`/org/[slug]/settings/sso`): SvelteKit page with SAML and OIDC tabs; SAML tab accepts `idp_metadata_url`, `acs_url`, `entity_id` and calls `PUT /organizations/{id}/sso/saml`; OIDC tab accepts `issuer_url`, `client_id`, `client_secret` and calls `PUT /organizations/{id}/sso/oidc`; inline field error display on 400 `invalid_sso_config`; `Test SSO login` button opens new tab to `/api/v1/sso/{provider}/{org_id}/login`; `requireEnterpriseTier` guard redirects non-enterprise orgs; `requireOrgOwner` guard redirects non-owners; SSO sub-nav link gated on enterprise tier + owner role; `SsoTabs`, `SamlConfigForm`, `OidcConfigForm` components; typed `ssoApi` client; backend GET `/organizations/{id}/sso` endpoint with `SsoService.GetConfigAsync` (never returns secrets); 106 web unit tests; 7 Playwright E2E assertions; svelte-check and eslint clean (M5-003)
- Backend: OIDC SSO auth flow: `PUT /organizations/{id}/sso/oidc` configures OIDC IdP with issuer URL, client ID, client secret, and optional redirect URI (org owner only, performs eager discovery probe, persists `sso_enabled=true, sso_provider="oidc"`); `GET /sso/oidc/{org}/login` returns 302 to IdP `/authorize` URL with `state`, `nonce`, and PKCE challenge in HTTP-only cookies; `GET /sso/oidc/{org}/callback` exchanges authorization code for id_token, validates signature/nonce/audience via JWKS, issues `curlew_session` cookie and redirects to web portal; `IOidcHandler` abstraction with `OidcHandler` (PKCE + token exchange) and `FakeOidcHandler` for tests; `IOidcDiscoveryClient` backed by `ConfigurationManager<OpenIdConnectConfiguration>` with 15-minute per-issuer cache; `OidcService` with structured `SsoError` result; rate-limit policies `oidc-login` / `oidc-callback`; Swagger lists all three OIDC endpoints; 53 OIDC xUnit tests (M5-002)
- Backend: SAML 2.0 SSO auth flow: `PUT /organizations/{id}/sso/saml` configures IdP metadata URL, ACS URL, and entity ID (org owner only, persists `sso_enabled=true`); `GET /sso/saml/{org_id}/login` returns 302 with signed SAMLRequest via HTTP-Redirect binding; `POST /sso/saml/{org_id}/acs` validates SAMLResponse signature, extracts NameID, and issues a session cookie; signature-wrapping attack mitigation (Reference URI cross-checked against assertion ID); `SsoCredential` entity and EF migration `AddSsoCredentials`; `ISamlHandler` abstraction with BCL `SamlHandler` and `FakeSamlHandler` for tests; `SsoService` with structured `SsoError` result type; Swagger lists all three SAML endpoints; 47 SSO xUnit tests (M5-001)
- Web: billing and seat management portal (`/org/[slug]/billing` and `/org/[slug]/members`): subscription card showing tier, price, next renewal date, and seats usage bar (`aria-valuenow/min/max`); `AddSeatsModal` sends `PATCH /subscriptions/{id}` and renders proration preview; Manage Billing anchor with `rel="noopener noreferrer"` calls `POST /subscriptions/portal` and redirects to `portal_url`; members page lists members and pending invitations in separate tables; `InviteMemberModal` sends `POST /invitations` with email validation; cancel invitation calls `DELETE /invitations/{id}` with inline confirmation banner; `requireOrgOwner` guard (owner-only billing, redirects others with `owner_required` toast); `requireOrgAdmin` guard (admin+owner members page); org sub-nav Billing and Members links gated on role; typed API clients `subscriptionsApi`, `invitationsApi`, `membersApi`; types in `web/src/lib/types/subscriptions.ts` mirror backend OpenAPI; `web/README.md` documents seeding a team-tier org; 80 unit tests; 7 Playwright E2E assertions covering all 8 behaviors; `svelte-check` 0 errors/warnings; `eslint` clean (M4-011)
- E2E convergence slice (M4-012): `curlew run --report-upload` uploads in-memory results to the backend and optionally posts a PR check in a single CLI invocation; `--org <slug>`, `--pr <number>`, `--repo <owner/repo>`, `--triggered-by <who>`, `--git-sha <sha>` flags; exit code 2 on upload failure for a passing run; backend `POST/GET /api/v1/organizations/{orgId}/pr-checks` endpoint with EF migration `AddPrChecks`; web `/org/[slug]/pr-checks` page with state badges and data-testid attributes; `testdata/team/e2e-collection.yaml` and `e2e-collection-failing.yaml` E2E fixtures; Playwright `full-pipeline.spec.ts` with 5 assertions including mocked assertion; CI workflow `.github/workflows/e2e-m4.yml`; `CURLEW_BACKEND_URL` / `CURLEW_BACKEND_TOKEN` documented in help text (M4-012)
- Backend: billing, seat management, and invitations API: `POST /api/v1/subscriptions/checkout` creates a Stripe Checkout session (FakeStripeGateway for dev/test), `GET/PATCH/DELETE /api/v1/subscriptions/{id}` manages subscription lifecycle with proration; `POST /api/v1/organizations/{id}/invitations` creates invitations (seat-counted, audit-logged), `POST /api/v1/invitations/accept` accepts invitation and creates membership; `GET/DELETE/PATCH /api/v1/organizations/{id}/members` manages member list; seat counting rule (members + pending invitations) enforced via shared `SeatCounter`; EF migration `20260417123128_AddBillingAndInvitations`; rate limiters for all 7 new policy names; `IStripeGateway` abstraction with `FakeStripeGateway` (deterministic) and `StripeGateway` stub (throws `NotImplementedException`); Swagger lists all 15 billing/invitation/member endpoints; 59 new tests (228 total) (M4-010)
- Web: notification rule configuration UI (`/org/[slug]/settings/notifications`): SvelteKit admin-only settings page listing notification rules and delivery log; `NotificationRuleModal` component for adding Slack/email rules with client-side validation (non-HTTPS Slack target blocked before API call); inline confirmation banner replacing native `confirm()`/`alert()` dialogs; `requireOrgAdmin` guard (redirects members to `/org/[slug]?toast=admin_required`); sub-nav `Notifications` link gated on team-tier + admin/owner role; `GET /api/v1/organizations/{orgId}/notification-rules` and `DELETE .../notification-rules/{ruleId}` backend endpoints added (not wired in M4-008); idempotent docker-compose seed script extension; typed API client `web/src/lib/api/notifications.ts`; 54 unit tests; 7 Playwright E2E tests covering all 6 specified behaviors + a11y keyboard dismiss (M4-009)
- Backend: notification dispatcher (Slack + email): `POST /api/v1/organizations/{orgId}/notification-rules` creates Slack webhook or email notification rules (201); `GET .../notification-deliveries` lists delivery attempts newest-first; `NotificationsDispatcher` implements `IResultIngestedNotifier`, fires matching rules on `run_failed` events, retries twice with 500 ms / 2 s exponential backoff, records delivery status (delivered/failed) with response code and error message; `ISlackWebhookPoster` + `ISmtpSender` seams with `NoopSmtpSender` default and `FakeSlackWebhookPoster`/`FakeSmtpSender` in tests; EF migration `20260417085206_AddNotifications` adds `notification_rules` and `notification_deliveries` tables; `TimeProvider` injected for testable timestamps; 24-test Notifications suite; overall 162 tests passing (M4-008)
- CLI: `pr-check` subcommand uploads test results JSON to the Curlew backend (`POST /api/v1/organizations/{org}/results`) and posts a PR status check (`POST /api/v1/organizations/{org}/pr-checks`); `--dry-run` prints JSON payloads without any HTTP traffic; retries connection errors twice with 200 ms backoff; `CURLEW_BACKEND_URL` / `CURLEW_BACKEND_TOKEN` env var configuration; exit 1 on failing tests, exit 2 on backend errors, exit 2 on missing config; `internal/prcheck` package with 36 tests; `testdata/team/mock-backend.sh` stub server checked in (M4-007)
- Backend: scheduled run trigger service (`src/ApiTool.Backend`): `POST /api/v1/organizations/{orgId}/schedules` creates a named schedule with a cron expression and collection reference (201 with computed `next_run_at`), `GET .../schedules` lists all schedules for org members, `GET .../schedules/{name}` returns a single schedule, `POST .../schedules/{name}/run-now` enqueues an immediate run (202 with `run_id` and `queued` status), `GET .../schedules/{name}/runs` lists runs newest-first; `Schedule` and `ScheduledRun` EF entities with migration `20260416061847_AddSchedules`; Cronos NuGet dependency for cron expression parsing; `SchedulerHost` `IHostedService` polls every 30 s for due schedules, enqueues run records, updates `last_run_at`/`next_run_at`, and logs `"scheduler tick N schedules due"` at info level; `ISchedulerEnqueuer` seam for testability; org-scoped RBAC (admins create/trigger, members read, 403 for non-members); 400 for malformed/null cron (`invalid_cron`), 409 for duplicate schedule name in same org (`schedule_name_taken`); Swagger exposes `CreateScheduleRequest` schema with `cron` field; 41-test Schedules suite; overall coverage 93.3% (M4-006)
- Web: team test results dashboard page (`web/src/routes/(app)/org/[slug]/results`): SvelteKit 2.x + Svelte 4 + Tailwind project bootstrapped; `+page.server.ts` loader resolves org slug → id, enforces team-tier via `RequireTeamTier` guard, fetches results from `GET /api/v1/organizations/{orgId}/results?limit=100`, derives stats (total runs, pass/fail, pass rate, avg duration) and trend buckets in-process; `SummaryCards`, `TrendChart` (inline SVG), `RecentRunsTable`, `TimeRangePicker`, `EmptyState`, `ErrorState` components; `data-testid` attributes on every observable element; `RequireTeamTier` redirects non-team-tier orgs to `/org/[slug]?toast=team_tier_required`; `?range=24h|7d|30d|all` filter applied before stats; error state with retry button on backend 5xx; `web/src/lib/api/{client,organizations,results}.ts` typed fetch wrapper; `web/src/lib/results/stats.ts` pure stat helpers; 43 Vitest unit tests; Playwright E2E spec at `web/tests/e2e/org-results.spec.ts` (6 tests); `docker-compose.test.yml` + `scripts/test-stack.sh` + `scripts/seed-test-data.sh` docker harness; 5 seed fixture files; `web/README.md` with local E2E setup guide (M4-005)
- Backend: test results ingestion API (`src/ApiTool.Backend`): `POST /api/v1/organizations/{orgId}/results` ingests a JUnit-like JSON payload (202 Accepted), `GET .../results` lists results newest-first with pagination, `GET /api/v1/results/{resultId}` returns full per-test detail; `Result`/`ResultItem` EF entities with EF migration `AddResults`; org-scoped RBAC (403 for non-members); 5 MB Content-Length cap (413); schema validation with JSON-pointer field error (400); fixed-window rate limiter 60 req/min/org via `Microsoft.AspNetCore.RateLimiting`; `IResultIngestedNotifier` seam for M4-008 events; snake_case wire format; Swagger annotations on all three endpoints; testdata fixture at `testdata/backend/sample-result-upload.json`; 91-test xUnit suite (M4-004)
- Backend: organization + seat RBAC data model and service (`src/ApiTool.Backend`): ASP.NET Core 9 minimal API with EF Core 9 SQLite; `organizations`, `organization_members`, `organization_invitations`, and `organization_audit_log` tables created via EF migration `20260415131819_InitialOrganizationsRbac`; JWT bearer authentication with user upsert on first authenticated request; `GET /api/v1/organizations`, `POST /api/v1/organizations`, `GET /api/v1/organizations/{id}` endpoints enforcing slug validation (lowercase, hyphens, 2–100 chars), owner role assignment, 404-for-non-member semantics, and 409 on duplicate slug; snake_case JSON serialization via `JsonNamingPolicy.SnakeCaseLower`; Swagger UI at `/swagger` in Development; 38-test xUnit suite using EF Core InMemory provider via `WebApplicationFactory`; `scripts/test-token.sh` helper for issuing dev JWTs from the command line (M4-003)
- CLI consumes shared vault template at runtime: `CURLEW_TEAM_CONFIG=path/to/template.yaml` loads a shared vault configuration template; `{{secrets.ALIAS}}` placeholders in collections are resolved through the template for the environment named by `--env`; `CURLEW_VAULT_STUB=1` enables an in-memory stub provider so teams can test without real AWS/Azure credentials; resolved secrets are injected as a dedicated `secrets.*` namespace that is distinct from regular variables; free/solo/professional tiers are blocked by the `shared_vault_templates` feature gate (Team tier required, exit code 6); missing `--env` flag when `{{secrets.X}}` is used returns a structured error (exit 3); unknown aliases fail before any HTTP traffic is dispatched; `Resolved N secrets from shared template (<env>)` is printed to stderr once per run; secrets are never serialized into stdout formats (JSON/TAP/JUnit/HTML); `internal/vault/teamtemplate.SecretsResolver` provides per-run in-memory caching so the vault provider is queried at most once per invocation (M4-002)
- Shared vault configuration template format (`team_secrets.vault_configs`): `curlew validate` now recognises files with a top-level `team_secrets:` key and dispatches them to a new `internal/vault/teamtemplate` package; supported providers are `aws-secrets-manager` (requires `region`) and `azure-key-vault` (requires `vault_name`); invalid templates exit 2 with key-path error messages (e.g. `team_secrets.vault_configs.production.provider: unknown provider 'foo'`); valid templates print a one-line summary (`OK: shared vault template valid (N environments, M secrets)`); duplicate key aliases within the same environment are detected during YAML parsing and reported as validation errors; `internal/validator` gains `ValidateAuto` dispatcher and `ResultKind` enum; `--help` mentions shared vault configuration template support (M4-001)
- OpenAPI import enriched with headers, request bodies, and status assertions: header parameters (`in: header`) become `headers: { X-API-Key: '{{x_api_key}}' }` with snake_case variable names; query parameters (`in: query`) are appended to the URL as `?limit={{limit}}`; shared parameters across operations produce a single deduplicated collection variable; `requestBody` with `application/json` schema generates type-appropriate placeholder values (`string`, `0`, `false`, nested objects/arrays) and prefers explicit `example`/`examples` values; recursive `$ref` cycles are detected during body generation and a warning is written to stderr (warn-once per ref name); response codes are aggregated into `assertions: { status: [200, 404] }` (numeric codes only; `default` and `2XX` patterns ignored); `internal/openapi/schema.go` adds `schemaWalker` for type-aware placeholder synthesis; `importWithWarn` test seam captures warnings without OS pipe complexity (M3-006)
- `curlew import openapi <spec-path> [--output path]` imports an OpenAPI 3.0/3.1 spec into a minimal collection skeleton: one request per operation with method, URL, name (from `operationId` or synthesised `method_path_by_param`), and a `variables: { base_url: ... }` entry derived from `servers[0]`; path parameters `{petId}` are rewritten to `{{petId}}`; deterministic output order (paths sorted lexicographically, methods iterated GET/POST/PUT/PATCH/DELETE/HEAD/OPTIONS/TRACE); `--output` writes to file with mode 0644, otherwise stdout; `openapi_import` registered as a Professional-tier feature gate (exit code 6 at Free/Solo tier); invalid or missing spec files return exit code 3 with a structured parse error; `internal/openapi` package backed by `github.com/getkin/kin-openapi/openapi3` (M3-005)
- JSON Schema response body validation: `assertions: { schema: ./path/to/schema.json }` validates the response body against a JSON Schema (Draft 2020-12) at request time; schema path is relative to the collection file directory; compiled once at parse time and reused across all iterations (parallel and data-driven safe); missing schema file returns a structured parse error with `ErrSchemaFileNotFound`; invalid JSON Schema returns a structured parse error with `ErrSchemaInvalid`; validation failures produce one `Result` per leaf schema error with JSONPath-style location, expected type/constraint, and actual value; `schema_validation` registered as Professional-tier feature gate (exit code 6 at Free/Solo tier, checked before any schema file is opened); `SchemaGate` callback added to `parser.ParseOptions` (mirrors `IncludeGate` pattern); `CompiledSchema *assertion.CompiledSchema` field added to `parser.Assertions`; `Schema *CompiledSchema` field added to `assertion.EvalInput`; `CheckSchema` called in `Evaluate` after body assertions; `internal/assertion` package gains `CompileSchemaFile`, `CheckSchema`, `CompiledSchema` types backed by `github.com/santhosh-tekuri/jsonschema/v6`; JSON Schema collection spec updated with `schema` string property in assertions (M3-004)
- `include:` directive for collection composition: top-level `include: [./shared/auth.yaml, ./shared/common.yaml]` splices child collection setup/requests/teardown items into the parent in declaration order; parent variables form a read-only snapshot that each child inherits; child variable declarations override locally but never leak back to the parent scope; transitive includes (A → B → C) accumulate overrides at each level; circular include detection (A → B → A) returns a structured error naming both files; relative include paths resolve relative to the including file's directory; missing include files return a structured error with the line number of the failing entry; `include_directive` registered as Professional-tier feature gate (exit code 6 at Free/Solo tier, checked before any child file is opened); `ParseFileWithOptions` added to `internal/parser` with opt-in `IncludeGate` callback; `CollectPaths` in `internal/watch` updated to forward parse options; JSON Schema updated with `include` array-of-strings property (M3-003)
- Glob pattern discovery for `curlew run`: passing a glob pattern (containing `*`, `?`, or `[`) as the collection argument expands all matching YAML files under the working directory; results are sorted deterministically; `.curlewignore` rules are honored; patterns containing `../` or absolute paths are rejected; zero matches returns exit code 2; a single aggregated terminal summary is printed after all collections run; `--format json` produces a single `MultiJSONOutput` document with a `collections` array; `test_discovery` registered as Professional-tier feature gate (exit code 6 at Free/Solo tier, checked before any file I/O); `CURLEW_TIER` env var allows tier override for testing and smoke scripts; `internal/discovery` package provides `Expand`, `IsGlob`, `LoadIgnore` and a hand-rolled `**`-aware matcher (M3-002)
- Global `rate_limit_rps` throttle across all requests: top-level `rate_limit_rps: N` field on a collection gates all HTTP dispatches through a shared token-bucket limiter; sequential and `--parallel` workers share the same bucket so global throughput never exceeds N requests/second; `rate_limit_rps: 0` or unset disables throttling; negative values return a structured validation error with line number; `rate_limit_global` registered as a Professional-tier feature gate (exit code 6 at Free/Solo tier); context cancellation propagates promptly through limiter wait; `internal/ratelimit` package provides nil-safe, mutex-protected token-bucket used by runner, datadriven, and websocket paths (M3-001)
- WebSocket reconnection, heartbeat, and parallel connection support: `reconnect: { enabled: true, max_attempts: N, backoff: exponential }` auto-reconnects on connection-level read failures during expect steps (assertion failures, timeouts, and context cancellation are not retried); `heartbeat: { enabled: true, interval_ms: N }` sends periodic ping control frames and fails the test with `ErrHeartbeatTimeout` if no pong is received within one interval; WebSocket requests dispatch through `parallel.WebSocketFunc` injection point when `--parallel` is used, so independent connections run concurrently while steps within each connection remain sequential; URL scheme auto-detection sets `protocol: websocket` automatically when URL starts with `ws://` or `wss://`; new sentinel errors `ErrReconnectExhausted`, `ErrHeartbeatTimeout`, `ErrHeartbeatFailed`; `Conn` interface extended with `SetPongHandler`; `buildWebSocketFunc`/`buildWSOutcome` helpers in runner for result synthesis (M2-034)
- HTML report data-driven and parallel visualization: per-group iteration summary card (total iterations, pass rate, average duration, throughput); filterable iteration table with status filter buttons (All/Passed/Failed) using inline JavaScript; inline SVG timeline chart showing iteration durations scaled to max (self-contained, no external dependencies); parallel wave diagram with wave-by-wave breakdown of request items, duration, and status; speedup factor and max parallelism summary cards; `BuildDataDrivenGroups`, `BuildWaves`, `ComputeSpeedup` helpers in `internal/output`; `IterationData map[string]string` added to `runner.RequestResult` to propagate row data through to the report (M2-028)
- WebSocket message buffering, expect patterns, and variable extraction: FIFO message buffer (pre-buffered messages matched instantly before waiting); `any_of:` multi-pattern matching passes when any pattern matches; `count: N` collects N matching messages and extracts variables as JSON arrays; buffer warning issued once when buffer exceeds 100 messages; `message_template:` loads external JSON file with `{{var}}` interpolation at send time; `message_raw:` sends plain text verbatim without JSON serialization; extracted variables available in subsequent steps via child scope; `internal/websocket/templates` subpackage owns template file loading (M2-033)
- GraphQL error handling and partial success configuration: three error handling modes (`fail` default, `warn`, `ignore`) for GraphQL responses; global configuration via `defaults.graphql.error_handling.partial_success` in `curlew.yaml`; per-request `graphql.error_handling` override with per-request taking precedence over global; full-failure responses (errors present, data null) always fail regardless of mode; partial success (data + errors) behavior controlled by effective mode; `Warnings []string` field on `RequestResult` surfaces GraphQL warnings in terminal and JSON output; JSONPath assertions on `$.errors[0].extensions.code`, `$.errors[0].message`, and nested error fields work without changes; malformed GraphQL response bodies return clear errors; `internal/parallel.RequestOutcome` extended with `Warnings` for propagation through parallel and data-driven execution paths (M2-031)
- GraphQL external query files and fragment support: `graphql.query_file: path/to/query.graphql` loads the query from an external `.graphql` file; `graphql.fragments: [path1, path2]` loads fragment files and concatenates them onto the query with automatic topological ordering so dependencies appear before dependents; circular fragment dependencies return a clear error with the offending chain; duplicate fragment names across files are rejected; missing files produce errors that include the expected path; `{{var}}` placeholders inside loaded files are interpolated at run time via the existing variable pipeline; loaded file paths are tracked in `Collection.ExternalFiles` so watch mode reruns on edits; `graphql.query` and `graphql.query_file` are mutually exclusive; new `internal/graphql/files` subpackage owns loading, fragment parsing, topological sort, and cycle detection (M2-030)
- WebSocket protocol adapter: `protocol: websocket` with `websocket.steps` containing `send` (JSON `message:` or raw `message_raw:`), `expect` (JSONPath assertions on received `message:`, `timeout_ms`, `extract:`), `wait` (`duration_ms`), and `close` (`code`, `reason`) actions; establishes a real WebSocket connection via HTTP upgrade using `gorilla/websocket`; variable interpolation in URL/headers/message fields; variables extracted in `expect` steps are available to subsequent steps and later collection requests; Professional-tier feature gate (exit code 6 at Free tier); each WebSocket request counts as 1 request against the guard rail regardless of step count; `internal/websocket/` package with `Dialer` interface for test injection and `DefaultDialer` backed by gorilla/websocket (M2-032)
- GraphQL protocol adapter: `protocol: graphql` with `graphql.query` and `graphql.variables` fields; transforms into HTTP POST with JSON body `{"query":"...","variables":{...}}`; Content-Type set to application/json; `error_handling: fail` (default) fails on `$.errors`, `error_handling: warn` logs warning without failing; variable interpolation in query and variables; JSONPath assertions on `$.data` and `$.errors` work identically to HTTP; Professional-tier feature gate (exit code 6 at Free tier); `internal/graphql/` package with `BuildRequest`, `CheckResponse`, `ParseErrorHandling` (M2-029)
- HTML report generation: `--format html --report report.html` generates a self-contained HTML file with embedded CSS and minimal JavaScript; summary dashboard shows total, passed, failed, skipped, and duration; per-request rows show name, status, method, URL, status code, and timing; collapsible assertion details with expected vs actual for failed assertions; XSS-safe via Go `html/template` auto-escaping; `--format html` requires `--report` flag with clear error message; Professional-tier feature gate (exit code 6 at Free tier) (M2-027)
- Retry output and integration with parallel/data-driven execution: `AttemptDetail` struct records per-attempt status code, duration, delay, and error; parallel executor wraps requests with `retry.ExecuteWithRetry` via `RetryConfigFunc` callback (retries within wave, not blocking other wave members); data-driven per-iteration retry with `fail_fast` support; verbose terminal output shows attempt number, status, duration, and delay for each retry attempt; JSON output includes `attempt_details` array with `attempt`, `status_code`, `duration_ms`, `delay_ms`, `error` fields; TAP output adds `# retry:` diagnostic comments with per-attempt details; `AttemptDetails` propagated through runner, parallel executor, and data-driven paths (M2-025)
- Advanced retry trigger conditions and method restrictions: `retry_on.status_ranges` matches status codes against ranges (e.g., `500-599`); `do_not_retry_on.status_codes` and `do_not_retry_on.status_ranges` exclude specific codes/ranges from retry (exclusion takes precedence); `retry_on.methods` restricts which HTTP methods are retried (defaults to idempotent methods GET, HEAD, OPTIONS, TRACE); non-idempotent methods (POST, PATCH) produce warnings when retried; `retry_on.network_errors: true` retries on connection failures; `retry_on.timeouts: true` retries on request timeouts; combined condition logic: `(status OR network OR timeout) AND method AND NOT exclusion`; `do_not_retry_on.methods` excludes methods from retry; `ShouldRetry` function with `ClassifyForRetry` bridge replaces hardcoded `IsRetriable` in retry loop; `Outcome.Warnings` field surfaces non-idempotent method warnings (M2-024)
- Backoff strategies (exponential, linear, constant) with jitter: `backoff_strategy` field supports `exponential` (default), `linear`, and `constant` strategies; `max_delay_ms` caps delay at configurable maximum (default 30s); `jitter: true` with `jitter_factor` adds randomized variation to delays; injectable `JitterFunc` for deterministic testing; `custom_backoff` feature gate requires Professional tier for linear/constant strategies; Solo tier defaults to exponential-only (M2-023)
- Data-driven parallel execution and large dataset support: `parallel: true` runs data-driven iterations concurrently (up to 20 workers); `rate_limit_rps` throttles requests per second via token-bucket rate limiter; large datasets (>10,000 rows) trigger confirmation prompt with performance/storage estimates (`--confirm-large-dataset` flag to bypass); chunked processing (1,000-row chunks) reduces peak memory for large parallel runs; `store_results: summary` stores only pass/fail/timing per iteration, `store_results: failed_only` stores full details only for failed iterations; data-driven requests treated as atomic units in parallel dependency graphs; accumulated extraction variables available in index order after all iterations complete (M2-022)
- Data-driven output formatting: compact terminal output with progress summary for >= 10 iterations; verbose per-iteration terminal output for < 10 iterations; failed iteration details always shown regardless of mode; JSON output includes `data_driven` array with `type`, `total_iterations`, `passed_iterations`, `failed_iterations`, `total_duration_ms`, `average_duration_ms`; TAP output annotates data-driven groups with `# Data-Driven:` comments; data-driven summary shows total, passed, failed, average duration, and failed iteration numbers (M2-021)
- Data-driven YAML support, filtering, and row control: YAML data source loader (`.yaml`/`.yml` auto-detection); filter expressions with comparison operators (`==`, `!=`, `<`, `>`, `<=`, `>=`), string operators (`contains`, `starts_with`, `ends_with`), and boolean logic (`AND`, `OR`, `NOT`); row control with `limit`, `start_row`, `end_row`; `fail_fast: true` stops on first iteration failure; CSV type conversion filters (`|int`, `|float`, `|bool`); `LoadWithControls` entry point combines loading with filter/range/limit; negative start_row/end_row bounds validated (M2-020)
- JUnit XML output format: `--format junit` produces standard JUnit XML with `<testsuites>`, `<testsuite>`, `<testcase>` elements; `<failure>` for assertion failures, `<error>` for network/execution errors, `<skipped>` for skipped requests; `time` attribute reflects duration in seconds; `--report <file>` writes XML to file instead of stdout; early bailout error paths produce valid JUnit XML with `<error>` element; Professional-tier feature gate (exit code 6 at Free tier) (M2-026)
- Data-driven testing with CSV and JSON data sources: `data_driven: { source: ./data.csv }` on a request runs it once per row; CSV column values and JSON object fields available as `{{column}}` variables; special iteration variables `{{_index}}` (0-based), `{{_iteration}}` (1-based), `{{_total}}`, `{{_row_number}}`; extracted variables accumulate as JSON arrays across iterations; empty data files skip with warning (exit 0); missing files produce clear error with checked paths; Professional-tier feature gate (exit code 6 at Free tier); `internal/datadriven` package with CSV/JSON loaders, iteration execution, and special variable injection (M2-019)
- Parallel execution output formatting: terminal output groups requests by wave with wave headers (`Wave N (M concurrent):`); JSON output includes `wave_index` per request and `parallel_execution` block with `wave_count`, `max_parallelism`, and `speedup_factor`; TAP output adds `# Wave N` comments at wave boundaries; terminal summary shows waves, max parallelism, and speedup factor; `FormatWaves` dry-run output enhanced with max parallelism and expected speedup (M2-018)
- Parallel execution edge cases: variable-specific skip messages when dependencies fail (e.g., "depends on 'user_id' from Request A, which failed"); auth profile variables excluded from dependency creation; external file extractions included in dependency analysis; impact analysis output showing how many requests skipped per failed dependency; default values do not prevent skip when producer fails; nested variable resolution depth > 10 produces warning; skip reasons displayed in terminal and JSON output (M2-017)
- Parallel request execution with wave-based scheduling: `--parallel` flag executes independent requests concurrently using goroutines; wave-based scheduling ensures dependent requests run in correct order; per-request scope snapshots prevent data races; variable extraction propagates between waves sequentially; setup/teardown phases remain sequential; guard rail limit enforced across parallel waves; Professional-tier feature gate (exit code 6 at Free tier); `internal/requtil` package extracts shared request helpers (M2-016)
- Dependency analysis algorithm: `internal/parallel/` package implementing 6-phase dependency analysis (variable scanning, graph building, Kahn's topological sort for cycle detection, variable collision detection, wave computation); `--show-dependencies` flag outputs DOT-format dependency graph; `--show-dependencies --dry-run` shows execution wave groupings; `parallel_execution` registered as Professional-tier feature gate; pre-execution variables and dynamic functions excluded from dependency tracking (M2-015)
- Retry configuration precedence chain: builtin defaults < global (`curlew.yaml` `defaults.retry:`) < collection < section (setup/requests/teardown) < request; deep merge rules per spec (scalars replaced, arrays replaced entirely, objects deep-merged); `Section` type with dual YAML form support (array and object); `FullConfig` pointer-based types for nil-means-unset merge semantics; `internal/config` parses `defaults.retry` from project config (M2-014)
- Basic retry logic with configurable max attempts: `retry: { enabled: true, max_attempts: 3 }` on collections or individual requests; retries transient failures (429, 502, 503, 504, network errors) with exponential backoff and Retry-After header support; non-idempotent methods (POST, PATCH, PUT, DELETE) excluded by default; per-request retry override; retry count shown in terminal, JSON, and TAP output; Solo-tier feature gate (M2-013)
- Watch mode terminal UX: re-run separator with timestamp and trigger file name, running totals across re-runs, "Watching for changes..." status with file list, `--format json` suppresses all terminal decorations, `--clear` flag clears terminal between re-runs, graceful parse error recovery (M2-012)
- `curlew watch <file>` command: monitors collection files and related resources (external requests, environments, .env, curlew.yaml) for changes, automatically re-running the collection; 500ms debounce prevents rapid re-runs; `--env` and all run flags preserved across re-runs; Ctrl+C for clean exit (M2-011)
- `internal/watch` package with `Paths`, `CollectPaths`, `Config`, and `Run` for file system monitoring via `fsnotify` (M2-011)
- `ExternalFiles` field on `parser.Collection` to track resolved external file paths (M2-011)
- Auth profile token caching and refresh: `cache_ttl` field prevents re-executing auth profiles within TTL (file-based cache in `.curlew/cache/` with atomic writes and XOR+base64 obfuscation); `refresh_on_failure: true` re-executes the auth profile and retries the request once on 401 responses; `CacheStore` interface enables test injection; `.curlew/cache/` excluded from git (M2-009)
- Per-request auth profile reference: `auth: <profile_name>` field on individual requests injects the resolved Bearer token as an `Authorization` header; explicit `Authorization` header takes precedence (case-insensitive); missing or unresolved profile returns a clear error listing available profiles (M2-010)
- Auth profile configuration and execution: `auth_profiles:` block in `curlew.yaml` with `type: dynamic` support; login collection runs before main requests; extracted variables injected as pre-execution scope and auto-marked sensitive; auth profile failure returns exit code 5 and skips all main requests; auth requests excluded from 1,000-request guard rail; Solo-tier feature gate (M2-008)
- Vault variable precedence integration: vault secrets resolved at precedence level 6 (above `from_command`, below collection variables); CLI `--var` overrides win; fetch failures emit `*errors.Structured` with `CategoryConfig`; all vault variables auto-sensitive; `BulkFetch` path deduplication minimises vault API calls (M2-007)
- GCP Secret Manager and 1Password CLI providers: fetches secrets via `gcloud secrets versions access` and `op item get`/`op read`; structured JSON field extraction via `key#field` syntax; clear install-hint errors when CLI not installed; all five built-in providers (AWS, Azure, HashiCorp, GCP, 1Password) registered and recognized by `curlew vault list` (M2-006)
- Azure Key Vault provider: fetches secrets via `az keyvault secret show` with structured extraction (`key#field` syntax), auth error classification with Azure docs link, credential validation via `az account show` (M2-005)
- HashiCorp Vault provider: fetches secrets via `vault kv get` with token and AppRole authentication, `.data.data` JSON extraction, AppRole token caching across calls, network and auth error classification with helpful hints (M2-005)
- Vault provider interface and AWS Secrets Manager provider: `Provider` interface (`Fetch`, `BulkFetch`, `Name`, `ValidateConfig`) for extensible secret backends; AWS Secrets Manager implementation via CLI shell-out with POSIX shell quoting; TTL-based cache with invalidation; JSON field extraction (`prod/db#password` syntax); resolver orchestrator with path deduplication; secrets injected at variable precedence 6 (between `from_command` and collection values); clear auth error messages with AWS CLI docs link; Solo-tier feature gate (M2-004)
- `curlew vault` CLI subcommand: feature-gated at Free tier (exit code 6 with gate message in terminal and `--format json`); at Solo tier, `vault list` displays configured vault provider profiles with key counts and name-to-path mappings; supports `--format json` for structured output; handles no-profiles and invalid-config edge cases; help text updated with `vault list` subcommand (M2-003)
- Vault provider profile configuration: `secrets:` block in `curlew.yaml` with support for AWS Secrets Manager, Azure Key Vault, HashiCorp Vault, GCP Secret Manager, and 1Password; structured extraction syntax (`key#field`); provider-specific validation (region, vault_name, address/auth, project); all vault variables auto-marked sensitive; Solo-tier feature gate (exit code 6 at Free tier) (M2-002)
- `from_command` variable source: resolve variables by executing shell commands with stdout capture, caching (configurable TTL), sensitivity support, and Solo-tier feature gating; object-form YAML syntax with `from_command`, `sensitive`, and `cache` fields; precedence level 5 (between dotenv and collection values); CLI/env-var overrides skip command execution; exit code 6 at Free tier with feature gate message (M2-001)
- Guard rail: 1,000-request limit per collection run; execution stops at the limit with exit code 2 and a message suggesting to split into smaller collections; counter shared across setup, main, and teardown phases; all output formats (terminal, JSON, TAP) handle the guard rail (M1-029)
- Feature gate framework: `internal/auth` package with configurable tier-based feature gating; `curlew vault` command exits with code 6 and structured gate message (terminal and `--format json`) showing feature name, required tier, register URL, upgrade URL, and workaround; registry pattern for declarative feature-to-tier mapping (M1-028)
- `curlew exec` command: execute a single HTTP request for AI agents and scripts; supports `--stdin` for piped JSON input, inline URL, `--dry-run`, `--log <file>` for JSONL logging, `--non-interactive`, `--format json`, `--var`/`--env-var` interpolation (M1-026)
- `internal/output.AppendJSONL`: structured JSONL log entry writer with create/append semantics (M1-026)
- `curlew info` command: shows project metadata (collections, environments, root path) in human-readable or `--format json` output; returns exit 5 with clear error when run outside a project directory (M1-027)
- `curlew schema` command: outputs the JSON Schema for the collection YAML format; supports `--format json` (default) (M1-027)
- `internal/schema` package: embedded JSON Schema for the collection format via `go:embed` (M1-027)
- `internal/config.ListCollections`: discovers `.yaml`/`.yml` files in `collections/` subdirectory (M1-027)
- `curlew validate <file>` command: validates collection files without executing HTTP requests; reports all issues (errors and warnings) in a single pass; supports `--format json` for structured output and glob patterns for multiple files (M1-025)
- `internal/validator` package with `Validate(path, knownVars)` returning all structural issues; `Severity`, `Issue`, and `Result` types (M1-025)
- `curlew init [dir]` command: scaffolds a new project with `curlew.yaml`, `.gitignore`, `.env.example`, `environments/dev.yaml`, and a working `collections/sample.yaml`; detects existing projects and returns an error (M1-024)
- `internal/scaffold` package with `Init(Options)` function and `ErrProjectExists` sentinel error (M1-024)
- Sensitive variable detection and redaction: variables named `password`, `token`, `secret`, `api_key`, `credential`, or `authorization` are automatically redacted to `[REDACTED]` in all output (M1-023)
- `!sensitive` YAML tag on collection variables: marks a variable as sensitive regardless of name (M1-023)
- `!sensitive` prefix in `.env` files: `!sensitive KEY=VALUE` marks a dotenv variable sensitive (M1-023)
- `--allow-sensitive` flag: shows sensitive values in plain text instead of redacting them (M1-023)
- `Authorization`, `X-API-Key`, `X-Auth-Token`, `Proxy-Authorization`, and `Cookie` request headers always redacted in verbose output (M1-023)
- `SensitiveSet` type in `internal/variable` with `Add`, `IsSensitive`, `Merge`, `AddHeuristicNames`, `RedactValue`, `RedactHeaders` (M1-023)
- `-v` flag: verbose mode showing request method/URL and response headers per request (M1-022)
- `-vv` flag: debug mode showing full HTTP request/response dump including bodies (truncated at 10KB) (M1-022)
- `-q` / `--quiet` flag: quiet mode showing only the summary line, suppressing per-request output (M1-022)
- Verbosity support in `--format json`: `-v` adds `request_headers`/`response_headers` fields; `-vv` adds `response_body` field (M1-022)
- `Verbosity` type in `internal/output` with `VerbosityQuiet`, `VerbosityDefault`, `VerbosityVerbose`, `VerbosityDebug` constants (M1-022)
- `RequestHeaders` and `RequestBody` fields on `runner.RequestResult` for verbose output (M1-022)
- `--format tap` flag: outputs TAP version 13 format consumable by standard TAP harnesses; `ok`/`not ok` lines, YAML diagnostic blocks for failures, `Bail out!` for pre-execution errors, `1..0` for empty collections (M1-021)
- `internal/output.WriteTAP`, `TAPResult`, `TAPFailure` types for TAP 13 formatting (M1-021)
- `--format json` flag: outputs structured JSON matching the full collection schema (name, status, duration_ms, requests array with method, url, status_code, assertions); errors included in JSON, not stderr (M1-020)
- `internal/output.WriteJSON` and JSON schema types (`JSONOutput`, `JSONRequest`, `JSONAssertion`, `JSONError`) (M1-020)
- `Method` and `URL` fields on `runner.RequestResult` populated at all execution sites (M1-020)
- ANSI color support in terminal output: green checkmark for pass, red X for fail, bold collection header, bold+cyan section headers, gray separator and duration in summary (M1-019)
- `--no-color` flag to disable ANSI codes; `NO_COLOR` env var support per the no-color.org spec; automatic TTY detection strips codes when piped (M1-019)
- `Printer` struct with `NewPrinter(w, color)` replacing package-level functions; `IsTerminal` helper for TTY detection (M1-019)
- Dynamic variable functions: `{{$uuid}}`, `{{$timestamp}}`, `{{$isoTimestamp}}`, `{{$timestampMs}}`, `{{$guid}}`, `{{$randomInt}}`, `{{$randomFloat}}`, `{{$randomBoolean}}`, `{{$randomString}}`, `{{$randomHex}}`, `{{$randomEmail}}`, `{{$randomName}}`, `{{$randomFirstName}}`, `{{$randomLastName}}`, `{{$randomColor}}` interpolated at precedence level 1 (M1-018)
- `--seed <number>` flag for deterministic random variable functions; same seed produces identical values across runs (M1-018)
- `curlew.yaml` project-level config: global variables at precedence 2, `project_name` field, auto-discovered by walking up from the collection directory (M1-017)
- `.env` loaded from project root when `curlew.yaml` is found, falling back to collection directory (M1-017)
- `setup:` and `teardown:` sections in collections: setup runs before main requests, teardown runs after main unconditionally (M1-016)
- `required: true` on setup items: if a required setup item fails, all main requests are skipped but teardown still runs (M1-016)
- Variables extracted in setup are available to main requests and teardown via shared scope (M1-016)
- Teardown failures are reported in output but excluded from exit code determination (M1-016)
- Setup and teardown section headers printed in terminal output ("Setup:", "Teardown:") (M1-016)
- `Phase` type on `RequestResult` and `TeardownErrors`/`TeardownAssertionErrors` on `Summary` for phase-aware reporting (M1-016)
- External request file references: `path:` field in collection requests loads standalone request YAML files, resolved relative to the collection file's directory (M1-015)
- Circular file reference detection: self-referencing or circular `path:` chains produce clear error messages (M1-015)
- Inline variable overrides at reference site: `variables:` on a `path:` entry override the external request's variables (M1-015)
- Reference site name override: `name:` on a `path:` entry overrides the external file's request name (M1-015)
- Mutual exclusivity validation: `path:` and `request.url` cannot both be specified on the same item (M1-015)
- Request-level variables: `variables:` block on individual requests, scoped per-request (precedence 8), supports interpolation chains with collection-level variables (M1-014)
- `--env-var VAR_NAME` flag to import OS environment variables (precedence 9), supports mapped form `--env-var VAR=$OS_VAR`, repeatable (M1-014)
- Full variable precedence chain: CLI (10) > env-var (9) > request (8) > extract (7) > .env (4) > env file (3) > collection (2) > project config (1) (M1-014)
- `VarSources` struct replaces positional parameters in `runner.Run` for cleaner API (M1-014)
- `.env` file auto-loading for local secrets: `KEY=VALUE` format, comments, quoted values, optional (no error if missing); precedence: environment < .env < collection < CLI `--var` (M1-013)
- `--env <name>` flag to load environment files from `environments/<name>.yaml` with variable precedence: environment < collection < CLI `--var` (M1-012)
- Environment file parsing with nested variable flattening using underscore-separated keys (M1-012)
- Missing environment error lists available environments; invalid YAML reports file path (M1-012)
- Both `.yaml` and `.yml` extensions accepted for environment files (M1-012)
- CLI `--var key=value` flag for variable overrides: repeatable, splits on first `=`, overrides collection-level variables at highest precedence (M1-011)
- Variable extraction from responses: `extract:` field with JSONPath expressions captures values for subsequent requests (M1-010)
- Extracted variables override collection-level variables (higher precedence) (M1-010)
- Extraction error handling: non-JSON body and path-not-found produce structured errors (M1-010)
- `Scope.Set` method for injecting variables after initial resolution (M1-010)
- Collection-level variables with `{{var}}` interpolation in URL, headers, query params, and body (M1-009)
- Variable chain resolution with circular reference detection and depth limiting (max 10 levels) (M1-009)
- Undefined variable errors with hint listing available variables (M1-009)
- Exit code 5 for variable resolution failures (circular, depth, undefined) (M1-009)
- `internal/variable` package for variable interpolation engine (M1-009)
- Structured error messages with `[ERROR] file:line — message` format and actionable hints (M1-008)
- Network error classification: DNS, connection refused, TLS, and timeout errors with distinct messages and hints (M1-008)
- Missing required field validation at parse time with file path and field name in error (M1-008)
- `internal/errors` package for structured error types, network classification, and consistent formatting (M1-008)
- Full body assertion operator set: `matches`, `contains`, `contains_all`, `length`, `greater_than`, `less_than`, `greater_than_or_equal`, `less_than_or_equal`, `approximately`, `in_range` (M1-007)
- Numeric string type coercion: `"42"` equals `42` for `equals` and numeric comparison operators (M1-007)
- Header assertions: `assertions.headers` supports `equals`, `exists`, and `matches` (regex) operators with case-insensitive name matching (M1-006)
- Timing assertions: `assertions.timing.max_duration_ms` validates response time against threshold (M1-006)
- `EvalInput` struct for assertion evaluation, replacing positional parameters (M1-006)
- Response headers captured in HTTP executor for assertion evaluation (M1-006)
- Body assertions with JSONPath: `assertions.body` supports `equals`, `exists`, `not_exists`, and `type` operators (M1-005)
- Minimal JSONPath evaluator: dot notation and array indexing (`$.data.items[0].name`) (M1-005)
- `internal/jsonpath` package for JSONPath evaluation (M1-005)
- Response body capture in HTTP executor for assertion evaluation (M1-005)
- Status code assertions: `assertions.status` supports single int or list of ints (M1-004)
- Pass/fail indicators (✓/✗) in request output (M1-004)
- Exit code 1 for assertion failures, distinct from exit code 4 for network errors (M1-004)
- `internal/assertion` package for assertion evaluation logic (M1-004)
- Sequential multi-request execution with per-request output and summary (M1-003)
- `options.stop_on_failure` collection option: skip remaining requests after first failure (M1-003)
- Duration tracking in execution summary (M1-003)
- Warning for empty requests collections with exit code 0 (M1-003)
- `internal/runner` package for testable request orchestration (M1-003)
- All HTTP methods: POST, PUT, DELETE, PATCH, HEAD, OPTIONS with method validation and normalization (M1-002)
- JSON body serialization: YAML map bodies auto-serialized as JSON with Content-Type header (M1-002)
- String body passthrough: string bodies sent as-is without Content-Type (M1-002)
- Query parameter handling: `query:` map appended to URL with proper URL-encoding (M1-002)
- Method defaulting: requests with no method default to GET (M1-002)
- `curlew run <file>` command: parse YAML collection files, execute GET requests, display name/status/duration (M1-001)
- YAML collection parser with sentinel errors for file-not-found, invalid YAML, and empty collection (M1-001)
- HTTP executor with network error wrapping and response body drain (M1-001)
- Terminal output formatter for results, headers, summaries, and errors (M1-001)
- Sample collection file `sample/hello.yaml` (M1-001)
- Exit codes: 0 (success), 1 (usage error), 3 (file/parse error), 4 (network error) (M1-001)
- Project scaffolding: Go module, minimal CLI binary with help and version
- Development workflow: task management, slash commands, skills
- Linter configuration (golangci-lint) and smoke test script
- Project documentation moved to `docs/`
- CLAUDE.md with development standards and conventions

## [0.0.0] — Initial development baseline
