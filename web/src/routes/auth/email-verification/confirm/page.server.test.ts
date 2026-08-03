import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import { load } from './+page.server';

const confirmEmailVerification = vi.fn();
vi.mock('$lib/api/auth', () => ({
	authApi: { confirmEmailVerification: (...a: unknown[]) => confirmEmailVerification(...a) }
}));

function event(search = '') {
	return {
		url: new URL(`http://localhost/auth/email-verification/confirm${search}`),
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

async function loadThrows(ev: unknown) {
	try {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		await load(ev as any);
	} catch (e) {
		return e as { status: number; location?: string };
	}
	throw new Error('expected load to throw');
}

beforeEach(() => {
	vi.clearAllMocks();
	confirmEmailVerification.mockResolvedValue(undefined);
});

describe('email-verification confirm load', () => {
	it('confirms the token and redirects home with the verified toast', async () => {
		const e = await loadThrows(event('?token=evtk_validtoken'));

		expect(confirmEmailVerification).toHaveBeenCalledWith(
			'evtk_validtoken',
			expect.objectContaining({ fetch: expect.anything() })
		);
		expect(e.status).toBe(303);
		expect(e.location).toBe('/?toast=email_verified');
	});

	it('reports a missing token without calling the backend', async () => {
		expect(await load(event())).toEqual({ error: 'missing_token' });
		expect(confirmEmailVerification).not.toHaveBeenCalled();
	});

	it('maps the token-invalid problem type onto token_invalid', async () => {
		confirmEmailVerification.mockRejectedValue(
			new ApiError(
				'err',
				'Token invalid',
				400,
				undefined,
				undefined,
				'https://apitool.dev/errors/email-verification-token-invalid'
			)
		);

		expect(await load(event('?token=bad_token'))).toEqual({ error: 'token_invalid' });
	});

	it('maps any other failure onto server_error', async () => {
		confirmEmailVerification.mockRejectedValue(new Error('socket hang up'));

		expect(await load(event('?token=evtk_x'))).toEqual({ error: 'server_error' });
	});

	it('treats an unrelated ApiError as a server error rather than an invalid token', async () => {
		confirmEmailVerification.mockRejectedValue(
			new ApiError('err', 'boom', 500, undefined, undefined, 'https://apitool.dev/errors/internal')
		);

		expect(await load(event('?token=evtk_x'))).toEqual({ error: 'server_error' });
	});
});
