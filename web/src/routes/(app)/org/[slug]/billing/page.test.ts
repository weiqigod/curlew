import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { Subscription } from '$lib/types/subscriptions';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const invalidateAll = vi.fn();
vi.mock('$app/navigation', () => ({ invalidateAll: () => invalidateAll() }));

const update = vi.fn();
const portal = vi.fn();
vi.mock('$lib/api/subscriptions', () => ({
	subscriptionsApi: {
		update: (...a: unknown[]) => update(...a),
		portal: (...a: unknown[]) => portal(...a)
	}
}));

const resendEmailVerification = vi.fn();
vi.mock('$lib/api/auth', () => ({
	authApi: { resendEmailVerification: (...a: unknown[]) => resendEmailVerification(...a) }
}));

const SUBSCRIPTION = {
	id: 'sub_test',
	org_id: 'org_test',
	tier: 'team',
	status: 'active',
	interval: 'month',
	seat_count: 3,
	seat_limit: 10,
	current_period_start: '2026-04-01T00:00:00Z',
	current_period_end: '2026-05-01T00:00:00Z',
	cancel_at_period_end: false,
	created_at: '2026-04-01T00:00:00Z'
} as Subscription;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		subscription: SUBSCRIPTION,
		tier: 'team',
		error: null,
		...overrides
	};
}

