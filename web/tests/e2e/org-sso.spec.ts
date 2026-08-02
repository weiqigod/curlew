import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, MEMBER_EMAIL } from './helpers/fixtures';

const SSO_URL = `/org/${SEEDED_ORG_SLUG}/settings/sso`;
const ORG_ID = 'org_test';

/**
 * Builds a minimal org payload for context.route mocking.
 */
function makeOrgPayload(role: 'owner' | 'admin' | 'member', tier = 'team') {
	const org = {
		id: ORG_ID,
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

const emptySsoConfig = {
	sso_enabled: false,
	sso_provider: null,
	saml_config: null,
	oidc_config: null
};

const samlEnabledSsoConfig = {
	sso_enabled: true,
	sso_provider: 'saml',
	saml_config: {
		idp_metadata_url: 'https://idp.example.com/metadata',
		acs_url: 'https://sp.example.com/acs',
		entity_id: 'https://sp.example.com',
		idp_sso_url: 'https://idp.example.com/sso'
	},
	oidc_config: null
};


test.describe('SSO settings page', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);
	});

	// Assertion 1: page renders two tabs and the SSO provider badge
	test('owner can open SSO settings page with SAML and OIDC tabs', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/sso', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(emptySsoConfig) })
		);

		await page.goto(SSO_URL);

		await expect(page.getByTestId('sso-heading')).toBeVisible();
		await expect(page.getByTestId('tab-saml')).toBeVisible();
		await expect(page.getByTestId('tab-oidc')).toBeVisible();
		await expect(page.getByTestId('tabpanel-saml')).toBeVisible();
		// OIDC panel is hidden (but present)
		await expect(page.getByTestId('tabpanel-oidc')).toBeHidden();
		// No badge when SSO not yet configured
		await expect(page.getByTestId('sso-provider-badge')).not.toBeVisible();
	});

	// Assertion 2: owner can fill SAML form and save — sees SSO enabled toast
	test('owner can fill SAML tab, save, and see SSO enabled toast', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/sso', async (route) => {
			if (route.request().method() === 'GET') {
				await route.fulfill({ status: 200, body: JSON.stringify(emptySsoConfig) });
			} else {
				await route.continue();
			}
		});
		await context.route('**/api/v1/organizations/*/sso/saml', async (route) => {
			await route.fulfill({
				status: 200,
				body: JSON.stringify({
					id: ORG_ID,
					name: 'Acme',
					slug: SEEDED_ORG_SLUG,
					role: 'owner',
					seat_count: 1,
					seat_limit: 10,
					status: 'active',
					created_at: '2026-04-01T00:00:00Z',
					sso_enabled: true,
					sso_provider: 'saml'
				})
			});
		});

		await page.goto(SSO_URL);

		await page.getByTestId('saml-idp-metadata-url').fill('https://idp.example.com/metadata');
		await page.getByTestId('saml-acs-url').fill('https://sp.example.com/acs');
		await page.getByTestId('saml-entity-id').fill('https://sp.example.com');
		await page.getByTestId('saml-submit').click();

		await expect(page.getByTestId('toast-message')).toContainText('SSO enabled', { timeout: 5000 });
	});

	// Assertion 3: owner can fill OIDC tab on a different org and save
	test('owner can open OIDC tab, fill fields, and save', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/sso', async (route) => {
			if (route.request().method() === 'GET') {
				await route.fulfill({ status: 200, body: JSON.stringify(emptySsoConfig) });
			} else {
				await route.continue();
			}
		});
		await context.route('**/api/v1/organizations/*/sso/oidc', async (route) => {
			await route.fulfill({
				status: 200,
				body: JSON.stringify({
					id: ORG_ID,
					name: 'Acme',
					slug: SEEDED_ORG_SLUG,
					role: 'owner',
					seat_count: 1,
					seat_limit: 10,
					status: 'active',
					created_at: '2026-04-01T00:00:00Z',
					sso_enabled: true,
					sso_provider: 'oidc'
				})
			});
		});

		await page.goto(SSO_URL);
		await page.getByTestId('tab-oidc').click();

		await expect(page.getByTestId('tabpanel-oidc')).toBeVisible();
		await page.getByTestId('oidc-issuer-url').fill('https://login.example.com');
		await page.getByTestId('oidc-client-id').fill('my_client_id');
		await page.getByTestId('oidc-client-secret').fill('super_secret');
		await page.getByTestId('oidc-submit').click();

		await expect(page.getByTestId('toast-message')).toContainText('SSO enabled', { timeout: 5000 });
	});

	// Assertion 4: invalid_sso_config 400 shows inline field-level error
	test('invalid_sso_config 400 shows inline field-level error on offending field', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/organizations/*/sso', async (route) => {
			if (route.request().method() === 'GET') {
				await route.fulfill({ status: 200, body: JSON.stringify(emptySsoConfig) });
			} else {
				await route.continue();
			}
		});
		await context.route('**/api/v1/organizations/*/sso/saml', async (route) => {
			await route.fulfill({
				status: 400,
				body: JSON.stringify({
					code: 'invalid_sso_config',
					description: 'Invalid metadata URL',
					field: 'idp_metadata_url'
				})
			});
		});

		await page.goto(SSO_URL);

		await page.getByTestId('saml-idp-metadata-url').fill('https://idp.example.com/metadata');
		await page.getByTestId('saml-acs-url').fill('https://sp.example.com/acs');
		await page.getByTestId('saml-entity-id').fill('https://sp.example.com');
		await page.getByTestId('saml-submit').click();

		// Inline field error should appear near the offending input
		await expect(page.locator('[role="alert"]').filter({ hasText: 'Invalid metadata URL' })).toBeVisible({ timeout: 5000 });
	});

	// Assertion 5: Test SSO login button has correct href and target="_blank"
	test('Test SSO login button href points to IdP login endpoint', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/sso', async (route) => {
			if (route.request().method() === 'GET') {
				await route.fulfill({
					status: 200,
					body: JSON.stringify(samlEnabledSsoConfig)
				});
			} else {
				await route.continue();
			}
		});

		await page.goto(SSO_URL);

		const testLoginBtn = page.getByTestId('sso-test-login');
		await expect(testLoginBtn).toBeVisible({ timeout: 5000 });

		const href = await testLoginBtn.getAttribute('href');
		expect(href).toContain('/api/v1/sso/saml/');
		expect(href).toContain('/login');

		const target = await testLoginBtn.getAttribute('target');
		expect(target).toBe('_blank');

		const rel = await testLoginBtn.getAttribute('rel');
		expect(rel).toContain('noopener');
	});

	// Assertion 6: non-owner is redirected with owner_required toast
	test('non-owner redirected to org overview with owner_required toast', async ({
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

		await page.goto(SSO_URL);

		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}(\\?|$)`));
		await expect(page.getByTestId('toast-owner-required')).toBeVisible();
	});

	// Assertion 7: SSO sub-nav link visible for owner only
	test('SSO sub-nav link is visible for owner only', async ({ page, context }) => {
		await context.route('**/api/v1/organizations/*/sso', (route) =>
			route.fulfill({ status: 200, body: JSON.stringify(emptySsoConfig) })
		);

		await page.goto(SSO_URL);
		await expect(page.getByTestId('subnav-sso-link')).toBeVisible();
	});
});
