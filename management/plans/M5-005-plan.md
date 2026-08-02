# Implementation Plan: M5-005

## Overview
Add a SvelteKit web route `/org/[slug]/audit-log` that lets enterprise-tier admins view,
filter, paginate, and CSV-export the organization audit log produced by the M5-004
backend middleware. Ships a typed API client, a page loader with guard chain, a
table with pagination + filter controls, a CSV export endpoint, and a Playwright
E2E spec covering the observable scenarios.

## Task Details
- **ID:** M5-005
- **Title:** Web: audit log viewer page
- **Phase:** M5: Enterprise Tier
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M5-004 | Backend: audit log capture middleware | done |

The backend already exposes:
- `GET /api/v1/organizations/{id}/audit-log` with query params
  `event_type`, `user_id`, `from`, `to`, `limit`, `format` (json\|csv).
- Admin/owner only (`permission_denied` on 403 for members).
- JSON response shape: `{ items: AuditLogEntryDto[] }` where `AuditLogEntryDto = { event_type, user_id, target_type, target_id, created_at, ip_address, success, failure_reason }`.
- CSV format (`format=csv`) returns `text/csv` with its own header set. Because the task's
  observable requires a specific header row (`event_type,user,target,timestamp,ip`), the
  SvelteKit export endpoint will call the **JSON** endpoint and re-format in-web,
  keeping the backend API stable and the web-layer presentation concerns co-located.

## Architectural Decisions

1. **Guard chain:** `requireAuth` → `requireEnterpriseTier` → `requireOrgAdmin`.
   The first behavior's "Admin access required" toast and the DoD "admins of
   enterprise-tier orgs" pin down enterprise + admin-or-owner. A non-admin is
   redirected to `/org/[slug]?toast=admin_required` (reusing the existing
   `toast-admin-required` banner).
2. **Pagination:** Fetched server-side with `limit=100` (backend max=200),
   paginated client-side into 10-row pages via `?page=N`. The backend
   `AuditLogQueryService` exposes only `limit` today — adding offset is
   out of scope for this web-only slice and a future optimisation ticket.
3. **Date ranges:** Rendered as a small picker with presets (`last 24h`, `7d`, `30d`,
   `all`). Picking a preset sets `?from=<iso>` (and for `all`, clears it). A `to`
   param is supported but not surfaced in the preset picker; the loader forwards
   it if present so future UI can add a custom range.
4. **Event-type filter:** Hardcoded dropdown of known event types from M5-004
   (`member.invited`, `member.removed`, `member.role_changed`, `org.settings.updated`,
   `sso.login`, `sso.login_failed`, `subscription.updated`) plus `all`. This
   matches the observable's `member.invited` selection. Raw typing is not required.
5. **CSV export:** A SvelteKit server endpoint
   `/org/[slug]/audit-log/export/+server.ts` that calls the backend JSON endpoint
   (forwarding current filters), re-formats into CSV with the header
   `event_type,user,target,timestamp,ip`, and streams back with
   `Content-Type: text/csv` and `Content-Disposition: attachment; filename=audit-log-<slug>-<YYYYMMDD>.csv`.
6. **Sub-nav link:** Added to the `(app)/+layout.svelte` as `audit-log-link`,
   shown for enterprise-tier admin or owner (same RBAC as the page itself).

## Implementation Steps

The order is chosen by **blast radius (smallest first)**: pure types → pure formatter →
pure API client → loader → CSV endpoint → UI → layout link → E2E. Each step is
independently testable and leaves the tree green.

### Step 1: Audit log types and CSV formatter (pure modules)
**Rationale:** Zero coupling to other modules; unit-tested with Vitest in isolation.
Everything else builds on these.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/audit-log.ts` | create | TS types mirroring the backend DTO and filter. |
| `web/src/lib/audit-log/csv.ts` | create | Pure formatter that turns `AuditLogEntry[]` into the CSV string with the required header and RFC 4180 quoting. |
| `web/src/lib/audit-log/csv.test.ts` | create | Table-driven tests for the formatter. |

#### New Code (`web/src/lib/types/audit-log.ts`)

```ts
/** Known organization audit-log event types emitted by the M5-004 middleware. */
export type AuditEventType =
  | 'member.invited'
  | 'member.removed'
  | 'member.role_changed'
  | 'org.settings.updated'
  | 'sso.login'
  | 'sso.login_failed'
  | 'subscription.updated';

/** JSON entry returned by GET /organizations/{id}/audit-log. */
export interface AuditLogEntry {
  event_type: string;
  user_id: string | null;
  target_type: string | null;
  target_id: string | null;
  created_at: string; // ISO 8601 UTC
  ip_address: string | null;
  success: boolean;
  failure_reason: string | null;
}

/** Loader-side filter parsed from URL search params. */
export interface AuditLogFilter {
  event_type: string | null;
  from: string | null; // ISO 8601
  to: string | null;   // ISO 8601
  page: number;        // 1-based
}

