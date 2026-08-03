/**
 * M14-021: Revenue loop convergence E2E spec.
 *
 * Proves the full happy path:
 *   results ingest + pr-check upload →
 *   backend persists result + posts check-run to github-mock →
 *   Stripe invoice.payment_succeeded → billing_receipt email queued.
 *
 * Prerequisites (handled by ci-local.sh --full or manual test-stack.sh up):
 *   - Backend:    http://localhost:5000 (override with CURLEW_BACKEND_URL)
 *   - Web:        http://localhost:3000
 *   - github-mock sidecar (docker-compose service "github-mock")
 *   - stripe-mock: http://localhost:12111
 *   - M14 state seeded by seed-test-data.sh (team tier, github_installations row)
 *
 * The run is seeded over HTTP. `curlew run --report-upload` used to drive this
 * and its stdout carried the "check-run posted" proof; that proof now comes
 * from the pr-check upload response, which is where the CLI read it from.
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import {
	backendReachable,
	mintToken,
	resolveOrgId,
	seedM14,
	seedRunWithPrCheck,
	stripeMockCustomerId,
	type SeededPrCheck,
	type SeededResult
} from './helpers/seed';
import { replayStripeEvent, waitForEmailAuditEntry } from './helpers/m14-seed';
import { execSync } from 'child_process';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../..');

const NO_STACK = 'backend not reachable — run ./scripts/test-stack.sh up';

// ── State shared across tests in beforeAll ────────────────────────────────
let stackUp = false;
let seededResult: SeededResult | null = null;
let seededPrCheck: SeededPrCheck | null = null;
let seedError = '';
// ─────────────────────────────────────────────────────────────────────────

test.describe('M14 revenue loop', () => {
	test.beforeAll(async () => {
		stackUp = await backendReachable();
		if (!stackUp) return;

		try {
			const ownerToken = mintToken(OWNER_EMAIL, OWNER_USER_ID);
			const orgId = await resolveOrgId(SEEDED_ORG_SLUG, ownerToken);
			// Point the org at the customer stripe-mock puts on its invoices, so
			// the replayed webhook in assertion 3 resolves back to this org.
			await seedM14(orgId, { stripeCustomerId: await stripeMockCustomerId() });
			const seeded = await seedRunWithPrCheck(
				orgId,
				ownerToken,
				{ repo: 'acme/api', pr: 7, state: 'success', summary: 'all checks passed' },
				{ collectionName: 'm14-e2e', triggeredBy: 'm14-playwright', passCount: 2 }
			);
			seededResult = seeded.result;
			seededPrCheck = seeded.prCheck;
		} catch (err: unknown) {
			// Capture but don't rethrow — the tests assert on the captured values.
			seedError = err instanceof Error ? err.message : String(err);
		}
	});

	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	// Assertion 5: the pr-check upload posted a check run to github-mock
	test('5. pr-check upload reports a posted check run', () => {
		test.skip(!stackUp, NO_STACK);

		expect(seedError, 'seeding must succeed').toBe('');
		expect(seededResult?.id, 'result id should be present').toMatch(/^res_/);
		expect(seededPrCheck?.state, seededPrCheck?.body ?? 'no response').toBe('posted');
		expect(
			seededPrCheck?.githubCheckRunId,
			'a posted check run carries the GitHub check_run id'
		).toBeTruthy();
	});

	// Assertion 1: dashboard /org/acme/runs shows the new run (via 307 redirect to /results)
	test('1. /org/acme/runs redirects and shows the new run', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/runs`);
		// The 307 redirect lands us on /results — check the table is present.
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}/results`), { timeout: 5000 });
		const table = page.getByTestId('recent-runs-table');
		await expect(table).toBeVisible({ timeout: 5000 });
		await expect(table.locator('tbody tr').first()).toBeVisible({ timeout: 5000 });
	});

	// Assertion 2: /org/acme/integrations/github shows posted_at within 5s of upload
	test('2. /org/acme/integrations/github shows posted_at within 5s of upload', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/github`);
		await expect(page.getByTestId('github-integration-page')).toBeVisible({ timeout: 5000 });
		// The last pr-check should have been posted (check_run_id non-null).
		await expect(page.getByTestId('check-run-id')).toBeVisible({ timeout: 5000 });
		// posted_at should be present (the check-run was posted synchronously).
		await expect(page.getByTestId('check-posted-at')).toBeVisible({ timeout: 5000 });
	});

	// Assertion 3: Stripe replay enqueues billing_receipt; email-audit lists it
	test('3. Stripe invoice.payment_succeeded enqueues billing_receipt email', async () => {
		test.skip(!stackUp, NO_STACK);

		// Replay the invoice.payment_succeeded event using a unique object id.
		const invoiceId = `inv_m14_e2e_${Date.now()}`;
		replayStripeEvent('invoice.payment_succeeded', invoiceId);

		// Wait for the email-audit endpoint to show a billing_receipt row.
		await waitForEmailAuditEntry('billing_receipt', 5000);
	});

	// Assertion 4: github-mock recorded POST /repos/acme/api/check-runs
	test('4. github-mock recorded POST /repos/acme/api/check-runs', async () => {
		test.skip(!stackUp, NO_STACK);

		// The mock logs each request to stdout; docker compose is the only way to
		// read it back (it exposes no log endpoint).
		let logsOutput = '';
		try {
			logsOutput = execSync('docker compose -f docker-compose.test.yml logs github-mock', {
				encoding: 'utf8',
				cwd: REPO_ROOT
			});
		} catch {
			test.skip(true, 'docker compose logs not available');
			return;
		}

		expect(logsOutput).toContain('/repos/acme/api/check-runs');
	});
});
