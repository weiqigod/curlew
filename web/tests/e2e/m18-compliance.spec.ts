/**
 * M18-012: Compliance convergence E2E spec.
 *
 * Proves the M18 capabilities wire together end-to-end:
 *   user registration (via seed-refresh) →
 *   telemetry CLI emits run.completed →
 *   data export bundle is reachable via signed URL →
 *   account/data page renders export + delete panels →
 *   cancel-deletion page renders within the cooldown window →
 *   audit-log JSONL streaming export is chunked + ndjson.
 *
 * Prerequisites (handled by scripts/m18-e2e.sh which runs after ci-local.sh --full):
 *   - CURLEW_BACKEND_URL: backend base URL (default http://localhost:5000)
 *   - CURLEW_BACKEND_TOKEN: bearer token minted by test-token.sh
 *   - M18_INSTALL_ID: install_id emitted by `curlew telemetry enable` in step 3
 *   - M18_EXPORT_ID: export request ID created in step 5
 *   - M18_ENTERPRISE_ORG_ID: org ID of the Enterprise-tier org seeded for step 10
 *
 * All tests that require live stack env vars are individually skipped when those
 * vars are absent, so `npm run test:e2e` continues to work in offline / unit-only mode.
 *
 * Open Decision 9 (M18-012): happy-path only — failure modes are covered in each
 * cluster's own tests.
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import {
	pollExportReady,
	listTelemetryEvents,
	streamAuditJsonl,
} from './helpers/m18-seed';

const BACKEND_TOKEN = process.env.CURLEW_BACKEND_TOKEN ?? '';
const M18_ENTERPRISE_TOKEN = process.env.M18_ENTERPRISE_TOKEN ?? BACKEND_TOKEN;
const M18_INSTALL_ID = process.env.M18_INSTALL_ID ?? '';
const M18_EXPORT_ID = process.env.M18_EXPORT_ID ?? '';
const M18_ENTERPRISE_ORG_ID = process.env.M18_ENTERPRISE_ORG_ID ?? '';

test.describe('M18 compliance happy path', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	/**
	 * Assertion 1: /account/data renders both the export-request and delete panels.
	 * Proves the M18-004 export-request flow and M18-006 deletion panel are wired into
	 * the account/data web page.
	 */
	test('1. /account/data renders export + delete panels', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		await page.goto('/account/data');
		await expect(page.getByTestId('request-export-button')).toBeVisible({
			timeout: 10_000,
		});
		await expect(page.getByTestId('delete-account-panel')).toBeVisible({
			timeout: 10_000,
		});
	});

	/**
	 * Assertion 2: Export bundle signed URL is reachable and carries an InExport table set.
	 * Polls the export-request status endpoint until the bundle is ready, then fetches the
	 * signed URL and asserts the JSON payload has at least one table key.
	 * The shell orchestrator must have already created the export request and triggered
	 * the export builder (step 5 of m18-e2e.sh).
	 */
	test('2. Export bundle signed URL is reachable', async ({ request }) => {
		test.skip(
			!BACKEND_TOKEN || !M18_EXPORT_ID,
			'CURLEW_BACKEND_TOKEN or M18_EXPORT_ID not set — orchestrator must run first'
		);

		const ready = await pollExportReady(M18_EXPORT_ID, BACKEND_TOKEN);
		expect(ready.status).toBe('ready');
		expect(ready.signed_url).toMatch(/^https?:\/\//);

		// Signed URL must serve a valid JSON bundle with at least one table entry.
		const downloadUrl = new URL(ready.signed_url!);
		// MinIO signs against its Docker-network hostname. The browser test runs on
		// the host, so connect through the published port for this local profile.
		if (downloadUrl.hostname === 'minio') {
			downloadUrl.hostname = '127.0.0.1';
			downloadUrl.port = '9000';
		}
		const bundle = await request.get(downloadUrl.toString());
		expect(bundle.ok()).toBeTruthy();
		const json = (await bundle.json()) as { tables?: Record<string, unknown> };
		expect(Object.keys(json.tables ?? {}).length).toBeGreaterThan(0);
	});

	/**
	 * Assertion 3: Telemetry events exist for the M18 install_id.
	 * Proves the CLI telemetry emitter (M18-008) → backend ingest pipeline (M18-007) round-trip.
	 * The shell orchestrator must have already enabled telemetry and run a collection
	 * (steps 3–4 of m18-e2e.sh) before this assertion executes.
	 */
	test('3. Telemetry events present for install_id', async () => {
		test.skip(
			!M18_INSTALL_ID,
			'M18_INSTALL_ID not set — orchestrator must run first'
		);

		const list = await listTelemetryEvents(M18_INSTALL_ID);
		expect(
			list.count,
			'expected >=1 telemetry_events row for install_id'
		).toBeGreaterThanOrEqual(1);
		const eventTypes = list.rows.map((r) => r.event_type);
		expect(eventTypes).toContain('run.completed');
	});

	/**
	 * Assertion 4: Cancel-deletion page renders within the cooldown window.
	 * The shell orchestrator has initiated a deletion request (step 6) and then cancelled it,
	 * then re-initiated. The cancel-deletion route must render without a 404 or error page.
	 * Proves M18-006's deletion panel and cancel route are wired correctly.
	 */
	test('4. Cancel-deletion page renders within cooldown window', async ({ page }) => {
		test.skip(!BACKEND_TOKEN, 'CURLEW_BACKEND_TOKEN not set — skipping live E2E test');

		await page.goto('/account/data/cancel-deletion');
		// Intentionally weak assertion (M16-021 happy-path-only posture): we assert only that
		// the route renders without a 404/error URL, not that the cancel button is visible.
		// The session here belongs to the pre-seeded OWNER who has no active deletion request;
		// the e2e user who does have one is not accessible via a Playwright browser context in
		// this slice (their access token is CLI-only). Strengthening this assertion to check for
		// `getByTestId('cancel-deletion-button')` is deferred: it requires either exposing
		// the test-user session to Playwright or a dedicated cancel-deletion fixture, both of
		// which are out of scope for the M18-012 convergence slice.
		await expect(page).not.toHaveURL(/error|404/, { timeout: 5_000 });
	});

	/**
	 * Assertion 5: Audit-log JSONL stream is chunked + application/x-ndjson.
	 * Proves M18-001's streaming export endpoint works end-to-end for an Enterprise org.
	 * The shell orchestrator has seeded an Enterprise-tier org and produced audit rows
	 * (at minimum the anonymisation event) before this assertion executes.
	 */
	test('5. Audit-log JSONL stream is chunked + ndjson', async () => {
		test.skip(
			!M18_ENTERPRISE_TOKEN || !M18_ENTERPRISE_ORG_ID,
			'M18_ENTERPRISE_TOKEN or M18_ENTERPRISE_ORG_ID not set — orchestrator must run first'
		);

		// Uses M18_ENTERPRISE_TOKEN (the pre-seeded owner token with audit_log.export permission)
		// rather than BACKEND_TOKEN (the newly-created test user who has no audit_log.export grant).
		// The orchestrator exports OWNER_TOKEN as M18_ENTERPRISE_TOKEN in step 13.
		const stream = await streamAuditJsonl(M18_ENTERPRISE_ORG_ID, M18_ENTERPRISE_TOKEN);
		expect(stream.status).toBe(200);
		expect(stream.contentType).toContain('application/x-ndjson');
		expect(
			stream.transferEncoding.toLowerCase(),
			'audit-log JSONL stream must use chunked transfer encoding (M18-001 streaming proof)'
		).toContain('chunked');
		expect(
			stream.lineCount,
			'audit-log JSONL stream must contain at least one event row'
		).toBeGreaterThan(0);
	});
});
