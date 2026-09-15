# Documentation map

Release archives include the core manuals and agent guide. Additional references
and fixtures are in the [source repository](https://github.com/weiqigod/curlew).

## Current local product

- [User manual](MANUAL.md): installation, YAML authoring, browser walkthrough and reference.
- [Agent guide](AGENT_GUIDE.md): shell integration, structured results and failure handling.
- [CLI specification](CLI_SPECIFICATION.md): command and file-format contracts.
- [UI specification](UI_SPECIFICATION.md): current browser/server contract.
- [Plugins](plugins.md): external-process hooks; this is not MCP.
- [Examples cookbook](../site/README.md): complete local examples and how to run them.
- [Mudflat](../testapi/README.md): local fixture API and adversarial regression suite.
- [Event schema v1.6](EVENTS_SCHEMA_v1.6.md): current event format; older numbered
  versions remain versioned protocol references, not the default output schema.
- [Technical choices](TECH_CHOICES.md), [development philosophy](DEVELOPMENT_PHILOSOPHY.md),
  and [contributing](../CONTRIBUTING.md): build and contribution conventions.
- [Product roadmap](PRODUCT_ROADMAP.md): current status followed by the original plan.
  Task YAML and the backlog verifier provide live completion status.

## Retained platform material

[SPECIFICATION.md](SPECIFICATION.md), [COMPLIANCE.md](COMPLIANCE.md), `security/`,
`api-errors.md`, `../web/`, `../src/`, `../deploy/`, and email templates describe
the retained platform. They do not add accounts, cloud reporting, licensing,
distributed workers or backend dependencies to the local CLI. Platform examples
need their documented backend/services and are not local CLI quickstarts.

## Historical evidence

`ASSESSMENT.md`, `REVIEW.md`, `SCRIPTING.md`, `M*_INVESTIGATION.md`, `history/`,
`design/`, the root `ROADMAP.md`, and completed files under `management/` preserve
earlier proposals, investigations and verification evidence. Their examples may
intentionally describe removed, unimplemented or invalid behavior. They are not
current executable recipes. Do not restore a removed feature from an old plan.

## What “examples work” means

The README quickstart, agent guide recipes, seven skill topic examples and
fourteen cookbook commands execute against
local fixtures. The Go gate also runs documentation table/prose checks, schema
parity, protocol dogfood tests and the smoke suite. Cookbook YAML is compared
byte-for-byte with its executable source. Site build, type checks and lint check
the rendered documentation independently.

The manual includes partial configuration fragments, deliberate failures, CI
workflow templates and provider-specific recipes. These need the surrounding
files or services named in their section; they are not all standalone shell
scripts. Do not run a live cloud command just to validate a documentation example.
Historical and platform documents retain their own scope above.
