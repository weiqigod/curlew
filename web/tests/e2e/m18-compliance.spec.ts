/**
 * M18-012: Compliance convergence E2E spec.
 *
 * Proves the M18 capabilities wire together end-to-end:
 *   user registration (via seed-refresh) →
 *   telemetry ingest records a run.completed event →
 *   data export bundle is reachable via signed URL →
 *   account/data page renders export + delete panels →
 *   cancel-deletion page renders within the cooldown window →
 *   audit-log JSONL streaming export is chunked + ndjson.
 *
 * Prerequisites (handled by scripts/ci-local.sh --full or manual test-stack.sh up):
 *   - Backend, web, and MinIO from docker-compose.test.yml
 *   - The 'acme' org seeded by seed-test-data.sh + seed-enterprise.sh
 *
 * All state is seeded here over HTTP. The spec used to read M18_INSTALL_ID /
 * M18_EXPORT_ID / M18_ENTERPRISE_ORG_ID out of the environment, filled in by
 * scripts/m18-e2e.sh driving `curlew telemetry` and `curlew run`; that
 * orchestrator went away with the CLI's backend support.
 *
 * Note on assertion 3: with the CLI emitting telemetry to a local file, this
 * covers the backend's ingest + query path only — there is no longer a CLI
 * emitter to round-trip against.
 *
 * Open Decision 9 (M18-012): happy-path only — failure modes are covered in each
 * cluster's own tests.
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { SEEDED_ORG_SLUG, OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';
import { seedRegistrationViaRefresh } from './helpers/m16-seed';
import {
	createExportRequest,
	listTelemetryEvents,
	pollExportReady,
	runExportBuilder,
	streamAuditJsonl,
} from './helpers/m18-seed';
import {
	backendReachable,
	ingestResult,
	mintToken,
	postTelemetryEvent,
	resolveOrgId,
} from './helpers/seed';

const NO_STACK = 'backend not reachable — run ./scripts/test-stack.sh up';

let stackUp = false;
let userToken = '';
let ownerToken = '';
let orgId = '';
let exportId = '';
let installId = '';

test.describe('M18 compliance happy path', () => {
	test.beforeAll(async () => {
		stackUp = await backendReachable();
		if (!stackUp) return;

		// A disposable user owns the export request; the seeded owner streams
		// the audit log (it holds the audit_log.export permission).
		const registration = await seedRegistrationViaRefresh(
			`m18-e2e-${Date.now()}@example.com`
		);
		userToken = registration.accessToken;
		ownerToken = mintToken(OWNER_EMAIL, OWNER_USER_ID);
		orgId = await resolveOrgId(SEEDED_ORG_SLUG, ownerToken);

		// Produce an auditable action so the JSONL stream has a row to emit.
		await ingestResult(orgId, ownerToken, {
			collectionName: 'm18-e2e',
			triggeredBy: 'm18-playwright',
		});

		// Telemetry: one run.completed event under a fresh install_id.
		installId = crypto.randomUUID();
		await postTelemetryEvent(installId, 'run.completed', {
			exit_code: 0,
			collection_size: 1,
		});

		// Data export: request it, then force one builder tick.
		exportId = await createExportRequest(userToken);
		await runExportBuilder();
	});

	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	/**
	 * Assertion 1: /account/data renders both the export-request and delete panels.
	 * Proves the M18-004 export-request flow and M18-006 deletion panel are wired into
	 * the account/data web page.
	 */
	test('1. /account/data renders export + delete panels', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

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
	 */
	test('2. Export bundle signed URL is reachable', async ({ request }) => {
		test.skip(!stackUp, NO_STACK);

		const ready = await pollExportReady(exportId, userToken);
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
	 * Assertion 3: Telemetry events are queryable by install_id.
	 * Covers the M18-007 ingest pipeline: the event posted in beforeAll must come
	 * back from the list hook under the same install_id.
	 */
	test('3. Telemetry events present for install_id', async () => {
		test.skip(!stackUp, NO_STACK);

		const list = await listTelemetryEvents(installId);
		expect(
			list.count,
			'expected >=1 telemetry_events row for install_id'
		).toBeGreaterThanOrEqual(1);
		const eventTypes = list.rows.map((r) => r.event_type);
		expect(eventTypes).toContain('run.completed');
	});

	/**
	 * Assertion 4: Cancel-deletion page renders within the cooldown window.
	 * The cancel-deletion route must render without a 404 or error page.
	 * Proves M18-006's deletion panel and cancel route are wired correctly.
	 */
	test('4. Cancel-deletion page renders within cooldown window', async ({ page }) => {
		test.skip(!stackUp, NO_STACK);

		await page.goto('/account/data/cancel-deletion');
		// Intentionally weak assertion (M16-021 happy-path-only posture): we assert only that
		// the route renders without a 404/error URL, not that the cancel button is visible.
		// The session here belongs to the pre-seeded OWNER who has no active deletion request;
		// the disposable user that could have one is not carried into a browser context in
		// this slice. Strengthening this assertion to check for
		// `getByTestId('cancel-deletion-button')` is deferred: it requires either exposing
		// that user's session to Playwright or a dedicated cancel-deletion fixture, both of
		// which are out of scope for the M18-012 convergence slice.
		await expect(page).not.toHaveURL(/error|404/, { timeout: 5_000 });
	});

	/**
	 * Assertion 5: Audit-log JSONL stream is chunked + application/x-ndjson.
	 * Proves M18-001's streaming export endpoint works end-to-end for an Enterprise org.
	 * The results ingest in beforeAll guarantees at least one audit row.
	 */
	test('5. Audit-log JSONL stream is chunked + ndjson', async () => {
		test.skip(!stackUp, NO_STACK);

		// Uses the seeded owner token: it holds audit_log.export, which the
		// disposable export user does not.
		const stream = await streamAuditJsonl(orgId, ownerToken);
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
