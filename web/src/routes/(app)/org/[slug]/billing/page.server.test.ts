import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page.server';

const findBySlug = vi.fn();
const getSubscription = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/subscriptions', () => ({
	subscriptionsApi: { get: (...a: unknown[]) => getSubscription(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const SUB = { id: 'sub_test', tier: 'team', seat_count: 3, seat_limit: 10 };

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/billing'),
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
	getSubscription.mockResolvedValue({ subscription: SUB, tier: 'team' });
});

describe('billing load', () => {
	it('returns the subscription for a team-tier owner', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.subscription).toEqual(SUB);
		expect(r.tier).toBe('team');
		expect(r.error).toBeNull();
	});

	it('degrades to the org tier and an error message when the fetch fails', async () => {
		getSubscription.mockRejectedValue(new Error('billing down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('billing down');
		expect(r.subscription).toBeNull();
		// Falls back to the tier already known from the org record.
		expect(r.tier).toBe('team');
	});

	it('redirects non-owners with the owner_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'admin' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=owner_required');
	});

	it('redirects below team tier before checking the role', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member', tier: 'professional' });

		expect((await loadThrows(event())).location).toBe('/org/acme?toast=team_tier_required');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fbilling');
	});
});
