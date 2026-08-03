import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { AuditLogEntry } from '$lib/types/audit-log';
import { GET } from './+server';

const findBySlug = vi.fn();
const listAudit = vi.fn();

vi.mock('$lib/api/organizations', () => ({
	organizationsApi: { findBySlug: (...a: unknown[]) => findBySlug(...a) }
}));
vi.mock('$lib/api/audit-log', () => ({
	auditLogApi: { list: (...a: unknown[]) => listAudit(...a) }
}));

const ORG = { id: 'org_1', slug: 'acme', name: 'Acme', role: 'owner', tier: 'enterprise' };

const ENTRY: AuditLogEntry = {
	event_type: 'member.invited',
	user_id: 'user_0',
	user_email: 'owner@example.com',
	target_type: 'invitation',
	target_id: 'inv_0',
	created_at: '2026-04-15T00:00:00Z',
	ip_address: '10.0.0.1',
	success: true,
	failure_reason: null
};

function event(search = '') {
	return {
		locals: { accessToken: 'tok' },
		params: { slug: 'acme' },
		url: new URL(`http://localhost/org/acme/audit-log/export${search}`),
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

async function getThrows(ev: unknown) {
	try {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await GET(ev as any);
	} catch (e) {
		return e as { status: number; location?: string };
	}
	throw new Error('expected GET to throw');
}

beforeEach(() => {
	vi.clearAllMocks();
	findBySlug.mockResolvedValue(ORG);
	listAudit.mockResolvedValue([ENTRY]);
});

describe('audit-log CSV export route', () => {
	it('returns a CSV attachment with a dated, slug-scoped filename', async () => {
		const res = (await GET(event())) as Response;

		expect(res.status).toBe(200);
		expect(res.headers.get('Content-Type')).toBe('text/csv; charset=utf-8');
		expect(res.headers.get('Content-Disposition')).toMatch(
			/^attachment; filename="audit-log-acme-\d{8}\.csv"$/
		);

		const body = await res.text();
		expect(body.split('\r\n')[0]).toBe('event_type,user,target,timestamp,ip');
		expect(body).toContain('member.invited');
	});

	it('exports a deeper window than the paged view and forwards the active filters', async () => {
		await GET(event('?event_type=sso.login&from=2026-04-01T00:00:00Z&to=2026-04-08T00:00:00Z'));

		expect(listAudit).toHaveBeenCalledWith(
			'org_1',
			expect.objectContaining({
				limit: 200,
				event_type: 'sso.login',
				from: '2026-04-01T00:00:00Z',
				to: '2026-04-08T00:00:00Z'
			})
		);
	});

	it('still returns a header-only CSV when nothing matches', async () => {
		listAudit.mockResolvedValue([]);

		const res = (await GET(event())) as Response;
		const body = await res.text();

		expect(res.status).toBe(200);
		expect(body.split('\r\n')[0]).toBe('event_type,user,target,timestamp,ip');
	});

	it('502s when the upstream audit-log fetch fails', async () => {
		listAudit.mockRejectedValue(new Error('upstream boom'));

		expect((await getThrows(event())).status).toBe(502);
	});

	it('applies the same enterprise + admin guards as the page', async () => {
		findBySlug.mockResolvedValue({ ...ORG, role: 'member' });
		expect((await getThrows(event())).location).toBe('/org/acme?toast=admin_required');

		findBySlug.mockResolvedValue({ ...ORG, tier: 'team' });
		expect((await getThrows(event())).location).toBe('/org/acme?toast=enterprise_tier_required');

		findBySlug.mockResolvedValue(null);
		expect((await getThrows(event())).status).toBe(404);
	});
});
