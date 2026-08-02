# Implementation Plan: M4-011

## Overview

Deliver a team-tier billing and seat management portal in the SvelteKit web app: two new role-guarded routes (`/org/[slug]/billing` for owners, `/org/[slug]/members` for admins) backed by typed API clients for `/api/v1/subscriptions` and `/api/v1/organizations/{id}/invitations`, plus a Playwright E2E spec that drives them against the docker-compose stack.

## Task Details

- **ID:** M4-011
- **Title:** Web: billing and seat management portal
- **Phase:** M4: Team Tier
- **Priority:** 3
- **Complexity:** high
- **Branch:** `feature/M4-011-web-billing-members`

## Dependencies

| Task   | Title                                              | Status |
|--------|----------------------------------------------------|--------|
| M4-010 | Backend: billing, seat management, and invitations | done   |

M4-010 ships `SubscriptionsEndpoints` (checkout/get/patch/cancel/reactivate/portal), `InvitationsEndpoints` (create/list/resend/revoke/accept), and `MembersEndpoints` (list/update-role/remove/transfer/leave). All response bodies use snake_case via `JsonNamingPolicy.SnakeCaseLower`.

---

## Key Architectural Decisions

1. **Mirror existing web conventions.** The notifications slice (M4-009) is the closest precedent — PageServerLoad uses `requireAuth` + `requireTeamTier` + role guard; typed api modules in `$lib/api/*.ts`; Svelte modals under `$lib/components/<domain>/*Modal.svelte`; `data-testid` attributes on every interactive element. This plan reuses all of that.
2. **New `requireOrgOwner` guard.** The existing `requireOrgAdmin` covers admin routes (`/members`) but billing must be owner-only per backend rules (`SubscriptionsService` only allows Owner to PATCH/cancel/portal). Add a small companion guard rather than inlining the role check.
3. **Types mirror backend DTOs exactly.** Snake_case wire fields (`seat_count`, `current_period_end`, `cancel_at_period_end`, etc.) in `$lib/types/subscriptions.ts` and `$lib/types/invitations.ts`. This satisfies the DoD item "Types in web/src/lib/types/subscriptions.ts mirror backend OpenAPI".
4. **Backend GET /subscriptions takes `org_id` query param** (see `SubscriptionsEndpoints.GetSubscription`). The client module must pass it via `params: { org_id: ... }`.
5. **Portal redirect via anchor with `rel="noopener"`.** After the POST returns `{ portal_url }`, we set `window.location.href = portal_url` inside a click handler on an `<a>` tag rendered with `rel="noopener noreferrer" target="_self"` — satisfies DoD a11y item.
6. **Seats usage bar is CSS-only.** No chart library; a simple progress-bar div with `aria-valuenow/min/max` keeps the bundle small and a11y-friendly.
7. **E2E spec uses seeded-backend facts for the owner happy path** (billing card render, members list) and **Playwright route mocks for error/edge states** (403 member, proration preview, invite creation) — matching the pattern in `org-notifications.spec.ts`. This keeps the spec deterministic without needing new seed fixtures beyond a team-tier org that already exists via `scripts/seed-test-data.sh`.
8. **Members endpoint path for ops** uses `/api/v1/organizations/{orgId}/members` and `/api/v1/organizations/{orgId}/invitations` — already backend-ready.
9. **Sub-nav additions are role-gated in `(app)/+layout.svelte`.** Owner sees "Billing"; Admin+Owner see "Members". Follows the existing `isAdmin`/`isTeamTier` pattern.
10. **Proration preview uses PATCH response.** The backend PATCH returns both the updated `SubscriptionDto` and a `ProrationResult { credit, charge, net }`. The "Add seats" modal submits directly and shows the returned proration inline; no separate dry-run endpoint exists.

Open questions resolved:

- *Do we need separate "preview" vs "confirm" steps for Add seats?* The backend has no preview endpoint; proration is only returned on PATCH. The modal will submit → show proration result inline → close button. Matches spec section 6554–6567.
- *Which role can cancel an invitation?* Admin or Owner per `InvitationsService.RevokeAsync` (matches the `requireOrgAdmin` guard).
- *Routing for non-owner on `/billing`?* The existing `requireOrgAdmin` redirects non-admins to `/org/[slug]?toast=admin_required`. A new `requireOrgOwner` will redirect non-owners similarly with `toast=owner_required`. The layout gains a matching toast. This satisfies behavior #7 by rendering a visible banner rather than a blank layout.

