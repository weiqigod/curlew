# Spec-vs-Implementation Review

> **Historical analysis.** This records an earlier design or investigation and may
> describe removed features, licensing, or already-completed work. It is not a
> current operating guide. Use [MANUAL.md](MANUAL.md), [CLI_SPECIFICATION.md](CLI_SPECIFICATION.md),
> and the [documentation map](README.md) for current behavior.


A line-by-line audit of `docs/SPECIFICATION.md` against the current codebase, intended as the input for the next round of milestones and tasks. Where `docs/ASSESSMENT.md` characterised the project from the outside, this document looks inward — what is built, what is half-built, what is named in the spec but not yet present.

The principal surprise of this audit, and the correction the prior assessment owes its readers: **the backend exists and works**. The C# ASP.NET Core 8 service in `src/ApiTool.Backend/`, the SvelteKit dashboard in `web/`, the 19 EF Core migrations, the SAML and OIDC handlers, the RBAC system, the audit logging, and the on-prem deployment bundle are all real and functional. The prior framing of Curlew as primarily-CLI is wrong; what's missing is concentrated, not pervasive.

---

## Method

Three exploration passes covered the spec in parallel:

- **CLI surface** — parser, variables, HTTP execution, assertions, retry, parallel, GraphQL, WebSocket, OpenAPI, output formats, project commands, watch mode, plugins, perf, dynamic functions, faker, error catalog, exit codes.
- **Backend and web surface** — C#/.NET service, licensing service, dashboards, billing, team management, scheduled runs, PR checks, notifications, SSO, audit logging, telemetry.
- **AI affordances and cross-cutting concerns** — markdown sentinel-splice, `curlew exec --stdin`, the bundled Claude skill, JSON output for orientation commands, event streams, JSONL logging, sensitive-value redaction, smoke and CI gates.

Each pass cross-checked spec sections against `internal/` packages, `src/ApiTool.Backend/`, `web/`, and the completed task records in `management/backlog.yaml`. The highest-stakes findings were verified directly by reading the implicated files.

The audit's scope is roughly 95% of the spec by line count. Sections covering pricing strategy, conversion-tracking philosophy, and partner-integration narratives were treated as input to product decisions rather than implementation surface.

---

## Executive summary

Curlew is further along than its prior assessment indicates. 151 vertical slices have shipped across milestones M1–M13 plus M17 (M14–M16 and M18–M19 not yet generated); the CLI is largely production-ready; the backend has working auth, RBAC, scheduling, SSO, and audit modules; the web dashboard renders real data. Twenty-five gaps were identified in this audit, distributed unevenly across importance tiers (status updates from subsequent milestones noted inline below):

| Tier | Definition | Count |
|---|---|---|
| T1 | Revenue-critical: paid tiers cannot operate without these | 5 |
| T2 | Spec-promised CLI completeness: documented as if available, not built | 5 |
| T3 | Workflow completion: scaffolding exists but is not wired through | 9 |
| T4 | Compliance and post-launch | 4 |
| T5 | Strategic / polish | 4 |

Five gaps concentrate the project's launch risk: license JWT issuance, the live Stripe gateway, the Stripe webhook handler, the SMTP provider, and CLI ↔ backend integration. Without these, the paid tiers do not function end-to-end no matter how complete the surrounding surface is.

The next-most-urgent cluster is the dynamic-function rollout. The manual already documents helpers (`$base64`, `$hmacSha256`, etc.) as if available; the code does not implement them. This is a small amount of engineering work but a meaningful credibility issue for the documentation.

Beyond those two clusters, the remaining gaps are real but bounded — they slot cleanly into discrete milestones with predictable scope.

---

## What's complete

A short factual catalogue, not a victory lap. Items in this list have been verified to exist in code and (where relevant) to appear in completed tasks in `management/backlog.yaml`.

### CLI

