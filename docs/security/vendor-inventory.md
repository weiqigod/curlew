# Vendor Inventory

**Owner:** Engineering
**Version:** 1.0
**Effective date:** 2026-05-19
**Next review:** 2027-05-19 (annual, aligned with policy review cadence)

This inventory enumerates every external service vendor that processes ApiTool
customer data or holds infrastructure access. Each row records data shared,
retention, breach-notification SLA, vendor SOC 2 status, and DPA location.

Cross-references: [Information Security Policy](info-sec-policy.md) §
Vendor Management; [Data Classification Matrix](data-classification-matrix.md);
[Customer Data Flow Diagram](data-flow-customer.md);
[Internal Data Flow Diagram](data-flow-internal.md).

## Vendor Table

| Vendor | Service | Data shared | Retention | Breach notification SLA | Vendor SOC 2 status | DPA on file |
| --- | --- | --- | --- | --- | --- | --- |
| Stripe | Payment processing (billing, subscriptions, invoices) | Customer email, billing name, payment-method tokens (raw card data never leaves Stripe) | Retained per Stripe PCI-DSS obligations (7 years for tax records, indefinite for fraud-defence) | 72 h (Stripe Data Processing Agreement §6.3) | SOC 2 Type II (publicly attested) | `legal@apitool.dev` — DPA executed 2025-09-01 [^1] |
| SendGrid | Transactional email delivery (verification, password reset, billing receipts, trial-expiry, account-deletion notices) | Recipient email, transactional template variables (per-template allowlist) | 30 days for delivery logs; message bodies not retained beyond send | 72 h (Twilio Data Protection Addendum §8) | SOC 2 Type II (Twilio attestation) | `legal@apitool.dev` — DPA executed 2025-09-01 [^1] |
| Google KMS (Google Cloud KMS) | Key-encryption-key (KEK) custody for `team_vaults.template_jsonb` and `schedules.env_vars` envelope encryption (M18-009); signing-key wrap for `signing_keys.private_key` (v3-10) | Wrapped data-encryption keys (DEKs); never sees plaintext customer secrets or DEK plaintext | Indefinite (key material persists until revocation) | 24 h for confirmed key-material breach (Google Cloud SLA addendum) | SOC 2 Type II (Google Cloud Platform attestation) | `legal@apitool.dev` — Google Cloud DPA accepted via console 2025-09-01 [^1] |
| AWS | Infrastructure hosting (RDS Postgres for all backend tables; S3 for export bundles, audit log archives, backup snapshots; SES as backup transactional email path) | All persisted backend tables; export bundles; audit log archives | Backups: 90 days; S3 versioning 30 days; RDS automated snapshots 7 days | 72 h (AWS Customer Agreement §11.4) | SOC 2 Type II (AWS Security Hub) | `legal@apitool.dev` — AWS DPA and BAA executed 2025-09-01 [^1] |
| GitHub Apps | GitHub PR-checks integration (M16); CI artefact distribution (GitHub Releases for CLI binaries) | Org-controlled PAT (encrypted at-rest); commit SHAs; PR status payloads | PAT retained until uninstall; check-run records 90 days | 72 h (GitHub Customer DPA §8) | SOC 2 Type II (GitHub Trust attestation) | `legal@apitool.dev` — GitHub Customer DPA executed 2025-09-01 [^1] |
| GitLab | GitLab PR-checks integration (M16-014/M16-015/M16-016) | Project-scoped PAT (encrypted at-rest); commit SHAs; pipeline status payloads | PAT retained until uninstall; check-run records 90 days | 72 h (GitLab DPA §6) | SOC 2 Type II (GitLab Trust Center) | `legal@apitool.dev` — GitLab DPA executed 2025-09-01 [^1] |

[^1]: Executed DPA copies are filed in the legal mailbox (``legal@apitool.dev``)
and mirrored in the `s3://apitool-legal-vault/dpa-<vendor>/` bucket
per Information Security Policy § Vendor Management. An auditor following
this column should request access via the legal-mailbox principal.

## Vendor Onboarding

A new vendor processing customer data is onboarded by adding a row to this
inventory, executing a DPA, and obtaining a Vendor SOC 2 attestation or
documenting compensating controls. The Information Security Policy § Vendor
Management owns the procedure; this inventory is the per-vendor record.

## Review Cadence

Annual, aligned with the Information Security Policy review (2027-05-19).
Out-of-cycle review is triggered by:

- A new vendor processing customer data being added.
- An incumbent vendor's SOC 2 attestation lapsing.
- A confirmed breach notification from an incumbent vendor.