---

## Implementation Steps

Ordered by blast radius (smallest first).

### Step 1: Types for subscriptions and invitations

**Rationale:** Pure TypeScript definitions — no runtime impact. Everything downstream depends on these types, so lock them in first.

#### Files to Modify

| File                                              | Action | Description                                   |
|---------------------------------------------------|--------|-----------------------------------------------|
| `web/src/lib/types/subscriptions.ts`              | create | Wire types for `SubscriptionDto`, `ProrationResult`, request bodies, portal response |
| `web/src/lib/types/invitations.ts`                | create | Wire types for `InvitationDto`, create/list responses |
| `web/src/lib/types/members.ts`                    | create | Wire type for `MemberDto` list response       |

#### New Code — `subscriptions.ts`

```ts
/** Subscription tier identifier (backend mirror). */
export type SubscriptionTier = 'solo' | 'professional' | 'team';

/** Lifecycle state of a subscription. */
export type SubscriptionStatus = 'active' | 'past_due' | 'canceled' | 'trialing' | 'incomplete';

/** Billing interval. */
export type BillingInterval = 'month' | 'year';

/** Wire model mirroring backend SubscriptionDto. */
export interface Subscription {
  id: string;
  org_id: string;
  tier: SubscriptionTier;
  status: SubscriptionStatus;
  interval: BillingInterval;
  seat_count: number;
  seat_limit: number;
  current_period_start: string; // ISO 8601 UTC
  current_period_end: string;
  cancel_at_period_end: boolean;
  created_at: string;
}

/** Proration breakdown (amounts in cents). */
export interface ProrationResult {
  credit: number;
  charge: number;
  net: number;
}

export interface GetSubscriptionResponse {
  subscription: Subscription | null;
  tier: string;
}

export interface UpdateSubscriptionRequest {
  tier?: SubscriptionTier;
  seat_count?: number;
  interval?: BillingInterval;
}

export interface UpdateSubscriptionResponse {
  subscription: Subscription;
  proration: ProrationResult;
}

export interface PortalRequest {
  org_id: string;
  return_url: string;
}

export interface PortalResponse {
  portal_url: string;
}
```

#### New Code — `invitations.ts`

```ts
export type InvitationRole = 'admin' | 'member';

export interface Invitation {
  id: string;
  org_id: string;
  email: string;
  role: InvitationRole;
  expires_at: string;
  created_at: string;
  accepted_at: string | null;
  revoked_at: string | null;
}

export interface CreateInvitationRequest {
  email: string;
  role: InvitationRole;
}

export interface CreateInvitationResponse {
  invitation: Invitation;
  token: string;
}

export interface ListInvitationsResponse {
  invitations: Invitation[];
}
```

#### New Code — `members.ts`

```ts
import type { OrgRole } from './organization';

export interface Member {
  user_id: string;
  role: OrgRole;
  joined_at: string;
}

export interface ListMembersResponse {
  members: Member[];
}
```

#### Tests to Write FIRST (RED phase)

No runtime logic — these are pure type declarations. Type correctness is enforced by `svelte-check` in Step 8 verification. No new `.test.ts` file required for this step.

#### Impact on Existing Tests

None.

---

### Step 2: `subscriptionsApi` client

**Rationale:** Small self-contained module with unit tests (mocked `fetch`) — matches the shape of `notifications.ts` / `pr-checks.ts`. No cross-file impact.

#### Files to Modify

| File                                   | Action | Description                                        |
|----------------------------------------|--------|----------------------------------------------------|
| `web/src/lib/api/subscriptions.ts`     | create | `get(orgId)`, `update(id, body)`, `portal(orgId, returnUrl)` |
| `web/src/lib/api/subscriptions.test.ts`| create | Vitest unit tests with mocked `fetch`              |

#### New Code — `subscriptions.ts`

