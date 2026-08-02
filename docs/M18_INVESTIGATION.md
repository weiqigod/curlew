# M18 (Compliance and Launch Readiness) — Pre-Backlog Investigation

Investigation date: 2026-05-17 (skeleton + first verification pass)
Design pass landed: **TBD** — see `docs/SPECIFICATION.md` vX.Y Changelog and "Decisions Resolved (vX.Y, all M18 themes)" subsection (to be added).

> **Note on line numbers:** This audit was written against `docs/SPECIFICATION.md` at the version current on 2026-05-17 (post-v4.3, post-M19). Subsequent spec revisions may shift line offsets; prefer section names when navigating later.

Scope: ground REVIEW.md's M18 framing in the current code, surface design questions before `/backlog M18` is run.

Sources audited (first verification pass complete; flagged TBDs remain): `docs/REVIEW.md`, `docs/SPECIFICATION.md`, `docs/MANUAL.md`, `docs/M14_INVESTIGATION.md` and `docs/M16_INVESTIGATION.md` (templates), `src/ApiTool.Backend/{Audit,Organizations,Data/Entities,Rbac,GitHub,GitLab,Licensing/Keys,VaultConfig,Notifications}/`, `internal/{output,backend,license,worker}/`, `cmd/curlew/`, `web/src/routes/(app)/org/[slug]/`, `.claude/skills/backlog/milestone-mapping.md`, `management/backlog.yaml`.

---

## Why this file exists

`/backlog M18` is blocked. Two independent blockers, the same shape as M14's and M16's:

1. **Skill gate.** `.claude/skills/backlog/SKILL.md` requires an entry in `milestone-mapping.md`. M18 is not there — only M1–M5, M12, M13, M14, M15, M16, M17, M19 are. **Action:** add a `## Phase 18 — Compliance and launch readiness (M18)` section before `/backlog M18`.
2. **Design pass missing.** `docs/REVIEW.md:195` and `:241` defer M18 with the framing "should begin only when a public launch is queued; the compliance work has a half-life and needs to land close to launch." That deferral was reasonable while M16/M17/M19 were still in flight; with those shipped, M18 is now the only remaining pre-launch milestone. The spec covers some M18 surfaces extensively (telemetry, encryption-at-rest for some columns) and others thinly (GDPR data inventory, SOC 2 artifacts, audit-log bulk export semantics).

This file is the investigation behind that design pass. The first verification pass (2026-05-17) audited backend, CLI, web, and spec; the cross-cutting question list, slice-count estimate, and spec-edit list are anchored in real findings below.

---

## Summary

REVIEW.md:195 frames M18 as "T4 items: GDPR export and user deletion, telemetry phases 1–3, audit log bulk export, SOC 2 / ISO 27001 prep work." After verification, that framing needs three corrections:

- **Audit-log CSV export already exists** as a query-parameter on the paginated endpoint (`?format=csv`) and as a web download button. The real gap is bulk-scale (>200 rows), JSONL format, tier-gating, and async export — narrower than "no export exists."
- **Telemetry is extensively specified** at `SPECIFICATION.md:6091–6809` (three phases, retention, CLI surface for Phase 3 GDPR rights). Phase 1 explicitly sends nothing — it's "accept strategic blindness." This collapses the Phase 1 implementation work to zero. Real work concentrates in Phase 3.
- **Faker locale support drift** (REVIEW.md §1c) is **not an M18 item** — the spec already documents 16 locales at `SPECIFICATION.md:949–994`. The code is behind the spec, but the compliance/launch frame is wrong for it. Carry as a separate small follow-up milestone.

| Source | Item | M18 disposition |
|---|---|---|
| REVIEW.md gap 17 | Audit log bulk export | **In scope** — narrower than originally stated (CSV-via-query-param exists) |
| REVIEW.md gap 18 | GDPR per-user export + deletion | **In scope** — spec coverage at `:6643–6710` is thin; full data inventory missing |
| REVIEW.md gap 19 | Telemetry / conversion tracking, Phases 1–3 | **In scope (Phase 3 only)** — Phase 1 is a no-op by design; Phase 2 is registration UX, not telemetry; Phase 3 is the implementation work |
| REVIEW.md gap 20 | SOC 2 Type II / ISO 27001 prep | **In scope** — spec at `:6711–6720` only marks "Planned (Phase 4/5)"; no controls, no vendor inventory |
| `SPECIFICATION.md:5658, :9321` (v3-10) | Server-side AES-256-GCM for `team_vaults.template_jsonb` | **In scope** — `IKmsClient` envelope pattern already exists; small slice |
| `SPECIFICATION.md:11057` | Richer secrets-per-schedule story (`schedules.env_vars`) | **In scope** — explicitly named M18 deferral; same envelope pattern applies |
| `SPECIFICATION.md:9076` | GitHub Enterprise Server (GHES) support | **Out of scope** — Enterprise feature, not a compliance gate; spec says "M18 or later," recommend "later" |
| `SPECIFICATION.md:10241` | Hourly-rollup materialised view for health dashboard | **Out of scope** — performance optimisation |
| `SPECIFICATION.md:10245` | p99 latency / error-rate / response-size charts | **Out of scope** — feature enhancement |
| REVIEW.md §1c, `SPECIFICATION.md:949–994` | Faker locale code-vs-spec drift | **Out of scope** — own follow-up milestone; spec is already complete, code lags |