Parser (YAML schema, `curlew.yaml`, environments, `include:` composition, request items, setup/teardown, data-driven blocks). Variable system (the full ten-level precedence, `from_command`, vault provider profiles, dynamic auth profiles, sensitive ratchet, redaction). HTTP execution (all body forms including JSON, multipart, form-encoded, raw binary, `body_file`, `body_binary_file`, TLS options, redirects, timeouts). Assertions (status, headers, JSONPath body, JSON Schema body, timing, all operators). Extraction (JSONPath, save_to file output). Retry (exponential, linear, constant backoff with jitter, conditional rules, `respect_retry_after`). Data-driven testing (CSV, JSON, YAML data sources, iteration, skip-on-failure, aggregation). Parallel execution (six-phase dependency analysis, `depends_on`, `--parallel`, wave computation, race detection). GraphQL (queries, mutations, subscriptions, fragments, error array handling). WebSocket (connect, send, expect, heartbeat at Professional tier, reconnect). OpenAPI import. All six output formats — terminal, JSON, TAP, JUnit, HTML, Markdown with sentinel-splice. Project utility commands — `validate`, `extract`, `info`, `schema`, `init` including `--skill claude` and `--output <fmt>`, `test`, `--only`. Watch mode. Plugin host (JSON-RPC, all hooks, handshake, discovery, signal handling). Performance testing (`curlew perf`) and distributed worker mode (`curlew worker`). JSONL logging (`--log`) with correlation IDs (`run_id`, `request_id`). Event stream (`--events`). Tier gating with the full exit-code table (0, 1, 2, 3, 4, 5, 6, 9, 10, 130). Grace-period state machine. Sensitive-value redaction with heuristic name matching, propagation through interpolation, and `!sensitive` YAML tag support. JSON Schema for collections. Smoke tests and the `ci-local.sh` gate.

### Backend (C#/.NET)

Local password authentication (Argon2id with timing-safe placeholder hash). Bootstrap admin user seeding (`BOOTSTRAP_ADMIN_EMAIL` / `BOOTSTRAP_ADMIN_PASSWORD`). Organization CRUD endpoints. Member seat management with seat-limit enforcement. RBAC — three built-in roles plus custom-role CRUD. Invitations (create, resend, accept). Audit logging via `AuditCaptureMiddleware` with paginated query endpoints. Subscription CRUD (`POST checkout`, `GET`, `PATCH`, `DELETE`, `POST {id}/reactivate`, `POST portal`) — backed by an in-memory `FakeStripeGateway` that supports proration calculations. Scheduling endpoints (cron storage and CRUD, `SchedulerHost` background service). Coordinator (job/shard endpoints, `ShardReaper`, heartbeat). Rate limiting (13 fixed-window policies). Notifications dispatcher with Slack webhook posting and `IResultIngestedNotifier`. SAML 2.0 (configuration endpoints, assertion verification, session-token issuance). OIDC (discovery, token exchange, callback). On-prem self-hosted bundle in `deploy/self-hosted/` (Postgres, Redis, backend, web, healthchecks, volume persistence).

### Web (SvelteKit)

Working pages: results with computed trends, members management with `requireOrgAdmin` guards, billing UI scaffolding, audit log with pagination, PR checks display, settings (SSO setup forms for SAML and OIDC, custom roles, notifications rule creation).

### AI affordances

Markdown sentinel-splice with the full ten-section structure and byte-for-byte preservation outside sentinels (`internal/output/markdown/formatter.go`, `splice.go`, six handled cases). `curlew exec --stdin --format json` (one-shot agent execution). `curlew init --skill claude` with embedded skill content at `templates/skills/claude/curlew/SKILL.md`. JSON output for `curlew info` and `curlew schema`. Event stream and JSONL logging with correlation IDs that thread through markdown sentinels.

---

## The gap inventory

Twenty-five gaps. Each entry: spec reference, status, evidence, and impact.

### CLI gaps

**1. Dynamic-function rollout — Phases 2 through 4.** `internal/variable/dynamic.go` registers 16 functions: timestamps, UUIDs, basic random primitives, basic faker primitives. The spec describes three further phases that are entirely absent.

