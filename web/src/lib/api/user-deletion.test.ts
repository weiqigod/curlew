import { describe, it, expect, vi } from 'vitest';
import { userDeletionApi } from './user-deletion';

function fetchOk(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function fetchErr(status: number, code: string, extras: Record<string, unknown> = {}): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: code,
		json: () => Promise.resolve({ code, detail: code, ...extras })
	}) as unknown as typeof fetch;
}

const reauthOk = { reauth_token: 'drto_test123' };
const deletionOk = {
	finalizes_at: '2026-06-17T03:00:00Z',
	cancellable_until: '2026-06-17T03:00:00Z',
	cancel_url: 'http://web.test/account/data/cancel-deletion',
};
const statusPending = {
	pending_deletion_at: '2026-05-18T03:00:00Z',
	finalizes_at: '2026-06-17T03:00:00Z',
};

describe('userDeletionApi.issueReauthToken', () => {
	it('POSTs to /api/v1/auth/reauth and returns drto_ token', async () => {
		const result = await userDeletionApi.issueReauthToken('correctPass!', { fetch: fetchOk(reauthOk) });
		expect(result.reauth_token).toBe('drto_test123');
	});

	it('throws ApiError on 401 invalid password', async () => {
		await expect(
			userDeletionApi.issueReauthToken('wrong', { fetch: fetchErr(401, 'unauthorized') })
		).rejects.toMatchObject({ status: 401 });
	});
});

describe('userDeletionApi.requestDeletion', () => {
	it('sends X-Reauth-Token header', async () => {
		const mockFetch = fetchOk(deletionOk, 202);
		await userDeletionApi.requestDeletion('drto_test123', { fetch: mockFetch });
		const call = vi.mocked(mockFetch).mock.calls[0];
		const headers = call[1]?.headers;
		const headersObj = headers instanceof Headers ? headers : new Headers(headers as HeadersInit);
		expect(headersObj.get('X-Reauth-Token')).toBe('drto_test123');
	});

	it('returns DeletionRequestResponse on 202', async () => {
		const result = await userDeletionApi.requestDeletion('drto_abc', { fetch: fetchOk(deletionOk, 202) });
		expect(result.finalizes_at).toBe('2026-06-17T03:00:00Z');
		expect(result.cancel_url).toContain('cancel-deletion');
	});

	it('throws ApiError with code owner_cannot_leave on 409', async () => {
		const extras = {
			blocking_orgs: [{ slug: 'my-org', name: 'My Org' }],
			error_code: 'OwnerCannotLeave',
		};
		await expect(
			userDeletionApi.requestDeletion('drto_abc', {
				fetch: fetchErr(409, 'owner_cannot_leave', extras)
			})
		).rejects.toMatchObject({ status: 409, code: 'owner_cannot_leave' });
	});
});

describe('userDeletionApi.cancelDeletion', () => {
	it('posts to cancel endpoint and resolves on 200', async () => {
		await expect(
			userDeletionApi.cancelDeletion({ fetch: fetchOk({}, 200) })
		).resolves.toBeUndefined();
	});

	it('throws ApiError on 404 no_pending_request', async () => {
		await expect(
			userDeletionApi.cancelDeletion({ fetch: fetchErr(404, 'no_pending_request') })
		).rejects.toMatchObject({ status: 404 });
	});
});

describe('userDeletionApi.getStatus', () => {
	it('returns UserDeletionStatus with pending_deletion_at', async () => {
		const result = await userDeletionApi.getStatus({ fetch: fetchOk(statusPending) });
		expect(result.pending_deletion_at).toBe('2026-05-18T03:00:00Z');
		expect(result.finalizes_at).toBe('2026-06-17T03:00:00Z');
	});

	it('throws ApiError on 404 when no request exists', async () => {
		await expect(
			userDeletionApi.getStatus({ fetch: fetchErr(404, 'not_found') })
		).rejects.toMatchObject({ status: 404 });
	});
});
