# Implementation Plan: M4-005

## Overview

Deliver the team-tier **Test Results Dashboard** web page at `/org/[slug]/results`, along with the minimum SvelteKit scaffolding required to host it (this is the first `track: web` task in the backlog). The slice ships: a new `web/` SvelteKit + TypeScript + Tailwind project, a typed fetch wrapper, a results API client, an org-resolve-by-slug helper, a `RequireTeamTier` guard, the results page (+loader + components), a Playwright E2E spec, and a `scripts/test-stack.sh` docker-compose harness that brings up backend + web for the spec.

## Task Details

- **ID:** M4-005
- **Title:** Web: team test results dashboard page
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** high
- **Estimated effort:** 8–12 hours
- **Track:** web
- **Branch:** `feature/M4-005-web-results-dashboard`

## Dependencies

| Task   | Title                                   | Status |
|--------|-----------------------------------------|--------|
| M4-004 | Backend: test results ingestion API     | done   |

## Exploration Summary

### Backend API surface (from M4-004, already deployed)

The results page consumes these existing endpoints:

| Method | Path | Notes |
|--------|------|-------|
| `GET`  | `/api/v1/organizations`                             | List orgs the caller belongs to — used to resolve `slug → orgId`. |
| `GET`  | `/api/v1/organizations/{orgId}`                     | Org detail (role, seat count/limit). |
| `GET`  | `/api/v1/organizations/{orgId}/results?limit=N`     | Newest-first result headers. Backend clamps `limit` to `[1,100]` (`ResultsService.ListAsync` line 102). |
| `GET`  | `/api/v1/results/{resultId}`                        | Full detail (headers + items). |
| `POST` | `/api/v1/organizations/{orgId}/results`             | Ingest (used by the seed script to create fixtures). |

Response wire format is snake_case (`ResultsEndpoints.cs` + `Program.cs` JSON options). `ListResultsResponse` shape is:

```json
{ "results": [ { "id":"res_...", "collection_name":"...", "pass_count":3, "fail_count":0,
                 "skipped_count":0, "duration_ms":1234, "run_at":"...", "created_at":"...",
                 "triggered_by":"cli", "git_sha":"abc1234" } ] }
```

### Spec-called endpoints that do **not** yet exist

The spec (section "Team Features (Team Tier+)", line 8260) lists `/results/stats` and `/results/failures` for the dashboard. **Neither exists in the backend** and the backlog has no task to build them (M4-004 was the only results backend slice). The task scope for M4-005 explicitly says "calls the M4-004 endpoints". **Decision:** derive all overview stats (total runs, pass/fail, pass rate %, average duration, trend) from the `GET /organizations/{orgId}/results?limit=100` payload inside the `+page.server.ts` loader. This keeps the slice self-contained and defers true stats endpoints until a future backend task.

### Tier gating

`OrganizationDto` (`src/ApiTool.Backend/Organizations/OrganizationDto.cs`) exposes `role`, `seat_count`, `seat_limit`, `status` — **no `tier` field**. The spec's `RequireTeamTier` guard therefore has nothing authoritative to check today. **Decision:** introduce an optional `tier?: 'free' | 'solo' | 'professional' | 'team'` field on the *web-side* `Organization` type. For now we treat the absence of the field (or `tier === 'team'`) as "team tier allowed", matching the M4-003/M4-004 stance that the backend currently defaults all orgs to Team-tier behaviour. The E2E test covers the redirect path by Playwright-route-intercepting `GET /api/v1/organizations` to return `tier: 'professional'` — no backend change is needed.

### Authentication

No magic-link backend exists (confirmed by grep — there is no `magic-link` or `/auth` route under `src/ApiTool.Backend/`). The task description says "logs in as owner via seeded magic link", which we interpret as "seeded bearer token acting as the owner" — the same pattern used by `src/ApiTool.Backend.Tests/TestInfrastructure/TestTokens.cs` and `scripts/test-token.sh`. The E2E spec will mint a dev JWT via `scripts/test-token.sh` and inject it as an `access_token` cookie using Playwright's `context.addCookies()`. This is documented as an intentional deviation in the plan's Risks section and in `web/README.md`.

### No pre-existing web project

`web/` does not exist. `package.json` does not exist anywhere in the repo. This slice bootstraps the entire web project. Subsequent web tasks (M4-009, M4-011, M4-012) will extend it.

### Backend Dockerization

No Dockerfile or docker-compose.yml exists. The test stack must create them. Because backend currently uses SQLite (not Postgres — see `appsettings.Development.json`), the compose file only needs two services: `backend` and `web`. Health checks use `/swagger/v1/swagger.json` for backend (known-good endpoint — curl not required in the image because the compose `depends_on` condition uses `service_healthy` via a `HEALTHCHECK` that shells out to `wget`, present in the aspnet alpine image).

## Implementation Steps

Steps are ordered from **smallest blast radius outward**: scaffolding and tooling first (no test impact), then types/API client (unit-testable with Vitest), then the guard + loader (server-rendered unit path), then the page component, then docker harness, then the full E2E spec. Each step lands with a green test suite before the next begins.

