# IMPROVEMENT.md — Developer & Agent Exploration Experience

> **Status: ARCHIVED** — fully implemented in M7–M11 (April 22–26, 2026).
> Retained for design-rationale context only. The source-of-truth for built
> behaviour is `docs/SPECIFICATION.md` (v4.1+) and `CHANGELOG.md`. Historical
> task records under `management/` reference this file by its original name
> (`IMPROVEMENT.md`); those references resolve here.

**Status:** Shipped — W1, W2, W3 complete (2026-04-24), W4 complete (2026-04-25), W5 complete (2026-04-25); §2.4 / §6 W5 bonus follow-ups closed in M11 (2026-04-26)
**Date:** 2026-04-22 (proposed); 2026-04-24 (status annotations added)
**Scope:** Post-M6 work. Builds on the agent-diagnosability contract established by M6-004 through M6-007.

---

## 1. Motivation

M6 shipped the agent-diagnosability contract: `--events` NDJSON with a v1.0 schema, error classification, source locations, sentinel-error surfacing, and a validation harness. The AI-agent consumer is now arguably the best-served audience of the tool.

The remaining weak audience is the **developer exploring an API in their editor** — and the related workflow where a **human drives an AI agent (Copilot, Claude Code, etc.) that drives our CLI**. Today that experience has friction at every step:

- No editor integration (a JSON Schema exists but is not published or wired to `yaml.schemas`).
- No single-request execution (entire collection re-runs on every tweak).
- Output is terminal-first; large response bodies render badly even with `-vv`.
- No predictable, human-readable artifact for the VS Code pane the way `--events` is for the agent.
- No canonical "how to use apitest" instructions for the agent — each agent freestyles.

This document proposes a coherent direction that closes these gaps and makes the `human → agent → CLI` loop a first-class workflow.

---

## 2. Current State (verified in this session)

### 2.1 Output formats that exist

| Format | Flag | Audience | Notes |
| --- | --- | --- | --- |
| Terminal (TTY) | default | Developer (local) | Color-coded pass/fail, wave markers, data-driven summaries. `-q` / `-v` / `-vv` for verbosity. |
| JSON | `--format json` | DevOps, general automation | Structured tree of requests, assertions, errors, parallel/data-driven metadata. Stdout or `--report`. |
| TAP v13 | `--format tap` | CI systems speaking TAP | Plan + per-test + YAML diagnostic blocks + wave comments. Stdout only. |
| JUnit XML | `--format junit` | QA / CI dashboards | Standard testsuites/testsuite/testcase. Free tier (ungated in M11-001). |
| HTML | `--format html --report <file>` | QA / stakeholders | Self-contained dashboard with SVG timelines. Professional-tier gated. |
| NDJSON events (M6) | `--events <file>` on `run` | **AI agents** | v1.0 locked schema: `run.start`, `request.start`, `assertion.result`, `request.end`, `run.error`, `run.end`. Error taxonomy + source locations + redaction. |
| JSONL exec log (M1-026) | `--log <file>` on `exec` | Audit / CI artifact | Append-one-line-per-exec. Different subcommand, different purpose from `--events`. |

Both JSONL streams (`--events`, `--log`) are live and complementary. An earlier mental model treated `jsonl.go` as dead code — that was wrong; it is the `exec --log` feature from M1-026.

### 2.2 CLI entry points

- `apitest exec <url>` or `apitest exec --stdin` — single-request, minimal, no assertions or extraction.
- `apitest run <collection.yaml>` — full collection with setup/main/teardown, assertions, extraction.
- `apitest watch <collection.yaml>` — auto-re-run on file change with 500 ms debounce.
- `apitest init [name]` — scaffold `apitest.yaml`, `environments/dev.yaml`, `collections/sample.yaml`, `.env.example`, `.gitignore`.
- `apitest validate <file>` — static validation against the embedded collection schema.

### 2.3 Editor integration

- **Zero VS Code extension.** No `.vscode/` in the repo. No LSP. No plans in SPECIFICATION.md (search for "vscode", "editor", "extension", "LSP" yields nothing relevant).
- **JSON Schema exists at `internal/schema/collection.json`** (draft 2020-12). It is embedded in the binary but not published anywhere `yaml.schemas` in VS Code can point at.
- Developers who edit collection YAML in VS Code today get YAML syntax highlighting and nothing else — no autocomplete, no inline validation, no jump-to-definition for `{{variable}}` references.

### 2.4 Gaps in existing output formats (not central to this proposal but worth tracking)

