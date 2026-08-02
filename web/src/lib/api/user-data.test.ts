import { describe, it, expect, vi } from 'vitest';
import { userDataApi } from './user-data';

function fetchOk(body: unknown): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function fetch202(body: unknown): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 202,
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

const queuedRequest = {
	id: 'req-id-1',
	status: 'queued',
	created_at: '2026-05-18T10:00:00Z'
};

const readyRequest = {
	id: 'req-id-2',
	status: 'ready',
	created_at: '2026-05-18T10:00:00Z',
	ready_at: '2026-05-18T10:01:00Z',
	expires_at: '2026-05-19T10:01:00Z',
	signed_url: 'http://test/objects/exports/...'
};

describe('userDataApi.createExportRequest', () => {
	it('returns the queued request on 202', async () => {
		const result = await userDataApi.createExportRequest({ fetch: fetch202(queuedRequest) });
		expect(result.id).toBe('req-id-1');
		expect(result.status).toBe('queued');
	});

	it('propagates 429 rate-limit error', async () => {
		await expect(
			userDataApi.createExportRequest({ fetch: fetchErr(429, 'export_rate_limited') })
		).rejects.toMatchObject({ status: 429 });
	});

	it('propagates 401 unauthorized', async () => {
		await expect(
			userDataApi.createExportRequest({ fetch: fetchErr(401, 'unauthorized') })
		).rejects.toMatchObject({ status: 401 });
	});
});

describe('userDataApi.getExportRequest', () => {
	it('returns the request by id', async () => {
		const result = await userDataApi.getExportRequest('req-id-2', { fetch: fetchOk(readyRequest) });
		expect(result.status).toBe('ready');
		expect(result.signed_url).toContain('http://test');
	});

	it('propagates 404 not found', async () => {
		await expect(
			userDataApi.getExportRequest('unknown', { fetch: fetchErr(404, 'not_found') })
		).rejects.toMatchObject({ status: 404 });
	});
});
