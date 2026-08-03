import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listRules = vi.fn();
const listDeliveries = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/notifications', () => ({
	notificationsApi: {
		listRules: (...a: unknown[]) => listRules(...a),
		listDeliveries: (...a: unknown[]) => listDeliveries(...a)
	}
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const RULES = [{ id: 'nrule_1', channel: 'slack', target: 'https://x', on: ['run_failed'] }];
const DELIVERIES = [{ id: 'ndel_1', status: 'delivered' }];

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/settings/notifications'),
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
	listRules.mockResolvedValue(RULES);
	listDeliveries.mockResolvedValue(DELIVERIES);
});

describe('notifications load', () => {
	it('returns rules and the delivery log for a team-tier admin', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.rules).toEqual(RULES);
		expect(r.deliveries).toEqual(DELIVERIES);
		expect(r.error).toBeNull();
		// The delivery log is capped server-side.
		expect(listDeliveries).toHaveBeenCalledWith('org_1', expect.objectContaining({ limit: 25 }));
	});

	it('admits admins as well as owners', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'admin' });

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.org.role).toBe('admin');
	});

	it('redirects members with the admin_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=admin_required');
	});

	it('checks tier before role, so a below-tier member gets the tier toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member', tier: 'professional' });

		const e = await loadThrows(event());

		expect(e.location).toBe('/org/acme?toast=team_tier_required');
	});

	it('degrades to an error message when either fetch fails', async () => {
		listDeliveries.mockRejectedValue(new Error('deliveries down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('deliveries down');
		expect(r.rules).toEqual([]);
		expect(r.deliveries).toEqual([]);
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fsettings%2Fnotifications');
	});
});
