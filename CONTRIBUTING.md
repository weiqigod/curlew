# Contributing to Curlew

This file is addressed to a person sending a pull request. `CLAUDE.md`
carries the same rules in more detail, addressed to the AI assistant that
does most of the day-to-day development here — read it too if you want the
full picture, including the task-management workflow under `management/`.

## Never commit to main

All development happens on a feature branch. No exceptions, including for a
one-line fix.

```
feature/<TASK-ID>-description   # new features
fix/<TASK-ID>-description       # bug fixes
refactor/<TASK-ID>-description  # code improvements
```

If you are not working from a tracked task, pick a short branch name that
starts with `feature/`, `fix/`, or `refactor/`.

## TDD, without exception

All development follows RED → GREEN → REFACTOR:

1. **RED** — write a failing test first that defines the expected behavior.
2. **GREEN** — write the minimum code to make it pass.
3. **REFACTOR** — improve the code while keeping the tests green.

Never write implementation code without a failing test first. A pull request
that adds behaviour with no test change attached to it will be asked to add
one.

## The gate: nothing runs in CI

**Nothing runs in CI today.** Every workflow under `.github/workflows/`
carries `# Auto-triggers disabled while GitHub Actions billing is paused` and
fires only on `workflow_dispatch`. Opening a pull request verifies nothing by
itself.

That makes `./scripts/ci-local.sh` the whole gate, not a fast pre-check ahead
of a real one. Run it — or the narrower mode that covers your change — before
you say a change is done:

```bash
./scripts/ci-local.sh                        # auto: scoped to what changed on this branch
./scripts/ci-local.sh --full                 # force every gate: backend + web + e2e
./scripts/ci-local.sh --go                   # Go gate only, skip the docker stack
./scripts/ci-local.sh --down                 # tear down the test stack idempotently
./scripts/ci-local.sh --check-signing-keys   # SaaS lint: fail on any NULL kms_key_id
```

The Go gate needs Go 1.24+, Node.js 22+ / npm, Python 3, Git, authenticated
GitHub CLI access to this private repository, golangci-lint and GoReleaser v2.
The frontend build is part of the binary build; release users do not need Node.

The Go gate (always run, whichever mode you pick) directly runs:

- `./scripts/build-ui.sh` — install locked UI dependencies and build embedded assets
- `go build` — the binary must build cleanly
- `go test` — every package's tests
- `go test -race` — the same tests under the race detector
- `golangci-lint` — lint, including `gofumpt` formatting
- `./smoke/run.sh` — the hermetic smoke suite against the built binary
- Documentation, agent and cookbook recipes against local fixtures
- Release snapshots/archives, embedded UI smoke check and README installation tests
- Mudflat dogfood suites and the independent ledger cross-check

If your change touches `src/` (the .NET backend), `web/` (the dashboard), or
the Docker test stack, `auto` mode detects committed branch changes relative to
main and pulls in the matching gate. It does not detect uncommitted edits.
For UI changes, also run `cd ui && npm run test:e2e`. For the cookbook site,
run `npm ci`, `npm run check`, `npm run lint` and `npm run build` under `site/`.

## Task lifecycle

Substantial work in this repository is tracked as a task under
`management/tasks/`, moving through:

```
backlog → planned → in_progress → review → done
```

driven by slash commands, in order:

`/plan` → `/execute` → `/review` → `/improve` (if needed) → `/verify`

A drive-by pull request that is not tied to a task is welcome too — just
follow the branch and TDD rules above, and make sure `./scripts/ci-local.sh`
passes before you open it.

## Before you open a pull request

- [ ] You are on a feature branch, not `main`
- [ ] `go build ./cmd/curlew` succeeds
- [ ] `go test ./...` passes
- [ ] `golangci-lint run` passes
- [ ] `./scripts/ci-local.sh` passes for the parts of the tree you touched

The pull-request template repeats this checklist so it is in front of you
when you open the PR.

## Reporting a security issue

Do not open a public issue for a vulnerability — see
[SECURITY.md](SECURITY.md) for the private route.
