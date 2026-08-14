# Product Roadmap — from green build to shipped tool

**Written:** 2026-08-14 · **Milestones:** M25–M29 · **Status of M1–M24:** complete

This is the roadmap for the *product*. `ROADMAP.md` at the repository root is a
different document — it hardens the *workflow* (how tasks get planned, executed
and verified). Neither supersedes the other.

## Where we actually are

`main` is at `e252b50`. The gate is green: `./scripts/ci-local.sh` passes at
86.1% coverage across 52 packages, all four dogfood harnesses included. Both
debt registers — `docs/table-execution-baseline.txt` and
`testapi/harness/redaction-known-leaks.txt` — are empty, and both are
shrink-only, so neither can quietly refill. 231 tasks across M1–M24 are done,
which is the fourth time this backlog has been emptied.

That is a good engineering position and a poor product position. The gap
between them is the whole of this roadmap:

| Question a new user would ask | Answer today |
|---|---|
| Where do I download it? | Nowhere. No tag, no release, no artifact has ever been built. |
| What version am I running? | `0.1.0-dev`, on every machine, forever. |
| Does the release build work? | Unknown. `.goreleaser.yaml` has never been executed; `goreleaser` is not installed and the gate never invokes it. |
| Is the agent skill accurate? | No. It documents a licensing system that was deleted, two exit codes the binary cannot return, and a command that does not exist. |
| Is the manual accurate? | Yes — checked. Removed features appear only in explicit "not in this CLI" and migration sections. |
| What is `src/`? | 139,627 lines of C# — *larger than the entire Go CLI* — that the shipped product does not call. Undecided. |
| How do I contribute? | No `CONTRIBUTING.md`, no issue or PR templates, no `SECURITY.md`. |
| Does anything run on push? | No. All eight workflows are `workflow_dispatch`-only. |

## The through-line

Four campaigns have now run on this codebase — feature gating stripped, backend
stripped, post-strip drift closed, documentation tables executed. Each one
found real defects, and each one worked the same way: **inventory what has not
been checked, make that inventory a build-failing register, then empty it.**

Eight defects fell out of the table campaign alone, including a run reporting
`3 passed, 0 failed` and exiting 0 on a suite with a failing assertion.

The method works. It has simply never been pointed at the product surface —
the release, the skill, the front door. That is M25–M28. M29 is the one item
that is not an engineering decision.

---

## M25 — A deliverable that exists

*Capability: `release_pipeline`*

The single largest gap. `.goreleaser.yaml` is 94 careful lines — static builds,
`-X main.version` injection, LICENSE and NOTICE carried into every archive per
Apache-2.0 §4(a) and §4(d), draft releases so a human reviews before publish.
None of it has ever run.

A release config that has never been executed is not a release pipeline; it is
a document about one. And a release is the one build that must not be the first
place a failure is noticed.

- **M25-001** — Exercise the release build in the gate. `goreleaser check` plus
  a single-target snapshot, wired into `ci-local.sh`, so a broken release config
  fails on an ordinary day rather than on tag day.
- **M25-002** — Cut `v0.1.0`. Six targets, real archives, real checksums.
  Verified by extracting each archive and running the binary inside it, which is
  the only check that proves `-X main.version` reached the artifact rather than
  the source tree.
- **M25-003** — Make every install path in the README executable, and hold it
  there by a test that runs the documented commands.

**Done when:** a person who has never seen this repository can get a working
`curlew` onto their machine from a URL, and it reports a real version.

## M26 — The agent skill tells the truth

*Capability: `agent_truthfulness`*

`curlew init --skill agent` writes `.claude/skills/curlew/` into a user's own
repository, where Claude Code and GitHub Copilot read it. Three of its files
still describe the five-tier licensing system that was removed:

- `SKILL.md:118` — exit 6, "Feature gate denied … name the required tier"
- `SKILL.md:119` — exit 9, "License grace period expired"
- `exit-codes.md:43-44` — the same two codes
- `failure-playbook.md:84-87` — a full playbook for a license lapse