---

### Step 1: Bootstrap the SvelteKit project

**Rationale:** Every subsequent step depends on having a runnable SvelteKit app. Doing this first in isolation keeps the remaining steps focused on domain code.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/package.json` | create | SvelteKit 2.x, Svelte 5.x, TypeScript 5.x, Tailwind 4.x, Playwright 1.47+, Vitest 2.x, `@sveltejs/adapter-node` 5.x |
| `web/svelte.config.js` | create | `adapter: adapterNode()`, `alias: { $lib: 'src/lib' }`, preprocess Vite |
| `web/vite.config.ts` | create | `sveltekit()` plugin, Vitest `test.environment: 'jsdom'` |
| `web/tsconfig.json` | create | Extends `.svelte-kit/tsconfig.json`, `strict: true`, `moduleResolution: 'bundler'` |
| `web/tailwind.config.js` | create | Content globs for `./src/**/*.{html,svelte,ts}` |
| `web/postcss.config.js` | create | `tailwindcss` + `autoprefixer` |
| `web/src/app.css` | create | `@tailwind base; @tailwind components; @tailwind utilities;` |
| `web/src/app.html` | create | SvelteKit template boilerplate with `%sveltekit.head%` / `%sveltekit.body%` |
| `web/src/app.d.ts` | create | Ambient types: `App.Locals { user: User \| null; accessToken: string \| null }` |
| `web/.gitignore` | create | `node_modules/`, `.svelte-kit/`, `build/`, `.env`, `test-results/`, `playwright-report/` |
| `web/README.md` | create | How to `npm install`, `npm run dev`, `npm run build`, and run the E2E spec via `scripts/test-stack.sh` |
| `web/playwright.config.ts` | create | `testDir: './tests/e2e'`, `webServer` *not* used (stack is managed by `scripts/test-stack.sh`), `use.baseURL` from env `WEB_BASE_URL` (default `http://localhost:3000`) |
| `web/.env.example` | create | `PUBLIC_API_URL=http://localhost:5000` |

#### package.json scripts

```json
{
  "scripts": {
    "dev": "vite dev",
    "build": "vite build",
    "preview": "vite preview --host --port 3000",
    "start": "node build",
    "check": "svelte-kit sync && svelte-check --tsconfig ./tsconfig.json",
    "lint": "eslint .",
    "test:unit": "vitest run",
    "test:e2e": "playwright test"
  }
}
```

#### Tests to write FIRST (RED phase)

Create `web/src/lib/smoke.test.ts` with one sanity check:

```ts
import { describe, it, expect } from 'vitest';
describe('scaffolding', () => {
  it('loads the test runner', () => { expect(1 + 1).toBe(2); });
});
```

Verifies `vitest`, TypeScript compilation, and tsconfig paths work. Run `npm run check && npm run test:unit` — both green gate this step.

#### Impact on existing tests

None — no tests exist.

---

### Step 2: Typed `Result` / `ResultSummary` / `Organization` types

**Rationale:** Types must exist before the API client and loader; they're a pure-source-file change with zero runtime impact.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/results.ts` | create | `Result`, `ResultItem`, `ResultSummary`, `TimeRange`, `TrendPoint` |
| `web/src/lib/types/organization.ts` | create | `Organization`, `OrgRole`, `OrgTier` |
| `web/src/lib/types/api-error.ts` | create | `ApiError` class + `isApiError` type guard |

#### New code

```ts
// web/src/lib/types/results.ts
export interface Result {
  id: string;
  collection_name: string;
  pass_count: number;
  fail_count: number;
  skipped_count: number;
  duration_ms: number;
  run_at: string;        // ISO 8601
  created_at: string;    // ISO 8601
  triggered_by: string | null;
  git_sha: string | null;
}

export interface ResultSummary {
  total_runs: number;
  total_pass: number;
  total_fail: number;
  pass_rate: number;        // 0–100
  average_duration_ms: number;
}

export type TimeRange = '24h' | '7d' | '30d' | 'all';

export interface TrendPoint {
  bucket_start: string;  // ISO date (day-bucket)
  pass: number;
  fail: number;
}
```

```ts
// web/src/lib/types/organization.ts
export type OrgRole = 'owner' | 'admin' | 'member';
export type OrgTier = 'free' | 'solo' | 'professional' | 'team';

export interface Organization {
  id: string;
  name: string;
  slug: string;
  role: OrgRole;
  seat_count: number;
  seat_limit: number;
  status: string;
  created_at: string;
  tier?: OrgTier;   // optional until backend exposes it; absent ⇒ treat as 'team'
}
```

#### Tests to write FIRST (RED phase)

`web/src/lib/types/api-error.test.ts`:

```ts
import { describe, it, expect } from 'vitest';
import { ApiError, isApiError } from './api-error';

describe('ApiError', () => {
  const cases = [
    { name: 'captures code and status',           code: 'permission_denied', status: 403 },
    { name: 'captures 5xx for retryable errors',  code: 'server_error',      status: 500 },
  ];
  for (const c of cases) {
    it(c.name, () => {
      const err = new ApiError(c.code, 'boom', c.status);
      expect(err.code).toBe(c.code);
      expect(err.status).toBe(c.status);
      expect(isApiError(err)).toBe(true);
    });
  }
  it('rejects non-errors in the type guard', () => {
    expect(isApiError({})).toBe(false);
    expect(isApiError(null)).toBe(false);
  });
});
```

#### Impact on existing tests

None.

---

### Step 3: Fetch wrapper `$lib/api/client.ts`

**Rationale:** The results client, org client, and every future web slice depends on the base `api` object. Isolate it so it can be mocked by Vitest and exercised independently.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/api/client.ts` | create | `request<T>()` + `api.get/post/patch/delete` |
| `web/src/lib/api/client.test.ts` | create | Unit tests using `vi.fn()` fetch mock |

#### New code

```ts
// web/src/lib/api/client.ts
import { env } from '$env/dynamic/public';
import { ApiError } from '$lib/types/api-error';

export interface RequestOptions extends Omit<RequestInit, 'body'> {
  body?: unknown;
  params?: Record<string, string | number | undefined>;
  token?: string;
  fetch?: typeof fetch;
}

function buildUrl(path: string, params?: RequestOptions['params']): string {
  const base = env.PUBLIC_API_URL ?? 'http://localhost:5000';
  const url = new URL(path.startsWith('/') ? path : `/${path}`, base);
  if (params) {
    for (const [k, v] of Object.entries(params)) {
      if (v !== undefined) url.searchParams.set(k, String(v));
    }
  }
  return url.toString();
}

export async function request<T>(method: string, path: string, opts: RequestOptions = {}): Promise<T> {
  const fetchFn = opts.fetch ?? fetch;
  const headers = new Headers(opts.headers);
  if (opts.body !== undefined && !headers.has('Content-Type')) {
    headers.set('Content-Type', 'application/json');
  }
  if (opts.token) headers.set('Authorization', `Bearer ${opts.token}`);

  const res = await fetchFn(buildUrl(path, opts.params), {
    ...opts,
    method,
    headers,
    body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
  });

  if (!res.ok) {
    let code = 'server_error';
    let description = res.statusText;
    try {
      const body = await res.json();
      if (body?.code) code = body.code;
      if (body?.description) description = body.description;
    } catch {
      /* non-JSON error body */
    }
    throw new ApiError(code, description, res.status);
  }

  if (res.status === 204) return undefined as T;
  return (await res.json()) as T;
}

export const api = {
  get:    <T>(path: string, opts?: RequestOptions) => request<T>('GET', path, opts),
  post:   <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('POST', path, { ...opts, body }),
  patch:  <T>(path: string, body?: unknown, opts?: RequestOptions) => request<T>('PATCH', path, { ...opts, body }),
  delete: <T>(path: string, opts?: RequestOptions) => request<T>('DELETE', path, opts),
};
```

#### Tests to write FIRST (RED phase)

`web/src/lib/api/client.test.ts` — table-driven:

```ts
describe('api.get', () => {
  const cases = [
    { name: 'returns parsed body on 200',             status: 200, body: '{"ok":true}',                 expect: { ok: true } },
    { name: 'attaches bearer token',                  status: 200, token: 't',                          assertHeader: 'Authorization=Bearer t' },
    { name: 'throws ApiError with backend code/msg',  status: 403, body: '{"code":"permission_denied","description":"nope"}', throws: ['permission_denied', 403] },
    { name: 'throws generic ApiError on non-JSON 5xx',status: 500, body: '<html>oops</html>',           throws: ['server_error', 500] },
    { name: 'serializes query params',                status: 200, body: '{}', params: { limit: 10, range: '7d' }, assertUrlEndsWith: '?limit=10&range=7d' },
  ];
  // table loop here
});
```

All tests inject a `vi.fn()` fake fetch via `opts.fetch` so no real network call is made.

#### Impact on existing tests

None.

---

### Step 4: Organizations client `$lib/api/organizations.ts`

**Rationale:** The results page loader must resolve `slug → orgId` before hitting the results endpoint. Splitting this out keeps the results client single-purpose.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/api/organizations.ts` | create | `listOrganizations(opts)`, `findBySlug(slug, opts)` |
| `web/src/lib/api/organizations.test.ts` | create | Unit tests with fetch mock |

#### New code

```ts
// web/src/lib/api/organizations.ts
import { api, type RequestOptions } from './client';
import type { Organization } from '$lib/types/organization';

interface ListResponse { organizations: Organization[]; }

export const organizationsApi = {
  async list(opts?: RequestOptions): Promise<Organization[]> {
    const { organizations } = await api.get<ListResponse>('/api/v1/organizations', opts);
    return organizations;
  },

  async findBySlug(slug: string, opts?: RequestOptions): Promise<Organization | null> {
    const orgs = await this.list(opts);
    return orgs.find((o) => o.slug === slug) ?? null;
  },
};
```

