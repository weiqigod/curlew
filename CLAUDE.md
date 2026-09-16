# CLAUDE.md

## Hard Rules

- **Never take an action that may incur real-world cost (cloud spend, API charges, paid service usage) without explicit user approval.** This includes: running load tests against live endpoints, provisioning cloud resources, calling paid APIs, or anything else that could trigger a bill.

## Project Overview

**Curlew** is a file-based API testing tool — a Postman replacement built for developers who prefer files, version control, and CLI workflows.

**Two codebases:**
- **Go CLI** — the primary tool, distributed as a single static binary. Entirely local:
  no account, no login, no backend calls.
- **C# .NET backend** + web dashboard — retained in `src/` and `web/`, but the CLI no
  longer talks to them.

**Current Status (2026-09-16):** M1–M28 are complete. M29-001 (CI auto-triggers)
remains open pending a billing decision. M30 tracks documentation and agent
usability; inspect the live task files for status and verification evidence.
`v0.1.0` is published; main includes subsequent fixes. The CLI is local and
ungated. `src/` and `web/` are retained, frozen platform code, not CLI dependencies.
Use `go test ./internal/backlog/ -run TestBacklog_repository_is_consistent -v`
for live task counts. Older assessments and plans are historical; start with
`docs/README.md`, `docs/AGENT_GUIDE.md`, and the current CLI specification.

The following removals define the current CLI boundary:

1. The five-tier licensing/feature-gating system was removed — every CLI feature is unconditional, and there is no `curlew license` command.
2. All backend and login functionality was removed (2026-08-03). Gone: `curlew login`, `curlew worker`, distributed execution (`--workers`), report upload (`--report-upload`), and every `CURLEW_BACKEND_*` / `CURLEW_COORDINATOR_URL` variable. `pr-check` is now a local results-file gate, `telemetry` writes to a local NDJSON file, and shared vault templates load only from `CURLEW_TEAM_CONFIG`.

`docs/CLI_SPECIFICATION.md` (v1, 2026-08-04) is the CLI's specification — contracts, invariants, file formats, exit codes, and a conformance checklist — extracted from the platform spec and corrected against the shipped binary. `docs/MANUAL.md` is the CLI's user-facing reference. `docs/SPECIFICATION.md` is now scoped to the platform (`src/` backend + `web/` dashboard) and no longer claims to describe the CLI. The Go CLI builds (`go build ./cmd/curlew`) and runs; the .NET backend, web dashboard, and local web UI are all built. `web/tests/e2e/` holds exactly five convergence Playwright specs; they seed the backend over HTTP (no `curlew` binary involved) and run in `scripts/ci-local.sh`. The route-mocking `org-*` specs that used to sit alongside them were deleted — dashboard routes are covered by component and `load` tests under `web/src/routes/` instead. New backlog work resumes the task lifecycle below.

## Key Documentation

- **docs/PRODUCT_ROADMAP.md** — **Current product status and the original M25–M29 plan.** Read the current-status header, then the live backlog before starting anything new. Not to be confused with `ROADMAP.md` at the repository root, which hardens the *workflow* rather than the product.
- **docs/CLI_SPECIFICATION.md** — The CLI's specification: contracts, invariants, formats, exit codes (v1)
- **docs/TESTAPI_SPECIFICATION.md** — Mudflat, the dedicated test API for dogfooding the CLI against a server curlew did not write. **Phases 1 through 4 are implemented in `testapi/`** and run as `ci-local.sh` gate steps (dogfood suite, --parallel rendezvous, and the gaps/redaction/openapi/crosscheck/ledger harnesses). Dogfooding has found **seventeen defects — all fixed**: five from Phases 1–2 (§11A, §11B), ten from Phase 3 (§11C.1–10) plus an eleventh found while fixing them (§11C.11) — those sixteen closed by 2026-08-13 — and one from Phase 4 (§11D.1, closed 2026-08-19), which is the first that was a wrong *number* rather than an error and so shipped inside a run that reported success and exited 0. Every reproduction was *moved*, not deleted — into a passing collection, into `expected-failures.yaml` inverted so it still fails but for the right reason, or into a unit test — so a closed gap keeps the test that proves it stayed closed. Two harnesses were retired by their own promotion instructions (`timing.sh`, and the `*.run-abort.yaml` category in `gaps.sh`); `openapi.sh` was inverted from "these must be rejected" to "these must import". `testapi/harness/redaction-known-leaks.txt` is empty, which means every output surface is enforced rather than baselined. §9.N (the nine-port TLS matrix) is deliberately reduced to one listener — §14.3 has the measurement showing no client-side route to trusting mudflat's CA exists on this toolchain
- **docs/MANUAL.md** — The CLI's user-facing reference (how to use it)
- **docs/SPECIFICATION.md** — Platform specification for the `src/` backend and `web/` dashboard (v4)
- **docs/DEVELOPMENT_PHILOSOPHY.md** — Always-runnable, vertical slices, completeness contract
- **docs/TECH_CHOICES.md** — Language choices, tooling, conventions for both codebases
- **docs/WORKFLOW_TEMPLATE.md** — Reference template for the task-managed workflow

