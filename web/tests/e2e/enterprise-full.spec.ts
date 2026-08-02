/**
 * E2E M5: SSO login → audit capture → dashboard with custom role (M5-020)
 *
 * Prerequisites (handled by global-setup.ts or scripts/test-stack.sh up):
 * - Backend running at http://localhost:5000
 * - Web running at http://localhost:3000
 * - Fake IdP running at http://localhost:8088
 * - The 'acme' org seeded via seed-test-data.sh + seed-enterprise.sh:
 *     ./scripts/seed-enterprise.sh acme qa-lead "results.upload,results.view,dashboard.view"
 * - The apitest binary built at ./apitest (go build ./cmd/apitest)
 * - APITEST_BACKEND_TOKEN env set to a qa@acme.example JWT
 *
 * This spec exercises 7 assertions: 6 happy-path + 1 failure-path (bogus SAML).
 */
import { test, expect } from '@playwright/test';
import { execSync } from 'node:child_process';
import path from 'path';
import { seedAuthCookie } from './helpers/auth';
import { runApitest } from './helpers/cli';
import { lookupOrgGuid, triggerSamlLogin } from './helpers/saml';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../..');
const ORG = 'acme';
const BACKEND_URL = process.env.BACKEND_URL ?? 'http://localhost:5000';
const QA_USER_ID = '00000000-0000-0000-0000-000000000002';

