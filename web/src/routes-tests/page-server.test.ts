import { describe, it, expect, vi, beforeEach } from 'vitest';

// Mock the API modules before importing the loader
vi.mock('$lib/api/organizations', () => ({
	organizationsApi: {
		findBySlug: vi.fn()
	}
}));

vi.mock('$lib/api/schedules', () => ({
	schedulesApi: {
		list: vi.fn(),
		get: vi.fn(),
		listRuns: vi.fn()
	}
}));

vi.mock('$lib/api/audit-log', () => ({
	auditLogApi: {
		list: vi.fn()
	}
}));

vi.mock('$lib/api/results', () => ({
	resultsApi: {
		list: vi.fn()
	}
}));

vi.mock('$lib/api/sso', () => ({
	ssoApi: {
		get: vi.fn()
	}
}));

vi.mock('$lib/api/roles', () => ({
	rolesApi: {
		list: vi.fn()
	}
}));

vi.mock('$lib/api/members', () => ({
	membersApi: {
		list: vi.fn()
	}
}));

vi.mock('$lib/api/dashboard', () => ({
	dashboardApi: {
		getStats: vi.fn(),
		getFailures: vi.fn()
	}
}));

import { load } from '../routes/(app)/org/[slug]/results/+page.server';
import { load as ssoLoad } from '../routes/(app)/org/[slug]/settings/sso/+page.server';
import { load as rolesLoad } from '../routes/(app)/org/[slug]/settings/roles/+page.server';
import { load as dashboardLoad } from '../routes/(app)/org/[slug]/dashboard/+page.server';
import { organizationsApi } from '$lib/api/organizations';
import { rolesApi } from '$lib/api/roles';
import { membersApi } from '$lib/api/members';
import type { RoleView } from '$lib/types/roles';
import { resultsApi } from '$lib/api/results';
import { dashboardApi } from '$lib/api/dashboard';
import { ssoApi } from '$lib/api/sso';
import type { Organization } from '$lib/types/organization';
import type { Result, ResultSummary, TrendPoint, TimeRange } from '$lib/types/results';
import type { SsoConfigView } from '$lib/types/sso';
import type { StatsResponse, FailuresResponse } from '$lib/types/dashboard';

const teamOrg: Organization = {
	id: 'org_1',
	name: 'Acme',
	slug: 'acme',
	role: 'owner',
	seat_count: 1,
	seat_limit: 10,
	status: 'active',
	created_at: '2026-04-01T00:00:00Z',
	tier: 'team'
};

const sampleResults: Result[] = [
	{
		id: 'res_1',
		collection_name: 'smoke',
		pass_count: 3,
		fail_count: 0,
		skipped_count: 0,
		duration_ms: 1000,
		run_at: '2026-04-15T10:00:00Z',
		created_at: '2026-04-15T10:00:00Z',
		triggered_by: 'cli',
		git_sha: null
	}
];

interface LoadResult {
	org: Organization;
	range: TimeRange;
	results: Result[];
	summary: ResultSummary;
	trend: TrendPoint[];
	error: string | null;
}

function makeEvent(opts: {
	accessToken?: string | null;
	slug?: string;
	range?: string;
}): Parameters<typeof load>[0] {
	const { accessToken = 'tok123', slug = 'acme', range } = opts;
	const searchParams = new URLSearchParams(range ? { range } : {});
	return {
		locals: { accessToken, user: null },
		params: { slug },
		url: { pathname: `/org/${slug}/results`, search: range ? `?range=${range}` : '', searchParams },
		fetch: vi.fn()
	} as unknown as Parameters<typeof load>[0];
}

beforeEach(() => {
	vi.mocked(organizationsApi.findBySlug).mockResolvedValue(teamOrg);
	vi.mocked(resultsApi.list).mockResolvedValue(sampleResults);
});

