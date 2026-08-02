import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, SEEDED_ORG_ID, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const BILLING_URL = `/org/${SEEDED_ORG_SLUG}/billing`;
const MEMBERS_URL = `/org/${SEEDED_ORG_SLUG}/members`;

/** Reusable subscription fixture. */
function makeSubPayload(seatCount = 3) {
	return {
		subscription: {
			id: 'sub_test',
			org_id: SEEDED_ORG_ID,
			tier: 'team',
			status: 'active',
			interval: 'month',
			seat_count: seatCount,
			seat_limit: 10,
			current_period_start: '2026-04-01T00:00:00Z',
			current_period_end: '2026-05-01T00:00:00Z',
			cancel_at_period_end: false,
			created_at: '2026-04-01T00:00:00Z'
		},
		tier: 'team'
	};
}

function makeOrgPayload(role: 'owner' | 'admin' | 'member', tier = 'team') {
	const org = {
		id: SEEDED_ORG_ID,
		slug: SEEDED_ORG_SLUG,
		name: 'Acme',
		role,
		seat_count: 3,
		seat_limit: 10,
		status: 'active',
		created_at: '2026-04-01T00:00:00Z',
		tier
	};
	return { organizations: [org], org };
}

test.describe('Team billing & members portal', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
	});

	// Assertion 1: owner sees subscription card on /billing
	test('1. owner sees subscription card on /billing', async ({ page, context }) => {
		await context.route('**/api/v1/subscriptions?**', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(makeSubPayload(3)) })
		);
		await page.goto(BILLING_URL);
		// Subscription card visible
		await expect(page.getByTestId('subscription-card')).toBeVisible();
		// Tier is displayed
		await expect(page.getByTestId('subscription-tier')).toContainText('team');
		// Price is displayed
		await expect(page.getByTestId('subscription-price')).toBeVisible();
		// Renewal date is displayed (current_period_end = 2026-05-01)
		await expect(page.getByTestId('subscription-renewal')).toContainText('2026');
		// Seats usage bar has correct aria-valuenow
		await expect(page.getByTestId('seats-usage-bar')).toHaveAttribute('aria-valuenow', '3');
	});

	// Assertion 2: owner adds +3 seats and sees proration preview
	test('2. owner adds +3 seats and sees proration preview', async ({ page, context }) => {
		let patchCalled = false;

		await context.route('**/api/v1/subscriptions?**', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(makeSubPayload(3)) })
		);

		await context.route('**/api/v1/subscriptions/sub_test', async (route) => {
			if (route.request().method() === 'PATCH') {
				patchCalled = true;
				const body = JSON.parse(route.request().postData() ?? '{}');
				const newCount = body.seat_count ?? 3;
				route.fulfill({
					status: 200,
					body: JSON.stringify({
						subscription: { ...makeSubPayload(newCount).subscription, seat_count: newCount },
						proration: { credit: 0, charge: 5400, net: 5400 }
					})
				});
			} else {
				await route.continue();
			}
		});

		await page.goto(BILLING_URL);
		await expect(page.getByTestId('add-seats-button')).toBeVisible();
		await page.getByTestId('add-seats-button').click();
		await expect(page.getByTestId('add-seats-modal')).toBeVisible();

		// Clear default and enter 3
		await page.getByTestId('add-seats-delta').fill('3');
		await page.getByTestId('add-seats-submit').click();

		// Proration result appears (net = 5400 cents = $54.00)
		await expect(page.getByTestId('add-seats-proration')).toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('add-seats-proration')).toContainText('$54');
		expect(patchCalled).toBe(true);
	});

	// Assertion 3: non-owner member redirected from /billing with owner_required toast
	test('3. non-owner member is redirected from /billing with owner_required toast', async ({
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
		// Return member org for slug lookup
		await context.route(`**/api/v1/organizations?slug=**`, (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ organizations }) })
		);

		await page.goto(BILLING_URL);
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page.getByTestId('toast-owner-required')).toBeVisible();
	});

	// Assertion 4: admin invites member and sees pending row
	test('4. admin invites newuser@example.com and sees pending invitation row', async ({
		page,
		context
	}) => {
		const newInv = {
			id: 'inv_test123',
			org_id: SEEDED_ORG_ID,
			email: 'newuser@example.com',
			role: 'member',
			expires_at: '2026-04-24T00:00:00Z',
			created_at: '2026-04-17T00:00:00Z',
			accepted_at: null,
			revoked_at: null
		};

		let getCount = 0;
		await context.route('**/api/v1/organizations/*/invitations', async (route) => {
			if (route.request().method() === 'GET') {
				getCount++;
				route.fulfill({
					status: 200,
					body: JSON.stringify({ invitations: getCount === 1 ? [] : [newInv] })
				});
			} else if (route.request().method() === 'POST') {
				route.fulfill({
					status: 201,
					body: JSON.stringify({ invitation: newInv, token: 'raw_tok' })
				});
			} else {
				await route.continue();
			}
		});

		await context.route('**/api/v1/organizations/*/members', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ members: [] }) })
		);

		await page.goto(MEMBERS_URL);

		// Both tables are rendered on the members page
		await expect(page.getByTestId('members-table')).toBeVisible();
		await expect(page.getByTestId('invitations-table')).toBeVisible();

		await expect(page.getByTestId('invite-button')).toBeVisible();
		await page.getByTestId('invite-button').click();
		await expect(page.getByTestId('invite-modal')).toBeVisible();

		await page.getByTestId('invite-modal-email').fill('newuser@example.com');
		await page.getByTestId('invite-modal-submit').click();

		// Modal closes and new invitation row appears
		await expect(page.getByTestId('invite-modal')).not.toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId(`invitation-row-${newInv.id}`)).toBeVisible({ timeout: 5000 });
	});

	// Assertion 5: admin cancels a pending invitation
	test('5. admin cancels a pending invitation', async ({ page, context }) => {
		const invId = 'inv_tocancel';
		const pendingInv = {
			id: invId,
			org_id: SEEDED_ORG_ID,
			email: 'pending@example.com',
			role: 'member',
			expires_at: '2026-04-24T00:00:00Z',
			created_at: '2026-04-17T00:00:00Z',
			accepted_at: null,
			revoked_at: null
		};

		let deleteCount = 0;
		await context.route('**/api/v1/organizations/*/invitations', async (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					body: JSON.stringify({ invitations: deleteCount === 0 ? [pendingInv] : [] })
				});
			} else {
				await route.continue();
			}
		});

		await context.route(`**/api/v1/organizations/*/invitations/${invId}`, async (route) => {
			if (route.request().method() === 'DELETE') {
				deleteCount++;
				route.fulfill({ status: 204, body: '' });
			} else {
				await route.continue();
			}
		});

		await context.route('**/api/v1/organizations/*/members', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify({ members: [] }) })
		);

		await page.goto(MEMBERS_URL);
		await expect(page.getByTestId(`invitation-row-${invId}`)).toBeVisible();

		// Click cancel — shows confirmation banner
		await page.getByTestId(`invitation-cancel-${invId}`).click();
		await expect(page.getByTestId('cancel-confirm-banner')).toBeVisible();

		// Confirm cancellation
		await page.getByTestId('cancel-confirm-yes').click();

		// Row disappears after revoke + invalidateAll
		await expect(page.getByTestId(`invitation-row-${invId}`)).not.toBeVisible({ timeout: 5000 });
	});

	// Assertion 6: manage-billing click calls /portal and dispatches POST
	test('6. manage-billing click calls /portal endpoint', async ({ page, context }) => {
		await context.route('**/api/v1/subscriptions?**', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(makeSubPayload()) })
		);

		// Intercept the portal POST and respond with a URL that keeps us in the app
		await context.route('**/api/v1/subscriptions/portal', (route) =>
			route.fulfill({
				status: 200,
				body: JSON.stringify({ portal_url: `http://localhost:3000/org/${SEEDED_ORG_SLUG}` })
			})
		);

		await page.goto(BILLING_URL);
		await expect(page.getByTestId('manage-billing-button')).toBeVisible();

		const [req] = await Promise.all([
			page.waitForRequest((r) => r.url().includes('/api/v1/subscriptions/portal')),
			page.getByTestId('manage-billing-button').click()
		]);
		expect(req.method()).toBe('POST');
	});

	// Assertion 7: Billing and Members links visible in org sub-nav for owner
	test('7. billing and members links visible in org sub-nav for owner', async ({
		page,
		context
	}) => {
		// The stack will return the owner's org through the layout server load.
		// For owners, both links should appear.
		await context.route('**/api/v1/subscriptions?**', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(makeSubPayload()) })
		);

		await page.goto(BILLING_URL);
		await expect(page.getByTestId('subnav-billing-link')).toBeVisible();
		await expect(page.getByTestId('subnav-members-link')).toBeVisible();
	});

	// Assertion 8: email_not_verified 403 from update opens resend modal
	test('8. email_not_verified 403 from subscription update opens email-verify modal', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/subscriptions?**', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(makeSubPayload(3)) })
		);

		// Mock PATCH to return 403 email-not-verified
		await context.route('**/api/v1/subscriptions/sub_test', async (route) => {
			if (route.request().method() === 'PATCH') {
				route.fulfill({
					status: 403,
					contentType: 'application/problem+json',
					body: JSON.stringify({
						type: 'https://apitool.dev/errors/email-not-verified',
						title: 'Email not verified',
						status: 403,
						detail: 'You must verify your email address before managing your subscription.'
					})
				});
			} else {
				await route.continue();
			}
		});

		// Mock the resend endpoint
		await context.route('**/api/v1/auth/email-verification/resend', (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ ok: true })
			})
		);

		await page.goto(BILLING_URL);
		await expect(page.getByTestId('subscription-card')).toBeVisible();

		// Open add-seats modal and submit (triggers the 403)
		await page.getByTestId('add-seats-button').click();
		await expect(page.getByTestId('add-seats-modal')).toBeVisible();
		await page.getByTestId('add-seats-delta').fill('1');
		await page.getByTestId('add-seats-submit').click();

		// Email verified required modal should appear
		await expect(page.getByTestId('email-verified-required-modal')).toBeVisible({
			timeout: 5000
		});

		// Fill email and click Resend
		await page.getByLabel('Email address').fill('owner@example.com');
		await page.getByTestId('resend-verification-button').click();

		// Confirmation appears
		await expect(page.getByTestId('verification-resent-confirmation')).toBeVisible({
			timeout: 5000
		});
	});
});
