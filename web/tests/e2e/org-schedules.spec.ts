import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL } from './helpers/fixtures';

test.describe('Team schedules dashboard', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
	});

	test('renders schedules list with name/cron/timezone for team org', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		// Subnav Schedules link is visible for team-tier admins
		await expect(page.getByTestId('subnav-schedules-link')).toBeVisible();
		// At least one schedule row with name, cron, timezone columns
		const nameLinks = page.getByTestId('schedule-name-link');
		await expect(nameLinks.first()).toBeVisible();
		// New schedule button
		await expect(page.getByTestId('new-schedule-btn')).toBeVisible();
	});

	test('create-schedule modal shows live cron preview in selected timezone', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		await page.getByTestId('new-schedule-btn').click();

		// Modal should open with cron input and preview
		await expect(page.getByTestId('sched-cron')).toBeVisible();
		// Fill in a valid cron expression
		await page.getByTestId('sched-cron').fill('0 9 * * *');
		// Cron preview should appear
		await expect(page.getByTestId('cron-preview')).toBeVisible({ timeout: 2000 });
	});

	test('Run now button enqueues a run and shows queued badge', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		await page.getByTestId('run-now-btn').first().click();
		// Queued feedback badge should appear
		await expect(page.getByTestId('run-now-feedback').first()).toContainText('queued', { timeout: 5000 });
	});

	test('clicking schedule row navigates to run history page', async ({ page }) => {
		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		await page.getByTestId('schedule-name-link').first().click();
		// Should navigate to /org/[slug]/schedules/[name]
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}/schedules/`));
	});

	test('422 invalid-timezone surfaces inline form error', async ({ page, context }) => {
		await context.route(`**/organizations/**/schedules`, async (route) => {
			if (route.request().method() === 'POST') {
				await route.fulfill({
					status: 422,
					contentType: 'application/problem+json',
					body: JSON.stringify({
						type: 'https://api.apitool.dev/errors/invalid-timezone',
						title: 'Invalid IANA timezone',
						detail: 'Unknown IANA timezone: Bad/Zone',
						status: 422,
						valid_examples: ['UTC', 'Europe/Stockholm']
					})
				});
			} else {
				await route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		await page.getByTestId('new-schedule-btn').click();
		await page.getByTestId('sched-name').fill('bad-tz-sched');
		await page.getByTestId('sched-cron').fill('0 9 * * *');
		await page.getByTestId('sched-ref').fill('smoke.yaml');
		await page.getByTestId('sched-submit').click();
		// The modal should stay open and surface the API error as a form-level error message
		await expect(page.getByTestId('sched-form-error')).toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('sched-form-error')).toContainText(/timezone|Invalid|422/i);
	});

	test('non-team-tier org is redirected with tier-gate toast', async ({ page, context }) => {
		// Override the organizations endpoint to return a professional-tier org
		await context.route(`**/api/v1/users/me/organizations`, (route) =>
			route.fulfill({
				status: 200,
				body: JSON.stringify({
					organizations: [
						{
							id: 'org_free',
							slug: SEEDED_ORG_SLUG,
							name: 'Free Org',
							role: 'owner',
							seat_count: 1,
							seat_limit: 5,
							status: 'active',
							created_at: '2026-01-01T00:00:00Z',
							tier: 'professional'
						}
					]
				})
			})
		);
		await page.goto(`/org/${SEEDED_ORG_SLUG}/schedules`);
		// Should redirect to org home with team_tier_required toast
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}\\?toast=team_tier_required`));
		await expect(page.getByTestId('toast-team-tier-required')).toBeVisible();
	});
});