test.describe('E2E M5: SSO → audit → dashboard with custom role', () => {
	// ── Assertion 1: SSO login via fake-idp sets session cookie and lands on /org/acme ─

	test('SSO login via fake-idp sets session cookie and lands on /org/acme', async ({
		page,
		context
	}) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping SSO E2E');

		await triggerSamlLogin(page, ORG);

		await expect(page).toHaveURL(new RegExp(`/org/${ORG}(\\?|$)`));
		const cookies = await context.cookies();
		const sessionCookie = cookies.find((c) => c.name === 'access_token');
		expect(sessionCookie?.value).toBeTruthy();
	});

	// ── Assertion 2: audit log shows sso.login row within 5s ─────────────────────────

	test('audit log shows sso.login row within 5s', async ({ page, context }) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping SSO E2E');

		await triggerSamlLogin(page, ORG);
		// The SSO user intentionally has a narrow custom role without audit-log access.
		// Switch to the owner session before asserting the organization-wide audit row.
		await seedAuthCookie(context, 'owner@example.com');

		await page.goto(`/org/${ORG}/audit-log`);
		const ssoLoginRow = page
			.getByTestId('audit-log-table')
			.locator('tbody tr')
			.filter({ hasText: 'sso.login' })
			.first();

		await expect(ssoLoginRow).toBeVisible({ timeout: 5000 });

		// Behavior 2: the row must display the actor's email (qa@acme.example)
		await expect(ssoLoginRow).toContainText('qa@acme.example');

		// Failure-path sub-assertion: the row must show success (no failure_reason text)
		await expect(ssoLoginRow).not.toContainText('failure');
	});

	// ── Assertion 3: CLI upload as qa-lead custom role succeeds ───────────────────────

	test('CLI upload as qa-lead custom role succeeds and stdout shows result id', async () => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping CLI E2E');

		const tokenScript = path.join(REPO_ROOT, 'scripts', 'test-token.sh');
		const qaToken = execSync(`bash ${tokenScript} qa@acme.example ${QA_USER_ID}`, {
			encoding: 'utf8'
		}).trim();

		const result = runApitest({
			collection: 'testdata/enterprise/e2e-collection.yaml',
			flags: ['--report-upload', '--org', ORG],
			expectExit: 0,
			env: {
				APITEST_BACKEND_URL: BACKEND_URL,
				APITEST_BACKEND_TOKEN: qaToken
			}
		});

		expect(result.stdout).toMatch(/Uploaded result res_/);
	});

	// ── Assertion 4: audit log shows results.upload row for the CLI run ───────────────

	test('audit log shows results.upload row for the CLI run', async ({ page, context }) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping CLI E2E');

		// Seed auth as owner so we can view the audit log
		await seedAuthCookie(context, 'owner@example.com');

		// Upload a result as the qa user first so there's definitely a row
		const tokenScript = path.join(REPO_ROOT, 'scripts', 'test-token.sh');
		const qaToken = execSync(`bash ${tokenScript} qa@acme.example ${QA_USER_ID}`, {
			encoding: 'utf8'
		}).trim();

		runApitest({
			collection: 'testdata/enterprise/e2e-collection.yaml',
			flags: ['--report-upload', '--org', ORG],
			expectExit: 0,
			env: {
				APITEST_BACKEND_URL: BACKEND_URL,
				APITEST_BACKEND_TOKEN: qaToken
			}
		});

		await page.goto(`/org/${ORG}/audit-log`);

		const uploadRow = page
			.getByTestId('audit-log-table')
			.locator('tbody tr')
			.filter({ hasText: 'results.upload' })
			.first();

		await expect(uploadRow).toBeVisible({ timeout: 5000 });
		// Behavior 4: verify the actor display name matches the qa service token user
		await expect(uploadRow).toContainText('qa@acme.example');
	});

	// ── Assertion 5: uploaded run appears in /org/acme/results within 5s ─────────────

	test('uploaded run appears in /org/acme/results within 5s', async ({ page, context }) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping CLI E2E');

		await seedAuthCookie(context, 'owner@example.com');

		await page.goto(`/org/${ORG}/results?range=all`);
		await expect(page.getByTestId('recent-runs-table').locator('tbody tr').first()).toBeVisible({
			timeout: 5000
		});
	});

	// ── Assertion 6: qa-lead custom role row shows member_count=1 and is_builtin=false ─

	test('qa-lead custom role row shows member_count=1 and is_builtin=false', async ({
		page,
		context
	}) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping E2E');

		await seedAuthCookie(context, 'owner@example.com');

		await page.goto(`/org/${ORG}/settings/roles`);
		// Match only custom role rows (data-testid="role-row-role_<hex>", never "role-row-builtin_*")
		// and filter to the qa-lead row specifically.
		const row = page.getByTestId(/^role-row-role_/).filter({ hasText: 'qa-lead' });
		await expect(row.first()).toBeVisible({ timeout: 5000 });
		await expect(row.first()).toContainText('1'); // member count
		// Behavior 6: verify is_builtin=false — custom role rows must not display the "Built-in" badge
		await expect(row.first()).not.toContainText('Built-in');
	});

	// ── Assertion 7 (failure-path): bogus SAMLResponse is rejected with 401 ──────────

	test('bogus SAMLResponse is rejected with 401 and audit-log records success=false', async ({
		page,
		context
	}) => {
		test.skip(!process.env.APITEST_BACKEND_TOKEN, 'Live stack not available — skipping SSO E2E');

		const orgGuid = await lookupOrgGuid(ORG);
		const acsUrl = `${BACKEND_URL}/api/v1/sso/saml/${orgGuid}/acs`;

		// POST a bogus SAMLResponse directly — must be rejected
		const resp = await page.request.post(acsUrl, {
			form: { SAMLResponse: 'bogus-invalid-base64' }
		});
		expect(resp.status()).toBe(401);

		// Check the audit log for a failed sso.login row
		await seedAuthCookie(context, 'owner@example.com');
		await page.goto(`/org/${ORG}/audit-log`);

		const failRow = page
			.getByTestId('audit-log-table')
			.locator('tbody tr')
			.filter({ hasText: 'sso.login' })
			.first();

		// The row should exist (the backend creates an audit row on failed login)
		// and must indicate failure via text content (success=false in the rendered row).
		await expect(failRow).toBeVisible({ timeout: 5000 });
		await expect(failRow).toContainText('failure');
	});
});
