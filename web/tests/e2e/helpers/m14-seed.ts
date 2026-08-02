/**
 * M14-021 convergence test helpers.
 * Provides utilities for replaying Stripe events and waiting for the email-audit log.
 */
import { execSync } from 'child_process';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../../..');
const BACKEND_URL = process.env.CURLEW_BACKEND_URL ?? 'http://localhost:5000';

/**
 * Replays a Stripe event via scripts/replay-stripe-event.sh.
 * Uses the default BACKEND_URL unless overridden by env.
 */
export function replayStripeEvent(eventType: string, objectId: string): void {
	const script = path.join(REPO_ROOT, 'scripts', 'replay-stripe-event.sh');
	const cmd = `bash "${script}" "${eventType}" "${objectId}"`;
	execSync(cmd, {
		encoding: 'utf8',
		cwd: REPO_ROOT,
		env: {
			...process.env,
			BACKEND_URL,
		},
	});
}

/**
 * Polls GET /internal/test/email-audit until an entry with the given
 * template_slug appears, or throws after timeoutMs.
 */
export async function waitForEmailAuditEntry(
	templateSlug: string,
	timeoutMs = 5000,
	recipient?: string
): Promise<void> {
	const deadline = Date.now() + timeoutMs;
	const url = `${BACKEND_URL}/internal/test/email-audit?limit=50`;

	while (Date.now() < deadline) {
		try {
			const res = await fetch(url);
			if (res.ok) {
				const body = (await res.json()) as {
					entries: Array<{ template_slug: string; to: string }>;
				};
				if (
					body.entries.some(
						(e) => e.template_slug === templateSlug && (!recipient || e.to === recipient)
					)
				) {
					return;
				}
			}
		} catch {
			// Swallow transient errors — backend may still be starting.
		}
		await new Promise((r) => setTimeout(r, 200));
	}

	throw new Error(
		`waitForEmailAuditEntry: template_slug="${templateSlug}"${recipient ? ` recipient="${recipient}"` : ''} not found within ${timeoutMs}ms at ${url}`
	);
}