The four T4 gaps plus the two encryption deferrals (`team_vaults`, `schedules.env_vars`) are the **core** of M18. The rough slice count is **13–14 slices** (revised down from the 13–18 initial estimate based on the audit pass — see Slice-count estimate below).

---

## Gap-by-gap audit

### Gap 17 — Audit log bulk export

**What exists today:**

- `AuditCaptureMiddleware` at `src/ApiTool.Backend/Audit/AuditCaptureMiddleware.cs:8-33` populates a scoped `AuditContext` with IP and User-Agent; registered after `UseAuthentication`.
- Paginated query endpoint `GET /api/v1/organizations/{orgId}/audit-log` at `src/ApiTool.Backend/Audit/AuditLogEndpoints.cs:10-86`. Query parameters: `event_type`, `user_id`, `from`, `to`, `limit`, `format`. `DefaultLimit=50`, `MaxLimit=200` (`AuditLogQueryService.cs:10-11`). Ordering: `OrderByDescending(e => e.CreatedAt).Take(limit)`. **No cursor / offset pagination — limit-only model.**
- **CSV export already shipped** at `AuditLogEndpoints.cs:76-82`: `if (string.Equals(format, "csv", ...))` returns CSV with `Content-Disposition: attachment; filename=audit-log-{orgId}-{date}.csv`. Web side: `audit-log/export/+server.ts` (42 lines) + button at `web/src/routes/(app)/org/[slug]/audit-log/+page.svelte` (`data-testid="audit-log-export"`).
- Entity at `src/ApiTool.Backend/Data/Entities/OrganizationAuditLogEntry.cs:4-54` carries `Id`, `OrgId`, `ActorId`, `ActorEmail`, `EventType`, `PayloadJson`, `TargetType`, `TargetId`, `PreviousStateJson`, `NewStateJson`, `IpAddress`, `UserAgent`, `Success`, `FailureReason`, `CreatedAt`. (Notable: `PreviousStateJson` + `NewStateJson` give first-class change-tracking; useful for SOC 2 audit-of-changes.)
- **No `AuditLogCleanupHost`** found. Webhook-event tables have 30/90-day cleanup hosts; audit log has none.
- **No RBAC permission for audit log.** `Rbac/Permissions.cs:8-141` has no `audit_log.view` or `audit_log.export` entry. Access is hardcoded to Owner/Admin at `AuditLogQueryService.cs:24` (`if member is null or member.Role == OrgRole.Member`).

**What's missing on the backend:**

- **Bulk-scale support.** Existing CSV export inherits `MaxLimit=200` — so it's "download this page as CSV," not bulk. Need either (a) lift the cap for `?format=csv|jsonl` exports, or (b) streaming endpoint with `Transfer-Encoding: chunked`.
- **JSONL format.** Only CSV today. SIEM-friendly format is JSONL.
- **Tier gate.** No `Enterprise`-tier check on the export.
- **Retention policy + cleanup host.** Need a per-org `audit_log_retention_days` (or a deployment-level default) and an `AuditLogCleanupHost` background service.
- **RBAC for audit-log access.** Add `audit_log.view` + `audit_log.export` permissions so a "security auditor" custom role can hold only audit-log rights (separation of duties — SOC 2 requirement).

**What spec says — and where it's loose:**

- "Subscription Audit Log" section at `SPECIFICATION.md:7637–7654` documents 7 event types for subscription state changes. Four audit-log tables defined in the schema appendix: `user_audit_log` (`:10498`), `auth_audit_log` (`:10657`), `subscription_audit_log` (`:10763`), `organization_audit_log` (`:11176`). Each has a `created_at` index.
- **No bulk-export endpoint specified in the API section.** No SIEM integration narrative. No JSONL vs CSV decision. No tier-gating. No sync-vs-async export model.

**Genuinely open design questions:**

1. **Sync vs async export.** A 5M-row org pulling a year of audit log is too large for a synchronous response. Either (a) stream JSONL with `Transfer-Encoding: chunked` (simple, no infrastructure), (b) queue a job, email a signed S3 URL when ready (SIEM-standard, requires storage), or (c) hybrid: stream <100k rows sync, queue otherwise. Recommendation: (a) for v1, since the existing CSV path is already synchronous and customers downloading <1M rows can wait.
2. **Format support.** Add JSONL; keep CSV. Recommendation: both. CSV is requested by less-sophisticated buyers; JSONL is SIEM-canonical.
3. **Retention policy default.** SOC 2 typical: 1 year; some frameworks want 7. Recommendation: default 365 days, per-org configurable, with Enterprise-tier ability to extend beyond 365.

### Gap 18 — GDPR per-user data export + deletion

**What exists today:**

- User entity at `src/ApiTool.Backend/Data/Entities/User.cs:4-26` carries `Email`, `PasswordHash`, `IsAdmin`, `CreatedAt`, `EmailVerified` (M16). **Absent:** `DeletedAt`, `AnonymisedAt`, `PendingDeletionAt` — no soft-delete column on `User`.
- **Org-level soft-delete exists.** `MembersService.cs:240-269` marks `OrgStatus.PendingDeletion`; cascade rules are declared in `AppDbContext.cs` as `OnDelete(DeleteBehavior.Cascade)` but **don't fire** because soft-delete sets status, doesn't hard-delete. No scheduled hard-delete job — orgs in `PendingDeletion` stay forever.
- **Last-admin protection partially exists.** `MembersService.cs:183-184`: `if member.Role == OrgRole.Owner: return MemberError.OwnerCannotLeave;` — but this is owner-leave only, not user-delete.
- **User-attributable tables** (entities with `UserId` or `CreatedBy`): `RefreshToken`, `OrganizationMember`, `EmailVerificationToken`, `PasswordResetToken`, `NotificationRule`, `CustomRole`, `Schedule`, `CoordinatorJob`, `TeamVault`, `OrganizationAuditLogEntry` (via `ActorId`/`ActorEmail`), `GithubInstallation`, `GitLabInstallation`. **Every one needs a decision in the data inventory.**
- **No `/api/v1/users/me/export` endpoint.** Grep confirms.
- **No `/api/v1/users/me/delete` endpoint** as a working implementation. (Spec mentions one at `:10169` with guards; not implemented.)

