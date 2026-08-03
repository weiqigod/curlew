/**
 * HTTP seeding helpers for the convergence E2E specs.
 *
 * These specs used to seed the backend by shelling out to
 * `curlew run --report-upload`. The CLI is backend-free now — it has no
 * upload path, no login, and no worker — so the specs drive the same REST
 * endpoints the CLI used to call:
 *
 *   POST /api/v1/organizations/{orgId}/results   (was: --report-upload)
 *   POST /api/v1/pr-checks                       (was: --pr/--repo)
 *   POST /api/v1/telemetry/events                (was: curlew telemetry)
 *
 * Seeding over HTTP also removes the build dependency: the specs no longer
 * need a compiled ./curlew binary on disk, only a running stack.
 */
import { mintToken } from './auth';

export const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';

export { mintToken };

/**
 * True when the backend answers its health probe.
 *
 * Specs call this once in `beforeAll` and skip themselves when it is false, so
 * `npm run test:e2e` stays usable without the docker stack. It replaces the old
 * `CURLEW_BACKEND_TOKEN` guard, which only ever stood in for "a stack is up".
 */
export async function backendReachable(timeoutMs = 3000): Promise<boolean> {
	try {
		const res = await fetch(`${BACKEND_URL}/health`, {
			signal: AbortSignal.timeout(timeoutMs)
		});
		return res.ok;
	} catch {
		return false;
	}
}

/** One request row inside a seeded result. */
export interface SeedResultItem {
	name: string;
	/** passed | failed | skipped | error — matches the backend ResultStatus enum. */
	status: 'passed' | 'failed' | 'skipped' | 'error';
	duration_ms: number;
	message?: string;
	method?: string;
	request_url?: string;
}

/** Overrides for the synthetic run body posted to the results endpoint. */
export interface SeedResultOptions {
	collectionName?: string;
	passCount?: number;
	failCount?: number;
	skippedCount?: number;
	durationMs?: number;
	triggeredBy?: string;
	gitSha?: string;
	items?: SeedResultItem[];
}

/** What the results endpoint returns on a successful ingest. */
export interface SeededResult {
	/** Wire-format result id, e.g. `res_9f2c…`. */
	id: string;
}

/** Outcome of a pr-check upload, including non-2xx responses. */
export interface SeededPrCheck {
	httpStatus: number;
	/** posted | queued when the upload succeeded; undefined on an error response. */
	state?: string;
	prCheckId?: string;
	githubCheckRunId?: number | null;
	/** Raw response body, for assertion messages when the upload fails. */
	body: string;
}

/**
 * Resolves an organization's wire id (`org_…`) from its slug.
 * Throws when the slug is not among the caller's organizations — that means
 * the seed scripts have not run, which is worth failing loudly on.
 */
export async function resolveOrgId(slug: string, bearerToken: string): Promise<string> {
	const res = await fetch(`${BACKEND_URL}/api/v1/organizations`, {
		headers: { Authorization: `Bearer ${bearerToken}` }
	});
	if (!res.ok) {
		throw new Error(`GET /organizations: HTTP ${res.status} ${await res.text()}`);
	}
	const body = (await res.json()) as { organizations?: Array<{ id: string; slug: string }> };
	const match = (body.organizations ?? []).find((o) => o.slug === slug);
	if (!match) {
		throw new Error(
			`no organization with slug "${slug}" — has scripts/seed-test-data.sh run against ${BACKEND_URL}?`
		);
	}
	return match.id;
}

/**
 * Ingests a synthetic run into an organization — the HTTP equivalent of what
 * `curlew run --report-upload` used to POST after executing a collection.
 *
 * Counts default to a single passing request. Pass `failCount` (and matching
 * `items`) to seed a failing run.
 */
