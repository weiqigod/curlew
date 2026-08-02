# Implementation Plan: M4-009

## Overview

Build a SvelteKit admin-only notification configuration page at `/org/[slug]/settings/notifications` that lists notification rules, lets an admin add/delete Slack/email rules via a modal, and shows the recent delivery log, backed by the M4-008 backend notification endpoints (plus two small gap-closing additions — list and delete rule).

## Task Details

- **ID:** M4-009
- **Title:** Web: notification rule configuration UI
- **Phase:** M4: Team Tier
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-008 | Backend: notification dispatcher (Slack + email) | done |

## Code Exploration Findings

### Existing web patterns
- Routes live under `web/src/routes/(app)/org/[slug]/`. The results dashboard at `.../results/` establishes the pattern: `+page.server.ts` loader + `+page.svelte` view + typed API client in `web/src/lib/api/*.ts` + types in `web/src/lib/types/*.ts`.
- Guards live in `web/src/lib/server/guards.ts` (`requireAuth`, `requireTeamTier`). Guard test pattern is table-driven in `guards.test.ts`.
- Sub-nav is rendered in `(app)/+layout.svelte`, gated on `isTeamTier` + current org role.
- Toasts are driven by `?toast=<key>` query params consumed in `(app)/+layout.svelte`. An existing `team_tier_required` toast is shown on redirect.
- Typed fetch wrapper lives in `web/src/lib/api/client.ts` — supports `api.get/post/delete` and throws `ApiError` on non-2xx. Errors expose `{ code, description, status }`.
- Vitest unit tests mock `fetch` per-call with `vi.fn().mockResolvedValue({ ok, json })`.
- Playwright E2E pattern: `web/tests/e2e/*.spec.ts` with `seedAuthCookie(context, EMAIL)` in `beforeEach` + `context.route(...)` to mock backend responses when needed.

### Existing backend endpoints (M4-008)
`web/src/lib/api/notifications.ts` will talk to:
- `POST /api/v1/organizations/{orgId}/notification-rules` — creates rule, returns `NotificationRuleDto`. Validates channel ∈ {slack,email}, slack target starts with `https://`, email target contains `@`, `on` non-empty. Admin-only (403 on non-admin).
- `GET /api/v1/organizations/{orgId}/notification-deliveries?limit=N` — returns `{deliveries: NotificationDeliveryDto[]}`, newest-first. Any member.

Backend DTOs (from `src/ApiTool.Backend/Notifications/`):
```csharp
NotificationRuleDto(Id, Channel, Target, On, CreatedAt)
NotificationDeliveryDto(Id, RuleId, Channel, Status, ResponseCode, AttemptCount, ErrorMessage, AttemptedAt)
```

### Backend gap (must be closed to satisfy the observable)

The observable requires "rules table" + delete flow. The backend already has `NotificationsService.ListRulesAsync` fully implemented (any-member auth) but **no endpoint wires it**. There is no `DeleteRuleAsync` method or endpoint. Two minimal additions are required to close the slice:
- `GET /api/v1/organizations/{orgId}/notification-rules` → `{rules: NotificationRuleDto[]}` (member-auth, calls existing `ListRulesAsync`).
- `DELETE /api/v1/organizations/{orgId}/notification-rules/{ruleId}` → 204 (admin-only). Requires a new `NotificationsService.DeleteRuleAsync` method and a `NotFound` error variant.

These are contained backend changes with near-zero blast radius: they use existing service patterns, existing `OrgId`/`NotificationRuleId` parsers, and existing `NotificationError` enum (plus one new `NotFound` variant).

### Role/permission contract for the web route
- Members of team-tier orgs: redirect to `/org/[slug]?toast=admin_required`.
- Admins/owners of non-team-tier orgs: redirect to `/org/[slug]?toast=team_tier_required` (same as results).
- Admins/owners of team-tier orgs: render.

The `Organization.role` field (`'owner' | 'admin' | 'member'`) already ships on the org-list payload, so no extra API call is needed to determine admin status.

## Implementation Steps

Steps are ordered by blast radius: backend gap first (the deepest dependency — the web UI cannot be verified without it), then shared helpers, then loader + page, then e2e wiring.

---

### Step 1: Backend — add `GET` and `DELETE` notification-rules endpoints

