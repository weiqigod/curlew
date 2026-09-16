# M30-003 verification

Date: 2026-09-16
Verifier: implementing agent
Verdict: PASS
Branch: work/M30-003-agent-skill-installation

## Authoritative gate

`./scripts/ci-local.sh --go` completed with exit 0 and `=== ci-local PASS ===`.
Environment: Go 1.25.5, Apple Command Line Tools, shared temporary Go cache.
Full log: `/private/tmp/curlew-m30-003-ci-final.log` (local execution artifact).

| Check | Result |
|---|---|
| Go build, ordinary tests and race tests | PASS |
| Coverage | 86.6% total; CLI 81.1%; new skillinstall package 85.6% |
| golangci-lint | 0 issues |
| Smoke suite, including external-process plugin example | PASS |
| Embedded UI build and artifact/discovery check | PASS |
| Six release archives and recomputed checksums | PASS |
| README installation commands | PASS |
| Mudflat suite, parallel, negative cases, redaction, OpenAPI and curl crosscheck | PASS |
| Report/ledger comparison | All 4 checks agreed |
| Skill creator quick_validate.py | Skill is valid |
| git diff --check | PASS |

The generated tracked UI index was restored to its source fallback after the gate,
as in M30-001/M30-002; it is not part of this change. The built local executable
and release snapshots retain the complete embedded UI.

## Behavior evidence

| Behavior | Evidence |
|---|---|
| Three explicit destinations, project configuration preserved, repeated installs | TestSkillInstallCLI runs the built CLI for codex, claude and copilot |
| Managed upgrade, custom files, missing references | internal/skillinstall.TestUpdate; old -> new version substitution, retained custom file, restored reference |
| Conflict prevents partial writes | TestConflictPreflight checks skill and manifest unchanged despite another file eligible for upgrade |
| Legacy installation adoption and edit protection | TestUnmanaged |
| Bad state, links, file types, missing installation | TestInvalidStateAndPaths |
| Argument validation and side-effect-free help | TestSkillCommandArguments, TestAgentHelpDiscovery, help flag parity |
| Authoring commands | TestSkillAuthoringRecipe executes the installed-reference source's Bash block verbatim against loopback, then verifies results |
| Evaluation fixture validity | TestSkillEvaluationFixtures runs prepare.py and the actual CLI: assertion exit 1 with source events; YAML exit 3 leaving old Markdown intact; instruction-bearing response with a real pass |
| Existing skill contract | Existing snapshot, command, output, exit-code, reference and hygiene tests passed in full gate |

Observable also executed directly with the built `./curlew`: both `skill install
--agent codex` and `skill update --agent codex` succeeded in fresh workspace
`/private/tmp/curlew-skill-demo.QpsFXp`, reporting the installed directory.

## Red/green and improvements

The initial CLI tests failed because `skill` was an unknown command (log
`/private/tmp/curlew-m30-003-red.log`). Focused installer tests passed after
implementation. An initial fixture test run could not bind sockets in the sandbox;
the same tests passed with local socket access. This environment failure is not
claimed as a product red test.

The first full gate (`/private/tmp/curlew-m30-003-ci.log`) found a missing test
help-printer registration and two unlinked executable prose claims. Registered
the new help surface and connected the claims to actual installation/update tests.
Formatting findings from a separate lint run were corrected with gofumpt.
The entire authoritative gate was then rerun successfully, not resumed partially.

## Review and limits

Static self-review: management/reviews/M30-003-review.md. No independent agent
review or model evaluation was performed. The evaluation tasks and rubric are
ready for real agent runs; deterministic fixture tests do not establish host
skill discovery or a model decision pass rate. Discovery locations were checked
against linked official documentation on 2026-09-16.

No live provider calls, paid services, MCP server, CI trigger changes or tagged
binary release. Conflict preflight is all-or-nothing; filesystem failures during
individual writes and concurrent external mutation are not a transaction guarantee.
All definition-of-done items are covered above.
