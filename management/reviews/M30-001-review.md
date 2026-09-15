# M30-001 — Documentation and agent usability review

Date: 2026-09-15
Reviewer: implementing agent, self-review
Branch: work/M30-001-agent-docs-and-examples

## Verdict: PASS

## Scope and findings resolved

- Subcommand help previously treated `--help` as a run flag or an exec URL.
  Dispatch now handles standalone help before configuration and side effects;
  tests derive the command list from dispatch and cover nested commands too.
- The agent scaffold configured `.curlew/run.ndjson` without making its parent
  directory. A fresh scaffold/run regression exposed and now checks this path.
- JSON `--report` examples did not produce their promised files. The guides and
  skill now redirect stdout and keep diagnostics separate. Tests execute the
  recipes, including assertion failure and stale artifacts after a usage error.
- Installed skill references contained unsupported assertion/CEL shapes,
  `wave`, worker settings, retry keys, signing keys, env import and vault forms.
  Seven complete replacement examples are checked against their source and
  executed against Mudflat. Numeric CEL comparison explicitly converts string
  variables. Provider configuration is separated from local redaction proof.
- Cookbook pages advertised removed or external features and invalid protocol
  shapes. All fourteen now use local executable fixtures, explicit prerequisites
  and actual commands. YAML is compared with its executable source.
- Markdown/event correlation tests previously checked only nonempty IDs and
  attached a claim about JSONL they never executed. They now compare actual run
  IDs, request IDs, event slugs and schema versions under all retention policies.
  Documentation explains iteration suffixes and expanded slugs. A separate test
  proves exec logs represent distinct invocations and run rejects `--log`.
- Release builds could embed HTML referencing absent JS/CSS. Build hooks now
  compile the UI first; artifact smoke tests fetch the resources and authenticated
  collection tree from a newly initialized project. A temporary source copy with
  no generated assets reproduced the failure and passed after the documented build.
- The original UI design and platform-era investigations contained obsolete
  product promises. Their history is retained with explicit scope; the current
  UI contract is based on implemented routes and browser tests.

- Final review found `run --dry-run` was a no-op without `--show-dependencies`.
  A counted loopback server observed nine requests across three dry-run variants.
  Standalone dry runs now use the existing planning path; regression cases cover
  ordinary, JSON-selected, parallel and glob invocations with zero HTTP calls.
  Two locale tests depended on the former no-op; they now execute explicitly
  against their existing loopback server.
- The coverage pass exposed intermittent invalid WebSocket frames in Mudflat's
  push/heartbeat fixture. Its control pump wrote pongs between a foreground
  header and payload. A yielding connection reproduced corruption, and a mutex
  now serializes whole frames. The cookbook assertions were retained unchanged.

## Review boundaries

No MCP server, provider account access, native desktop installer, deployment or
new release is claimed. The built-in JSON-RPC plugin API is distinct from MCP.
Historical evidence and deliberately invalid examples are not live quickstarts.
CI auto-triggers remain disabled under M29-001; the owner has not changed the
billing decision. No frontend dependency versions were changed.

## Verification

See M30-001-verification.md for final gate results. Current entry-point local file
links resolve. `git diff --check` is clean. UI checks passed (116 unit tests and
20 real-binary browser scenarios); the cookbook site passes check/lint/build.