describe('results page loader', () => {
	const cases = [
		{ name: 'redirects to login when no token', accessToken: null, expectThrows: true },
		{ name: '404 when org not found', orgNull: true, expectThrows: true },
		{ name: 'redirects when non-team tier', nonTeamTier: true, expectThrows: true },
		{ name: 'returns summary + trend for team-tier happy path', expectSummary: true },
		{ name: 'returns error field + empty summary when api throws', apiThrows: true, expectError: true },
		{ name: 'parses ?range=7d and passes it through', range: '7d', expectRange: '7d' },
		{ name: 'invalid range falls back to 30d', range: 'invalid', expectRange: '30d' }
	];

	for (const c of cases) {
		it(c.name, async () => {
			// Set up mocks per case
			if ('orgNull' in c && c.orgNull) {
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue(null);
			}
			if ('nonTeamTier' in c && c.nonTeamTier) {
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
					...teamOrg,
					tier: 'professional'
				});
			}
			if ('apiThrows' in c && c.apiThrows) {
				vi.mocked(resultsApi.list).mockRejectedValue(new Error('server error'));
			}

			const accessToken = ('accessToken' in c ? c.accessToken : 'tok123') as string | null;
			const event = makeEvent({
				accessToken,
				range: ('range' in c ? c.range : undefined) as string | undefined
			});

			if (c.expectThrows) {
				await expect(load(event)).rejects.toBeDefined();
			} else {
				// Use unknown → LoadResult cast to avoid SvelteKit's void union
				const result = (await load(event)) as unknown as LoadResult;
				if (c.expectSummary) {
					expect(result.summary).toBeDefined();
					expect(result.trend).toBeDefined();
					expect(result.results).toBeDefined();
				}
				if (c.expectError) {
					expect(result.error).toBeTruthy();
					expect(result.summary.total_runs).toBe(0);
				}
				if (c.expectRange) {
					expect(result.range).toBe(c.expectRange);
				}
			}

			// Reset org mock to default after each case
			vi.mocked(organizationsApi.findBySlug).mockResolvedValue(teamOrg);
			vi.mocked(resultsApi.list).mockResolvedValue(sampleResults);
		});
	}
});

// ── SSO page loader tests ────────────────────────────────────────────────────

const emptySsoConfig: SsoConfigView = {
	sso_enabled: false,
	sso_provider: null,
	saml_config: null,
	oidc_config: null
};

function makeSsoEvent(opts: {
	accessToken?: string | null;
	slug?: string;
}): Parameters<typeof ssoLoad>[0] {
	const { accessToken = 'tok123', slug = 'acme' } = opts;
	return {
		locals: { accessToken, user: null },
		params: { slug },
		url: { pathname: `/org/${slug}/settings/sso`, search: '', searchParams: new URLSearchParams() },
		fetch: vi.fn()
	} as unknown as Parameters<typeof ssoLoad>[0];
}

describe('sso page loader', () => {
	const cases: Array<{
		name: string;
		accessToken?: string | null;
		orgNull?: boolean;
		nonOwnerRole?: Organization['role'];
		nonEnterpriseTier?: boolean;
		apiThrows?: boolean;
		ssoProvider?: 'saml' | 'oidc';
		expectThrows?: boolean;
		expectError?: boolean;
		expectProvider?: 'saml' | 'oidc';
	}> = [
		{ name: 'redirects to login when no token', accessToken: null, expectThrows: true },
		{ name: '404 when org not found', orgNull: true, expectThrows: true },
		{ name: 'redirects admin role (non-owner)', nonOwnerRole: 'admin', expectThrows: true },
		{ name: 'redirects member role', nonOwnerRole: 'member', expectThrows: true },
		{ name: 'redirects when non-enterprise tier', nonEnterpriseTier: true, expectThrows: true },
		{ name: 'returns empty config when SSO not configured', expectProvider: undefined },
		{
			name: 'returns saml config when provider=saml',
			ssoProvider: 'saml',
			expectProvider: 'saml'
		},
		{
			name: 'returns oidc config when provider=oidc',
			ssoProvider: 'oidc',
			expectProvider: 'oidc'
		},
		{ name: 'returns error field when ssoApi.get throws', apiThrows: true, expectError: true }
	];

	beforeEach(() => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise' });
		vi.mocked(ssoApi.get).mockResolvedValue(emptySsoConfig);
	});

	for (const c of cases) {
		it(c.name, async () => {
			if (c.orgNull) {
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue(null);
			}
			if (c.nonOwnerRole) {
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
					...teamOrg,
					tier: 'enterprise',
					role: c.nonOwnerRole
				});
			}
			if (c.nonEnterpriseTier) {
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
					...teamOrg,
					tier: 'professional'
				});
			}
			if (c.ssoProvider === 'saml') {
				vi.mocked(ssoApi.get).mockResolvedValue({
					sso_enabled: true,
					sso_provider: 'saml',
					saml_config: {
						idp_metadata_url: 'https://idp.example.com/metadata',
						acs_url: 'https://sp.example.com/acs',
						entity_id: 'https://sp.example.com',
						idp_sso_url: null
					},
					oidc_config: null
				});
			}
			if (c.ssoProvider === 'oidc') {
				vi.mocked(ssoApi.get).mockResolvedValue({
					sso_enabled: true,
					sso_provider: 'oidc',
					saml_config: null,
					oidc_config: {
						issuer_url: 'https://idp.example.com',
						client_id: 'my_client',
						redirect_uri: 'https://sp.example.com/oidc/callback',
						scopes: 'openid email profile'
					}
				});
			}
			if (c.apiThrows) {
				vi.mocked(ssoApi.get).mockRejectedValue(new Error('api error'));
			}

			const accessToken = ('accessToken' in c ? c.accessToken : 'tok123') as string | null;
			const event = makeSsoEvent({ accessToken });

			if (c.expectThrows) {
				await expect(ssoLoad(event)).rejects.toBeDefined();
			} else {
				interface SsoLoadResult {
					org: Organization;
					config: SsoConfigView;
					error: string | null;
				}
				const result = (await ssoLoad(event)) as unknown as SsoLoadResult;
				if (c.expectError) {
					expect(result.error).toBeTruthy();
				} else {
					expect(result.error).toBeNull();
				}
				if (c.expectProvider) {
					expect(result.config.sso_provider).toBe(c.expectProvider);
				}
			}

			// Reset mocks
			vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'enterprise' });
			vi.mocked(ssoApi.get).mockResolvedValue(emptySsoConfig);
		});
	}
});