```ts
import { api, type RequestOptions } from './client';
import type {
  GetSubscriptionResponse,
  UpdateSubscriptionRequest,
  UpdateSubscriptionResponse,
  PortalResponse
} from '$lib/types/subscriptions';

/** Typed client for /api/v1/subscriptions endpoints. */
export const subscriptionsApi = {
  /** Fetches the subscription for an org (owner or admin). */
  async get(orgId: string, opts?: RequestOptions): Promise<GetSubscriptionResponse> {
    return api.get<GetSubscriptionResponse>('/api/v1/subscriptions', {
      ...opts,
      params: { ...opts?.params, org_id: orgId }
    });
  },

  /** Updates a subscription (owner only). */
  async update(
    id: string,
    body: UpdateSubscriptionRequest,
    opts?: RequestOptions
  ): Promise<UpdateSubscriptionResponse> {
    return api.patch<UpdateSubscriptionResponse>(
      `/api/v1/subscriptions/${encodeURIComponent(id)}`,
      body,
      opts
    );
  },

  /** Creates a Stripe customer-portal session URL (owner only). */
  async portal(
    orgId: string,
    returnUrl: string,
    opts?: RequestOptions
  ): Promise<PortalResponse> {
    return api.post<PortalResponse>(
      '/api/v1/subscriptions/portal',
      { org_id: orgId, return_url: returnUrl },
      opts
    );
  }
};
```

#### Tests to Write FIRST (RED phase)

```ts
describe('subscriptionsApi.get', () => {
  it('fetches subscription with org_id query param');
  it('returns { subscription: null, tier } when org has no subscription');
  it('propagates ApiError on 403 permission_denied');
});

describe('subscriptionsApi.update', () => {
  it('sends PATCH with body and returns subscription + proration');
  it('propagates ApiError on 403 permission_denied');
  it('propagates ApiError on 403 subscription_downgrade_blocked');
});

describe('subscriptionsApi.portal', () => {
  it('POSTs { org_id, return_url } and returns { portal_url }');
  it('propagates ApiError on 403 permission_denied');
});
```

#### Impact on Existing Tests

None.

---

### Step 3: `invitationsApi` and `membersApi` clients

**Rationale:** Members are read-only for this slice (listed; role changes are deferred to a later task). Invitations need list/create/revoke. Same pattern as Step 2.

#### Files to Modify

| File                                   | Action | Description                                   |
|----------------------------------------|--------|-----------------------------------------------|
| `web/src/lib/api/invitations.ts`       | create | `list(orgId)`, `create(orgId, body)`, `revoke(orgId, invId)` |
| `web/src/lib/api/invitations.test.ts`  | create | Vitest unit tests                             |
| `web/src/lib/api/members.ts`           | create | `list(orgId)`                                 |
| `web/src/lib/api/members.test.ts`      | create | Vitest unit tests                             |

#### New Code — `invitations.ts`

```ts
import { api, type RequestOptions } from './client';
import type {
  Invitation,
  CreateInvitationRequest,
  CreateInvitationResponse,
  ListInvitationsResponse
} from '$lib/types/invitations';

export const invitationsApi = {
  async list(orgId: string, opts?: RequestOptions): Promise<Invitation[]> {
    const { invitations } = await api.get<ListInvitationsResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/invitations`,
      opts
    );
    return invitations;
  },

  async create(
    orgId: string,
    body: CreateInvitationRequest,
    opts?: RequestOptions
  ): Promise<CreateInvitationResponse> {
    return api.post<CreateInvitationResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/invitations`,
      body,
      opts
    );
  },

  async revoke(orgId: string, invitationId: string, opts?: RequestOptions): Promise<void> {
    await api.delete<void>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/invitations/${encodeURIComponent(invitationId)}`,
      opts
    );
  }
};
```

#### New Code — `members.ts`

```ts
import { api, type RequestOptions } from './client';
import type { Member, ListMembersResponse } from '$lib/types/members';

