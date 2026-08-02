# Compliance

ApiTool's compliance artefacts. Each document below is a load-bearing input for the
SOC 2 Type II / ISO 27001 audit work tracked in M18 `compliance_artifacts` (v4-14).
The audit engagement itself is out of scope per v4-14; M18 produces the evidence
the audit will consume.

## Document Inventory

### Policies (M18-010)

- [Information Security Policy](security/info-sec-policy.md) — scope, acceptable use,
  access control, change management, vendor management, incident response overview,
  business continuity, review cadence.
- [Access Review Policy](security/access-review-policy.md) — quarterly cadence,
  Owner/Admin reviewer, all OrganizationMember rows + all CustomRole assignments +
  Security Auditor role population reviewed; signed-off CSV evidence committed under
  `docs/security/access-reviews/YYYY-Q.csv`.
- [Incident Response Runbook](security/incident-response-runbook.md) — SEV-1/2/3/4
  taxonomy, on-call rota expectation, customer/internal comms templates,
  post-incident review within 5 business days, sample incident timeline.

### Data Classification and Inventory (M18-003, M18-010)

- [Data Classification Matrix](security/data-classification-matrix.md) — four-tier
  Public/Internal/Confidential/Restricted mapping of every table in the schema
  appendix with rationale and storage encryption posture (M18-009 envelope
  encryption referenced for `team_vaults` and `schedules.env_vars`).
- [GDPR Data Inventory](security/data-inventory.md) — 14-table per-user export and
  deletion decision matrix backing M18-004/-005/-006.

### Vendor and Data-Flow Artefacts (M18-011)

- [Vendor Inventory](security/vendor-inventory.md) — Stripe, SendGrid, Google
  Cloud KMS, AWS, GitHub Apps, GitLab. Seven-column table: Vendor | Service |
  Data shared | Retention | Breach notification SLA | Vendor SOC 2 status |
  DPA on file.
- [Customer Data Flow Diagram](security/data-flow-customer.md) — every customer-PII
  path: CLI to backend (telemetry); web portal to backend (registration,
  dashboard, account/data); backend to SendGrid (transactional email); backend to
  Stripe (billing).
- [Internal Data Flow Diagram](security/data-flow-internal.md) — every
  internal-secrets path: backend and KMS (signing keys + DEK wrapping per
  M18-009); team-vault envelope encryption; `schedules.env_vars` envelope
  encryption; refresh/access-token storage (DB hash for refresh tokens + CLI keychain/encrypted files).

### Pen-test Cadence (v4-15)

**Annual.** First engagement: [Cure53, 2026-Q2](security/pentest-2026-Q2.md)
(orchestrated as part of M18-011). Remediation log lands as
`docs/security/pentest-YYYY-Q.md` per the annual cadence.

**Next engagement target window:** 2027-Q2. Procurement trigger: 2027-Q1
(NDA + scope statement re-signed at least 30 days before the engagement
window opens).

**Budget-line owner:** Engineering — Security pen-test annual budget line.

## Cross-Controls

- **Signing-key plaintext fallback (v4-13)** — SaaS production builds run
  `scripts/check-signing-keys.sh` as a CI lint; self-hosted may opt out via
  `APITEST_SIGNING_KEY_MODE=file`. Documented here as a build-time control.

## Review Cadence

The Information Security Policy, Access Review Policy, and Incident Response
Runbook are reviewed annually; the Data Classification Matrix is reviewed each
time the schema appendix changes. See the "Review Cadence" section in each
document for the next-review date.