**Rationale:** The observable's "rules table" + "delete rule" assertions require endpoints that do not exist yet. Smallest change that unblocks the web slice; isolated to one service method, one endpoint file, one error variant.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Notifications/NotificationError.cs` | modify | Add `NotFound` variant |
| `src/ApiTool.Backend/Notifications/NotificationsService.cs` | modify | Add `DeleteRuleAsync`; no change to `ListRulesAsync` |
| `src/ApiTool.Backend/Notifications/NotificationsEndpoints.cs` | modify | Map `GET /notification-rules`, `DELETE /notification-rules/{ruleId}` |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsEndpointsTests.cs` | modify | Add tests for list + delete endpoints |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsServiceTests.cs` | modify | Add `DeleteRuleAsync` service tests |

#### New Code (service method sketch)

```csharp
// NotificationsService.cs — add after CreateRuleAsync
public async Task<NotificationError> DeleteRuleAsync(
    Guid userId, Guid orgId, Guid ruleId, CancellationToken ct)
{
    if (!await IsAdminAsync(userId, orgId, ct))
        return NotificationError.PermissionDenied;

    var rule = await db.NotificationRules
        .SingleOrDefaultAsync(r => r.Id == ruleId && r.OrgId == orgId, ct);
    if (rule is null) return NotificationError.NotFound;

    db.NotificationRules.Remove(rule);
    await db.SaveChangesAsync(ct);
    return NotificationError.None;
}
```

```csharp
// NotificationsEndpoints.cs — inside MapNotificationsEndpoints
group.MapGet("/notification-rules", ListRules)
    .WithName("ListNotificationRules")
    .Produces<ListRulesResponse>()
    .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
    .Produces<ErrorResponse>(StatusCodes.Status403Forbidden);

group.MapDelete("/notification-rules/{ruleId}", DeleteRule)
    .WithName("DeleteNotificationRule")
    .Produces(StatusCodes.Status204NoContent)
    .Produces<ErrorResponse>(StatusCodes.Status401Unauthorized)
    .Produces<ErrorResponse>(StatusCodes.Status403Forbidden)
    .Produces<ErrorResponse>(StatusCodes.Status404NotFound);

// Handler private static async Task<IResult> ListRules(...) — calls svc.ListRulesAsync
// Handler private static async Task<IResult> DeleteRule(...) — parses NotificationRuleId, calls svc.DeleteRuleAsync
public sealed record ListRulesResponse(IReadOnlyList<NotificationRuleDto> Rules);
```

#### Tests to Write FIRST (RED phase)

`NotificationsEndpointsTests.cs` additions:

```csharp
[Fact] public async Task Get_notification_rules_returns_200_with_rules_newest_first() { /* seed 2 rules, call GET */ }
[Fact] public async Task Get_notification_rules_returns_empty_list_when_no_rules() { /* fresh org */ }
[Fact] public async Task Get_notification_rules_returns_403_for_non_member() { /* different user */ }
[Fact] public async Task Delete_notification_rule_returns_204_for_admin() { /* seed rule, DELETE, verify gone */ }
[Fact] public async Task Delete_notification_rule_returns_404_for_unknown_id() { /* valid-format id, not in db */ }
[Fact] public async Task Delete_notification_rule_returns_403_for_member_role() { /* non-admin user */ }
```

`NotificationsServiceTests.cs` additions (table-driven where useful):

```csharp
[Fact] public async Task DeleteRuleAsync_removes_rule_and_returns_none_for_admin()
[Fact] public async Task DeleteRuleAsync_returns_PermissionDenied_for_member()
[Fact] public async Task DeleteRuleAsync_returns_NotFound_for_unknown_id()
[Fact] public async Task DeleteRuleAsync_returns_NotFound_when_rule_belongs_to_different_org()
```

#### Impact on Existing Tests
- No existing notification tests break (additions only).
- `NotificationsEndpoints` mapping changes: already-passing tests for POST + GET deliveries continue to pass.
- `AppDbContextSchemaTests`: no schema change — no impact.

---

### Step 2: Web — typed API client and types

