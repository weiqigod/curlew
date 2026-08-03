import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page.server';

const findBySlug = vi.fn();
const getSso = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/sso', () => ({
	ssoApi: { get: (...a: unknown[]) => getSso(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'enterprise' };
const CONFIG = { sso_enabled: true, sso_provider: 'saml', saml_config: {}, oidc_config: null };

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/settings/sso'),
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
	getSso.mockResolvedValue(CONFIG);
});

describe('sso load', () => {
	it('returns the SSO config for an enterprise owner', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.config).toEqual(CONFIG);
		expect(r.error).toBeNull();
	});

	it('falls back to a disabled config when the fetch fails', async () => {
		getSso.mockRejectedValue(new Error('sso down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.error).toBe('sso down');
		// The page still has a well-formed config to render its tabs against.
		expect(r.config).toEqual({
			sso_enabled: false,
			sso_provider: null,
			saml_config: null,
			oidc_config: null
		});
	});

	it('redirects non-owners with the owner_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'admin' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=owner_required');
	});

	it('redirects below-enterprise tiers before checking the role', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member', tier: 'team' });

		expect((await loadThrows(event())).location).toBe('/org/acme?toast=enterprise_tier_required');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fsettings%2Fsso');
	});
});
