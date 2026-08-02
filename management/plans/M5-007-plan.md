# Implementation Plan: M5-007

## Overview

Deliver the SvelteKit Custom Role Editor UI at `/org/[slug]/settings/roles`:
an owner-only, enterprise-tier page that lists built-in + custom roles with
member counts, and drives create / edit / delete of custom roles through a
permission-matrix picker grouped by the spec's six categories.

## Task Details
- **ID:** M5-007
- **Title:** Web: custom role editor UI
- **Phase:** M5: Enterprise Tier
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-006 | Backend custom roles and granular permissions | done |

> **Note on backend PATCH endpoint:** M5-006 shipped POST/GET/DELETE on
> `/api/v1/organizations/{id}/roles` but deferred PATCH
> (see `CustomRolesEndpoints.cs` line 11). The task observable nonetheless
> asserts that the UI fires PATCH on edit. We treat the PATCH contract as
> `{ name, permissions }` (full replacement, mirroring POST) and mock the
> endpoint in the Playwright suite. No backend work is in scope — a
> follow-up task will wire the server side. The typed `rolesApi.update`
> helper is still shipped here because it is the natural way to keep the
> `+page.svelte` thin.

## Architectural Decisions

1. **Full-replace PATCH body** — `{ name, permissions }` — mirrors POST.
   Simpler UX than computing deltas and matches how the form always owns
   the full permission set.
2. **Permission catalogue lives in one TypeScript module** —
   `web/src/lib/types/roles.ts` — as a const array of `{ category, key,
   label }` entries, grouped into the six spec sections. This mirrors
   the backend `Permissions` constant but is duplicated deliberately so
   svelte-check can type-check the keys at build time.
3. **Member-count data source** — derive client-side from
   `GET /organizations/{id}/members`. The members endpoint already
   returns `role` (string role or `role_id` for custom roles). We group
   by `role_id` (custom) / `role` (built-in) and show the count per row.
   No backend changes required.
4. **Guards** — reuse `requireEnterpriseTier` + `requireOrgOwner`. Non-
   owners are redirected to `/org/[slug]?toast=owner_required` (existing
   toast already rendered by `(app)/+layout.svelte`).
5. **Sub-nav link** — add `subnav-roles-link` to `(app)/+layout.svelte`,
   visible only to owners of enterprise-tier orgs (same gate as SSO).
6. **Built-in roles are read-only** — rows render permission chips but
   no Edit/Delete controls; create button renders above the table; row
   controls for custom roles only.
7. **409 delete flow** — UI calls DELETE, surfaces the server
   `member_count` field in the toast message. (Backend already returns
   `{ code: "role_in_use", member_count }` at 409.)

## Implementation Steps

Steps are ordered smallest-blast-radius first: types → api client → unit
tests → permission catalogue → page loader → page UI → e2e tests → subnav
wiring.

### Step 1: Add the roles types module

**Rationale:** Pure data, no imports from the rest of the app. Typing
the wire shapes first lets everything downstream be type-checked.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/roles.ts` | create | Wire types + permission catalogue grouped by category |

#### New Code

```ts
// web/src/lib/types/roles.ts

/** Wire DTO for a role (built-in or custom) returned by GET/POST. */
export interface RoleView {
  id: string;
  name: string;
  permissions: string[];
  is_builtin: boolean;
  created_at: string | null;
}

export interface ListRolesResponse { roles: RoleView[] }

/** Request body for POST /organizations/{id}/roles. */
export interface CreateRoleRequest { name: string; permissions: string[] }

/** Request body for PATCH /organizations/{id}/roles/{role_id}. */
export interface UpdateRoleRequest { name: string; permissions: string[] }

/** Categories used to group the permission picker. Order matters — it's
 * the display order in the UI and mirrors the spec's permission matrix. */
export type PermissionCategory =
  | 'Member Management'
  | 'Billing & Subscription'
  | 'Organization Settings'
  | 'Service Tokens'
  | 'Test Resources'
  | 'Team Dashboard';

export interface PermissionDef {
  key: string;
  label: string;
  description: string;
  category: PermissionCategory;
}