/** Preset date-range keys exposed by the UI picker. */
export type AuditRangePreset = '24h' | '7d' | '30d' | 'all';

/** Page size for client-side pagination of audit log rows. */
export const AUDIT_LOG_PAGE_SIZE = 10;

/** Event types shown in the filter dropdown, plus an 'all' sentinel. */
export const AUDIT_EVENT_TYPES: readonly AuditEventType[] = [
  'member.invited',
  'member.removed',
  'member.role_changed',
  'org.settings.updated',
  'sso.login',
  'sso.login_failed',
  'subscription.updated'
] as const;
```

#### New Code (`web/src/lib/audit-log/csv.ts`)

```ts
import type { AuditLogEntry } from '$lib/types/audit-log';

const CSV_HEADER = 'event_type,user,target,timestamp,ip';

/**
 * Formats a list of audit-log entries as CSV matching the M5-005 observable header.
 * Uses RFC 4180 quoting: wraps fields in double-quotes if they contain comma,
 * double-quote, newline, or carriage return; escapes internal quotes by doubling.
 * Fields starting with =, +, -, or @ are prefixed with a single quote to
 * neutralise spreadsheet formula injection.
 */
export function formatAuditLogCsv(entries: readonly AuditLogEntry[]): string {
  const lines = [CSV_HEADER];
  for (const e of entries) {
    const target = [e.target_type, e.target_id].filter(Boolean).join(':');
    lines.push(
      [
        quote(e.event_type),
        quote(e.user_id ?? ''),
        quote(target),
        quote(e.created_at),
        quote(e.ip_address ?? '')
      ].join(',')
    );
  }
  return lines.join('\r\n') + '\r\n';
}