// ── Audit log page loader tests ─────────────────────────────────────────────

import { load as auditLoad } from '../routes/(app)/org/[slug]/audit-log/+page.server';
import { auditLogApi } from '$lib/api/audit-log';
import type { AuditLogEntry } from '$lib/types/audit-log';

const sampleEntries: AuditLogEntry[] = Array.from({ length: 15 }, (_, i) => ({
	event_type: i % 2 === 0 ? 'member.invited' : 'sso.login',
	user_id: 'user_1',
	user_email: 'user@example.com',
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
		url: {
			pathname: `/org/${slug}/audit-log`,
			search: sp.toString() ? `?${sp}` : '',
			searchParams: sp
		},
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
		{
			name: 'redirects when role is member (admin_required)',
			memberRole: true,
			expectThrows: true
		},
		{ name: 'first page returns 10 entries newest-first', expectPage: 1, expectCount: 10 },
		{
			name: 'page=2 returns remaining 5 entries',
			query: { page: '2' },
			expectPage: 2,
			expectCount: 5
		},
		{ name: 'page beyond last is clamped', query: { page: '99' }, expectClamped: true },
		{
			name: 'event_type query param flows into filter',
			query: { event_type: 'member.invited' },
			expectFilter: { event_type: 'member.invited' }
		},
		{
			name: 'from/to query params flow into filter',
			query: { from: '2026-04-01', to: '2026-04-30' },
			expectFilter: { from: '2026-04-01', to: '2026-04-30' }
		},
		{ name: 'api throws -> error field populated', apiThrows: true, expectError: true }
	];

	for (const c of cases) {
		it(c.name, async () => {
			if ('orgNull' in c && c.orgNull)
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue(null);
			if ('nonEnterprise' in c && c.nonEnterprise)
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
					...teamOrg,
					tier: 'professional'
				});
			if ('memberRole' in c && c.memberRole)
				vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
					...teamOrg,
					tier: 'enterprise',
					role: 'member'
				});
			if ('apiThrows' in c && c.apiThrows)
				vi.mocked(auditLogApi.list).mockRejectedValue(new Error('boom'));

			const accessToken = ('accessToken' in c ? c.accessToken : 'tok123') as string | null;
			const event = makeAuditEvent({
				accessToken,
				query: c.query as Record<string, string> | undefined
			});

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

// ── Audit log export endpoint tests ─────────────────────────────────────────

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
		const req = makeAuditEvent({
			query: { event_type: 'member.invited' }
		}) as unknown as Parameters<typeof auditExport>[0];
		await auditExport(req);
		expect(vi.mocked(auditLogApi.list)).toHaveBeenCalledWith(
			expect.any(String),
			expect.objectContaining({ event_type: 'member.invited' })
		);
	});

	it('redirects non-admin (via requireOrgAdmin)', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
			...teamOrg,
			tier: 'enterprise',
			role: 'member'
		});
		const req = makeAuditEvent({ query: {} }) as unknown as Parameters<typeof auditExport>[0];
		await expect(auditExport(req)).rejects.toBeDefined();
	});
});