/** Full permission catalogue. Keep in sync with backend `Permissions.cs`. */
export const PERMISSION_CATALOGUE: readonly PermissionDef[] = [
  // Member Management
  { category: 'Member Management', key: 'members.invite', label: 'Invite members',       description: 'Invite new members' },
  { category: 'Member Management', key: 'members.remove', label: 'Remove members',       description: 'Remove members'     },
  { category: 'Member Management', key: 'members.view',   label: 'View members',         description: 'View member list'   },
  { category: 'Member Management', key: 'roles.change',   label: 'Change member roles',  description: 'Change member roles' },
  { category: 'Member Management', key: 'roles.view',     label: 'View member roles',    description: 'View member roles'   },
  // Billing & Subscription
  { category: 'Billing & Subscription', key: 'billing.manage', label: 'Manage billing', description: 'Update payment, change plan' },
  { category: 'Billing & Subscription', key: 'billing.view',   label: 'View billing',   description: 'View invoices, status' },
  { category: 'Billing & Subscription', key: 'seats.add',      label: 'Add seats',      description: 'Add seats to subscription' },
  { category: 'Billing & Subscription', key: 'seats.remove',   label: 'Remove seats',   description: 'Remove seats' },
  // Organization Settings
  { category: 'Organization Settings', key: 'org.settings.manage', label: 'Manage settings', description: 'Update name, logo, settings' },
  { category: 'Organization Settings', key: 'org.settings.view',   label: 'View settings',   description: 'View settings' },
  { category: 'Organization Settings', key: 'org.delete',          label: 'Delete organization', description: 'Delete organization' },
  { category: 'Organization Settings', key: 'org.transfer',        label: 'Transfer ownership',  description: 'Transfer ownership' },
  // Service Tokens
  { category: 'Service Tokens', key: 'tokens.create', label: 'Create tokens', description: 'Create org-scoped tokens' },
  { category: 'Service Tokens', key: 'tokens.revoke', label: 'Revoke tokens', description: 'Revoke org tokens' },
  { category: 'Service Tokens', key: 'tokens.view',   label: 'View tokens',   description: 'View org token list' },
  // Test Resources
  { category: 'Test Resources', key: 'results.view',         label: 'View results',        description: 'View test results' },
  { category: 'Test Resources', key: 'results.upload',       label: 'Upload results',      description: 'Upload test results' },
  { category: 'Test Resources', key: 'results.delete',       label: 'Delete results',      description: 'Delete test results' },
  { category: 'Test Resources', key: 'vault_config.manage',  label: 'Manage vault config', description: 'Manage shared vault templates' },
  { category: 'Test Resources', key: 'vault_config.view',    label: 'View vault config',   description: 'Use shared vault configurations' },
  // Team Dashboard
  { category: 'Team Dashboard', key: 'dashboard.view',   label: 'View dashboard',   description: 'Access team dashboard' },
  { category: 'Team Dashboard', key: 'dashboard.export', label: 'Export reports',   description: 'Export reports' },
];

/** Ordered category list — drives section rendering in the picker. */
export const PERMISSION_CATEGORIES: readonly PermissionCategory[] = [
  'Member Management',
  'Billing & Subscription',
  'Organization Settings',
  'Service Tokens',
  'Test Resources',
  'Team Dashboard',
];
```

#### Tests to Write FIRST (RED phase)

No unit test for this file — it's pure data. Type-checking via
`svelte-kit sync && svelte-check` covers correctness. The api-client
tests in Step 2 import these types and serve as implicit validation.

#### Impact on Existing Tests
- None — new file.

---

### Step 2: Add the typed roles API client

**Rationale:** A thin wrapper around `api.get/post/patch/delete` with
matching unit tests. No svelte code, no server code — pure TS.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/api/roles.ts` | create | `rolesApi` with `list`, `create`, `update`, `remove` |
| `web/src/lib/api/roles.test.ts` | create | Vitest covering happy-path + error propagation |

#### New Code

