# M30-001 verification

Date: 2026-09-15
Branch: work/M30-001-agent-docs-and-examples
Result: PASS (local source and snapshot artifacts; no publication)

## Required gate

`./scripts/ci-local.sh --go` completed with `=== ci-local PASS ===`.
Local log: `/private/tmp/curlew-ci-final.log`.

- Ordinary Go tests and race detector: pass.
- Coverage: 86.6% total; cmd/curlew 81.0%.
- golangci-lint: 0 issues (also checked independently).
- Hermetic smoke suite: pass.
- GoReleaser configuration and host snapshot: pass, version 0.1.1-snapshot.
- All six Linux/macOS/Windows × amd64/arm64 archives: complete, correctly
  versioned and matched to recomputed checksums. Core agent/UI docs are included.
- README installation blocks: all executed successfully, including the published
  release download, go install and clone/build paths. The published release
  remains v0.1.0; this does not claim that it contains the new source fixes.
- Mudflat: full suite, parallel rendezvous, eight expected failures, redaction
  (zero known leaks), OpenAPI round trip, independent curl cross-check and ledger
  reconciliation all pass.

## Documentation and agent behavior

Included in all three Go test passes:

- Fourteen cookbook pages: prerequisite/run commands execute verbatim against
  loopback; complete YAML matches executable source. The perf report is valid JSON.
- Seven installed-skill topic examples: source parity, validation and real local
  execution, including independent signature verification.
- Agent guide: fresh init, discovery, validation, plan, Markdown/events, JSON
  redirection, pr-check and stdin exec. Assertion failure and an early usage error
  demonstrate why stderr and report freshness matter.
- Help: every dispatched command with --help/-h, nested import/vault/plugins,
  and unknown-command behavior.
- Standalone dry run: no setup/main/teardown HTTP for ordinary, JSON-selected,
  parallel or glob invocation; command-backed variables are not executed by plans.
- Markdown correlation: real run/request/slug mapping, iteration suffixes and
  event schema under every retention policy. Exec logs use separate run IDs.
- Existing manual table/prose, schema, flag, exit-code and generated-skill checks.

The WebSocket whole-frame regression passed 20 repetitions under the race
detector. Its red test produced interleaved frame bytes; its green test checks
frame boundaries and every payload without a timing threshold.

## Frontends and clean builds

- ui: check and lint pass; 116 unit tests, 20 real-binary Chromium scenarios pass.
  Browser log: `/private/tmp/curlew-ui-e2e.log`.
- site: locked dependency install, check (0 warnings/errors), lint and production
  static build pass. Build log: `/private/tmp/curlew-site-build.log`.
- Temporary source copy excluding generated assets: CLI-only build shows the
  explicit build-required fallback and fails the asset smoke check as expected.
  Running build-ui.sh then go build produces a binary whose JS/CSS and authenticated
  collection discovery pass. Log: `/private/tmp/curlew-clean-build.log`.
- The actual macOS arm64 archive was extracted separately; its UI assets,
  collection tree and standalone run help pass.
- The tracked dist/index.html is an intentional CLI-only fallback. Frontend builds
  overwrite it locally; it was restored after verification. Built binaries and
  archives retain their compiled frontend assets.
- Current entry-point local file links resolve. The old UI specification is
  preserved verbatim below its historical header. git diff --check is clean.

## Boundaries

Provider examples require the named provider CLI, credentials, permissions and
account resources. No real provider account was contacted to prove them. Partial
manual fragments, deliberately invalid examples and historical/platform material
retain explicit scope; they are not claimed as standalone runnable quickstarts.
No MCP server or native desktop wrapper was added. No release was published and
CI auto-triggers remain disabled under M29-001.