#### Tests to write FIRST (RED phase)

```ts
describe('organizationsApi.findBySlug', () => {
  const cases = [
    { name: 'returns matching org',       orgs: [{ slug: 'acme' }], slug: 'acme', expected: { slug: 'acme' } },
    { name: 'returns null on no match',   orgs: [{ slug: 'other' }], slug: 'acme', expected: null },
    { name: 'returns null on empty list', orgs: [], slug: 'acme', expected: null },
  ];
  // ...
});
```

#### Impact on existing tests

None.

---

### Step 5: Results client `$lib/api/results.ts` and stats helper

**Rationale:** Pure computation (stats + trend bucketing) is the ideal TDD seam — 100% deterministic, no network, easy to cover edge cases. This is where all the observable-facing math lives.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/api/results.ts` | create | `list({orgId, limit, token, fetch})` |
| `web/src/lib/api/results.test.ts` | create | Unit tests with fetch mock |
| `web/src/lib/results/stats.ts` | create | `summarize(results)`, `bucketByDay(results)`, `filterByRange(results, range)` |
| `web/src/lib/results/stats.test.ts` | create | Exhaustive table-driven cases |

#### New code (client)

```ts
// web/src/lib/api/results.ts
import { api, type RequestOptions } from './client';
import type { Result } from '$lib/types/results';

interface ListResultsResponse { results: Result[]; }

export const resultsApi = {
  async list(orgId: string, opts: RequestOptions & { limit?: number } = {}): Promise<Result[]> {
    const { limit = 100, ...rest } = opts;
    const { results } = await api.get<ListResultsResponse>(
      `/api/v1/organizations/${encodeURIComponent(orgId)}/results`,
      { ...rest, params: { ...rest.params, limit } },
    );
    return results;
  },
};
```

#### New code (stats)

```ts
// web/src/lib/results/stats.ts
import type { Result, ResultSummary, TimeRange, TrendPoint } from '$lib/types/results';

export function filterByRange(results: Result[], range: TimeRange, now: Date = new Date()): Result[] {
  if (range === 'all') return results;
  const hours = range === '24h' ? 24 : range === '7d' ? 24 * 7 : 24 * 30;
  const cutoff = now.getTime() - hours * 60 * 60 * 1000;
  return results.filter((r) => Date.parse(r.run_at) >= cutoff);
}

export function summarize(results: Result[]): ResultSummary {
  const total_runs = results.length;
  if (total_runs === 0) {
    return { total_runs: 0, total_pass: 0, total_fail: 0, pass_rate: 0, average_duration_ms: 0 };
  }
  const total_pass = results.reduce((n, r) => n + r.pass_count, 0);
  const total_fail = results.reduce((n, r) => n + r.fail_count, 0);
  const tests = total_pass + total_fail;
  const pass_rate = tests === 0 ? 0 : Math.round((total_pass / tests) * 10_000) / 100; // 2dp
  const average_duration_ms = Math.round(
    results.reduce((n, r) => n + r.duration_ms, 0) / total_runs,
  );
  return { total_runs, total_pass, total_fail, pass_rate, average_duration_ms };
}

export function bucketByDay(results: Result[]): TrendPoint[] {
  const buckets = new Map<string, TrendPoint>();
  for (const r of results) {
    const day = r.run_at.slice(0, 10); // YYYY-MM-DD
    const point = buckets.get(day) ?? { bucket_start: day, pass: 0, fail: 0 };
    point.pass += r.pass_count;
    point.fail += r.fail_count;
    buckets.set(day, point);
  }
  return [...buckets.values()].sort((a, b) => a.bucket_start.localeCompare(b.bucket_start));
}
```

#### Tests to write FIRST (RED phase)

`stats.test.ts` table-driven cases:

```ts
describe('summarize', () => {
  const cases = [
    { name: 'empty list',                    input: [],                          expect: { total_runs: 0, total_pass: 0, total_fail: 0, pass_rate: 0, average_duration_ms: 0 } },
    { name: 'single all-pass run',           input: [mk({ pass: 3, fail: 0, dur: 1000 })], expect: { total_runs: 1, pass_rate: 100, average_duration_ms: 1000 } },
    { name: 'mixed passes and failures',     input: [mk({ pass: 7, fail: 3 }), mk({ pass: 8, fail: 2 })], expect: { pass_rate: 75 } },
    { name: 'zero tests does not divide by zero', input: [mk({ pass: 0, fail: 0 })], expect: { pass_rate: 0 } },
    { name: 'average duration rounds to int',input: [mk({ dur: 100 }), mk({ dur: 201 })], expect: { average_duration_ms: 151 } },
  ];
});

describe('filterByRange', () => {
  const cases = [
    { name: 'range=all returns all',                range: 'all', window: '30d', expectedCount: 'all' },
    { name: 'range=7d drops older than 7 days',     range: '7d',  ...expectedDropsOld },
    { name: 'range=24h keeps only last 24h',        range: '24h', ... },
    { name: 'range=30d keeps 29-day-old entry',     range: '30d', ... },
    { name: 'equal boundary is inclusive',          range: '7d',  ... },
  ];
});