**Rationale:** Isolated pure TypeScript changes. Validate types with unit tests before any SvelteKit wiring.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/notifications.ts` | create | Types `NotificationRule`, `NotificationDelivery`, `NotificationChannel`, `NotificationEventName`, `CreateRuleRequest` |
| `web/src/lib/api/notifications.ts` | create | `notificationsApi` with `listRules`, `createRule`, `deleteRule`, `listDeliveries` |
| `web/src/lib/api/notifications.test.ts` | create | Vitest unit tests mocking `fetch` |

#### New Code

```typescript
// web/src/lib/types/notifications.ts
export type NotificationChannel = 'slack' | 'email';
export type NotificationEventName = 'run_failed' | 'flaky';
export type NotificationDeliveryStatus = 'pending' | 'delivered' | 'failed';

export interface NotificationRule {
    id: string;             // nrule_...
    channel: NotificationChannel;
    target: string;
    on: NotificationEventName[];
    created_at: string;     // ISO 8601 UTC
}

export interface NotificationDelivery {
    id: string;             // ndel_...
    rule_id: string;
    channel: NotificationChannel;
    status: NotificationDeliveryStatus;
    response_code: number | null;
    attempt_count: number;
    error_message: string | null;
    attempted_at: string;   // ISO 8601 UTC
}

export interface CreateRuleRequest {
    channel: NotificationChannel;
    target: string;
    on: NotificationEventName[];
}
```

```typescript
// web/src/lib/api/notifications.ts
import { api, type RequestOptions } from './client';
import type {
    NotificationRule,
    NotificationDelivery,
    CreateRuleRequest
} from '$lib/types/notifications';

interface ListRulesResponse { rules: NotificationRule[]; }
interface ListDeliveriesResponse { deliveries: NotificationDelivery[]; }

export const notificationsApi = {
    async listRules(orgId: string, opts?: RequestOptions): Promise<NotificationRule[]> {
        const { rules } = await api.get<ListRulesResponse>(
            `/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules`, opts);
        return rules;
    },
    async createRule(orgId: string, body: CreateRuleRequest, opts?: RequestOptions): Promise<NotificationRule> {
        return api.post<NotificationRule>(
            `/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules`, body, opts);
    },
    async deleteRule(orgId: string, ruleId: string, opts?: RequestOptions): Promise<void> {
        await api.delete<void>(
            `/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules/${encodeURIComponent(ruleId)}`,
            opts);
    },
    async listDeliveries(orgId: string, opts?: RequestOptions & { limit?: number }): Promise<NotificationDelivery[]> {
        const { limit = 25, ...rest } = opts ?? {};
        const { deliveries } = await api.get<ListDeliveriesResponse>(
            `/api/v1/organizations/${encodeURIComponent(orgId)}/notification-deliveries`,
            { ...rest, params: { ...rest.params, limit } });
        return deliveries;
    }
};
```

#### Tests to Write FIRST (RED phase)

```typescript
// web/src/lib/api/notifications.test.ts
describe('notificationsApi.listRules', () => {
    it('returns rules array')
    it('returns empty list on zero rules')
});
describe('notificationsApi.createRule', () => {
    it('posts JSON body and returns created rule')
    it('propagates ApiError on 400 invalid_target')
});
describe('notificationsApi.deleteRule', () => {
    it('sends DELETE and resolves void on 204')
    it('throws ApiError on 404')
});
describe('notificationsApi.listDeliveries', () => {
    it('passes limit=25 by default')
    it('passes custom limit param')
});
```

#### Impact on Existing Tests
- None — new files only.

---

### Step 3: Web — `requireOrgAdmin` guard

**Rationale:** Smallest possible addition to `guards.ts`, needed by the loader in Step 4. Keeps role-policy logic in one place.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/server/guards.ts` | modify | Add `requireOrgAdmin(org, slug)` → throws 303 redirect with `?toast=admin_required` for members |
| `web/src/lib/server/guards.test.ts` | modify | Table-driven cases for the new guard |

#### New Code

```typescript
// guards.ts — append
/**
 * Asserts that the current user's role in the given org is owner or admin.
 * Throws a 303 redirect to /org/[slug]?toast=admin_required for members.
 * Use AFTER requireTeamTier so non-team-tier orgs take precedence.
 */
export function requireOrgAdmin(org: Organization, slug: string): Organization {
    if (org.role === 'owner' || org.role === 'admin') return org;
    throw redirect(303, `/org/${slug}?toast=admin_required`);
}
```

#### Tests to Write FIRST (RED phase)