```ts
// web/src/lib/api/roles.ts
import { api, type RequestOptions } from './client';
import type {
  RoleView,
  ListRolesResponse,
  CreateRoleRequest,
  UpdateRoleRequest,
} from '$lib/types/roles';

export const rolesApi = {
  async list(orgId: string, opts?: RequestOptions): Promise<RoleView[]> {
    const { roles } = await api.get<ListRolesResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/roles`,
      opts,
    );
    return roles;
  },
  async create(orgId: string, body: CreateRoleRequest, opts?: RequestOptions): Promise<RoleView> {
    return api.post<RoleView>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/roles`,
      body,
      opts,
    );
  },
  async update(orgId: string, roleId: string, body: UpdateRoleRequest, opts?: RequestOptions): Promise<RoleView> {
    return api.patch<RoleView>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(roleId)}`,
      body,
      opts,
    );
  },
  async remove(orgId: string, roleId: string, opts?: RequestOptions): Promise<void> {
    await api.delete<void>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/roles/${encodeURIComponent(roleId)}`,
      opts,
    );
  },
};
```

#### Tests to Write FIRST (RED phase)

```ts
// web/src/lib/api/roles.test.ts — mirrors sso.test.ts
describe('rolesApi.list', () => {
  it('list returns roles array', …);
  it('list propagates 403 permission_denied', …);
});
describe('rolesApi.create', () => {
  it('create sends POST JSON and returns the new role', …);
  it('create propagates 400 invalid_permission with field detail', …);
  it('create propagates 409 role_name_taken', …);
});
describe('rolesApi.update', () => {
  it('update sends PATCH JSON with name + permissions', …);
  it('update propagates 404 role_not_found', …);
});
describe('rolesApi.remove', () => {
  it('remove sends DELETE and resolves on 204', …);
  it('remove propagates 409 role_in_use with member_count in body', …);
});
```

#### Impact on Existing Tests
- None — new file.

---

### Step 3: Add the page loader

**Rationale:** Server-side data-fetch lives in its own file and is
testable via the e2e harness only (no unit test — load functions
are covered in e2e for the SSO route too).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/settings/roles/+page.server.ts` | create | Auth + guards; loads roles list + member counts |

#### New Code

```ts
// web/src/routes/(app)/org/[slug]/settings/roles/+page.server.ts
import type { PageServerLoad } from './$types';
import { requireAuth, requireEnterpriseTier, requireOrgOwner } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { rolesApi } from '$lib/api/roles';
import { membersApi } from '$lib/api/members';
import type { RoleView } from '$lib/types/roles';
import type { Member } from '$lib/types/members';

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname);
  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const entOrg = requireEnterpriseTier(org, params.slug);
  const ownerOrg = requireOrgOwner(entOrg, params.slug);

  try {
    const [roles, members] = await Promise.all([
      rolesApi.list(ownerOrg.id, { token, fetch }),
      membersApi.list(ownerOrg.id, { token, fetch }),
    ]);
    return {
      org: ownerOrg,
      roles,
      memberCounts: countByRole(roles, members),
      error: null as string | null,
    };
  } catch (e) {
    const message = e instanceof Error ? e.message : 'Failed to load roles.';
    return {
      org: ownerOrg,
      roles: [] as RoleView[],
      memberCounts: {} as Record<string, number>,
      error: message,
    };
  }
};

