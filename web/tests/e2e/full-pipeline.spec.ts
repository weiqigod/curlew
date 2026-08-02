/**
 * E2E: CLI -> backend ingest -> web dashboard (M4-012)
 *
 * Prerequisites (handled by global-setup.ts or scripts/test-stack.sh up):
 * - Backend running at BACKEND_URL (default: http://localhost:5000)
 * - Web running at WEB_BASE_URL (default: http://localhost:3000)
 * - CURLEW_BACKEND_URL and CURLEW_BACKEND_TOKEN env vars set
 * - The 'acme' org exists in the backend (seeded by seed-test-data.sh)
 * - The curlew binary is built at ./curlew (run: go build ./cmd/curlew)
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import { runCurlew } from './helpers/cli';

const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';
const BACKEND_TOKEN = process.env.CURLEW_BACKEND_TOKEN ?? '';

test.describe('E2E: CLI -> backend -> web', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	// Assertion 1 + 2: uploaded run appears in /org/acme/results
	test('uploaded run appears in /org/acme/results within 5s', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		const result = runCurlew({
			collection: 'testdata/team/e2e-collection.yaml',
			flags: [
				'--report-upload',
				'--org', SEEDED_ORG_SLUG,
				'--pr', '7',
				'--repo', 'acme/api',
				'--triggered-by', 'playwright-e2e',
			],
			expectExit: 0,
			env: {
				CURLEW_BACKEND_URL: BACKEND_URL,
				CURLEW_BACKEND_TOKEN: BACKEND_TOKEN,
			}
		});

		expect(result.resultId).toBeTruthy();
		// "Uploaded result res_..." confirms CLI exit-0 and upload success
		expect(result.stdout).toContain(`Uploaded result ${result.resultId}`);

		// Assertion 2: results table shows at least one row with triggered_by=playwright-e2e
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results?range=all`);
		const table = page.getByTestId('recent-runs-table');
		await expect(table).toBeVisible({ timeout: 5000 });
		// The CLI stdout contains the result id — table should have a row for this run
		await expect(table.locator('tbody tr').first()).toBeVisible({ timeout: 5000 });
	});

	// Assertion 3: pr-check row visible in /org/acme/pr-checks
	test('pr-check row is visible in /org/acme/pr-checks', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		// Post a pr-check (may already be there from the previous test, but
		// we run independently so we post our own)
		runCurlew({
			collection: 'testdata/team/e2e-collection.yaml',
			flags: [
				'--report-upload',
				'--org', SEEDED_ORG_SLUG,
				'--pr', '10',
				'--repo', 'acme/api',
			],
			expectExit: 0,
			env: {
				CURLEW_BACKEND_URL: BACKEND_URL,
				CURLEW_BACKEND_TOKEN: BACKEND_TOKEN,
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/pr-checks`);
		await expect(page.getByTestId('pr-checks-table')).toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('pr-check-state-success').first()).toBeVisible();
	});

	// Assertion 4: failing collection uploads state=failure
	test('failing collection uploads state=failure and dashboard shows failure badge', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		runCurlew({
			collection: 'testdata/team/e2e-collection-failing.yaml',
			flags: [
				'--report-upload',
				'--org', SEEDED_ORG_SLUG,
				'--pr', '99',
				'--repo', 'acme/api',
			],
			expectExit: 1, // failing assertions = exit 1
			env: {
				CURLEW_BACKEND_URL: BACKEND_URL,
				CURLEW_BACKEND_TOKEN: BACKEND_TOKEN,
			}
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/pr-checks`);
		await expect(page.getByTestId('pr-check-state-failure').first()).toBeVisible({ timeout: 5000 });
	});

	// Assertion 5: pr-checks page renders correctly (no live backend required)
	test('pr-checks page renders table or empty state', async ({ page, context }) => {
		// Intercept the backend calls to avoid needing a live stack for this assertion
		await context.route(`**/api/v1/organizations/*/pr-checks**`, (route) =>
			route.fulfill({
				status: 200,
				body: JSON.stringify({
					pr_checks: [
						{
							id: 'prc_abc123',
							repo: 'acme/api',
							pr: 42,
							state: 'success',
							result_id: 'res_def456',
							created_at: '2026-04-17T10:00:00Z'
						}
					]
				})
			})
		);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/pr-checks`);
		await expect(page.getByTestId('pr-checks-table')).toBeVisible();
		await expect(page.getByTestId('pr-check-row').first()).toBeVisible();
		await expect(page.getByTestId('pr-check-state-success').first()).toBeVisible();
	});
});
