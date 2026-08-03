import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { RoleView } from '$lib/types/roles';
import type { Member } from '$lib/types/members';
import type { Organization } from '$lib/types/organization';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listRoles = vi.fn();
const listMembers = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/roles', () => ({
	rolesApi: { list: (...a: unknown[]) => listRoles(...a) }
}));
vi.mock('$lib/api/members', () => ({
	membersApi: { list: (...a: unknown[]) => listMembers(...a) }
}));

const ORG: Organization = {
	id: 'org_1',
	slug: 'acme',
	name: 'Acme',
	role: 'owner',
	tier: 'enterprise',
	seat_count: 3,
	seat_limit: 50,
	status: 'active',
	created_at: '2026-04-01T00:00:00Z'
} as Organization;

const ROLES: RoleView[] = [
	{ id: 'builtin_owner', name: 'owner', permissions: [], is_builtin: true, created_at: null },
	{ id: 'builtin_member', name: 'member', permissions: [], is_builtin: true, created_at: null },
	{
		id: 'role_qa_lead',
		name: 'qa-lead',
		permissions: ['results.view'],
		is_builtin: false,
		created_at: '2026-04-15T00:00:00Z'
	}
];

const MEMBERS = [
	{ user_id: 'u1', role: 'owner', joined_at: '2026-01-01T00:00:00Z' },
	{ user_id: 'u2', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-02-01T00:00:00Z' },
	{ user_id: 'u3', role: 'member', role_id: 'role_qa_lead', joined_at: '2026-03-01T00:00:00Z' }
] as Member[];

/** Builds the RequestEvent fields this load actually reads. */
function event(overrides: Record<string, unknown> = {}) {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/settings/roles'),
		fetch: vi.fn(),
		...overrides
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

/** Runs `load` and returns the thrown redirect/error rather than the result. */
async function loadThrows(ev: unknown) {
	try {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await load(ev as any);
	} catch (e) {
		return e as { status: number; location?: string };
	}
	throw new Error('expected load to throw');
}

beforeEach(() => {
	vi.clearAllMocks();
	findBySlug.mockResolvedValue(ORG);
	listRoles.mockResolvedValue(ROLES);
	listMembers.mockResolvedValue(MEMBERS);
});

describe('roles load', () => {
	it('returns roles and per-role member counts for an enterprise owner', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = (await load(event())) as any;

		expect(result.org).toEqual(ORG);
		expect(result.roles).toEqual(ROLES);
		expect(result.error).toBeNull();
		// u2 and u3 carry role_id=role_qa_lead; u1 is a built-in owner, resolved
		// through the built-in name -> id alias map.
		expect(result.memberCounts).toEqual({
			builtin_owner: 1,
			builtin_member: 0,
			role_qa_lead: 2
		});
	});

	it('passes the access token and SvelteKit fetch through to every API call', async () => {
		const ev = event();
		await load(ev);

		for (const spy of [findBySlug, listRoles, listMembers]) {
			expect(spy).toHaveBeenCalledWith(
				expect.anything(),
				expect.objectContaining({ token: 'tok', fetch: ev.fetch })
			);
		}
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows(event({ locals: { accessToken: null } }));

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fsettings%2Froles');
	});

	it('redirects non-owners with the owner_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=owner_required');
	});

	it('redirects below-enterprise tiers with the enterprise_tier_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, tier: 'team' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=enterprise_tier_required');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		const e = await loadThrows(event());

		expect(e.status).toBe(404);
	});

	it('degrades to an error message when the roles fetch fails', async () => {
		listRoles.mockRejectedValue(new Error('backend down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const result = (await load(event())) as any;

		expect(result.error).toBe('backend down');
		expect(result.roles).toEqual([]);
		expect(result.memberCounts).toEqual({});
		// The org still renders so the page keeps its heading and nav.
		expect(result.org).toEqual(ORG);
	});
});