describe('bucketByDay', () => {
  const cases = [
    { name: 'single day',                      inputDays: ['2026-04-15'], expectedBuckets: 1 },
    { name: 'multiple days sorted ascending',  inputDays: ['2026-04-14','2026-04-15','2026-04-13'], expected: ['2026-04-13','2026-04-14','2026-04-15'] },
    { name: 'same day passes and fails merge', ... },
  ];
});
```

#### Impact on existing tests

None.

---

### Step 6: Auth hook and `RequireTeamTier` guard

**Rationale:** The `+page.server.ts` loader needs (a) the access token from the cookie and (b) a way to short-circuit with a redirect when the target org isn't team-tier. The guard is pure logic; test it with a mocked `event`.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/hooks.server.ts` | create | Reads `access_token` cookie, puts it on `event.locals.accessToken` (and, if present, stubs a `user` object derived from the JWT claims) |
| `web/src/lib/server/guards.ts` | create | `requireAuth(event)`, `requireTeamTier(org)` |
| `web/src/lib/server/guards.test.ts` | create | Unit tests for the guards |

#### New code

```ts
// web/src/hooks.server.ts
import type { Handle } from '@sveltejs/kit';

export const handle: Handle = async ({ event, resolve }) => {
  const token = event.cookies.get('access_token');
  event.locals.accessToken = token ?? null;
  event.locals.user = token ? decodeJwtClaims(token) : null;
  return resolve(event);
};

function decodeJwtClaims(token: string): { sub: string; email: string } | null {
  try {
    const [, payload] = token.split('.');
    const json = Buffer.from(payload, 'base64url').toString('utf8');
    const claims = JSON.parse(json);
    return { sub: claims.sub, email: claims.email };
  } catch {
    return null;
  }
}
```

```ts
// web/src/lib/server/guards.ts
import { redirect, error } from '@sveltejs/kit';
import type { Organization } from '$lib/types/organization';

export function requireAuth(accessToken: string | null, redirectTo: string): string {
  if (!accessToken) throw redirect(303, `/login?redirect=${encodeURIComponent(redirectTo)}`);
  return accessToken;
}

export function requireTeamTier(org: Organization | null, slug: string): Organization {
  if (!org) throw error(404, 'Organization not found');
  // Absent tier field ⇒ treat as 'team' (backend doesn't expose tier yet; see plan).
  const tier = org.tier ?? 'team';
  if (tier !== 'team') {
    throw redirect(303, `/org/${slug}?toast=team_tier_required`);
  }
  return org;
}
```

#### Tests to write FIRST (RED phase)

```ts
describe('requireTeamTier', () => {
  const cases = [
    { name: 'team tier passes through',         tier: 'team',         expected: 'pass' },
    { name: 'absent tier passes through',       tier: undefined,      expected: 'pass' },
    { name: 'professional tier redirects',      tier: 'professional', expected: 'redirect' },
    { name: 'solo tier redirects',              tier: 'solo',         expected: 'redirect' },
    { name: 'free tier redirects',              tier: 'free',         expected: 'redirect' },
    { name: 'null org throws 404',              org: null,            expected: 'error' },
  ];
});
```

#### Impact on existing tests

None.

---

### Step 7: Route loader `+page.server.ts`

**Rationale:** The page needs its data before the Svelte component mounts. The loader composes auth → org resolve → guard → results fetch → stats — it glues all prior steps together.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/+layout.server.ts` | create | Lists the current user's organizations once per app-layout render (for sub-nav) |
| `web/src/routes/(app)/+layout.svelte` | create | Placeholder app shell with `<slot />` (full shell lands in M4-011) |
| `web/src/routes/(app)/org/[slug]/results/+page.server.ts` | create | Main results loader |
| `web/src/routes/(app)/org/[slug]/results/+page.svelte` | create | Results UI (see Step 8) |
| `web/src/routes/(app)/org/[slug]/results/+page.server.test.ts` | create | Loader unit tests |

#### Loader code

```ts
// web/src/routes/(app)/org/[slug]/results/+page.server.ts
import type { PageServerLoad } from './$types';
import { requireAuth, requireTeamTier } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { resultsApi } from '$lib/api/results';
import { summarize, bucketByDay, filterByRange } from '$lib/results/stats';
import type { TimeRange } from '$lib/types/results';
import { error } from '@sveltejs/kit';