- **1a. Phase 2 — encoding and date arithmetic.** ~~Missing: `$base64`, `$base64Decode`, `$urlEncode`, `$jsonEncode`, `$dateAdd`, `$dateSubtract`, `$formatDate`, `$parseDate`.~~ **CLOSED by M12** (M12-002, M12-003, M12-006, M12-007). All eight functions ship. Spec ref: `SPECIFICATION.md:746`.
- **1b. Phase 3 — hashing and advanced strings.** ~~Missing: `$sha256`, `$md5`, `$hmacSha256`, `$randomPassword`, `$randomBase64`.~~ **CLOSED by M12** (M12-004, M12-005, M12-008). `$hmacSha256` includes sensitive-key propagation. Spec ref: `SPECIFICATION.md:747`.
- **1c. Phase 4 — faker integration.** ~~Missing: roughly 37 of the 53 faker functions described in `SPECIFICATION.md:751–847` across personal, location, company, internet, content, financial, and file-data categories.~~ **CLOSED by M13** (M13-001 through M13-008). All 53 functions ship across the seven categories, en-US only. Locale support (`SPECIFICATION.md:961–1006`) shipped from M20-001 — the `--locale` flag was **unparsed (silently ignored)** before M20, never warn-and-ignore; M20-001 builds the resolver from zero with de-DE as the first non-en-US pool.

**2. `curlew license --refresh` and `--debug` are stubbed.** Both return `"Not yet implemented: --refresh"` (or `--debug`) at `cmd/curlew/license.go:28`. Help text and spec advertise both. Impact: **MEDIUM**. `--debug` blocks the documented troubleshooting workflow at `MANUAL.md:3097` ("`curlew license --debug` shows the current state"). `--refresh` blocks the explicit re-validation flow.

**3. Plugin loading is not tier-gated.** Spec mandates plugin loading as Enterprise-only (`MANUAL.md:3066`). The code does not enforce this. `internal/auth/registry.go` has no `plugin_loading` feature entry; `CURLEW_PLUGINS` loads at any tier. Impact: **MEDIUM**. Direct revenue leak for the Enterprise tier; users on lower plans can use plugins for free.

**4. First-party plugins not shipped.** ~~`curlew-sigv4` appears as the canonical example at `MANUAL.md:2761` but no binary or source is in this repo.~~ **CLOSED by M17** (M17-001 through M17-005), with rescoped framing: SigV4 and OAuth1 ship as built-in **signers** (request-level `signing:` field) rather than plugin binaries; webhook signatures (Stripe, GitHub, Slack) and JWT decode ship as built-in **dynamic functions** (`$webhookSign.*`, `$jwtDecodeHeader`, `$jwtDecodeClaims`). All universally available across tiers — not Enterprise-gated. The no-scripting story for the four canonical signing scenarios is now closed without shipping any plugin binaries. See `.claude/skills/backlog/milestone-mapping.md:415–495` for the rescope rationale.

### Backend gaps

**5. License JWT issuance is missing.** The CLI verifies JWTs (`internal/license/jwt.go`) using an embedded public key. The backend has no endpoint to *issue* them. There is no `/api/v1/license/jwks`, no token-minting flow, no integration between subscription state and a `Claims`-shaped JWT. Impact: **CRITICAL**. Without issuance, no paid tier can be activated end-to-end. The CLI can verify whatever you hand it; the backend cannot hand it anything.

**6. Stripe live gateway is a stub.** `src/ApiTool.Backend/Subscriptions/StripeGateway.cs:17` throws `NotImplementedException` on every method, with comments reading `"StripeGateway is a placeholder. Implement a real Stripe SDK integration before setting Stripe:Mode=live."` The interface and `FakeStripeGateway` (in-memory) work in development; integration with the live Stripe SDK does not. Impact: **CRITICAL**. No actual money can change hands.

**7. Stripe webhook handler is missing.** No POST endpoint for `customer.subscription.*` events. No signature verification on incoming webhooks. The full flow is described at `SPECIFICATION.md:6748–6788`. Impact: **CRITICAL**. Without webhooks, the backend cannot reflect Stripe state changes (cancellations, payment failures, plan changes); subscription state will silently drift from truth.

**8. SMTP / email provider is a no-op.** `ISmtpSender` interface exists; the wired default is `NoopSmtpSender`. Impact: **HIGH**. Blocks: password reset, email verification, email notifications, billing receipts. None of these flows can complete.

**9. Password reset and email verification flows missing.** No endpoints, no service layer. Tied to (8). Impact: **HIGH**. Account recovery is impossible; new users cannot verify their emails.