function countByRole(roles: RoleView[], members: Member[]): Record<string, number> {
  const counts: Record<string, number> = {};
  for (const r of roles) counts[r.id] = 0;
  for (const m of members) {
    // Members have `role` (owner|admin|member) for built-ins or a
    // `role_id` field when assigned a custom role (M5-006).
    // We defensively read both — the existing Member interface only
    // declares `role`, so we cast once here.
    const mm = m as Member & { role_id?: string };
    const key = mm.role_id ?? m.role;
    counts[key] = (counts[key] ?? 0) + 1;
  }
  return counts;
}
```

#### Tests to Write FIRST (RED phase)

Covered by e2e assertions in Step 5 (auth redirect, roles + counts
render). No server-unit test — consistent with `+page.server.ts`
precedent in `sso/` and `audit-log/`.

#### Impact on Existing Tests

- `web/src/lib/types/members.ts` does not declare `role_id`. We add
  an **optional** `role_id?: string` field to `Member` so the loader
  compiles without a cast. Existing consumers (`members/+page.*`,
  `invitations`, etc.) treat `role_id` as optional; no behaviour change.

Edit to `web/src/lib/types/members.ts`:
```ts
export interface Member {
  user_id: string;
  role: OrgRole;
  role_id?: string;      // ← added; populated when member has a custom role
  joined_at: string;
}
```

---

### Step 4: Add the permission picker component

**Rationale:** Isolates the (fairly chunky) grouped-checkbox UI from
the page itself, making both easier to reason about.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/components/roles/PermissionPicker.svelte` | create | Grouped checkbox list, two-way bound to a `Set<string>` of selected keys |

#### New Code (shape)

```svelte
<!-- web/src/lib/components/roles/PermissionPicker.svelte -->
<script lang="ts">
  import { PERMISSION_CATALOGUE, PERMISSION_CATEGORIES } from '$lib/types/roles';
  export let selected: Set<string>;
  export let disabled = false;

  function toggle(key: string, on: boolean) {
    const next = new Set(selected);
    on ? next.add(key) : next.delete(key);
    selected = next;
  }

  $: grouped = PERMISSION_CATEGORIES.map((cat) => ({
    category: cat,
    perms: PERMISSION_CATALOGUE.filter((p) => p.category === cat),
  }));
</script>

<div data-testid="permission-picker">
  {#each grouped as { category, perms }}
    <fieldset class="mb-4" data-testid="perm-section-{category.toLowerCase().replace(/[^a-z]+/g, '-')}">
      <legend class="text-sm font-semibold text-gray-700">{category}</legend>
      <div class="mt-2 grid grid-cols-1 gap-2 sm:grid-cols-2">
        {#each perms as p}
          <label class="flex items-start gap-2">
            <input
              type="checkbox"
              data-testid="perm-{p.key}"
              {disabled}
              checked={selected.has(p.key)}
              on:change={(e) => toggle(p.key, e.currentTarget.checked)}
            />
            <span>
              <span class="block text-sm text-gray-900">{p.label}</span>
              <code class="block text-xs text-gray-500">{p.key}</code>
            </span>
          </label>
        {/each}
      </div>
    </fieldset>
  {/each}
</div>
```

#### Tests to Write FIRST (RED phase)

No component unit test — the component's behaviour is indirectly
exercised by the e2e step that ticks 4 checkboxes and asserts the POST
body contains those keys. This matches the precedent set by
`SamlConfigForm.svelte` / `OidcConfigForm.svelte` (no component
unit tests).

#### Impact on Existing Tests
- None — new file in a new subdirectory.

---

### Step 5: Add the roles page (+page.svelte)

