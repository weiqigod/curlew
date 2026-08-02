# Incident Response Runbook

**Owner:** Engineering  
**Version:** 1.0  
**Effective date:** 2026-05-19  
**Next review date:** 2027-05-19

---

## Severity Taxonomy

### SEV-1

**Production down, data loss risk, or active security breach.**

Conditions: complete service unavailability affecting all tenants; confirmed or
suspected data exfiltration; active exploitation of a security vulnerability; KMS
key compromise.

Response:

- Page on-call engineer immediately (target ack within 1 minute).
- Declare incident in the `#incidents` Slack channel; assign Incident Commander (IC).
- Customer-facing status page updated within 10 minutes of declaration.
- Engineering leadership notified within 15 minutes.
- Customer notification email sent within 1 hour.

### SEV-2

**Major degradation, single-tenant outage, or security vulnerability with no active exploit.**

Conditions: one or more organisations unable to use core features; a confirmed
vulnerability with CVSS ≥ 7.0 but no evidence of exploitation; payment processing
failures; data pipeline stall causing stale exports.

Response:

- Page on-call engineer (target ack within 5 minutes).
- Incident declared in `#incidents`; IC assigned.
- Status page updated within 30 minutes.
- Customer notification within 2 hours if external impact is confirmed.

### SEV-3

**Partial degradation or non-critical feature broken.**

Conditions: a feature affecting a subset of users or a non-critical integration is
broken; elevated error rates (5xx > 1 %) without tenant-wide impact; a scheduled
run fails but retry will recover.

Response:

- On-call engineer notified via PagerDuty (non-paging alert, escalates to page if
  unacked in 30 minutes).
- Ticket created and assigned; resolution targeted within 1 business day.
- No immediate customer notification unless the feature is customer-visible and the
  degradation is confirmed.

### SEV-4

**Cosmetic, monitoring alert false positive, or documentation drift.**

Conditions: UI cosmetic bug; a monitoring alert that does not reflect real impact;
an outdated doc or help-text string.

Response:

- Ticket created; triaged and assigned in the next sprint planning.
- No incident declaration or on-call page required.

---

## On-call Rota

- **Primary on-call:** rotates weekly among senior engineers; owns SEV-1 and SEV-2
  acks.
- **Secondary on-call:** rotates weekly; backs up the primary if unresponsive after
  2 minutes (SEV-1) or 10 minutes (SEV-2).
- **Escalation path:** Primary → Secondary → Engineering Lead → CTO.
- **PagerDuty (or equivalent):** schedules and escalation policies are defined in the
  on-call tooling; this runbook is the authoritative source for ack SLAs and
  escalation thresholds.
- **Handoff:** a brief written handoff note is posted in `#incidents` at each weekly
  rotation boundary covering any open SEV-3 or higher incidents.

---

## Customer-Facing Comms Template

Use this template for the initial customer notification email:

---

Subject: [Curlew] Service Incident — [Date]

Dear Curlew user,

We are writing to inform you of a service incident affecting [scope — e.g. "all
organisations" / "organisations using scheduled runs"].

**What happened:** [1–2 sentence factual description; no speculation on cause.]

**Impact:** [Describe what users could or could not do during the incident window.]

**Timeline:**

- [ISO 8601 time] — Incident detected.
- [ISO 8601 time] — Mitigation applied / service restored.

**What we are doing:** [Describe immediate remediation and any follow-up actions.]

We apologise for the disruption. If you have questions, please contact us at
<support@apitool.dev>.

— The Curlew team

---

**Status-page update template:**

> **[Investigating / Identified / Monitoring / Resolved]** — [1-sentence summary of
> current status]. [ETA for next update or resolution, if known.] Last updated:
> [ISO 8601 timestamp].

---

## Internal Comms Template

Post the following in `#incidents` immediately on declaration:

```text
🚨 INCIDENT DECLARED — SEV-[N]

Summary: [1-sentence description]
IC: @[handle]
Comms lead: @[handle]
Scribe: @[handle]
Status: [Investigating | Identified | Mitigating | Resolved]
Impact: [External / Internal / Unknown]
Next update: [time]
War-room: [Zoom/Meet link]
```

Update the thread (not the original post) at each status change. Post "RESOLVED" when
the incident is closed.

---

## Post-Incident Review

A **blameless post-mortem** is held within **5 business days** of resolution for all
SEV-1 and SEV-2 incidents. SEV-3 incidents may be reviewed at the team's discretion.

**Format:** published as `docs/security/incidents/YYYY-MM-DD-[slug].md`.

**Sections required:**

1. **Incident summary** — date, duration, severity, impact.
2. **Timeline** — chronological events with timestamps.
3. **Root cause analysis** — what failed and why; no blame.
4. **Contributing factors** — process, tooling, or monitoring gaps that enabled the
   incident.
5. **Action items** — each item has an owner and a due date; tracked to closure.
6. **What went well** — detection, response, communication wins.

Action items are filed as GitHub issues and referenced in the post-mortem document.
Closure is confirmed in the next quarterly review.

---

## Sample Incident Timeline

**Incident:** SEV-2 backend deploy regression causing scheduled-run failures.

| Timestamp (UTC) | Event |
| --- | --- |
| T+0 (09:14) | PagerDuty alert fires: `scheduled_run_failure_rate > 10 %` for 5 minutes. |
| T+2m | Primary on-call acks. Checks recent deployments. |
| T+5m | Deploy regression identified: `v0.42.1` introduced a nil-pointer dereference in the schedule executor. |
| T+7m | Incident declared SEV-2 in `#incidents`. IC assigned. |
| T+10m | Rollback to `v0.42.0` initiated via GitHub Actions. |
| T+12m | Rollback complete. Smoke test passes. Failure rate drops to 0 %. |
| T+15m | Status page updated: Monitoring. |
| T+20m | Monitoring period ends. Incident resolved. |
| T+25m | Post-mortem scheduled for T+3 business days. |
| T+3d | Post-mortem published at `docs/security/incidents/2026-05-22-schedule-executor-regression.md`. |

---

## Review Cadence

This runbook is reviewed annually. The next scheduled review date is **2027-05-19**.