**10. Trial JWT issuance and on-demand trials missing.** Spec describes seven-day per-feature trials and post-expiry on-demand re-trials at `SPECIFICATION.md:5666`. No service issues these tokens. The `Claims` struct in `internal/license/` has neither `trial_state` nor `trial_expiry` fields. Impact: **HIGH**. The conversion funnel depends on trials; they cannot run without this.

**11. Schedule executor is missing.** Schedule endpoints (`SchedulesEndpoints.cs`) exist and store cron expressions; `SchedulerHost` runs as a background service. But nothing actually invokes `curlew run` against a stored schedule. Impact: **HIGH** for Team tier. Scheduled runs is a Team-tier promise that does not currently keep itself.

**12. PR check posting to GitHub/GitLab missing.** `PrChecksEndpoints.cs` stores PR-check state internally; there is no outbound integration that calls the GitHub/GitLab Checks API. The CLI `pr-check` command exists but does not call the backend or external APIs from a real test run. Impact: **HIGH** for Team tier. PR-check integration is another Team-tier promise that is currently inert.

**13. CLI ↔ backend integration is largely missing.** The CLI does not call the backend for license fetch, subscription state, or PR-check posting on test completion. Local license verification works; remote license refresh does not. Impact: **HIGH**. Decouples the CLI from the service layer; users have no way to receive an updated license without manually re-exporting one. This is the connective tissue that turns the backend from an island into part of the tool.

**14. Shared vault templates for Team tier missing.** Spec mandates Team-tier shared vault config (`MANUAL.md:3063`). No backend endpoints, no propagation to CLI. Impact: **MEDIUM** for Team tier.

**15. Health metrics dashboard missing.** Spec describes Team-tier dashboards including health and trends. The web frontend has results and audit pages but no dedicated `/dashboard` or aggregate-metrics page. Impact: **MEDIUM**.

**16. SSO is not Enterprise tier-gated.** The Organization schema does not have a tier column, so SSO endpoints are usable at any subscription level. Impact: **MEDIUM**. Direct revenue leak; SSO is supposed to be Enterprise-only and currently is not.

**17. Audit log bulk export missing.** `AuditLog` data is stored and queryable per-page; no bulk export endpoint or time-range optimization for Enterprise customers who want to feed their SIEM. Impact: **LOW–MEDIUM** depending on enterprise customer requirements.

### Compliance and post-launch gaps

**18. GDPR data export and personal user deletion missing.** Org deletion exists; per-user export and deletion do not. Impact: **HIGH for an EU launch; LOW for early pilots.**

**19. Telemetry and conversion tracking missing.** Spec describes a three-phase rollout: Phase 1 (blind), Phase 2 (minimal attribution), Phase 3 (optional telemetry with consent). None implemented. Impact: **MEDIUM**. Without any conversion tracking, the team cannot measure which feature gates drive registration — exactly the question the strategic-tracking section of the spec was designed to answer.

**20. SOC 2 Type II and ISO 27001 artifacts missing.** Planned for Enterprise but not present. Impact: **LOW until the Enterprise sales pipeline opens; HIGH then.**

### AI affordances and cross-cutting gaps

**21. Manual-as-skill not delivered.** The bundled Claude skill at `templates/skills/claude/curlew/SKILL.md` is short. There is no infrastructure to deliver `MANUAL.md` (or a derivative) as a skill payload. Spec doesn't strictly require this. Impact: **LOW**. Strategic — the project's distinctive AI bet is well-served by the existing Claude skill, and a manual-as-skill is an enhancement not a requirement.

**22. MCP server interface not present.** No spec requirement; design choice favors JSON-RPC plugins. Impact: **LOW**. Listed for completeness only.

### Strategic gaps surfaced in prior analysis

These appear in `ASSESSMENT.md` and `SCRIPTING.md` and are carried forward here because they belong in the inventory.

**23. `from_command` tier placement.** Currently Solo. The assessment argues it should be free to make the no-scripting story coherent — the only escape valve from declarative YAML should not also be a paywall. Impact: **MEDIUM**, mostly product/pricing.

**24. `if:` conditional-execution field on request items.** Per `SCRIPTING.md`, the smallest move that closes the largest residual gap (response-content branching). Not in the spec today. Impact: **MEDIUM**. Closes a real gap that scripting otherwise absorbs, with very small implementation cost.