const ALLOWED_RANGES: TimeRange[] = ['24h', '7d', '30d', 'all'];

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
  const token = requireAuth(locals.accessToken, url.pathname + url.search);
  const rawRange = url.searchParams.get('range') ?? '30d';
  const range: TimeRange = (ALLOWED_RANGES as string[]).includes(rawRange) ? (rawRange as TimeRange) : '30d';

  const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
  const teamOrg = requireTeamTier(org, params.slug);

  try {
    const allResults = await resultsApi.list(teamOrg.id, { token, fetch, limit: 100 });
    const scoped = filterByRange(allResults, range);
    return {
      org: teamOrg,
      range,
      results: scoped,
      summary: summarize(scoped),
      trend: bucketByDay(scoped),
      error: null as string | null,
    };
  } catch (e) {
    const message = e instanceof Error ? e.message : 'failed to load results';
    return {
      org: teamOrg,
      range,
      results: [],
      summary: summarize([]),
      trend: [],
      error: message,
    };
  }
};
```

#### Tests to write FIRST (RED phase)

```ts
describe('results page loader', () => {
  const cases = [
    { name: 'redirects to login when no token' },
    { name: '404 when org not found' },
    { name: 'redirects when non-team tier' },
    { name: 'returns summary + trend for team-tier happy path' },
    { name: 'returns error field + empty summary when api throws' },
    { name: 'parses ?range=7d and passes it through' },
    { name: 'invalid range falls back to 30d' },
  ];
});
```

Uses `vi.mock('$lib/api/organizations')` and `vi.mock('$lib/api/results')`.

#### Impact on existing tests

None.

---

### Step 8: Results page component `+page.svelte`

**Rationale:** Pure presentational. Takes `data` from the loader and renders summary cards, trend chart, recent-runs table, time-range picker, empty state, error state. Tested end-to-end via Playwright in Step 10.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/src/routes/(app)/org/[slug]/results/+page.svelte` | create | Page component |
| `web/src/lib/components/results/SummaryCards.svelte` | create | Four stat cards |
| `web/src/lib/components/results/TrendChart.svelte` | create | Inline SVG bar-stack; no external dep (matches M2-028 HTML-report style) |
| `web/src/lib/components/results/RecentRunsTable.svelte` | create | Table with rows: date, file, user, pass, duration |
| `web/src/lib/components/results/TimeRangePicker.svelte` | create | Buttons `24h / 7d / 30d / all` that link to `?range=X` |
| `web/src/lib/components/ui/EmptyState.svelte` | create | Generic empty-state slot component |
| `web/src/lib/components/ui/ErrorState.svelte` | create | Error message + retry button |

#### Key page behaviours (required by the task's behaviours list)

1. If `data.error` → render `<ErrorState retry-url={url} />` and **nothing else** (no widgets).
2. If `data.summary.total_runs === 0` → render `<EmptyState message="Run curlew and upload results to get started" />`.
3. Otherwise render `<SummaryCards>`, `<TrendChart>`, `<RecentRunsTable>`, with `<TimeRangePicker>` always visible.

Each element carries a `data-testid` attribute used by the Playwright spec — `data-testid="summary-total-runs"`, `"summary-pass-rate"`, `"trend-chart"`, `"recent-runs-table"`, `"empty-state"`, `"error-state"`, `"range-picker-7d"`, etc.

#### Tests to write FIRST

Visual regression is covered by Playwright (Step 10). No unit tests for the Svelte components themselves — they are thin and assertions flow through the E2E.

#### Impact on existing tests

None.

---

### Step 9: Docker harness & test-stack script

**Rationale:** The Playwright spec in Step 10 needs a running backend + web. This step produces the infrastructure. Placing it after the app code ensures the containers build against the real source.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/Dockerfile` | create | Multi-stage build: `sdk:9.0` builder → `aspnet:9.0-alpine` runtime; exposes 5000; runs `dotnet ApiTool.Backend.dll` with `ASPNETCORE_ENVIRONMENT=Development` so the SQLite DB is auto-migrated on startup |
| `web/Dockerfile` | create | Multi-stage: `node:22-alpine` builder (npm ci + build) → runtime with `node build` on port 3000 |
| `docker-compose.test.yml` | create | Two services: `backend` (build context `.`, dockerfile `src/ApiTool.Backend/Dockerfile`, env `ASPNETCORE_URLS=http://0.0.0.0:5000`, port 5000, volume for SQLite) and `web` (build context `./web`, env `PUBLIC_API_URL=http://backend:5000`, port 3000, depends_on `backend: { condition: service_healthy }`) |
| `scripts/test-stack.sh` | create | `up` / `down` / `seed` subcommands; `up` runs `docker compose -f docker-compose.test.yml up --build -d` and waits for `/swagger/v1/swagger.json`; `seed` mints a dev JWT via `scripts/test-token.sh`, creates an org via `POST /api/v1/organizations`, then POSTs several `testdata/backend/sample-result-upload.json` variants for trend data; `down` runs `docker compose -f docker-compose.test.yml down -v` |
| `scripts/seed-test-data.sh` | create | Extracted from the `seed` subcommand for reuse; idempotent |
| `testdata/web/seed-results/` | create | 5 fixture JSON files (variations of sample-result-upload.json across different `run_at` days and pass/fail ratios) |

#### Health check pattern for backend

Dockerfile adds:
```
HEALTHCHECK --interval=5s --timeout=3s --retries=20 \
  CMD wget -qO- http://localhost:5000/swagger/v1/swagger.json >/dev/null || exit 1
```
`wget` ships in the `aspnet:9.0-alpine` base image.

#### test-stack.sh skeleton