export const membersApi = {
  async list(orgId: string, opts?: RequestOptions): Promise<Member[]> {
    const { members } = await api.get<ListMembersResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/members`,
      opts
    );
    return members;
  }
};
```

#### Tests to Write FIRST (RED phase)

Table-driven tests covering the same matrix as `notifications.test.ts`: one happy-path per method, one ApiError propagation per method. Named cases:

- `invitationsApi.list: returns invitations array`
- `invitationsApi.list: returns empty list`
- `invitationsApi.create: posts body, returns invitation + token`
- `invitationsApi.create: propagates 400 invalid_email`
- `invitationsApi.create: propagates 409 invitation_pending`
- `invitationsApi.create: propagates 409 seat_limit_reached`
- `invitationsApi.revoke: DELETE resolves void`
- `invitationsApi.revoke: propagates 404 invitation_not_found`
- `membersApi.list: returns members array`
- `membersApi.list: propagates 403 permission_denied`

#### Impact on Existing Tests

None.

---

### Step 4: Add `requireOrgOwner` guard and a matching toast

**Rationale:** Shared server-side helper used by both the billing page and (future) webhooks. Small surgical addition to an existing file — and its test file — with table-driven role matrix.

#### Files to Modify

| File                                    | Action | Description                                            |
|-----------------------------------------|--------|--------------------------------------------------------|
| `web/src/lib/server/guards.ts`          | modify | Add `requireOrgOwner` function                         |
| `web/src/lib/server/guards.test.ts`     | modify | Add describe block with owner/admin/member cases       |
| `web/src/routes/(app)/+layout.svelte`   | modify | Add `showOwnerRequiredToast` mirroring the admin one; add Billing + Members links to subnav |

#### Current Code — `guards.ts` (bottom of file)

```ts
export function requireOrgAdmin(org: Organization, slug: string): Organization {
  if (ADMIN_ROLES.includes(org.role)) return org;
  throw redirect(303, `/org/${slug}?toast=admin_required`);
}
```

#### New Code — `guards.ts`

```ts
/**
 * Asserts the current user is the owner of the given org.
 * Redirects admins/members to /org/[slug]?toast=owner_required.
 */
export function requireOrgOwner(org: Organization, slug: string): Organization {
  if (org.role === 'owner') return org;
  throw redirect(303, `/org/${slug}?toast=owner_required`);
}
```

#### New Code — `+layout.svelte` additions

- Add `$: showOwnerRequiredToast = toastParam === 'owner_required';` and a rendered toast block with `data-testid="toast-owner-required"`.
- Extend sub-nav:

```svelte
{#if isTeamTier && currentOrg?.role === 'owner'}
  <a
    href="/org/{currentOrg.slug}/billing"
    data-testid="subnav-billing-link"
    class="text-gray-600 hover:text-gray-900"
  >
    Billing
  </a>
{/if}
{#if isTeamTier && isAdmin}
  <a
    href="/org/{currentOrg.slug}/members"
    data-testid="subnav-members-link"
    class="text-gray-600 hover:text-gray-900"
  >
    Members
  </a>
{/if}
```

- Update the toast-dismissal-reset reactive statement to include `'owner_required'` in the recognised param set.

#### Tests to Write FIRST (RED phase) — `guards.test.ts`

```ts
describe('requireOrgOwner', () => {
  const cases: Array<{ name: string; role: Organization['role']; expected: 'pass' | 'redirect' }> = [
    { name: 'owner passes', role: 'owner', expected: 'pass' },
    { name: 'admin redirects', role: 'admin', expected: 'redirect' },
    { name: 'member redirects', role: 'member', expected: 'redirect' }
  ];
  // ...same runner shape as requireOrgAdmin describe
});
```

#### Impact on Existing Tests

- `guards.test.ts`: the existing `requireAuth`, `requireTeamTier`, and `requireOrgAdmin` blocks remain untouched — new block added.
- `(app)/+layout.svelte` has no tests (visual component) but is exercised by existing E2E specs. The new subnav links and owner-required toast are gated on `currentOrg?.role === 'owner'` so the notifications spec (uses owner) is unaffected — new links appear but its assertions don't count nav items.

---

### Step 5: Modal components `AddSeatsModal.svelte` and `InviteMemberModal.svelte`

**Rationale:** Self-contained presentation components with no server interaction — easiest to unit-test visually and reuse. Built before the page routes so the pages can import them directly.

#### Files to Modify

| File                                                         | Action | Description                                             |
|--------------------------------------------------------------|--------|---------------------------------------------------------|
| `web/src/lib/components/billing/AddSeatsModal.svelte`        | create | Modal for adding seats: input with `+N` delta, submit   |
| `web/src/lib/components/billing/SeatsUsageBar.svelte`        | create | Reusable `aria-valuenow` bar rendering seat_count/seat_limit |
| `web/src/lib/components/members/InviteMemberModal.svelte`    | create | Modal with email + role select, submit                  |

All three components follow the `NotificationRuleModal.svelte` pattern: `export let open/submitting/error`, `createEventDispatcher` with `submit` / `close`, `Escape` keyboard handler, labelled inputs, `data-testid` on every interactive element, validation before dispatch.

#### New Code — `AddSeatsModal.svelte` (critical fragments)

```svelte
<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { ProrationResult } from '$lib/types/subscriptions';

  export let open = false;
  export let submitting = false;
  export let error: string | null = null;
  /** Current seat count — used as baseline for the "+N" input. */
  export let currentSeatCount: number;
  /** Proration result from a successful PATCH, shown inline before close. */
  export let proration: ProrationResult | null = null;

  const dispatch = createEventDispatcher<{
    submit: { seatCount: number };
    close: void;
  }>();

  let addCount = 3;
  let validationError: string | null = null;

  function validate(): boolean {
    if (!Number.isInteger(addCount) || addCount < 1) {
      validationError = 'Add at least 1 seat.';
      return false;
    }
    validationError = null;
    return true;
  }

  function handleSubmit() {
    if (!validate()) return;
    dispatch('submit', { seatCount: currentSeatCount + addCount });
  }
</script>
```

Markup highlights:

- `data-testid="add-seats-modal"`
- `data-testid="add-seats-delta"` on `<input type="number">`
- `data-testid="add-seats-submit"`
- `data-testid="add-seats-proration"` block rendered when `proration !== null` showing `net` in dollars.

#### New Code — `InviteMemberModal.svelte` (critical fragments)

```svelte
<script lang="ts">
  import { createEventDispatcher } from 'svelte';
  import type { CreateInvitationRequest, InvitationRole } from '$lib/types/invitations';

  export let open = false;
  export let submitting = false;
  export let error: string | null = null;

  const dispatch = createEventDispatcher<{ submit: CreateInvitationRequest; close: void }>();

  let email = '';
  let role: InvitationRole = 'member';
  let validationError: string | null = null;

  const EMAIL_RE = /^[^\s@]+@[^\s@]+\.[^\s@]+$/;

  function handleSubmit() {
    if (!EMAIL_RE.test(email)) {
      validationError = 'Enter a valid email address.';
      return;
    }
    validationError = null;
    dispatch('submit', { email, role });
  }
</script>
```

Test ids: `invite-modal`, `invite-modal-email`, `invite-modal-role`, `invite-modal-submit`, `invite-modal-error`.

#### New Code — `SeatsUsageBar.svelte`

Simple CSS progress bar: `<div role="progressbar" aria-valuenow={seatCount} aria-valuemin="0" aria-valuemax={seatLimit} data-testid="seats-usage-bar">`.

#### Tests to Write FIRST (RED phase)

Svelte component unit tests are out of scope for this slice (matches the notifications slice, which relies on E2E coverage). Validation logic is covered by the E2E spec assertions in Step 8.

#### Impact on Existing Tests

None.

---

### Step 6: Billing page — `/org/[slug]/billing`

**Rationale:** First composed route. Uses every piece from Steps 1–5 and the existing guard chain.

#### Files to Modify

| File                                                                  | Action | Description                                             |
|-----------------------------------------------------------------------|--------|---------------------------------------------------------|
| `web/src/routes/(app)/org/[slug]/billing/+page.server.ts`             | create | Loader: `requireAuth` → `requireTeamTier` → `requireOrgOwner` → `subscriptionsApi.get(org.id)` |
| `web/src/routes/(app)/org/[slug]/billing/+page.svelte`                | create | Renders subscription card (tier, interval, price, renewal, seats bar, add seats button, manage billing link); orchestrates `AddSeatsModal` |

#### New Code — `+page.server.ts`

```ts
import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier, requireOrgOwner } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { subscriptionsApi } from '$lib/api/subscriptions';

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname);
  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const teamOrg = requireTeamTier(org, params.slug);
  const ownerOrg = requireOrgOwner(teamOrg, params.slug);

  try {
    const { subscription, tier } = await subscriptionsApi.get(ownerOrg.id, { token, fetch });
    return { org: ownerOrg, subscription, tier, error: null as string | null };
  } catch (e) {
    const message = e instanceof Error ? e.message : 'Failed to load subscription.';
    return { org: ownerOrg, subscription: null, tier: 'team', error: message };
  }
};
```

#### New Code — `+page.svelte` (contract-level)

- `<h1>Billing</h1>` with `data-testid="billing-heading"`
- Subscription card `data-testid="subscription-card"` containing:
  - `data-testid="subscription-tier"` → `{subscription.tier}`
  - `data-testid="subscription-price"` → formatted (USD, from hardcoded pricebook: team=$39/mo or $390/yr) — kept simple; backend has no price field
  - `data-testid="subscription-renewal"` → formatted `current_period_end`
  - `<SeatsUsageBar seatCount={sub.seat_count} seatLimit={sub.seat_limit} />`
- Buttons:
  - `data-testid="add-seats-button"` → opens `AddSeatsModal`
  - `data-testid="manage-billing-button"` → triggers `subscriptionsApi.portal(org.id, window.location.href)` then `window.location.href = portal_url`. Rendered as `<a rel="noopener noreferrer" href="#" on:click|preventDefault>`.
- Inline error banner when `data.error` set.
- Handler for `AddSeatsModal` submit: calls `subscriptionsApi.update(sub.id, { seat_count })`, stores `proration` result, re-invalidates via `invalidateAll()`.

#### Tests to Write FIRST (RED phase)

Server loader has no unit tests (no pure logic beyond composition of guards, matches `pr-checks/+page.server.ts`). Page is tested via E2E in Step 8.

#### Impact on Existing Tests

None — new route.

---

### Step 7: Members page — `/org/[slug]/members`

**Rationale:** Second composed route. Admin-only. Same shape as billing but with two tables (active members + pending invitations) and an invite modal.

#### Files to Modify

| File                                                                  | Action | Description                                             |
|-----------------------------------------------------------------------|--------|---------------------------------------------------------|
| `web/src/routes/(app)/org/[slug]/members/+page.server.ts`             | create | Loader: auth → team tier → admin → `Promise.all([membersApi.list, invitationsApi.list])` |
| `web/src/routes/(app)/org/[slug]/members/+page.svelte`                | create | Renders MembersTable + InvitationsTable + invite modal orchestration |
| `web/src/lib/components/members/MembersTable.svelte`                  | create | Table of active members (user_id truncated, role, joined date) |
| `web/src/lib/components/members/InvitationsTable.svelte`              | create | Table of pending invitations with per-row "Cancel" button |

Both tables follow the `RulesTable.svelte` pattern — `data-testid="members-table"` / `"invitations-table"`, per-row `data-testid="member-row-{userId}"` and `"invitation-row-{inviteId}"`, and the invitations table dispatches a `cancel` event handled in the page with the same inline-confirmation banner pattern as the notifications page.

#### New Code — `+page.server.ts`

```ts
import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { membersApi } from '$lib/api/members';
import { invitationsApi } from '$lib/api/invitations';

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname);
  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const teamOrg = requireTeamTier(org, params.slug);
  const adminOrg = requireOrgAdmin(teamOrg, params.slug);

  try {
    const [members, invitations] = await Promise.all([
      membersApi.list(adminOrg.id, { token, fetch }),
      invitationsApi.list(adminOrg.id, { token, fetch })
    ]);
    return { org: adminOrg, members, invitations, error: null as string | null };
  } catch (e) {
    const message = e instanceof Error ? e.message : 'Failed to load members.';
    return { org: adminOrg, members: [], invitations: [], error: message };
  }
};
```

#### Tests to Write FIRST (RED phase)

None at unit level (components are visual, loaders are composition). E2E coverage in Step 8.

#### Impact on Existing Tests

None.

---

### Step 8: E2E Playwright spec `tests/e2e/org-billing.spec.ts`

**Rationale:** The task's observable outcome. Last step so it exercises the entire stack assembled above.

#### Files to Modify

| File                                         | Action | Description                                    |
|----------------------------------------------|--------|------------------------------------------------|
| `web/tests/e2e/org-billing.spec.ts`          | create | Playwright spec with ≥6 assertions             |
| `web/README.md`                              | modify | Document "team-tier org seed" for local dev    |
| `scripts/seed-test-data.sh`                  | modify | Add idempotent seat/subscription seeding if not already present (check via `GET /api/v1/subscriptions?org_id=...`) |

The spec mirrors the mocking-heavy style of `org-notifications.spec.ts`. The docker-compose stack already provides the team-tier org via `seed-test-data.sh`. For subscription data the backend's `FakeStripeGateway` returns deterministic data in dev — we let the stack respond directly for the owner-view assertions and mock only the PATCH / POST interactions for deterministic proration values.

#### Spec structure (≥ 6 assertions required)

```ts
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const BILLING_URL = `/org/${SEEDED_ORG_SLUG}/billing`;
const MEMBERS_URL = `/org/${SEEDED_ORG_SLUG}/members`;

test.describe('Team billing & members portal', () => {
  test.beforeEach(async ({ context }) => {
    await seedAuthCookie(context, OWNER_EMAIL);
  });

  test('1. owner sees subscription card on /billing', async ({ page, context }) => {
    await context.route('**/api/v1/subscriptions?**', (route) =>
      route.fulfill({ status: 200, body: JSON.stringify({
        subscription: { id: 'sub_test', org_id: 'org_test', tier: 'team', status: 'active',
          interval: 'month', seat_count: 3, seat_limit: 10,
          current_period_start: '2026-04-01T00:00:00Z',
          current_period_end: '2026-05-01T00:00:00Z',
          cancel_at_period_end: false, created_at: '2026-04-01T00:00:00Z' },
        tier: 'team'
      }) })
    );
    await page.goto(BILLING_URL);
    await expect(page.getByTestId('subscription-card')).toBeVisible();
    await expect(page.getByTestId('subscription-tier')).toContainText('team');
    await expect(page.getByTestId('seats-usage-bar')).toHaveAttribute('aria-valuenow', '3');
  });

  test('2. owner adds +3 seats and sees proration preview', async ({ page, context }) => {
    // mock GET first (3 seats), then PATCH returning proration, then GET again (6 seats)
    let seatCount = 3;
    await context.route('**/api/v1/subscriptions?**', (route) => { /* ... */ });
    await context.route('**/api/v1/subscriptions/*', (route) => {
      if (route.request().method() === 'PATCH') {
        const body = JSON.parse(route.request().postData() ?? '{}');
        seatCount = body.seat_count;
        route.fulfill({ status: 200, body: JSON.stringify({
          subscription: { /* seat_count: seatCount */ },
          proration: { credit: 0, charge: 5400, net: 5400 }
        }) });
      }
    });
    await page.goto(BILLING_URL);
    await page.getByTestId('add-seats-button').click();
    await page.getByTestId('add-seats-delta').fill('3');
    await page.getByTestId('add-seats-submit').click();
    await expect(page.getByTestId('add-seats-proration')).toContainText('$54');
  });

  test('3. member is redirected from /billing with owner_required toast', async ({ page, context }) => {
    await context.clearCookies();
    await seedAuthCookie(context, MEMBER_EMAIL);
    // mock orgs list so org appears with role=member
    await context.route('**/api/v1/organizations', (route) =>
      route.fulfill({ status: 200, body: JSON.stringify({ organizations: [{
        id: 'org_test', slug: SEEDED_ORG_SLUG, name: 'Acme', role: 'member',
        seat_count: 3, seat_limit: 10, status: 'active',
        created_at: '2026-04-01T00:00:00Z', tier: 'team'
      }] }) })
    );
    await page.goto(BILLING_URL);
    await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
    await expect(page.getByTestId('toast-owner-required')).toBeVisible();
  });

  test('4. admin invites member@example.com and sees pending row', async ({ page, context }) => {
    const newInv = {
      id: 'inv_test123', org_id: 'org_test', email: 'newuser@example.com',
      role: 'member', expires_at: '2026-04-24T00:00:00Z',
      created_at: '2026-04-17T00:00:00Z', accepted_at: null, revoked_at: null
    };
    let getCount = 0;
    await context.route('**/api/v1/organizations/*/invitations', (route) => {
      if (route.request().method() === 'GET') {
        getCount++;
        route.fulfill({ status: 200, body: JSON.stringify({
          invitations: getCount === 1 ? [] : [newInv]
        }) });
      } else if (route.request().method() === 'POST') {
        route.fulfill({ status: 201, body: JSON.stringify({ invitation: newInv, token: 'raw_tok' }) });
      }
    });
    await context.route('**/api/v1/organizations/*/members', (route) =>
      route.fulfill({ status: 200, body: JSON.stringify({ members: [] }) })
    );
    await page.goto(MEMBERS_URL);
    await page.getByTestId('invite-button').click();
    await page.getByTestId('invite-modal-email').fill('newuser@example.com');
    await page.getByTestId('invite-modal-submit').click();
    await expect(page.getByTestId(`invitation-row-${newInv.id}`)).toBeVisible();
  });

  test('5. admin cancels a pending invitation', async ({ page, context }) => { /* ... */ });

  test('6. manage-billing click calls /portal and redirects', async ({ page, context }) => {
    await context.route('**/api/v1/subscriptions/portal', (route) =>
      route.fulfill({ status: 200, body: JSON.stringify({ portal_url: 'https://stripe.test/portal/xyz' }) })
    );
    await page.goto(BILLING_URL);
    const [req] = await Promise.all([
      page.waitForRequest((r) => r.url().includes('/api/v1/subscriptions/portal')),
      page.getByTestId('manage-billing-button').click()
    ]);
    expect(req.method()).toBe('POST');
  });

  test('7. billing and members links visible in org subnav for owner', async ({ page }) => {
    await page.goto(`/org/${SEEDED_ORG_SLUG}`);
    await expect(page.getByTestId('subnav-billing-link')).toBeVisible();
    await expect(page.getByTestId('subnav-members-link')).toBeVisible();
  });
});
```

Total: 7 assertions (spec asks for ≥6).

#### Impact on Existing Tests

- `org-notifications.spec.ts`: the new subnav items (`Billing`, `Members`) appear for owners. Existing spec does not assert on negative visibility; **unaffected**.
- `guards.test.ts`: already updated in Step 4.

---

### Step 9: Documentation

**Rationale:** DoD requires README docs for seeding a team-tier org for local dev. Small, last, no code impact.

#### Files to Modify

| File              | Action | Description                                                        |
|-------------------|--------|--------------------------------------------------------------------|
| `web/README.md`   | modify | Add "Seeding a team-tier org" section covering `scripts/seed-test-data.sh` and how to exercise billing/members locally |

---

## Test Impact Summary

| Test File                                 | Test Function                            | Impact | Action Required                     |
|-------------------------------------------|------------------------------------------|--------|-------------------------------------|
| `web/src/lib/server/guards.test.ts`       | existing describe blocks                 | none   | Append `requireOrgOwner` describe   |
| `web/src/lib/api/subscriptions.test.ts`   | (new)                                    | add    | Write 8 table-driven cases          |
| `web/src/lib/api/invitations.test.ts`     | (new)                                    | add    | Write 8 table-driven cases          |
| `web/src/lib/api/members.test.ts`         | (new)                                    | add    | Write 2 table-driven cases          |
| `web/tests/e2e/org-notifications.spec.ts` | all                                      | none   | Sub-nav additions don't conflict    |
| `web/tests/e2e/org-billing.spec.ts`       | (new)                                    | add    | Implement 7 Playwright assertions   |

---

## Risks and Edge Cases

- **Risk: Svelte-check breaks on snake_case types.** The codebase mixes camelCase TS conventions with snake_case wire payloads. Mitigation: follow the existing `NotificationRule` / `NotificationDelivery` precedent which uses snake_case directly on wire types — `svelte-check` already accepts this.
- **Risk: Backend returns `tier` field on Organization only for team (precedent)** — existing `requireTeamTier` treats absent tier as team. Our seeded org has no `tier` field so both guards pass. Mitigation: unchanged; billing page already requires the guard chain.
- **Risk: Portal redirect opens cross-origin.** Mitigation: render as `<a rel="noopener noreferrer">` and navigate via `window.location.href` after receiving `portal_url`. DoD explicitly requires this.
- **Risk: E2E spec order-dependency with shared stack state.** `fullyParallel: false, workers: 1` in `playwright.config.ts` already serialises. Any real DB writes (invitations) would require cleanup. Mitigation: all mutating operations in spec use `context.route` mocks — no writes hit the real backend.
- **Edge case: user has no subscription yet.** Backend returns `{ subscription: null, tier: 'free' }`. Mitigation: page renders an "Upgrade to team" CTA card instead of the subscription card when `data.subscription === null`.
- **Edge case: seat_count > seat_limit (overage).** Mitigation: `SeatsUsageBar` clamps width visually but sets `aria-valuenow={seatCount}`. The PATCH will fail backend-side with `seat_limit_reached`; the modal displays that error.
- **Edge case: proration net = 0 (free seats).** Mitigation: show "No additional charge" inline; still treat as success and close modal on dismiss.
- **Edge case: owner_required toast reset.** Mitigation: extend the dismissal reactive statement to recognise `'owner_required'` — mirrors `'admin_required'`.

---

## Verification

```bash
# Unit/Type checks
cd web
npm run check        # svelte-check
npm run lint         # eslint
npm run test:unit    # vitest

# E2E (observable)
APITOOL_MANAGE_STACK=1 npm run test:e2e -- tests/e2e/org-billing.spec.ts
```

Observable verification (from task YAML):

```bash
cd web && npm run test:e2e -- tests/e2e/org-billing.spec.ts
# Expected: ≥6 Playwright assertions pass
```

Backend guardrails (should be unaffected):

```bash
cd src/ApiTool.Backend.Tests && dotnet test
```