**25. CEL or Starlark expression evaluator for `extract` and `assertions`.** Per `SCRIPTING.md`, the second-step move that would close cross-field aggregation cases without embedding general scripting. Larger move; only worth doing once the dynamic-function set ships. Impact: **LOW** until faker / encoding / hashing land.

---

## Ranking

A multi-axis ranking. Each gap receives a tier:

- **T1 — revenue-critical.** Without these, the paid tiers cannot operate end-to-end. Block launch of any paid plan.
- **T2 — spec-promised CLI completeness.** Documented in the manual as if available; users will hit them and be confused.
- **T3 — workflow completion.** Endpoints or scaffolding exist but are not wired through to a working user experience.
- **T4 — compliance and post-launch.** Required for public launch in regulated markets or enterprise sales; not blocking for pilots or beta users.
- **T5 — strategic / polish.** Worth doing but not urgent; quality-of-life or differentiation.

| # | Item | Tier | Rationale |
|---|---|---|---|
| 5 | License JWT issuance backend | T1 | Cannot activate any paid tier |
| 6 | Stripe live gateway | T1 | Cannot accept payment |
| 7 | Stripe webhook handler | T1 | Subscription state will drift |
| 8 | SMTP / email provider | T1 | Blocks password reset, verification, billing receipts |
| 13 | CLI ↔ backend integration | T1 | License refresh, subscription state, PR-check posting all depend on this |
| 1a | Dynamic-Function Phase 2 | T2 | ✅ Closed by M12 |
| 1b | Dynamic-Function Phase 3 | T2 | ✅ Closed by M12 |
| 1c | Dynamic-Function Phase 4 (faker) | T2 | ✅ Closed by M13 (en-US); locale shipped from M20-001 (--locale was unparsed pre-M20) |
| 2 | `license --refresh` / `--debug` | T2 | Help text and spec advertise; users will try and fail |
| 3 | Plugin tier gate | T2 | Spec/help text say Enterprise; code allows free |
| 9 | Password reset / email verification | T3 | Account recovery — needed for any real users |
| 10 | Trial JWT issuance / on-demand trials | T3 | Conversion funnel; needed before public launch |
| 11 | Schedule executor | T3 | Team-tier promise; endpoints exist but inert |
| 12 | PR check posting to GitHub/GitLab | T3 | Team-tier promise; endpoints exist but no external API |
| 14 | Shared vault for Team tier | T3 | Team-tier promise |
| 15 | Health metrics dashboard | T3 | Team-tier surface |
| 16 | SSO Enterprise tier gate | T3 | Revenue leak; small change |
| 4 | First-party plugins (SigV4, OAuth1, JWT decode, webhook-sig) | T3 | ✅ Closed by M17 — shipped as built-in signers + dynamic functions, not plugin binaries |
| 23 | `from_command` to free tier | T3 | Product/pricing; tied to no-scripting story |
| 17 | Audit log bulk export | T4 | Enterprise nice-to-have |
| 18 | GDPR export and user deletion | T4 | EU launch blocker, not pilot blocker |
| 19 | Telemetry / conversion tracking | T4 | Measure conversion; safe to defer past launch |
| 20 | SOC 2 / ISO 27001 | T4 | Enterprise sales requirement, not technical |
| 21 | Manual-as-skill | T5 | Polish |
| 22 | MCP server | T5 | Not spec'd; only worth doing if customer demand emerges |
| 24 | `if:` conditional-execution field | T5 | Closes residual gap from `SCRIPTING.md`; worth doing after T1–T2 |
| 25 | CEL / Starlark expression evaluator | T5 | Larger move; defer until dynamic-function set ships |

---

## Proposed milestone structure

Each milestone is a coherent vertical of work. Sized so any one of them produces a shippable, defensible improvement. Numbering is sequential from the latest shipped milestone (M11), not by tier. The ranking-by-tier table above governs priority within and across milestones.

