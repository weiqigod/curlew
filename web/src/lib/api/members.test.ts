import { describe, it, expect, vi } from 'vitest';
import { membersApi } from './members';
import type { Member } from '$lib/types/members';

function makeFetchOk(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function makeFetchError(status: number, code: string, description: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: description,
		json: () => Promise.resolve({ code, description })
	}) as unknown as typeof fetch;
}

function makeMember(overrides: Partial<Member> = {}): Member {
	return {
		user_id: 'user_abc123',
		role: 'member',
		joined_at: '2026-04-01T00:00:00Z',
		...overrides
	};
}

describe('membersApi.list', () => {
	it('returns members array', async () => {
		const member = makeMember();
		const fetchFn = makeFetchOk({ members: [member] });
		const result = await membersApi.list('org_test', { fetch: fetchFn });
		expect(result).toHaveLength(1);
		expect(result[0].user_id).toBe('user_abc123');
	});

	it('propagates 403 permission_denied', async () => {
		const fetchFn = makeFetchError(403, 'permission_denied', 'Forbidden');
		await expect(membersApi.list('org_test', { fetch: fetchFn })).rejects.toMatchObject({
			code: 'permission_denied',
			status: 403
		});
	});
});