/** Opens the add-seats modal and submits the given delta. */
async function addSeats(user: ReturnType<typeof userEvent.setup>, delta: string) {
	await user.click(screen.getByTestId('add-seats-button'));
	const input = screen.getByTestId('add-seats-delta');
	await user.clear(input);
	await user.type(input, delta);
	await user.click(screen.getByTestId('add-seats-submit'));
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('billing page', () => {
	it('shows the subscription card with tier, price, renewal and seat usage', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('subscription-card')).toBeVisible();
		expect(screen.getByTestId('subscription-tier')).toHaveTextContent('team');
		expect(screen.getByTestId('subscription-price')).toHaveTextContent('$39/mo');
		expect(screen.getByTestId('subscription-renewal')).toHaveTextContent('2026');

		const bar = screen.getByTestId('seats-usage-bar');
		expect(bar).toHaveAttribute('aria-valuenow', '3');
		expect(bar).toHaveAttribute('aria-valuemax', '10');
	});

	it('falls back to a dash for a tier/interval with no listed price', () => {
		render(
			Page,
			{ props: { data: makeData({ subscription: { ...SUBSCRIPTION, interval: 'week' } }) } }
		);

		expect(screen.getByTestId('subscription-price')).toHaveTextContent('—');
	});

	it('adds seats as an absolute count and reports the proration charge', async () => {
		const user = userEvent.setup();
		update.mockResolvedValue({ proration: { credit: 0, charge: 5400, net: 5400 } });

		render(Page, { props: { data: makeData() } });
		await addSeats(user, '3');

		await waitFor(() => expect(update).toHaveBeenCalledTimes(1));
		// The modal collects a delta; the API takes the resulting total (3 + 3).
		expect(update).toHaveBeenCalledWith('sub_test', { seat_count: 6 });

		const proration = await screen.findByTestId('add-seats-proration');
		expect(proration).toHaveTextContent('$54.00');
		expect(invalidateAll).toHaveBeenCalledTimes(1);
	});

	it('says there is no additional charge when the proration nets to zero', async () => {
		const user = userEvent.setup();
		update.mockResolvedValue({ proration: { credit: 5400, charge: 5400, net: 0 } });

		render(Page, { props: { data: makeData() } });
		await addSeats(user, '1');

		expect(await screen.findByTestId('add-seats-proration')).toHaveTextContent(
			'No additional charge'
		);
	});

	it('rejects a seat delta below one without calling the API', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await addSeats(user, '0');

		expect(screen.getByTestId('add-seats-error')).toHaveTextContent('Add at least 1 seat.');
		expect(update).not.toHaveBeenCalled();
	});

	it('surfaces a generic update failure inside the modal', async () => {
		const user = userEvent.setup();
		update.mockRejectedValue(new ApiError('server_error', 'seat update failed', 500));

		render(Page, { props: { data: makeData() } });
		await addSeats(user, '1');

		await waitFor(() =>
			expect(screen.getByTestId('add-seats-error')).toHaveTextContent('seat update failed')
		);
		expect(screen.queryByTestId('email-verified-required-modal')).not.toBeInTheDocument();
	});

	it('opens the verification modal on an email-not-verified 403 and resends the link', async () => {
		const user = userEvent.setup();
		update.mockRejectedValue(
			new ApiError(
				'email_not_verified',
				'Email not verified',
				403,
				undefined,
				undefined,
				'https://apitool.dev/errors/email-not-verified'
			)
		);
		resendEmailVerification.mockResolvedValue(undefined);

		render(Page, { props: { data: makeData() } });
		await addSeats(user, '1');

		expect(await screen.findByTestId('email-verified-required-modal')).toBeVisible();

		await user.type(screen.getByLabelText('Email address'), 'owner@example.com');
		await user.click(screen.getByTestId('resend-verification-button'));

		await waitFor(() =>
			expect(resendEmailVerification).toHaveBeenCalledWith('owner@example.com')
		);
		expect(await screen.findByTestId('verification-resent-confirmation')).toBeVisible();
	});

	it('reports a failed resend without claiming success', async () => {
		const user = userEvent.setup();
		update.mockRejectedValue(
			new ApiError(
				'email_not_verified',
				'Email not verified',
				403,
				undefined,
				undefined,
				'https://apitool.dev/errors/email-not-verified'
			)
		);
		resendEmailVerification.mockRejectedValue(new Error('smtp down'));

		render(Page, { props: { data: makeData() } });
		await addSeats(user, '1');

		await user.type(await screen.findByLabelText('Email address'), 'owner@example.com');
		await user.click(screen.getByTestId('resend-verification-button'));

		expect(await screen.findByText('Failed to send verification email. Please try again.')).toBeVisible();
		expect(screen.queryByTestId('verification-resent-confirmation')).not.toBeInTheDocument();
	});

	it('requests a billing-portal session with a return URL', async () => {
		const user = userEvent.setup();
		// Never resolves: keeps the component in its loading branch so jsdom is
		// not asked to navigate away.
		portal.mockReturnValue(new Promise(() => {}));

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('manage-billing-button'));

		await waitFor(() => expect(portal).toHaveBeenCalledTimes(1));
		const [orgId, returnUrl] = portal.mock.calls[0] as [string, string];
		expect(orgId).toBe('org_test');
		expect(returnUrl).toBe(window.location.href);
		expect(screen.getByTestId('manage-billing-button')).toHaveTextContent('Opening…');
	});

	it('reports a portal failure inline and re-enables the link', async () => {
		const user = userEvent.setup();
		portal.mockRejectedValue(new ApiError('server_error', 'portal unavailable', 500));

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('manage-billing-button'));

		expect(await screen.findByText('portal unavailable')).toBeVisible();
		expect(screen.getByTestId('manage-billing-button')).toHaveTextContent('Manage billing');
	});

	it('shows the no-subscription card when the org has none', () => {
		render(Page, { props: { data: makeData({ subscription: null }) } });

		expect(screen.getByTestId('no-subscription-card')).toBeVisible();
		expect(screen.queryByTestId('subscription-card')).not.toBeInTheDocument();
		expect(screen.queryByTestId('add-seats-button')).not.toBeInTheDocument();
	});

	it('shows an error state instead of the card when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load subscription.' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('Failed to load subscription.');
		expect(screen.queryByTestId('subscription-card')).not.toBeInTheDocument();
	});

	it('flags an over-limit seat count on the usage bar', () => {
		render(
			Page,
			{
				props: {
					data: makeData({ subscription: { ...SUBSCRIPTION, seat_count: 12, seat_limit: 10 } })
				}
			}
		);

		const bar = screen.getByTestId('seats-usage-bar');
		expect(bar).toHaveAttribute('aria-valuenow', '12');
		// The fill is clamped to 100% rather than overflowing its track.
		expect(bar.querySelector('div')).toHaveStyle({ width: '100%' });
	});
});
