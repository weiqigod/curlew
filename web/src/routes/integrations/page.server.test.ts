import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import { load } from './+page.server';

const listOrgs = vi.fn();
const apiGet = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { list: (...a: unknown[]) => listOrgs(...a) }
}));
vi.mock('$lib/api/client', () => ({
	api: { get: (...a: unknown[]) => apiGet(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const INSTALL_URL = 'https://github.com/apps/curlew-checks-test/installations/new?state=abc';
const INSTALLATION = {
	installation_id: 12345,
	account_login: 'curlew-checks-test',
	account_type: 'Organization',
	repo_set: [],
	claimed_at: '2026-05-01T00:00:00Z',
	suspended_at: null
};

/** Routes the two GET calls this loader makes by path. */
function routeGet(handlers: {
	installUrl?: () => unknown;
	state?: () => unknown;
}) {
	apiGet.mockImplementation((path: string) => {
		if (path.endsWith('/install-url')) {
			return Promise.resolve(
				handlers.installUrl ? handlers.installUrl() : { install_url: INSTALL_URL }
			);
		}
		return Promise.resolve(
			handlers.state ? handlers.state() : { installation: INSTALLATION }
		);
	});
}

function event(search = '') {
	return {
		locals: { accessToken: 'tok' },
		url: new URL(`http://localhost/integrations${search}`),
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
	listOrgs.mockResolvedValue([ORG]);
	routeGet({});
});

describe('integrations load', () => {
	it('returns the install URL and state for an admin', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.isAdmin).toBe(true);
		expect(r.installUrl).toBe(INSTALL_URL);
		expect(r.installation).toEqual(INSTALLATION);
		expect(r.stateError).toBeNull();
	});

	it.each(['owner', 'admin'])('treats %s as an admin', async (role) => {
		listOrgs.mockResolvedValue([{ ...ORG, role }]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event())) as any).isAdmin).toBe(true);
	});

	it('never mints an install URL for a member', async () => {
		listOrgs.mockResolvedValue([{ ...ORG, role: 'member' }]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.isAdmin).toBe(false);
		expect(r.installUrl).toBeNull();
		// The state call still runs so members get the read-only view.
		expect(r.installation).toEqual(INSTALLATION);
		expect(apiGet).toHaveBeenCalledTimes(1);
	});

	it('treats a 404 install state as not-yet-installed, not an error', async () => {
		routeGet({
			state: () => {
				throw new ApiError('not_installed', 'no install', 404);
			}
		});

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.installation).toBeNull();
		expect(r.stateError).toBeNull();
	});

	it('reports a non-404 install-state failure', async () => {
		routeGet({
			state: () => {
				throw new ApiError('server_error', 'state unavailable', 500);
			}
		});

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.stateError).toBe('state unavailable');
		expect(r.installation).toBeNull();
	});

	it('hides the Connect button silently when the install URL 4xxs', async () => {
		routeGet({
			installUrl: () => {
				throw new ApiError('bad_request', 'no app configured', 400);
			}
		});

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.installUrl).toBeNull();
		// A 4xx here is not worth alarming the user about.
		expect(r.stateError).toBeNull();
	});

	it('reports a 5xx from the install-url endpoint', async () => {
		routeGet({
			installUrl: () => {
				throw new ApiError('server_error', 'minting failed', 503);
			}
		});

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.installUrl).toBeNull();
		expect(r.stateError).toBe('minting failed');
	});

	it('renders an org-less state when the user belongs to no organization', async () => {
		listOrgs.mockResolvedValue([]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.org).toBeNull();
		expect(r.isAdmin).toBe(false);
		// Neither call is worth making without an org.
		expect(apiGet).not.toHaveBeenCalled();
	});

	it('survives a failing organization list', async () => {
		listOrgs.mockRejectedValue(new Error('orgs down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.org).toBeNull();
		expect(r.isAdmin).toBe(false);
	});

	it('flags the one-time toast only for ?installed=true', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event('?installed=true'))) as any).showInstalledToast).toBe(true);
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event())) as any).showInstalledToast).toBe(false);
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event('?installed=1'))) as any).showInstalledToast).toBe(false);
	});

	it('redirects to /login preserving the query string', async () => {
		const e = await loadThrows({ ...event('?installed=true'), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Fintegrations%3Finstalled%3Dtrue');
	});
});