export async function ingestResult(
	orgId: string,
	bearerToken: string,
	opts: SeedResultOptions = {}
): Promise<SeededResult> {
	const passCount = opts.passCount ?? 1;
	const failCount = opts.failCount ?? 0;
	const skippedCount = opts.skippedCount ?? 0;
	const items =
		opts.items ??
		[
			...Array.from({ length: passCount }, (_, i) => ({
				name: `seeded request ${i + 1}`,
				status: 'passed' as const,
				duration_ms: 12,
				method: 'GET',
				request_url: `${BACKEND_URL}/healthz`
			})),
			...Array.from({ length: failCount }, (_, i) => ({
				name: `seeded failing request ${i + 1}`,
				status: 'failed' as const,
				duration_ms: 15,
				message: 'expected status 200, got 500',
				method: 'GET',
				request_url: `${BACKEND_URL}/healthz`
			}))
		];

	const payload = {
		collection_name: opts.collectionName ?? 'e2e-collection',
		run_at: new Date().toISOString(),
		duration_ms: opts.durationMs ?? 250,
		pass_count: passCount,
		fail_count: failCount,
		skipped_count: skippedCount,
		triggered_by: opts.triggeredBy ?? 'playwright-e2e',
		git_sha: opts.gitSha,
		items
	};

	const res = await fetch(`${BACKEND_URL}/api/v1/organizations/${orgId}/results`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${bearerToken}`,
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(payload)
	});
	if (res.status !== 202) {
		throw new Error(`POST /results: HTTP ${res.status} ${await res.text()}`);
	}
	const body = (await res.json()) as { result_id: string };
	return { id: body.result_id };
}

/** Inputs for a pr-check upload. */
export interface SeedPrCheckOptions {
	repo: string;
	pr: number;
	state: 'success' | 'failure' | 'cancelled' | 'timed_out' | 'neutral' | 'skipped';
	/** 40-char hex SHA. Defaults to a unique synthetic one per call. */
	headSha?: string;
	resultId?: string;
	provider?: 'github' | 'gitlab';
	summary?: string;
}

/**
 * Uploads a pr-check — the HTTP equivalent of `curlew run --report-upload
 * --pr N --repo owner/name`. The backend resolves the organization from the
 * bearer token's membership, so there is no org parameter.
 *
 * Non-2xx responses are returned rather than thrown: the pr_checks row is
 * written before the check run is posted to GitHub, so a stack without a
 * usable GitHub installation still produces the dashboard row these specs
 * assert on. Callers that need a posted check run assert on `state`.
 */
export async function uploadPrCheck(
	bearerToken: string,
	opts: SeedPrCheckOptions
): Promise<SeededPrCheck> {
	const payload = {
		repo: opts.repo,
		pr: opts.pr,
		state: opts.state,
		head_sha: opts.headSha ?? syntheticHeadSha(),
		result_id: opts.resultId,
		provider: opts.provider,
		output: opts.summary ? { title: 'curlew', summary: opts.summary } : undefined
	};

	const res = await fetch(`${BACKEND_URL}/api/v1/pr-checks`, {
		method: 'POST',
		headers: {
			Authorization: `Bearer ${bearerToken}`,
			'Content-Type': 'application/json'
		},
		body: JSON.stringify(payload)
	});
	const body = await res.text();
	if (!res.ok) {
		return { httpStatus: res.status, body };
	}
	const parsed = JSON.parse(body) as {
		state: string;
		pr_check_id: string;
		github_check_run_id: number | null;
	};
	return {
		httpStatus: res.status,
		state: parsed.state,
		prCheckId: parsed.pr_check_id,
		githubCheckRunId: parsed.github_check_run_id,
		body
	};
}

/**
 * Seeds a result and its pr-check together — one call for what a single
 * `curlew run --report-upload --pr … --repo …` invocation produced.
 */
export async function seedRunWithPrCheck(
	orgId: string,
	bearerToken: string,
	prCheck: Omit<SeedPrCheckOptions, 'resultId'>,
	result: SeedResultOptions = {}
): Promise<{ result: SeededResult; prCheck: SeededPrCheck }> {
	const seeded = await ingestResult(orgId, bearerToken, result);
	const check = await uploadPrCheck(bearerToken, { ...prCheck, resultId: seeded.id });
	return { result: seeded, prCheck: check };
}

/**
 * Posts a telemetry event, standing in for the emitter the CLI used to have.
 * The endpoint is anonymous but requires an Idempotency-Key header; a unique
 * one is generated per call so repeated seeding is never deduplicated away.
 */
export async function postTelemetryEvent(
	installId: string,
	eventType: string,
	payload: Record<string, unknown> = {}
): Promise<void> {
	const res = await fetch(`${BACKEND_URL}/api/v1/telemetry/events`, {
		method: 'POST',
		headers: {
			'Content-Type': 'application/json',
			'Idempotency-Key': `e2e-${eventType}-${crypto.randomUUID()}`.slice(0, 64)
		},
		body: JSON.stringify({
			install_id: installId,
			event_type: eventType,
			event_payload: payload
		})
	});
	if (res.status !== 202) {
		throw new Error(`POST /telemetry/events: HTTP ${res.status} ${await res.text()}`);
	}
}

/** A unique 40-char hex string, shaped like a git commit SHA. */
export function syntheticHeadSha(): string {
	return crypto.randomUUID().replace(/-/g, '').padEnd(40, '0').slice(0, 40);
}

/** Converts a wire org id (`org_<32 hex>`) to the dashed GUID internal APIs take. */
export function orgGuid(wireOrgId: string): string {
	const hex = wireOrgId.replace(/^org_/, '');
	if (!/^[0-9a-f]{32}$/i.test(hex)) {
		throw new Error(`not a wire org id: ${wireOrgId}`);
	}
	return [
		hex.slice(0, 8),
		hex.slice(8, 12),
		hex.slice(12, 16),
		hex.slice(16, 20),
		hex.slice(20)
	].join('-');
}

/** Fixture state seeded through POST /internal/test/seed-m14. */
export interface SeedM14Options {
	installationId?: number;
	appId?: number;
	repos?: string[];
	stripeCustomerId?: string;
	tier?: string;
}

/**
 * Seeds the M14 convergence state for an org: subscription tier, GitHub App
 * installation, and Stripe customer linkage. Idempotent — the endpoint updates
 * the existing installation row rather than inserting a second one.
 */
export async function seedM14(wireOrgId: string, opts: SeedM14Options = {}): Promise<void> {
	const res = await fetch(`${BACKEND_URL}/internal/test/seed-m14`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({
			org_id: orgGuid(wireOrgId),
			installation_id: opts.installationId ?? 12345,
			app_id: opts.appId ?? 111,
			repos: opts.repos ?? ['acme/api'],
			stripe_customer_id: opts.stripeCustomerId,
			tier: opts.tier
		})
	});
	if (!res.ok) {
		throw new Error(`POST /internal/test/seed-m14: HTTP ${res.status} ${await res.text()}`);
	}
}

/** Base URL of the stripe-mock sidecar published by docker-compose.test.yml. */
export const STRIPE_MOCK_URL = process.env.STRIPE_MOCK_URL ?? 'http://localhost:12111';

/**
 * Returns the customer id stripe-mock attaches to every invoice it serves.
 *
 * stripe-mock answers `GET /v1/invoices/{id}` with a fixture whose customer is
 * fixed, and the invoice webhook handler skips any invoice whose customer does
 * not map to a local org. Reading the id here — rather than hard-coding it —
 * keeps the linkage correct across stripe-mock versions.
 */
export async function stripeMockCustomerId(): Promise<string> {
	const res = await fetch(`${STRIPE_MOCK_URL}/v1/invoices/in_e2e_probe`, {
		headers: { Authorization: 'Bearer sk_test_123' }
	});
	if (!res.ok) {
		throw new Error(`stripe-mock GET /v1/invoices: HTTP ${res.status} ${await res.text()}`);
	}
	const body = (await res.json()) as { customer?: string };
	if (!body.customer) {
		throw new Error('stripe-mock invoice fixture has no customer id');
	}
	return body.customer;
}
