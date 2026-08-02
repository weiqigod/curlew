import { describe, it, expect } from 'vitest';
import { ApiError, isApiError } from './api-error';

describe('ApiError', () => {
	const cases = [
		{ name: 'captures code and status', code: 'permission_denied', status: 403 },
		{ name: 'captures 5xx for retryable errors', code: 'server_error', status: 500 }
	];
	for (const c of cases) {
		it(c.name, () => {
			const err = new ApiError(c.code, 'boom', c.status);
			expect(err.code).toBe(c.code);
			expect(err.status).toBe(c.status);
			expect(isApiError(err)).toBe(true);
		});
	}
	it('rejects non-errors in the type guard', () => {
		expect(isApiError({})).toBe(false);
		expect(isApiError(null)).toBe(false);
	});

	it('field round-trip: captures field when provided', () => {
		const err = new ApiError('invalid_sso_config', 'bad field', 400, 'idp_metadata_url');
		expect(err.field).toBe('idp_metadata_url');
		expect(err.code).toBe('invalid_sso_config');
		expect(err.status).toBe(400);
	});

	it('field round-trip: field is undefined when not provided', () => {
		const err = new ApiError('permission_denied', 'nope', 403);
		expect(err.field).toBeUndefined();
	});

	it('ApiError preserves details payload for 409 role_in_use', () => {
		const e = new ApiError('role_in_use', 'msg', 409, undefined, { member_count: 3 });
		expect((e.details as { member_count: number }).member_count).toBe(3);
	});

	it('details is undefined when not provided', () => {
		const err = new ApiError('permission_denied', 'nope', 403);
		expect(err.details).toBeUndefined();
	});
});
