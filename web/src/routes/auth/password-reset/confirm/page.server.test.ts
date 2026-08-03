import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import { actions, load } from './+page.server';

const confirmPasswordReset = vi.fn();
vi.mock('$lib/api/auth', () => ({
	authApi: { confirmPasswordReset: (...a: unknown[]) => confirmPasswordReset(...a) }
}));

function event(fields: Record<string, string>) {
	const data = new FormData();
	for (const [k, v] of Object.entries(fields)) data.set(k, v);
	return {
		request: { formData: async () => data },
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

/** Runs the action and returns whatever it threw (the redirect). */
async function actionThrows(ev: unknown) {
	try {
		await actions.default(ev as never);
	} catch (e) {
		return e as { status: number; location?: string };
	}
	throw new Error('expected the action to throw');
}

/** Builds an ApiError carrying an RFC 7807 problem type. */
function problem(type: string, details?: Record<string, unknown>) {
	return new ApiError('err', 'msg', 422, undefined, details, type);
}

beforeEach(() => {
	vi.clearAllMocks();
	confirmPasswordReset.mockResolvedValue(undefined);
});

describe('password-reset confirm load', () => {
	it('extracts the token from the query string', async () => {
		const r = await load({
			url: new URL('http://localhost/auth/password-reset/confirm?token=prst_abc')
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
		} as any);

		expect(r).toEqual({ token: 'prst_abc', hasToken: true });
	});

	it('reports no token when the query string omits it', async () => {
		const r = await load({
			url: new URL('http://localhost/auth/password-reset/confirm')
			// eslint-disable-next-line @typescript-eslint/no-explicit-any
		} as any);

		expect(r).toEqual({ token: '', hasToken: false });
	});
});

describe('password-reset confirm action', () => {
	it('redirects to /login with a success toast when the reset succeeds', async () => {
		const e = await actionThrows(
			event({ token: 'prst_valid', new_password: 'Tr0ub4dor&3-very-strong' })
		);

		expect(confirmPasswordReset).toHaveBeenCalledWith(
			'prst_valid',
			'Tr0ub4dor&3-very-strong',
			expect.anything()
		);
		expect(e.status).toBe(303);
		expect(e.location).toBe('/login?toast=password_reset_success');
	});

	it('fails 422 with the score for a too-weak password', async () => {
		confirmPasswordReset.mockRejectedValue(
			problem('https://apitool.dev/errors/password-too-weak', { score: 1 })
		);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await actions.default(event({ token: 't', new_password: 'weak' }))) as any;

		expect(r.status).toBe(422);
		expect(r.data).toEqual({ error: 'weak_password', score: 1 });
	});

	it('defaults a missing score to 0 rather than NaN', async () => {
		confirmPasswordReset.mockRejectedValue(
			problem('https://apitool.dev/errors/password-too-weak')
		);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await actions.default(event({ token: 't', new_password: 'weak' }))) as any;

		expect(r.data.score).toBe(0);
	});

	it('fails 400 for an invalid or expired token', async () => {
		confirmPasswordReset.mockRejectedValue(
			problem('https://apitool.dev/errors/password-reset-token-invalid')
		);

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await actions.default(event({ token: 'bad', new_password: 'StrongP@ss1!' }))) as any;

		expect(r.status).toBe(400);
		expect(r.data).toEqual({ error: 'token_invalid' });
	});

	it('fails 400 with missing_field when either input is absent', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const noToken = (await actions.default(event({ new_password: 'StrongP@ss1!' }))) as any;
		expect(noToken.status).toBe(400);
		expect(noToken.data).toEqual({ error: 'missing_field' });

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const noPassword = (await actions.default(event({ token: 't' }))) as any;
		expect(noPassword.data).toEqual({ error: 'missing_field' });

		expect(confirmPasswordReset).not.toHaveBeenCalled();
	});

	it('fails 500 for an unrecognised backend error', async () => {
		confirmPasswordReset.mockRejectedValue(new Error('socket hang up'));

		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		const r = (await actions.default(event({ token: 't', new_password: 'StrongP@ss1!' }))) as any;

		expect(r.status).toBe(500);
		expect(r.data).toEqual({ error: 'server_error' });
	});
});
