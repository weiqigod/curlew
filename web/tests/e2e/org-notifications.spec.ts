import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const NOTIF_URL = `/org/${SEEDED_ORG_SLUG}/settings/notifications`;

/**
 * Builds a minimal org payload for context.route mocking.
 */
function makeOrgPayload(role: 'owner' | 'admin' | 'member', tier = 'team') {
	const org = {
		id: 'org_test',
		slug: SEEDED_ORG_SLUG,
		name: 'Acme',
		role,
		seat_count: 1,
		seat_limit: 10,
		status: 'active',
		created_at: '2026-04-01T00:00:00Z',
		tier
	};
	return { organizations: [org], org };
}

test.describe('Team notification settings', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
	});

	// Assertion 1: page renders rules and delivery log from backend
	test('renders rules and delivery log from backend', async ({ page }) => {
		await page.goto(NOTIF_URL);
		// Page title / heading
		await expect(page.getByRole('heading', { name: 'Notification Settings' })).toBeVisible();
		// Rules table section
		await expect(page.getByTestId('rules-table')).toBeVisible();
		// Delivery log section
		await expect(page.getByTestId('delivery-log')).toBeVisible();
		// Subnav notifications link should be visible for owner
		await expect(page.getByTestId('subnav-notifications-link')).toBeVisible();
	});

	// Assertion 2: admin can add a Slack rule via the modal
	test('admin can add a slack rule via the modal', async ({ page }) => {
		await page.goto(NOTIF_URL);
		await page.getByTestId('add-rule-button').click();
		// Modal is open
		await expect(page.getByTestId('notif-modal')).toBeVisible();
		// Fill in the form
		await page.getByTestId('notif-modal-channel').selectOption('slack');
		await page.getByTestId('notif-modal-target').fill('https://hooks.slack.test/e2e');
		await page.getByTestId('notif-modal-on-run_failed').check();
		// Submit
		await page.getByTestId('notif-modal-submit').click();
		// Modal should close and rule should appear (or error shown if backend unreachable)
		// In e2e stack: expect modal closes and rule appears in table
		await expect(page.getByTestId('notif-modal')).not.toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('rules-table')).toBeVisible();
	});

	// Assertion 3: client-side validation blocks non-https slack target
	test('client-side validation blocks non-https slack target', async ({ page }) => {
		await page.goto(NOTIF_URL);
		await page.getByTestId('add-rule-button').click();
		await expect(page.getByTestId('notif-modal')).toBeVisible();

		await page.getByTestId('notif-modal-channel').selectOption('slack');
		await page.getByTestId('notif-modal-target').fill('http://not-https.example.com/hook');
		await page.getByTestId('notif-modal-on-run_failed').check();
		await page.getByTestId('notif-modal-submit').click();

		// Error message appears, modal stays open
		await expect(page.getByTestId('notif-modal-error')).toBeVisible();
		await expect(page.getByTestId('notif-modal-error')).toContainText('https');
		// No API call made — modal still visible
		await expect(page.getByTestId('notif-modal')).toBeVisible();
	});

	// Assertion 4: admin can delete a rule
	test('admin can delete a rule', async ({ page, context }) => {
		const ruleId = 'nrule_aabbccddeeff00112233445566778899';

		// Track how many times the GET notification-rules endpoint has been called.
		// First call (page load) returns one rule; subsequent calls (after delete) return empty.
		let getRulesCallCount = 0;

		// Register ALL route mocks before any navigation to avoid race conditions.
		await context.route(`**/api/v1/organizations/*/notification-rules`, async (route) => {
			if (route.request().method() === 'GET') {
				getRulesCallCount++;
				// First GET (page load): return one rule. Subsequent GETs: return empty list.
				const rules =
					getRulesCallCount === 1
						? [
								{
									id: ruleId,
									channel: 'slack',
									target: 'https://hooks.slack.test/del',
									on: ['run_failed'],
									created_at: '2026-04-17T00:00:00Z'
								}
							]
						: [];
				await route.fulfill({
					status: 200,
					body: JSON.stringify({ rules })
				});
			} else if (route.request().method() === 'DELETE') {
				await route.fulfill({ status: 204, body: '' });
			} else {
				await route.continue();
			}
		});
		await context.route(`**/api/v1/organizations/*/notification-deliveries**`, (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ deliveries: [] }) })
		);

		await page.goto(NOTIF_URL);
		await expect(page.getByTestId(`rule-row-${ruleId}`)).toBeVisible();

		// Click delete — shows inline confirmation banner (no browser dialog)
		await page.getByTestId(`rule-delete-${ruleId}`).click();
		await expect(page.getByTestId('delete-confirm-banner')).toBeVisible();

		// Confirm deletion via inline button
		await page.getByTestId('delete-confirm-yes').click();

		// After delete + invalidateAll, rules list re-fetches and returns empty
		await expect(page.getByTestId(`rule-row-${ruleId}`)).not.toBeVisible({ timeout: 5000 });
	});

	// Assertion 5: member is redirected with admin-required toast
	test('member is redirected to org overview with admin-required toast', async ({
		page,
		context
	}) => {
		await context.clearCookies();
		await seedAuthCookie(context, MEMBER_EMAIL);

		const { organizations, org } = makeOrgPayload('member');
		await context.route('**/api/v1/organizations', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ organizations }) })
		);
		await context.route(`**/api/v1/organizations/${org.id}`, (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(org) })
		);

		await page.goto(NOTIF_URL);
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page.getByTestId('toast-admin-required')).toBeVisible();
	});

	// Assertion 6: delivery log shows distinct status badges for delivered + failed
	test('delivery log shows distinct status badges for delivered and failed', async ({
		page,
		context
	}) => {
		const deliveries = [
			{
				id: 'ndel_111',
				rule_id: 'nrule_aaa',
				channel: 'slack',
				status: 'delivered',
				response_code: 200,
				attempt_count: 1,
				error_message: null,
				attempted_at: '2026-04-17T10:00:00Z'
			},
			{
				id: 'ndel_222',
				rule_id: 'nrule_aaa',
				channel: 'slack',
				status: 'failed',
				response_code: null,
				attempt_count: 1,
				error_message: 'connection refused',
				attempted_at: '2026-04-17T09:00:00Z'
			}
		];

		await context.route(`**/api/v1/organizations/*/notification-deliveries**`, (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ deliveries }) })
		);

		await page.goto(NOTIF_URL);
		await expect(page.getByTestId('delivery-log')).toBeVisible();

		const deliveredBadge = page.getByTestId('delivery-status-ndel_111');
		const failedBadge = page.getByTestId('delivery-status-ndel_222');

		await expect(deliveredBadge).toBeVisible();
		await expect(failedBadge).toBeVisible();
		await expect(deliveredBadge).toContainText('delivered');
		await expect(failedBadge).toContainText('failed');

		// Badges must have distinct classes (different colours)
		const deliveredClass = await deliveredBadge.getAttribute('class');
		const failedClass = await failedBadge.getAttribute('class');
		expect(deliveredClass).not.toBe(failedClass);
	});

	// DoD a11y: modal is keyboard-dismissable and has labelled inputs
	test('modal is keyboard-dismissable and has labelled inputs', async ({ page }) => {
		await page.goto(NOTIF_URL);
		await page.getByTestId('add-rule-button').click();
		await expect(page.getByTestId('notif-modal')).toBeVisible();

		// Labelled inputs
		await expect(page.getByLabel('Channel')).toBeVisible();
		await expect(page.getByLabel(/Webhook URL|Email address/)).toBeVisible();

		// Escape key closes modal
		await page.keyboard.press('Escape');
		await expect(page.getByTestId('notif-modal')).not.toBeVisible({ timeout: 3000 });
	});
});
