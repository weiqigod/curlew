# M30-002 verification

Date: 2026-09-16
Branch: work/M30-002-local-examples
Result: PASS

## Required gate

`./scripts/ci-local.sh --go` exited 0 with `=== ci-local PASS ===`.
Log: `/private/tmp/curlew-m30-002-ci-final.log`.

- Go build, ordinary tests, race detector and coverage: pass.
- Coverage: 86.6% total; CLI 81.0%.
- golangci-lint: 0 issues.
- Smoke suite, including building/testing the separate Datadog plugin module: pass.
- Embedded UI build and artifact smoke, all six release archives and checksums: pass.
- README installation commands: pass.
- Mudflat suite, parallel runs, expected failures, redaction, OpenAPI, curl
  cross-check and all four ledger comparisons: pass.

## Executable example evidence

`TestLocalExampleRecipes` runs during ordinary, race and coverage passes. It:

- Starts the shipped Python fixture using the command extracted from both READMEs
  and their documented ephemeral-port option.
- Runs the JSON inheritance, expected-failure and plugin walkthrough commands
  verbatim from their README sections in a temporary checkout of examples.
- Verifies JSON total/failure counts, exit 3 and the invalid-format diagnostic.
- Observes exactly two GET requests and one metric submission, checking its name
  and status tag. The invalid example sends no request to the counted server.

The initial regression failed because the local fixture/walkthrough was absent.
A first full gate caught an unchecked response-body close in the new test; that
was corrected, standalone lint passed, and the complete gate passed on rerun.
Initial logs: `/private/tmp/curlew-m30-002-red.log`,
`/private/tmp/curlew-m30-002-ci.log`. The final log above is authoritative.

Documentation file links resolve and `git diff --check` passes. The generated
embedded UI index was restored to its tracked fallback after the build, so no
frontend artifact changes are included. Only task closeout documentation changed
after the final gate; backlog and documentation consistency were checked again.

## Boundaries

No production CLI/plugin code or frontend behavior changed. Python 3 and loopback
sockets are required for these examples. The fixture records local submissions;
it does not validate the production Datadog API or a real provider account.
No binary release was published; CI auto-triggers remain disabled under M29-001.
