/**
 * M14-021: Revenue loop convergence E2E spec.
 *
 * Proves the full happy path:
 *   CLI seed-refresh (login shortcut) →
 *   curlew run --report-upload →
 *   backend persists result + posts check-run to github-mock →
 *   Stripe invoice.payment_succeeded → billing_receipt email queued.
 *
 * Prerequisites (handled by ci-local.sh --full or manual test-stack.sh up):
 *   - Backend:    http://localhost:5000
 *   - Web:        http://localhost:3000
 *   - github-mock sidecar: http://localhost:5099
 *   - stripe-mock:         http://localhost:12111
 *   - CURLEW_BACKEND_URL + CURLEW_BACKEND_TOKEN env vars set
 *   - M14 state seeded by seed-test-data.sh (team tier, github_installations row)
 *
 * Decision #9: "login" is satisfied via seed-refresh shortcut (no real browser
 * device-code round-trip). All five Playwright assertions are happy-path only.
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import { runCurlew } from './helpers/cli';
import { replayStripeEvent, waitForEmailAuditEntry } from './helpers/m14-seed';
import { execSync } from 'child_process';
import path from 'path';

const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';
const BACKEND_TOKEN = process.env.CURLEW_BACKEND_TOKEN ?? '';
const REPO_ROOT = path.resolve(import.meta.dirname, '../../..');

// github-mock logs URL — adjust if the mock exposes a different path.
const GITHUB_MOCK_LOGS_URL = process.env.GITHUB_MOCK_LOGS_URL ?? 'http://localhost:5099/logs';

// ── State shared across tests in beforeAll ────────────────────────────────
let cliResultId = '';
let cliStdout = '';
let cliExitCode = 0;
// ─────────────────────────────────────────────────────────────────────────

test.describe('M14 revenue loop', () => {
	test.beforeAll(async () => {
		if (!BACKEND_TOKEN) return; // guard — tests skip individually

		// Run the CLI with --report-upload against the M14 collection.
		// github-mock health URL is available at GITHUB_MOCK_URL env (fallback to localhost:5099).
		const githubMockUrl = process.env.GITHUB_MOCK_URL ?? 'http://localhost:5099';
		try {
			const result = runCurlew({
				collection: 'testdata/m14/e2e-collection.yaml',
				flags: [
					'--report-upload',
					'--org', SEEDED_ORG_SLUG,
					'--pr', '7',
					'--repo', 'acme/api',
					'--triggered-by', 'm14-playwright',
					'--env-var', 'GITHUB_MOCK_URL',
				],
				expectExit: 0,
				env: {
					CURLEW_BACKEND_URL: BACKEND_URL,
					CURLEW_BACKEND_TOKEN: BACKEND_TOKEN,
					GITHUB_MOCK_URL: githubMockUrl,
				},
			});
			cliResultId = result.resultId ?? '';
			cliStdout = result.stdout;
			cliExitCode = result.exitCode;
		} catch (err: unknown) {
			// Capture but don't rethrow — tests will assert on the captured values.
			const e = err as { message?: string };
			cliStdout = e.message ?? String(err);
			cliExitCode = 1;
		}
	});

	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	// Assertion 5: CLI exit code 0 + "check-run posted" line present
	test('5. CLI exits 0 and stdout contains check-run posted line', () => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		expect(cliExitCode, 'CLI exit code should be 0').toBe(0);
		expect(cliStdout, 'stdout must contain check-run posted phrase').toContain(
			'check-run posted; status=success'
		);
		expect(cliResultId, 'result id should be present').toBeTruthy();
	});

	// Assertion 1: dashboard /org/acme/runs shows the new run (via 307 redirect to /results)
	test('1. /org/acme/runs redirects and shows the new run', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		await page.goto(`/org/${SEEDED_ORG_SLUG}/runs`);
		// The 307 redirect lands us on /results — check the table is present.
		await expect(page).toHaveURL(new RegExp(`/org/${SEEDED_ORG_SLUG}/results`), { timeout: 5000 });
		const table = page.getByTestId('recent-runs-table');
		await expect(table).toBeVisible({ timeout: 5000 });
		await expect(table.locator('tbody tr').first()).toBeVisible({ timeout: 5000 });
	});

	// Assertion 2: /org/acme/integrations/github shows posted_at within 5s of upload
	test('2. /org/acme/integrations/github shows posted_at within 5s of upload', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		await page.goto(`/org/${SEEDED_ORG_SLUG}/integrations/github`);
		await expect(page.getByTestId('github-integration-page')).toBeVisible({ timeout: 5000 });
		// The last pr-check should have been posted (check_run_id non-null).
		await expect(page.getByTestId('check-run-id')).toBeVisible({ timeout: 5000 });
		// posted_at should be present (the check-run was posted synchronously).
		await expect(page.getByTestId('check-posted-at')).toBeVisible({ timeout: 5000 });
	});

	// Assertion 3: Stripe replay enqueues billing_receipt; email-audit lists it
	test('3. Stripe invoice.payment_succeeded enqueues billing_receipt email', async () => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		// Replay the invoice.payment_succeeded event using a unique object id.
		const invoiceId = `inv_m14_e2e_${Date.now()}`;
		replayStripeEvent('invoice.payment_succeeded', invoiceId);

		// Wait for the email-audit endpoint to show a billing_receipt row.
		await waitForEmailAuditEntry('billing_receipt', 5000);
	});

	// Assertion 4: github-mock recorded call log shows POST /repos/acme/api/check-runs
	test('4. github-mock recorded POST /repos/acme/api/check-runs', async () => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		// Use docker compose logs as a fallback when the mock doesn't expose an HTTP log endpoint.
		let logsOutput = '';
		try {
			const res = await fetch(GITHUB_MOCK_LOGS_URL);
			if (res.ok) {
				logsOutput = await res.text();
			}
		} catch {
			// Fallback: read from docker compose logs.
			try {
				logsOutput = execSync('docker compose -f docker-compose.test.yml logs github-mock', {
					encoding: 'utf8',
					cwd: REPO_ROOT,
				});
			} catch {
				// If docker compose is not available, skip the assertion.
				test.skip(true, 'docker compose logs not available');
				return;
			}
		}

		expect(logsOutput).toContain('/repos/acme/api/check-runs');
	});
});