- Data-driven iteration visibility is inconsistent: terminal aggregates per request; JSON has aggregate stats but no per-iteration status; HTML has full iteration detail; TAP has group markers only. **JSON half shipped (M11-002). TAP half shipped (M11-003).**
- TAP lacks parallel-execution speedup metrics that terminal/JSON/HTML include. **Shipped (M11-003).**
- JUnit is Professional-gated — atypical; most projects treat JUnit as CI table-stakes. **Shipped (M11-001).**
- No quiet-mode minimal summary JSON for scripts wanting "did it pass?" without parsing a full tree. **Shipped (M11-002).**

---

## 3. Target Experience

### 3.1 Direct developer in VS Code (no agent)

1. `apitest init` scaffolds a project.
2. Developer opens `collections/explore.yaml`. **VS Code provides autocomplete, hover documentation, and inline validation** via a published JSON Schema mapped in `yaml.schemas`.
3. Developer adds a request, saves. `apitest watch collections/explore.yaml --only "Get user"` re-runs that one request on every save.
4. A markdown response file opens in the right-hand split pane, showing the formatted request + response + assertions. Updates on every run.
5. Developer iterates: tweak the YAML on the left, watch the markdown update on the right. Large response bodies render well because they are pretty-printed markdown, not truncated terminal dumps.

### 3.2 Agent-driven (Copilot / Claude Code / etc.)

1. Human says *"run the users collection against staging."*
2. Agent invokes `apitest run collections/users.yaml --env staging`. **Zero flags beyond env selection** — the collection YAML declares its own output mode (markdown reports + events stream).
3. Agent reads `.apitest/run.ndjson` for structured results and `responses/*.md` for human-facing artifacts.
4. Agent narrates in chat: "Three requests ran, one failed. See `responses/get-user.md` — the `email` field is null, your assertion expected it present."
5. Human asks to fix it. Agent edits the YAML. Loop continues.

**The key property:** one invocation contract (`apitest run <file>`), zero required flags. Everything else lives in the YAML or the skill.

---

## 4. Design Principles

These shape every work item below.

1. **Division of labor.**
   - **CLI**: execute requests; emit deterministic artifacts. Never hosts an LLM.
   - **YAML**: source of truth for what runs *and* what output it produces.
   - **Skill** (`.claude/skills/apitest/SKILL.md`): teach the agent how to use the artifacts. User-editable.
   - **Agent**: edit YAML, invoke CLI, read artifacts, narrate.
   - **Human**: collaborate with agent; edit YAML directly when wanted; open the markdown report alongside YAML in VS Code.

2. **Declarative output.** The YAML says what format to produce. The CLI obeys. Flags override, but zero-flag invocation is the normal case.

3. **Deterministic artifacts.** Given the same inputs, the CLI produces artifacts with identical structure, field names, ordering, and formatting (allowing for explicitly volatile fields like timestamps and durations). This is snapshot-testable — same discipline as the v1.0 events schema.

4. **Correlation IDs span artifacts.** `run_id` and `request_id` appear in the markdown report, every `--events` record, and every `--log` JSONL entry. Agent can fan out from any fragment back to the whole run.

5. **Skill as adapter, not hardcoded behavior.** The CLI does not know the agent exists. The skill is a thin, editable adapter between our artifacts and the agent's narration style. We ship a default; users override.

6. **No embedded LLM.** The agent lives in the user's AI tool (Copilot, Claude Code). Our CLI never calls an LLM API. This dissolves cost-gating, BYO-key config, provider choice — all out of scope.

---

## 5. Proposed Work Items

Five steps. Each independently useful. Ordered so each unblocks the next but none is required for its predecessors to ship value.

### W1 — Publish collection JSON Schema + VS Code wiring

