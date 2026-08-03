import { beforeEach, describe, expect, it, vi } from 'vitest';
import { actions, load } from './+page.server';

const resendEmailVerification = vi.fn();
vi.mock('$lib/api/auth', () => ({
	authApi: { resendEmailVerification: (...a: unknown[]) => resendEmailVerification(...a) }
}));

function event(email: string | null) {
	const data = new FormData();
	if (email !== null) data.set('email', email);
	return {
		request: { formData: async () => data },
		fetch: vi.fn()
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
	} as any;
}

beforeEach(() => {
	vi.clearAllMocks();
	resendEmailVerification.mockResolvedValue(undefined);
});

describe('email-verification request action', () => {
	it('loads with no data — the form renders unconditionally', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(await load({} as any)).toEqual({});
	});

	it('forwards the submitted email to the resend endpoint', async () => {
		const r = await actions.default(event('user@example.com'));

		expect(resendEmailVerification).toHaveBeenCalledWith(
			'user@example.com',
			expect.objectContaining({ fetch: expect.anything() })
		);
		expect(r).toEqual({ submitted: true });
	});

	it('reports success even when the backend rejects — enumeration defence', async () => {
		resendEmailVerification.mockRejectedValue(new Error('already verified'));

		expect(await actions.default(event('nobody@example.com'))).toEqual({ submitted: true });
	});

	it('still reports success for a missing email field', async () => {
		expect(await actions.default(event(null))).toEqual({ submitted: true });
		expect(resendEmailVerification).toHaveBeenCalledWith('', expect.anything());
	});
});
