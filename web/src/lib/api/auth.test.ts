import { describe, it, expect, vi } from 'vitest';
import { authApi } from './auth';
import { ApiError } from '$lib/types/api-error';

function fetchOk(body: unknown): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status: 200,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function fetchProblem(status: number, problemType: string, extra: Record<string, unknown> = {}): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: 'Error',
		json: () =>
			Promise.resolve({
				type: problemType,
				title: 'Error',
				status,
				...extra
			})
	}) as unknown as typeof fetch;
}

describe('authApi', () => {
	describe('requestPasswordReset', () => {
		it('200 OK returns response body', async () => {
			const fetch = fetchOk({ ok: true, message: 'If that email exists, a reset link has been sent.' });
			const result = await authApi.requestPasswordReset('a@b.com', { fetch });
			expect(result).toMatchObject({ ok: true });
		});
	});

	describe('confirmPasswordReset', () => {
		it('200 OK returns response body', async () => {
			const fetch = fetchOk({ ok: true });
			const result = await authApi.confirmPasswordReset('prst_xxx', 'StrongP@ss1!', { fetch });
			expect(result).toMatchObject({ ok: true });
		});

		it('422 weak password exposes problemType and score in details', async () => {
			const fetch = fetchProblem(
				422,
				'https://apitool.dev/errors/password-too-weak',
				{ score: 1 }
			);
			await expect(
				authApi.confirmPasswordReset('prst_xxx', 'weak', { fetch })
			).rejects.toSatisfy(
				(err: unknown) =>
					err instanceof ApiError &&
					err.problemType?.endsWith('/password-too-weak') === true &&
					(err.details as Record<string, unknown>)?.score === 1
			);
		});

		it('400 token invalid exposes problemType ending in password-reset-token-invalid', async () => {
			const fetch = fetchProblem(400, 'https://apitool.dev/errors/password-reset-token-invalid');
			await expect(
				authApi.confirmPasswordReset('bad_token', 'StrongP@ss1!', { fetch })
			).rejects.toSatisfy(
				(err: unknown) =>
					err instanceof ApiError &&
					err.problemType?.endsWith('/password-reset-token-invalid') === true
			);
		});
	});

	describe('resendEmailVerification', () => {
		it('200 OK returns response body', async () => {
			const fetch = fetchOk({ ok: true });
			const result = await authApi.resendEmailVerification('a@b.com', { fetch });
			expect(result).toMatchObject({ ok: true });
		});
	});

	describe('confirmEmailVerification', () => {
		it('200 OK returns response body', async () => {
			const fetch = fetchOk({ ok: true });
			const result = await authApi.confirmEmailVerification('evtk_xxx', { fetch });
			expect(result).toMatchObject({ ok: true });
		});

		it('400 token invalid exposes problemType ending in email-verification-token-invalid', async () => {
			const fetch = fetchProblem(
				400,
				'https://apitool.dev/errors/email-verification-token-invalid'
			);
			await expect(
				authApi.confirmEmailVerification('bad_token', { fetch })
			).rejects.toSatisfy(
				(err: unknown) =>
					err instanceof ApiError &&
					err.problemType?.endsWith('/email-verification-token-invalid') === true
			);
		});
	});
});
