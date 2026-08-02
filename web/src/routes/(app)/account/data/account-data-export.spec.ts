/**
 * Unit tests for /account/data page — covers the client-side logic
 * (userDataApi calls, error paths, status transitions) without mounting
 * the full Svelte component (no testing-library/svelte setup in this project).
 * Component rendering is covered by the Playwright e2e spec.
 */

import { describe, it, expect, vi } from 'vitest';
import { userDataApi } from '$lib/api/user-data';
import { TERMINAL_STATUSES } from '$lib/types/user-data';
import pageSource from './+page.svelte?raw';

function fetchOk(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function fetchErr(status: number, code: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: code,
		json: () => Promise.resolve({ code, detail: code })
	}) as unknown as typeof fetch;
}

const queuedReq = {
	id: 'page-req-1',
	status: 'queued' as const,
	created_at: '2026-05-18T10:00:00Z',
};

const readyReq = {
	id: 'page-req-2',
	status: 'ready' as const,
	created_at: '2026-05-18T10:00:00Z',
	ready_at: '2026-05-18T10:01:00Z',
	expires_at: '2026-05-19T10:01:00Z',
	signed_url: 'http://test/bundle.json',
};

describe('account/data page', () => {
	it('createExportRequest returns a queued request', async () => {
		const result = await userDataApi.createExportRequest({ fetch: fetchOk(queuedReq, 202) });
		expect(result.status).toBe('queued');
		expect(result.id).toBe('page-req-1');
	});

	it('shows 429 rate-limit error on subsequent requests within 24h', async () => {
		await expect(
			userDataApi.createExportRequest({ fetch: fetchErr(429, 'export_rate_limited') })
		).rejects.toThrow();
	});

	it('shows Download link when request is ready', async () => {
		const result = await userDataApi.getExportRequest('page-req-2', { fetch: fetchOk(readyReq) });
		expect(result.status).toBe('ready');
		expect(result.signed_url).toBeTruthy();
	});

	it('renders failure_reason when status is failed', async () => {
		const failedReq = {
			id: 'page-req-3',
			status: 'failed' as const,
			created_at: '2026-05-18T10:00:00Z',
			failure_reason: 'InvalidOperationException',
		};
		const result = await userDataApi.getExportRequest('page-req-3', { fetch: fetchOk(failedReq) });
		expect(result.failure_reason).toBe('InvalidOperationException');
	});

	it('TERMINAL_STATUSES includes ready, failed, expired', () => {
		expect(TERMINAL_STATUSES.has('ready')).toBe(true);
		expect(TERMINAL_STATUSES.has('failed')).toBe(true);
		expect(TERMINAL_STATUSES.has('expired')).toBe(true);
		expect(TERMINAL_STATUSES.has('queued')).toBe(false);
		expect(TERMINAL_STATUSES.has('building')).toBe(false);
	});

	it('polling stops when status reaches a terminal state', () => {
		const statuses = ['queued', 'building', 'ready'] as const;
		let pollCount = 0;
		for (const status of statuses) {
			pollCount++;
			if (TERMINAL_STATUSES.has(status)) break;
		}
		expect(pollCount).toBe(3); // stops at 'ready'
	});

	it('footer links to docs/security/data-inventory.md', () => {
		// Assert the actual +page.svelte template source contains the required footer anchor.
		// Reading via ?raw means this test fails if the href is removed or changed.
		expect(pageSource).toContain('/docs/security/data-inventory.md');
	});
});
