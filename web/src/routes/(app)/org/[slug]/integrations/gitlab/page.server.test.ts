import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listInstallations = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/gitlab-integrations', () => ({
	gitlabIntegrationsApi: { list: (...a: unknown[]) => listInstallations(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const INSTALLS = [{ id: 'inst_1', project_path: 'group/project' }];

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/integrations/gitlab'),
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
	listInstallations.mockResolvedValue(INSTALLS);
});

describe('gitlab integrations load', () => {
	it('returns the installation list for a team-tier org', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.installations).toEqual(INSTALLS);
		expect(r.loadError).toBeNull();
	});

	it('degrades to an empty list plus a message when the fetch fails', async () => {
		listInstallations.mockRejectedValue(new Error('gitlab api down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.loadError).toBe('gitlab api down');
		expect(r.installations).toEqual([]);
	});

	it('rethrows a 402 rather than swallowing it into the page', async () => {
		listInstallations.mockRejectedValue(new ApiError('tier_ineligible', 'nope', 402));

		// requireTeamTier should have caught this already; the rethrow is the
		// defensive backstop so a tier failure never renders as a page error.
		expect((await loadThrows(event())).status).toBe(402);
	});

	it('redirects below team tier with the team_tier_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, tier: 'professional' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=team_tier_required');
		expect(listInstallations).not.toHaveBeenCalled();
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fintegrations%2Fgitlab');
	});
});
