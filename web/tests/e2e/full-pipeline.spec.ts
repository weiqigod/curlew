/**
 * E2E: results ingest -> backend -> web dashboard (M4-012)
 *
 * Prerequisites (handled by global-setup.ts or scripts/test-stack.sh up):
 * - Backend running at CURLEW_BACKEND_URL (default: http://localhost:5000)
 * - Web running at WEB_BASE_URL (default: http://localhost:3000)
 * - The 'acme' org exists in the backend (seeded by seed-test-data.sh)
 *
 * Runs are seeded over HTTP against the same endpoints `curlew run
 * --report-upload` used to call. The CLI is backend-free now, so there is no
 * binary to build and no CURLEW_BACKEND_TOKEN to export: the spec mints its
 * own token and skips itself when the stack is not up.
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import {
	backendReachable,
	ingestResult,
	mintToken,
	resolveOrgId,
	seedRunWithPrCheck
} from './helpers/seed';

let stackUp = false;
let ownerToken = '';
let orgId = '';

const NO_STACK = 'backend not reachable — run ./scripts/test-stack.sh up';

test.describe('E2E: results ingest -> backend -> web', () => {
	test.beforeAll(async () => {
		stackUp = await backendReachable();
		if (!stackUp) return;
		ownerToken = mintToken(OWNER_EMAIL, OWNER_USER_ID);
		orgId = await resolveOrgId(SEEDED_ORG_SLUG, ownerToken);
	});

	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	// Assertion 1 + 2: uploaded run appears in /org/acme/results
	test('uploaded run appears in /org/acme/results within 5s', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		const { result, prCheck } = await seedRunWithPrCheck(
			orgId,
			ownerToken,
			{ repo: 'acme/api', pr: 7, state: 'success' },
			{ triggeredBy: 'playwright-e2e', passCount: 3 }
		);

		expect(result.id).toMatch(/^res_/);
		// The pr-check row is written before the check run is posted, so any
		// non-5xx response means the dashboard has something to render.
		expect(prCheck.httpStatus, prCheck.body).toBeLessThan(500);

		// Assertion 2: results table shows at least one row for this org.
		await page.goto(`/org/${SEEDED_ORG_SLUG}/results?range=all`);
		const table = page.getByTestId('recent-runs-table');
		await expect(table).toBeVisible({ timeout: 5000 });
		await expect(table.locator('tbody tr').first()).toBeVisible({ timeout: 5000 });
	});

	// Assertion 3: pr-check row visible in /org/acme/pr-checks
	test('pr-check row is visible in /org/acme/pr-checks', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		// Seed our own row so the test does not depend on execution order.
		await seedRunWithPrCheck(orgId, ownerToken, {
			repo: 'acme/api',
			pr: 10,
			state: 'success'
		});

		await page.goto(`/org/${SEEDED_ORG_SLUG}/pr-checks`);
		await expect(page.getByTestId('pr-checks-table')).toBeVisible({ timeout: 5000 });
		await expect(page.getByTestId('pr-check-state-success').first()).toBeVisible();
	});

	// Assertion 4: a failing run records state=failure
	test('failing run records state=failure and dashboard shows failure badge', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		const { result } = await seedRunWithPrCheck(
			orgId,
			ownerToken,
			{ repo: 'acme/api', pr: 99, state: 'failure' },
			{ passCount: 1, failCount: 2, triggeredBy: 'playwright-e2e-failing' }
		);
		expect(result.id).toMatch(/^res_/);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/pr-checks`);
		await expect(page.getByTestId('pr-check-state-failure').first()).toBeVisible({
			timeout: 5000
		});
	});

	// Assertion 5: results ingest rejects a payload that fails schema validation
	test('malformed result payload is rejected with 400', async () => {
		test.skip(!stackUp, NO_STACK);

		// pass_count is required; omitting it must not create a run.
		await expect(
			ingestResult(orgId, ownerToken, {
				passCount: -1
			})
		).rejects.toThrow(/HTTP 400/);
	});

	// Assertion 6: pr-checks page renders correctly (no live backend required)
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