**Status:** Shipped 2026-04-24 — M8-001 (PR #117, publishing pipeline) + M8-002 (PR #118, schema-completeness gaps from the W1 audit).

**What:** Export `internal/schema/collection.json` to a stable in-repo path (`schemas/collection.json`) and document a `.vscode/settings.json` snippet:

```json
{
  "yaml.schemas": {
    "./schemas/collection.json": "collections/*.yaml"
  }
}
```

Also consider publishing to `schemastore.org` for discoverability outside this repo.

**Why:** Highest value-per-hour. Turns "editing YAML blind" into proper type-ahead, hover docs, and inline validation for anyone with `redhat.vscode-yaml` installed (most developers already have it). No extension needed.

**Size:** Afternoon.

**Risks:** Schema may not be complete enough to drive good autocomplete. Worth a one-shot audit of the embedded schema against the full collection grammar before shipping.

**Definition of done:**

- `schemas/collection.json` exists and is a byte copy of `internal/schema/collection.json` (or the embed points at the new path).
- `.vscode/settings.json` snippet documented in README or MANUAL.md.
- One integration test asserts the schema validates the `sample.yaml` that `init` produces.

---

### W2 — `output:` block in project and collection YAML

**Status:** Shipped 2026-04-24 — M8-003 (PR #119). CLI > collection > project > built-in precedence implemented and tested.

**What:** Add an `output:` section, valid at both `apitest.yaml` (project default) and collection file (override). Fields:

```yaml
output:
  format: markdown          # terminal | json | tap | junit | markdown | html
  report: responses/        # directory for per-request markdown, or file path for other formats
  events: .apitest/run.ndjson   # omit to disable
  verbosity: quiet          # quiet | normal | verbose | debug
```

Precedence: **CLI flag > collection > project > built-in default**.

**Why:** The agent-invokes-with-zero-flags contract depends on this. The YAML becomes self-describing; the agent doesn't need to learn our CLI surface.

**Size:** 1–2 days. The plumbing is the ceremony — threading config through the command layer and formatter selection. Logic is trivial.

**Risks:**

- Scope creep on what's configurable. Resist adding per-request output overrides in v1.
- Conflicts with existing flag semantics need explicit precedence tests.

**Definition of done:**

- Schema updated to accept `output:` at both levels.
- Config precedence documented and tested end-to-end (flag overrides file, collection overrides project).
- `init` scaffolds a default `output:` block in `apitest.yaml` appropriate for the target audience.
- Validation errors for unknown formats or malformed paths.

---

### W3 — `run --only <name>` for single-request execution

**Status:** Shipped 2026-04-24 — M8-004 (PR #120) and M8-005. Duplicate-name rejection from §8 question 2 was bundled into M8-004; events schema bumped to v1.1 with an additive `selection` field on `run.start`. The §8.3 V2 minimal-setup follow-up (transitive-closure of `{{variable}}` refs via `internal/parallel/analyze.go` + new `parallel.AncestorClosure` primitive) shipped in M8-005: setup is pruned to the transitive closure of variable references, pure-seeder items (no `extract:`) always run, and an analyzer-invalid fallback with a stderr diagnostic covers the structural failure modes.

**What:** `apitest run collections/foo.yaml --only "Get user"` runs exactly that request, with its declared dependencies in the `setup:` phase. Multiple `--only` flags = union.

**Why:** The iteration loop is painfully slow when the whole collection re-runs on every tweak. This is the single biggest inner-loop improvement for both humans and agents. Also enables agents to respond precisely to *"re-run just the auth flow."*

**Size:** Day. Dependency resolution is the judgment call — "include whole setup" is the safe default; "include only declared dependencies" is cleaner but needs a dependency model we may not have.

**Open question:** Should `--only` have a YAML equivalent (`default_only:` in collection)? Probably not — "which request to run" is a runtime decision, not a file property. Leave as flag-only.

**Risks:**

- Fails loudly when setup phase dependencies are unclear.
- Users may try `--only` with data-driven requests; need defined behavior (run all iterations of that one, presumably).

**Definition of done:**

- `--only "Name"` selects by request name; multiple occurrences union.
- Setup phase still runs (conservative default); teardown runs on the selected set.
- Clear error when the name does not match any request.
- Integration test with a three-request collection: `--only` runs one, setup still executes.

---

### W4 — Markdown response format with sentinel-delimited deterministic block

**Status:** Shipped 2026-04-25 — M9-001 (PR #123) + M9-002 (PR #124) + M9-003 (PR #125) + M9-004 (PR #126) + M9-005 (PR #127).

**What:** `--format markdown --report <dir>` emits one `.md` per request under `<dir>/`. Each file has a CLI-owned region delimited by HTML comment sentinels:

```markdown
# Get user

## Notes
<!-- agent or human can edit this freely; survives re-runs -->

<!-- BEGIN apitest:response id=req-3 slug=get-user run=abc123 -->
## Response (deterministic)

**Request**
```
GET https://api.example.com/users/123
Authorization: Bearer ***
```

**Response — 200 OK**
Content-Type: application/json

```json
{
  "id": 123,
  "name": "Alice"
}
```

**Timing**

- Duration: 234 ms
- Wave: 1

**Assertions**

- [x] status == 200
- [x] body.name is string
<!-- END apitest:response id=req-3 slug=get-user run=abc123 -->

## Analysis
<!-- agent writes here, survives re-runs -->
```

**Rules for determinism:**
- Same inputs → same bytes for the CLI-owned block, *except* explicitly volatile fields (duration, wave index if parallelized, wall-clock timestamp). These live on dedicated lines so diffs are clean and the agent can ignore them.
- Canonical pretty-printing per content-type: JSON via 2-space `json.Indent`; YAML via stable serializer; text verbatim; binary as `hex.Dump` preview + total length; empty body as `(empty)` line.
- Redaction applied *before* formatting. The CLI never emits a secret into this block, regardless of `--allow-sensitive` (that flag only affects `-vv` terminal dumps).
- Section order is fixed. No conditional sections. Assertion block always present, empty list if no assertions.
- Re-running a collection rewrites only the region between the sentinels. Agent/human notes above and below survive.

**Why:**
- Large response bodies render well in VS Code's markdown preview — the terminal-truncation problem disappears.
- Split-pane UX: YAML on the left, rendered markdown on the right. Save → update.
- Diffable: the same request run twice produces two `.md` files that `git diff` cleanly, separating structural changes from timing noise.
- Agent-friendly: agent reads the markdown alongside the event stream; sentinel IDs pair the two.

**Size:** Few days. The formatter itself is small; the care is in edge cases — streaming responses, multipart, empty bodies, HEAD responses, binary, very large bodies (needs a reasonable byte cap, e.g. 1 MiB, with a truncation marker).

**Risks:**
- Scope drift into rich visualizations. Resist — this is markdown, not HTML.
- Edge-case matrix for content types is larger than it looks.
- File sprawl in `responses/`. One file per request name is fine; overwrite on re-run. No timestamp-suffixed history in v1 (user handles git themselves).

**Definition of done:**
- `--format markdown --report <dir>` produces one `.md` per request.
- Snapshot tests cover: JSON body, text body, empty body, binary body (hex preview), streaming response, multipart, assertion pass, assertion fail, redacted body.
- Sentinel region is byte-identical across two runs of the same request against a deterministic mock, modulo the documented volatile lines.
- Agent-authored content outside the sentinels survives a re-run.
- `run_id`, `request_id`, and `request_slug` are present in the sentinel opening tag.

---

### W5 — Default Claude skill packaged with the tool
**Status:** Shipped 2026-04-25 — M10-001 (PR #128, plumbing) + M10-002 (PR #129, real SKILL.md content + executable-spec test + docs).

**What:** Ship a skill at `templates/skills/claude/apitest/SKILL.md` in the repo. `apitest init` gains `--skill claude` which copies the skill into `.claude/skills/apitest/SKILL.md` in the user's project.

**Skill contents** (sketch, not final text):

```markdown
---
name: apitest
description: Run ApiTool collections and interpret results. Use when the user asks to run, test, or explore an API.
---

# ApiTool skill

## When to invoke
Triggered by phrases like "run the test", "hit that endpoint", "check the auth flow", "run apitest".

## Invocation
- Default: `apitest run <collection.yaml>` with no extra flags.
- Environment: append `--env <name>` if the user names one.
- Single request: append `--only "<request name>"` if the user wants just one.

## Where results land
The collection's `output:` block declares paths. Typical defaults:
- Summary on stdout.
- Structured events in `.apitest/run.ndjson`.
- Per-request markdown in `responses/<request-name>.md`.

## How to narrate
- Reference file paths ("see responses/get-user.md"). Do not paraphrase response bodies — the markdown file is the canonical view.
- On failure, read `.apitest/run.ndjson` for structured error details; read the relevant response markdown for the rendered context.

## Failure playbook by exit code
- 0: All passed.
- 1: Assertion failed. Point the user at the failing assertion line in the markdown.
- 2: Guard rail hit. Explain the request limit.
- 3: YAML parse error. Read stderr; show the user the line number.
- 4: Network error. Check environment variables and reachability.
- 5: Undefined variable. Name the missing variable from the event stream.
- 6: Feature gate. User needs to upgrade or remove the gated feature.

## What to edit
- Always edit the YAML collection. Never hand-roll curl or generate a one-off script.
- When adding a new request, prefer copying an existing request as a template.
```

**Why:**
- Pins agent behavior in an editable, shareable, version-controlled file.
- Fixes bugs in "how the agent uses apitest" once, globally.
- Users customize for their team's conventions.
- Parallel `--skill copilot` becomes obvious once GitHub Copilot's skill format stabilizes.

**Size:** Half a day once W4 settles (the skill references W4's artifacts).

**Risks:**
- Default skill content will evolve with user feedback. Treat it as a living artifact; version it.
- Do not force-install. Opt-in via `init --skill claude` only.

**Definition of done:**
- `templates/skills/claude/apitest/SKILL.md` exists and is well-tested against representative scenarios.
- `apitest init --skill claude` copies it to `.claude/skills/apitest/SKILL.md`.
- Integration test: a mock agent loop that follows the skill and exercises the five failure exit codes.

---

## 6. Correlation ID Scheme

Three identifiers, present in every artifact:

- **`run_id`**: 32-char lowercase hex (128 bits, crypto/rand) per CLI invocation. Already present in the events schema (`internal/output/events/emitter.go:296`). Spans the entire run — setup, main, teardown, all requests, all data-driven iterations.
- **`request_id`**: stable sequential counter per request within a run, formatted as `req-N` (`internal/runner/runner.go:293`). Locked by the v1.0 events schema. The correlation key.
- **`request_slug`**: human-readable identifier derived from the request name, used in filenames and sentinel tags. Added to the events schema as an additive field in v1.2 (M9-001). Not a correlation key — `request_id` is.

**Appears in:**
- Markdown sentinel: `<!-- BEGIN apitest:response id=<request_id> slug=<request_slug> run=<run_id> -->`
- Every `--events` NDJSON record (already does per M6).
- Every `--log` JSONL entry from `exec` (W5 bonus — add the IDs to the existing schema). **Shipped (M11-004).**

**Agent workflow this enables:**
1. Agent reads stdout summary: "3 requests, 1 failed."
2. Agent parses `.apitest/run.ndjson` for the failing `request_id`.
3. Agent reads `responses/<slug>.md` for rendered context.
4. All three reference the same IDs — no guessing which file corresponds to which event.

---

## 7. Explicitly Rejected Alternatives

Decisions that should not be re-litigated when backlog items are written:

- **No embedded LLM, no `apitest analyze` subcommand, no BYO-key provider config, no tier-gated AI analysis.** The agent lives in the user's AI tool. Our CLI never calls an LLM API.
- **No automatic `.gitignore` management.** User's responsibility. The CLI writes where it's told; git concerns are out of scope.
- **No mandatory skill installation.** `init --skill claude` is opt-in.
- **No per-request output overrides in the YAML in v1.** Project and collection levels only.
- **No timestamp-suffixed history files by default.** Overwrite. Users who want history use git.
- **No REPL / interactive mode / step-debugger.** Outside the vertical-slice scope of this direction.
- **No embedding of AI analysis inline during the request.** Would add latency to every run. If analysis becomes part of the loop later, it is a separate subcommand or a separate skill invocation — not coupled to `run`.

---

## 8. Resolved Questions

Decisions recorded on 2026-04-22 after a codebase audit. Treat these as settled unless new evidence emerges.

1. **Request ID format.**
   - **Resolution:** Keep `request_id` unchanged as `req-N` (already locked in the v1.0 events schema). Add a new additive field `request_slug` for human-readable surfaces (markdown filenames, sentinel). Events schema bumps to v1.1 (`selection`, M8-004) and then to v1.2 (`request_slug`, M9-001) — both additive, backward-compatible, no v2.0 required.
   - **Rationale:** `request_id` is already `req-N` at `internal/runner/runner.go:293` and was locked by M6-007. Changing it would be a breaking schema change for a non-problem. `request_slug` delivers readable paths without schema risk.

2. **Markdown file naming collisions.**
   - **Resolution:** Reject at collection-load time, unconditionally, regardless of output format. Add duplicate-name detection to the parser's validator pass with a line-numbered error message. Prerequisite fix for W4 but applied universally.
   - **Rationale:** The parser does not currently dedupe (`internal/parser/` has no such check). Duplicate names are a latent bug in every output format, not just markdown.

3. **W3 `--only` scope.**
   - **Resolution:** V1 (M8-004) ran the full setup phase (conservative). V2 (M8-005) shipped: `internal/parallel/analyze.go` + new `parallel.AncestorClosure` primitive compute the transitive closure of `{{variable}}` references; `--only` now runs the minimum setup needed. Pure-seeder items (no `extract:`) always run. Analyzer-invalid fallback covers structural failure modes with a stderr diagnostic.
   - **Status:** Shipped 2026-04-24 — M8-005.

4. **`run.md` index file.**
   - **Resolution:** Yes. W4 emits `<report>/run.md` with run summary, environment, pass/fail counts, `run_id`, and a linked list of per-request files. Canonical landing file for humans in VS Code and agents narrating a run.

5. **stdout/stderr discipline audit.**
   - **Resolution:** Yes, as a standalone prerequisite task before W2. 97+ `Fprintln(os.Stderr)` calls across `cmd/apitest/` have never been audited for stream correctness. Fix before W2 codifies `output:` — `format: json` on stdout depends on strict discipline being correct by default. Add two regression gates: (a) `apitest run <collection> --format json | jq .` parses cleanly; (b) split-capture test asserts stdout contains only the declared format.
   - **Tracked in:** `management/tasks/M7-001.yaml` through `M7-005.yaml` under the `output_discipline` capability group in `management/backlog.yaml`. Recon findings F1–F6 map to those five tasks; M7-001/002/003/005 are parallelizable and M7-004 is the consolidating CI gate.
   - **Status:** Shipped 2026-04-22 — all five M7 tasks done before W2 landed.

6. **Publishing JSON Schema to schemastore.org.**
   - **Resolution:** Defer. W1 ships in-repo `schemas/collection-v<version>.json` (versioned path) plus a `.vscode/settings.json` snippet — delivers 90% of the value. Gate the schemastore.org PR on a tagged release. Prep that *does* happen in W1: choose the versioned path now so the later PR is a 10-minute task, not a refactor.

7. **Volatile-field policy in W4's deterministic block.**
   - **Resolution:** Three tiers, clearly labeled:
     1. **Deterministic** — method, URL, request headers (post-redaction), request body (post-redaction), response status, response body (post-redaction, canonically formatted), assertion list. Byte-identical across runs given the same inputs.
     2. **Response metadata** — Content-Type, Content-Length, other response headers sorted alphabetically (post-redaction). Stable in practice against the same server; no cross-run guarantee.
     3. **Timing** — `duration_ms`, `wave_index` (under parallelism), `started_at`. Each on its own line with a fixed prefix so diffs are trivial to mask.
   - Known-volatile headers (`Date`, `X-Request-ID`, `Set-Cookie`, `ETag`) go in Response metadata. Snapshot tests run against mock servers that do not emit these; real-world diffs tolerate them because they are physically separated from the signal.

---

## 9. Backlog Decomposition Guidance

When converting this document to tasks:

- Each of W1–W5 is a **vertical slice** per the project's development philosophy. Each produces observable output and ships independently.
- Suggested ordering (repeated from §5): W1 → W2 → W3 → W4 → W5. Dependencies: W4 sentinel format depends on §6 ID scheme; W5 skill depends on W4 artifacts being stable.
- W2 (`output:` block) is the lynchpin — everything else becomes more valuable once the YAML can declare its own output.
- W4 (markdown formatter) is the largest; consider splitting into a behavior-driven substructure: base format + one content-type at a time.
- Gaps identified in §2.4 (data-driven visibility, TAP speedup metrics, JUnit gating, quiet-mode summary JSON) are not part of this proposal but should become separate backlog items when prioritized.

---

## 10. Appendix — Minimum Viable Example

After all five work items ship, the end-to-end minimal flow looks like this:

**`apitest.yaml`** (project default, written by `apitest init`):
```yaml
name: my-api
output:
  format: markdown
  report: responses/
  events: .apitest/run.ndjson
```

**`collections/users.yaml`**:
```yaml
name: Users
setup:
  - name: Login
    request: { method: POST, url: "{{base_url}}/auth", body: { user: admin } }
    extract: { token: "$.access_token" }
requests:
  - name: Get user
    request:
      method: GET
      url: "{{base_url}}/users/123"
      headers: { Authorization: "Bearer {{token}}" }
    assertions: { status: 200 }
```

**Developer in VS Code:** opens `collections/users.yaml`, gets autocomplete from W1's published schema, runs:
```
apitest run collections/users.yaml --only "Get user"
```
Opens `responses/get-user.md` in the right pane (W4). Iterates.

**Human driving Claude Code:** says *"run the users collection against staging."* Agent (guided by W5's skill) runs:
```
apitest run collections/users.yaml --env staging
```
Reads `.apitest/run.ndjson` and `responses/*.md`. Narrates the result. No flags beyond `--env`. No CLI surface the agent had to memorize.

That is the target experience. Every work item above is justified by its contribution to it.