**What's missing on the backend:**

- **Data inventory pass.** For each of the 13 user-attributable tables above, decide: in-export, in-deletion (hard or anonymise), or excluded.
- **Export:** endpoint that produces every personal datum the system holds about a single user — profile fields, membership rows, audit-log entries authored by them, notification preferences, license-history rows. JSON bundle, signed download URL or chunked response.
- **Deletion (right-to-erasure):** add `User.PendingDeletionAt`, `User.AnonymisedAt`; clone the org soft-delete pattern; add a `UserDeletionFinalizerHost` background service to hard-delete after cooldown.
- **Anonymisation function for audit-attributable rows.** When a user is hard-deleted, audit-log `ActorId`/`ActorEmail` should be replaced with an anonymous token preserving audit integrity. New `IUserAnonymiser` service.
- **Self-service surface.** GDPR requires the data subject can request directly. Need both an authenticated `DELETE /api/v1/users/me` (re-auth required) and an admin-mediated `POST /api/v1/organizations/{id}/members/{userId}/gdpr-delete` for admins acting on user behalf.

**What spec says — and where it's loose:**

- "User Data Rights (GDPR/CCPA Compliance)" at `SPECIFICATION.md:6643–6710` covers GDPR via Phase 3 telemetry only: `curlew telemetry delete-request` (30-day window, line 6657), `curlew telemetry export` (immediate, line 6647), 2-year telemetry retention (line 6591).
- Account-settings spec at `:10169` mentions `DELETE /api/v1/users/me` with guards against last-provider and org-owner deletion, soft-delete with 30-day recovery (lines 6671–6672), and "anonymised payment records" + "anonymised survey responses" (6673–6674).
- **What's loose:** no full data inventory across non-telemetry tables; no per-table retention policy; no anonymisation-function semantics (irreversible? logged? reversible during 30-day cooldown?); no re-authentication requirement; no rate-limit / enumeration defense for the export endpoint.

**Genuinely open design questions:**

4. **Hard delete vs anonymise.** Audit-log entries authored by a deleted user — preserve as `<deleted user>` (anonymise, retain audit integrity) or hard-delete (breaks "who did this" trail)? Recommendation: anonymise audit-attributable rows, hard-delete everything else. Spec hints at this pattern with "anonymised payment records" at `:6673`.
5. **Cooldown period.** Spec already commits to 30 days (`:6671`). Confirm we adopt this for user deletion, with email notification on initiation and on completion.
6. **Last-admin protection extension.** Current code blocks owner-leave; M18 needs to extend to user-delete (block if the user is sole owner of any org with members). Same error shape, broader trigger.
7. **Re-authentication requirement.** Should `DELETE /api/v1/users/me` require a fresh password / TOTP / step-up? GDPR doesn't mandate it, but it prevents a stolen session token from causing irreversible damage. Recommendation: require password re-entry within last-5-minutes window.
8. **Export scope.** Just the user's data, or also data the user can see (org metadata, other members' email addresses)? The latter is broader than GDPR requires and likely should be excluded. Recommendation: export limited strictly to the data subject's own attributable records (the 13 tables above filtered to `UserId = me`).

### Gap 19 — Telemetry / conversion tracking

**What exists today:**

- JSONL logging at `internal/output/jsonl.go:28-51` (`AppendJSONL()` → local file via `os.OpenFile()`, no HTTP).
- Event stream at `internal/output/events/emitter.go:312-327` (`writeEvent()` → provided `io.Writer`, no HTTP).
- **No telemetry SDK, no `posthog`/`mixpanel`/`segment`/`amplitude` import, no `curlew telemetry` subcommand** (CLI subcommand registry at `cmd/curlew/main.go:102-155` lists `run, exec, validate, init, info, schema, watch, vault, pr-check, import, login, license, worker, perf, plugins, internal` — no telemetry).
- **No `install_id` storage anywhere.** Files actually present under `~/.config/curlew/`: `jwks_cache.json`, `license.json`, `team_vault.json`, `pending-uploads/`.
- **No backend telemetry ingest endpoint, no `telemetry_events` table.** Grep confirms.

**What spec says — and where it's loose:**

The spec is **extensively** specified for telemetry:

- **Phase 1** at `SPECIFICATION.md:6091–6150` — "Accept strategic blindness." CDN downloads + GitHub stars + support volume only. **Phase 1 sends nothing from the CLI.** This is a deliberate decision, not a placeholder.
- **Phase 2** at `:6152–6306` — registration prompt + email + conversion survey. UX-side; not CLI telemetry. Targets 15–25% registration rate via feature-gate prompts.
- **Phase 3** at `:6308–6809` — opt-in telemetry post-PMF. Data points fully specified at `:6368–6513` (session-level: version, tier, locale; collection-level: request counts, feature gates, execution time; feature usage: assertions, extractions, faker counts; error categories). Configuration spec at `:6571–6583`. Retention: 2 years, archived after 90 days (lines 6591, 6604).
- **Critical:** spec uses a **per-execution session UUID** (line 6374), NOT a persistent install ID. "Anonymous UUID not considered PII" (line 6365).

