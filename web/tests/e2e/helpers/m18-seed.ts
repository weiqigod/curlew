/**
 * M18-012 convergence test helpers.
 * Provides utilities for the M18 compliance e2e scenario:
 * - polling export-request status
 * - backdating deletion requests
 * - running the deletion finalizer
 * - listing telemetry events by install_id
 * - streaming audit-log JSONL for Enterprise orgs
 *
 * Mirrors helpers/m16-seed.ts in structure.
 */

const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';

/** Response shape from GET /api/v1/users/me/export-requests/{id}. */
interface ExportRequestStatus {
	status: string;
	signed_url?: string;
	expires_at?: string;
}

/** Response shape from GET /api/v1/internal/test-hooks/list-telemetry-events. */
interface TelemetryEventRow {
	install_id: string;
	event_type: string;
	received_at: string;
}

interface ListTelemetryEventsResponse {
	count: number;
	rows: TelemetryEventRow[];
}

/** Result from streaming the audit-log JSONL response. */
interface AuditJsonlResult {
	status: number;
	contentType: string;
	transferEncoding: string;
	lineCount: number;
}

/**
 * Polls GET /api/v1/users/me/export-requests/{exportId} until status === 'ready'
 * or the timeout elapses. Returns the final status object on success, throws on timeout.
 */
export async function pollExportReady(
	exportId: string,
	bearerToken: string,
	timeoutMs = 30_000
): Promise<ExportRequestStatus> {
	const deadline = Date.now() + timeoutMs;
	while (Date.now() < deadline) {
		const res = await fetch(
			`${BACKEND_URL}/api/v1/users/me/export-requests/${exportId}`,
			{ headers: { Authorization: `Bearer ${bearerToken}` } }
		);
		if (res.ok) {
			const body = (await res.json()) as ExportRequestStatus;
			if (body.status === 'ready') return body;
		}
		await new Promise<void>((r) => setTimeout(r, 500));
	}
	throw new Error(
		`export ${exportId} did not reach status=ready within ${timeoutMs}ms`
	);
}

/**
 * Creates a GDPR data-export request for the bearer token's user and returns
 * its id. Pair with {@link runExportBuilder} and {@link pollExportReady}.
 */
export async function createExportRequest(bearerToken: string): Promise<string> {
	const res = await fetch(`${BACKEND_URL}/api/v1/users/me/export-requests`, {
		method: 'POST',
		headers: { Authorization: `Bearer ${bearerToken}` },
	});
	if (!res.ok)
		throw new Error(`export-requests: HTTP ${res.status} ${await res.text()}`);
	const body = (await res.json()) as { id?: string };
	if (!body.id) throw new Error('export-request response carried no id');
	return body.id;
}

/**
 * Posts to /api/v1/internal/test-hooks/run-export-builder to deterministically
 * trigger one export-builder tick instead of waiting for its timer.
 */
export async function runExportBuilder(): Promise<void> {
	const res = await fetch(
		`${BACKEND_URL}/api/v1/internal/test-hooks/run-export-builder`,
		{ method: 'POST' }
	);
	if (!res.ok)
		throw new Error(`run-export-builder: HTTP ${res.status} ${await res.text()}`);
}

/**
 * Posts to /api/v1/internal/test-hooks/backdate-deletion-request to push
 * users.pending_deletion_at past the 30-day cooldown, enabling the finalizer to run.
 */
export async function backdateDeletionRequest(
	userId: string,
	daysAgo = 31
): Promise<void> {
	const res = await fetch(
		`${BACKEND_URL}/api/v1/internal/test-hooks/backdate-deletion-request`,
		{
			method: 'POST',
			headers: { 'Content-Type': 'application/json' },
			body: JSON.stringify({ user_id: userId, days_ago: daysAgo }),
		}
	);
	if (!res.ok)
		throw new Error(
			`backdate-deletion-request: HTTP ${res.status} ${await res.text()}`
		);
}

/**
 * Posts to /api/v1/internal/test-hooks/run-deletion-finalizer to deterministically
 * trigger one UserDeletionFinalizerHost tick.
 */
export async function runDeletionFinalizer(): Promise<void> {
	const res = await fetch(
		`${BACKEND_URL}/api/v1/internal/test-hooks/run-deletion-finalizer`,
		{ method: 'POST' }
	);
	if (!res.ok)
		throw new Error(
			`run-deletion-finalizer: HTTP ${res.status} ${await res.text()}`
		);
}

/**
 * Calls GET /api/v1/internal/test-hooks/list-telemetry-events?install_id={installId}
 * and returns the count + rows.
 */
export async function listTelemetryEvents(
	installId: string
): Promise<ListTelemetryEventsResponse> {
	const url = `${BACKEND_URL}/api/v1/internal/test-hooks/list-telemetry-events?install_id=${encodeURIComponent(installId)}`;
	const res = await fetch(url);
	if (!res.ok)
		throw new Error(
			`list-telemetry-events: HTTP ${res.status} ${await res.text()}`
		);
	return res.json() as Promise<ListTelemetryEventsResponse>;
}

/**
 * Streams GET /api/v1/organizations/{orgId}/audit-log?format=jsonl from the backend
 * and returns status, content-type, transfer-encoding header value, and NDJSON line count.
 * Requires an Enterprise-tier bearer token.
 */
export async function streamAuditJsonl(
	orgId: string,
	bearerToken: string
): Promise<AuditJsonlResult> {
	const res = await fetch(
		`${BACKEND_URL}/api/v1/organizations/${orgId}/audit-log?format=jsonl&from=2025-01-01`,
		{ headers: { Authorization: `Bearer ${bearerToken}` } }
	);
	const body = await res.text();
	const lineCount = body
		.split('\n')
		.filter((l) => l.trim().length > 0).length;
	return {
		status: res.status,
		contentType: res.headers.get('content-type') ?? '',
		transferEncoding: res.headers.get('transfer-encoding') ?? '',
		lineCount,
	};
}
