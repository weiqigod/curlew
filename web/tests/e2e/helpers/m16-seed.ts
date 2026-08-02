/**
 * M16-021 convergence test helpers.
 * Provides utilities for seeding M16 state via internal endpoints and
 * extracting tokens from the email-audit log.
 */

const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';

/** Response shape from POST /internal/test/seed-refresh. */
interface SeedRefreshResponse {
	plaintext: string;
	device_id: string;
}

/** Response shape from POST /api/v1/auth/refresh. */
interface RefreshResponse {
	access_token: string;
	license_jwt: string;
	refresh_token: string;
	device_id: string;
}

/** An entry from GET /internal/test/email-audit. */
interface EmailAuditEntry {
	template_slug: string;
	to: string;
	variables: Record<string, string>;
	enqueued_at: string;
}

/**
 * Seeds a user via POST /internal/test/seed-refresh, then immediately calls
 * POST /api/v1/auth/refresh to mint a full token trio.
 * Returns all tokens and IDs needed by the e2e scenario.
 */
export async function seedRegistrationViaRefresh(email: string): Promise<{
	refreshToken: string;
	deviceId: string;
	/** User ID parsed from the License JWT `sub` claim. */
	userId: string;
	accessToken: string;
	licenseJwt: string;
}> {
	const seedRes = await fetch(`${BACKEND_URL}/internal/test/seed-refresh`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ email }),
	});
	if (!seedRes.ok)
		throw new Error(`seed-refresh failed: ${seedRes.status} ${await seedRes.text()}`);

	const seed = (await seedRes.json()) as SeedRefreshResponse;

	const refreshRes = await fetch(`${BACKEND_URL}/api/v1/auth/refresh`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ refresh_token: seed.plaintext, device_id: seed.device_id }),
	});
	if (!refreshRes.ok)
		throw new Error(`auth/refresh failed: ${refreshRes.status} ${await refreshRes.text()}`);

	const tokens = (await refreshRes.json()) as RefreshResponse;

	// Parse user_id from the License JWT `sub` claim.
	const payload = decodeJwtPayload(tokens.license_jwt);
	const userId = (payload['sub'] as string) ?? '';

	return {
		refreshToken: seed.plaintext,
		deviceId: seed.device_id,
		userId,
		accessToken: tokens.access_token,
		licenseJwt: tokens.license_jwt,
	};
}

/**
 * Seeds a near-expiry trial row via POST /internal/test/seed-near-expiry-trial.
 */
export async function seedNearExpiryTrial(
	userId: string,
	feature: string,
	daysUntilExpiry = 2
): Promise<{ trialId: string; expiresAt: string }> {
	const res = await fetch(`${BACKEND_URL}/internal/test/seed-near-expiry-trial`, {
		method: 'POST',
		headers: { 'Content-Type': 'application/json' },
		body: JSON.stringify({ user_id: userId, feature, days_until_expiry: daysUntilExpiry }),
	});
	if (!res.ok)
		throw new Error(`seed-near-expiry-trial failed: ${res.status} ${await res.text()}`);

	const body = (await res.json()) as { trial_id: string; expires_at: string };
	return { trialId: body.trial_id, expiresAt: body.expires_at };
}

/**
 * Forces a TrialExpiryNotifier tick via POST /internal/test/trial-expiry-tick.
 * Throws if the endpoint returns a non-2xx status.
 */
export async function forceTrialExpiryTick(): Promise<void> {
	const res = await fetch(`${BACKEND_URL}/internal/test/trial-expiry-tick`, { method: 'POST' });
	if (!res.ok && res.status !== 503)
		throw new Error(`trial-expiry-tick failed: ${res.status} ${await res.text()}`);
}

/**
 * Extracts the token embedded in the given template's email-audit entry.
 * Looks for a variable named `reset_url` or `verification_url` and parses
 * the `?token=` query parameter. Polls until the entry appears or times out.
 */
export async function extractTokenFromAuditEntry(
	templateSlug: string,
	tokenVariableName: string,
	timeoutMs = 5000,
	recipient?: string
): Promise<string> {
	const deadline = Date.now() + timeoutMs;
	const url = `${BACKEND_URL}/internal/test/email-audit?limit=50`;

	while (Date.now() < deadline) {
		try {
			const res = await fetch(url);
			if (res.ok) {
				const body = (await res.json()) as { entries: EmailAuditEntry[] };
				// The audit endpoint returns entries oldest-first. Select the newest
				// matching token so reruns do not reuse an already-consumed link.
				const entry = [...body.entries]
					.reverse()
					.find(
						(e) => e.template_slug === templateSlug && (!recipient || e.to === recipient)
					);
				if (entry) {
					const raw = entry.variables[tokenVariableName] ?? '';
					// Extract token from URL query param if it looks like a URL.
					if (raw.startsWith('http')) {
						const parsed = new URL(raw);
						const token = parsed.searchParams.get('token');
						if (token) return token;
					}
					// Otherwise return the value directly (already a plain token).
					if (raw) return raw;
				}
			}
		} catch {
			// Swallow transient errors — backend may still be starting.
		}
		await new Promise((r) => setTimeout(r, 200));
	}

	throw new Error(
		`extractTokenFromAuditEntry: template_slug="${templateSlug}" variable="${tokenVariableName}"${recipient ? ` recipient="${recipient}"` : ''} not found within ${timeoutMs}ms`
	);
}

/**
 * Decodes a JWT payload (no signature verification — for test assertions only).
 */
export function decodeJwtPayload(jwt: string): Record<string, unknown> {
	const parts = jwt.split('.');
	if (parts.length < 2) throw new Error(`Invalid JWT: ${jwt}`);
	const padding = '='.repeat((4 - (parts[1].length % 4)) % 4);
	const decoded = Buffer.from(parts[1].replace(/-/g, '+').replace(/_/g, '/') + padding, 'base64').toString('utf8');
	return JSON.parse(decoded) as Record<string, unknown>;
}