**What's still loose:**
- Backend ingest endpoint shape (`POST /api/v1/telemetry/events`, request/response schema).
- GDPR lawful-basis analysis (legitimate interest vs consent) — spec asserts session-UUID is not PII but offers no legal grounding.
- The team-side analytics dashboard (what we use to consume the data).
- Phase-transition triggers — spec documents them but doesn't decide which phases ship in M18 vs post-launch.

**Genuinely open design questions:**

9. **Per-session UUID vs persistent install ID.** Spec says per-execution UUID (privacy-friendlier); typical SaaS telemetry uses persistent install ID (necessary for conversion-funnel cohort analysis). The spec's session-only model produces no funnel data — you can't tell whether the same person ran the CLI 10 times this week or 10 different people ran it once. Either (a) accept the spec, lose funnel analytics, redesign Phase 2 around registration-side cohorting; or (b) revise the spec to add a persistent UUID (still anonymous, still UUID, just stable across runs) and document the lawful basis. Recommendation: (b) — the analytics utility justifies the (minor) privacy delta.
10. **Phase scope for M18.** Phase 1 = no work (already in compliance with spec). Phase 2 = registration UX, mostly already implicit in the existing checkout/auth flow — split investigation needed. Phase 3 = full telemetry pipeline (backend ingest + CLI emitter + storage + dashboard). Recommendation: M18 ships Phase 3 ingest + CLI emitter + storage; Phase 2 registration polish and the team-side dashboard each split to follow-up milestones.
11. **Lawful basis.** Spec asserts session-UUID is non-PII; for a persistent UUID this argument weakens. Decide between legitimate-interest (no consent prompt required, allowed under GDPR for non-PII or for service-improvement purposes) and explicit consent (Phase 3 is already opt-in per spec, so consent fits naturally). Recommendation: opt-in stays as the spec says; the lawful basis is consent, not legitimate interest.

### Gap 20 — SOC 2 Type II + ISO 27001 prep + encryption-at-rest cleanup

**What exists today:**

- **No `docs/SECURITY.md`, `docs/COMPLIANCE.md`, `docs/security/` directory.** Confirmed by ls.
- **Encryption-at-rest is uneven across sensitive columns:**

| Storage site | File | Status |
|---|---|---|
| GitHub App private keys | `GoogleKmsGitHubAppKeyProvider.cs:1-42` | KMS HSM (`RSA_SIGN_PKCS1_2048_SHA256`), sign-only, **never extracted** — strongest |
| GitLab PATs | `GitLabInstallation.cs:28-29` + `GoogleKmsGitLabKeyProvider.cs:1-49` | AES-256-GCM envelope, per-row DEK, KMS-wrapped KEK |
| Signing keys (license JWT) | `SigningKey.cs:19-20` | Optional `KmsKeyId`; **plaintext file fallback** when KMS unavailable |
| Refresh tokens | `RefreshToken.cs:19-20` | SHA-256 hashed (correct model for tokens — not encryption) |
| `team_vaults.template_jsonb` | `TeamVault.cs:13-16` | **Plaintext** (confirmed v3-10 deferral) |
| `schedules.env_vars` sensitive values | per spec `:11057` | "until a richer secrets-per-schedule story emerges in M18" — same envelope pattern needs applying |

- **Generic `IKmsClient` abstraction already exists** at `Licensing/Keys/IKmsClient.cs:8-43` — supports asymmetric signing, envelope encryption, decryption, public-key retrieval. Implementations: `GoogleKmsClient`, `GoogleKmsKeyProvider`, `GoogleKmsGitHubAppKeyProvider`, `GoogleKmsGitLabKeyProvider`. The pattern is established; M18 just applies it to two more columns.
- **RBAC exists** at `Rbac/Permissions.cs:8-141` but lacks audit-log permissions (see Gap 17).
- **Audit logging shipped** (see Gap 17 details).

**What's missing:**

- **Artifacts**, not code. SOC 2 Type II requires demonstrated operating effectiveness of controls over a 6–12 month window. ISO 27001 requires an Information Security Management System (ISMS) with documented policies and risk assessments. Both are paperwork-heavy and audit-driven.
- A vendor pen-test (one-shot, but needed before the audit window).
- Access-review cadence documentation.
- Incident response runbook.
- Vendor inventory (every SaaS the project depends on: SendGrid, Stripe, Google Cloud KMS, AWS, GitHub Apps, GitLab — list assembled from spec mentions but not collated).
- Data flow diagrams.
- **Encryption-at-rest extension** to `team_vaults.template_jsonb` and `schedules.env_vars` using the existing `IKmsClient` envelope pattern.
- **Signing-key plaintext fallback decision.** Currently `SigningKey.KmsKeyId` is optional. For SOC 2 builds, should this be mandatory?

**What spec says — and where it's loose:**