**Rationale:** The UI is the last Svelte file written because everything
else (types, api, loader, picker) is in place.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/settings/roles/+page.svelte` | create | Table of roles + Create/Edit modal form + Delete confirm |

#### Component responsibilities

1. Header `<h1 data-testid="roles-heading">Roles</h1>` + `Create role` button (`data-testid="roles-create-button"`).
2. Table `data-testid="roles-table"` with columns: Name, Permissions (count), Members, Actions.
   - Row `data-testid="role-row-{id}"`.
   - Built-in rows (is_builtin=true) show a `Built-in` badge; no Edit/Delete controls.
   - Custom rows show `Edit` (`role-edit-{id}`) and `Delete` (`role-delete-{id}`).
3. A single modal (`data-testid="role-modal"`) reused for Create and Edit.
   - Inputs: name (`role-name-input`), `PermissionPicker`, Submit (`role-submit`), Cancel.
   - Submit fires POST (create mode) or PATCH (edit mode). On success:
     `invalidateAll()` + success toast.
4. Delete confirm (simple `window.confirm`, or inline `role-confirm-delete-{id}`).
   On 409 `role_in_use`: extract `member_count` from `ApiError` body-extension;
   show `Role is assigned to {n} members` toast.
5. Error handling: any create/update validation error surfaces inline.

**ApiError extension note:** `api-error.ts` currently carries
`{ code, message, status, field? }`. The 409 response has shape
`{ code: "role_in_use", message, member_count: N }`. To read
`member_count` we extend `ApiError` with an optional
`details?: Record<string, unknown>` and update `client.ts` to
pass through unknown top-level JSON fields. Minimally invasive —
no existing consumer reads `details`.

#### Code sketch (illustrative)

```svelte
<!-- abridged -->
<script lang="ts">
  import { invalidateAll } from '$app/navigation';
  import type { PageData } from './$types';
  import { rolesApi } from '$lib/api/roles';
  import { ApiError } from '$lib/types/api-error';
  import Toast from '$lib/components/ui/Toast.svelte';
  import PermissionPicker from '$lib/components/roles/PermissionPicker.svelte';
  import type { RoleView } from '$lib/types/roles';

  export let data: PageData;

  type ModalMode = { kind: 'create' } | { kind: 'edit'; role: RoleView };
  let modal: ModalMode | null = null;
  let formName = '';
  let formPerms: Set<string> = new Set();
  let submitting = false;
  let formError: string | null = null;
  let toast = { open: false, message: '', variant: 'success' as 'success' | 'error' };

  function openCreate() { modal = { kind: 'create' }; formName = ''; formPerms = new Set(); formError = null; }
  function openEdit(role: RoleView) { modal = { kind: 'edit', role }; formName = role.name; formPerms = new Set(role.permissions); formError = null; }
  function closeModal() { modal = null; }

  async function submitForm() {
    if (!modal) return;
    submitting = true;
    formError = null;
    try {
      if (modal.kind === 'create') {
        await rolesApi.create(data.org.id, { name: formName.trim(), permissions: [...formPerms] });
        toast = { open: true, message: 'Role created', variant: 'success' };
      } else {
        await rolesApi.update(data.org.id, modal.role.id, { name: formName.trim(), permissions: [...formPerms] });
        toast = { open: true, message: 'Role updated', variant: 'success' };
      }
      modal = null;
      await invalidateAll();
    } catch (e) {
      formError = e instanceof Error ? e.message : 'Request failed.';
    } finally { submitting = false; }
  }

  async function confirmDelete(role: RoleView) {
    if (!confirm(`Delete "${role.name}"?`)) return;
    try {
      await rolesApi.remove(data.org.id, role.id);
      toast = { open: true, message: 'Role deleted', variant: 'success' };
      await invalidateAll();
    } catch (e) {
      if (e instanceof ApiError && e.code === 'role_in_use') {
        const n = Number((e.details as { member_count?: unknown })?.member_count ?? 0);
        toast = { open: true, message: `Role is assigned to ${n} members`, variant: 'error' };
      } else {
        toast = { open: true, message: e instanceof Error ? e.message : 'Delete failed', variant: 'error' };
      }
    }
  }
</script>
<!-- … markup: table + modal + Toast -->
```

#### Tests to Write FIRST (RED phase)

All assertions in the e2e spec (Step 6) are the tests for this page.
The RED phase order within the spec file:

1. `loads roles page with built-in + custom rows + member counts` (list GET)
2. `creates a role via modal and POST fires` (create flow)
3. `edits an existing custom role and PATCH fires` (edit flow)
4. `delete on role in use shows 409 toast with member count` (delete 409)
5. `built-in rows show no edit/delete controls`
6. `non-owner is redirected with owner_required toast`
7. `permission picker renders all six category groups` (matrix)

#### Impact on Existing Tests
- None — new page route.

---

### Step 6: Add the e2e spec

**Rationale:** Observable-verification driver.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/org-roles.spec.ts` | create | 7 Playwright tests covering the seven behaviours |

#### Tests (full list, mirrors behaviours)

