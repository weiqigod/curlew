import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { Schedule, ScheduledRun } from '$lib/types/schedules';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

vi.mock('$app/stores', async () => {
	const { readable } = await import('svelte/store');
	return {
		page: readable({
			url: new URL('http://localhost/org/acme/schedules'),
			data: { accessToken: 'tok' }
		})
	};
});

const runNow = vi.fn();
const create = vi.fn();
vi.mock('$lib/api/schedules', () => ({
	schedulesApi: {
		runNow: (...a: unknown[]) => runNow(...a),
		create: (...a: unknown[]) => create(...a)
	}
}));

const NIGHTLY = {
	name: 'nightly',
	cron_expression: '0 2 * * *',
	timezone: 'Europe/Stockholm',
	collection_ref: 'smoke.yaml',
	next_run_at: '2026-05-13T02:00:00Z',
	last_run_at: '2026-05-12T02:00:00Z'
} as Schedule;

const LAST_RUN = { status: 'completed' } as ScheduledRun;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		schedules: [NIGHTLY],
		lastRuns: [LAST_RUN],
		error: null,
		...overrides
	};
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('schedules page', () => {
	it('lists each schedule with its cron, timezone and last-run status', () => {
		render(Page, { props: { data: makeData() } });

		const link = screen.getByTestId('schedule-name-link');
		expect(link).toHaveTextContent('nightly');
		expect(link).toHaveAttribute('href', '/org/acme/schedules/nightly');

		const table = link.closest('table')!;
		expect(table).toHaveTextContent('0 2 * * *');
		expect(table).toHaveTextContent('Europe/Stockholm');
		expect(screen.getByTestId('last-run-status')).toHaveTextContent('completed');
		expect(screen.getByTestId('new-schedule-btn')).toBeVisible();
	});

	it('URL-encodes schedule names in the run-history link', () => {
		render(Page, { props: { data: makeData({ schedules: [{ ...NIGHTLY, name: 'nightly build' }] }) } });

		expect(screen.getByTestId('schedule-name-link')).toHaveAttribute(
			'href',
			'/org/acme/schedules/nightly%20build'
		);
	});

	it('renders a dash instead of a status badge when a schedule has never run', () => {
		render(Page, { props: { data: makeData({ lastRuns: [null] }) } });

		expect(screen.queryByTestId('last-run-status')).not.toBeInTheDocument();
	});

	it('shows a live cron preview in the create modal', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('new-schedule-btn'));

		const cron = screen.getByTestId('sched-cron');
		expect(cron).toBeVisible();
		await user.clear(cron);
		await user.type(cron, '0 9 * * *');

		await waitFor(() => expect(screen.getByTestId('cron-preview')).toBeVisible());
	});

	it('enqueues a run and shows the queued badge', async () => {
		const user = userEvent.setup();
		runNow.mockResolvedValue({ status: 'queued' });

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('run-now-btn'));

		await waitFor(() =>
			expect(screen.getByTestId('run-now-feedback')).toHaveTextContent('queued')
		);
		expect(runNow).toHaveBeenCalledWith('org_test', 'nightly', { token: 'tok' });
	});

	it('shows an error badge when the run-now call fails', async () => {
		const user = userEvent.setup();
		runNow.mockRejectedValue(new Error('nope'));

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('run-now-btn'));

		await waitFor(() => expect(screen.getByTestId('run-now-feedback')).toHaveTextContent('error'));
	});

	it('surfaces a 422 invalid-timezone as an inline form error and keeps the modal open', async () => {
		const user = userEvent.setup();
		create.mockRejectedValue(
			new ApiError('invalid_timezone', 'Unknown IANA timezone: Bad/Zone', 422)
		);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('new-schedule-btn'));
		await user.type(screen.getByTestId('sched-name'), 'bad-tz-sched');
		await user.clear(screen.getByTestId('sched-cron'));
		await user.type(screen.getByTestId('sched-cron'), '0 9 * * *');
		await user.type(screen.getByTestId('sched-ref'), 'smoke.yaml');
		await user.click(screen.getByTestId('sched-submit'));

		await waitFor(() =>
			expect(screen.getByTestId('sched-form-error')).toHaveTextContent(
				'Unknown IANA timezone: Bad/Zone'
			)
		);
		// Modal stays open and the submit button is re-enabled for a retry.
		expect(screen.getByTestId('sched-submit')).toBeEnabled();
	});

	it('appends the created schedule to the table and closes the modal on success', async () => {
		const user = userEvent.setup();
		const created = { ...NIGHTLY, name: 'weekly', cron_expression: '0 3 * * 1' };
		create.mockResolvedValue(created);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('new-schedule-btn'));
		await user.type(screen.getByTestId('sched-name'), 'weekly');
		await user.type(screen.getByTestId('sched-ref'), 'smoke.yaml');
		await user.click(screen.getByTestId('sched-submit'));

		await waitFor(() => expect(screen.getAllByTestId('schedule-name-link')).toHaveLength(2));
		expect(screen.queryByTestId('sched-submit')).not.toBeInTheDocument();
		expect(create).toHaveBeenCalledWith(
			'org_test',
			{ name: 'weekly', cron: '0 9 * * *', timezone: 'UTC', collection_ref: 'smoke.yaml' },
			{ token: 'tok' }
		);
	});

	it('validates required fields client-side before calling the API', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('new-schedule-btn'));
		await user.click(screen.getByTestId('sched-submit'));

		expect(screen.getByTestId('sched-form-error')).toHaveTextContent('Name is required.');
		expect(create).not.toHaveBeenCalled();
	});

	it('shows the empty state when the org has no schedules', () => {
		render(Page, { props: { data: makeData({ schedules: [], lastRuns: [] }) } });

		expect(screen.getByTestId('empty-state')).toHaveTextContent('No schedules yet');
		expect(screen.queryByTestId('schedule-name-link')).not.toBeInTheDocument();
	});

	it('shows an error state instead of the table when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'schedules unavailable' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('schedules unavailable');
		expect(screen.queryByTestId('schedule-name-link')).not.toBeInTheDocument();
	});
});
