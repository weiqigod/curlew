# CLAUDE.md

## Hard Rules

- **Never take an action that may incur real-world cost (cloud spend, API charges, paid service usage) without explicit user approval.** This includes: running load tests against live endpoints, provisioning cloud resources, calling paid APIs, or anything else that could trigger a bill.

## Project Overview

**Curlew** is a file-based API testing tool — a Postman replacement built for developers who prefer files, version control, and CLI workflows.

**Two codebases:**
- **Go CLI** — the primary tool, distributed as a single static binary
- **C# .NET backend** + web dashboard — authentication, dashboards

**Current Status:** Implemented through M20 — the entire planned backlog (M1–M20) is complete (backlog exhausted 2026-06-12). The five-tier licensing/feature-gating system has since been removed from the Go CLI: every CLI feature is unconditional, and there is no `curlew license` command. `docs/MANUAL.md` is the authoritative feature reference. The Go CLI builds (`go build ./cmd/curlew`) and runs; the .NET backend, web dashboard, and local web UI are all built. New backlog work resumes the task lifecycle below.

## Key Documentation

- **docs/SPECIFICATION.md** — Full system specification (v4)
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
spans milestones **M1 through M20** (all complete). The table below illustrates the
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
- [ ] `./scripts/ci-local.sh` passes — this is the authoritative gate and mirrors the GitHub workflows
- [ ] Coverage >= 80%
- [ ] All Definition of Done items verified
- [ ] CHANGELOG.md updated

`ci-local.sh` auto-detects scope via `git diff --name-only main...HEAD`: it always runs Go build/test/race/lint/smoke, and additionally runs `dotnet test`, `npm run test:unit`, and the Playwright E2E specs (against the `docker-compose.test.yml` stack) when backend, web, or stack files have changed. Running it locally before pushing avoids the ~5-minute CI round-trip per attempt for failures that reproduce on the developer's machine.