```bash
#!/usr/bin/env bash
set -euo pipefail
cmd="${1:-up}"
case "$cmd" in
  up)
    docker compose -f docker-compose.test.yml up --build -d
    for i in {1..60}; do
      if curl -fsS http://localhost:5000/swagger/v1/swagger.json >/dev/null 2>&1; then
        echo "backend healthy"; break
      fi
      sleep 1
    done
    for i in {1..60}; do
      if curl -fsS http://localhost:3000/ >/dev/null 2>&1; then
        echo "web healthy"; break
      fi
      sleep 1
    done
    ./scripts/seed-test-data.sh
    ;;
  down)  docker compose -f docker-compose.test.yml down -v ;;
  seed)  ./scripts/seed-test-data.sh ;;
  *) echo "usage: $0 {up|down|seed}"; exit 2 ;;
esac
```

#### Impact on existing tests

`scripts/test-token.sh` already works and is reused.

---

### Step 10: Playwright E2E spec

**Rationale:** The task's observable command. Comes last because every prior step is a prerequisite.

#### Files to create

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/org-results.spec.ts` | create | >=5 tests |
| `web/tests/e2e/helpers/auth.ts` | create | `seedAuthCookie(page, email)` calls `scripts/test-token.sh` via `execSync` and sets `access_token` via `context.addCookies` |
| `web/tests/e2e/helpers/fixtures.ts` | create | Exposes the seeded org slug (`acme`) and e2e user id |
| `web/tests/e2e/global-setup.ts` | create | `beforeAll` runs `./scripts/test-stack.sh up` when `CURLEW_MANAGE_STACK=1` (opt-in) |

#### Tests to write (all RED first, but Playwright RED ≡ spec fails against a not-yet-complete build, which is implicit once Steps 1–9 run)

```ts
// web/tests/e2e/org-results.spec.ts
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';

test.describe('Team test results dashboard', () => {
  test.beforeEach(async ({ context }) => {
    await seedAuthCookie(context, 'owner@example.com');
  });

  test('renders overview stats for a seeded team org', async ({ page }) => {
    await page.goto('/org/acme/results');
    await expect(page.getByTestId('summary-total-runs')).toContainText(/\d+/);
    await expect(page.getByTestId('summary-pass-count')).toBeVisible();
    await expect(page.getByTestId('summary-fail-count')).toBeVisible();
    await expect(page.getByTestId('summary-pass-rate')).toContainText('%');
    await expect(page.getByTestId('summary-avg-duration')).toBeVisible();
    await expect(page.getByTestId('trend-chart')).toBeVisible();
  });

  test('recent runs table renders newest-first rows seeded by fixtures', async ({ page }) => {
    await page.goto('/org/acme/results');
    const rows = page.getByTestId('recent-runs-table').locator('tbody tr');
    await expect(rows).toHaveCount(5);
    await expect(rows.nth(0)).toContainText('smoke-tests');
  });

  test('empty state shown when backend has no results', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/results**',
      (route) => route.fulfill({ status: 200, body: JSON.stringify({ results: [] }) }));
    await page.goto('/org/acme/results');
    await expect(page.getByTestId('empty-state')).toContainText('Run curlew and upload results');
    await expect(page.getByTestId('summary-total-runs')).toHaveCount(0);
  });

  test('non-team-tier member is redirected with toast', async ({ page, context }) => {
    await context.route('**/api/v1/organizations', (route) =>
      route.fulfill({ status: 200, body: JSON.stringify({
        organizations: [{ id: 'org_123', slug: 'acme', name: 'Acme', role: 'member',
                          seat_count: 1, seat_limit: 10, status: 'active',
                          created_at: '2026-04-01T00:00:00Z', tier: 'professional' }] }) }));
    await page.goto('/org/acme/results');
    await expect(page).toHaveURL(/\/org\/acme(\?|$)/);
    await expect(page).not.toHaveURL(/\/results/);
    await expect(page.getByTestId('toast-team-tier-required')).toBeVisible();
  });

  test('time-range picker re-queries with ?range=7d', async ({ page }) => {
    await page.goto('/org/acme/results');
    const req = page.waitForRequest('**/api/v1/organizations/*/results**');
    await page.getByTestId('range-picker-7d').click();
    const hit = await req;
    expect(hit.url()).toContain('/results');
    await expect(page).toHaveURL(/\?range=7d/);
  });

  test('error state with retry button is shown on 500', async ({ page, context }) => {
    await context.route('**/api/v1/organizations/*/results**',
      (route) => route.fulfill({ status: 500, body: '{"code":"server_error","description":"boom"}' }));
    await page.goto('/org/acme/results');
    await expect(page.getByTestId('error-state')).toBeVisible();
    await expect(page.getByTestId('error-retry-button')).toBeVisible();
    await expect(page.getByTestId('summary-total-runs')).toHaveCount(0);
  });
});
```

Six tests, satisfying the `>=5 passing` DoD.

#### Impact on existing tests

None.

---

### Step 11: Update root project docs and CHANGELOG

**Rationale:** The completeness contract requires `CHANGELOG.md` updated and docs current.

#### Files to modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add `- Web: team test results dashboard page (web/src/routes/(app)/org/[slug]/results) ... (M4-005)` under `## [Unreleased] / ### Added` |
| `web/README.md` | already created in Step 1 | Document how to run `npm run test:e2e` locally, env vars, and the test-stack docker flow |
| `.gitignore` | modify if needed | Ensure `web/node_modules/` and `web/test-results/` are ignored (root gitignore or web-local) |