- "Compliance Certifications" at `SPECIFICATION.md:6711–6720`: "SOC 2 Type II — Planned (Phase 4)" and "ISO 27001 — Planned (Phase 5)" with note "For Enterprise tier." That's it. No audit-readiness section, no control inventory, no vendor inventory, no incident-response runbook, no data-flow diagrams.
- Encryption-at-rest deferrals at `:5658` (team_vaults), `:11057` (schedules.env_vars), `:9076` (GHES) are clearly flagged.
- "Data Storage and Security" at `:6677–6684` says "all data at rest encrypted AES-256" — this refers to database-level encryption (RDS at-rest), not column-level. Auditors will distinguish.

**Genuinely open design questions:**

12. **Scope of SOC 2 / ISO 27001 prep within M18.** Two ends of the range:
    - **(a) Minimum viable evidence collection.** Document existing controls, produce policy templates, schedule a vendor pen-test, no code changes. ~2–3 slices.
    - **(b) Audit-readiness.** Engage an auditor, complete a 6-month window, address findings, produce the Type II report. Calendar-bound (cannot accelerate), substantial external cost, scopes out of M18 entirely.

    Recommendation: M18 ships (a). The actual audit engagement is a business-process initiative outside the milestone framework.
13. **Encryption-at-rest cleanup scope.** Fold `team_vaults` and `schedules.env_vars` into one slice using `IKmsClient` envelope (small lift — pattern is in place). Decide separately whether signing-key plaintext fallback gets forced-to-KMS for SOC 2 builds or stays as deployment-mode-dependent (self-hosted can opt out).
14. **Pen-test cadence.** One-time before audit, or annually thereafter? Recommendation: annually; budget for the second engagement before the first SOC 2 Type II window closes.
15. **Audit-log RBAC permission.** Cross-cuts with Gap 17 — lift hardcoded Owner/Admin gate to `audit_log.view` + `audit_log.export` permissions so the SOC 2 evidence story can show separation of duties.

### Carry-along deferrals — final triage

| Deferral | M18 disposition |
|---|---|
| `v3-10` server-side encryption for `team_vaults` | **In scope** — single slice using existing `IKmsClient` envelope |
| Richer secrets-per-schedule (`:11057`) | **In scope** — same envelope pattern, same slice |
| GHES PR-check posting (`:9076`) | **Out of scope** — Enterprise feature, not compliance; defer to a future Enterprise-features milestone |
| Health-dashboard hourly-rollup view (`:10241`) | **Out of scope** — performance optimisation |
| p99 / error-rate / response-size charts (`:10245`) | **Out of scope** — feature enhancement |
| Faker locale code-vs-spec drift (`:949–994`) | **Out of scope** — own follow-up milestone; spec is complete, code lags |

---

## Cross-cutting open questions

Fifteen questions surfaced after the first verification pass. The first ten were anticipated by the skeleton; questions K–O surfaced from reading the code.

| # | Question | Origin gap | Notes |
|---|----------|-----------|-------|
| A | Lawful basis for telemetry (Phase 3) | Gap 19 | Spec asserts session-UUID non-PII; if we adopt persistent install ID, lawful basis must be reconsidered |
| B | Hard delete vs anonymise for audit-log-author rows | Gap 18 | Pattern endorsed by spec at `:6673`; semantics still vague |
| C | Cooldown window for user deletion | Gap 18 | Spec commits to 30 days at `:6671`; just confirm |
| D | Re-authentication requirement for irreversible operations | Gap 18 | Affects login session model |
| E | Bulk export sync vs async vs hybrid | Gap 17 | Affects storage infra (signed URLs require S3-compatible store) |
| F | Encryption-at-rest scope (which columns, which tables) | Gap 20 | Verified: `team_vaults` + `schedules.env_vars`; reuse `IKmsClient` |
| G | Phase 3 telemetry consent UI shape | Gap 19 | Spec says opt-in but UX not designed |
| H | Whether M18 includes audit-engagement scheduling or only artifact production | Gap 20 | Affects calendar binding |
| I | (was: install ID stability) — see K | Gap 19 | Superseded by K |
| J | Anonymisation function semantics — irreversible? Logged? | Gap 18 | Affects audit-of-audit |
| **K** | **Per-session UUID (spec) vs persistent install ID (typical telemetry)** | Gap 19 | **Spec contradicts typical-telemetry pragmatic need; resolve before drafting Phase 3 scope. Recommendation: revise spec to add persistent UUID** |
| **L** | **Lift audit-log access from hardcoded Owner/Admin to RBAC permissions?** | Gap 17/20 | **Required for SOC 2 separation-of-duties evidence; small code lift** |
| **M** | **Audit-log retention period default** | Gap 17 | **No `AuditLogCleanupHost` today; SOC 2 wants 1–7 years; recommend 365 days default, configurable** |
| **N** | **Signing-key plaintext fallback — keep or force-KMS for SOC 2 builds?** | Gap 20 | **`SigningKey.KmsKeyId` is currently optional; self-hosted may opt out** |
| **O** | **CLI telemetry-emitter HTTP client — share `internal/backend/client.go` or new package?** | Gap 19 | **Telemetry sends anonymously (no Bearer), doesn't fit existing client shape cleanly** |

Two questions that the spec already answers (do NOT re-litigate):
- Phase 1 default state — spec is unambiguous: Phase 1 sends nothing.
- Telemetry retention — spec is unambiguous: 2 years, 90-day archival.

---

## Slice-count estimate

Revised after the first verification pass:

