import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listMembers = vi.fn();
const listInvitations = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/members', () => ({
	membersApi: { list: (...a: unknown[]) => listMembers(...a) }
}));
vi.mock('$lib/api/invitations', () => ({
	invitationsApi: { list: (...a: unknown[]) => listInvitations(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const MEMBERS = [{ user_id: 'u1', role: 'owner' }];
const INVITATIONS = [{ id: 'inv_1', email: 'pending@example.com' }];

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/members'),
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

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
	listMembers.mockResolvedValue(MEMBERS);
	listInvitations.mockResolvedValue(INVITATIONS);
});

describe('members load', () => {
	it('returns members and pending invitations for a team-tier admin', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.members).toEqual(MEMBERS);
		expect(r.invitations).toEqual(INVITATIONS);
		expect(r.error).toBeNull();
	});

	it('admits admins as well as owners', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'admin' });

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event())) as any).org.role).toBe('admin');
	});

	it('redirects members with the admin_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=admin_required');
	});

	it('redirects below team tier before checking the role', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member', tier: 'professional' });

		expect((await loadThrows(event())).location).toBe('/org/acme?toast=team_tier_required');
	});

	it('degrades to an error message when either fetch fails', async () => {
		listInvitations.mockRejectedValue(new Error('invitations down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('invitations down');
		expect(r.members).toEqual([]);
		expect(r.invitations).toEqual([]);
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fmembers');
	});
});