// ── Roles page loader tests ──────────────────────────────────────────────────

const enterpriseOwnerOrg: Organization = { ...teamOrg, tier: 'enterprise', role: 'owner' };

const builtinOwnerRole: RoleView = {
	id: 'builtin_owner',
	name: 'owner',
	permissions: [],
	is_builtin: true,
	created_at: null
};

const builtinMemberRole: RoleView = {
	id: 'builtin_member',
	name: 'member',
	permissions: [],
	is_builtin: true,
	created_at: null
};

const customQaLeadRole: RoleView = {
	id: 'role_qa_lead',
	name: 'qa-lead',
	permissions: ['results.view', 'results.upload'],
	is_builtin: false,
	created_at: '2026-04-15T00:00:00Z'
};

const sampleRoles: RoleView[] = [builtinOwnerRole, builtinMemberRole, customQaLeadRole];

function makeRolesEvent(opts: {
	accessToken?: string | null;
	slug?: string;
}): Parameters<typeof rolesLoad>[0] {
	const { accessToken = 'tok123', slug = 'acme' } = opts;
	return {
		locals: { accessToken, user: null },
		params: { slug },
		url: {
			pathname: `/org/${slug}/settings/roles`,
			search: '',
			searchParams: new URLSearchParams()
		},
		fetch: vi.fn()
	} as unknown as Parameters<typeof rolesLoad>[0];
}

describe('roles page loader', () => {
	beforeEach(() => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue(enterpriseOwnerOrg);
		vi.mocked(rolesApi.list).mockResolvedValue(sampleRoles);
		vi.mocked(membersApi.list).mockResolvedValue([]);
	});

	it('redirects to login when no token', async () => {
		const event = makeRolesEvent({ accessToken: null });
		await expect(rolesLoad(event)).rejects.toBeDefined();
	});

	it('redirects when org is on professional tier (not enterprise or team)', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
			...teamOrg,
			tier: 'professional' as const,
			role: 'owner' as const
		});
		const event = makeRolesEvent({});
		await expect(rolesLoad(event)).rejects.toBeDefined();
	});

	it('redirects non-owner (admin role)', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({
			...teamOrg,
			tier: 'enterprise',
			role: 'admin'
		});
		const event = makeRolesEvent({});
		await expect(rolesLoad(event)).rejects.toBeDefined();
	});

	it('happy path returns roles and org', async () => {
		const event = makeRolesEvent({});
		const result = (await rolesLoad(event)) as unknown as {
			org: Organization;
			roles: RoleView[];
			memberCounts: Record<string, number>;
			error: string | null;
		};
		expect(result.error).toBeNull();
		expect(result.roles).toHaveLength(3);
		expect(result.org.id).toBe('org_1');
	});

	it('counts custom role members correctly via role_id', async () => {
		vi.mocked(membersApi.list).mockResolvedValue(
			[
				{ user_id: 'u2', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-02-01T00:00:00Z' },
				{ user_id: 'u3', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-03-01T00:00:00Z' }
			] as unknown as Awaited<ReturnType<typeof membersApi.list>>
		);
		const event = makeRolesEvent({});
		const result = (await rolesLoad(event)) as unknown as {
			memberCounts: Record<string, number>;
			error: string | null;
		};
		expect(result.error).toBeNull();
		// 2 members with role_id=role_qa_lead
		expect(result.memberCounts['role_qa_lead']).toBe(2);
		// built-in roles have 0 custom members assigned via role_id
		expect(result.memberCounts['builtin_member']).toBe(0);
	});

	it('counts built-in role members correctly via role name', async () => {
		vi.mocked(membersApi.list).mockResolvedValue(
			[
				// Two members whose role field is 'owner' (built-in)
				{ user_id: 'u1', role: 'owner', joined_at: '2026-01-01T00:00:00Z' },
				{ user_id: 'u4', role: 'owner', joined_at: '2026-04-01T00:00:00Z' }
			] as unknown as Awaited<ReturnType<typeof membersApi.list>>
		);
		const event = makeRolesEvent({});
		const result = (await rolesLoad(event)) as unknown as {
			memberCounts: Record<string, number>;
			error: string | null;
		};
		expect(result.error).toBeNull();
		// Built-in owner role: id='builtin_owner', name='owner'. Members with
		// role='owner' must be counted under 'builtin_owner' (not 'owner').
		expect(result.memberCounts['builtin_owner']).toBe(2);
		expect(result.memberCounts['builtin_member']).toBe(0);
		expect(result.memberCounts['role_qa_lead']).toBe(0);
	});

	it('returns error field and empty roles when API throws', async () => {
		vi.mocked(rolesApi.list).mockRejectedValue(new Error('api error'));
		const event = makeRolesEvent({});
		const result = (await rolesLoad(event)) as unknown as {
			roles: RoleView[];
			memberCounts: Record<string, number>;
			error: string | null;
		};
		expect(result.error).toBeTruthy();
		expect(result.roles).toHaveLength(0);
	});
});