| Slice cluster | Slices | Notes |
|---|---|---|
| Audit log bulk export | 1 | CSV scaffolding already exists at `AuditLogEndpoints.cs:76-82` + web. One slice: remove `MaxLimit` cap for export-format requests, add JSONL alongside CSV, add `Enterprise` tier gate. |
| Audit log retention + RBAC | 1 | `AuditLogCleanupHost` background service + per-org `audit_log_retention_days` column + `audit_log.view` / `audit_log.export` RBAC permissions + lift hardcoded Owner/Admin gate. |
| GDPR data inventory + export | 2 | (1) Data inventory pass across the 13 user-attributable tables + decision matrix; (2) `GET /api/v1/users/me/export` endpoint + signed URL or chunked-stream response + web download trigger in account settings. **Note:** requires a new user-level account-settings page (no such page exists today; existing `/settings/` is org-scoped). |
| GDPR deletion + anonymisation | 2 | (1) `User.PendingDeletionAt` + `User.AnonymisedAt` columns + state machine cloning `OrgStatus.PendingDeletion` pattern + 30-day cooldown + re-auth check + `UserDeletionFinalizerHost`; (2) `IUserAnonymiser` service for audit-log `ActorId`/`ActorEmail` substitution + last-admin protection extension + `DELETE /api/v1/users/me` endpoint + web UX. |
| Telemetry Phase 3 — ingest | 2 | (1) `telemetry_events` table migration + ingest endpoint + 2-year retention + 90-day archive cron; (2) CLI telemetry-emitter (new `internal/telemetry/` package per Q O), opt-in flag + `curlew telemetry {enable, disable, status, export, delete-request}` subcommand + persistent UUID at `~/.config/curlew/install_id` (per Q K recommendation) + spec revision to match. |
| Encryption-at-rest cleanup | 1 | Apply `IKmsClient` envelope encryption to `team_vaults.template_jsonb` and `schedules.env_vars` sensitive values. Single migration, two providers (`TeamVaultKeyProvider`, `ScheduleEnvKeyProvider`) cloned from `GitLabKeyProvider`. |
| Compliance artifact production | 2–3 | (1) `docs/COMPLIANCE.md` + policy templates (info-sec, access review, incident response, data classification); (2) vendor inventory (Stripe, SendGrid, Google KMS, AWS, GitHub Apps, GitLab) + data-flow diagrams; (3) pen-test engagement + remediation log. (3) may collapse to (2) depending on how much pen-test paperwork is included vs deferred to the audit engagement. |
| Tier-gate hookup (cross-cutting) | 0 | Existing `ITierGate` from M16 (`v3-12`) covers new Enterprise gates with no extra slice. |

**Total: 11–12 slices** (down from initial 13–18 estimate; CSV-export scaffolding, `IKmsClient` envelope, and Phase 1 = no-work collapsed three slices).

Depending on:
- whether Phase 3 telemetry ships in M18 (current recommendation) or splits to a post-launch follow-up (−2 slices, but launch posture suffers);
- whether the compliance artifact slices include pen-test orchestration (2 → 3) or just paperwork (stays at 2);
- whether Phase 2 registration-UX polish is folded in (+1–2 slices) or kept separate.

For comparison: M14 was 21 slices, M16 was 21, M17 was 5, M19 was 5. M18 lands smaller because the existing infrastructure (audit-log CSV path, `IKmsClient` envelope, `OrgStatus.PendingDeletion` pattern) does a lot of the heavy lifting.

Recommended track assignment:
- **`backend`** track: ~6 slices (audit-log export+retention, GDPR endpoints, telemetry ingest, encryption-at-rest, RBAC).
- **`go-cli`** track: 2 slices (telemetry emitter + subcommand, GDPR export download verification).
- **`web`** track: 2 slices (user-level account-settings page with delete/export, telemetry-consent UI for Phase 3).
- **`compliance` / `docs`** track: 2–3 slices (policy templates, vendor inventory, pen-test orchestration).

---

## Recommended pre-work before `/backlog M18`

The first verification pass closed several "TBD" items. Remaining pre-work:

1. **Resolve question K (per-session UUID vs persistent install ID) with stakeholders.** This is the single decision that most reshapes M18's Phase 3 telemetry slices. Without it, the spec-vs-pragmatic-utility argument will surface during `/backlog M18` and force a re-plan.
2. **Draft a SPECIFICATION.md vX.Y edition** covering: (a) GDPR data inventory across the 13 user-attributable tables + per-table retention policy + anonymisation function semantics, (b) telemetry persistent install ID (if Q K resolves that way) + ingest endpoint shape + dashboard, (c) audit-log bulk export endpoint (sync streaming JSONL + CSV) + retention policy + RBAC permissions, (d) `docs/COMPLIANCE.md` introduction + policy-document inventory, (e) encryption-at-rest extension to `team_vaults` and `schedules.env_vars`.
3. **Add an `## Phase 18 — Compliance and launch readiness (M18)` section to `.claude/skills/backlog/milestone-mapping.md`.** Capability keys to seed: `audit_log_export`, `audit_log_retention_rbac`, `gdpr_export`, `gdpr_deletion`, `telemetry_phase3_ingest`, `encryption_at_rest_v2`, `compliance_artifacts`. DAG and track assignments per the table above.
4. **Settle the "is the audit engagement in scope" question with stakeholders.** Recommendation: M18 produces evidence; the auditor engagement is a business-process initiative.
5. **File faker locale support as its own follow-up milestone.** It's not M18; the spec is already complete; the code is the lag. Small dedicated milestone (3–4 slices) post-M18.

---

## Alternatives if M18 design pass isn't appetising right now

(Pattern lifted from `docs/M14_INVESTIGATION.md`.)

