import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import { summarize, bucketByDay } from '$lib/results/stats';
import type { Result, TimeRange } from '$lib/types/results';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

vi.mock('$app/stores', async () => {
	const { readable } = await import('svelte/store');
	return { page: readable({ url: new URL('http://localhost/org/acme/results?range=30d') }) };
});

function result(overrides: Partial<Result> = {}): Result {
	return {
		id: 'res_1',
		collection_name: 'smoke-tests',
		run_at: '2026-04-15T10:00:00Z',
		pass_count: 9,
		fail_count: 1,
		duration_ms: 1200,
		triggered_by: 'owner@example.com',
		...overrides
	} as Result;
}

const RESULTS = [
	result({ id: 'res_1', collection_name: 'smoke-tests', run_at: '2026-04-15T10:00:00Z' }),
	result({ id: 'res_2', collection_name: 'regression', run_at: '2026-04-14T10:00:00Z' })
];

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	const results = (overrides.results as Result[]) ?? RESULTS;
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		range: '30d' as TimeRange,
		results,
		summary: summarize(results),
		trend: bucketByDay(results),
		error: null,
		...overrides
	};
}

describe('results page', () => {
	it('renders the overview summary cards and trend chart', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('summary-total-runs')).toHaveTextContent('2');
		expect(screen.getByTestId('summary-pass-count')).toHaveTextContent('18');
		expect(screen.getByTestId('summary-fail-count')).toHaveTextContent('2');
		expect(screen.getByTestId('summary-pass-rate')).toHaveTextContent('%');
		expect(screen.getByTestId('summary-avg-duration')).toBeVisible();
		expect(screen.getByTestId('trend-chart')).toBeVisible();
	});

	it('renders one recent-runs row per result, in the order supplied by load', () => {
		render(Page, { props: { data: makeData() } });

		const rows = screen.getByTestId('recent-runs-table').querySelectorAll('tbody tr');
		expect(rows).toHaveLength(2);
		expect(rows[0]).toHaveTextContent('smoke-tests');
		expect(rows[1]).toHaveTextContent('regression');
	});

	it('points each range-picker tab at its ?range= query and marks the current one', () => {
		render(Page, { props: { data: makeData({ range: '7d' }) } });

		expect(screen.getByTestId('range-picker-24h')).toHaveAttribute(
			'href',
			'/org/acme/results?range=24h'
		);
		expect(screen.getByTestId('range-picker-7d')).toHaveAttribute(
			'href',
			'/org/acme/results?range=7d'
		);
		expect(screen.getByTestId('range-picker-all')).toHaveAttribute(
			'href',
			'/org/acme/results?range=all'
		);
		expect(screen.getByTestId('range-picker-7d')).toHaveAttribute('aria-current', 'page');
	});

	it('shows the empty state and no summary when there are no results', () => {
		render(Page, { props: { data: makeData({ results: [] }) } });

		expect(screen.getByTestId('empty-state')).toHaveTextContent(
			'Run curlew and upload results to get started'
		);
		expect(screen.queryByTestId('summary-total-runs')).not.toBeInTheDocument();
	});

	it('shows an error state with a retry link instead of the summary', () => {
		render(Page, { props: { data: makeData({ error: 'boom' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('boom');
		expect(screen.getByTestId('error-retry-button')).toHaveAttribute(
			'href',
			'/org/acme/results?range=30d'
		);
		expect(screen.queryByTestId('summary-total-runs')).not.toBeInTheDocument();
	});
});
