import { describe, it, expect, vi, beforeEach } from 'vitest';
import { rolesApi } from './roles';
import { isApiError } from '$lib/types/api-error';

function makeFetch(status: number, body: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		statusText:
			status === 400
				? 'Bad Request'
				: status === 403
					? 'Forbidden'
					: status === 404
						? 'Not Found'
						: status === 409
							? 'Conflict'
							: status === 204
								? 'No Content'
								: 'OK',
		json: () => (status === 204 ? Promise.reject(new Error('no body')) : Promise.resolve(JSON.parse(body))),
		text: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

const roleA = {
	id: 'role_001',
	name: 'qa-lead',
	permissions: ['results.view', 'results.upload'],
	is_builtin: false,
	created_at: '2026-04-01T00:00:00Z'
};

const builtinRole = {
	id: 'role_owner',
	name: 'owner',
	permissions: [],
	is_builtin: true,
	created_at: null
};

beforeEach(() => {
	vi.restoreAllMocks();
});

describe('rolesApi.list', () => {
	it('list returns roles array', async () => {
		const fetchFn = makeFetch(200, JSON.stringify({ roles: [builtinRole, roleA] }));
		const result = await rolesApi.list('org_abc', { fetch: fetchFn });
		expect(result).toHaveLength(2);
		expect(result[0].is_builtin).toBe(true);
		expect(result[1].name).toBe('qa-lead');
	});

	it('list propagates 403 permission_denied', async () => {
		const fetchFn = makeFetch(
			403,
			'{"code":"permission_denied","description":"Owner only"}'
		);
		let caught: unknown;
		try {
			await rolesApi.list('org_abc', { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('permission_denied');
		expect(err.status).toBe(403);
	});
});

describe('rolesApi.create', () => {
	it('create sends POST JSON and returns the new role', async () => {
		const fetchFn = makeFetch(201, JSON.stringify(roleA));
		const result = await rolesApi.create(
			'org_abc',
			{ name: 'qa-lead', permissions: ['results.view'] },
			{ fetch: fetchFn }
		);
		expect(result.id).toBe('role_001');
		expect(result.name).toBe('qa-lead');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('POST');
		const body = JSON.parse(call[1].body as string);
		expect(body.name).toBe('qa-lead');
	});

	it('create propagates 400 invalid_permission with field detail', async () => {
		const fetchFn = makeFetch(
			400,
			'{"code":"invalid_permission","description":"Unknown permission key","field":"permissions"}'
		);
		let caught: unknown;
		try {
			await rolesApi.create(
				'org_abc',
				{ name: 'bad', permissions: ['unknown.perm'] },
				{ fetch: fetchFn }
			);
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('invalid_permission');
		expect(err.field).toBe('permissions');
	});

	it('create propagates 409 role_name_taken', async () => {
		const fetchFn = makeFetch(
			409,
			'{"code":"role_name_taken","description":"Role name already exists"}'
		);
		let caught: unknown;
		try {
			await rolesApi.create(
				'org_abc',
				{ name: 'existing', permissions: [] },
				{ fetch: fetchFn }
			);
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('role_name_taken');
		expect(err.status).toBe(409);
	});
});

describe('rolesApi.update', () => {
	it('update sends PATCH JSON with name + permissions', async () => {
		const updated = { ...roleA, permissions: ['results.view', 'results.upload', 'results.delete'] };
		const fetchFn = makeFetch(200, JSON.stringify(updated));
		const result = await rolesApi.update(
			'org_abc',
			'role_001',
			{ name: 'qa-lead', permissions: updated.permissions },
			{ fetch: fetchFn }
		);
		expect(result.permissions).toHaveLength(3);
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('PATCH');
		const body = JSON.parse(call[1].body as string);
		expect(body.name).toBe('qa-lead');
		expect(body.permissions).toContain('results.delete');
	});

	it('update propagates 404 role_not_found', async () => {
		const fetchFn = makeFetch(404, '{"code":"role_not_found","description":"Role not found"}');
		let caught: unknown;
		try {
			await rolesApi.update(
				'org_abc',
				'role_ghost',
				{ name: 'x', permissions: [] },
				{ fetch: fetchFn }
			);
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('role_not_found');
		expect(err.status).toBe(404);
	});
});

describe('rolesApi.remove', () => {
	it('remove sends DELETE and resolves on 204', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			statusText: 'No Content',
			json: () => Promise.reject(new Error('no body')),
			text: () => Promise.resolve('')
		}) as unknown as typeof fetch;
		await expect(
			rolesApi.remove('org_abc', 'role_001', { fetch: fetchFn })
		).resolves.toBeUndefined();
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[1].method).toBe('DELETE');
	});

	it('remove propagates 409 role_in_use with member_count in body', async () => {
		const fetchFn = makeFetch(
			409,
			'{"code":"role_in_use","message":"Role is in use","member_count":3}'
		);
		let caught: unknown;
		try {
			await rolesApi.remove('org_abc', 'role_001', { fetch: fetchFn });
		} catch (e) {
			caught = e;
		}
		expect(isApiError(caught)).toBe(true);
		const err = caught as import('$lib/types/api-error').ApiError;
		expect(err.code).toBe('role_in_use');
		expect(err.status).toBe(409);
		expect((err.details as { member_count: number }).member_count).toBe(3);
	});
});