- **Defer launch.** Keep M18 unbuilt; restrict the project to pilots / closed beta where compliance pressure is contractual rather than regulatory. Workable for 6–12 months; the cost is conversion-funnel blindness (no telemetry) and inability to take EU customers (no GDPR surface).
- **Slice M18 in half.** Ship "M18a — launch-blocking compliance" (GDPR export+delete, audit-log retention+RBAC, encryption-at-rest cleanup) as a focused ~6-slice milestone. Defer "M18b — telemetry + SOC 2 artifacts" to a later phase tied to the first enterprise pipeline opportunity. Risks: launching without telemetry means no conversion data from the launch window itself, which is the most valuable data the project will ever have.
- **Outsource compliance documentation.** Engage a compliance-as-a-service vendor (Vanta, Drata, Secureframe) to produce the SOC 2 / ISO 27001 artifact set. Reduces internal slice count by ~2; introduces an external dependency and a recurring cost.

---

## Status of related milestones (for context)

Per `docs/REVIEW.md:179-217` and the current state of `management/`:

| Milestone | Theme | Status |
|-----------|-------|--------|
| M12 | Foundational dynamic-function helpers | done |
| M13 | Faker depth | done (locale support deferred to a future small milestone) |
| M14 | Revenue plumbing | done |
| M15 | Tier-matrix corrections | done |
| M16 | Workflow completion | done |
| M17 | First-party signers / signing helpers | done |
| M18 | Compliance and launch readiness (this doc) | **blocked on design pass** |
| M19 | Strategic expressiveness (`if:` field, CEL eval, manual-as-skill) | done |

M18 is the only remaining unbuilt pre-launch milestone. Per project memory `project_milestone_release_model.md`, no public launch happens until M18 ships.

---

## Notes on what was NOT considered in scope for this investigation

- **Code changes.** Investigation only; no source files outside `docs/M18_INVESTIGATION.md` are modified.
- **Pricing-tier reshuffling.** Out of scope per `docs/REVIEW.md:226`.
- **Re-litigating M18's existence.** REVIEW.md and the release-model commitment already establish the "must ship before launch" framing.
- **REVIEW.md gaps 1–16 and 21–25.** Those are shipped (M12–M17, M19) or explicitly rejected (manual-as-skill shipped in M19, MCP rejected).
- **The actual SOC 2 / ISO 27001 audit engagement.** Producing evidence is in scope; sitting in the auditor's chair-time is a business-process initiative outside the milestone framework.
- **Marketing / launch-comms / pricing-page work.** Adjacent to "launch readiness" but not engineering surface.
- **Faker locale code-vs-spec drift.** Its own follow-up milestone post-M18.
- **GHES support.** Enterprise feature, not compliance; defer to a future Enterprise-features milestone.

---

If something specific in the M18 spec surface was expected to appear in this investigation and did not, name it. The verification pass on 2026-05-17 covered backend, CLI, web, and the relevant spec sections; cross-cutting questions can still hide between sections.

---

## Decisions Resolved (v4.4, all M18 themes)

All fifteen cross-cutting questions (1–15 and K–O) are closed by these fifteen `v4-N` decisions, resolved 2026-05-17. Decisions are settled commitments, not proposals — see `docs/SPECIFICATION.md` v4.4 Changelog + "Decisions Resolved (v4.4, all M18 themes)" for the same table cross-referenced to spec sections.