```typescript
describe('requireOrgAdmin', () => {
    const cases = [
        { name: 'owner passes',  role: 'owner',  expected: 'pass' },
        { name: 'admin passes',  role: 'admin',  expected: 'pass' },
        { name: 'member redirects', role: 'member', expected: 'redirect' }
    ];
    // ...
});
```

#### Impact on Existing Tests
- `guards.test.ts` existing cases unchanged.

---

### Step 4: Web — `NotificationRuleModal` component

**Rationale:** Self-contained presentational component with its own client-side validation; can be tested in isolation via vitest + jsdom before integrating.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/components/notifications/NotificationRuleModal.svelte` | create | Form modal for channel/target/on selection |

#### Component contract

**Props:** `open: boolean`, `submitting: boolean`, `error: string \| null`.
**Events:** `submit: CustomEvent<CreateRuleRequest>`, `close: CustomEvent<void>`.

**Accessibility:**
- `role="dialog"` + `aria-modal="true"` + `aria-labelledby="notif-modal-title"`.
- Labelled inputs (`<label for>`).
- Keyboard dismiss: `Escape` key fires `close`.
- Submit button disabled while `submitting`.

**Client-side validation (runs before dispatching submit):**
- Slack channel ⇒ target must start with `https://` (else show inline error, no dispatch).
- Email channel ⇒ target must match minimal `/^[^\s@]+@[^\s@]+\.[^\s@]+$/` regex.
- `on` must have ≥1 event selected.

Test ids used by the e2e spec: `notif-modal`, `notif-modal-channel`, `notif-modal-target`, `notif-modal-on-run_failed`, `notif-modal-on-flaky`, `notif-modal-submit`, `notif-modal-error`, `notif-modal-close`.

#### Tests to Write FIRST (RED phase)

The component is covered end-to-end by the Playwright spec in Step 7 rather than a unit test — consistent with the existing pattern (no component-level vitest tests for `RecentRunsTable` etc.).

#### Impact on Existing Tests
- None.

---

### Step 5: Web — delivery log and rules table components

**Rationale:** Pure presentational components for the new page. Kept separate from the page shell so the page stays readable.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/components/notifications/RulesTable.svelte` | create | Lists rules with channel/target/events/delete button |
| `web/src/lib/components/notifications/DeliveryLog.svelte` | create | Lists deliveries with status badges |

#### Component contracts

**RulesTable:** Props `rules: NotificationRule[]`, event `delete: CustomEvent<{ruleId: string}>`. Test ids: `rules-table`, `rule-row-${id}`, `rule-delete-${id}`.

**DeliveryLog:** Props `deliveries: NotificationDelivery[]`. Status badge colour by `status`: `delivered` (green), `failed` (red), `pending` (gray). Test ids: `delivery-log`, `delivery-row-${id}`, `delivery-status-${id}`.

Empty-state fallback inside both (renders `"No rules yet. Click Add rule to get started."` / `"No deliveries yet."`).

#### Tests to Write FIRST
- Covered by the e2e spec (DOM-level assertions).

#### Impact on Existing Tests
- None.

---

### Step 6: Web — settings/notifications page and loader

**Rationale:** Wires Steps 2–5 together. Depends on all prior steps; largest blast radius of the web layer.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/settings/notifications/+page.server.ts` | create | Loader: auth → org → team-tier → admin → fetch rules + deliveries in parallel |
| `web/src/routes/(app)/org/[slug]/settings/notifications/+page.svelte` | create | Page shell with Add-rule button, RulesTable, DeliveryLog, modal integration, POST/DELETE via `notificationsApi`, `invalidateAll` to refresh data |
| `web/src/routes/(app)/+layout.svelte` | modify | Add `Notifications` sub-nav link (admin+team only); add `admin_required` toast handling |

#### Loader sketch

```typescript
// +page.server.ts
export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
    const token = requireAuth(locals.accessToken, url.pathname);
    const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
    const teamOrg = requireTeamTier(org, params.slug);
    const adminOrg = requireOrgAdmin(teamOrg, params.slug);

    try {
        const [rules, deliveries] = await Promise.all([
            notificationsApi.listRules(adminOrg.id, { token, fetch }),
            notificationsApi.listDeliveries(adminOrg.id, { token, fetch, limit: 25 })
        ]);
        return { org: adminOrg, rules, deliveries, error: null as string | null };
    } catch (e) {
        const message = e instanceof Error ? e.message : 'failed to load notifications';
        return { org: adminOrg, rules: [], deliveries: [], error: message };
    }
};
```

