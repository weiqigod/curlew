# GDPR Data Inventory

This document is the canonical decision matrix for GDPR data subject rights across the 14
user-attributable tables in the Curlew backend. Each row records whether the table's rows
appear in the per-user export (`GET /api/v1/users/me/export`, M18-004) and whether they are
hard-deleted, anonymised, or excluded on user deletion (M18-005, M18-006). Refs:
`docs/SPECIFICATION.md` v4-4 + v4-5 decision; `docs/M18_INVESTIGATION.md` Gap 18.

The matrix below is mirrored in code via `[GdprIncluded]` / `[GdprAnonymise]` property
attributes on the EF entities and a class-level `[GdprTable(ExcludedFromBoth)]` marker
for tables enumerated in v4-4 that carry no user-attribution column. The
`GdprAttributeScanner` produces a `GdprBundleManifest` consumed by both the export
builder (M18-004) and the anonymiser (M18-006); the
`GdprInventoryDocConsistencyTests` test asserts that every row below matches the
scanner output disposition exactly. Adding a new user-attributable table without
GDPR attributes will fail `GdprInventoryCoverageTests` in CI.

## User-Facing Export UI

Authenticated users can request and download their own data bundle at `/account/data` in the
web application (M18-004). Clicking **Request data export** calls
`POST /api/v1/users/me/export-requests` (one request per 24-hour window); the page polls
`GET /api/v1/users/me/export-requests/{id}` every 5 seconds until the status reaches a
terminal state (`ready`, `failed`, or `expired`). When ready, a time-limited download link
is displayed. The bundle is a JSON file containing one key per InExport table (see the
`yes` rows below), with sensitive columns (`password_hash`, `token_hash`) omitted.

| Table | Owner | InExport | InDeletion | Retention | Notes |
| --- | --- | --- | --- | --- | --- |
| `coordinator_jobs` | coord | no | anonymise creator | per-org retention | ExcludedFromExportAnonymisedInDeletion — org-scoped job header; `CreatedBy` anonymised. |
| `deletion_reauth_tokens` | auth | yes | hard | 5m | InExportInDeletionHard — short-lived re-auth tokens gating deletion requests (M18-005); hard-deleted with the user. |
| `email_verification_tokens` | auth | yes | hard | 24h | InExportInDeletionHard — short-lived verification artefacts. |
| `github_installations` | git | no | excluded | until uninstalled | ExcludedFromBoth — org-scoped, no user-attribution column. |
| `gitlab_installations` | git | no | excluded | until uninstalled | ExcludedFromBoth — org-scoped, no user-attribution column. |
| `notification_rules` | notify | no | anonymise creator | per-org retention | ExcludedFromExportAnonymisedInDeletion — org-scoped business record; `CreatedBy` anonymised. |
| `organization_audit_log` | audit | yes | anonymise | per-org retention | InExportInDeletionAnonymise — `ActorId` → NULL, `ActorEmail` → `deleted-user-<token>` to preserve audit integrity (v4-6). |
| `organization_custom_roles` | rbac | no | anonymise creator | per-org retention | ExcludedFromExportAnonymisedInDeletion — org-scoped role; `CreatedBy` anonymised. |
| `organization_members` | org | yes | hard | membership | InExportInDeletionHard — user's membership rows; `InvitedBy` anonymised on the inviter's deletion. |
| `password_reset_tokens` | auth | yes | hard | 30m | InExportInDeletionHard — short-lived reset artefacts. |
| `refresh_tokens` | auth | yes | hard | session-bound | InExportInDeletionHard — auth tokens belong to the user. |
| `schedules` | sched | no | anonymise creator | per-org retention | ExcludedFromExportAnonymisedInDeletion — org-scoped cron rule; `CreatedBy` anonymised. |
| `team_vaults` | vault | no | anonymise creator+updater | per-org retention | ExcludedFromExportAnonymisedInDeletion — org-scoped template; `CreatedBy` and `UpdatedBy` anonymised. |
| `users` | auth | yes | hard | until deleted | InExportInDeletionHard — data subject's profile row. |

## Anonymisation function (M18-006)

The `UserAnonymiser` concrete implementation finalises GDPR account deletions. Its
contract is **deterministic, irreversible, and per-(user, org)-scoped**.

**Token shape:** `deleted-user-{first8(sha256(user_id||org_id))}` — lowercase
hex, total length 21 chars. The same `(user_id, org_id)` pair always derives the
same token; different orgs derive different tokens; given just the token,
recovering the user_id is computationally infeasible. The `users.email` column
uses `(user_id, Guid.Empty)` as the org axis.

**What is hard-deleted:** rows in tables whose disposition is
`InExportInDeletionHard` attributed to the user (`refresh_tokens`,
`email_verification_tokens`, `password_reset_tokens`, `deletion_reauth_tokens`,
`organization_members` for the user).

**What is anonymised in place:** `organization_audit_log.actor_id` → NULL,
`organization_audit_log.actor_email` → per-(user, org) token; `*_by` columns on
`custom_roles`, `schedules`, `coordinator_jobs`, `team_vaults`,
`notification_rules` → NULL; `organization_members.invited_by` → NULL (for
inviter-axis on other members' rows).

**Audit-of-audit trail:** for each affected org, exactly one
`user.anonymised` audit row is emitted with `payload.anonymisation_token`
carrying the per-org token. The audit row's own `actor_id` is NULL and
`actor_email` is the token, so the audit row itself is anonymised at write-time.

**User row tombstone:** the `users` row survives with `anonymised_at` set,
`email` replaced by `deleted-user-{first8(sha256(user_id||0x00...))}`,
`password_hash` NULL, `email_verified` and `is_admin` cleared. The row is
retained for state-machine accountability; no PII remains.
