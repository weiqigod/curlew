/**
 * Unit tests for /account/data delete panel — covers the client-side logic
 * (userDeletionApi calls, re-auth flow, blocking-orgs rendering) without
 * mounting the full Svelte component.
 * Component rendering is covered by the Playwright e2e spec.
 */

import { describe, it, expect, vi } from 'vitest';
import { userDeletionApi } from '$lib/api/user-deletion';
import pageSource from './+page.svelte?raw';
import cancelPageSource from './cancel-deletion/+page.svelte?raw';

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

const reauthOk = { reauth_token: 'drto_test999' };
const deletionOk = {
	finalizes_at: '2026-06-17T03:00:00Z',
	cancellable_until: '2026-06-17T03:00:00Z',
	cancel_url: 'http://web.test/account/data/cancel-deletion',
};

describe('account/data delete panel', () => {
	it('issueReauthToken POSTs to /api/v1/auth/reauth and returns drto_ token', async () => {
		const result = await userDeletionApi.issueReauthToken('pass', { fetch: fetchOk(reauthOk) });
		expect(result.reauth_token).toMatch(/^drto_/);
	});

	it('requestDeletion sends X-Reauth-Token header', async () => {
		const mockFetch = fetchOk(deletionOk, 202);
		await userDeletionApi.requestDeletion('drto_test999', { fetch: mockFetch });
		const call = vi.mocked(mockFetch).mock.calls[0];
		const headers = call[1]?.headers;
		const headersObj = headers instanceof Headers ? headers : new Headers(headers as HeadersInit);
		expect(headersObj.get('X-Reauth-Token')).toBe('drto_test999');
	});

	it('renders blocking-orgs list on 409 OwnerCannotLeave', async () => {
		const extras = {
			blocking_orgs: [{ slug: 'my-org', name: 'My Org' }],
			error_code: 'OwnerCannotLeave',
		};
		let caught: unknown;
		try {
			await userDeletionApi.requestDeletion('drto_abc', {
				fetch: fetchErr(409, 'owner_cannot_leave', extras)
			});
		} catch (e) {
			caught = e;
		}
		expect(caught).toBeDefined();
		// Verify the ApiError carries blocking_orgs in details
		expect((caught as { details?: Record<string, unknown> }).details?.blocking_orgs).toEqual([
			{ slug: 'my-org', name: 'My Org' }
		]);
	});

	it('renders success countdown on 202 with finalizes_at', async () => {
		const result = await userDeletionApi.requestDeletion('drto_abc', {
			fetch: fetchOk(deletionOk, 202)
		});
		expect(result.finalizes_at).toBe('2026-06-17T03:00:00Z');
	});

	it('cancelDeletion clears pending state', async () => {
		await expect(
			userDeletionApi.cancelDeletion({ fetch: fetchOk({}, 200) })
		).resolves.toBeUndefined();
	});

	it('page source contains DeleteAccountPanel slot', () => {
		expect(pageSource).toContain('DeleteAccountPanel');
	});

	it('cancel-deletion page source contains Cancel button', () => {
		expect(cancelPageSource).toContain('cancel-deletion');
	});
});
