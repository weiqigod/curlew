# M30-006 review

Date: 2026-09-17
Reviewer: branch-review agent plus implementing-agent follow-up
Verdict: PASS for static review; final gate recorded separately.

## Findings resolved

- POSIX plugin shutdown sent a final process-group `SIGKILL` even after the child
  had exited and been reaped. Shutdown now returns after each successful bounded
  wait and escalates only from EOF to `SIGTERM` to `SIGKILL` when needed.
- The remaining real plugin fixture build now disables VCS stamping, matching the
  other mounted-checkout-safe fixture build.
- The first review questioned literal percent handling through the Windows batch
  editor transport. The existing real argv-recorder matrix preserves `%PATH%`,
  `!value!`, quotes, metacharacters, spaces and Unicode through `.cmd`, `.bat` and
  PATH-resolved `code.cmd`; no production change was needed.
- Native full-package validation exposed repeated CLI builds, a one-minute recipe
  budget that included compilation, a POSIX permission assertion on Windows and
  extensionless fake Windows plugins. Tests now share one read-only CLI binary per
  package run and keep platform-specific assertions platform-specific.

## Validation reviewed

| Check | Result |
|---|---|
| Second branch review | PASS, no blocking findings |
| Native `cmd/curlew` package with M30-004-owned tests excluded | PASS in 948.161s |
| Other applicable native Go packages | 57 of 57 PASS |
| Native plugin package and real process-tree lifecycle | PASS |
| POSIX plugin package and real process-tree lifecycle | PASS |
| Windows editor real argv-recorder matrix | PASS |
| Native smoke success and readiness-timeout cleanup | PASS |
| POSIX smoke | PASS through `SMOKE_EXIT=0` |
| UI check, lint, unit tests and build | PASS |
| Changed-file golangci-lint | PASS, 0 issues |
| GoReleaser configuration check | PASS |

## Scope and limits

M30-004 owns native `from_command` and vault command behavior and remains excluded
from this branch's final native package evidence. M30-005 owns clean-host and
packaged-release acceptance. No live provider calls, paid services, hosted CI,
cloud resources or release publication were used.

The authoritative auto-scoped POSIX gate, final coverage value and final task-state
transition belong in `management/plans/M30-006-verified.md` after that gate completes.