**M12 — Foundational dynamic-function helpers.** Items 1a and 1b. Argument-syntax foundation for the dynamic-function evaluator (slice 1) plus 13 helper functions across encoding (`$base64`, `$base64Decode`, `$urlEncode`, `$jsonEncode`), hashing (`$sha256`, `$md5`, `$hmacSha256`), date arithmetic and formatting (`$dateAdd`, `$dateSubtract`, `$formatDate`, `$parseDate`), advanced strings (`$randomPassword`, `$randomBase64`). Closes the documentation/implementation drift in MANUAL.md (which already shows these as if available) and unblocks first-party signing plugins in M17. **Generated as M12-001 through M12-008 — see `management/tasks/M12-*.yaml`.**

**M13 — Faker depth.** Item 1c: ~37 realistic-data faker functions across personal, location, company, internet, content, financial, file-data categories. High-volume mechanical work; parallelisable across many small slices. Does not block any other milestone — can ship opportunistically once M12 lands.

**M14 — Revenue plumbing.** All T1 items: license JWT issuance, Stripe live gateway, Stripe webhook handler, SMTP provider, CLI ↔ backend integration. The single largest milestone outside M12/M13. Every paid tier depends on its completion. Sub-vertical-slices: license issuance + JWKS endpoint, live Stripe gateway behind feature flag, webhook handler with signature verification, SMTP via SendGrid or AWS SES, CLI auth + license refresh + PR-check posting integration. **Needs a design pass before `/backlog M14` because the spec doesn't pin down JWT claims structure, Stripe SDK strategy, SMTP provider choice, or the CLI ↔ backend API contract.**

**M15 — CLI partial fixes.** T2 items 2 and 3: implement `license --refresh` and `--debug`; add the `plugin_loading` feature gate. Small, fast wins. `license --refresh` depends on M14's issuance endpoint.

**M16 — Workflow completion.** T3 items 9–16 and 23: password reset, email verification, trial issuance / on-demand trials, schedule executor, PR check posting to GitHub/GitLab, shared vault for Team tier, health metrics dashboard, SSO Enterprise gate, `from_command` retiering. The "Team tier actually works" milestone. Each item is independently deliverable but several need design passes before `/backlog M16` (password-reset flow, schedule-executor architecture, PR-check posting model).

**M17 — First-party plugins.** Item 4 plus the first-party plugin work argued for in `SCRIPTING.md`: `curlew-sigv4`, `curlew-oauth1`, `curlew-jwt-decode` (or fold the JWT decoder into M12 as a dynamic function), `curlew-webhook-sig` (Stripe / GitHub / Slack signing helpers). Closes the no-scripting story by giving free-tier users actual escape hatches. Depends on M12 (`$hmacSha256` is the building block for the webhook helpers).

**M18 — Compliance and launch readiness.** T4 items: GDPR export and user deletion, telemetry phases 1–3, audit log bulk export, SOC 2 / ISO 27001 prep work. Pre-public-launch.

**M19 — Strategic expressiveness.** T5 items 24 and 25: the `if:` conditional-execution field, then evaluation of CEL or Starlark for `extract` and `assertions`. Only after M12 has shipped and at least M17's first plugins are in production. The remaining T5 items (manual-as-skill, MCP server) sit here too if ever pursued.

---

## Recommended order of attack

Sequenced by dependency and priority, not by time.

**First — M12 (Foundational dynamic-function helpers).** Tasks generated as M12-001 through M12-008. Closes the documentation/implementation drift in the manual and is the cheapest unblock for M17 (first-party plugins). M12-001 (argument-parsing foundation) is a strict prerequisite for the seven downstream slices, which are mutually independent and parallelisable.

**Next — M14 (Revenue plumbing) after a design pass.** Cannot launch paid tiers without this. Open design questions before `/backlog M14`: JWT claims structure (`trial_state`, `trial_expiry`, feature flags), Stripe SDK strategy (test mode vs recorded responses vs `FakeStripeGateway`), SMTP provider choice (SendGrid vs SES), CLI ↔ backend API contract (REST endpoints, auth scheme, refresh flow). Once those are decided and folded into the spec, `/backlog M14` can decompose the milestone cleanly.

**In parallel with M14 — M15 (CLI partial fixes).** Plugin tier gate is trivial; `license --debug` is mostly clear; `license --refresh` blocks on M14's issuance endpoint design but the `--debug` and tier-gate slices can ship independently.

