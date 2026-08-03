import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuditLogEntry } from '$lib/types/audit-log';
import { load } from './+page.server';

const findBySlug = vi.fn();
const listAudit = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/audit-log', () => ({
	auditLogApi: { list: (...a: unknown[]) => listAudit(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'enterprise' };

function entries(count: number): AuditLogEntry[] {
	return Array.from({ length: count }, (_, i) => ({
		event_type: 'member.invited',
		user_id: `user_${i}`,
		user_email: null,
		target_type: 'invitation',
		target_id: `inv_${i}`,
		created_at: new Date(Date.UTC(2026, 3, 15)).toISOString(),
		ip_address: '10.0.0.1',
		success: true,
		failure_reason: null
	}));
}

function event(search = '') {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL(`http://localhost/org/acme/audit-log${search}`),
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
	listAudit.mockResolvedValue(entries(20));
});

describe('audit-log load', () => {
	it('returns the first page of 10 rows with the total page count', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.entries).toHaveLength(10);
		expect(r.page).toBe(1);
		expect(r.totalPages).toBe(2);
		expect(r.totalCount).toBe(20);
		expect(r.error).toBeNull();
	});

	it('slices the requested page out of the fetched window', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?page=2'))) as any;

		expect(r.page).toBe(2);
		expect(r.entries[0].user_id).toBe('user_10');
		expect(r.entries).toHaveLength(10);
	});

	it('clamps a page beyond the end back to the last page', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?page=99'))) as any;

		expect(r.page).toBe(2);
		expect(r.entries).toHaveLength(10);
	});

	it('clamps a zero or negative page to 1', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event('?page=0'))) as any).page).toBe(1);
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event('?page=-3'))) as any).page).toBe(1);
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(((await load(event('?page=abc'))) as any).page).toBe(1);
	});

	it('forwards the event_type and date filters to the API and echoes them back', async () => {
		listAudit.mockResolvedValue(entries(5));

		const r = (await load(
			event('?event_type=member.invited&from=2026-04-01T00:00:00Z&to=2026-04-08T00:00:00Z')
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
		)) as any;

		expect(listAudit).toHaveBeenCalledWith(
			'org_1',
			expect.objectContaining({
				event_type: 'member.invited',
				from: '2026-04-01T00:00:00Z',
				to: '2026-04-08T00:00:00Z',
				limit: 100
			})
		);
		expect(r.filter).toEqual({
			event_type: 'member.invited',
			from: '2026-04-01T00:00:00Z',
			to: '2026-04-08T00:00:00Z'
		});
	});

	it('reports one page when the log is empty', async () => {
		listAudit.mockResolvedValue([]);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event())) as any;

		expect(r.entries).toEqual([]);
		expect(r.totalPages).toBe(1);
		expect(r.totalCount).toBe(0);
	});

	it('redirects non-admins with the admin_required toast', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member' });

		const e = await loadThrows(event());

		expect(e.status).toBe(303);
		expect(e.location).toBe('/org/acme?toast=admin_required');
	});

	it('redirects below-enterprise tiers before checking the role', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member', tier: 'team' });

		expect((await loadThrows(event())).location).toBe('/org/acme?toast=enterprise_tier_required');
	});

	it('degrades to an error message when the fetch fails', async () => {
		listAudit.mockRejectedValue(new Error('audit down'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await load(event('?event_type=sso.login'))) as any;

		expect(r.error).toBe('audit down');
		expect(r.entries).toEqual([]);
		expect(r.totalPages).toBe(1);
		// The filter is preserved so the UI keeps its selection.
		expect(r.filter.event_type).toBe('sso.login');
	});

	it('404s when the org slug does not resolve', async () => {
		findBySlug.mockResolvedValue(null);

		expect((await loadThrows(event())).status).toBe(404);
	});
});
