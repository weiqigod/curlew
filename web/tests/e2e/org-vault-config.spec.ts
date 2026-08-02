// Playwright E2E specs for /org/[slug]/vault-config (M16-017).
// All backend calls mocked via context.route() — no real backend required.
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL } from './helpers/fixtures';

const SPEC_EXAMPLE_YAML =
	'team_secrets:\n  provider: aws-secrets-manager\n  keys:\n    api_key: prod/api-key\n';

/** Baseline team-tier org stub. */
const TEAM_ORG = {
	id: 'org_vaulttest',
	slug: SEEDED_ORG_SLUG,
	name: 'Acme',
	role: 'owner',
	seat_count: 1,
	seat_limit: 25,
	status: 'active',
	created_at: '2026-01-01T00:00:00Z',
	tier: 'team',
};

const VAULT_CONFIG_RESPONSE = {
	template: SPEC_EXAMPLE_YAML,
	version: 1,
	updated_at: '2026-05-12T10:00:00Z',
	updated_by_email: 'owner@test.com',
	warnings: null,
};

test.describe('vault-config', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);

		// Default: team-tier org
		await context.route(`**/api/v1/users/me/organizations`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ organizations: [TEAM_ORG] }),
			})
		);
	});

	test('team-tier admin sees editor with placeholder when no row yet', async ({
		page,
		context,
	}) => {
		// GET returns 404
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
			} else {
				route.continue();
			}
		});
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ items: [] }),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await expect(page.locator('textarea[data-testid="vault-editor"]')).toBeVisible();
	});

	test('saving valid YAML shows success toast with version', async ({ page, context }) => {
		// GET returns 404 initially
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) => {
			const method = route.request().method();
			if (method === 'GET') {
				route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
			} else if (method === 'PUT') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify(VAULT_CONFIG_RESPONSE),
				});
			} else {
				route.continue();
			}
		});
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ items: [] }),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await page.locator('textarea[data-testid="vault-editor"]').fill(SPEC_EXAMPLE_YAML);
		await page.getByTestId('vault-save-button').click();
		await expect(page.getByText('Updated to version 1')).toBeVisible();
	});

	test('Generate CLI snippet button shows curlew license --refresh command', async ({
		page,
		context,
	}) => {
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(VAULT_CONFIG_RESPONSE),
			})
		);
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ items: [] }),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await page.getByTestId('vault-cli-snippet-button').click();
		await expect(page.getByText('curlew license --refresh')).toBeVisible();
	});

	test('non-team-tier org redirects with team_tier_required toast', async ({
		page,
		context,
	}) => {
		await context.route(`**/api/v1/users/me/organizations`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					organizations: [{ ...TEAM_ORG, tier: 'professional' }],
				}),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await expect(page).toHaveURL(
			new RegExp(`/org/${SEEDED_ORG_SLUG}\\?toast=team_tier_required`)
		);
	});

	test('production-mode 422 response shows offending paths', async ({ page, context }) => {
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) => {
			const method = route.request().method();
			if (method === 'GET') {
				route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
			} else if (method === 'PUT') {
				route.fulfill({
					status: 422,
					contentType: 'application/problem+json',
					body: JSON.stringify({
						type: 'https://api.apitool.dev/errors/vault-template-suspicious-value',
						title: 'Template contains likely-secret values',
						status: 422,
						offending_paths: ['team_secrets.password'],
					}),
				});
			} else {
				route.continue();
			}
		});
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ items: [] }),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await page.locator('textarea[data-testid="vault-editor"]').fill(
			'team_secrets:\n  password: abcdefghij1234567890\n'
		);
		await page.getByTestId('vault-save-button').click();
		await expect(page.getByTestId('vault-suspicious-warning')).toBeVisible();
		await expect(page.getByText('team_secrets.password')).toBeVisible();
	});

	test('Delete clears the editor and shows empty-state', async ({ page, context }) => {
		let deleted = false;
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) => {
			const method = route.request().method();
			if (method === 'GET') {
				if (deleted) {
					route.fulfill({ status: 404, contentType: 'application/json', body: '{}' });
				} else {
					route.fulfill({
						status: 200,
						contentType: 'application/json',
						body: JSON.stringify(VAULT_CONFIG_RESPONSE),
					});
				}
			} else if (method === 'DELETE') {
				deleted = true;
				route.fulfill({ status: 204 });
			} else {
				route.continue();
			}
		});
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ items: [] }),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await page.getByTestId('vault-delete-button').click();
		// Confirm dialog
		await page.getByTestId('vault-delete-confirm').click();
		// After delete, editor should be empty
		const editor = page.locator('textarea[data-testid="vault-editor"]');
		await expect(editor).toHaveValue('');
	});

	test('audit log subsection shows vault_config entries', async ({ page, context }) => {
		await context.route(`**/api/v1/organizations/*/vault-config`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(VAULT_CONFIG_RESPONSE),
			})
		);
		await context.route(`**/api/v1/organizations/*/audit-log**`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					items: [
						{
							event_type: 'vault_config.upserted',
							user_id: 'user-123',
							user_email: 'owner@test.com',
							target_type: null,
							target_id: null,
							created_at: '2026-05-12T10:00:00Z',
							ip_address: null,
							success: true,
							failure_reason: null,
						},
					],
				}),
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/vault-config`);
		await expect(page.getByTestId('vault-audit-log')).toBeVisible();
		await expect(page.getByText('vault_config.upserted')).toBeVisible();
	});
});