| # | Decision area | Resolution | Closes |
|---|---|---|---|
| v4-1 | Audit-log export format + transport | **JSONL + CSV; streaming via `Transfer-Encoding: chunked`; cap removed for `?format=csv\|jsonl` requests; Enterprise tier gate.** No async-job model in v1 — sync streaming covers up to ~1M rows comfortably; async deferred to a future milestone if a customer's log exceeds that comfortably. | 1, 2, E |
| v4-2 | Audit-log retention | **Per-org `audit_log_retention_days` column (default 365); `AuditLogCleanupHost` background service runs daily, hard-deletes rows older than `now() - retention_days`.** Enterprise can extend beyond 365 via column edit; other tiers capped at 365 to bound storage. | 3, M |
| v4-3 | Audit-log RBAC | **Add `audit_log.view` and `audit_log.export` permissions to `Rbac/Permissions.cs`; lift hardcoded `Owner/Admin` gate at `AuditLogQueryService.cs:24` to permission check.** Default roles: Owner+Admin get both permissions; new "Security Auditor" custom role template carries `audit_log.view` only — supports SOC 2 separation of duties. | L, 15 |
| v4-4 | GDPR export scope + endpoint | **`GET /api/v1/users/me/export` returns a signed-URL pointer to a JSON bundle limited strictly to data-subject's own attributable rows across the 13 user-attributable tables enumerated in Gap 18.** Bundle includes profile, memberships, audit-log entries authored, notification preferences, license history, refresh-token metadata (not values), and per-org memberships. **Excludes** data the user can see but doesn't own (other members' emails, org metadata). Rate-limited 1 request per user per 24h. | 8 |
| v4-5 | GDPR deletion state machine | **`User.PendingDeletionAt` + `User.AnonymisedAt` columns; 30-day cooldown matching spec `:6671`; re-authentication required (password re-entry within last-5-minutes window) before initiation; `UserDeletionFinalizerHost` background service runs daily and finalizes deletions past cooldown.** Clones the existing `OrgStatus.PendingDeletion` pattern from `MembersService.cs:240-269`. Email notification on initiation AND on completion. | 5, 7, D |
| v4-6 | Anonymisation function semantics | **`IUserAnonymiser` service replaces `ActorId` with NULL and `ActorEmail` with deterministic `deleted-user-{first-8-chars-of-sha256(user_id+org_id)}` on hard-delete.** Anonymisation is **irreversible** (designed property: the user record is gone, no original to restore). The act of anonymisation itself emits an audit-log event `user.anonymised` with the deleted user's anonymisation token as `TargetId` — preserves the audit-of-audit trail without breaking the right-to-erasure. | 4, B, J |
| v4-7 | Last-admin protection extension | **Block `DELETE /api/v1/users/me` (and any soft-delete initiation) when the user is the sole owner of any org with members.** Reuses the existing `MembersService.cs:183-184` `OwnerCannotLeave` error code (renamed conceptually but kept for wire compatibility — same shape, broader trigger site). Error response lists every blocking org so the user knows what to transfer. | 6 |
| v4-8 | Telemetry identity model | **Revise spec from per-execution session UUID to persistent install ID.** Anonymous UUID generated on first run, stored at `~/.config/curlew/install_id` (mode `0600`), regeneratable via `curlew telemetry reset-id`, deleted when `curlew telemetry delete-request` finalizes. Session UUID is preserved as a per-execution sub-identifier nested inside the install ID's events — both layers ship. This contradicts the prior spec's session-only model; the trade is justified by the conversion-funnel analytics utility, which the session-only model cannot provide. | K, I |
| v4-9 | Telemetry lawful basis | **Explicit opt-in consent** for Phase 3. Spec already commits to opt-in (`:6308+`); v4-9 reaffirms the lawful basis is consent under GDPR Article 6(1)(a), not legitimate interest. Phase 1 sends nothing (per spec; no lawful-basis question arises). Phase 2 is registration UX, not telemetry. | 9, 11, A, G |
| v4-10 | Telemetry phase scope for M18 | **M18 ships Phase 3 ingest infrastructure + CLI emitter + backend storage.** Phase 2 registration polish and the team-side analytics dashboard each split to post-M18 follow-up milestones. Rationale: the data Phase 3 collects becomes immediately useful on launch day; Phase 2 polish and the team dashboard can iterate on real data rather than be designed in a vacuum. | 10 |
| v4-11 | Telemetry CLI emitter shape | **New `internal/telemetry/` package with a dedicated HTTP client.** Does NOT reuse `internal/backend/client.go` — telemetry sends anonymously (no Bearer auth, no refresh-token flow), carries an idempotency key per event for at-most-once delivery, and uses an `curlew.org/telemetry` endpoint distinct from the licensing/billing API host. The shapes don't fit `backend.Client`'s authenticated-RPC pattern cleanly enough to share. | O |
| v4-12 | Encryption-at-rest extension | **Apply existing `IKmsClient` envelope encryption (per-row DEK, KMS-wrapped KEK) to `team_vaults.template_jsonb` (closes v3-10 deferral) AND `schedules.env_vars` sensitive values (closes spec `:11057` deferral) in a single migration.** New `TeamVaultKeyProvider` and `ScheduleEnvKeyProvider` cloned from `GoogleKmsGitLabKeyProvider`. The plaintext-with-coordinate-validation manifest validator from v3-10 stays — encryption is added in addition, not in replacement. For self-hosted deployments without KMS, a `FileTeamVaultKeyProvider` mirrors the GitLab file-provider pattern. | 13, F |
| v4-13 | Signing-key plaintext fallback | **Keep deployment-mode-dependent.** Self-hosted may opt out of KMS by setting `CURLEW_SIGNING_KEY_MODE=file`. SaaS production builds add a CI lint (`scripts/ci-local.sh` extension) that runs `SELECT COUNT(*) FROM signing_keys WHERE kms_key_id IS NULL` on the production DB connection and fails the build if any row matches. Documented in `docs/COMPLIANCE.md` as a build-time control. | N |
| v4-14 | SOC 2 / ISO 27001 scope for M18 | **Minimum viable evidence collection only.** M18 produces: `docs/COMPLIANCE.md` umbrella, info-sec policy template, access-review policy template, incident-response runbook, data-classification matrix, vendor inventory, data-flow diagrams, and orchestrates the first vendor pen-test (engagement + remediation, not the audit itself). The actual SOC 2 audit engagement (6-month observation window with an auditor) is a business-process initiative outside the milestone framework — M18 produces the inputs the audit will consume. | 12, H |
| v4-15 | Pen-test cadence | **Annual.** Budget for the next engagement to be scheduled before the prior SOC 2 Type II observation window closes (typically 9 months in, so the report has fresh test evidence). First engagement is part of M18's compliance-artifact slice (v4-14). | 14 |

Two additional clarifications surfaced during this design pass that don't warrant their own `v4-N` numbering but should be recorded:

- **Faker locale follow-up.** Removed from M18 (spec is complete at `:949–994`; code lag is M13 follow-up). Filed as a new capability stub `faker_locale_completion` (provisional M20) in `management/backlog.yaml`.
- **GHES support.** Removed from M18. Spec at `:9076` says "M18 or later" — this design pass takes "later." It will land in a future Enterprise-features milestone, not in pre-launch.

All v4.4 decisions resolved 2026-05-17. The fifteen cross-cutting questions identified in this investigation (questions 1–15 from the gap audit plus K–O from the verification pass) are closed by entries v4-1 through v4-15 above. **`/backlog M18` is unblocked.**
