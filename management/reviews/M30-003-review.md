# M30-003 self-review

Date: 2026-09-16
Reviewer: implementing agent (self-review, not an independent agent evaluation)
Verdict: PASS for static review; final gate recorded separately.

## Scope and findings

- CLI dispatch and standalone/nested help expose the new command. Explicit agent
  selection avoids guessing a host or writing into multiple discovery directories.
- All targets consume one embedded payload. Existing init behavior and aliases are
  preserved. Installation does not touch collection, environment or project output.
- The installer validates the manifest and bundled destination types before writes;
  local modifications block updates even when another file could be upgraded.
  Unknown files survive. Paths from the manifest never select a write destination.
- Repeated identical installs are idempotent. Managed version upgrades are allowed
  only in update mode; identical legacy files can be adopted. Customized legacy
  copies fail with a concrete temporary-install/manual-diff recovery route.
- Symlink destinations are rejected. Individual files use temporary files plus
  rename, with manifest written last. Multi-file I/O failure and concurrent
  external mutation are explicitly outside the transaction guarantee.
- Skill trigger is scoped to Curlew. Authoring instructions preserve the user's
  chosen tools and contract; they no longer forbid requested alternative tools.
- Authoring example runs verbatim with a real local fixture. Evaluation setup is
  isolated and loopback-only and refuses to overwrite prior results. Its rubric
  requires retained evidence; fixture tests do not claim to evaluate an LLM.
- Official discovery paths are linked in the guide. Current README, manual,
  command specification, guide and skill snapshot agree with the new behavior.

## Validation reviewed

Focused CLI/installer tests, authoring recipe, three evaluation fixtures, skill
schema validation, existing skill reference/snapshot tests, and clean diff check.
The first workflow attempt was sandbox-blocked at socket startup; the identical
loopback test passed with local socket permissions. Full gate is separate.

## Limits

No live Claude/Copilot/Codex host-discovery or model decision evaluation was run.
The checked-in evaluation kit enables those runs without conflating them with
ordinary regression tests. No paid service, MCP server or release publication.

## Full-gate findings resolved

The first full gate found the new help printer missing from the test's explicit
help-surface registry, and two new prose claims not yet associated with executable
tests. Added the printer and linked claims to actual installation/update tests;
all three checks pass. The repository formatter also corrected three new files.
No production behavior change was needed for these findings.