#### Impact on existing tests

None.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|-----------------|
| — | — | none | No existing web tests. No existing Go/C# tests change. |

No Go or C# tests are modified. The backend `dotnet test` suite, Go `go test ./...`, and `./smoke/run.sh` all run unchanged and must remain green as part of quality gates.

## New Test Inventory

| Suite | File | Test count (approx) |
|-------|------|----------------------|
| Vitest unit | `web/src/lib/smoke.test.ts` | 1 |
| Vitest unit | `web/src/lib/types/api-error.test.ts` | 3 |
| Vitest unit | `web/src/lib/api/client.test.ts` | 5 |
| Vitest unit | `web/src/lib/api/organizations.test.ts` | 3 |
| Vitest unit | `web/src/lib/api/results.test.ts` | 3 |
| Vitest unit | `web/src/lib/results/stats.test.ts` | 11 (5 summarize + 5 filter + 3 bucket) |
| Vitest unit | `web/src/lib/server/guards.test.ts` | 6 |
| Vitest unit | `web/src/routes/(app)/org/[slug]/results/+page.server.test.ts` | 7 |
| Playwright E2E | `web/tests/e2e/org-results.spec.ts` | 6 |

Total new tests: **~45**.

## Risks and Edge Cases

- **Risk — backend doesn't yet expose a `tier` field.** → **Mitigation:** treat absent `tier` as `"team"` in `requireTeamTier`. The non-team-tier test uses Playwright route interception to return `tier: 'professional'`, so the behaviour is still covered end-to-end. Document this in `web/README.md` and in a `TODO(M4-010)` comment in `guards.ts` so the field becomes mandatory once M4-010 (subscriptions) lands.
- **Risk — magic-link auth doesn't exist in the backend.** → **Mitigation:** the E2E spec seeds the `access_token` cookie directly by minting a dev JWT via `scripts/test-token.sh`. This is an explicit deviation from the task wording ("seeded magic link") justified by scope — building magic-link is a separate slice. Documented in `web/README.md` and the plan.
- **Risk — `ListAsync` clamps `limit` to 100** (backend line 102). The loader requests `limit=100`, and we compute `total_runs` from that truncated list. For seeded fixtures with ≤100 runs this is exact; for larger orgs the "total" becomes "total in the last 100". → **Mitigation:** acceptable for M4-005; mark with a `TODO` to switch to a `GET /results/stats` endpoint in a follow-up backend task. The Playwright spec seeds exactly 5 results so assertions are deterministic.
- **Risk — docker-compose startup time on CI blowing up E2E time.** → **Mitigation:** `scripts/test-stack.sh up` polls swagger JSON with a 60s budget; `docker compose up --build` caches layers between runs; local dev can skip the stack via `CURLEW_MANAGE_STACK=0` and point Playwright at an already-running stack.
- **Risk — snake_case vs camelCase drift.** Backend emits snake_case. The TypeScript `Result` type mirrors snake_case exactly — no transform layer. → **Mitigation:** explicit field names in the type definitions match the backend DTO exactly; no automatic conversion is applied.
- **Edge case — empty results list.** Covered by `summarize([])` returning zeros and by the empty-state Playwright test.
- **Edge case — zero total tests (pass + fail).** `summarize` guards division by zero and returns `pass_rate: 0`.
- **Edge case — run_at strings not timezone-normalized.** Backend stores UTC via `clock.GetUtcNow().UtcDateTime`; TypeScript `Date.parse` handles `Z`-suffixed ISO strings. Documented as UTC-only.
- **Edge case — invalid `?range` query parameter.** Loader falls back to `30d`; unit-tested.
- **Edge case — organization slug not matching any of the caller's orgs.** `findBySlug` returns `null`, the guard throws `error(404, ...)`, SvelteKit renders its default 404 page. E2E does not cover this path (not in DoD).

## Verification

### Quality gates (must all pass)

```bash
# Go — untouched, must remain green
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh

# Backend — untouched, must remain green
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj

# Web — new
cd web
npm install
npm run check
npm run lint
npm run test:unit
npm run build
```

### Observable verification (task YAML)

```bash
cd web && npm install && npm run build
CURLEW_MANAGE_STACK=1 npm run test:e2e -- tests/e2e/org-results.spec.ts
# Expected: Playwright reports >=5 tests passing (target: 6), including:
#   - renders overview stats
#   - recent runs table rows
#   - empty state
#   - non-team-tier redirect
#   - time-range picker
#   - error state with retry
```

After the run, `./scripts/test-stack.sh down` tears everything down.
