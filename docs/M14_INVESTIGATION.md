# M14 (Revenue Plumbing) — Pre-Backlog Investigation

> **Historical analysis.** This records an earlier design or investigation and may
> describe removed features, licensing, or already-completed work. It is not a
> current operating guide. Use [MANUAL.md](MANUAL.md), [CLI_SPECIFICATION.md](CLI_SPECIFICATION.md),
> and the [documentation map](README.md) for current behavior.


Investigation date: 2026-05-03
Scope: ground REVIEW.md's M14 framing in the current code, surface design questions before `/backlog M14` is run.
Sources audited: `docs/REVIEW.md`, `docs/SPECIFICATION.md`, `internal/license/`, `cmd/curlew/license.go`, `src/ApiTool.Backend/Subscriptions/`, `src/ApiTool.Backend/Notifications/`, `.claude/skills/backlog/milestone-mapping.md`, `management/backlog.yaml`.

---

## Why this file exists

`/backlog M14` does not work today. Two independent blockers:

1. **Skill gate.** `.claude/skills/backlog/SKILL.md:37–49` requires an entry in `milestone-mapping.md`. M14 is not there — only M1–M5, M12, M13, M17 are.
2. **Design pass missing.** REVIEW.md:187 and 207 explicitly say M14 needs a SPECIFICATION.md v4.2 edition pinning down JWT claims structure, Stripe SDK strategy, SMTP provider choice, and the CLI ↔ backend API contract. None of that exists in the spec yet.

This file is the investigation behind the design pass — what's actually in the code today, where REVIEW.md is loose, what design questions REVIEW.md missed, and a realistic slice-count estimate.

---

## Summary

M14 is the largest pending milestone, the only T1 (revenue-critical) milestone, and the gate for ~half of M15 and most of M16. REVIEW.md frames it as "five sub-slices behind a design pass." The actual code state suggests **12–15 vertical slices**, and at least three of REVIEW.md's "open questions" are either already resolved in the spec (and REVIEW.md is loose) or missing entirely from REVIEW.md's list.

Out of ten cross-cutting design questions identified in this investigation, REVIEW.md names three explicitly, hand-waves a fourth, and misses six.

---

## Gap-by-gap audit

The five gaps listed in REVIEW.md as comprising M14:

| Gap | Title | Tier |
|---|---|---|
| 5 | License JWT issuance backend | T1 |
| 6 | Stripe live gateway | T1 |
| 7 | Stripe webhook handler | T1 |
| 8 | SMTP / email provider | T1 |
| 13 | CLI ↔ backend integration | T1 |

### Gap 5 — License JWT issuance backend

**What exists today:**
- CLI verifier complete: `internal/license/jwt.go` defines `Claims` (Iss, Aud, Sub, Email, Tier, Features, Exp, Nbf, Iat), `ParseToken`, `VerifyRS256`, `CheckTime`. Embedded JWKS path documented at `docs/SPECIFICATION.md:7796`.
- Spec describes the *consumption* side end-to-end — daily validation, grace period, key rotation — at `docs/SPECIFICATION.md:7716–7878`.
- Offline grace-period state machine shipped as M5-013/014.

**What's missing on the backend:**
- `grep -rn "jwks\|MintToken\|/license/issue" src/ApiTool.Backend` returns **zero hits**. No issuer at all.
- No signing-key storage. The spec talks about embedded *public* keys at line 7826 but never says where the *private* key lives on the backend (KMS? file in `Keys/`? Azure Key Vault? PKCS#8 in env var?).
- No `/api/v1/public-key` endpoint, despite `docs/SPECIFICATION.md:7798` saying the CLI fetches from it.
- No `Claims` shape on the backend side. The CLI's `Claims` struct has nine fields. The spec's `LicenseCache` (at line 7745–7760) has fifteen fields including `requestLimit`, `orgRole`, `deviceId`, `sessionId` — none of which appear in the CLI's `Claims`. **Three different views of the claim shape live in three places** and no single source of truth exists.

**Genuinely open design questions:**
1. **Authoritative claim shape.** Reconcile CLI `Claims` ↔ spec `LicenseCache` ↔ backend issuer. Add `trial_state`, `trial_expiry`, `org_id`, `org_role`, `device_id`, `session_id`, `request_limit` (or decide they belong in a separate `/me` response, not the JWT).
2. **Signing key bootstrap and rotation.** Spec line 7849–7862 describes the *rotation timeline* but not the *storage mechanism*. On-prem self-hosted (`deploy/self-hosted/`) and SaaS multi-tenant probably want different answers.
3. **Refresh-token shape.** `docs/SPECIFICATION.md:7746` says `refreshToken: "Opaque refresh token"`. Opaque to whom? Stored where? Rotation on use, or long-lived? No migration today defines a `refresh_tokens` table — though the ER diagram at line 8804 references one.

### Gap 6 — Stripe live gateway

**What exists today:**
- `src/ApiTool.Backend/Subscriptions/IStripeGateway.cs` — clean three-method interface (Checkout, Portal, ComputeProration).
- `src/ApiTool.Backend/Subscriptions/FakeStripeGateway.cs` — full proration math with cents-per-seat tables, deterministic session URLs.
- `src/ApiTool.Backend/Subscriptions/StripeGateway.cs` — three methods, all `throw new NotImplementedException("StripeGateway is a placeholder...")`.
- `src/ApiTool.Backend/Subscriptions/StripeOptions.cs` — feature-flag switch (`Mode = "fake" | "live"`).

**What's missing:**
- `Stripe` NuGet SDK reference. Spec mandates it (`docs/SPECIFICATION.md:7980`: `Stripe 9.x`).
- No price-ID resolution from `SubscriptionTier + interval` into Stripe's `price_test_*` IDs (spec line 6740 sketches the table but it's not in code).
- `ComputeProration` is the most architecturally interesting: the `FakeStripeGateway` does it locally, but the real Stripe API computes it server-side via the *upcoming-invoice* preview endpoint. The interface needs to either become async or accept the trade-off that local proration in the fake diverges from real Stripe's.

**Genuinely open design questions REVIEW.md flagged:**
- **Test strategy.** Three options: (a) `stripe-mock` Docker container in CI; (b) recorded fixtures via VCR-style cassettes; (c) keep `FakeStripeGateway` for unit tests, accept zero CI coverage of real-Stripe paths and rely on staging. The spec doesn't pin this. (a) is most defensible but adds a CI service.

**Open questions REVIEW.md missed:**
- **Webhook secret rotation.** The webhook signing secret is per-environment; rotation requires accepting both old and new secrets during a window. Not addressed anywhere.
- **Idempotency-key strategy on outbound calls.** Stripe SDK supports `IdempotencyKey` on `CreateAsync`. Without one, network retries can double-charge. Not in any current code path.

### Gap 7 — Stripe webhook handler

**What exists today:** Nothing. `find src -iname '*webhook*'` returns only the *Slack* outbound webhook poster, unrelated.

**What spec says** (`docs/SPECIFICATION.md:6748–6801`):
- 9 events to handle: `customer.subscription.{created,updated,deleted}`, `invoice.{payment_succeeded,payment_failed,finalized}`, `customer.updated`, `payment_method.{attached,detached}`.
- HMAC-SHA256 timestamp verification, 5-minute replay window, constant-time comparison.
- "Store event IDs in database or Redis, expire after 24 hours" — but no migration exists for an event-log table, and Redis is in `deploy/self-hosted/` for caching, not currently wired as a webhook idempotency store.