#### Page sketch

```svelte
<!-- +page.svelte -->
<script lang="ts">
    import { invalidateAll } from '$app/navigation';
    import { notificationsApi } from '$lib/api/notifications';
    // ...
    let modalOpen = false;
    let submitting = false;
    let submitError: string | null = null;

    async function handleSubmit(e: CustomEvent<CreateRuleRequest>) {
        submitting = true;
        submitError = null;
        try {
            await notificationsApi.createRule(data.org.id, e.detail);
            modalOpen = false;
            await invalidateAll();
        } catch (err) {
            submitError = err instanceof Error ? err.message : 'failed to create rule';
        } finally { submitting = false; }
    }

    async function handleDelete(e: CustomEvent<{ruleId: string}>) {
        if (!confirm('Delete this notification rule?')) return;
        await notificationsApi.deleteRule(data.org.id, e.detail.ruleId);
        await invalidateAll();
    }
</script>
```

**Test ids:** `add-rule-button`, plus ids from child components.

#### Layout change

Extend `(app)/+layout.svelte`:
- Add `showAdminRequiredToast = toastParam === 'admin_required'` with its own banner.
- Add sub-nav link `subnav-notifications-link` shown when `isTeamTier` and `currentOrg.role` is `'owner' | 'admin'`.

#### Impact on Existing Tests
- `org-results.spec.ts` asserts `subnav-results-link` is visible — unchanged. Adding a second sub-nav link does not break that assertion.
- The new `admin_required` toast uses a distinct test id, so existing `toast-team-tier-required` assertions are unchanged.

---

### Step 7: Web — Playwright E2E spec

**Rationale:** The observable. Written last to cover the integrated stack; mirrors `org-results.spec.ts` conventions.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/org-notifications.spec.ts` | create | 6 tests covering observable assertions 1–5 plus a11y |
| `web/tests/e2e/helpers/fixtures.ts` | modify | Add `MEMBER_EMAIL = 'member@example.com'` |
| `scripts/seed-test-data.sh` | modify | Seed one slack rule + one failing result to produce one delivery row |
| `testdata/web/seed-results/failing-result.json` | create | A failing-result payload that triggers the `run_failed` dispatcher |

#### Seed script extensions

Add after the existing org/result seeding:

```bash
# 1) Create a slack rule for run_failed events (idempotent: check existing rules first)
EXISTING_RULES=$(curl -sS -H "Authorization: Bearer $TOKEN" \
  "$BACKEND_URL/api/v1/organizations/$ORG_ID/notification-rules" | jq '.rules | length')
if [ "$EXISTING_RULES" = "0" ]; then
  curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
    -H "Content-Type: application/json" \
    -d '{"channel":"slack","target":"https://hooks.slack.test/seed","on":["run_failed"]}' \
    "$BACKEND_URL/api/v1/organizations/$ORG_ID/notification-rules" >/dev/null
fi

# 2) Upload a failing result to trigger a delivery attempt
curl -sS -X POST -H "Authorization: Bearer $TOKEN" \
  -H "Content-Type: application/json" \
  --data-binary "@$REPO_ROOT/testdata/web/failing-result.json" \
  "$BACKEND_URL/api/v1/organizations/$ORG_ID/results" >/dev/null
```

Since the test stack boots a dev Slack webhook target (`https://hooks.slack.test/seed`) which cannot resolve, the resulting delivery will be recorded with `status=failed`. That still satisfies "one delivery row" — the spec will assert distinct status badges only when there are multiple. For the single-row case we assert "at least one row".

**Decision on distinct status badges test:** The observable says "Given the delivery log contains one failed and one delivered entry, when the page loads, then both are displayed with distinct status badges." We cannot cheaply produce a delivered result in the test stack (no live Slack). We will cover this assertion via `context.route` mocking in the e2e spec for that specific test — overriding the deliveries response with two mock rows — consistent with the `empty state` and `500` patterns in `org-results.spec.ts`.

#### E2E spec outline