## Development Philosophy

### Test-Driven Development (Mandatory)

**All development MUST follow TDD. No exceptions.**

1. **RED**: Write a failing test first that defines expected behavior
2. **GREEN**: Write the minimum code to make the test pass
3. **REFACTOR**: Improve code quality while keeping tests green

**Never write implementation code without a failing test first.**

### Vertical Slice Approach

**Every task delivers observable, runnable output.** Tasks are organized by capability (what the user can do), not by technical layer.

1. **Observable Output**: Every task produces something you can run and see
2. **TDD Path**: Behaviors → Tests → Implementation → Observable
3. **Incremental Value**: Each slice builds on previous, always runnable

### Always Runnable

At every point during development, the application must run. Build cleanly, start without errors, do what it claims. Features not yet built are invisible — no commands or flags that exist but don't work. See `docs/DEVELOPMENT_PHILOSOPHY.md` for the full commitment.

### Completeness Contract

Every slice must be complete before moving on: input to output, error handling included, help text updated, integration test that exercises the real binary.

## Go Development Standards

### Error Handling
- Return errors, don't panic
- Wrap errors with `fmt.Errorf("context: %w", err)` for traceable chains
- Sentinel errors for well-known failure modes (e.g., `ErrCircularReference`, `ErrUnsupportedFeature`)

### Naming Conventions
- Follow Effective Go — short names in tight scopes, descriptive at package level
- No stuttering: `parser.Config` not `parser.ParserConfig`
- Exported symbols get doc comments

### Dependencies
- Minimise external dependencies — standard library first
- Evaluate every `go get` against the cost of the dependency

### Concurrency
- Always use `context.Context` for cancellation and timeouts
- No shared mutable state — channels or clearly scoped mutexes
- Goroutines only where the spec calls for parallelism

### Code Organization
- `internal/` packages enforce encapsulation at the compiler level
- Each package owns its domain and exposes a narrow interface
- `cmd/curlew/` contains only the entry point

## Task Management

Tasks are managed in the `management/` folder:

- **`management/backlog.yaml`** — Task index organized by capability
- **`management/tasks/<TASK-ID>.yaml`** — Individual task definitions
- **`management/plans/<TASK-ID>-plan.md`** — Implementation plans
- **`management/plans/<TASK-ID>-verified.md`** — Verification reports
- **`management/plans/<TASK-ID>-improved.md`** — Improvement reports
- **`management/reviews/<TASK-ID>-review.md`** — Code review reports

Task workflow: `backlog → planned → in_progress → review → done`

Use slash commands to work with tasks:
- `/plan <TASK-ID>` — Create implementation plan
- `/execute <TASK-ID>` — TDD implementation
- `/review <TASK-ID>` — Code quality audit
- `/improve <TASK-ID>` — Fix review findings
- `/verify <TASK-ID>` — Final verification + PR

Task lifecycle: `/plan` → `/execute` → `/review` → `/improve` (if needed) → `/verify`