function quote(field: string): string {
  let f = field;
  if (f.length > 0 && (f[0] === '=' || f[0] === '+' || f[0] === '-' || f[0] === '@')) {
    f = "'" + f;
  }
  if (f.includes(',') || f.includes('"') || f.includes('\n') || f.includes('\r')) {
    return '"' + f.replace(/"/g, '""') + '"';
  }
  return f;
}
```

#### Tests to Write FIRST (RED phase) — `web/src/lib/audit-log/csv.test.ts`

```ts
import { describe, it, expect } from 'vitest';
import { formatAuditLogCsv } from './csv';
import type { AuditLogEntry } from '$lib/types/audit-log';

function entry(partial: Partial<AuditLogEntry> = {}): AuditLogEntry {
  return {
    event_type: 'member.invited',
    user_id: 'aaaa1111',
    target_type: 'invitation',
    target_id: 'inv_1',
    created_at: '2026-04-18T10:00:00Z',
    ip_address: '10.0.0.1',
    success: true,
    failure_reason: null,
    ...partial
  };
}

describe('formatAuditLogCsv', () => {
  const cases = [
    { name: 'header only when no entries', entries: [], expectLines: 1 },
    { name: 'single entry produces header + one row', entries: [entry()], expectLines: 2 },
    { name: 'header matches observable spec', entries: [], expectHeader: 'event_type,user,target,timestamp,ip' },
    { name: 'combines target_type and target_id with colon', entries: [entry()], expectSubstring: 'invitation:inv_1' },
    { name: 'empty target when both null', entries: [entry({ target_type: null, target_id: null })], expectSubstring: ',,' },
    { name: 'quotes fields containing comma', entries: [entry({ ip_address: '1.2,3.4' })], expectSubstring: '"1.2,3.4"' },
    { name: 'escapes internal double quotes', entries: [entry({ event_type: 'a"b' })], expectSubstring: '"a""b"' },
    { name: 'neutralises formula-injection prefix', entries: [entry({ event_type: '=1+1' })], expectSubstring: "'=1+1" },
    { name: 'quotes fields with newlines', entries: [entry({ ip_address: 'a\nb' })], expectSubstring: '"a\nb"' },
    { name: 'null user_id renders as empty column', entries: [entry({ user_id: null })], expectSubstring: 'member.invited,,invitation:inv_1' }
  ];

  for (const c of cases) {
    it(c.name, () => {
      const csv = formatAuditLogCsv(c.entries);
      if (c.expectLines !== undefined) {
        expect(csv.trimEnd().split('\r\n')).toHaveLength(c.expectLines);
      }
      if (c.expectHeader) {
        expect(csv.split('\r\n')[0]).toBe(c.expectHeader);
      }
      if (c.expectSubstring) {
        expect(csv).toContain(c.expectSubstring);
      }
    });
  }
});
```

#### Impact on Existing Tests
- No existing tests affected — purely additive.

---

### Step 2: `auditLogApi` client
**Rationale:** Mirrors every other `$lib/api/*` module; depends only on Step 1
types and the existing `client.ts`. Keeps HTTP concerns in one spot.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/api/audit-log.ts` | create | `auditLogApi.list(orgId, opts)` typed client. |
| `web/src/lib/api/audit-log.test.ts` | create | Table-driven Vitest client tests. |

#### New Code

```ts
import { api, type RequestOptions } from './client';
import type { AuditLogEntry } from '$lib/types/audit-log';

interface ListAuditLogResponse {
  items: AuditLogEntry[];
}

/** Typed API client for GET /organizations/{id}/audit-log. */
export const auditLogApi = {
  /**
   * Lists audit-log entries for an organisation, newest-first.
   * Admin/owner only — raises 403 permission_denied otherwise.
   * Backend clamps limit to [1, 200].
   */
  async list(
    orgId: string,
    opts: RequestOptions & {
      event_type?: string;
      from?: string;
      to?: string;
      limit?: number;
    } = {}
  ): Promise<AuditLogEntry[]> {
    const { event_type, from, to, limit, ...rest } = opts;
    const params: Record<string, string | number> = {};
    if (event_type) params.event_type = event_type;
    if (from) params.from = from;
    if (to) params.to = to;
    if (limit !== undefined) params.limit = limit;

    const { items } = await api.get<ListAuditLogResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/audit-log`,
      { ...rest, params: { ...rest.params, ...params } }
    );
    return items;
  }
};
```

#### Tests to Write FIRST (RED phase)

```ts
import { describe, it, expect, vi } from 'vitest';
import { auditLogApi } from './audit-log';
import type { AuditLogEntry } from '$lib/types/audit-log';

function fetchOk(body: unknown): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: true,
    status: 200,
    json: () => Promise.resolve(body)
  }) as unknown as typeof fetch;
}
function fetchErr(status: number, code: string): typeof fetch {
  return vi.fn().mockResolvedValue({
    ok: false,
    status,
    statusText: code,
    json: () => Promise.resolve({ code, description: code })
  }) as unknown as typeof fetch;
}

describe('auditLogApi.list', () => {
  const cases = [
    { name: 'returns items array', items: [{ event_type: 'member.invited' } as Partial<AuditLogEntry>], expected: 1 },
    { name: 'returns empty list', items: [], expected: 0 },
    { name: 'forwards event_type param', items: [], filter: { event_type: 'sso.login' }, expectParam: 'event_type=sso.login' },
    { name: 'forwards from/to params', items: [], filter: { from: '2026-04-01T00:00:00Z', to: '2026-04-30T00:00:00Z' }, expectParam: 'from=2026-04-01' },
    { name: 'forwards limit clamp', items: [], filter: { limit: 25 }, expectParam: 'limit=25' },
    { name: 'propagates 403 permission_denied', fetchMaker: () => fetchErr(403, 'permission_denied'), expectReject: 'permission_denied' }
  ];

  for (const c of cases) {
    it(c.name, async () => {
      const fetchFn = c.fetchMaker ? c.fetchMaker() : fetchOk({ items: c.items });
      const call = auditLogApi.list('org_test', { fetch: fetchFn, ...(c.filter ?? {}) });

      if (c.expectReject) {
        await expect(call).rejects.toMatchObject({ code: c.expectReject });
        return;
      }

      const got = await call;
      expect(got).toHaveLength(c.expected ?? 0);
      if (c.expectParam) {
        const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
        expect(url).toContain(c.expectParam);
      }
    });
  }
});
```

#### Impact on Existing Tests
- None — new module.

---

### Step 3: Page loader with guard chain + filter parsing
**Rationale:** Depends on Steps 1–2. Produces the data contract consumed by the
Svelte component. Covered by the existing `routes-tests/page-server.test.ts`
harness pattern.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/audit-log/+page.server.ts` | create | Loader applying guard chain, parsing query params, calling `auditLogApi.list`, computing paginated slice. |
| `web/src/routes-tests/page-server.test.ts` | modify | Add an `audit-log page loader` describe block with eight table-driven cases (see below). |

#### New Code (`+page.server.ts`)

```ts
import type { PageServerLoad } from './$types';
import { requireAuth, requireEnterpriseTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { auditLogApi } from '$lib/api/audit-log';
import { AUDIT_LOG_PAGE_SIZE, type AuditLogEntry } from '$lib/types/audit-log';

const FETCH_LIMIT = 100;

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname + url.search);
  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const entOrg = requireEnterpriseTier(org, params.slug);
  const adminOrg = requireOrgAdmin(entOrg, params.slug);

  const event_type = url.searchParams.get('event_type') ?? null;
  const from = url.searchParams.get('from') ?? null;
  const to = url.searchParams.get('to') ?? null;
  const page = Math.max(1, Number(url.searchParams.get('page') ?? '1') || 1);

  try {
    const all = await auditLogApi.list(adminOrg.id, {
      token,
      fetch,
      limit: FETCH_LIMIT,
      event_type: event_type ?? undefined,
      from: from ?? undefined,
      to: to ?? undefined
    });
    const totalPages = Math.max(1, Math.ceil(all.length / AUDIT_LOG_PAGE_SIZE));
    const clamped = Math.min(page, totalPages);
    const start = (clamped - 1) * AUDIT_LOG_PAGE_SIZE;
    const pageRows: AuditLogEntry[] = all.slice(start, start + AUDIT_LOG_PAGE_SIZE);
    return {
      org: adminOrg,
      entries: pageRows,
      totalCount: all.length,
      page: clamped,
      totalPages,
      filter: { event_type, from, to },
      error: null as string | null
    };
  } catch (e) {
    const message = e instanceof Error ? e.message : 'Failed to load audit log.';
    return {
      org: adminOrg,
      entries: [] as AuditLogEntry[],
      totalCount: 0,
      page: 1,
      totalPages: 1,
      filter: { event_type, from, to },
      error: message
    };
  }
};
```

#### Tests to Write FIRST (RED phase) — append to `routes-tests/page-server.test.ts`

```ts
// ── Audit log page loader tests ─────────────────────────────────────────────

vi.mock('$lib/api/audit-log', () => ({
  auditLogApi: { list: vi.fn() }
}));
import { load as auditLoad } from '../routes/(app)/org/[slug]/audit-log/+page.server';
import { auditLogApi } from '$lib/api/audit-log';
import type { AuditLogEntry } from '$lib/types/audit-log';

const sampleEntries: AuditLogEntry[] = Array.from({ length: 15 }, (_, i) => ({
  event_type: i % 2 === 0 ? 'member.invited' : 'sso.login',
  user_id: 'user_1',
  target_type: 'invitation',
  target_id: `inv_${i}`,
  created_at: `2026-04-${10 + i}T10:00:00Z`,
  ip_address: '10.0.0.1',
  success: true,
  failure_reason: null
}));

function makeAuditEvent(opts: {
  accessToken?: string | null;
  slug?: string;
  query?: Record<string, string>;
}): Parameters<typeof auditLoad>[0] {
  const { accessToken = 'tok123', slug = 'acme', query = {} } = opts;
  const sp = new URLSearchParams(query);
  return {
    locals: { accessToken, user: null },
    params: { slug },
    url: { pathname: `/org/${slug}/audit-log`, search: sp.toString() ? `?${sp}` : '', searchParams: sp },
    fetch: vi.fn()
  } as unknown as Parameters<typeof auditLoad>[0];
}

describe('audit log page loader', () => {
  beforeEach(() => {
    vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise' });
    vi.mocked(auditLogApi.list).mockResolvedValue(sampleEntries);
  });

  const cases = [
    { name: 'redirects to login when no token', accessToken: null, expectThrows: true },
    { name: '404 when org not found', orgNull: true, expectThrows: true },
    { name: 'redirects when non-enterprise tier', nonEnterprise: true, expectThrows: true },
    { name: 'redirects when role is member (admin_required)', memberRole: true, expectThrows: true },
    { name: 'first page returns 10 entries newest-first', expectPage: 1, expectCount: 10 },
    { name: 'page=2 returns remaining 5 entries', query: { page: '2' }, expectPage: 2, expectCount: 5 },
    { name: 'page beyond last is clamped', query: { page: '99' }, expectClamped: true },
    { name: 'event_type query param flows into filter', query: { event_type: 'member.invited' }, expectFilter: { event_type: 'member.invited' } },
    { name: 'from/to query params flow into filter', query: { from: '2026-04-01', to: '2026-04-30' }, expectFilter: { from: '2026-04-01', to: '2026-04-30' } },
    { name: 'api throws -> error field populated', apiThrows: true, expectError: true }
  ];

  for (const c of cases) {
    it(c.name, async () => {
      if ('orgNull' in c && c.orgNull) vi.mocked(organizationsApi.findBySlug).mockResolvedValue(null);
      if ('nonEnterprise' in c && c.nonEnterprise)
        vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'professional' });
      if ('memberRole' in c && c.memberRole)
        vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise', role: 'member' });
      if ('apiThrows' in c && c.apiThrows) vi.mocked(auditLogApi.list).mockRejectedValue(new Error('boom'));

      const accessToken = ('accessToken' in c ? c.accessToken : 'tok123') as string | null;
      const event = makeAuditEvent({ accessToken, query: c.query });

      if (c.expectThrows) {
        await expect(auditLoad(event)).rejects.toBeDefined();
        return;
      }

      const r = (await auditLoad(event)) as unknown as {
        entries: AuditLogEntry[];
        page: number;
        totalPages: number;
        filter: { event_type: string | null; from: string | null; to: string | null };
        error: string | null;
      };
      if (c.expectCount !== undefined) expect(r.entries).toHaveLength(c.expectCount);
      if (c.expectPage !== undefined) expect(r.page).toBe(c.expectPage);
      if (c.expectClamped) expect(r.page).toBe(r.totalPages);
      if (c.expectFilter) expect(r.filter).toMatchObject(c.expectFilter);
      if (c.expectError) expect(r.error).toBeTruthy();
    });
  }
});
```

#### Impact on Existing Tests
- `routes-tests/page-server.test.ts` already mocks `organizationsApi` and `ssoApi`;
  adding another `vi.mock` block for `audit-log` is additive. No existing assertions
  change.

---

### Step 4: CSV export endpoint
**Rationale:** A thin SvelteKit endpoint is the simplest way to expose a
browser-triggerable download. Calls the JSON client and re-formats via Step 1.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/audit-log/export/+server.ts` | create | `GET` handler that enforces guards, calls `auditLogApi.list`, returns CSV. |
| `web/src/routes-tests/page-server.test.ts` | modify | Add an `audit log export endpoint` describe block testing the handler. |

