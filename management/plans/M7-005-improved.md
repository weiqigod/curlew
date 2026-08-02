# Improvement Report: M7-005

**Task:** Thread stdout and stderr writers through runCmdInner (remove os.Stdout swap)
**Date:** 2026-04-22
**Review:** management/reviews/M7-005-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | Medium | `runWithWriters` doc comment was factually incorrect — stated "Only the 'run' subcommand and help output route through the writers; other subcommands still use their own os.Stdout/os.Stderr paths", which described the pre-M7-005 state. In reality all subcommands thread the provided writers through their `*CmdOut` variants. | Updated the doc comment on `runWithWriters` in `cmd/curlew/main.go` to accurately state that all subcommands dispatch through writer-aware `*CmdOut` functions and no subcommand writes directly to process-global `os.Stdout`/`os.Stderr`. | ✓ tests pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/curlew` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage | 86.2% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 367c435 | fix(cmd): correct stale doc comment on runWithWriters | #1 |

## Summary
1/1 findings resolved. 0 deferred.
