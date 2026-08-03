import { beforeEach, describe, expect, it, vi } from 'vitest';
import { load } from './+page.server';

const findBySlug = vi.fn();
const getConfig = vi.fn();
const listAudit = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/vault-config', () => ({
	vaultConfigApi: { get: (...a: unknown[]) => getConfig(...a) }
}));
vi.mock('$lib/api/audit-log', () => ({
	auditLogApi: { list: (...a: unknown[]) => listAudit(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'team' };
const CONFIG = { template: 'team_secrets:\n', version: 1 };
const ENTRIES = [{ event_type: 'vault_config.upserted' }];

function event() {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL('http://localhost/org/acme/vault-config'),
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
	getConfig.mockResolvedValue(CONFIG);
	listAudit.mockResolvedValue(ENTRIES);
});

describe('vault-config load', () => {
	it('returns the current template and the recent vault audit entries', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.currentConfig).toEqual(CONFIG);
		expect(r.auditEntries).toEqual(ENTRIES);
		// Only vault-config events, capped at 10.
		expect(listAudit).toHaveBeenCalledWith(
			'org_1',
			expect.objectContaining({
				event_type: 'vault_config.upserted,vault_config.deleted',
				limit: 10
			})
		);
	});

	it('treats a missing template as null rather than an error', async () => {
		getConfig.mockRejectedValue(new Error('404'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.currentConfig).toBeNull();
		// The audit log still renders alongside the empty editor.
		expect(r.auditEntries).toEqual(ENTRIES);
	});

	it('falls back to an empty audit list when that fetch fails', async () => {
		listAudit.mockRejectedValue(new Error('audit down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.auditEntries).toEqual([]);
		expect(r.currentConfig).toEqual(CONFIG);
	});

	it('redirects below team tier with the team_tier_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, tier: 'professional' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=team_tier_required');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});

	it('redirects to /login when unauthenticated', async () => {
		const e = await loadThrows({ ...event(), locals: { accessToken: null } });

		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?redirect=%2Forg%2Facme%2Fvault-config');
	});
});