#### New Code

```ts
import type { RequestHandler } from './$types';
import { error } from '@sveltejs/kit';
import { requireAuth, requireEnterpriseTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { auditLogApi } from '$lib/api/audit-log';
import { formatAuditLogCsv } from '$lib/audit-log/csv';

export const GET: RequestHandler = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname);
  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const entOrg = requireEnterpriseTier(org, params.slug);
  const adminOrg = requireOrgAdmin(entOrg, params.slug);

  const event_type = url.searchParams.get('event_type') ?? undefined;
  const from = url.searchParams.get('from') ?? undefined;
  const to = url.searchParams.get('to') ?? undefined;

  let entries;
  try {
    entries = await auditLogApi.list(adminOrg.id, {
      token,
      fetch,
      limit: 200,
      event_type,
      from,
      to
    });
  } catch {
    throw error(502, 'Upstream audit-log fetch failed');
  }

  const csv = formatAuditLogCsv(entries);
  const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
  const filename = `audit-log-${params.slug}-${stamp}.csv`;
  return new Response(csv, {
    status: 200,
    headers: {
      'Content-Type': 'text/csv; charset=utf-8',
      'Content-Disposition': `attachment; filename="${filename}"`
    }
  });
};
```

#### Tests to Write FIRST (RED phase)