**Open questions REVIEW.md missed entirely:**
- **Idempotency storage choice.** Postgres table (durable, joinable, costs an EF migration) vs Redis (fast, ephemeral, requires a client we don't currently use for this). Real-world pick is usually Postgres for the audit trail value.
- **Event ordering / out-of-order delivery.** Stripe doesn't guarantee order. `subscription.updated` can arrive before `subscription.created` for the same record. The handler logic has to be defensive.
- **Failed-handler replay.** When a handler throws, return 500 and Stripe retries (per spec line 6780–6781). The current notifications dispatcher pattern doesn't have a "retry budget" concept — a poison-pill event would loop forever.

### Gap 8 — SMTP provider

**What exists today:**
- `src/ApiTool.Backend/Notifications/ISmtpSender.cs` — single `SendAsync(to, subject, body, ct)`.
- `src/ApiTool.Backend/Notifications/NoopSmtpSender.cs` — logs, doesn't send. Wired as default.
- Test double at `src/ApiTool.Backend.Tests/Notifications/FakeSmtpSender.cs`.

**What spec says — and where REVIEW.md is loose:**
SPECIFICATION.md is **not actually open** on this. SendGrid is pinned in three places:
- Line 6403: "SendGrid: Email addresses, Transactional email, DPA signed"
- Line 7959: "Email Provider | SendGrid | Reliable delivery, good .NET SDK, abstraction allows swap"
- Line 7980: `SendGrid 9.x`
- Line 8126–8131: env var contract (`SENDGRID__APIKEY`, `SENDGRID__TEMPLATES__*`) plus `EmailQueueProcessor` background service consuming from `System.Threading.Channels`.

REVIEW.md:187 frames this as "SMTP provider choice (SendGrid vs SES)." Reading the spec, SendGrid was already chosen.

**The genuinely open question:** Are the spec's SendGrid Dynamic Template IDs (referenced as env vars `SENDGRID__TEMPLATES__*` at line 8129) authored anywhere? They're not in the repo. The provider is decided; the *content* (subject lines, dynamic-template variables, transactional template inventory for password reset, email verification, billing receipts, payment-failed dunning) is undecided.

### Gap 13 — CLI ↔ backend integration

**What exists today:**
- CLI license code in `cmd/curlew/license.go` handles `--validate` and `export` only. `--refresh` and `--debug` return `"Not yet implemented"` and exit 1 (lines 28–30).
- No HTTP client in `internal/` that calls the backend. Search for "api/v1" in `internal/` returns nothing relevant.
- No `curlew login` command. No token storage path beyond the offline-validate cache. The `LicenseCache` shape (`docs/SPECIFICATION.md:7745`) is specified but nothing writes it.

**What's missing — which REVIEW.md correctly flags but doesn't decompose:**
- `curlew login` interactive flow (browser-based OAuth-style? device-code? username/password?)
- License-cache persistence at `~/.config/curlew/license_cache.json` (spec defines shape, no code writes it)
- Refresh-token storage and rotation
- Online JWKS fetch from `/api/v1/public-key` to populate `~/.config/curlew/jwks_cache.json`
- PR-check posting client (note: this is *CLI → backend*. The backend → GitHub posting is a separate gap, **#12, not in M14** — it's slated for M16 per REVIEW.md:191. Easy to confuse.)

**Open questions REVIEW.md flagged:**
- **Auth scheme between CLI and backend.** Spec doesn't pin it. Bearer JWT? Mutual TLS? API key? Almost certainly Bearer JWT given the rest of the design — but no spec section confirms it.

**Open questions REVIEW.md missed:**
- **Where does the CLI's machine identity come from?** Spec line 7758 has a `deviceId` claim. First-run device registration flow isn't specified.
- **Network-failure behaviour for `--refresh`.** If the user runs `curlew license --refresh` and the backend is unreachable, what's the exit code? The existing `--validate` path has a careful taxonomy (exit 6 for verification fail, 2 for no cache, 1 for internal error — see `cmd/curlew/license.go:42–66`). `--refresh` needs an analogous one and the spec doesn't provide it.

---

## Cross-cutting open questions

Ten questions surfaced. REVIEW.md names three explicitly, hand-waves a fourth, misses six.

| # | Question | REVIEW.md status | Why it matters for `/backlog M14` |
|---|----------|------------------|------------------------------------|
| A | Authoritative JWT claims shape (CLI `Claims` vs spec `LicenseCache`) | Listed | Without resolution, every M14 issuance slice issues tokens of an undefined shape; M16 trial slices then need a re-issuer. |
| B | Signing-key storage at rest | **Missed** | Bootstrapping the on-prem bundle vs SaaS-multi-tenant requires different choices. |
| C | Refresh-token model (opaque vs JWT, rotation, table schema) | **Missed** | Touches login, refresh, device management, logout. Cuts across 4+ slices. |
| D | Stripe SDK test strategy (`stripe-mock` vs cassettes vs none) | Listed | Affects CI, slice count, and on-call confidence. |
| E | Webhook idempotency store (Postgres vs Redis) | **Missed** | Schema decision; Postgres needs an EF migration in M14. |
| F | Stripe webhook event-handler retry budget | **Missed** | Defensive coding patterns differ if poison-pill replay loops are possible. |
| G | SendGrid template inventory and content | Loose ("SMTP provider choice") | Provider is pinned; content is the actual missing piece. |
| H | CLI ↔ backend auth scheme | Listed | Implicitly Bearer JWT but not spec'd. |
| I | First-run device registration / device-id minting | **Missed** | `deviceId` claim has no specified origin. |
| J | Trial-claim coupling (M14 issuer must anticipate M16 trial state) | **Missed** | M14 lands tokens; M16 lands trials. If M14 issues tokens lacking trial fields, M16 ships with claim-shape churn. |

---

## Slice count estimate

REVIEW.md:187 names five sub-slices for the original M14 scope. Working from what each gap actually requires, and including Gap 12 (PR-check posting to GitHub) folded in per decision #11:

| Slice cluster | Slices | Notes |
|---|---|---|
| License issuance | 3 | (1) signing-key bootstrap + storage (`signing_keys` table, `IKeyProvider`, `FileKeyProvider`, `GoogleKmsKeyProvider`); (2) `/api/v1/auth/refresh` unified mint endpoint with the v4.2 claim shape; (3) `/api/v1/.well-known/jwks.json` endpoint |
| Live Stripe gateway | 3 | (1) SDK wiring + checkout session; (2) portal session; (3) proration via upcoming-invoice preview (interface change to async) |
| Stripe webhooks | 3 | (1) signature-verify middleware + `stripe_webhook_events` table migration + idempotency pattern; (2) subscription event handlers; (3) invoice + payment-method handlers |
| SMTP / email | 2 | (1) SendGrid sender + EmailQueueProcessor channel host + MJML pipeline; (2) 6-template inventory + manifests + CI upload job + dev-mode preview |
| CLI ↔ backend | 4 | (1) `curlew login` (device-code flow) + cache write + hybrid keychain storage + `flock`; (2) `license --refresh` + `--debug` + exit-code taxonomy; (3) JWKS fetch + cache; (4) HTTP client foundation in `internal/backend/` |
| GitHub Checks API integration (Gap 12, folded in) | 4–6 | (1) GitHub App registration runbook + private-key handling + JWT signing for installation-token requests; (2) `installations` table + per-org credential storage + linking flow; (3) `PrCheck` entity expansion (`check_run_id`, `installation_id`, `conclusion`, `details_url`, `output_summary`, `output_text`) + payload mapping from internal state to Checks API shape; (4) outbound `POST /repos/{owner}/{repo}/check-runs` + retry/error handling; (5, optional) inbound webhook handler for re-run events (`check_run.rerequested`); (6, optional) Web-dashboard wiring for the App-install entry point. Slices 5–6 may be deferred; minimum viable is slices 1–4 |

**Total: ~19–21 slices.** The biggest milestone shipped to date is M2 at 34 slices, but those were highly parallel and lower-risk per slice. M14's ~20 are a tighter graph (the issuer slice gates Stripe webhooks and CLI-refresh both; the GitHub-installation slice gates the outbound Checks API slice), and each one has more T1 risk per slice than any of M2's. The Gap-12 cluster does NOT depend on the issuer or Stripe clusters — it's parallelisable with the SMTP cluster on a separate track.

---

## Recommended pre-work before `/backlog M14`

1. **SPECIFICATION.md v4.2 edition** with sections covering:
   - JWT claims authoritative shape (resolves A, J)
   - Signing-key storage and rotation operational story (resolves B, partially I)
   - Refresh-token table schema + rotation policy (resolves C)
   - CLI ↔ backend endpoint table with auth scheme + error model (resolves H, I)
   - Webhook idempotency store choice + event-replay budget (resolves E, F)
   - Stripe test strategy decision (resolves D)
   - SendGrid template inventory (resolves G — content, not provider)
2. **An M14 section in `.claude/skills/backlog/milestone-mapping.md`** with capability keys, expected slice counts, the DAG (issuer ← webhooks; issuer ← CLI-refresh; SMTP independent; Stripe-checkout independent), and track assignments (mostly `backend`, some `go-cli`, no `web`).
3. ~~**Decide whether to fold Gap 12 (PR-check posting *to* GitHub) into M14 or keep it in M16.**~~ **Resolved 2026-05-03: fold Gap 12 into M14 (Option A).** The backend → GitHub Checks API integration moves out of M16 and into M14. Rationale: although the no-public-launch-until-M19-complete commitment would have made Option C ("ship M14 with inert PR-check posting, complete in M16") technically defensible, the user accepted the M14 expansion to avoid setting a precedent for shipping user-visible features that silently fail to do their documented job. Net effect on M14 scope: adds ~4–6 slices for GitHub App registration, installation-token flow, Checks API posting, the `PrCheck` entity expansion (`check_run_id`, `installation_id`, `conclusion`, `details_url`, `output_summary`), and an `installations` table for per-org GitHub App credential storage. This raises the slice estimate from ~15 to ~19–21. **GitLab parity is explicitly NOT in M14** — GitHub-only is the M14 commitment; GitLab can land in a later milestone. Note: this resolution introduces a new pre-work requirement — `docs/SPECIFICATION.md` v4.2 currently has no GitHub-side coverage. Either a v4.2.1 supplement or a focused mini-design-pass is needed before `/backlog M14` runs, covering the GitHub App architecture (App registration, JWT signing for installation-token requests, two-stage auth flow, payload mapping from internal `PrCheck` state to GitHub Checks API `conclusion`/`output`, optional inbound webhook handling for re-run events, `installations` schema). The other cross-cutting questions in this investigation are settled; only the GitHub-side is open.

4. **`docs/SPECIFICATION.md` v4.2.1 GitHub-side supplement** (newly required by the Gap 12 fold-in). Covers: GitHub App registration runbook + private-key custody (parallel to the JWT signing-key custody decision in §2 — `FileKeyProvider` pattern for self-hosted, KMS-backed for SaaS); JWT signing for installation-token requests (App ID `iss`, ≤10-minute `exp`, RS256 per GitHub's requirement — note this is the one place ES256 does NOT apply, because GitHub mandates RS256 for App JWTs); the two-stage auth flow (App JWT → installation token → Checks API call); `installations` table schema; `PrCheck` entity expansion; payload mapping from internal `state ∈ {success, failure}` to GitHub's richer `conclusion ∈ {success, failure, neutral, cancelled, skipped, timed_out, action_required}` plus `output.{title, summary, text, annotations}`; optional inbound webhook handling (`check_run.rerequested`) using the same idempotency-store pattern as the Stripe webhook section; rate-limit handling (5000 req/hour per installation); error taxonomy.

Until items 1, 2, and 4 land, `/backlog M14` blocks — at SKILL.md Step 1 for #2, and at "the Plan agent will generate slices against undefined claim shapes" for #1 and #4. v4.2 (item 1) is now drafted; #2 and #4 remain open.

---

## Alternatives if M14 design pass isn't appetising right now

All of these are unblocked and self-contained:

- **M15-trivial subset.** Plugin tier-gate (one-line `internal/auth/registry.go` entry plus enforcement) and `license --debug` (small, well-defined). Would need its own M15 mapping section, but the design questions are minimal.
- **Schemastore.org publishing.** Deferred from M11 until a tagged release (see `management/backlog.yaml:1113`).
- **`--locale` for faker.** Deferred from M13. Could be a follow-up "M13.5" or sequenced as `faker_locale` capability under a later milestone.
- **SPECIFICATION.md v4.2 to document the M17 `signing:` field.** M17 notes flagged this as a separate follow-up; it's a doc-only task.

---

## Status of related milestones (for context)

Per REVIEW.md:179–217, the planned roadmap and current status:

| Milestone | Theme | Status |
|-----------|-------|--------|
| M12 | Foundational dynamic-function helpers | done |
| M13 | Faker depth | done |
| M14 | Revenue plumbing (this doc) | blocked on design pass |
| M15 | CLI partial fixes (`license --refresh`, `license --debug`, plugin_loading tier gate) | partially blocked (refresh depends on M14) |
| M16 | Workflow completion (password reset, email verify, trial issuance, schedule executor, PR check posting, etc.) | needs design passes for several sub-items |
| M17 | First-party signers/signing helpers | done (jumped ahead because it had no open design questions) |
| M18 | Compliance and launch readiness | not generated; deliberately deferred |
| M19 | Strategic expressiveness (`if:` field, CEL/Starlark eval) | not generated; deliberately deferred |

M17 was not "next" in the roadmap — it was the only unblocked milestone that didn't need a design pass. M14/M15/M16/M18/M19 were skipped, not completed.

---

## v4.2 design decisions

Investigation date: 2026-05-03. All decisions resolved 2026-05-03. Research grounded in RFC 8725 (JWT BCP), RFC 9700 (OAuth 2.0 Security BCP), RFC 9449 (DPoP), RFC 8628 (Device Authorization Grant), Stripe's webhook documentation, and current KMS/Key Vault best-practice guidance. This section is the authoritative design reference for the seven v4.2 sub-sections that will be written into `docs/SPECIFICATION.md`. Each subsection below is a settled commitment, not a proposal — see the resolved-decisions summary at the end for the ten load-bearing choices and their rationale in one place.

**Release-cadence assumption.** There is no user-facing release between M14 and the broader pre-launch milestone set (M15, M16, M18, M19) — everything lands together before any user touches it. Consequently, decisions in this pack are made on their own merits (security, implementation cost, operational simplicity) and never on phantom shipping-window grounds like "ship the easier option in M14, harden in M18." Either an option is the right answer for the system or it is not. "Out of M14 scope" only appears when something genuinely belongs to a different milestone's scope, never as a euphemism for deferred hardening.

### 0. Token model (architectural foundation for §1, §3, §4)

**Three tokens, three different roles, three different lifetimes.** This separation is essential: if normal CLI operation requires our backend to be reachable, then backend uptime becomes a hard dependency for the tool's core value, drastically raising operational/monitoring requirements and exposing every user to an outage cascade. The model below decouples the CLI's primary function (running tests against the user's own APIs) from backend availability.

| Token | What it proves | Lifetime | Verified | Used for | If backend down |
|---|---|---|---|---|---|
| **License JWT** | "User is licensed at tier X with features Y" | 30 days valid + 14-day grace period | Offline (CLI uses embedded JWKS / cached JWKS) | Every CLI invocation; gates feature access | CLI keeps working until grace exhausted (~6 weeks worst case) |
| **Access token** | "Bearer may call backend API endpoints" | 1 hour | Online (backend validates per call, or signed JWT it verifies locally) | Backend-touching commands only: `report upload`, `pr-checks post`, `account view`, `license refresh` itself | Those commands fail with "backend unreachable, retry later" — `curlew run` is unaffected |
| **Refresh token** | "Bearer may mint new License JWT + Access token" | 90-day sliding / 365-day absolute | Online (DB lookup, family-revocation check) | `/auth/refresh` only | After 365-day absolute expiry, user runs `curlew login` once |

**Why 1 hour for access tokens (not the 15 minutes recommended for typical SaaS):** CLI usage is bursty — a developer runs `curlew run` ten times in a debugging session, then nothing for hours. With 15-minute tokens, every session boundary triggers a refresh. With 1-hour tokens, most active sessions need zero refreshes, and revocation latency remains acceptable for the threats this tool faces (chargeback, ToS violation, leaked CI credential). Industry comparison: `gh` CLI uses 8-hour tokens; `gcloud` uses ~1-hour tokens. 15 minutes is right for a banking app or admin console; for a CLI testing tool it produces unnecessary refresh churn.

**Why 30+14 days for the License JWT:** the offline-grace machinery already shipped as M5-013/014 — the architecture exists; v4.2 only needs to use it correctly. 30-day validity means most users refresh once per month silently. 14-day grace means a single missed refresh window does not lock anyone out. Combined: a backend outage of up to 6 weeks (worst case for a user who refreshed just before the outage) does not interrupt the CLI's primary function.

**Background refresh pattern:**
- License JWT: when CLI is invoked AND license JWT is < 7 days from expiry AND backend is reachable, fire-and-forget background refresh. Never blocks the user. If refresh fails, retry on next invocation.
- Access token: lazy refresh — only attempted when the CLI is about to call a backend endpoint AND the token has < 5 minutes remaining. If refresh fails, the backend-touching command fails with a clear "backend unreachable" message; the CLI invocation overall does not fail unless that command was the only thing requested.
- Refresh token: never refreshed proactively; the user explicitly runs `curlew login` to start a new family.

**Revocation latency trade-off (License JWT):** because the License JWT is offline-verified for up to 30 days valid + 14-day grace = 44 days, we cannot instantly revoke a user's license. Three escape valves:
1. **Default — wait for expiry.** Acceptable for ToS violations, downgrades, cancellations: the user's "stolen window" is bounded by JWT lifetime.
2. **Severe incidents — key rotation** (per §2). Removing the signing key from JWKS invalidates all License JWTs signed under it. Nuclear, but available.
3. **Optional addition — lazy revocation list.** CLI fetches `/api/v1/license/revocations` once per refresh cycle; `jti` match → treat as revoked. Out of M14 scope by design: the value-add over expiry-plus-key-rotation is marginal for an API testing tool, and the architecture above does not need to change to add it later — it's a single endpoint plus a CLI cache file. Excluded unless a concrete revocation-latency requirement emerges before launch.

**CLI flow:**
1. `curlew run` → check License JWT validity (offline) → if valid, proceed → if expired but within grace, proceed with warning → if grace exhausted, refuse with clear instructions to run `curlew license --refresh` (or `curlew login`).
2. `curlew report upload` → check Access token validity → if expired/expiring, attempt silent refresh → if refresh succeeds, upload → if refresh fails, queue upload to `~/.config/curlew/pending-uploads/` for next successful refresh.
3. Background daily timer (only when CLI is invoked) → if License JWT < 7 days from expiry and backend reachable, refresh silently.

### 1. Token claim shapes (resolves A and J)

**Algorithm: ES256 (ECDSA with P-256 curve and SHA-256)** for both License JWT and Access token. Pre-launch is the cheapest possible migration window — no tokens in the wild, no compatibility burden, ~hours of CLI verifier rewrite — and the wins compound for the lifetime of the product:

- **Token size.** ES256 signatures are 64 bytes vs RS256-4096's ~512 bytes. A typical License JWT shrinks from ~1.4 KB to ~0.9 KB. Materially cheaper for embedded JWKS in the CLI binary, license-cache files, log lines, and every backend API call carrying the Access token in `Authorization`.
- **Signing throughput.** ES256 signs at ~10,000 sigs/sec vs RS256-4096's ~3,000 sigs/sec. Verification is the inverse but verification is not the bottleneck — it happens at the CLI (offline, no scale concern) and at backend per-API-call (1000s QPS is a non-issue with either).
- **Key footprint.** EC public keys are 64 bytes vs RSA-4096 public keys at 512 bytes. Embedded JWKS in CLI binary stays small; rotation overhead is negligible.
- **Modernity.** Every greenfield auth system designed in 2026 picks ES256 or EdDSA. RS256 is what you keep when you have legacy tokens to verify.

Per RFC 8725 §3.1, the verifier whitelists exactly one algorithm (`ES256`) and rejects everything else, defending against algorithm-confusion attacks (including the classic "swap RS256 for HS256 with the public key as MAC secret" downgrade). Per RFC 8725 §3.2, the ES256 implementation MUST use RFC 6979 deterministic ECDSA — Go's `crypto/ecdsa` does NOT do this by default; the implementation uses `crypto/ecdsa.SignASN1` with deterministic nonce derivation per RFC 6979 (or a vetted library that wraps it). This eliminates the nonce-recovery class of attacks that has historically broken several ECDSA implementations.

**Why ES256 over EdDSA (Ed25519):** EdDSA is technically nicer (deterministic by construction, slightly faster, no nonce-recovery class of bugs by design) but cloud-provider support is uneven — AWS KMS does not support EdDSA for asymmetric signing; Azure Key Vault supports Ed25519 only on the expensive managed-HSM tier; Google Cloud KMS does not currently support EdDSA for signing JWTs. ES256 is universally supported across cloud KMS offerings, including the chosen Google Cloud KMS HSM tier (per decision #2). ES256 is the final algorithm choice; EdDSA is not a planned upgrade path.

Both tokens are signed by the same `IKeyProvider` — only one signing-key registry to operate. The `kid`-based registry (per §2) decouples algorithm choice from claim shape, so a future re-evaluation (e.g., post-quantum) is a key-rotation event, not an architectural change.

**CLI verifier change:** [internal/license/jwt.go](internal/license/jwt.go) currently exports `VerifyRS256`. Rename to algorithm-agnostic `VerifyJWT(token, jwks)` that dispatches on the JWS header `alg` value and rejects anything not in the registered-algorithms allowlist (initially `{"ES256"}`). This satisfies RFC 8725 §3.1 strictly and accommodates a future EdDSA migration without re-renaming.

**Explicit typing per RFC 8725 §3.11** (defense against JWT confusion between systems): License JWT uses `typ: "license+jwt"`; Access token uses `typ: "at+jwt"` per RFC 9068 (OAuth 2.0 Access Token JWT profile). The CLI's License-verifier MUST reject any JWT with `typ != "license+jwt"`; the backend's Access-token validator MUST reject any JWT with `typ != "at+jwt"`. Cross-use is impossible by construction.

**License JWT claim shape:**

| Claim | Type | Notes |
|---|---|---|
| `iss` | string | `https://api.apitool.dev` (env-suffixed in dev/staging) |
| `aud` | string | `curlew-license` — License JWT audience; verified offline by CLI |
| `sub` | UUID string | User ID; immutable |
| `exp` | int | 30-day lifetime |
| `nbf` | int | `iat - 30s` skew tolerance |
| `iat` | int | Issuance time |
| `jti` | UUID string | Unique per token; future revocation-list lookup |
| `email` | string | Display only — never used for authz |
| `tier` | enum | `free \| solo \| team \| enterprise \| self_hosted` |
| `features` | string[] | Active feature flags for tier+org |
| `request_limit` | int | Daily request quota for tier |
| `org_id` | UUID string \| null | UUID for team/enterprise; null otherwise |
| `org_role` | enum \| null | `member \| admin \| owner` when `org_id` present |
| `device_id` | UUID string | Server-minted on device registration |
| `trial_state` | enum | `none \| active \| expired \| extended` — defaults to `none` in M14 |
| `trial_expiry` | int \| null | Unix timestamp; null when `trial_state = none` |
| `grace_until` | int | `exp + 14 days` — explicit grace-period boundary, makes CLI logic trivial |

The trial fields are why this resolves J: M14's issuer writes them with `none`/`null` defaults; M16's trial slices populate them; no claim-shape churn between M14 and M16.

**Access token claim shape** (deliberately minimal — backend looks up the rest):

| Claim | Type | Notes |
|---|---|---|
| `iss` | string | Same as License JWT |
| `aud` | string | `curlew-cli-api` — Access token audience; backend rejects mismatched `aud` |
| `sub` | UUID string | User ID |
| `exp` | int | 1-hour lifetime |
| `nbf` | int | `iat - 30s` skew tolerance |
| `iat` | int | Issuance time |
| `jti` | UUID string | Unique; enables fast cache-based revocation |
| `tier` | enum | Used for tier-gated endpoint authorization (cheap check without DB lookup) |
| `org_id` | UUID string \| null | For org-scoped endpoint authorization |
| `device_id` | UUID string | For audit logs and per-device rate limiting |

Authorization-relevant claims (`tier`, `org_id`, `device_id`) are duplicated into the Access token so the backend can enforce tier-gates and org-scope without a DB lookup on every API call. Volatile claims like `request_limit` (which the backend already tracks in real-time) and `email` (display-only) are NOT in the Access token.

**Moved out of both tokens** (vs the current `LicenseCache` at SPECIFICATION.md:7745):
- `sessionId` → belongs in the refresh-token chain (see §3), not in either of the user-facing tokens.
- `refreshToken` → never embedded in any signed token.

**CLI-side change:** `internal/license/jwt.go` `Claims` struct extended to the License JWT shape above (currently 9 fields; this adds 9). Backward-compatible because new fields default to zero values when verifying old test fixtures. A second small struct `AccessClaims` is added for the Access token shape — used only by the future HTTP client that calls our backend.

### 2. Signing-key storage + rotation (resolves B, partially I)

**Two backends behind one `IKeyProvider` interface.** Best-practice from research (cloud-KMS-backed JWT signing as used by GitHub Apps + Azure Key Vault, Stripe + AWS KMS, etc.): the private key stays inside an HSM boundary so a compromised app server can request *signatures* during the breach window but cannot exfiltrate the key.

| Deployment | Implementation | Where private key lives |
|---|---|---|
| `deploy/self-hosted/` | `FileKeyProvider` | `Keys/signing/<kid>.pem` mode 0600, owned by service account; operator generates at install |
| SaaS multi-tenant | `GoogleKmsKeyProvider` | Asymmetric `EC_SIGN_P256_SHA256` key in Google Cloud KMS HSM tier (FIPS 140-2 Level 3). Signatures via `projects.locations.keyRings.cryptoKeys.cryptoKeyVersions.asymmetricSign`. Private key never leaves the HSM |

**Multi-product context.** Curlew is one of four planned SaaS products at roughly the same scale, all sharing a single GCP account for KMS. Each product gets its own signing key (security boundary that matters); all keys live in the same KMS service (one ops surface, one billing line, one IAM model). Per-product backend service accounts have `roles/cloudkms.signerVerifier` granted only on their own product's keys, enforcing the boundary at the IAM layer. With ~3 active key versions per product × 4 products = ~12 active key versions, monthly KMS cost is ~$12 (or $0 for ~25 months on the $300 new-account credit). The `IKeyProvider` interface abstracts the per-product key URI so the same provider implementation serves all four.

Same `IKeyProvider` surface: `SignAsync(payload) → signature`, `GetActiveKidAsync()`, `GetVerificationJwksAsync() → JWKS`. Tests use an in-memory provider; integration tests use `FileKeyProvider` with ephemeral keys.

**JWKS endpoint: `GET /api/v1/.well-known/jwks.json`** — uses RFC 8615 well-known URI convention (renames the spec's bespoke `/api/v1/public-key`). Returns active key plus all keys still within the 60-day verification window. `Cache-Control: public, max-age=3600`.

**`kid` format:** `<env>-<alg>-<YYYYMM>-<6char-uuid>`, e.g. `prod-es256-202605-a3f4d2`. Per RFC 8725 §3.10, the verifier MUST validate `kid` against an allowlist regex (`^[a-z0-9-]{1,64}$`) before any lookup — prevents SQL/path injection through the header.

**Rotation:**
- Active signing key rotated every **90 days**.
- Old key kept in JWKS for **60 days** post-rotation (verification window). The window must exceed the longest-lived in-flight JWT, which is a License JWT signed just before rotation: 30 days valid + 14 days grace = 44 days. 60 days gives comfortable margin. (Refresh tokens are opaque DB-looked-up secrets and do not depend on the signing-key verification window — only JWTs do.)
- A `next` key is always pre-staged (loaded but not yet signing) and published in JWKS so emergency rotation is a flag flip.
- **Emergency rotation:** `curlew-backend keys rotate --emergency` promotes `next` to `current`, removes old key from JWKS within 1 minute (cache-bust), revokes all refresh tokens issued under the compromised key.

**`signing_keys` table** (registry):

```sql
CREATE TABLE signing_keys (
  kid              TEXT PRIMARY KEY,
  algorithm        TEXT NOT NULL,            -- 'ES256'
  status           TEXT NOT NULL,            -- 'current' | 'next' | 'verifying' | 'revoked'
  kms_key_id       TEXT,                     -- Google KMS resource URI 'projects/.../cryptoKeyVersions/N' for SaaS; NULL for FileKeyProvider
  public_key_jwk   JSONB NOT NULL,           -- cached for JWKS responses
  created_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  promoted_at      TIMESTAMPTZ,              -- when status became 'current'
  retiring_at      TIMESTAMPTZ,              -- when status becomes 'revoked'
  revoked_at       TIMESTAMPTZ,
  revoke_reason    TEXT
);
CREATE UNIQUE INDEX idx_signing_keys_current ON signing_keys(status) WHERE status = 'current';
CREATE UNIQUE INDEX idx_signing_keys_next    ON signing_keys(status) WHERE status = 'next';
```

### 3. Refresh-token model (resolves C)

**Opaque tokens, not JWTs.** A JWT refresh token forces you to either trust it (no revocation) or hit DB anyway (no benefit over opaque). Opaque + DB-lookup gives complete revocation control; refreshes are infrequent (License JWT refresh ~once/month per CLI, Access token refresh ~once/hour during active sessions), so the DB hit is negligible.

**The refresh endpoint mints both tokens in one round-trip.** A successful `POST /api/v1/auth/refresh` returns `{license_jwt, access_token, refresh_token}` — the new License JWT (30 days), new Access token (1 hour), and rotated refresh token. CLI persists all three. This collapses what would otherwise be two round-trips into one and keeps the two user-facing tokens' lifetimes synchronised at refresh boundaries.

**Format:** 256-bit cryptographically random, base64url-encoded (~43 chars). Stored in DB only as SHA-256 hash — never plaintext.

**Schema:**

```sql
CREATE TABLE refresh_tokens (
  id              UUID PRIMARY KEY,
  token_hash      BYTEA NOT NULL UNIQUE,                   -- SHA-256 of token
  user_id         UUID NOT NULL REFERENCES users(id),
  device_id       UUID NOT NULL REFERENCES devices(id),
  family_id       UUID NOT NULL,                            -- shared across rotated descendants
  parent_id       UUID REFERENCES refresh_tokens(id),       -- prior token; NULL for root
  issued_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
  expires_at      TIMESTAMPTZ NOT NULL,                     -- LEAST(now()+90d, family_root.issued_at+365d)
  rotated_at      TIMESTAMPTZ,                              -- non-null = already used (reuse-detection trigger)
  revoked_at      TIMESTAMPTZ,
  revoke_reason   TEXT,
  last_used_ip    INET,
  user_agent      TEXT
);
CREATE INDEX idx_refresh_tokens_family      ON refresh_tokens(family_id);
CREATE INDEX idx_refresh_tokens_user_device ON refresh_tokens(user_id, device_id);
```

**Lifetimes** (calibrated to RFC 9700 §4.14 and the §0 token model):
- Refresh token sliding window: 90 days from last use.
- Refresh token absolute lifetime: 365 days from family root.
- License JWT: 30 days valid + 14-day grace (offline-verified; see §0).
- Access token: 1 hour (per §0 reasoning — CLI usage is bursty, 15-min churn unjustified).

The loose refresh-token profile (90/365) is chosen deliberately to minimise re-auth friction — active CLI users effectively never see expiry, and even occasional users (CI runners that fire once a month) stay logged in for a year. The annual forced re-login serves as a hygiene measure for abandoned-laptop / employee-turnover scenarios. RFC 9700 §4.14 mandates rotation-on-every-use (which we implement) but does not pin specific lifetimes — `gcloud` and `gh auth` both treat refresh tokens as essentially indefinite for similar reasons.

**Rotation on every use** (RFC 9700 §4.14 mandate for public clients):
- On refresh: mark old token's `rotated_at`, mint new token sharing `family_id`, set new token's `parent_id` to old.
- **Reuse detection:** if a presented token's `rotated_at` is non-null, **revoke the entire `family_id`** — the canonical OAuth security pattern (Auth0, Okta, Stripe). Force re-auth, log a high-priority security event, send `account_security_alert` email.

**CLI-side single-flight lock for refresh.** Required because reuse-detection is unforgiving — two concurrent CLI invocations both attempting to refresh the same token would race: one succeeds and rotates the token; the other presents the now-rotated token and triggers family revocation, logging the user out as if they were under attack. The CLI implementation MUST hold an exclusive `flock` on `~/.config/curlew/refresh.lock` (or platform equivalent) for the duration of any refresh call. Concurrent invocations block on the lock; once the first refresh completes and updates the cache, subsequent invocations re-read the cache and find a fresh token, no second refresh needed. The lock has a hard timeout (5s) after which the waiting invocation falls back to re-reading the cache and proceeding with whatever token is there — preventing deadlock if a previous CLI process crashed mid-refresh and left a stale lock file. This is not optional once family-revocation is in play; without it, normal user behaviour (running two `curlew` commands in adjacent terminals) randomly nukes their session.

**Device binding (resolves part of I):**
- First-run CLI registers device: `POST /api/v1/devices` with `{name, fingerprint}`. Backend mints `device_id` UUID, returns it.
- CLI persists `device_id` at `~/.config/curlew/device.json` (mode 0600).
- Refresh requests must include `device_id`; backend rejects if it doesn't match the refresh token's bound `device_id`.
- Fingerprint = SHA-256 of `(machine-id + hostname + OS)`. Best-effort signal, NOT primary binding (machines can be cloned). Primary binding is the server-minted `device_id` UUID.

**CLI-side storage:** RFC 9700 §4.10.1 forbids plaintext storage. Three options:
- **(a)** OS keychain only (macOS Keychain, Windows Credential Manager, Linux `secret-tool` / libsecret). Strongest where available — but Linux `secret-tool` requires a running keyring daemon (gnome-keyring or KWallet) which is frequently absent on servers, headless boxes, CI runners, and Docker containers — exactly the environments an API testing tool is invoked in.
- **(b)** Encrypted file only — AES-256-GCM with a key derived from `(device_id + machine-id)` via HKDF, stored at `~/.config/curlew/refresh_token.enc` mode 0600. Single code path, no native deps, always works. Loses to anything that can read the file as the user.
- **(c)** Hybrid (the `gh` CLI pattern): try OS keychain first; fall back to encrypted file when keychain is unavailable. Two code paths but matches user expectations and handles every environment correctly.

Recommendation: **(c) hybrid.** Without a phantom shipping window forcing an "easy now / harden later" trade-off, the right call is to do this correctly the first time. Cross-platform keychain integration is a known quantity — Go libraries (e.g. `zalando/go-keyring`) cover all three OSes, and the encrypted-file fallback is the same code path option (b) would have shipped. Costs ~1 extra slice in M14 vs (b); avoids re-opening the storage decision later.

**Why NOT DPoP** (per RFC 9449 research): DPoP would sender-constrain tokens cryptographically. But it requires per-request JWT signing on CLI side and public-key verification on every API call backend-side. Heavy implementation cost (~3 slices alone) for a threat model that refresh-rotation + device-binding largely covers. Out of scope by design — not because it's "deferred," but because it's the wrong cost/benefit for an API testing tool. Re-evaluate only if a concrete sender-constraint requirement emerges before launch.

### 4. CLI ↔ backend endpoints, auth scheme, error model (resolves H, I)

**Auth scheme:** Bearer JWT in `Authorization: Bearer <access_token>`. Refresh token always passed in request body (never headers, never URL params — RFC 9700 §4.10 storage rules and the URL-leak class of bugs).

**Login flow: device-code grant (RFC 8628), with browser auto-open when available.** What `gh`, `stripe`, `gcloud` all do. Reasoning:
- Works in headless contexts (SSH, Docker, CI).
- No requirement for the CLI to spin up a local HTTP listener (which fails behind firewalls and on locked-down dev VMs).
- The CLI shows a URL + 8-character user code; user opens browser, signs in, confirms code; CLI polls until success.
- Cross-device authentication works: developer can SSH into a server and authenticate using their normal browser session on their laptop or phone — no shared filesystem or port forwarding required.

**Auto-open behaviour** (matches `gh auth login`): when the CLI detects an interactive TTY AND the platform has a default browser registered (best-effort detection — Go's `os/exec` invoking `open` on macOS, `xdg-open` on Linux, `start` on Windows), the CLI fires off the browser pointed at `verification_uri_complete` (the full URL with the code pre-filled, per RFC 8628 §3.3.1) in addition to printing the URL + code to stdout. User still sees the printed code as fallback. When `--no-browser` is passed or the TTY check fails, only the print path runs. PKCE + browser-callback is explicitly NOT supported as an alternative — supporting it would mean implementing two flows for the small UX win of one specific context (laptop with default browser available), at the cost of every CI/SSH/Docker user being a second-class citizen.

**Endpoint table:**

| Method | Path | Auth | Purpose |
|---|---|---|---|
| `POST` | `/api/v1/auth/device/start` | none | Initiate device-code flow → `{device_code, user_code, verification_uri, expires_in, interval}` |
| `POST` | `/api/v1/auth/device/poll` | device_code | CLI polls every `interval` seconds → `{license_jwt, access_token, refresh_token, device_id}` once user confirms |
| `POST` | `/api/v1/auth/refresh` | refresh_token + device_id (body) | Returns `{license_jwt, access_token, refresh_token}` — all three minted in one round-trip; old refresh rotated |
| `POST` | `/api/v1/auth/revoke` | refresh_token (body) | Logout; revokes refresh token family |
| `POST` | `/api/v1/devices` | bearer | Register a new device (called once on first auth) |
| `GET` | `/api/v1/.well-known/jwks.json` | none | Public key set; cacheable. Used by CLI to verify both License JWT and (if it ever needs to) Access token |
| `GET` | `/api/v1/me` | bearer | Account view: tier, features, request_limit consumed today, org details, billing summary |
| `POST` | `/api/v1/pr-checks` | bearer | (already exists) PR-check upload from CLI |

There is **no separate `/api/v1/license/issue`** endpoint — `/auth/refresh` is the unified issuance path that mints all three tokens. Simplifies the CLI surface: `curlew license --refresh` calls `/auth/refresh`, parses the response, persists all three tokens.

**Error model: RFC 7807 Problem Details (`application/problem+json`):**

```json
{
  "type": "https://api.apitool.dev/errors/refresh-token-reused",
  "title": "Refresh token reuse detected",
  "status": 401,
  "detail": "Token has been rotated; entire token family revoked. Re-authenticate via 'curlew login'.",
  "code": "AUTH_REFRESH_REUSED",
  "request_id": "req_a3f4d2c1"
}
```

- `code` is the stable machine-readable identifier the CLI maps to exit codes.
- `request_id` is the correlation ID (already a project pattern).
- `type` URLs resolve to docs pages explaining each error.

**CLI exit code taxonomy for `curlew license --refresh`** (mirrors and extends `--validate` at `cmd/curlew/license.go:42–66`). `--refresh` requests a new License JWT + Access token + rotated refresh token in one round-trip:

| Exit | Meaning | Triggers |
|---|---|---|
| 0 | Success | All three tokens refreshed and cached |
| 1 | Internal error | Unexpected; bug |
| 2 | No cache | No refresh token stored — run `curlew login` |
| 3 | Network failure | Cannot reach backend; previous License JWT still valid (within 30d + 14d grace) — CLI normal operation continues |
| 4 | Refresh expired | Refresh token past sliding-window or absolute deadline; re-auth required |
| 5 | Family revoked | Security event — reuse detected or admin-initiated revocation; re-auth required |
| 6 | Server error | 5xx response; retry later. Previous License JWT remains valid |
| 7 | Device not registered | First run; needs `curlew login` to register |

Note that exit codes 3 and 6 (network/server failure) are **non-fatal for normal CLI operation** — the user can keep running `curlew run` against their own APIs because the License JWT is offline-verified and remains valid until expiry+grace exhausts. The CLI logs a warning ("license refresh failed, will retry next invocation") and exits non-zero only because the user explicitly asked for a refresh.

### 5. Webhook idempotency + replay budget (resolves E, F)

**Idempotency store: Postgres, not Redis.** Reasoning:
- **Audit trail value.** "Did event evt_X arrive on May 3?" is answerable in Postgres; in Redis it's gone after TTL.
- **No new infrastructure.** Postgres is in the stack; Redis is currently optional/cache-only.
- **Transactional safety.** Idempotency check AND business-logic write in one transaction. With Redis, cache update and DB write are non-atomic; Stripe-retry races can double-process.
- **Cost is negligible.** Stripe retries up to 3 days; even a busy SaaS sees thousands of events/day, not millions. 90-day retention is hundreds of MB.

**Schema:**

```sql
CREATE TABLE stripe_webhook_events (
  event_id        TEXT PRIMARY KEY,                         -- Stripe's evt_... ID
  event_type      TEXT NOT NULL,                            -- e.g. 'invoice.payment_succeeded'
  received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
  processed_at    TIMESTAMPTZ,                              -- NULL = in-flight or failed
  status          TEXT NOT NULL DEFAULT 'pending',          -- pending | processed | quarantined
  payload         JSONB NOT NULL,                           -- raw event for replay/forensics
  attempt_count   INT NOT NULL DEFAULT 0,
  last_error      TEXT,
  last_error_at   TIMESTAMPTZ
);
CREATE INDEX idx_webhook_events_status_received ON stripe_webhook_events(status, received_at DESC);
CREATE INDEX idx_webhook_events_type_received   ON stripe_webhook_events(event_type, received_at DESC);
```

**Idempotency pattern:**

```sql
INSERT INTO stripe_webhook_events (event_id, event_type, payload)
VALUES ($1, $2, $3)
ON CONFLICT (event_id) DO NOTHING
RETURNING received_at;
```

First time → row returned → process. Duplicate → no row → return 200 immediately.

**Retention: 90 days.** A daily cleanup job deletes rows where `received_at < now() - 90 days AND status = 'processed'`. Quarantined events retained indefinitely for investigation.

**Event-ordering defense** (Stripe explicitly does NOT guarantee order):
- All handlers are idempotent and order-independent by design.
- For subscription events: re-fetch `Subscription` from Stripe API on receipt — trust Stripe's current state, not the event payload's snapshot. Handles `subscription.updated` arriving before `subscription.created`.
- Pattern: handlers ensure "DB state matches Stripe's current state for this object," never "apply this delta."

**Failed-handler retry budget:**
- Each retry increments `attempt_count`, sets `last_error`/`last_error_at`.
- After **5 internal failures**, mark `status = 'quarantined'` and return 200 to Stripe (preventing infinite retry storm).
- Quarantined events log to a high-priority alerting queue for manual investigation.
- Quarantine prevents poison-pill events from blocking newer ones.

**Signature verification:** `Stripe.Net`'s `EventUtility.ConstructEvent(json, signatureHeader, secret)` — handles HMAC-SHA256, 5-min default tolerance, constant-time comparison. Never set tolerance to 0 (disables timestamp verification entirely).

**Webhook secret rotation:** configuration accepts multiple active secrets via `STRIPE__WEBHOOK_SECRETS` (comma-separated list, NOT singular `STRIPE__WEBHOOK_SECRET`). Each verification attempt iterates the list; success on any passes. Supports Stripe's documented 24-hour rotation grace window.

### 6. Stripe test strategy (resolves D)

**Three-layer strategy:**

1. **Unit tests** — mock `IStripeGateway` directly. `FakeStripeGateway` already exists for proration math; reuse for handler-logic unit tests. No network, milliseconds per test.
2. **Integration tests** — `stripe-mock` Docker container in CI. Verifies our `StripeGateway.cs` correctly invokes Stripe SDK and parses responses. Stripe-maintained, regenerated from their OpenAPI spec.
3. **Smoke tests against Stripe test mode** — manually run before each release tag. Real Stripe test account, real webhooks via `stripe listen`. Catches things stripe-mock doesn't (proration math, real event payload shapes for new event types).

**Why NOT cassettes/VCR:** Stripe's API surface is wide and changes often; cassettes go stale silently and require manual refresh. stripe-mock is Stripe-maintained and OpenAPI-driven — kept current automatically.

**CI changes:**
- `docker-compose.test.yml` gains a `stripe-mock` service on `:12111`.
- Backend integration tests use `[Trait("Category", "stripe-integration")]` and connect to that endpoint.
- ~30s container startup, runs once per CI job.
- The existing `./scripts/ci-local.sh` already auto-detects backend changes — extend the backend gate to spin up stripe-mock.

**Known limitation to document:** stripe-mock is stateless. Integration tests cannot exercise real proration math; they verify request-shape and response-parse correctness. Real proration drift is caught by the test-mode smoke pass before release.

### 7. SendGrid template inventory (resolves G)

**Source-of-truth: in-repo MJML + variables manifest.** Templates as files in `templates/email/<slug>.mjml` + `templates/email/<slug>.json` (variables declaration). MJML compiles to responsive HTML; provides a `curlew-backend dev email-preview <slug>` renderer for local review. SendGrid's UI-managed templates drift silently and resist code review — files in the repo do not.

**CI flow:** on tagged release, a CI job uploads templates to SendGrid, captures the resulting template ID, writes the mapping to `SENDGRID__TEMPLATES__<SLUG>` in the secrets store. SendGrid as the *delivery* mechanism without ceding *authoring* to it.

**M14 template inventory (6 templates — only those an M14 slice actually sends):**

| Slug | Trigger | Variables |
|---|---|---|
| `email_verification` | Signup; "verify your email" link | `first_name`, `verification_url` |
| `auth_device_code` | Device-code login (when web fallback used) | `user_code`, `verification_url`, `expires_in_minutes` |
| `billing_receipt` | `invoice.payment_succeeded` webhook | `first_name`, `billing_period`, `amount_total`, `invoice_url` |
| `billing_payment_failed` | `invoice.payment_failed` webhook | `first_name`, `amount_total`, `update_payment_url`, `attempt_count` |
| `billing_subscription_canceled` | `customer.subscription.deleted` webhook | `first_name`, `tier`, `effective_date` |
| `account_security_alert` | Refresh-token reuse detected → family revoked | `first_name`, `event_time`, `event_ip`, `relogin_url` |

The 6 templates fully exercise the SendGrid pipeline: dynamic-template variables (all 6), per-template manifest validation (all 6), CI upload job (all 6), `email-preview` dev renderer (all 6). Adding more templates later is content-only — the infrastructure does not change.

**Templates explicitly NOT in M14:** `password_reset` (M16 sender — ships with M16's password-reset endpoint slice) and `trial_expiring` (M16 sender — ships with M16's trial cron slice). Pulling them forward to M14 was the inverse of the deferred-hardening pattern: padding M14 with M16 work that no M14 slice needs. Per the no-phased-shipping rule, work belongs to the milestone whose slices use it.

**Per-template manifest** (the JSON sidecar):

```json
{
  "slug": "billing_receipt",
  "subject": "Your Curlew receipt for {{billing_period}}",
  "variables": {
    "first_name":      "string",
    "billing_period":  "string (e.g. 'May 2026')",
    "amount_total":    "string (e.g. '$49.00')",
    "invoice_url":     "string (HTTPS URL to Stripe-hosted invoice)"
  },
  "test_data": {
    "first_name": "Alex",
    "billing_period": "May 2026",
    "amount_total": "$49.00",
    "invoice_url": "https://invoice.stripe.com/i/test_..."
  }
}
```

**Sending pattern:**
- Backend never composes HTML inline. Calls `ISmtpSender.SendTemplateAsync(to, slug, variables)`.
- `SendGridSmtpSender` resolves `slug` → SendGrid template ID via env config, posts dynamic-template payload.
- `EmailQueueProcessor` consumes from `System.Threading.Channels` (per spec line 8129 — no change there).

**Security: template injection prevention.**
- All variables flow through SendGrid's Handlebars escaping (default-on).
- At send time, reject any variable name NOT in the template's manifest. Caller cannot inject extra params. Defends against the bug class where a controller hands user-controlled keys directly into the template-data dict.

**Out of M14 scope** (explicitly): plain-text alternatives, internationalization, per-tier branding. None of these are needed for any M14 slice's email-sending behaviour. Add when a future milestone has a slice that requires them.

---

## Decisions resolved (ready to draft v4.2)

All eleven load-bearing decisions are resolved (decisions 1–10 settled at the v4.2 design pass on 2026-05-03; decision 11 settled later the same day after Gap 12 was re-examined in light of the no-public-launch-until-M19-complete commitment). Each is now a concrete commitment that the v4.2 spec sub-sections (or, for #11, a v4.2.1 GitHub-side supplement) will encode. Sign-off summary:

| # | Decision | Resolution | Why it matters |
|---|---|---|---|
| 1 | JWT algorithm | **ES256** (ECDSA P-256, RFC 6979 deterministic). Final — EdDSA not a planned upgrade path | Smaller tokens (~0.9 KB vs ~1.4 KB), faster signing, modern default for greenfield 2026 systems. Universally supported including the chosen Google KMS HSM tier |
| 2 | KMS choice for SaaS | **Google Cloud KMS, HSM tier**, single GCP account, separate signing key per product (4 SaaS planned). `FileKeyProvider` for `deploy/self-hosted/`. | ~$12/month for 12 active key versions across 4 products; $0 for ~25 months on $300 new-account credit. FIPS 140-2 Level 3. Hosting-portable (callable from any cloud) |
| 3 | Signing-key rotation cadence | **90 days** active rotation, **60-day verification window** for old keys | Security-conservative cadence; verification window exceeds the longest in-flight JWT (License JWT signed just before rotation: 30 days valid + 14-day grace = 44 days) with comfortable margin. Refresh tokens are opaque and do not depend on this window |
| 4 | License JWT lifetime + grace | **30 days valid + 14-day grace** | Decouples normal CLI operation from backend uptime; ~6-week worst-case outage tolerance |
| 5 | Access token lifetime | **1 hour** | vs 15 min — bursty CLI usage; matches `gcloud`-class tools; backend revocation latency acceptable |
| 6 | Refresh-token lifetimes | **90-day sliding / 365-day absolute** (loose profile). CLI MUST hold a single-flight `flock` around refresh calls to prevent concurrent-invocation races triggering family-revocation | Minimises re-auth friction; matches `gcloud`/`gh auth` posture; annual forced re-login as hygiene measure for abandoned-laptop cases |
| 7 | CLI refresh-token storage | **Hybrid: OS keychain with encrypted-file fallback (`gh` pattern)** | Handles headless/CI/Docker correctly; doing it right once vs revisiting later |
| 8 | Login flow | **Device-code grant (RFC 8628)** with browser auto-open when interactive TTY + default browser detected (matches `gh auth login`). PKCE+callback explicitly NOT supported | Universal context coverage (works in CI, Docker, SSH, behind firewalls); cross-device auth (SSH session can authenticate via phone browser); single flow to maintain |
| 9 | stripe-mock in CI | **Add to backend CI gate.** Integration tests tagged `[Trait("Category", "stripe-integration")]`, conditionally run under existing `./scripts/ci-local.sh` backend gate | ~10–20s overhead per backend CI run; catches regressions in `StripeGateway.cs` (highest-risk integration code in M14) at PR-review time vs release-time |
| 10 | Email template scope | **6 templates** — only those an M14 slice actually sends. `password_reset` and `trial_expiring` ship with their M16 senders | Cleaner milestone boundary; no dead code in M14; SendGrid pipeline fully exercised by 6 templates (adding more is content-only) |
| 11 | Gap 12 scope (PR-check posting *to* GitHub) | **Fold into M14 (Option A).** GitHub Checks API integration moves from M16 into M14. ~4–6 additional slices. GitHub-only — GitLab parity is explicitly NOT in M14. Requires a v4.2.1 spec supplement (GitHub App registration, JWT-signed installation-token flow, Checks API payload mapping, `installations` table, optional inbound webhook for re-run events) before `/backlog M14` can run | The CLI surface (`--report-upload`, `--pr`, `--repo`) was already shipped in M4-007 and is visible in `--help`. Splitting CLI from backend→GitHub leaves an "appears-to-work-but-silently-doesn't" feature. Even though the no-public-launch-until-M19-complete commitment would have made Option C ("ship inert in M14, complete in M16") technically defensible, accepting the M14 expansion avoids setting a precedent for that pattern |

All decisions resolved 2026-05-03. The seven v4.2 sub-sections above are the authoritative source for the v4.2 edition of `docs/SPECIFICATION.md`, drafted in this order: token model + claim shapes (§0/§1) → signing keys (§2) → refresh tokens (§3) → CLI/backend contract (§4) → webhook idempotency (§5) → Stripe test strategy (§6) → SendGrid templates (§7). Token model and claim shapes are drafted first because they are the most load-bearing — every other section references the artifacts they define. Decision #11 (Gap 12) requires a separate v4.2.1 supplement covering the GitHub-side architecture; that supplement is the only design artifact still missing before `/backlog M14` can run.

A fresh session can use this file as a self-contained design reference. The "Resolution" column captures the final commitment for each decision; the body sections capture the rationale and implementation detail. Do not reopen settled decisions without an explicit instruction to do so — the trade-offs were worked through in the discussion that produced this file and are now load-bearing for downstream slices.

---

## End-to-end verification

**Verified end-to-end on 2026-05-06** (M14-021 convergence slice):

The full M14 revenue loop was verified by the M14-021 convergence slice:
- `curlew run testdata/m14/e2e-collection.yaml --report-upload --org acme --pr 7 --repo acme/api` exits 0 and prints `check-run posted; status=success`
- The backend persists the result, posts the check-run to the github-mock sidecar via the M14-018 `CheckRunPoster`, and records `posted_at` on the `pr_checks` row
- The `/org/acme/integrations/github` web view surfaces `posted_at` and `check_run_id`
- A replayed `invoice.payment_succeeded` Stripe event triggers the M14-013 `StripeInvoiceHandler`, which enqueues a `billing_receipt` `EmailMessage`; the `InMemoryRecentlySentEmailLog` records the send and the `/internal/test/email-audit` endpoint confirms the entry
- `./scripts/ci-local.sh --down` tears down the test stack idempotently and exits 0
- All four M14 cluster heads (M14-006, M14-013, M14-015, M14-018) verified to interoperate