// ── Auth: password-reset pages ───────────────────────────────────────────────

vi.mock('$lib/api/auth', () => ({
	authApi: {
		requestPasswordReset: vi.fn(),
		confirmPasswordReset: vi.fn(),
		resendEmailVerification: vi.fn(),
		confirmEmailVerification: vi.fn()
	}
}));

import {
	load as prConfirmLoad,
	actions as prConfirmActions
} from '../routes/auth/password-reset/confirm/+page.server';
import { actions as prRequestActions } from '../routes/auth/password-reset/request/+page.server';
import {
	load as evConfirmLoad
} from '../routes/auth/email-verification/confirm/+page.server';
import { actions as evRequestActions } from '../routes/auth/email-verification/request/+page.server';
import { authApi } from '$lib/api/auth';
import { ApiError } from '$lib/types/api-error';

function makeSearchParams(params: Record<string, string>) {
	return new URLSearchParams(params);
}

function makeAuthEvent(opts: {
	params?: Record<string, string>;
	formData?: Record<string, string>;
}): Record<string, unknown> {
	const { params = {}, formData = {} } = opts;
	const sp = makeSearchParams(params);
	return {
		url: { searchParams: sp },
		request: {
			formData: () =>
				Promise.resolve({
					get: (key: string) => formData[key] ?? null
				})
		},
		fetch: vi.fn()
	};
}

