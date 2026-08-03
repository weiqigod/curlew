import { beforeEach, describe, expect, it, vi } from 'vitest';
import { actions, load } from './+page.server';

const requestPasswordReset = vi.fn();
vi.mock('$lib/api/auth', () => ({
	authApi: { requestPasswordReset: (...a: unknown[]) => requestPasswordReset(...a) }
}));

/** Builds the RequestEvent fields this action reads. */
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
	requestPasswordReset.mockResolvedValue(undefined);
});

describe('password-reset request action', () => {
	it('loads with no data — the form renders unconditionally', async () => {
		// eslint-disable-next-line @typescript-eslint/no-explicit-any
		expect(await load({} as any)).toEqual({});
	});

	it('forwards the submitted email to the backend', async () => {
		const r = await actions.default(event('user@example.com'));

		expect(requestPasswordReset).toHaveBeenCalledWith(
			'user@example.com',
			expect.objectContaining({ fetch: expect.anything() })
		);
		expect(r).toEqual({ submitted: true });
	});

	it('reports success even when the backend rejects — enumeration defence', async () => {
		requestPasswordReset.mockRejectedValue(new Error('user not found'));

		const r = await actions.default(event('nobody@example.com'));

		// An unregistered address must be indistinguishable from a registered one.
		expect(r).toEqual({ submitted: true });
	});

	it('still reports success for a missing email field', async () => {
		const r = await actions.default(event(null));

		expect(requestPasswordReset).toHaveBeenCalledWith('', expect.anything());
		expect(r).toEqual({ submitted: true });
	});
});
