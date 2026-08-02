import { describe, it, expect, vi } from 'vitest';
import { auditLogApi } from './audit-log';
import type { AuditLogEntry } from '$lib/types/audit-log';

function fetchOk(body: unknown): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}
function fetchErr(status: number, code: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: code,
		json: () => Promise.resolve({ code, description: code })
	}) as unknown as typeof fetch;
}

describe('auditLogApi.list', () => {
	const cases = [
		{
			name: 'returns items array',
			items: [{ event_type: 'member.invited' } as Partial<AuditLogEntry>],
			expected: 1
		},
		{ name: 'returns empty list', items: [], expected: 0 },
		{
			name: 'forwards event_type param',
			items: [],
			filter: { event_type: 'sso.login' },
			expectParam: 'event_type=sso.login'
		},
		{
			name: 'forwards from/to params',
			items: [],
			filter: { from: '2026-04-01T00:00:00Z', to: '2026-04-30T00:00:00Z' },
			expectParam: 'from=2026-04-01'
		},
		{ name: 'forwards limit clamp', items: [], filter: { limit: 25 }, expectParam: 'limit=25' },
		{
			name: 'propagates 403 permission_denied',
			fetchMaker: () => fetchErr(403, 'permission_denied'),
			expectReject: 'permission_denied'
		}
	];

	for (const c of cases) {
		it(c.name, async () => {
			const fetchFn = c.fetchMaker ? c.fetchMaker() : fetchOk({ items: c.items });
			const call = auditLogApi.list('org_test', { fetch: fetchFn, ...(c.filter ?? {}) });

			if (c.expectReject) {
				await expect(call).rejects.toMatchObject({ code: c.expectReject });
				return;
			}

			const got = await call;
			expect(got).toHaveLength(c.expected ?? 0);
			if (c.expectParam) {
				const url = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0][0] as string;
				expect(url).toContain(c.expectParam);
			}
		});
	}
});
