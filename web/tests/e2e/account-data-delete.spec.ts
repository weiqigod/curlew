/**
 * M18-006: GDPR /account/data delete panel — Playwright e2e smoke spec.
 *
 * Covers two scenarios via route interception (no live stack needed):
 *  1. Happy-path: click Delete → re-auth modal → POST deletion-requests → 202 → success state.
 *  2. Blocking-orgs path: DELETE request returns 409 owner_cannot_leave → panel
 *     renders blocking-orgs list.
 *
 * DoD item: "Playwright smoke covers the delete-with-blocking-orgs case."
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';

const ACCOUNT_DATA_URL = '/account/data';

const REAUTH_OK = { reauth_token: 'drto_e2etest123' };
const DELETION_202 = {
	finalizes_at: '2026-06-17T03:00:00Z',
	cancellable_until: '2026-06-17T03:00:00Z',
	cancel_url: 'http://web.test/account/data/cancel-deletion',
};
const BLOCKING_409 = {
	type: 'https://tools.ietf.org/html/rfc9110',
	title: 'Owner cannot leave',
	status: 409,
	detail: 'Transfer ownership before deleting.',
	code: 'owner_cannot_leave',
	error_code: 'OwnerCannotLeave',
	blocking_orgs: [{ slug: 'my-org', name: 'My Org' }],
};

test.describe('account/data delete panel', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	test('happy-path: delete → re-auth modal → 202 → success state', async ({
		page,
		context
	}) => {
		// Mock reauth endpoint
		await context.route('**/api/v1/auth/reauth', (route) => {
			return route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(REAUTH_OK),
			});
		});

		// Mock deletion endpoint → 202
		await context.route('**/api/v1/users/me/deletion-requests', (route) => {
			if (route.request().method() === 'POST') {
				return route.fulfill({
					status: 202,
					contentType: 'application/json',
					body: JSON.stringify(DELETION_202),
				});
			}
			return route.continue();
		});

		await page.goto(ACCOUNT_DATA_URL);

		// Delete account panel should be visible
		await expect(page.getByRole('region', { name: 'Delete account' })).toBeVisible();

		// Click "Delete my account"
		await page.getByRole('button', { name: 'Delete my account' }).click();

		// Confirm banner appears
		await page.getByRole('button', { name: 'Yes, delete my account' }).click();

		// Re-auth modal appears
		await page.getByLabel('Password').fill('TestPassword!1');
		await page.getByRole('button', { name: 'Confirm deletion' }).click();

		// Success state
		await expect(page.getByRole('status')).toContainText('scheduled for deletion');
		await expect(page.getByRole('link', { name: 'Cancel deletion' })).toBeVisible();
	});

	test('blocking-orgs path: 409 owner_cannot_leave → blocking orgs list', async ({
		page,
		context
	}) => {
		// Mock reauth endpoint
		await context.route('**/api/v1/auth/reauth', (route) => {
			return route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(REAUTH_OK),
			});
		});

		// Mock deletion endpoint → 409
		await context.route('**/api/v1/users/me/deletion-requests', (route) => {
			if (route.request().method() === 'POST') {
				return route.fulfill({
					status: 409,
					contentType: 'application/json',
					body: JSON.stringify(BLOCKING_409),
				});
			}
			return route.continue();
		});

		await page.goto(ACCOUNT_DATA_URL);

		// Click Delete → confirm → re-auth
		await page.getByRole('button', { name: 'Delete my account' }).click();
		await page.getByRole('button', { name: 'Yes, delete my account' }).click();
		await page.getByLabel('Password').fill('TestPassword!1');
		await page.getByRole('button', { name: 'Confirm deletion' }).click();

		// Blocking-orgs panel appears
		await expect(page.getByRole('alert')).toContainText('sole owner');
		await expect(page.getByText('My Org')).toBeVisible();
		await expect(page.getByRole('link', { name: 'Transfer ownership' })).toBeVisible();
	});
});