```ts
import { GET as auditExport } from '../routes/(app)/org/[slug]/audit-log/export/+server';

describe('audit log export endpoint', () => {
  beforeEach(() => {
    vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise' });
    vi.mocked(auditLogApi.list).mockResolvedValue(sampleEntries);
  });

  it('returns 200 text/csv with filename in Content-Disposition', async () => {
    const req = makeAuditEvent({ query: {} }) as unknown as Parameters<typeof auditExport>[0];
    const res = await auditExport(req);
    expect(res.headers.get('Content-Type')).toMatch(/text\/csv/);
    expect(res.headers.get('Content-Disposition')).toMatch(/audit-log-acme-\d{8}\.csv/);
  });

  it('first line is the required header', async () => {
    const req = makeAuditEvent({ query: {} }) as unknown as Parameters<typeof auditExport>[0];
    const res = await auditExport(req);
    const body = await res.text();
    expect(body.split('\r\n')[0]).toBe('event_type,user,target,timestamp,ip');
  });

  it('forwards event_type filter to the api client', async () => {
    const req = makeAuditEvent({ query: { event_type: 'member.invited' } }) as unknown as Parameters<typeof auditExport>[0];
    await auditExport(req);
    expect(vi.mocked(auditLogApi.list)).toHaveBeenCalledWith(
      expect.any(String),
      expect.objectContaining({ event_type: 'member.invited' })
    );
  });

  it('redirects non-admin (via requireOrgAdmin)', async () => {
    vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise', role: 'member' });
    const req = makeAuditEvent({ query: {} }) as unknown as Parameters<typeof auditExport>[0];
    await expect(auditExport(req)).rejects.toBeDefined();
  });
});
```

#### Impact on Existing Tests
- None; appended describe block in an existing file.

---

