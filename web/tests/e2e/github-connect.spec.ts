import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, SEEDED_ORG_ID, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const INTEGRATIONS_URL = '/integrations';

function makeOrgPayload(role: 'owner' | 'admin' | 'member') {
	return {
		organizations: [
			{
				id: SEEDED_ORG_ID,
				slug: SEEDED_ORG_SLUG,
				name: 'Acme',
				role,
				seat_count: 3,
				seat_limit: 10,
				status: 'active',
				created_at: '2026-04-01T00:00:00Z',
				tier: 'team'
			}
		]
	};
}

function makeInstallPayload(
	opts: {
		claimed?: boolean;
		suspended?: boolean;
		accountLogin?: string;
		repoCount?: number;
	} = {}
) {
	const {
		claimed = true,
		suspended = false,
		accountLogin = 'curlew-checks-test',
		repoCount = 3
	} = opts;
	return {
		installation: {
			installation_id: 12345,
			account_login: accountLogin,
			account_type: 'Organization',
			repo_set: Array.from({ length: repoCount }, (_, i) => ({
				owner: 'acme',
				name: `repo-${i}`,
				id: 1000 + i
			})),
			claimed_at: claimed ? '2026-05-01T00:00:00Z' : null,
			suspended_at: suspended ? '2026-05-04T00:00:00Z' : null
		}
	};
}

test.describe('GitHub install entry-point + install-state view', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeOrgPayload('owner')) })
		);
	});

	// Behaviour 1: admin sees Connect button linking to install-url
	test('admin sees Connect GitHub button on /integrations', async ({ page, context }) => {
		await context.route('**/api/v1/integrations/github/install-url', (r) =>
			r.fulfill({
				status: 200,
				body: JSON.stringify({
					install_url:
						'https://github.com/apps/curlew-checks-test/installations/new?state=abc',
					state_expires_at: '2026-05-06T01:00:00Z'
				})
			})
		);
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({
				status: 404,
				body: JSON.stringify({ code: 'not_installed', description: 'no install' })
			})
		);

		await page.goto(INTEGRATIONS_URL);
		const btn = page.getByTestId('github-connect-button');
		await expect(btn).toBeVisible();
		await expect(btn).toHaveAttribute(
			'href',
			/github\.com\/apps\/curlew-checks-test\/installations\/new\?state=/
		);
	});

	// Behaviour 2: member sees read-only card, no Connect button
	test('member sees read-only integration card (no Connect button)', async ({ page, context }) => {
		await context.unroute('**/api/v1/organizations').catch(() => {});
		await context.clearCookies();
		await seedAuthCookie(context, MEMBER_EMAIL);
		await context.route('**/api/v1/organizations', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeOrgPayload('member')) })
		);
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({
				status: 404,
				body: JSON.stringify({ code: 'not_installed', description: 'no install' })
			})
		);

		await page.goto(INTEGRATIONS_URL);
		await expect(page.getByTestId('github-integration-card')).toBeVisible();
		await expect(page.getByTestId('github-connect-button')).not.toBeVisible();
		await expect(page.getByTestId('github-admin-only-tooltip')).toBeVisible();
	});

	// Behaviour 3: connected with N repos
	test('shows "GitHub connected — <slug>, 3 repos covered"', async ({ page, context }) => {
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeInstallPayload({ repoCount: 3 })) })
		);
		await page.goto(INTEGRATIONS_URL);
		const status = page.getByTestId('github-install-status');
		await expect(status).toContainText('GitHub connected');
		await expect(status).toContainText('curlew-checks-test');
		await expect(status).toContainText('3 repos covered');
	});

	// Behaviour 4: suspended badge — badge visible, Connect button hidden, description shown
	test('suspended install shows yellow warning badge and hides Connect button', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/integrations/github/install-url', (r) =>
			r.fulfill({
				status: 200,
				body: JSON.stringify({
					install_url:
						'https://github.com/apps/curlew-checks-test/installations/new?state=abc',
					state_expires_at: '2026-05-06T01:00:00Z'
				})
			})
		);
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({
				status: 200,
				body: JSON.stringify(makeInstallPayload({ suspended: true }))
			})
		);
		await page.goto(INTEGRATIONS_URL);
		const badge = page.getByTestId('github-suspended-badge');
		await expect(badge).toBeVisible();
		await expect(badge).toContainText('Suspended on GitHub');
		// Connect button must NOT be shown — the user should unsuspend on GitHub, not reconnect
		await expect(page.getByTestId('github-connect-button')).not.toBeVisible();
		// Descriptive text for the suspended state must be present
		const desc = page.getByTestId('github-suspended-description');
		await expect(desc).toBeVisible();
		await expect(desc).toContainText('suspended');
	});

	// Behaviour 5: pending-claim button
	test('webhook-first pending install shows "Claim this install"', async ({ page, context }) => {
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeInstallPayload({ claimed: false })) })
		);
		await page.goto(INTEGRATIONS_URL);
		await expect(page.getByTestId('github-claim-button')).toBeVisible();
	});

	// Behaviour 6: callback success toast
	test('?installed=true shows one-time success toast', async ({ page, context }) => {
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({ status: 200, body: JSON.stringify(makeInstallPayload()) })
		);
		await page.goto(`${INTEGRATIONS_URL}?installed=true`);
		await expect(page.getByTestId('toast-github-connected')).toBeVisible();
		await expect(page.getByTestId('toast-github-connected')).toContainText('GitHub connected');
	});

	// Behaviour 7: bounded scope — page does not show billing/SSO content
	test('integrations page is bounded — no billing or license display', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/integrations/github', (r) =>
			r.fulfill({
				status: 404,
				body: JSON.stringify({ code: 'not_installed', description: 'none' })
			})
		);
		await page.goto(INTEGRATIONS_URL);
		// Negative assertions enforcing the Open Decision #10 boundary
		await expect(page.getByTestId('subscription-card')).not.toBeVisible();
		await expect(page.getByTestId('license-display')).not.toBeVisible();
	});
});