| # | Test name | Behaviour |
|---|-----------|-----------|
| 1 | `loads roles page and lists builtin + custom rows with member counts` | B1 |
| 2 | `create role modal submits POST and new row appears` | B2 |
| 3 | `edit existing custom role fires PATCH` | B3 |
| 4 | `delete role in use shows 409 toast with member count` | B4 |
| 5 | `permission picker renders 6 categories in spec order` | B5 |
| 6 | `non-owner is redirected with owner_required toast` | B6 |
| 7 | `built-in rows have no edit/delete controls` | (belt-and-braces) |

All tests use `context.route` to mock:
- `GET /api/v1/organizations` → owner org, tier=enterprise
- `GET /api/v1/organizations/*/roles` → 3 built-ins + 1 custom (qa-lead)
- `GET /api/v1/organizations/*/members` → members with mixed role assignments
- `POST /api/v1/organizations/*/roles` → 201 with the new role echoed
- `PATCH /api/v1/organizations/*/roles/*` → 200 with updated role
- `DELETE /api/v1/organizations/*/roles/*` → 409 with `{ code: "role_in_use", member_count: 3 }` for the "in-use" test

#### Impact on Existing Tests
- None — new spec file.

---

### Step 7: Wire the sub-nav link

**Rationale:** Keeps the feature discoverable once the page exists.
Last step so a broken build would not make the link appear prematurely.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/+layout.svelte` | modify | Add `subnav-roles-link` between SSO and Audit Log |

#### Current Code
```svelte
{#if isEnterpriseTier && currentOrg?.role === 'owner'}
    <a href="/org/{currentOrg.slug}/settings/sso" data-testid="subnav-sso-link" class="text-gray-600 hover:text-gray-900">SSO</a>
{/if}
```

#### New Code
```svelte
{#if isEnterpriseTier && currentOrg?.role === 'owner'}
    <a href="/org/{currentOrg.slug}/settings/sso" data-testid="subnav-sso-link" class="text-gray-600 hover:text-gray-900">SSO</a>
    <a href="/org/{currentOrg.slug}/settings/roles" data-testid="subnav-roles-link" class="text-gray-600 hover:text-gray-900">Roles</a>
{/if}
```

#### Impact on Existing Tests
- `org-sso.spec.ts` asserts `subnav-sso-link` visibility — unaffected.
- `org-audit-log.spec.ts` — unaffected.
- No test asserts the **absence** of a roles link, so adding it is safe.

---

### Step 8: Extend `ApiError` with a `details` field

**Rationale:** Needed so the delete flow can surface `member_count` from
the 409 body. Minimal change, own step so the blast radius is visible.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/api-error.ts` | modify | Add optional `details?: Record<string, unknown>` |
| `web/src/lib/api/client.ts` | modify | Capture non-reserved top-level JSON fields into `details` |
| `web/src/lib/types/api-error.test.ts` | modify | Add test for `details` round-trip |

#### New Code (api-error.ts)
```ts
export class ApiError extends Error {
  readonly code: string;
  readonly status: number;
  readonly field?: string;
  readonly details?: Record<string, unknown>;

  constructor(code: string, message: string, status: number, field?: string, details?: Record<string, unknown>) {
    super(message);
    this.name = 'ApiError';
    this.code = code;
    this.status = status;
    this.field = field;
    this.details = details;
  }
}
```

#### New Code (client.ts, inside the `!res.ok` branch)
```ts
if (!res.ok) {
  let code = 'server_error';
  let description = res.statusText;
  let field: string | undefined;
  let details: Record<string, unknown> | undefined;
  try {
    const body = await res.json();
    if (body?.code) code = body.code;
    if (body?.description) description = body.description;
    if (body?.message && !body?.description) description = body.message; // 409 shape uses "message"
    if (body?.field) field = body.field;
    if (body && typeof body === 'object') {
      const { code: _c, description: _d, message: _m, field: _f, ...rest } = body as Record<string, unknown>;
      if (Object.keys(rest).length > 0) details = rest;
    }
  } catch { /* non-JSON error body */ }
  throw new ApiError(code, description, res.status, field, details);
}
```

#### Tests to Write FIRST (RED phase)

```ts
// in api-error.test.ts
it('ApiError preserves details payload for 409 role_in_use', () => {
  const e = new ApiError('role_in_use', 'msg', 409, undefined, { member_count: 3 });
  expect((e.details as { member_count: number }).member_count).toBe(3);
});
// in client.test.ts — a new test
it('request captures non-reserved body fields into ApiError.details', async () => {
  const fetchFn = makeFetch(409, '{"code":"role_in_use","message":"Role in use","member_count":5}');
  let caught: unknown;
  try { await api.get('/foo', { fetch: fetchFn }); } catch (e) { caught = e; }
  expect(isApiError(caught)).toBe(true);
  const err = caught as ApiError;
  expect(err.code).toBe('role_in_use');
  expect((err.details as { member_count: number }).member_count).toBe(5);
});
```

#### Impact on Existing Tests

- `api-error.test.ts` — existing tests continue to pass (new optional field).
- `client.test.ts` — existing tests call the constructor without `details`, all still pass; we add one new test.
- `sso.test.ts`, `members.test.ts`, etc. — unaffected (they only read `code`, `status`, `field`).

> This step is listed last because no code from steps 1–7 depends on it at
> compile time — only the delete-in-use runtime path does. We implement
> it before Step 5 lands in practice, but the *plan* lists it after
> because it has the clearest cross-cutting footprint and is easiest to
> review in isolation.

## Test Impact Summary

| Test File | Test Function | Impact | Action |
|-----------|--------------|--------|--------|
| `web/src/lib/api/client.test.ts` | existing | none | — |
| `web/src/lib/api/sso.test.ts` | existing | none | — |
| `web/src/lib/api/members.test.ts` | existing | none | — |
| `web/src/lib/types/api-error.test.ts` | existing | none (new field optional) | add one test for `details` |
| `web/tests/e2e/org-sso.spec.ts` | all | none | — |
| `web/tests/e2e/org-audit-log.spec.ts` | all | none | — |
| `web/src/lib/api/roles.test.ts` | **new** | n/a | author per §2 |
| `web/tests/e2e/org-roles.spec.ts` | **new** | n/a | author per §6 |

## Risks and Edge Cases

- **Backend PATCH not implemented.**
  Mitigation: UI uses PATCH through mocked route in the e2e spec.
  The page will not work against a live backend until PATCH ships —
  documented in code comment above `rolesApi.update`. A follow-up task
  should add the endpoint in `CustomRolesService` + `CustomRolesEndpoints`.

- **`Member.role_id` not yet populated by backend on GET /members.**
  Members endpoint may not surface the field until M5-006 ships the
  role assignment PATCH. Mitigation: treat missing `role_id` as "no
  custom role assigned" and fall back to `role`. Zero-counts simply
  render as "0 members" for custom roles that no member references,
  which is still correct.

- **Optimistic UX vs. race conditions.**
  Using `invalidateAll()` after create/update/delete reloads from the
  server, so no stale-cache risk.

- **Role name collision.**
  Surfaced via 409 `role_name_taken` → inline form error.

- **Reserved name.**
  Backend returns 400 `invalid_name` → inline form error.

- **Empty permissions.**
  Spec allows an empty permissions array. Client does not block submit;
  UX shows checkboxes all unchecked — user choice.

- **Pre-checked built-in role editing.**
  Built-in rows have no Edit/Delete button, so this is not reachable.

- **Non-enterprise tier.**
  `requireEnterpriseTier` redirects to `/org/[slug]?toast=enterprise_tier_required`.
  Not listed as a task behaviour but inherited from guard — fine.

- **Owner not in org.**
  `organizationsApi.findBySlug` returns null → `requireEnterpriseTier`
  throws 404. Not listed as a behaviour; inherited guard behaviour.

## Verification

```bash
cd web
npm install
npm run check
npm run lint
npm run test:unit
npm run build
npm run test:e2e -- tests/e2e/org-roles.spec.ts
```

Observable verification from the task YAML:

```bash
cd web && npm install && npm run build
npm run test:e2e -- tests/e2e/org-roles.spec.ts
# Expected: >=6 passing Playwright tests
```