```typescript
test.describe('Team notification settings', () => {
    test.beforeEach(async ({ context }) => { await seedAuthCookie(context, OWNER_EMAIL); });

    test('renders rules and delivery log from backend', /* … */);                 // assertion 1 + 7
    test('admin can add a slack rule via the modal', /* … */);                    // assertion 2
    test('client-side validation blocks non-https slack target', /* … */);        // assertion 3
    test('admin can delete a rule', /* … */);                                     // assertion 4
    test('member is redirected to org overview with admin-required toast',
      /* uses context.route to override org-list with role=member */);            // assertion 5
    test('delivery log shows distinct status badges for delivered + failed',
      /* context.route mocks deliveries with two rows */);                        // assertion 6
    test('modal is keyboard-dismissable and has labelled inputs', /* a11y */);    // DoD a11y
});
```

#### Impact on Existing Tests
- `org-results.spec.ts`: unchanged. Both specs run serially (`workers: 1`).

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `src/ApiTool.Backend.Tests/Notifications/NotificationsEndpointsTests.cs` | all existing | none | — |
| `src/ApiTool.Backend.Tests/Notifications/NotificationsServiceTests.cs` | all existing | none | — |
| `src/ApiTool.Backend.Tests/Data/AppDbContextSchemaTests.cs` | all | none | — |
| `web/src/lib/server/guards.test.ts` | existing `requireAuth`, `requireTeamTier` | none | add `requireOrgAdmin` describe block |
| `web/src/lib/api/organizations.test.ts` | all | none | — |
| `web/src/lib/api/results.test.ts` | all | none | — |
| `web/tests/e2e/org-results.spec.ts` | all | none | — |

New tests added:
- Backend: ≥6 endpoint + ≥4 service tests
- Web unit: ≥10 vitest cases (api client + guard)
- E2E: 7 playwright tests (observable requires ≥5)

---

## Risks and Edge Cases

- **Risk: Backend endpoints not yet wired.** → **Mitigation:** Add the two endpoints as Step 1 with their own test suite; they reuse the proven service + auth patterns of M4-008.
- **Risk: NotificationRule id parsing fails for delete endpoint.** → **Mitigation:** Reuse existing `NotificationRuleId.TryParse`; return 404 on parse failure to avoid leaking whether the id format is valid.
- **Edge case: Multiple sub-nav links on team-tier orgs.** → **Handling:** The notifications link is gated on `currentOrg.role \u2208 {'owner','admin'}` so members see only the Test Results link.
- **Edge case: Modal opens while a request is in flight.** → **Handling:** Submit button shows `submitting` state and is `disabled`; escape-to-close is disabled during submission.
- **Edge case: Client-side validation mismatch with server.** → **Handling:** The client regex is deliberately a strict subset of server rules (slack ⇒ `https://` prefix, email ⇒ minimal `@.+\.` pattern). On server-side 400, the error bubbles via `ApiError.description` into `notif-modal-error`.
- **Edge case: `invalidateAll` races with modal close.** → **Handling:** Close modal after `invalidateAll` resolves so the table re-renders before the user sees the form disappear.
- **Edge case: Seeded rule duplicated on repeat `seed` runs.** → **Handling:** Seed script pre-checks rule count via the new `GET /notification-rules` endpoint and skips if ≥1 rule exists.
- **Risk: Backend emits `DateTime` without `Z` suffix breaking `new Date(...)` locale parsing.** → **Mitigation:** Use `.created_at` display via `toLocaleDateString`; backend already returns ISO-8601 UTC per `DateTime.UtcDateTime` in the service.
- **Risk: Distinct-status-badges test depends on live dispatcher which is flaky.** → **Mitigation:** Mock the deliveries endpoint with `context.route` in that single test, same pattern as `empty state shown when backend has no results` in `org-results.spec.ts`.

---

## Verification

```bash
# Backend
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj \
  --filter "FullyQualifiedName~Notifications"

# Web unit
cd web && npm run check && npm run lint && npm run test:unit

# E2E (observable)
cd web && CURLEW_MANAGE_STACK=1 npm run test:e2e -- tests/e2e/org-notifications.spec.ts
# Expected: 7 passing (observable requires >=5).
```

Observable verification (copied verbatim from the task YAML):

```bash
cd web && npm run test:e2e -- tests/e2e/org-notifications.spec.ts
```
