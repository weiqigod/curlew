import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, SEEDED_ORG_ID, OWNER_EMAIL } from './helpers/fixtures';

const STATS_BODY = JSON.stringify({
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
});

const FAILURES_BODY = JSON.stringify({
	window: '30d',
	limit: 10,
	limit_clamped: false,
	items: [
		{
			method: 'GET',
			path_template: '/api/users/:id',
			failure_count: 3,
			first_seen_at: '2026-05-01T00:00:00Z',
			last_seen_at: '2026-05-11T00:00:00Z',
			sample_run_ids: ['run_abc']
		}
	]
});

test.describe('Team dashboard', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
		// Intercept the dashboard API calls so tests don't depend on seeded run data
		await context.route('**/api/v1/organizations/*/results/stats**', (r) =>
			r.fulfill({ status: 200, body: STATS_BODY })
		);
		await context.route('**/api/v1/organizations/*/results/failures**', (r) =>
			r.fulfill({ status: 200, body: FAILURES_BODY })
		);
	});

	// Behavior 2 + 3 + 6: team-tier load renders overview cards + trend chart + failing endpoints
	test('renders overview cards, trend chart, and failing endpoints for seeded team org', async ({
		page
	}) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard`);
		await expect(page.getByTestId('dashboard-total-runs')).toContainText('42');
		await expect(page.getByTestId('dashboard-pass-rate')).toContainText('%');
		await expect(page.getByTestId('dashboard-avg-duration')).toBeVisible();
		await expect(page.getByTestId('dashboard-p50-duration')).toBeVisible();
		await expect(page.getByTestId('dashboard-p95-duration')).toBeVisible();
		await expect(page.getByTestId('dashboard-trend-chart')).toBeVisible();
		// Behavior 6: failing-endpoint row content — method, path, failure count, relative time
		const firstRow = page.getByTestId('failing-endpoint-row').first();
		await expect(firstRow).toBeVisible();
		await expect(page.getByTestId('failing-endpoint-method').first()).toContainText('GET');
		await expect(page.getByTestId('failing-endpoint-path').first()).toContainText('/api/users/:id');
		await expect(page.getByTestId('failing-endpoint-count').first()).toContainText('3');
		await expect(page.getByTestId('failing-endpoint-last-seen').first()).toHaveText(/\d+[smhd] ago/);
	});

	// Behavior 8: sidebar nav entry → page reachable from "Dashboard" link
	test('page is reachable from the org sidebar nav', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		await page.getByTestId('subnav-dashboard-link').click();
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}/dashboard`));
		await expect(page.getByTestId('dashboard-total-runs')).toBeVisible();
	});

	// Behavior 4: window selector re-queries with ?window=7d
	test('window picker re-queries with ?window=7d', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard`);
		await page.getByTestId('dashboard-window-7d').click();
		await expect(page).toHaveURL(/\?window=7d/);
		await expect(page.getByTestId('dashboard-total-runs')).toBeVisible();
	});

	test('window picker re-queries with ?window=90d', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard?window=30d`);
		await page.getByTestId('dashboard-window-90d').click();
		await expect(page).toHaveURL(/\?window=90d/);
	});

	// Behavior 1: Free-tier renders tier-gate prompt in place (org.tier = 'free' short-circuit)
	test('free-tier org sees in-place upgrade prompt', async ({ page, context }) => {
		const orgPayload = JSON.stringify({
			organizations: [
				{
					id: SEEDED_ORG_ID,
					slug: SEEDED_ORG_SLUG,
					name: 'Acme',
					role: 'owner',
					seat_count: 1,
					seat_limit: 1,
					status: 'active',
					created_at: '2026-04-01T00:00:00Z',
					tier: 'free'
				}
			]
		});
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: orgPayload })
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard`);
		await expect(page.getByTestId('dashboard-tier-gate')).toBeVisible();
		await expect(page.getByTestId('dashboard-upgrade-link')).toBeVisible();
		await expect(page.getByTestId('dashboard-total-runs')).toHaveCount(0);
	});

	// Behavior 1 alt: backend-driven 402 — intercept stats endpoint
	test('402 from /results/stats renders tier-gate prompt without crashing', async ({
		page,
		context
	}) => {
		// Override the beforeEach route for stats with a 402 response
		await context.route('**/api/v1/organizations/*/results/stats**', (r) =>
			r.fulfill({
				status: 402,
				contentType: 'application/problem+json',
				body: JSON.stringify({
					type: 'https://api.apitool.dev/errors/tier-ineligible',
					code: 'dashboard_tier_ineligible',
					title: 'Tier ineligible',
					detail: 'This feature requires the team tier or above.'
				})
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard`);
		await expect(page.getByTestId('dashboard-tier-gate')).toBeVisible();
	});

	// Behavior 5: ?window=14d → backend 400 → dashboard renders error state, no crash
	test('invalid ?window=14d renders error state without crashing', async ({ page, context }) => {
		// Override stats route with 400 for this test
		await context.route('**/api/v1/organizations/*/results/stats**', (r) =>
			r.fulfill({
				status: 400,
				contentType: 'application/problem+json',
				body: JSON.stringify({
					type: 'https://api.apitool.dev/errors/unsupported-window',
					code: 'unsupported_window',
					title: 'Unsupported window',
					detail: 'Allowed values: 7d, 30d, 90d.',
					received: '14d',
					allowed_values: ['7d', '30d', '90d']
				})
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard?window=14d`);
		await expect(page.getByTestId('error-state')).toBeVisible();
		await expect(page.getByTestId('dashboard-total-runs')).toHaveCount(0);
	});

	// Behavior 7: empty-state when zero runs in window
	test('empty state shown when stats has zero runs', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/results/stats**', (r) =>
			r.fulfill({
				status: 200,
				body: JSON.stringify({
					window: '30d',
					window_start: '2026-04-12T00:00:00Z',
					window_end: '2026-05-12T00:00:00Z',
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
				})
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/dashboard`);
		await expect(page.getByTestId('empty-state')).toContainText('No runs in this window');
		await expect(page.getByTestId('dashboard-trend-chart')).toHaveCount(0);
	});
});
