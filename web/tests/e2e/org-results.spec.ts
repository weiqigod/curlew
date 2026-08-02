import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL } from './helpers/fixtures';

test.describe('Team test results dashboard', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
	});

	test('renders overview stats for a seeded team org', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		await expect(page.getByTestId('summary-total-runs')).toContainText(/\d+/);
		await expect(page.getByTestId('summary-pass-count')).toBeVisible();
		await expect(page.getByTestId('summary-fail-count')).toBeVisible();
		await expect(page.getByTestId('summary-pass-rate')).toContainText('%');
		await expect(page.getByTestId('summary-avg-duration')).toBeVisible();
		await expect(page.getByTestId('trend-chart')).toBeVisible();
		// Subnav "Test Results" link is visible for team-tier orgs
		await expect(page.getByTestId('subnav-results-link')).toBeVisible();
	});

	test('recent runs table renders newest-first rows seeded by fixtures', async ({ page }) => {
		// Use ?range=all to avoid time-dependent filtering on hardcoded fixture dates.
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results?range=all`);
		const rows = page.getByTestId('recent-runs-table').locator('tbody tr');
		await expect(rows).toHaveCount(5);
		// First row should be the newest — smoke-tests run on 2026-04-15
		await expect(rows.nth(0)).toContainText('smoke-tests');
	});

	test('empty state shown when backend has no results', async ({ page, context }) => {
		await context.route(`**/api/v1/organizations/*/results**`, (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ results: [] }) })
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		await expect(page.getByTestId('empty-state')).toContainText('Run curlew and upload results');
		await expect(page.getByTestId('summary-total-runs')).toHaveCount(0);
	});

	test('non-team-tier member is redirected with toast', async ({ page, context }) => {
		const orgPayload = JSON.stringify({
			organizations: [
				{
					id: 'org_123',
					slug: SEEDED_ORG_SLUG,
					name: 'Acme',
					role: 'member',
					seat_count: 1,
					seat_limit: 10,
					status: 'active',
					created_at: '2026-04-01T00:00:00Z',
					tier: 'professional'
				}
			]
		});
		// Intercept list + per-org detail calls so /org/[slug] overview page also
		// sees the professional-tier org and renders the layout correctly.
		await context.route('**/api/v1/organizations', (route) =>
			route.fulfill({ status: 200, body: orgPayload })
		);
		await context.route(`**/api/v1/organizations/org_123`, (route) =>
			route.fulfill({
				status: 200,
				body: JSON.stringify({
					id: 'org_123',
					slug: SEEDED_ORG_SLUG,
					name: 'Acme',
					role: 'member',
					seat_count: 1,
					seat_limit: 10,
					status: 'active',
					created_at: '2026-04-01T00:00:00Z',
					tier: 'professional'
				})
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		// Should be redirected to /org/[slug] — not staying on /results
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page).not.toHaveURL(/\/results/);
		// Toast banner must be visible on the redirect destination page
		await expect(page.getByTestId('toast-team-tier-required')).toBeVisible();
		// Subnav "Test Results" link must NOT be shown for non-team-tier orgs
		await expect(page.getByTestId('subnav-results-link')).not.toBeVisible();
	});

	test('time-range picker re-queries with ?range=7d', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		await page.getByTestId('range-picker-7d').click();
		await expect(page).toHaveURL(/\?range=7d/);
	});

	test('error state with retry button is shown on 500', async ({ page, context }) => {
		await context.route(`**/api/v1/organizations/*/results**`, (route) =>
			route.fulfill({
				status: 500,
				body: JSON.stringify({ code: 'server_error', description: 'boom' })
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results`);
		await expect(page.getByTestId('error-state')).toBeVisible();
		await expect(page.getByTestId('error-retry-button')).toBeVisible();
		await expect(page.getByTestId('summary-total-runs')).toHaveCount(0);
	});
});
