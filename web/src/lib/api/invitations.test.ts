import { describe, it, expect, vi } from 'vitest';
import { invitationsApi } from './invitations';
import type { Invitation } from '$lib/types/invitations';

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

function makeInvitation(overrides: Partial<Invitation> = {}): Invitation {
	return {
		id: 'inv_test123',
		org_id: 'org_test',
		email: 'newuser@example.com',
		role: 'member',
		expires_at: '2026-04-24T00:00:00Z',
		created_at: '2026-04-17T00:00:00Z',
		accepted_at: null,
		revoked_at: null,
		...overrides
	};
}

describe('invitationsApi.list', () => {
	it('returns invitations array', async () => {
		const inv = makeInvitation();
		const fetchFn = makeFetchOk({ invitations: [inv] });
		const result = await invitationsApi.list('org_test', { fetch: fetchFn });
		expect(result).toHaveLength(1);
		expect(result[0].id).toBe('inv_test123');
	});

	it('returns empty list', async () => {
		const fetchFn = makeFetchOk({ invitations: [] });
		const result = await invitationsApi.list('org_test', { fetch: fetchFn });
		expect(result).toHaveLength(0);
	});

	it('propagates 403 permission_denied', async () => {
		const fetchFn = makeFetchError(403, 'permission_denied', 'Permission denied.');
		await expect(
			invitationsApi.list('org_test', { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'permission_denied', status: 403 });
	});
});

describe('invitationsApi.create', () => {
	it('posts body, returns invitation + token', async () => {
		const inv = makeInvitation();
		const fetchFn = makeFetchOk({ invitation: inv, token: 'raw_tok' }, 201);
		const result = await invitationsApi.create(
			'org_test',
			{ email: 'newuser@example.com', role: 'member' },
			{ fetch: fetchFn }
		);
		expect(result.invitation.id).toBe('inv_test123');
		expect(result.token).toBe('raw_tok');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('POST');
		expect(JSON.parse(opts.body as string)).toMatchObject({
			email: 'newuser@example.com',
			role: 'member'
		});
	});

	it('propagates 400 invalid_email', async () => {
		const fetchFn = makeFetchError(400, 'invalid_email', 'Invalid email address.');
		await expect(
			invitationsApi.create('org_test', { email: 'bad', role: 'member' }, { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'invalid_email', status: 400 });
	});

	it('propagates 409 invitation_pending', async () => {
		const fetchFn = makeFetchError(409, 'invitation_pending', 'Invitation already pending.');
		await expect(
			invitationsApi.create(
				'org_test',
				{ email: 'newuser@example.com', role: 'member' },
				{ fetch: fetchFn }
			)
		).rejects.toMatchObject({ code: 'invitation_pending', status: 409 });
	});

	it('propagates 409 seat_limit_reached', async () => {
		const fetchFn = makeFetchError(409, 'seat_limit_reached', 'Seat limit reached.');
		await expect(
			invitationsApi.create(
				'org_test',
				{ email: 'another@example.com', role: 'member' },
				{ fetch: fetchFn }
			)
		).rejects.toMatchObject({ code: 'seat_limit_reached', status: 409 });
	});
});

describe('invitationsApi.revoke', () => {
	it('DELETE resolves void', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			json: () => Promise.resolve(null)
		}) as unknown as typeof fetch;
		await expect(
			invitationsApi.revoke('org_test', 'inv_test123', { fetch: fetchFn })
		).resolves.toBeUndefined();
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('DELETE');
	});

	it('propagates 404 invitation_not_found', async () => {
		const fetchFn = makeFetchError(404, 'invitation_not_found', 'Invitation not found.');
		await expect(
			invitationsApi.revoke('org_test', 'inv_test123', { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'invitation_not_found', status: 404 });
	});
});