### Step 5: Page Svelte component with table, filters, pagination, export button
**Rationale:** Depends on the loader contract from Step 3. Encapsulates UI only —
no fetch calls at runtime (those happen in the loader or via a plain link for
export).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/audit-log/+page.svelte` | create | Table + filters + pagination + export button. |

#### New Code (outline)

```svelte
<script lang="ts">
  import type { PageData } from './$types';
  import { goto } from '$app/navigation';
  import { page } from '$app/stores';
  import EmptyState from '$lib/components/ui/EmptyState.svelte';
  import ErrorState from '$lib/components/ui/ErrorState.svelte';
  import { AUDIT_EVENT_TYPES } from '$lib/types/audit-log';

  export let data: PageData;

  function fmtDate(iso: string): string {
    return new Date(iso).toLocaleString();
  }
  function target(e: { target_type: string | null; target_id: string | null }): string {
    return [e.target_type, e.target_id].filter(Boolean).join(':') || '—';
  }

  async function applyFilter(patch: Record<string, string | null>) {
    const u = new URL($page.url);
    for (const [k, v] of Object.entries(patch)) {
      if (v === null || v === '') u.searchParams.delete(k);
      else u.searchParams.set(k, v);
    }
    u.searchParams.delete('page'); // reset to page 1 on filter change
    await goto(u.pathname + u.search, { keepFocus: true, noScroll: true });
  }

  function pickRange(preset: '24h' | '7d' | '30d' | 'all') {
    if (preset === 'all') {
      applyFilter({ from: null, to: null });
      return;
    }
    const now = new Date();
    const ms = preset === '24h' ? 86_400_000 : preset === '7d' ? 7 * 86_400_000 : 30 * 86_400_000;
    const from = new Date(now.getTime() - ms).toISOString();
    applyFilter({ from, to: null });
  }

  function goToPage(n: number) {
    const u = new URL($page.url);
    u.searchParams.set('page', String(n));
    goto(u.pathname + u.search, { keepFocus: true, noScroll: true });
  }

  $: exportHref = (() => {
    const u = new URL($page.url);
    u.pathname = `/org/${data.org.slug}/audit-log/export`;
    u.searchParams.delete('page');
    return u.pathname + u.search;
  })();
</script>

<svelte:head><title>{data.org.name} — Audit Log</title></svelte:head>

