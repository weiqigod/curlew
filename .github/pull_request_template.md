## What does this change?

<!-- One or two sentences: what changed and why. Link the task (management/tasks/<ID>.yaml) if there is one. -->

## Quality Gate Checklist

This is the checklist from `CLAUDE.md`'s Quality Gates section, copied here
so it is in front of you rather than something you have to go look up.
`./scripts/ci-local.sh` is the only thing that verifies this change — nothing
runs automatically in CI (see `CONTRIBUTING.md`).

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

## Anything a reviewer should know?

<!-- Deliberate scope cuts, follow-up work, anything you're unsure about. -->
