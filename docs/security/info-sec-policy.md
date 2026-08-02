# Information Security Policy

**Owner:** Engineering  
**Version:** 1.0  
**Effective date:** 2026-05-19  
**Next review date:** 2027-05-19

---

## Scope

This policy applies to:

- All ApiTool engineering personnel, contractors, and third-party service providers
  with access to ApiTool systems or customer data.
- The ApiTool SaaS backend (C# .NET, hosted on AWS).
- The ApiTool CLI distribution chain (Go binary; distributed via GitHub Releases and
  Homebrew tap).
- Development, staging, and production environments.

Vendor obligations are captured in the Vendor Inventory
(`docs/security/vendor-inventory.md`, forthcoming M18-011).

---

## Acceptable Use

- Production customer data must never be copied to development or staging environments
  without explicit approval from an Owner-level principal.
- Customer secrets (API keys, tokens, `team_vaults` contents, `schedules.env_vars`
  blobs) must not be logged, transmitted in clear text, or stored outside the
  designated encrypted fields (M18-009 envelope encryption).
- Developer laptops must have full-disk encryption enabled and a lock screen
  activating within 5 minutes of inactivity.
- Multi-factor authentication (MFA) is required for all SSO accounts used to access
  AWS, GitHub, and the ApiTool Admin panel.
- Personal accounts must not be used for work-related API access; all integrations
  use machine service tokens scoped to the minimum required permissions.

---

## Access Control

Access to ApiTool systems and customer data follows the principle of least privilege:

- **RBAC model:** The backend enforces a two-layer permission model: built-in
  organisation roles (`Owner`, `Admin`, `Member`) plus `CustomRole` templates. The
  Security Auditor custom-role template grants `audit_log.view` only — permitting
  review of audit logs without `audit_log.export` (bulk extraction) or any write
  permission. This separation of duties is documented in
  `docs/security/access-review-policy.md` and in the SPECIFICATION.md "Security
  Auditor Custom-Role Template" section.
- **Service tokens:** scoped to a single organisation; rotated on personnel change.
- **AWS IAM:** roles follow least-privilege; no wildcard resource ARNs in production
  policies.
- **Access reviews:** conducted quarterly per `docs/security/access-review-policy.md`.
  Deprovisioning tickets must be resolved within 5 business days of a personnel
  change.

---

## Change Management

- All code changes require a pull-request review by at least one engineer other than
  the author before merging.
- The CI gate (`scripts/ci-local.sh`) must pass before merge: Go build, tests, lint,
  .NET tests, and smoke tests are all mandatory.
- Production deployments are made from tagged releases only; no ad-hoc pushes to the
  production environment.
- Post-deploy verification (smoke test against the production health endpoint) is
  required within 15 minutes of deployment completion.
- Rollback is executed by redeploying the previous tagged release; the on-call
  engineer is notified within 5 minutes if automated health checks fail.

---

## Vendor Management

Third-party vendors with access to customer data or infrastructure are enumerated in
the Vendor Inventory (`docs/security/vendor-inventory.md`, forthcoming M18-011).
Selection criteria:

- Vendor must hold a current SOC 2 Type II report (or equivalent ISO 27001
  certification).
- Data-processing agreements (DPAs) must be signed before any personal data is
  shared.
- Breach-notification SLA from vendor to ApiTool: within 24 hours of confirmed
  breach.

See also `docs/security/incident-response-runbook.md` for vendor-breach escalation
procedures.

---

## Incident Response

Security and availability incidents are classified by severity and managed by the
on-call rota. Full procedures, severity taxonomy (SEV-1 through SEV-4), comms
templates, and post-incident review cadence are documented in
`docs/security/incident-response-runbook.md`.

Key commitments:

- SEV-1 (production down, active breach): page on-call within 1 minute; customer
  notification within 1 hour.
- Post-incident review: within 5 business days of resolution.

---

## Business Continuity

- **RPO:** 1 hour (AWS RDS automated snapshots taken hourly).
- **RTO:** 4 hours (documented in the DR runbook, forthcoming M18-011).
- **KMS key rotation:** Google KMS signing keys are rotated annually; AWS KMS
  data-encryption keys are set to auto-rotate annually.
- **Disaster-recovery test:** conducted annually; results logged to
  `docs/security/dr-test-YYYY.md` (forthcoming).
- **Database backups:** retained for 35 days in AWS S3 with object lock (WORM).

---

## Review Cadence

This policy is reviewed annually. The next scheduled review date is **2027-05-19**.
Changes require approval from at least one Owner-level principal and are committed
to the repository under the M18 compliance track.
