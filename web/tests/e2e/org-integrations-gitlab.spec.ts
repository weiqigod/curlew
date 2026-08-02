// Playwright E2E specs for /org/[slug]/integrations/gitlab (M16-016).
// All backend calls mocked via context.route() — no real backend required.
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL } from './helpers/fixtures';

const INST_ID_1 = 'aaaaaaaa-1111-2222-3333-444444444444';

/** Baseline team-tier org stub. */
const TEAM_ORG = {
	id: 'org_test',
	slug: SEEDED_ORG_SLUG,
	name: 'Acme',
	role: 'owner',
	seat_count: 1,
	seat_limit: 25,
	status: 'active',
	created_at: '2026-01-01T00:00:00Z',
	tier: 'team'
};

/** Returns a minimal GitLab installation object. */
function makeInstallation(overrides: Record<string, unknown> = {}) {
	return {
		id: INST_ID_1,
		project_id: 42,
		project_path: 'group/project',
		gitlab_base_url: 'https://gitlab.com',
		created_at: '2026-05-10T10:00:00Z',
		access_token_revoked_at: null,
		last_status_post_at: null,
		...overrides
	};
}

test.describe('integrations/gitlab', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL);

		// Default organizations stub — team tier owner.
		await context.route(`**/api/v1/users/me/organizations`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ organizations: [TEAM_ORG] })
			})
		);
	});

	test('non-team-tier org redirects with team_tier_required toast', async ({ page, context }) => {
		await context.route(`**/api/v1/users/me/organizations`, (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({
					organizations: [{ ...TEAM_ORG, tier: 'professional' }]
				})
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await expect(page).toHaveURL(
			new RegExp(`/org/${SEEDED_ORG_SLUG}\\?toast=team_tier_required`)
		);
		await expect(page.getByTestId('toast-team-tier-required')).toBeVisible();
	});

	test('team-tier empty list shows empty-state with Connect button', async ({ page, context }) => {
		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [] })
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await expect(page.getByTestId('gitlab-integration-page')).toBeVisible();
		await expect(page.getByTestId('gitlab-empty-state')).toBeVisible();
		await expect(page.getByTestId('connect-gitlab-button')).toBeVisible();
	});

	test('submitting valid PAT + path creates a row and the table updates', async ({
		page,
		context
	}) => {
		const newInst = makeInstallation({ project_path: 'new/project' });

		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [] })
				});
			} else if (route.request().method() === 'POST') {
				route.fulfill({
					status: 201,
					contentType: 'application/json',
					body: JSON.stringify(newInst)
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await page.getByTestId('connect-gitlab-button').click();

		await expect(page.getByTestId('gitlab-modal')).toBeVisible();
		await page.getByTestId('gitlab-modal-path').fill('new/project');
		await page.getByTestId('gitlab-modal-pat').fill('glpat-testtoken');
		await page.getByTestId('gitlab-modal-submit').click();

		// Modal should close and table should appear.
		await expect(page.getByTestId('gitlab-modal')).not.toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('gitlab-integrations-table')).toBeVisible();
	});

	test('submitting an unknown project path shows inline "Project not found" error', async ({
		page,
		context
	}) => {
		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [] })
				});
			} else if (route.request().method() === 'POST') {
				route.fulfill({
					status: 400,
					contentType: 'application/json',
					body: JSON.stringify({
						code: 'project_not_found',
						description: 'Project not found or PAT lacks access.'
					})
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await page.getByTestId('connect-gitlab-button').click();
		await page.getByTestId('gitlab-modal-path').fill('group/missing');
		await page.getByTestId('gitlab-modal-pat').fill('glpat-testtoken');
		await page.getByTestId('gitlab-modal-submit').click();

		// Inline field error should appear.
		await expect(page.getByTestId('gitlab-modal-error-project_path')).toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('gitlab-modal-error-project_path')).toContainText(/not found/i);
	});

	test('self-managed base URL with CA bundle persists both fields', async ({ page, context }) => {
		const selfManagedInst = makeInstallation({
			gitlab_base_url: 'https://gitlab.example.com',
			project_path: 'corp/project'
		});

		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [] })
				});
			} else if (route.request().method() === 'POST') {
				// Verify request contains both fields (postDataJSON() is synchronous).
				const body = JSON.parse(route.request().postData() ?? '{}');
				expect(body.gitlab_base_url).toBe('https://gitlab.example.com');
				expect(body.gitlab_ca_bundle).toBeTruthy();
				route.fulfill({
					status: 201,
					contentType: 'application/json',
					body: JSON.stringify(selfManagedInst)
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await page.getByTestId('connect-gitlab-button').click();

		// Fill in self-managed URL.
		await page.getByTestId('gitlab-modal-base-url').clear();
		await page.getByTestId('gitlab-modal-base-url').fill('https://gitlab.example.com');
		await page.getByTestId('gitlab-modal-path').fill('corp/project');
		await page.getByTestId('gitlab-modal-pat').fill('glpat-selfmanaged');

		// Expand CA bundle.
		await page.getByText('Advanced: Custom CA Bundle').click();
		await page.getByTestId('gitlab-modal-ca').fill('-----BEGIN CERTIFICATE-----\nMIIFake\n-----END CERTIFICATE-----');

		await page.getByTestId('gitlab-modal-submit').click();
		await expect(page.getByTestId('gitlab-modal')).not.toBeVisible({ timeout: 5000 });
	});

	test('installations with access_token_revoked_at show the token-revoked badge and Re-paste action', async ({
		page,
		context
	}) => {
		const revokedInst = makeInstallation({
			id: INST_ID_1,
			access_token_revoked_at: '2026-05-09T12:00:00Z'
		});

		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [revokedInst] })
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await expect(page.getByTestId('gitlab-token-revoked-badge')).toBeVisible();
		await expect(page.getByTestId('gitlab-repaste-pat-button')).toBeVisible();
	});

	test('Disconnect removes the row from the table', async ({ page, context }) => {
		const inst = makeInstallation({ id: INST_ID_1 });

		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [inst] })
				});
			} else {
				route.continue();
			}
		});

		await context.route(`**/api/v1/integrations/gitlab/${INST_ID_1}`, (route) => {
			if (route.request().method() === 'DELETE') {
				route.fulfill({ status: 204 });
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await expect(page.getByTestId(`gitlab-row-${INST_ID_1}`)).toBeVisible();

		await page.getByTestId('disconnect-gitlab-button').first().click();
		await expect(page.getByTestId('disconnect-confirm-button')).toBeVisible();
		await page.getByTestId('disconnect-confirm-button').click();

		// Row should disappear.
		await expect(page.getByTestId(`gitlab-row-${INST_ID_1}`)).not.toBeVisible({ timeout: 5000 });
	});

	test('most-recent last_status_post is shown as relative time', async ({ page, context }) => {
		const recentPost = new Date(Date.now() - 2 * 60 * 1000).toISOString(); // 2 min ago
		const inst = makeInstallation({ id: INST_ID_1, last_status_post_at: recentPost });

		await context.route(`**/api/v1/integrations/gitlab`, (route) => {
			if (route.request().method() === 'GET') {
				route.fulfill({
					status: 200,
					contentType: 'application/json',
					body: JSON.stringify({ installations: [inst] })
				});
			} else {
				route.continue();
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/gitlab`);
		await expect(page.getByTestId(`gitlab-row-${INST_ID_1}`)).toBeVisible();
		// Should show relative time (not "—")
		await expect(page.getByTestId(`gitlab-row-${INST_ID_1}`)).toContainText(/ago/);
	});
});
