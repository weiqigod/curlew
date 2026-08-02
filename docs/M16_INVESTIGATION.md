# M16 (Workflow Completion) — Pre-Backlog Investigation

Investigation date: 2026-05-07
Design pass landed: 2026-05-07 — see `docs/SPECIFICATION.md` v4.3 Changelog and "Decisions Resolved (v4.3, all M16 themes)" subsection. The cross-cutting questions identified below (A–I) are closed by v4.3 entries `v3-1` through `v3-15`; this document remains as the audit-trail and pre-work record. **`/backlog M16` is now unblocked.**

> **Note on line numbers:** This audit was written against `docs/SPECIFICATION.md` v4.2.1. Line-number references in the gap-by-gap audit below (e.g., `:9110-9116`, `:5671-5708`) reflect v4.2.1 positions, NOT post-v4.3 positions. v4.3 inserted ~960 new lines; for current navigation, prefer the section names (e.g., "Password Reset & Email Verification Flow", "Trial Persistence and Activation"). The line-numbered citations are preserved as-is for the audit trail.

Scope: ground REVIEW.md's M16 framing in the current code, surface design questions before `/backlog M16` is run.
Sources audited: `docs/REVIEW.md`, `docs/SPECIFICATION.md` (v4.2.1 at audit time; now v4.3 after the design pass), `docs/MANUAL.md`, `docs/M14_INVESTIGATION.md` (template), `src/ApiTool.Backend/{Auth,Licensing,Notifications,Schedules,Coordinator,GitHub,PrChecks,Sso,Subscriptions,Results,Health,Rbac,Migrations}/`, `internal/{license,auth,backend,variable}/`, `cmd/curlew/license.go`, `web/src/`, `.claude/skills/backlog/milestone-mapping.md`, `management/backlog.yaml`.

---

## Why this file exists

`/backlog M16` was blocked at the time of this investigation. Two independent blockers, the same shape as M14's:

1. **Skill gate.** `.claude/skills/backlog/SKILL.md` requires an entry in `milestone-mapping.md`. M16 is not there — only M1–M5, M12, M13, M14, M15, M17 are. **CLOSED 2026-05-07** by adding `## Phase 16 — Workflow completion (M16)` to `.claude/skills/backlog/milestone-mapping.md`.
2. **Design pass missing.** `docs/REVIEW.md:191` and `:239` say M16 needs design passes for "password-reset flow, schedule-executor architecture, PR-check posting model" before backlog generation. SPECIFICATION.md v4.2.1 covers M14's surface; the M16 areas are mostly hand-waved or absent. **CLOSED 2026-05-07** by SPECIFICATION.md v4.3.

This file is the investigation behind the design pass — what's actually in the code today (six months after M14 shipped), where REVIEW.md is now stale because M14/M15 absorbed scope, what design questions REVIEW.md missed, and a realistic slice-count estimate. With v4.3 shipped, this file's value is the audit trail and the "what was resolved" reference.

---

## Summary

REVIEW.md:191 frames M16 as "T3 items 9–16 and 23: password reset, email verification, trial issuance, schedule executor, PR check posting to GitHub/GitLab, shared vault, health metrics dashboard, SSO gate, `from_command` retiering." That framing is now stale. M14 and M15 absorbed nearly half of it:

| REVIEW.md gap | Original M16 scope | Status now |
|---|---|---|
| 9 (password reset / email verification) | Endpoints + flows + SMTP | **Reduced.** SMTP plumbing + email-template pipeline shipped in M14-014/015. Six M14 templates inventoried at [EmailTemplateInventory.cs:16-24](src/ApiTool.Backend/Notifications/Email/EmailTemplateInventory.cs); `password_reset` is **not** among them — explicitly punted to M16 per `EmailTemplateInventory.cs` and the M14 investigation. Endpoints, state machine, token tables: all still missing. |
| 10 (trial JWT issuance + on-demand re-trials) | Resolver + persistence + endpoints | **Reduced.** Claim slots reserved by M14: [LicenseTokenIssuer.cs:54-55](src/ApiTool.Backend/Licensing/Tokens/LicenseTokenIssuer.cs) hardcodes `trial_state="none"` and `trial_expiry=null` with comment "Trial fields default to none/null for M14 (M16 will inject `ITrialStateResolver`)". Resolver, persistence, endpoints, **and CLI claims-struct extension** all still missing. |
| 11 (schedule executor) | Cron + executor + result link | **Unchanged.** [SchedulerHost.cs](src/ApiTool.Backend/Schedules/SchedulerHost.cs) ticks every 30s and creates `ScheduledRun` rows in `Queued` state via `ISchedulerEnqueuer.EnqueueDueAsync()` but **nothing consumes the queue** — no `curlew run` invocation, no `result_id` link on `ScheduledRun`. |
| 12 (PR-check posting) | GitHub + GitLab | **GitHub closed (M14-016/017/018/019).** GitLab side untouched — `grep -rn gitlab src/ApiTool.Backend/` returns one comment, no code. |
| 14 (shared vault for Team tier) | Backend endpoints + CLI propagation | **Unchanged.** Permissions defined at [Permissions.cs:67-71](src/ApiTool.Backend/Rbac/Permissions.cs) (`vault_config.manage`/`vault_config.view`); CLI feature registered at [registry.go:126](internal/auth/registry.go) as `TierTeam`. **Both inert** — no entity, no endpoints, **and no enforcement** (`CURLEW_TEAM_CONFIG` works at any tier today, identical revenue-leak pattern to M15-001's plugin gap and M15-002's SSO gap). |
| 15 (health metrics dashboard) | Aggregate-metrics page | **Unchanged.** No `/dashboard` route in `web/src/routes/`. [HealthEndpoints.cs](src/ApiTool.Backend/Health/HealthEndpoints.cs) is liveness/readiness probes only; user-facing aggregation absent. |
| 16 (SSO Enterprise gate) | Tier gate enforcement | **CLOSED by M15-002** (PR #187, commit 20a4c5f7). |
| 23 (`from_command` retier) | Solo → Free | **CLOSED by M15-003** (PR #188, commit 8063389b). |

The effective M16 scope is six gaps: 9, 10, 11, 12-GitLab-side, 14, 15. The rough slice count is 16–22 — comparable to M14 but with looser coupling (most slices are independently shippable; the schedule executor is the one that pulls the most threads).

Out of nine cross-cutting design questions identified in this investigation, REVIEW.md names two explicitly, hand-waves three, and misses four.

---

## Gap-by-gap audit

### Gap 9 — Password reset + email verification

**What exists today:**

- User entity at [src/ApiTool.Backend/Data/Entities/User.cs](src/ApiTool.Backend/Data/Entities/User.cs) carries `Email` (indexed), `PasswordHash` (Argon2id, set in [AuthLoginEndpoints.cs](src/ApiTool.Backend/Auth/AuthLoginEndpoints.cs)), `IsAdmin`, `CreatedAt`. **No `EmailVerified` flag.**
- SendGrid pipeline shipping: [SendGridSmtpSender.cs](src/ApiTool.Backend/Notifications/Email/SendGridSmtpSender.cs), [EmailQueueProcessor.cs](src/ApiTool.Backend/Notifications/Email/EmailQueueProcessor.cs), `ChannelEmailQueue.cs` with the MJML compile pipeline + manifest source-of-truth design from M14-014.
- Six-template inventory at [EmailTemplateInventory.cs:16-24](src/ApiTool.Backend/Notifications/Email/EmailTemplateInventory.cs): `email_verification`, `auth_device_code`, `billing_receipt`, `billing_payment_failed`, `billing_subscription_canceled`, `account_security_alert`. **Note:** `email_verification` is in the inventory; the *flow* that triggers it is not.
- Refresh-token table from M14 (migration `20260504150157_SigningKeysAndRefreshTokens.cs`) provides the reusable pattern for time-bound, hash-stored, single-family tokens with rotation: [src/ApiTool.Backend/Auth/Refresh/RefreshTokenService.cs](src/ApiTool.Backend/Auth/Refresh/RefreshTokenService.cs).

**What's missing on the backend:**

- No `password_reset_tokens` table; no migration. `grep -rn password_reset_token src/ApiTool.Backend/` returns zero hits.
- No `email_verification_tokens` table; no migration. Could fold into `users` (single-token-per-user) but M14's per-flow-table convention argues against.
- No `is_email_verified` column on `users`. State has nowhere to live.
- No endpoints: `POST /api/v1/auth/password-reset/request`, `POST /api/v1/auth/password-reset/confirm`, `POST /api/v1/auth/email/verify`, `POST /api/v1/auth/email/resend`.
- No service layer: `IPasswordResetService`, `IEmailVerificationService` absent.
- The `password_reset` template is **not** in `EmailTemplateInventory` — has to be added to the inventory + MJML source + manifest.

**What spec says — and where it's loose:**

`docs/SPECIFICATION.md:9110-9116` covers password reset in seven lines:
- "Magic link sent to email (no password)" — wait, this conflicts with the existing Argon2id password store. The spec hedges between magic-link and password-reset models.
- "Token valid for 15 minutes."
- "One-time use (marked as consumed in database)."

Email verification mentioned only twice — `docs/SPECIFICATION.md:6679` (an `email_verified: boolean` field on the User schema) and `:6695` (a `pending` user status: "Awaiting email verification"). No flow.

**Genuinely open design questions:**

1. **Magic-link vs traditional password-reset.** Spec line 9110 says "magic link"; current code has Argon2id passwords. Pick one. Magic-link is simpler (no new password input UX, no password-strength validation, smaller attack surface) but breaks the existing CLI device-code flow that assumes a password. Traditional reset is more familiar to users but requires the new-password form + zxcvbn-style strength check + same-as-old check.
2. **Email-verification gating policy.** Is an unverified user blocked from anything (login? checkout? license fetch?) or is verification advisory until launch? Most SaaS makes verification mandatory before paid checkout but allow free use. The spec doesn't say.
3. **Rate-limit and enumeration defense.** Reset-request endpoint must throttle per-email AND per-IP, return identical responses for "email exists" vs "email doesn't exist", and avoid timing leaks. None of this is in the spec. Recommendation: piggyback on the existing `Internal/RateLimit` infrastructure (13 fixed-window policies already shipped per REVIEW.md:53).

### Gap 10 — Trial JWT issuance + on-demand re-trials

**What exists today:**

- M14 reserved the JWT slots: [LicenseTokenIssuer.cs:54-55](src/ApiTool.Backend/Licensing/Tokens/LicenseTokenIssuer.cs) hardcodes `trial_state="none"` and `trial_expiry=null`; the comment at `:13` says "Trial fields default to none/null for M14 (M16 will inject `ITrialStateResolver`)."
- [JwsBuilder.cs:18-27](src/ApiTool.Backend/Licensing/Tokens/JwsBuilder.cs) confirms null trial claims are serialized explicitly so the CLI verifier can rely on them being present.
- Spec covers the trial *behavior* in detail at `docs/SPECIFICATION.md:5671-5708` (38 lines): 14-day full trial at registration; 7-day per-feature on-demand trials post-expiry; "one per feature, no infinite re-trials."
- JWT claim shape spec at `docs/SPECIFICATION.md:7872-7876` reserves `trial_state` enum (`none | active | expired | extended`) and `trial_expiry` (Unix timestamp, null when state is `none`). M16 populates without claim-shape churn — this is the decoupling M14 paid for.

**What's missing on the backend:**

- No `ITrialStateResolver` interface. The injection point exists in `LicenseTokenIssuer` (where the hardcoded defaults live) but the resolver is undefined.
- No `trials` table; no migration. The "one per feature per user" uniqueness constraint and the cooldown clock both need a row to live in.
- No endpoint for on-demand activation: `POST /api/v1/trials/{feature}` is sketched at `docs/SPECIFICATION.md:5689-5690` only as a UX URL pattern.
- No expiry-cron — the `trial_expiring` template was deferred from M14 to "M16's password-reset and trial cron slices" (per `management/backlog.yaml:14-016 description`).
- Subscription state at [src/ApiTool.Backend/Subscriptions/](src/ApiTool.Backend/Subscriptions/) does not currently model trial periods; trial is a separate dimension from subscription tier.

**What's missing on the CLI:**

- **Critical delta — claims-struct mismatch.** [internal/license/jwt.go:49-61](internal/license/jwt.go) `Claims` struct does **not** carry `trial_state` or `trial_expiry`. The backend issuer puts them in the JWT (as `none`/`null`); the CLI verifier ignores them. This is fine while every JWT is `none`, but the moment M16's resolver issues a real trial state, the CLI cannot read it. Adding both fields to the Go `Claims` struct (with `omitempty`) is a one-line change but must land before any feature gate consults trial state.
- No trial consultation in `internal/auth/registry.go`. The feature-definition map gates by `RequiredTier` only; no `TrialEligibleFor(feature)` helper exists. M16 must add a `Claims.IsTrialActiveFor(feature string) bool` accessor and integrate at every premium-feature gate.
- No CLI subcommand for activation. Existing surface in [cmd/curlew/license.go](cmd/curlew/license.go) is `--validate`, `--refresh`, `--debug`, `export`. A natural extension is `curlew license trial start <feature>` — extend the switch at line 41.
- No backend client method. [internal/backend/client.go](internal/backend/client.go) has `RefreshTokens()` (M14-005) and the generic `GetJSON`/`PostJSON`. Add `StartTrial(ctx, feature)`.

**Genuinely open design questions:**

4. **Trial uniqueness scope.** Per-user, per-org, or per-(user, feature)? Spec line 5688 says "one per feature" but is silent on whether that's per-user (a user can re-trial the same feature in a new org) or globally per-user (one shot, ever). Globally-per-user is the simpler implementation; per-(user, org) is more generous but creates a shared-account abuse vector. Recommendation: per-(user, feature) globally — the abuse surface is small for an API-testing tool.
5. **Cooldown after expiry.** "Post-expiry on-demand re-trials" is ambiguous: does it mean (a) you trial Feature X for 7 days, expiry hits, you can immediately ask for another 7 days? (Then "one per feature" is meaningless.) (b) Or does it mean you trial X once, then after some period (90 days? a year?) you can trial it again? (c) Or is the answer that the 14-day initial trial is for *all* features and the per-feature 7-day trials are *one-shots* per feature? Read of the spec: probably (c). Worth pinning down in v4.3.
6. **Trial state transition on tier upgrade.** If a user is mid-trial for Feature X (Team-tier feature) and upgrades to Team subscription, does the trial silently complete? Spec doesn't say. Cleanest: transition to `none`, close the trial row, the user just has the feature now via tier.

### Gap 11 — Schedule executor

**What exists today:**

- [Schedule entity](src/ApiTool.Backend/Data/Entities/Schedule.cs) carries `Id`, `OrgId`, `Name`, `CronExpression`, `CollectionRef`, `Enabled`, `NextRunAt`, `LastRunAt`, `CreatedBy`, `CreatedAt`, `UpdatedAt`.
- [ScheduledRun entity](src/ApiTool.Backend/Data/Entities/ScheduledRun.cs) carries `Id`, `ScheduleId`, `Status` (enum: `Queued | Running | Completed | Failed`), `CreatedAt`, `StartedAt`, `CompletedAt`. **Missing: `result_id` foreign key** to `Results`.
- [SchedulesService.cs](src/ApiTool.Backend/Schedules/SchedulesService.cs) handles cron storage (uses `Cronos`), CRUD, run-now, list-runs, enqueue-due logic.
- [SchedulerHost.cs](src/ApiTool.Backend/Schedules/SchedulerHost.cs) is a `BackgroundService`, polls every 30s, calls `ISchedulerEnqueuer.EnqueueDueAsync()`.
- [Coordinator/](src/ApiTool.Backend/Coordinator/) — distributed-perf job/shard model with heartbeat, shard reaper, RBAC. Architecturally adjacent.

**What's missing:**

- **The actual executor.** The enqueuer creates `ScheduledRun` rows in `Queued` state and **nothing consumes them.** No service reads `CollectionRef`, fetches the YAML, invokes a runner, captures results. The "executor" is a hole shaped like a service.
- No `result_id` linkage. `ScheduledRun` and `Results` are unjoined.
- The collection source-of-truth: `CollectionRef` is a string. There's no backend-side store of project YAML; the CLI reads from disk. So the executor either (a) requires the customer to push their YAML to a `team_projects` store the backend manages, (b) requires the customer to run a self-hosted runner that has the YAML on its filesystem, or (c) requires a remote-fetch URL pattern. This isn't decided.

**What spec says:**

`docs/SPECIFICATION.md:5799-5800` mentions scheduled test runs ("smoke tests hourly, regression nightly") but doesn't pin the execution model. There is **no specification of push-vs-pull, runner registration, or result-ingestion API**.

**Genuinely open design questions REVIEW.md flagged:**

7. **Schedule executor architecture (REVIEW.md:191 flagged "needs design pass").** Three options:
   - **(a) Backend-resident executor.** A new `IHostedService` consumes the queue, fetches YAML from a `team_projects` table the user pushes via CLI, runs an embedded test runner, writes to `Results`. **Pros:** zero customer infra; mirrors how M14-021 ingested results. **Cons:** the backend now executes arbitrary user-supplied collections (security blast radius — the CLI hits whatever URLs the user wrote, and now those URLs are hit from our IP); the backend also needs the test runner embedded as a library (the CLI is Go, the backend is .NET — embedding means cross-compiling or running the CLI as a subprocess from the backend container).
   - **(b) Self-hosted runner.** A `curlew worker --schedule-pull` daemon the customer runs in their VPC; it polls `GET /api/v1/schedules/next-run` (similar to perf workers per REVIEW.md:53), runs the YAML locally, posts results back. **Pros:** no security blast radius, reuses existing distributed-worker code path. **Cons:** customer infra burden — Team-tier customers may not have somewhere to run a daemon.
   - **(c) Hybrid.** SaaS gets backend-resident execution against a small set of URL patterns (e.g., `*.customer.com` whitelist registered per-org); self-hosted gets the worker model. Lots of complexity for two paths to maintain.

   Recommendation: **(b)** — reuses the Coordinator's job/shard infra, ships earlier, defers the security questions until SaaS scale demands it. Document `(a)` as a future option.

8. **Run-time secrets propagation to the executor.** A scheduled run needs the org's variables/secrets. Vault provider profiles are CLI-side; for option (b), the worker fetches them via shared-vault endpoint (gap 14 dependency). For option (a), the backend has them already but must be careful not to leak into logs.
9. **Cron timezone.** Standard 5-field POSIX cron implies UTC. If a user schedules at `9 * * * *` from Stockholm, do they mean 09:00 CET or 09:00 UTC? Spec doesn't say. Add a `timezone` column on `Schedule`.

### Gap 12 GitLab side — PR-check posting parity

**What exists today (GitHub side, M14):**

- [src/ApiTool.Backend/GitHub/](src/ApiTool.Backend/GitHub/) — `IGitHubAppKeyProvider` with two implementations (`FileGitHubAppKeyProvider` for self-hosted, `GoogleKmsGitHubAppKeyProvider` for SaaS — RS256 HSM-tier).
- `AppJwtBuilder` mints 10-minute installation-token JWTs (RS256, mandated by GitHub).
- `github_installations` table (migration `20260506093300_AddGithubInstallations.cs`); `InstallationTokenCache` + `GitHubInstallationsApi`.
- [GithubWebhookEndpoint.cs](src/ApiTool.Backend/GitHub/Webhooks/GithubWebhookEndpoint.cs) verifies `X-Hub-Signature-256` HMAC-SHA256, dispatches to handlers.
- Outbound check-run posting via `PrChecksUploadEndpoint`; PR-check schema expanded in migration `20260507130000_PrChecksV421.cs` with `external_id`, `installation_id`, `check_run_id`, `conclusion`, `details_url`, `output_summary`, `annotations`, `head_sha`, `posting_started_at`, `posted_at`.

**What's missing (GitLab side):**

`grep -rn gitlab src/ApiTool.Backend/` → one comment hit only. **Nothing exists.** Need the full mirror:

- `IGitLabKeyProvider` (or token provider — GitLab's auth model is different; see below).
- `gitlab_installations` (or `gitlab_projects`) table.
- Webhook ingestion endpoint with HMAC-SHA256 verification (GitLab also uses HMAC, but with per-webhook secrets, not per-app).
- Outbound poster against GitLab Commit Status API or Pipeline Status API.
- The `PrCheck` schema needs a `provider` discriminator + nullable `installation_id`/`check_run_id` (for GitLab, swap with `project_id`/`pipeline_id`).

**What spec says:**

`docs/SPECIFICATION.md:8293` is a single sentence:
> "**GitLab parity is explicitly NOT in M14.** A future milestone may add a parallel `IGitLabCheckPoster` against the GitLab Commit Status API, behind a deployment-level `git_host` provider abstraction, but that is a separate design pass."

That single line is the entire GitLab specification. Everything else needs to be added.

**Genuinely open design questions REVIEW.md missed:**

10. **GitLab auth model.** Three possibilities, in roughly increasing complexity:
    - **(a) Project Access Token per-repo.** Customer creates a Project Access Token in GitLab, pastes it into our dashboard. We POST status with that token. Simple. Doesn't scale to GitHub-style App distribution.
    - **(b) GitLab OAuth App.** Customer authorizes our OAuth App org-wide. We hold a refresh token per org, mint short-lived access tokens.
    - **(c) GitLab "App" (incubating GitLab feature).** Mirrors GitHub Apps but is less mature in the GitLab ecosystem.

    Recommendation: **(a)** for M16, scales-to-(b) later. PATs are universally supported including GitLab self-managed; OAuth Apps need GitLab.com vs self-managed handling.

11. **GitLab self-managed support scope.** `gitlab.com` only, or GitLab self-managed too? GitLab self-managed customers tend to be the same crowd that self-hosts our backend. Recommendation: support `gitlab.com` + self-managed via a per-org `gitlab_base_url` field. Defer GitLab Dedicated / GitLab Cloud Native shenanigans.

### Gap 14 — Shared vault for Team tier

**What exists today:**

- [Permissions.cs:67-71](src/ApiTool.Backend/Rbac/Permissions.cs) defines `vault_config.manage` and `vault_config.view` permissions.
- [internal/auth/registry.go:126](internal/auth/registry.go) registers feature `shared_vault_templates` at `TierTeam` with description "Shared vault configuration templates require Team tier".
- CLI side: `internal/vault/teamtemplate/` parses local YAML team templates per `MANUAL.md:2904-2912`.
- Spec at `docs/SPECIFICATION.md:5606-5626` shows YAML structure for shared vault (per-environment vault provider configs).

**What's missing — and the revenue-leak parallel to M15:**

- **No backend storage.** No `team_vaults` (or `vault_configs`) table; no entity; no migration; no service.
- **No backend endpoints.** `GET /api/v1/organizations/{orgId}/vault-config`, `POST`, `PATCH`, `DELETE` all absent.
- **No CLI fetch.** `internal/backend/client.go` has no `GetTeamVault()` method.
- **No `internal/variable/` integration.** The vault template currently arrives via local YAML file (`CURLEW_TEAM_CONFIG`). Spec describes "pull `shared_vault` config from the backend at runtime" but no code does this.
- **Critical: tier gate is registered but not enforced at runtime.** The feature flag at `registry.go:126` is `TierTeam` but `CURLEW_TEAM_CONFIG` works at any tier today. **This is the same pattern as the closed M15-001 (plugin loading), M15-002 (SSO), and M15-003 (`from_command`) gaps — registered-but-not-enforced.** M16 must close this enforcement gap as part of the shared-vault slice; it's a small revenue leak today.

**What spec says — and where it's loose:**

`docs/SPECIFICATION.md:5606-5626` shows a YAML structure example. `docs/MANUAL.md:2904-2912` documents the local-file path. **What's not in either:** storage table schema, encryption-at-rest model, CLI fetch endpoint, per-environment override semantics.

**Genuinely open design questions REVIEW.md missed:**

12. **Encryption at rest.** Three options:
    - **(a) Plaintext.** Stored as JSONB / TEXT. Acceptable when the column itself is access-controlled and the *contents* are vault-provider *coordinates* (pointers like `aws-secrets-manager arn:...`), not actual secrets. Spec line 5606 examples are all coordinates, not secrets — vault providers do the secret fetching.
    - **(b) Server-side AES-256 with KMS-wrapped DEK.** Symmetric to GitHub App private key handling.
    - **(c) Client-side encryption.** Customer holds the key; we store ciphertext.

    Recommendation: **(a)** for M16, since the v4.2.1 model is "store the *config* of where secrets live, never the secrets themselves." If a customer puts an actual secret in the template, that's their bug (and the template manifest should validate against it).

13. **Propagation cadence.** CLI fetches at the start of every `curlew run` (slow but always-fresh) or caches with a TTL (faster but stale)? Recommendation: TTL of 5 minutes by default with `--refresh-vault` to force; cache file at `~/.config/curlew/team_vault.json`. Same shape as the JWKS cache.

### Gap 15 — Health metrics dashboard

**What exists today:**

- [Results entity](src/ApiTool.Backend/Results/) carries aggregate metrics: `PassCount`, `FailCount`, `SkippedCount`, `DurationMs`, `RunAt`, `CreatedAt` — well-shaped for trend computation.
- [HealthEndpoints.cs](src/ApiTool.Backend/Health/HealthEndpoints.cs) is liveness/readiness probe only (Kubernetes-facing). Not user-facing.
- [Permissions.cs:73-77](src/ApiTool.Backend/Rbac/Permissions.cs) defines `dashboard.view` and `dashboard.export`.
- Web routes today (under `web/src/routes/(app)/org/[slug]/`): `results/`, `audit-log/`, `members/`, `billing/`, `settings/`, `pr-checks/`, `runs/`. **No `dashboard/`.**

**What's missing:**

- No aggregation endpoint (`GET /api/v1/organizations/{orgId}/results/stats`, `/results/failures`, etc.).
- No web page. A `(app)/org/[slug]/dashboard/` route with `+page.server.ts` and `+page.svelte` is the natural location.
- No time-series rollup. `Result` rows exist but no rolled-up hourly/daily aggregates. For M16 scope this is fine — query on demand from `Results` indexed on `(org_id, created_at)`.

**What spec says:**

`docs/SPECIFICATION.md:9453` describes the dashboard in narrative form: pass/fail counts, pass rate, average duration, time-period selector (default last 30 days), recent runs table, frequently-failing-endpoints list. Endpoints sketched: `/results/stats`, `/results/failures`. **No response schemas.**

**Genuinely open design questions:**

14. **Time-series granularity.** Hourly, daily, or per-run points on the trend chart? Default time window? Recommendation: daily aggregation, 30-day default window, switchable to 7d / 90d.
15. **"Frequently failing" definition.** Per-endpoint by HTTP method+URL? Per-test-file? Threshold "frequently"? Recommendation: per-(method, path-template) failures over the selected window, sorted by failure count, top 10.

---

## Cross-cutting open questions

Nine questions surfaced. REVIEW.md named two explicitly (schedule-executor architecture, password-reset flow), hand-waved three (PR-check posting model, trial-cooldown design, dashboard scope), missed four. **All nine are now closed by SPECIFICATION.md v4.3** — the "Resolved by" column points at the relevant decision row in the `Decisions Resolved (v4.3, all M16 themes)` table.

| # | Question | REVIEW.md status | Resolved by | Resolution summary |
|---|----------|------------------|-------------|---------------------|
| A | Magic-link vs password-reset model | Hand-waved as "password-reset flow" | **v3-1, v3-2** | **Argon2id + token-based reset.** Magic-link retired as a design direction. `password_reset_tokens` table (30-min lifetime, single-use, hash-stored) per "Password Reset & Email Verification Flow." |
| B | Email-verification gating policy | **Missed** | **v3-3** | **Verification gates Stripe checkout and org-invite acceptance only.** Free use is unblocked. `[RequireVerifiedEmail]` filter for one-line endpoint gating. |
| C | Trial uniqueness scope (per-user-feature vs per-org) | **Missed** | **v3-4** | **Per-(user, feature), globally** — `UNIQUE (user_id, feature)` on `trials`. The 14-day full trial is per-feature rows; on-demand trials are unavailable for any feature consumed during the 14-day window. |
| D | Schedule executor topology (backend-resident vs self-hosted runner) | Listed | **v3-7** | **Self-hosted runner.** `curlew worker --schedule-pull` polls; backend never sees collection YAML or makes outbound calls toward customer APIs. |
| E | Schedule-run secrets propagation | **Missed** | **v3-9** | **Worker reuses Layer 4 shared vault config** with the same 5-min TTL cache. Backend never sees secret values. |
| F | GitLab auth model (PAT vs OAuth App) | **Missed** | **v3-13, GL-1, GL-2** | **Project Access Tokens with `api` scope.** Stored as AES-256-GCM ciphertext under per-row DEK; OAuth App and GitLab App models deferred. |
| G | Shared-vault encryption-at-rest | Hand-waved | **v3-10** | **Plaintext-with-coordinate-validation.** `team_vaults.template_jsonb` stores vault provider coordinates only; manifest validator rejects literal-secret-shaped values. Server-side encryption deferred to M18 if regulation demands. |
| H | Tier-gate enforcement standardization | **Missed** | **v3-12** | **`ITierGate.EnsureAsync(orgId, requiredTier, ct)`** in `Internal/TierGates/`. `SsoTierGate` and the new gates become call-site adapters. Centralized RFC 7807 mapping. |
| I | CLI claims-struct extension for `trial_state` / `trial_expiry` | **Missed** | **v3-5** | **`Claims.TrialState` and `Claims.TrialExpiry` fields added to `internal/license/jwt.go`** with `omitempty`. New `Claims.IsTrialActiveFor(feature)` accessor. Folded into M16 trial-resolver slice (M16-007). |

Two additional decisions surfaced during the v4.3 design pass that this audit's table did not anticipate:

| # | Decision area | Why this audit missed it |
|---|----------|------------|
| v3-6 | Trial-on-tier-upgrade transition | The original audit treated trial state as feature-list-only; the JWT-shape question of "what does `trial_state` say while a user is mid-trial AND mid-checkout?" surfaced when drafting "Trial Persistence and Activation." Resolved: trials transition to `preempted_by_subscription` on Stripe webhook. |
| v3-15 | Dashboard response schemas (`/results/stats`, `/results/failures`) | The audit treated the dashboard as "one slice for the page"; pinning the response schemas (windows, percentile latency presence/absence, failure grouping key) was its own design exercise. Resolved: daily aggregation, `7d/30d/90d` window selector, on-demand SQL (no rollup table); p99 deferred. |

All v4.3 decisions resolved 2026-05-07.

---

## Slice-count estimate

REVIEW.md:191 doesn't give a number for M16. Working from the gap-by-gap residual scope above:

| Slice cluster | Slices | Notes |
|---|---|---|
| Password reset + email verification | 3 | (1) `password_reset_tokens` + `email_verification_tokens` migrations + `is_email_verified` on `users` + `password_reset` template add to inventory; (2) request/confirm endpoints + service layer for both flows + rate limits via existing `Internal/RateLimit`; (3) web pages at `/auth/password-reset/{request,confirm}` + email-verification redirect handler. |
| Trial issuance + on-demand re-trials | 4 | (1) `trials` table migration + entity + cooldown clock; (2) `ITrialStateResolver` interface + `DatabaseTrialStateResolver` + integration into `LicenseTokenIssuer`; (3) `POST /api/v1/trials/{feature}` endpoint + eligibility check; (4) **CLI side**: extend `Claims` struct in `internal/license/jwt.go` with `trial_state`/`trial_expiry`, add `IsTrialActiveFor(feature)` accessor, integrate at registry feature gates, add `curlew license trial start <feature>` subcommand + `backend.Client.StartTrial()`. |
| Schedule executor (option (b) — self-hosted runner) | 4 | (1) `curlew worker --schedule-pull` daemon mode (reuses Coordinator job/shard plumbing per REVIEW.md:53); (2) `GET /api/v1/schedules/next-run` + `POST /api/v1/schedules/{run_id}/results` endpoints; (3) `result_id` link migration on `scheduled_runs` + linkage in result-ingest path; (4) `timezone` column on `schedules` + UI surface. |
| GitLab PR-check parity (option (a) — Project Access Token) | 4 | (1) `provider` discriminator + `gitlab_installations` (or `gitlab_projects`) table migration + `IGitLabTokenProvider`; (2) outbound poster against GitLab Commit Status API + retry/error taxonomy; (3) inbound webhook endpoint with HMAC-SHA256 verification + idempotency + 5-failure quarantine pattern (mirror GitHub); (4) dashboard "Connect GitLab" entry point + per-project PAT input UX. |
| Shared vault for Team tier | 2 | (1) `vault_configs` table + entity + `VaultConfigService` + `VaultConfigTierGate.EnsureTeamOrAboveAsync` + `GET/POST/PATCH/DELETE /api/v1/organizations/{orgId}/vault-config` endpoints + **enforcement at the existing `CURLEW_TEAM_CONFIG` load site** (close the M15-style revenue leak); (2) CLI `backend.Client.GetTeamVault()` + cache file at `~/.config/curlew/team_vault.json` with 5-min TTL + `--refresh-vault` flag + integration in variable resolver. |
| Health metrics dashboard | 2 | (1) `GET /api/v1/organizations/{orgId}/results/stats` + `/results/failures` endpoints with daily aggregation + 30-day default window + `DashboardTierGate.EnsureTeamOrAboveAsync`; (2) `(app)/org/[slug]/dashboard/` Svelte page with overview cards, trend chart, recent-runs table, frequently-failing endpoints list. |
| Tier-gate refactor (cross-cutting) | 1 | Lift `SsoTierGate` to a generic `TierGate.EnsureAsync(orgId, requiredTier, ct)` so the three new gates above don't copy-paste the implementation. Refactor existing call sites. (Could also fold into the first slice that adds a new gate, depending on Plan-agent preference.) |

**Total: 16–22 slices** depending on:
- whether the tier-gate refactor is its own slice or folded into shared-vault;
- whether GitLab parity is shipped at the minimum (4 slices) or gold-plated with self-managed-instance support and OAuth App (+2);
- whether magic-link is chosen over password-reset (-1 slice — no new-password form).

For comparison: M14 was 21 slices, M2 was 34, M16's looser coupling means most slices are independently deliverable. Recommended track assignment:
- **`backend`** track: ~12 slices (most of the migration + endpoint work).
- **`go-cli`** track: 3 slices (claims extension, vault fetch, schedule-pull worker mode).
- **`web`** track: 3 slices (password-reset pages, dashboard, GitLab-connect page).
- **Cross-cutting** (backend + cli): 1–2 slices (vault enforcement, schedule result-ingest).

---

## Recommended pre-work — STATUS

The original three pre-work items are all complete (2026-05-07):

1. **✅ SPECIFICATION.md v4.3 edition.** Shipped with the seven sections this audit recommended: auth-model resolution, trial data model, schedule execution model, GitLab integration, shared vault propagation, tier-gate generic abstraction, dashboard response schemas. Plus eight new tables in the Database Schema appendix (`team_vaults`, `password_reset_tokens`, `email_verification_tokens`, `trials`, `gitlab_installations`, `gitlab_webhook_events`; expanded `schedules` and `scheduled_runs`). See `docs/SPECIFICATION.md` Changelog v4.3.

2. **✅ M16 section in `.claude/skills/backlog/milestone-mapping.md`.** Eight capability keys (`tier_gate_generic`, `password_reset_verification`, `trial_resolver`, `schedule_executor`, `gitlab_pr_checks`, `shared_vault_propagation`, `health_dashboard`, `m16_e2e`) totaling 21 slices. DAG documented; track assignments split across `backend` (12 slices), `go-cli` (2), `web` (3), split (3), `e2e` (1).

3. **✅ CLI claims-struct extension** folded into M16-007 (trial-resolver cluster) per the recommendation in this audit and `docs/SPECIFICATION.md` decision v3-5.

**`/backlog M16` is unblocked.** The Plan agent should treat the v4.3 decisions (`v3-1` through `v3-15` and `GL-1` through `GL-7`) as commitments and the milestone-mapping M16 section as the authoritative slice taxonomy.

---

## Status of related milestones (for context)

Per `docs/REVIEW.md:179-217` and the current state of `management/`:

| Milestone | Theme | Status |
|-----------|-------|--------|
| M12 | Foundational dynamic-function helpers | done |
| M13 | Faker depth | done |
| M14 | Revenue plumbing | **done** (21 slices, all merged 2026-04 → 2026-05) |
| M15 | Tier-matrix corrections | **done** (3 slices, all merged 2026-05) |
| M16 | Workflow completion (this doc) | blocked on design pass |
| M17 | First-party signers / signing helpers | done (jumped ahead because it had no open design questions) |
| M18 | Compliance and launch readiness | not generated; deliberately deferred |
| M19 | Strategic expressiveness (`if:` field, CEL/Starlark eval) | not generated; deliberately deferred |

M16 is now the only remaining unbuilt T3 milestone. M18 and M19 are deferred per the release-model commitment that no public launch happens until all of M16-M19 ship as a unit.

---

## Notes on what was NOT considered in scope for this investigation

- **Code changes.** Investigation only; no source files outside `docs/M16_INVESTIGATION.md` are modified.
- **Pricing-tier reshuffling.** Out of scope per `docs/REVIEW.md:226`.
- **Re-litigating M16's existence.** REVIEW.md and the release-model commitment already establish the "must ship" framing.
- **REVIEW.md gaps 17–22 (T4 / T5).** Those are M18 and M19's surface, not M16's.
- **The two-letter v4.3 / v4.3.1 numbering convention.** M14 used v4.2 + v4.2.1 supplement; M16 may want the same split (v4.3 main + a v4.3.1 GitLab supplement) but that's a SPECIFICATION-shape question to settle while drafting.

---

If something specific in the M16 spec surface was expected to appear in this audit and did not, name it. The investigation read backend, CLI, web, and the relevant spec sections in full, but cross-cutting questions can hide between sections.
