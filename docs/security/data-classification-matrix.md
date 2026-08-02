# Data Classification Matrix

**Owner:** Engineering  
**Version:** 1.0  
**Effective date:** 2026-05-19  
**Next review:** at the next schema-appendix change

This matrix maps every table in the ApiTool schema appendix (SPECIFICATION.md) to one
of four classification tiers, records the encryption-at-rest posture for each table,
and provides a rationale. The matrix is the broader superset of
[`docs/security/data-inventory.md`](data-inventory.md): every table in the GDPR data
inventory also appears here, but the matrix additionally covers non-user-attributable
operational tables.

---

## Classification Tiers

### Public

Data that is intentionally externally visible. Loss or disclosure causes no harm.

Examples: OpenAPI specification, public documentation, marketing copy, CLI help text.

Handling: no access controls required beyond standard repository permissions.

### Internal

Engineering-team-only operational data with no individual user attribution. Not
customer-PII, but not intended for external publication.

Examples: webhook event metadata, aggregated telemetry, infrastructure monitoring
data.

Handling: access restricted to ApiTool engineering staff; no customer notification
required on loss unless the data allows inference of customer activity.

### Confidential

Customer-attributable business records that are not directly identifiable to a
natural person but are commercially sensitive and subject to contractual obligations.

Examples: audit logs, organisation settings, custom role definitions, schedules,
GitHub installation metadata.

Handling: access restricted to personnel with a business need; encryption in transit
(TLS 1.2+) required; incident notification to affected organisations within 24 hours
of a confirmed breach.

### Restricted

Directly personally-identifiable information, authentication credentials, secrets, or
payment data. The most sensitive tier; subject to GDPR data-subject rights and strict
access controls.

Examples: `users.email`, `users.password_hash`, `signing_keys.private_key`,
`team_vaults` encrypted templates (M18-009), `schedules.env_vars` encrypted blobs
(M18-009), Stripe payment-method tokens.

Handling: encrypted at rest (see Encryption-at-Rest Posture section below); access
limited to the minimum number of principals; customer notification required within
1 hour of a confirmed SEV-1 breach; GDPR rights (export + deletion) enforced per
`docs/security/data-inventory.md`.

---

## Per-Table Classification

Columns: Table | Classification | Encryption-at-Rest | Rationale

### Auth Tables