The binary returns 0–5. There is no exit 6, no exit 9, and no `curlew license`
command. So the file we install into other people's repositories teaches an
agent to diagnose failures that cannot occur and to run a command that does not
exist — and it is the highest-leverage document we ship, because an agent obeys
it literally.

- **M26-001** — Strip the dead licensing surface from the skill and hold its
  exit-code table to the binary's *reachable* exit codes, derived from source
  rather than hand-listed.
- **M26-002** — One exit-code truth across binary, specification, manual and
  skill, including the dead code-6 "feature gate" mapping still sitting in
  `cmd/curlew/discovery_run.go:25`.

**Done when:** every exit code the skill names can actually be produced, proven
by a test that reads the binary rather than the prose.

## M27 — The defects tables cannot find

*Capability: `defect_hunt`*

The table campaign is exhausted, which means the *cheapest* defect-finding
method is used up — not that the defects are gone. Two known blind spots:

**Prose claims that are not names.** Defect 8 — the cross-format correlation
ID — sat beside a table that *was* executed, because the promise it broke lived
in a sentence. Every register so far keys on names in rows. Sentences are the
larger surface and the unswept one.

**Structurally invalid input.** There are zero fuzz targets and zero
property-based tests in the repository. For a YAML parser, a CEL evaluator, a
JSONPath implementation and a variable interpolator, that is the gap where the
remaining crashes live.

- **M27-001** — Open the prose-claim register: same shrink-only contract as the
  table register, for checkable claims stated in sentences.
- **M27-002** — Fuzz the parser, the expression evaluator, JSONPath, and
  variable interpolation. Native Go fuzzing, corpora committed, findings become
  regression tests.
- **M27-003** — Mudflat Phase 4. Phases 1–3 are implemented and found sixteen
  defects; the specification's phases are now exhausted, so the next dogfooding
  surface has to be designed rather than executed.

**Done when:** a fresh sweep of a new kind finds nothing — and the sweep is
committed so it keeps finding nothing.

## M28 — A repository a stranger can respect

*Capability: `repo_front_door`*

- **M28-001** — Decide what `src/` and `web/` are. 139,627 lines of C# across
  866 files, plus a 186-file dashboard, none of which the shipped CLI calls.
  This is more than half the repository and it is the first thing a visitor
  sees. **This one needs a product decision, not an engineering one** — the task
  frames the options rather than presuming an answer.
- **M28-002** — The front-door files: `CONTRIBUTING.md`, issue and PR templates,
  `SECURITY.md`, and a README quickstart that is executed by a test rather than
  believed.
- **M28-003** — Two small true things. The `run` usage synopsis at
  `cmd/curlew/main.go:517` omits `--allow-sensitive`, `--events` and `--locale`
  — all three work and are documented everywhere else, and M23-001's parity test
  binds flags to `--help` without covering this second surface. And seven
  `*_LocaleDeferred` test stubs skip with a reason that M20-001 made untrue.

**Done when:** the repository root explains itself, and nothing in it is
evidence of an abandoned direction.

## M29 — Automation

*Capability: `ci_reenablement`*

- **M29-001** — Re-enable auto-triggers. All eight workflows carry
  `# Auto-triggers disabled while GitHub Actions billing is paused`. Every
  guarantee in this document currently rests on a human remembering to run
  `ci-local.sh`. **Blocked on a billing decision, which is the owner's to make.**
  `go.yml` already delegates to `./scripts/ci-local.sh --go`, so re-enabling is
  one uncommented block per file and cannot drift from the local gate.

---

## Sequencing

M25 first: without a deliverable, everything else improves a thing nobody can
run. M26 next and it is cheap — the skill is actively wrong today, and wrong
instructions to an agent are worse than absent ones. M27 is the long pole and
should start in parallel with M28, which is mostly writing. M29 unblocks
whenever the billing question is answered, and retroactively strengthens all of
it.

The dependency that matters: **M26-002 should land before M25-002.** A release
is the wrong moment to discover that the exit codes in the shipped skill and
the exit codes in the shipped binary disagree.