<div class="mx-auto max-w-6xl px-4 py-8">
  <div class="mb-4 flex items-center justify-between">
    <h1 data-testid="audit-log-heading" class="text-2xl font-bold text-gray-900">Audit Log</h1>
    <a
      href={exportHref}
      data-testid="audit-log-export"
      download
      class="rounded-md border border-gray-300 bg-white px-4 py-2 text-sm font-medium text-gray-700 hover:bg-gray-50"
    >Export CSV</a>
  </div>

  <!-- filters -->
  <div class="mb-4 flex flex-wrap items-center gap-3" data-testid="audit-log-filters">
    <label class="text-sm">
      Event type
      <select
        data-testid="audit-log-event-filter"
        value={data.filter.event_type ?? ''}
        on:change={(e) => applyFilter({ event_type: (e.currentTarget as HTMLSelectElement).value || null })}
      >
        <option value="">All</option>
        {#each AUDIT_EVENT_TYPES as t}<option value={t}>{t}</option>{/each}
      </select>
    </label>
    <div class="flex gap-1" data-testid="audit-log-range-picker">
      <button type="button" data-testid="range-24h" on:click={() => pickRange('24h')}>24h</button>
      <button type="button" data-testid="range-7d" on:click={() => pickRange('7d')}>7d</button>
      <button type="button" data-testid="range-30d" on:click={() => pickRange('30d')}>30d</button>
      <button type="button" data-testid="range-all" on:click={() => pickRange('all')}>All</button>
    </div>
  </div>

  {#if data.error}
    <ErrorState message={data.error} retryUrl={$page.url.pathname + $page.url.search} />
  {:else if data.entries.length === 0}
    <EmptyState message="No audit-log entries for the selected filter." />
  {:else}
    <table data-testid="audit-log-table" class="min-w-full divide-y divide-gray-200">
      <thead class="bg-gray-50">
        <tr>
          <th>Event type</th><th>User</th><th>Target</th><th>Timestamp</th><th>IP</th>
        </tr>
      </thead>
      <tbody>
        {#each data.entries as entry}
          <tr>
            <td>{entry.event_type}</td>
            <td>{entry.user_id ?? '—'}</td>
            <td>{target(entry)}</td>
            <td>{fmtDate(entry.created_at)}</td>
            <td>{entry.ip_address ?? '—'}</td>
          </tr>
        {/each}
      </tbody>
    </table>

    <div class="mt-4 flex items-center justify-between" data-testid="audit-log-pagination">
      <span>Page {data.page} of {data.totalPages}</span>
      <div class="flex gap-2">
        <button
          type="button"
          data-testid="audit-log-prev"
          disabled={data.page <= 1}
          on:click={() => goToPage(data.page - 1)}>Prev</button>
        <button
          type="button"
          data-testid="audit-log-next"
          disabled={data.page >= data.totalPages}
          on:click={() => goToPage(data.page + 1)}>Next</button>
      </div>
    </div>
  {/if}
</div>
```

(Tailwind utility classes trimmed in this plan; the final file will include them.)

#### Tests to Write FIRST (RED phase)
- No unit tests for the `.svelte` file itself; the Playwright spec in Step 7
  exercises the rendering. Loader behavior is covered by Step 3.

#### Impact on Existing Tests
- None.

---

### Step 6: Sub-nav link
**Rationale:** Tiny change, but must only happen after the route exists to
avoid 404s behind a visible link.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/+layout.svelte` | modify | Insert `subnav-audit-log-link` for enterprise + admin/owner. |

#### Current Code (relevant block)

```svelte
{#if isEnterpriseTier && currentOrg?.role === 'owner'}
  <a
    href="/org/{currentOrg.slug}/settings/sso"
    data-testid="subnav-sso-link"
    class="text-gray-600 hover:text-gray-900"
  >
    SSO
  </a>
{/if}
```

#### New Code (add immediately after the SSO link)

```svelte
{#if isEnterpriseTier && isAdmin}
  <a
    href="/org/{currentOrg.slug}/audit-log"
    data-testid="subnav-audit-log-link"
    class="text-gray-600 hover:text-gray-900"
  >
    Audit Log
  </a>
{/if}
```

#### Impact on Existing Tests
- `tests/e2e/org-sso.spec.ts` asserts `subnav-sso-link` visibility. That test's org
  defaults to `tier='team'` in mocked fixtures, which still passes the
  `isEnterpriseTier` check (team is a superset). Adding another link alongside the
  SSO link does not break those assertions — they only check `subnav-sso-link`
  presence, not absence of anything else.
- `tests/e2e/org-results.spec.ts` asserts `subnav-results-link`. Unaffected.

---

### Step 7: E2E spec
**Rationale:** The observable of the task. Uses `context.route()` mocks to
decouple from whether the backend audit log contains rows in the seeded DB.
Covers all six behaviors plus the non-admin redirect.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/org-audit-log.spec.ts` | create | Playwright spec with 7 tests. |

#### New Code (structure)

```ts
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const AUDIT_URL = `/org/${SEEDED_ORG_SLUG}/audit-log`;

function mockOrg(role: 'owner' | 'admin' | 'member' = 'owner', tier = 'enterprise') {
  return {
    organizations: [
      { id: 'org_audit', slug: SEEDED_ORG_SLUG, name: 'Acme', role,
        seat_count: 1, seat_limit: 50, status: 'active',
        created_at: '2026-04-01T00:00:00Z', tier }
    ]
  };
}

function mockEntries(count: number, eventType = 'member.invited') {
  return Array.from({ length: count }, (_, i) => ({
    event_type: i % 2 === 0 ? eventType : 'sso.login',
    user_id: `user_${i}`,
    target_type: 'invitation',
    target_id: `inv_${i}`,
    created_at: new Date(Date.UTC(2026, 3, 15 - i)).toISOString(),
    ip_address: '10.0.0.1',
    success: true,
    failure_reason: null
  }));
}

test.describe('Audit log viewer', () => {
  test.beforeEach(async ({ context }) => {
    await seedAuthCookie(context, OWNER_EMAIL);
    await context.route('**/api/v1/organizations', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify(mockOrg('owner')) })
    );
  });

  // Behaviors 1 + 2
  test('renders 10 newest-first rows with expected columns', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) })
    );
    await page.goto(AUDIT_URL);
    await expect(page.getByTestId('audit-log-heading')).toBeVisible();
    const rows = page.getByTestId('audit-log-table').locator('tbody tr');
    await expect(rows).toHaveCount(10);
    await expect(rows.first()).toContainText('member.invited');
  });

  // Behavior 2 — pagination
  test('pagination navigates between pages', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) })
    );
    await page.goto(AUDIT_URL);
    await page.getByTestId('audit-log-next').click();
    await expect(page).toHaveURL(/page=2/);
    const rows = page.getByTestId('audit-log-table').locator('tbody tr');
    await expect(rows).toHaveCount(10);
  });

  // Behavior 3 — event_type filter
  test('event_type filter updates URL and narrows rows', async ({ page, context }) => {
    let captured: string | null = null;
    await context.route('**/api/v1/organizations/*/audit-log**', (r) => {
      const u = new URL(r.request().url());
      if (u.searchParams.get('event_type') === 'member.invited') {
        captured = 'member.invited';
        return r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(5, 'member.invited') }) });
      }
      return r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(20) }) });
    });
    await page.goto(AUDIT_URL);
    await page.getByTestId('audit-log-event-filter').selectOption('member.invited');
    await expect(page).toHaveURL(/event_type=member\.invited/);
    await expect(page.getByTestId('audit-log-table').locator('tbody tr')).toHaveCount(5);
    expect(captured).toBe('member.invited');
  });

  // Behavior 4 — date range
  test('range "last 7 days" adds ?from= to URL', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(10) }) })
    );
    await page.goto(AUDIT_URL);
    await page.getByTestId('range-7d').click();
    await expect(page).toHaveURL(/from=/);
  });

  // Behavior 5 — CSV export
  test('Export CSV triggers a download with correct header row', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify({ items: mockEntries(3) }) })
    );
    await page.goto(AUDIT_URL);
    const [download] = await Promise.all([
      page.waitForEvent('download'),
      page.getByTestId('audit-log-export').click()
    ]);
    const stream = await download.createReadStream();
    const chunks: Buffer[] = [];
    for await (const chunk of stream!) chunks.push(chunk as Buffer);
    const text = Buffer.concat(chunks).toString('utf8');
    expect(text.split('\r\n')[0]).toBe('event_type,user,target,timestamp,ip');
    expect(text).toContain('member.invited');
  });

  // Behavior 6 — non-admin redirect
  test('non-admin is redirected with admin_required toast', async ({ page, context }) => {
    await context.unroute('**/api/v1/organizations').catch(() => {});
    await context.clearCookies();
    await seedAuthCookie(context, MEMBER_EMAIL);
    await context.route('**/api/v1/organizations', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify(mockOrg('member')) })
    );
    await page.goto(AUDIT_URL);
    await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
    await expect(page.getByTestId('toast-admin-required')).toBeVisible();
  });

  // DoD — subnav link visible for admins of enterprise-tier orgs
  test('audit-log subnav link is visible for owner of enterprise org', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/audit-log**', (r) =>
      r.fulfill({ status: 200, body: JSON.stringify({ items: [] }) })
    );
    await page.goto(AUDIT_URL);
    await expect(page.getByTestId('subnav-audit-log-link')).toBeVisible();
  });
});
```

#### Impact on Existing Tests
- None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `web/src/lib/audit-log/csv.test.ts` | all cases | new | write in Step 1 (RED) |
| `web/src/lib/api/audit-log.test.ts` | all cases | new | write in Step 2 (RED) |
| `web/src/routes-tests/page-server.test.ts` | `audit log page loader` + `audit log export endpoint` | new describe blocks | write in Steps 3 & 4 (RED) |
| `web/tests/e2e/org-audit-log.spec.ts` | 7 scenarios | new | write in Step 7 (RED) |
| `web/tests/e2e/org-sso.spec.ts` | `subnav-sso-link visible` | unchanged | none — asserts link presence only |
| `web/tests/e2e/org-results.spec.ts` | subnav assertions | unchanged | none |
| `web/tests/e2e/org-notifications.spec.ts` | subnav assertions | unchanged | none |

## Risks and Edge Cases

- **Risk:** Backend CSV header differs from observable spec.
  **Mitigation:** The SvelteKit export endpoint re-formats via Step 1 so the
  download always matches the observable, independent of backend changes.
- **Risk:** A seeded DB may not have audit-log rows, causing E2E flakiness if we
  assert counts against live data.
  **Mitigation:** All E2E tests `context.route()`-mock `**/audit-log**` so they
  are deterministic. (The observable's "10 rows render" is satisfied via mocked
  20 rows paged at 10/page.)
- **Risk:** `requireEnterpriseTier`'s transitional behaviour (absent tier ⇒ enterprise)
  means the page is accessible even for orgs that are not enterprise in production
  until the tier field is mandatory.
  **Mitigation:** Match the existing SSO page behaviour — this is the documented
  transitional state. The loader unit tests explicitly cover
  `tier='professional'` → redirect to make the non-enterprise case exercisable.
- **Edge case:** `?page=0` or a negative page.
  **Handling:** Loader uses `Math.max(1, Number(...) || 1)` and clamps to
  `totalPages`.
- **Edge case:** Empty result set.
  **Handling:** Loader returns empty array; component renders `<EmptyState>`.
- **Edge case:** Bad `from`/`to` string (non-ISO).
  **Handling:** Forwarded verbatim to the backend; backend returns 400
  `invalid_filter` which the loader surfaces via its catch block as `error`.
- **Edge case:** Filter change while on page 5.
  **Handling:** `applyFilter` deletes `page` from the URL so a new filter always
  resets to page 1.
- **Edge case:** CSV-injection via `event_type` or other string fields.
  **Handling:** `formatAuditLogCsv.quote` prefixes `=`, `+`, `-`, `@` with a
  single-quote, matching the backend's `AuditLogCsvFormatter` approach.
- **Edge case:** Download attribute ignored by some browsers for cross-origin
  responses.
  **Handling:** The export URL is same-origin (SvelteKit route), so `download`
  attribute + `Content-Disposition: attachment` combine to force a download.

## Verification

```bash
cd web
npm install
npm run check            # svelte-check: no type errors
npm run lint             # eslint clean
npm run test:unit        # vitest — all unit cases pass
npm run build            # production build
npm run test:e2e -- tests/e2e/org-audit-log.spec.ts  # observable
```

Observable verification (from task YAML):

```bash
cd web && npm install && npm run build
npm run test:e2e -- tests/e2e/org-audit-log.spec.ts
# Expected: Playwright run OK, >=6 passing.
```