**After M12 lands — M17 (First-party plugins).** Once `$hmacSha256` ships, the webhook-signature plugins are straightforward. SigV4 and OAuth1 are well-specified and can run in parallel with each other.

**M13 (Faker depth)** can ship opportunistically — high-volume mechanical work in small slices, does not block any other milestone.

**M16 (Workflow completion)** comes after M14 because most of its slices depend on revenue-plumbing infrastructure (trial issuance, schedule executor, PR check posting all need the backend ↔ CLI integration). The trivial subset (SSO tier gate, `from_command` retiering) can split out earlier.

**M18 (Compliance) and M19 (Strategic expressiveness)** are deliberately deferred. M18 should begin only when a public launch is queued; the compliance work has a half-life and needs to land close to launch. M19 (the `if:` field, CEL evaluation) should begin only after M12 ships and at least M17's first plugins are in production — by then, real user feedback will sharpen the case for or against expression-language work.

---

## Notes on what was not in scope

This audit covers the implementation surface. It does not re-litigate decisions covered elsewhere:

- The no-scripting design opinion is analyzed in `SCRIPTING.md`. This document accepts that analysis and only inventories the implementation status.
- Pricing tier placement is treated as a product question. Items 23 (`from_command` to free) and 16 (SSO to Enterprise) are noted because they are concrete tier-matrix changes; the broader question of whether the five-tier model is right for the market is for `ASSESSMENT.md` to address.
- The assessment's recommendation to give the Markdown sentinel-splice its own "novel bets" section in `ASSESSMENT.md` is a documentation change, not an implementation one. Out of scope here.

---

## What this means for `management/backlog.yaml`

The natural next step is to generate task definitions for the remaining milestones using the existing `/backlog` skill workflow. Status:

1. **M12 — DONE.** `management/tasks/M12-001.yaml` through `M12-008.yaml` shipped; all eight tasks merged. `dynamic_function_helpers` capability complete.
2. **M13 — DONE.** All eight slices shipped. 53 faker functions across seven categories, en-US only. Locale support deferred to a future milestone.
3. **M14 — DONE.** SPECIFICATION.md v4.2 + v4.2.1 design pass landed; all 21 slices (M14-001 through M14-021) merged. License JWT issuance, Stripe live gateway, Stripe webhook handler, SMTP via SendGrid, CLI ↔ backend integration, GitHub Checks API integration. See `docs/M14_INVESTIGATION.md` for the design-pass record.
4. **M15 — DONE.** All three slices (M15-001 plugin Enterprise gate, M15-002 SSO Enterprise gate, M15-003 `from_command` Solo→Free) merged. Closes REVIEW.md gaps 3, 16, 23.
5. **M16 — design pass complete (2026-05-07); `/backlog M16` unblocked.** SPECIFICATION.md v4.3 shipped covering password reset + email verification, trial persistence + on-demand re-trials, schedule execution model (self-hosted runner), GitLab Commit Status API integration, shared vault backend propagation, tier-gate generic abstraction, dashboard response schemas. M16 entry added to `.claude/skills/backlog/milestone-mapping.md` with eight capability clusters totaling 21 slices. See `docs/M16_INVESTIGATION.md` for the audit trail (questions A–I) and the v4.3 Decisions Resolved table for the resolutions (`v3-1` through `v3-15`, plus seven GitLab-specific `GL-*` decisions).
6. **M17 — DONE.** Rescoped during planning: ships as built-in signers (`signing:` request field + AWS SigV4 + OAuth1) plus built-in dynamic functions (`$webhookSign.stripe/.github/.slack`, `$jwtDecodeHeader`, `$jwtDecodeClaims`). No plugin binaries shipped. All five slices (M17-001 through M17-005) merged. Universally available across tiers — not Enterprise-gated.
7. **M18 (Compliance) and M19 (Strategic expressiveness)** — defer until launch queue is set.

Note on the previous review draft: an earlier version proposed labels "M13a" and "M13b" for splitting the dynamic-function rollout. The actual numbering uses M12 (foundational helpers) and M13 (faker depth) — sequential, regex-clean, matches the existing convention. The split itself was kept.

If something specific in the spec was expected to appear in this audit and did not, name it. The exploration covered roughly 95% of the spec by line count, but that final 5% sometimes hides exactly what someone is looking for.
