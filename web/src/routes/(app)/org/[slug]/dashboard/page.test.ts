import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import type { StatsResponse, FailuresResponse } from '$lib/types/dashboard';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

vi.mock('$app/stores', async () => {
	const { readable } = await import('svelte/store');
	return {
		page: readable({ url: new URL('http://localhost/org/acme/dashboard?window=30d') })
	};
});

const STATS: StatsResponse = {
	window: '30d',
	window_start: '2026-04-12T00:00:00Z',
	window_end: '2026-05-12T00:00:00Z',
	totals: {
		runs: 42,
		pass_count: 38,
		fail_count: 4,
		skipped_count: 0,
		pass_rate: 0.9048,
		avg_duration_ms: 350,
		p50_duration_ms: 300,
		p95_duration_ms: 900
	},
	trend: [
		{
			date: '2026-05-11',
			runs: 10,
			pass_count: 9,
			fail_count: 1,
			pass_rate: 0.9,
			avg_duration_ms: 350
		}
	]
} as StatsResponse;

const FAILURES: FailuresResponse = {
	window: '30d',
	limit: 10,
	limit_clamped: false,
	items: [
		{
			method: 'GET',
			path_template: '/api/users/:id',
			failure_count: 3,
			first_seen_at: '2026-05-01T00:00:00Z',
			// Relative to "now" so the rendered value stays a stable "…h ago".
			last_seen_at: new Date(Date.now() - 3 * 3600 * 1000).toISOString(),
			sample_run_ids: ['run_abc']
		}
	]
} as FailuresResponse;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		window: '30d',
		tierGate: false,
		stats: STATS,
		failures: FAILURES,
		recentRuns: [],
		windowError: null,
		loadError: null,
		...overrides
	};
}

describe('dashboard page', () => {
	it('renders overview cards, trend chart, and failing endpoints', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('dashboard-total-runs')).toHaveTextContent('42');
		expect(screen.getByTestId('dashboard-pass-rate')).toHaveTextContent('90.48%');
		expect(screen.getByTestId('dashboard-avg-duration')).toHaveTextContent('350ms');
		expect(screen.getByTestId('dashboard-p50-duration')).toHaveTextContent('300ms');
		expect(screen.getByTestId('dashboard-p95-duration')).toHaveTextContent('900ms');
		expect(screen.getByTestId('dashboard-trend-chart')).toBeVisible();

		expect(screen.getAllByTestId('failing-endpoint-row')).toHaveLength(1);
		expect(screen.getByTestId('failing-endpoint-method')).toHaveTextContent('GET');
		expect(screen.getByTestId('failing-endpoint-path')).toHaveTextContent('/api/users/:id');
		expect(screen.getByTestId('failing-endpoint-count')).toHaveTextContent('3');
		expect(screen.getByTestId('failing-endpoint-last-seen')).toHaveTextContent(/^\d+[smhd] ago$/);
	});

	it('points each window-picker tab at its ?window= query and marks the current one', () => {
		render(Page, { props: { data: makeData({ window: '30d' }) } });

		expect(screen.getByTestId('dashboard-window-7d')).toHaveAttribute(
			'href',
			'/org/acme/dashboard?window=7d'
		);
		expect(screen.getByTestId('dashboard-window-90d')).toHaveAttribute(
			'href',
			'/org/acme/dashboard?window=90d'
		);
		expect(screen.getByTestId('dashboard-window-30d')).toHaveAttribute('aria-current', 'page');
		expect(screen.getByTestId('dashboard-window-7d')).not.toHaveAttribute('aria-current');
	});

	it('renders the in-place upgrade prompt and no stats when tier-gated', () => {
		render(Page, { props: { data: makeData({ tierGate: true, stats: null, failures: null }) } });

		expect(screen.getByTestId('dashboard-tier-gate')).toBeVisible();
		expect(screen.getByTestId('dashboard-upgrade-link')).toHaveAttribute(
			'href',
			'/org/acme/billing'
		);
		expect(screen.queryByTestId('dashboard-total-runs')).not.toBeInTheDocument();
		// The window picker is meaningless behind the gate.
		expect(screen.queryByTestId('dashboard-window-7d')).not.toBeInTheDocument();
	});

	it('renders an error state for an unsupported window, with a reset link', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						windowError: 'Allowed values: 7d, 30d, 90d.',
						stats: null,
						failures: null
					})
				}
			}
		);

		expect(screen.getByTestId('error-state')).toBeVisible();
		expect(screen.getByTestId('error-state')).toHaveTextContent('Allowed values: 7d, 30d, 90d.');
		expect(screen.getByTestId('error-retry-button')).toHaveAttribute(
			'href',
			'/org/acme/dashboard'
		);
		expect(screen.queryByTestId('dashboard-total-runs')).not.toBeInTheDocument();
	});

	it('renders an error state when the dashboard fails to load outright', () => {
		render(Page, { props: { data: makeData({ loadError: 'backend down', stats: null }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('backend down');
		// Retry returns to the current URL, query string included.
		expect(screen.getByTestId('error-retry-button')).toHaveAttribute(
			'href',
			'/org/acme/dashboard?window=30d'
		);
	});

	it('shows the empty state and hides the chart when the window has zero runs', () => {
		const zeroed = {
			...STATS,
			totals: {
				runs: 0,
				pass_count: 0,
				fail_count: 0,
				skipped_count: 0,
				pass_rate: 0,
				avg_duration_ms: 0,
				p50_duration_ms: 0,
				p95_duration_ms: 0
			},
			trend: []
		};

		render(Page, { props: { data: makeData({ stats: zeroed, failures: null }) } });

		expect(screen.getByTestId('empty-state')).toHaveTextContent('No runs in this window');
		expect(screen.queryByTestId('dashboard-trend-chart')).not.toBeInTheDocument();
	});

	it('renders a no-failures message when the window has no failing endpoints', () => {
		render(Page, { props: { data: makeData({ failures: { ...FAILURES, items: [] } }) } });

		expect(screen.queryByTestId('failing-endpoint-row')).not.toBeInTheDocument();
		expect(screen.getByTestId('dashboard-failing-endpoints')).toHaveTextContent(
			'No failures in this window.'
		);
	});
});
