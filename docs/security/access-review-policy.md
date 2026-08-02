# Access Review Policy

**Owner:** Engineering  
**Version:** 1.0  
**Effective date:** 2026-05-19  
**Next review date:** 2027-05-19

---

## Cadence

**Quarterly access review** — conducted on the first business day of each calendar
quarter (January, April, July, October). Reviews must be completed and the signed-off
CSV committed to the repository within 5 business days of the start date.

---

## Reviewer Role

Access reviews are performed by an **Owner** or **Admin** of each organisation, as
defined by the ApiTool RBAC model (see SPECIFICATION.md "RBAC Matrix"). The reviewer
must not be the sole Owner of the organisation being reviewed; where only one Owner
exists, the review is escalated to an ApiTool platform administrator.

---

## Reviewed Surface

Each quarterly review must cover the following surfaces for every active organisation:

1. **All `organization_members` rows** — confirm each member's role and active status;
   flag accounts that have not signed in within 90 days for de-provisioning
   consideration.
2. **All `organization_custom_roles` rows and their permission sets** — confirm that
   no custom role has accumulated permissions beyond its stated intent.
3. **The Security Auditor role population** — the built-in **Security Auditor**
   custom-role template grants `audit_log.view` only (no `audit_log.export`, no write
   permissions). Review confirms that every principal assigned this role is an
   authorised auditor and that the template's permission set has not drifted from its
   declared scope (separation-of-duties per v4-3).
4. **Service tokens** — confirm each active service token is associated with a
   known integration and has not exceeded its intended scope.

---

## Evidence Artefact

The output of each quarterly review is a signed-off CSV committed under:

```text
docs/security/access-reviews/YYYY-Q.csv
```

Example path: `docs/security/access-reviews/2026-Q3.csv`

CSV schema:

```text
org_slug,user_email,role,custom_role_template,reviewer,reviewed_at,decision
```

`decision` values: `approved`, `deprovisioned`, `escalated`.

Rows with `deprovisioned` or `escalated` decisions must have a corresponding ticket
reference in the `notes` column (optional seventh column). Tickets must be resolved
within 5 business days of the review date.

---

## Separation of Duties

An auditor assigned the **Security Auditor** custom-role template holds only
`audit_log.view`. This grants read access to the organisation's audit log but
explicitly excludes:

- `audit_log.export` — bulk extraction of the audit log to an external destination.
- Any write, invite, or billing permission.

This design ensures that an auditor performing access reviews can inspect the audit
log for suspicious access events without being able to modify membership records,
export data in bulk, or alter billing state. The separation-of-duties decision is
documented in SPECIFICATION.md v4-3.

---

## Procedure

1. Export the current membership and custom-role list from the ApiTool Admin panel
   (or via the management API) as a CSV.
2. Cross-reference against the organisation's HR deactivation list (provided by the
   Owner) for any personnel changes since the last review.
3. Review each row against the reviewed surface criteria above.
4. For any `deprovisioned` row: revoke access in the ApiTool Admin panel and record
   the ticket reference in the CSV.
5. For any `escalated` row: open a ticket and notify the platform administrator.
6. Sign off the CSV by adding your name and the review date in the `reviewer` and
   `reviewed_at` columns.
7. Commit the signed CSV to `docs/security/access-reviews/YYYY-Q.csv` via a pull
   request; a second reviewer approves the PR.

---

## Review Cadence

This policy is reviewed annually. The next scheduled review date is **2027-05-19**.