describe('auth password-reset confirm page', () => {
	beforeEach(() => {
		vi.mocked(authApi.confirmPasswordReset).mockResolvedValue({ ok: true });
	});

	it('load returns token and hasToken=true when ?token= present', async () => {
		const event = makeAuthEvent({ params: { token: 'prst_xxx' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmLoad(event as any);
		expect((result as Record<string, unknown>).token).toBe('prst_xxx');
		expect((result as Record<string, unknown>).hasToken).toBe(true);
	});

	it('load returns hasToken=false when ?token= missing', async () => {
		const event = makeAuthEvent({ params: {} });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmLoad(event as any);
		expect((result as Record<string, unknown>).hasToken).toBe(false);
	});

	it('default action redirects on 200', async () => {
		const event = makeAuthEvent({
			formData: { token: 'prst_xxx', new_password: 'StrongP@ss1!' }
		});
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await expect(prConfirmActions.default(event as any)).rejects.toMatchObject({
			status: 303,
			location: expect.stringMatching(/\/login\?toast=password_reset_success/)
		});
	});

	it('default action returns fail(422, weak_password, score) on 422', async () => {
		vi.mocked(authApi.confirmPasswordReset).mockRejectedValue(
			new ApiError('server_error', 'weak', 422, undefined, { score: 1 }, 'https://apitool.dev/errors/password-too-weak')
		);
		const event = makeAuthEvent({
			formData: { token: 'prst_xxx', new_password: 'weak' }
		});
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmActions.default(event as any);
		expect(result).toMatchObject({ status: 422, data: { error: 'weak_password', score: 1 } });
	});

	it('default action returns fail(400, token_invalid) on 400', async () => {
		vi.mocked(authApi.confirmPasswordReset).mockRejectedValue(
			new ApiError('server_error', 'invalid', 400, undefined, undefined, 'https://apitool.dev/errors/password-reset-token-invalid')
		);
		const event = makeAuthEvent({
			formData: { token: 'bad', new_password: 'StrongP@ss1!' }
		});
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmActions.default(event as any);
		expect(result).toMatchObject({ status: 400, data: { error: 'token_invalid' } });
	});

	it('default action returns fail(500, server_error) on unexpected error', async () => {
		vi.mocked(authApi.confirmPasswordReset).mockRejectedValue(new Error('network failure'));
		const event = makeAuthEvent({
			formData: { token: 'prst_xxx', new_password: 'StrongP@ss1!' }
		});
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmActions.default(event as any);
		expect(result).toMatchObject({ status: 500, data: { error: 'server_error' } });
	});

	it('default action returns fail(400, missing_field) when token empty', async () => {
		const event = makeAuthEvent({ formData: { token: '', new_password: 'pw' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmActions.default(event as any);
		expect(result).toMatchObject({ status: 400, data: { error: 'missing_field' } });
	});

	it('default action returns fail(400, missing_field) when password empty', async () => {
		const event = makeAuthEvent({ formData: { token: 'prst_xxx', new_password: '' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prConfirmActions.default(event as any);
		expect(result).toMatchObject({ status: 400, data: { error: 'missing_field' } });
	});
});

describe('auth password-reset request page', () => {
	beforeEach(() => {
		vi.mocked(authApi.requestPasswordReset).mockResolvedValue({ ok: true, message: 'If that email exists, a reset link has been sent.' });
	});

	it('default action returns { submitted: true } on success', async () => {
		const event = makeAuthEvent({ formData: { email: 'user@example.com' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prRequestActions.default(event as any);
		expect(result).toMatchObject({ submitted: true });
	});

	it('default action returns { submitted: true } regardless of backend error (enumeration defense)', async () => {
		vi.mocked(authApi.requestPasswordReset).mockRejectedValue(new Error('network error'));
		const event = makeAuthEvent({ formData: { email: 'notfound@example.com' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await prRequestActions.default(event as any);
		expect(result).toMatchObject({ submitted: true });
	});
});

describe('auth email-verification confirm page', () => {
	beforeEach(() => {
		vi.mocked(authApi.confirmEmailVerification).mockResolvedValue({ ok: true });
	});

	it('load with valid token redirects to /?toast=email_verified', async () => {
		const event = makeAuthEvent({ params: { token: 'evtk_xxx' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await expect(evConfirmLoad(event as any)).rejects.toMatchObject({
			status: 303,
			location: expect.stringMatching(/\/\?toast=email_verified/)
		});
	});

	it('load with invalid token returns error=token_invalid', async () => {
		vi.mocked(authApi.confirmEmailVerification).mockRejectedValue(
			new ApiError('server_error', 'invalid', 400, undefined, undefined, 'https://apitool.dev/errors/email-verification-token-invalid')
		);
		const event = makeAuthEvent({ params: { token: 'bad' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await evConfirmLoad(event as any);
		expect((result as Record<string, unknown>).error).toBe('token_invalid');
	});

	it('load with missing token returns error=missing_token', async () => {
		const event = makeAuthEvent({ params: {} });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await evConfirmLoad(event as any);
		expect((result as Record<string, unknown>).error).toBe('missing_token');
	});

	it('load returns error=server_error on unexpected exception', async () => {
		vi.mocked(authApi.confirmEmailVerification).mockRejectedValue(new Error('network failure'));
		const event = makeAuthEvent({ params: { token: 'evtk_xxx' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await evConfirmLoad(event as any);
		expect((result as Record<string, unknown>).error).toBe('server_error');
	});
});

describe('auth email-verification request page', () => {
	beforeEach(() => {
		vi.mocked(authApi.resendEmailVerification).mockResolvedValue({ ok: true });
	});

	it('default action returns { submitted: true } on success', async () => {
		const event = makeAuthEvent({ formData: { email: 'user@example.com' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await evRequestActions.default(event as any);
		expect(result).toMatchObject({ submitted: true });
	});

	it('default action returns { submitted: true } regardless of backend error (enumeration defense)', async () => {
		vi.mocked(authApi.resendEmailVerification).mockRejectedValue(new Error('network error'));
		const event = makeAuthEvent({ formData: { email: 'notfound@example.com' } });
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = await evRequestActions.default(event as any);
		expect(result).toMatchObject({ submitted: true });
	});
});

// ── Schedules page loader tests (M16-012) ─────────────────────────────────────

import { load as schedulesLoad } from '../routes/(app)/org/[slug]/schedules/+page.server';
import { load as scheduleRunsLoad } from '../routes/(app)/org/[slug]/schedules/[scheduleName]/+page.server';
import { schedulesApi } from '$lib/api/schedules';
import type { Schedule, ScheduledRun } from '$lib/types/schedules';

const sampleSchedules: Schedule[] = [
	{
		id: 'sched_1',
		name: 'nightly',
		cron_expression: '0 2 * * *',
		timezone: 'Europe/Stockholm',
		collection_ref: 'smoke.yaml',
		enabled: true,
		next_run_at: '2026-05-12T01:00:00Z',
		last_run_at: null,
		created_at: '2026-05-11T00:00:00Z'
	}
];

const sampleRuns: ScheduledRun[] = [
	{
		run_id: 'run_1',
		status: 'completed',
		created_at: '2026-05-11T01:00:00Z',
		started_at: '2026-05-11T01:00:05Z',
		completed_at: '2026-05-11T01:00:35Z',
		result_id: 'res_abc'
	}
];

function makeSchedulesEvent(opts: { accessToken?: string | null; slug?: string; scheduleName?: string }) {
	const { accessToken = 'tok123', slug = 'acme', scheduleName } = opts;
	return {
		locals: { accessToken, user: null },
		params: scheduleName ? { slug, scheduleName } : { slug },
		url: { pathname: `/org/${slug}/schedules`, search: '', searchParams: new URLSearchParams() },
		fetch: vi.fn()
	} as unknown as Parameters<typeof schedulesLoad>[0];
}

describe('schedules list page loader', () => {
	beforeEach(() => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue(teamOrg);
		vi.mocked(schedulesApi.list).mockResolvedValue(sampleSchedules);
		vi.mocked(schedulesApi.listRuns).mockResolvedValue(sampleRuns);
	});

	it('redirects to login when no token', async () => {
		const event = makeSchedulesEvent({ accessToken: null });
		await expect(schedulesLoad(event)).rejects.toBeDefined();
	});

	it('redirects free-tier org with team_tier_required toast', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'professional' as const });
		const event = makeSchedulesEvent({});
		await expect(schedulesLoad(event)).rejects.toBeDefined();
	});

	it('returns schedules + matching last-run array for team-tier org', async () => {
		const event = makeSchedulesEvent({});
		const result = (await schedulesLoad(event)) as unknown as {
			schedules: Schedule[];
			lastRuns: Array<ScheduledRun | null>;
			error: string | null;
		};
		expect(result.schedules).toHaveLength(1);
		expect(result.lastRuns).toHaveLength(1);
		expect(result.lastRuns[0]?.run_id).toBe('run_1');
		expect(result.error).toBeNull();
	});

	it('returns error string when API throws', async () => {
		vi.mocked(schedulesApi.list).mockRejectedValue(new Error('network failure'));
		const event = makeSchedulesEvent({});
		const result = (await schedulesLoad(event)) as unknown as {
			schedules: Schedule[];
			error: string | null;
		};
		expect(result.error).toBeTruthy();
		expect(result.schedules).toHaveLength(0);
	});
});

describe('schedule run-history page loader', () => {
	beforeEach(() => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue(teamOrg);
		vi.mocked(schedulesApi.get).mockResolvedValue(sampleSchedules[0]);
		vi.mocked(schedulesApi.listRuns).mockResolvedValue(sampleRuns);
	});

	it('loads schedule + run history for team-tier admin', async () => {
		const event = makeSchedulesEvent({ scheduleName: 'nightly' }) as unknown as Parameters<typeof scheduleRunsLoad>[0];
		const result = (await scheduleRunsLoad(event)) as unknown as {
			schedule: Schedule;
			runs: ScheduledRun[];
			error: string | null;
		};
		expect(result.schedule?.name).toBe('nightly');
		expect(result.runs).toHaveLength(1);
		expect(result.error).toBeNull();
	});

	it('throws 404 when schedule name not found', async () => {
		vi.mocked(schedulesApi.get).mockRejectedValue(
			new ApiError('schedule_not_found', 'not found', 404)
		);
		const event = makeSchedulesEvent({ scheduleName: 'ghost' }) as unknown as Parameters<typeof scheduleRunsLoad>[0];
		await expect(scheduleRunsLoad(event)).rejects.toMatchObject({ status: 404 });
	});
});

// ── Dashboard page loader tests (M16-020) ─────────────────────────────────────

const sampleStats: StatsResponse = {
	window: '30d',
	window_start: '2026-04-12T00:00:00Z',
	window_end: '2026-05-12T00:00:00Z',
	totals: {
		runs: 42,
		pass_count: 38,
		fail_count: 4,
		skipped_count: 0,
		pass_rate: 0.9048,
		avg_duration_ms: 350,
		p50_duration_ms: 300,
		p95_duration_ms: 900
	},
	trend: []
};

const sampleFailures: FailuresResponse = {
	window: '30d',
	limit: 10,
	limit_clamped: false,
	items: [
		{
			method: 'GET',
			path_template: '/api/users/:id',
			failure_count: 3,
			first_seen_at: '2026-05-01T00:00:00Z',
			last_seen_at: '2026-05-11T00:00:00Z',
			sample_run_ids: ['run_abc']
		}
	]
};

function makeDashboardEvent(opts: {
	accessToken?: string | null;
	slug?: string;
	window?: string;
}): Parameters<typeof dashboardLoad>[0] {
	const { accessToken = 'tok123', slug = 'acme', window: win } = opts;
	const sp = new URLSearchParams(win ? { window: win } : {});
	return {
		locals: { accessToken, user: null },
		params: { slug },
		url: {
			pathname: `/org/${slug}/dashboard`,
			search: win ? `?window=${win}` : '',
			searchParams: sp
		},
		fetch: vi.fn()
	} as unknown as Parameters<typeof dashboardLoad>[0];
}

describe('dashboard page loader', () => {
	beforeEach(() => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue(teamOrg);
		vi.mocked(dashboardApi.getStats).mockResolvedValue(sampleStats);
		vi.mocked(dashboardApi.getFailures).mockResolvedValue(sampleFailures);
		vi.mocked(resultsApi.list).mockResolvedValue(sampleResults);
	});

	it('redirects to login when no token', async () => {
		const event = makeDashboardEvent({ accessToken: null });
		await expect(dashboardLoad(event)).rejects.toBeDefined();
	});

	it('throws 404 when org not found', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue(null);
		const event = makeDashboardEvent({});
		await expect(dashboardLoad(event)).rejects.toMatchObject({ status: 404 });
	});

	it('free-tier short-circuit returns tierGate:true without calling getStats', async () => {
		vi.mocked(organizationsApi.findBySlug).mockResolvedValue({ ...teamOrg, tier: 'free' });
		const event = makeDashboardEvent({});
		const result = (await dashboardLoad(event)) as unknown as {
			tierGate: boolean;
			stats: null;
		};
		expect(result.tierGate).toBe(true);
		expect(result.stats).toBeNull();
		expect(vi.mocked(dashboardApi.getStats)).not.toHaveBeenCalled();
	});

	it('402 from getStats sets tierGate without crashing', async () => {
		vi.mocked(dashboardApi.getStats).mockRejectedValue(
			new ApiError('dashboard_tier_ineligible', 'Tier ineligible', 402)
		);
		const event = makeDashboardEvent({});
		const result = (await dashboardLoad(event)) as unknown as {
			tierGate: boolean;
			stats: null;
		};
		expect(result.tierGate).toBe(true);
		expect(result.stats).toBeNull();
	});

	it('400 from getStats sets windowError without crashing', async () => {
		vi.mocked(dashboardApi.getStats).mockRejectedValue(
			new ApiError('unsupported_window', 'Allowed values: 7d, 30d, 90d.', 400)
		);
		const event = makeDashboardEvent({ window: '14d' });
		const result = (await dashboardLoad(event)) as unknown as {
			windowError: string | null;
			stats: null;
		};
		expect(result.windowError).toBeTruthy();
		expect(result.stats).toBeNull();
	});

	it('generic error from getStats sets loadError without crashing', async () => {
		vi.mocked(dashboardApi.getStats).mockRejectedValue(new Error('network failure'));
		const event = makeDashboardEvent({});
		const result = (await dashboardLoad(event)) as unknown as {
			loadError: string | null;
			stats: null;
		};
		expect(result.loadError).toBeTruthy();
		expect(result.stats).toBeNull();
	});

	it('happy path returns stats, failures, and recentRuns', async () => {
		const event = makeDashboardEvent({ window: '30d' });
		const result = (await dashboardLoad(event)) as unknown as {
			stats: StatsResponse;
			failures: FailuresResponse;
			recentRuns: Result[];
			tierGate: boolean;
			windowError: string | null;
			loadError: string | null;
		};
		expect(result.stats?.totals.runs).toBe(42);
		expect(result.failures?.items).toHaveLength(1);
		expect(result.recentRuns).toHaveLength(1);
		expect(result.tierGate).toBe(false);
		expect(result.windowError).toBeNull();
		expect(result.loadError).toBeNull();
	});

	it('recent-runs failure degrades gracefully (returns empty array)', async () => {
		vi.mocked(resultsApi.list).mockRejectedValue(new Error('results endpoint down'));
		const event = makeDashboardEvent({});
		const result = (await dashboardLoad(event)) as unknown as {
			stats: StatsResponse;
			recentRuns: Result[];
		};
		expect(result.stats?.totals.runs).toBe(42);
		expect(result.recentRuns).toHaveLength(0);
	});
});