| Table | Classification | Encryption-at-Rest | Rationale |
| --- | --- | --- | --- |
| `auth_audit_log` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Audit trail of authentication events; not directly user-attributable but commercially sensitive. |
| `deletion_reauth_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); `token_hash` is hashed | Short-lived re-auth tokens gating account deletion (M18-005); contain user identity linkage. |
| `device_authorization_codes` | Restricted | TLS in transit; at-rest AES-256 (RDS); code hashed | OAuth device-flow codes; short-lived but linked to a user identity. |
| `devices` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Device registration records; linked to user but not primary PII. |
| `email_verification_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); `token_hash` hashed | Short-lived email-verification artefacts linked to a specific user email. |
| `magic_link_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); token hashed | Passwordless login tokens; direct authentication credential. |
| `oauth_state_tokens` | Confidential | TLS in transit; at-rest AES-256 (RDS) | CSRF state for OAuth flows; short-lived, no PII stored. |
| `password_reset_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); `token_hash` hashed | Short-lived reset artefacts; function as temporary auth credentials. |
| `refresh_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); `token_hash` hashed | Long-lived auth tokens; direct authentication credential linked to a user. |
| `service_tokens` | Restricted | TLS in transit; at-rest AES-256 (RDS); token hashed | Machine authentication credentials for integrations. |
| `sessions` | Restricted | TLS in transit; at-rest AES-256 (RDS); session ID hashed | Active user session state; direct authentication artefact. |
| `signing_keys` | Restricted | KMS-wrapped per-row (v3-10); CI lint enforced by `scripts/check-signing-keys.sh` | Private signing keys for JWT issuance; most sensitive auth secret. |
| `user_audit_log` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Per-user audit trail; commercially sensitive but not primary PII. |
| `users` | Restricted | TLS in transit; at-rest AES-256 (RDS); `password_hash` argon2id | Primary identity record; contains email, password hash, and profile data. |

### Billing Tables

| Table | Classification | Encryption-at-Rest | Rationale |
| --- | --- | --- | --- |
| `invoices` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Customer billing records; commercially sensitive but payment tokens held by Stripe. |
| `payment_methods` | Restricted | TLS in transit; at-rest AES-256 (RDS); raw card data never stored — Stripe tokenised | Payment-method references; Stripe tokens are PCI-DSS Restricted. |
| `stripe_webhook_events` | Internal | TLS in transit; at-rest AES-256 (RDS) | Idempotency log of Stripe event payloads; no raw card data. |
| `subscription_audit_log` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Audit trail of subscription state changes; commercially sensitive. |
| `subscriptions` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Subscription plan and status per organisation; commercially sensitive. |

### Integration Tables

| Table | Classification | Encryption-at-Rest | Rationale |
| --- | --- | --- | --- |
| `github_installations` | Confidential | TLS in transit; at-rest AES-256 (RDS); `encrypted_pat` AES-256-GCM | GitHub App installation metadata; `encrypted_pat` is a customer-controlled secret. |
| `github_webhook_events` | Internal | TLS in transit; at-rest AES-256 (RDS) | Idempotency log of GitHub webhook payloads; no PII. |
| `gitlab_installations` | Confidential | TLS in transit; at-rest AES-256 (RDS); `encrypted_pat` AES-256-GCM | GitLab App installation metadata; `encrypted_pat` is a customer-controlled secret. |
| `gitlab_webhook_events` | Internal | TLS in transit; at-rest AES-256 (RDS) | Idempotency log of GitLab webhook payloads; no PII. |
| `pr_checks` | Internal | TLS in transit; at-rest AES-256 (RDS) | CI check run records; no PII; operational metadata only. |

### Workspace Tables

| Table | Classification | Encryption-at-Rest | Rationale |
| --- | --- | --- | --- |
| `coordinator_jobs` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Async job headers; `CreatedBy` links to a user but the job payload is org-scoped. |
| `license_validations` | Internal | TLS in transit; at-rest AES-256 (RDS) | CLI license check records; no PII; operational metadata only. |
| `notification_rules` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Org-scoped notification config; `CreatedBy` links to a user but payload is non-PII. |
| `organization_audit_log` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Org-level audit trail; `actor_id` / `actor_email` anonymised on user deletion (M18-006). |
| `organization_custom_roles` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Custom permission-set definitions; org-scoped business records. |
| `organization_invitations` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Pending invitations including invitee email; commercially sensitive. |
| `organization_members` | Restricted | TLS in transit; at-rest AES-256 (RDS) | Direct user-to-organisation binding; subject to GDPR data-subject rights. |
| `organizations` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Organisation profile and settings; commercially sensitive. |
| `scheduled_runs` | Confidential | TLS in transit; at-rest AES-256 (RDS) | Execution records for scheduled API tests; org-scoped operational data. |
| `schedules` | Confidential | TLS in transit; at-rest AES-256 (RDS); `env_vars` AES-256-GCM envelope (M18-009) | Cron schedule definitions; `schedules.env_vars` encrypted blob holds customer secrets (M18-009 envelope encryption). |
| `team_vaults` | Restricted | TLS in transit; at-rest AES-256 (RDS); `template_jsonb` AES-256-GCM envelope (M18-009) | Customer secret templates; `team_vaults.template_jsonb` holds customer API keys and tokens in an envelope-encrypted blob (M18-009; `ITeamVaultKeyProvider`). |
| `trials` | Internal | TLS in transit; at-rest AES-256 (RDS) | Trial period tracking; no PII beyond organisation linkage. |

---

## Encryption-at-Rest Posture

The following Restricted-tier fields use column-level or row-level encryption beyond
the baseline RDS AES-256 volume encryption:

| Table | Column | Mechanism | Introduced |
| --- | --- | --- | --- |
| `team_vaults` | `template_jsonb` | AES-256-GCM envelope encryption; data key wrapped by `ITeamVaultKeyProvider` (Google KMS) | M18-009 |
| `schedules` | `env_vars` | AES-256-GCM envelope encryption; data key wrapped by `IScheduleEnvKeyProvider` (Google KMS) | M18-009 |
| `signing_keys` | `private_key` | KMS-wrapped per-row key; plaintext never written to database; CI lint via `scripts/check-signing-keys.sh` | v3-10 |
| `github_installations` | `encrypted_pat` | AES-256-GCM; key managed by application secrets | Existing |
| `gitlab_installations` | `encrypted_pat` | AES-256-GCM; key managed by application secrets | Existing |
| `users` | `password_hash` | argon2id (not reversible encryption — one-way hash function) | Existing |

The term **envelope encryption** (used above for `team_vaults.template_jsonb` and
`schedules.env_vars`) refers to the pattern where each row's plaintext is encrypted
with a unique data-encryption key (DEK), which is itself encrypted by a key-encryption
key (KEK) managed in Google KMS. Neither the DEK plaintext nor the KEK ever touches
application memory after wrap/unwrap. Full implementation detail is in M18-009.

---

## Consistency with GDPR Data Inventory

Every table in [`docs/security/data-inventory.md`](data-inventory.md) appears in this
matrix. The `DataClassificationMatrixDocTests.Every_inventory_table_appears_in_matrix`
xUnit test enforces this invariant in CI (inventory ⊆ matrix direction).

The reverse is not required: many tables in this matrix (billing, integration,
workspace operational tables) carry no direct user attribution and therefore have no
GDPR disposition. Those tables are covered by `[GdprTable(ExcludedFromBoth)]` in code
or are omitted from the GDPR inventory by design.

---

## Review Cadence

This matrix is reviewed each time the schema appendix (SPECIFICATION.md) changes.
Next-review trigger: the next migration that adds or removes a table. No calendar
date is set; the `DataClassificationMatrixDocTests.Matrix_lists_all_36_schema_appendix_tables`
CI test will fail if a new table is added without a corresponding matrix row.