### Task ID Conventions

Task IDs use the form `M<N>-NNN`, where `N` is the milestone number. The project
spans milestones **M1 through M30**; inspect the backlog for live status. The table below illustrates the
journey-stage decomposition with M1's six stages as the worked example:

| Milestone | Journey Stage | Description |
|-----------|--------------|-------------|
| `M1` | first_test | From zero to a running test |
| `M1` | test_with_confidence | Assertions and error clarity |
| `M1` | organize_and_reuse | Variables, environments, composition |
| `M1` | output_and_ci | Output formats and redaction |
| `M1` | project_commands | Init and validate |
| `M1` | ai_and_gating | AI commands and feature gates |

Each task is a vertical slice cutting through multiple packages. Tasks form a DAG — see `.claude/skills/backlog/milestone-mapping.md` for the full dependency graph.

### Task Definition Format

```yaml
id: M1-001
title: "Short descriptive title"
status: backlog
priority: 1
created: YYYY-MM-DD
phase: "M1: Phase Name"
dependencies: []
observable: |
  What you can run to verify
behaviors:
  - "Testable behavior"
scope: |
  What needs to be built
definition_of_done:
  - All behavior tests pass
  - Observable output works
  - Test coverage >= 80%
  - No build warnings
complexity: low
estimated_effort: "2-3 hours"
```

## Git Workflow

### Never Commit to Main

**All development MUST happen on feature branches. No exceptions.**

### Branch Naming
- `feature/<TASK-ID>-description` — New features
- `fix/<TASK-ID>-description` — Bug fixes
- `refactor/<TASK-ID>-description` — Code improvements

### Commit Messages
```
type(scope): brief description

- Detail 1
- Detail 2

Refs: TASK-XXX
```

Types: `feat`, `fix`, `refactor`, `test`, `docs`, `chore`

### TDD Commit Pattern
1. `test(scope): add failing tests for feature X` (RED)
2. `feat(scope): implement feature X` (GREEN)
3. `refactor(scope): extract helper for feature X` (REFACTOR)

## Build Commands

```bash
./scripts/build-ui.sh          # Build embedded UI (Node.js 22+)
go build ./cmd/curlew          # Build binary
go test ./...                   # Run all tests
go test -coverprofile=coverage.out ./...  # Tests with coverage
go tool cover -func=coverage.out         # Coverage report
golangci-lint run               # Lint
./smoke/run.sh                  # Smoke test
./scripts/ci-local.sh           # Full local CI gate — scoped to what changed
./scripts/ci-local.sh --full    # Force every gate (backend + web + E2E)
./scripts/ci-local.sh --go      # Go gate only, skip docker stack
```

## Quality Gates

Before any commit:
- [ ] On feature branch (not main)
- [ ] `go build ./cmd/curlew` succeeds
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes

Before task completion (`/verify`):
- [ ] `./scripts/ci-local.sh` passes — this is the authoritative gate, and right now it is the *only* one (see below)
- [ ] Coverage >= 80%
- [ ] All Definition of Done items verified
- [ ] CHANGELOG.md updated

`ci-local.sh` auto-detects scope via `git diff --name-only main...HEAD`: it always runs Go build/test/race/lint/smoke, and additionally runs `dotnet test`, `npm run test:unit`, and the Playwright E2E specs (against the `docker-compose.test.yml` stack) when backend, web, or stack files have changed.

**Nothing runs in CI today.** Every workflow in `.github/workflows/` carries `# Auto-triggers disabled while GitHub Actions billing is paused` and fires only on `workflow_dispatch`, so pushing a branch and opening a PR verifies nothing. `ci-local.sh` is not a fast pre-check ahead of CI — it is the whole gate, and skipping it means the change was never tested by anything but the author's judgement.

`.github/workflows/go.yml` runs `./scripts/ci-local.sh --go` rather than restating its steps, so the workflow and the local gate cannot drift into disagreeing about what "green" means. Re-enabling it is one uncommented block in that file; the same applies to the `e2e-*` workflows, which cover the .NET, web and Playwright gates.
