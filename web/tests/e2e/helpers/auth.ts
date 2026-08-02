import { execSync } from 'child_process';
import type { BrowserContext } from '@playwright/test';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../../..');

// Fixed UUIDs seeded by scripts/seed-test-data.sh and scripts/seed-enterprise.sh.
// Keeps the JWT sub stable across repeated seedAuthCookie calls — otherwise
// test-token.sh falls back to uuidgen, and the users.Email UNIQUE index rejects
// the second upsert of the same email under a new sub, leaving the request
// authenticated as a user that has no organization membership.
const SEEDED_USER_IDS: Record<string, string> = {
	'owner@example.com': '00000000-0000-0000-0000-000000000001',
	'qa@acme.example': '00000000-0000-0000-0000-000000000002'
};

/**
 * Seeds an `access_token` cookie into the given Playwright browser context.
 *
 * Mints a dev JWT via `scripts/test-token.sh` and injects it as an
 * `access_token` cookie. This simulates magic-link authentication until
 * a real magic-link endpoint exists in the backend (see TODO(M4-auth)).
 */
export async function seedAuthCookie(
	context: BrowserContext,
	email: string,
	userId?: string
): Promise<void> {
	const tokenScript = path.join(REPO_ROOT, 'scripts', 'test-token.sh');
	const resolvedUserId = userId ?? SEEDED_USER_IDS[email];
	const userArg = resolvedUserId ? ` ${resolvedUserId}` : '';
	const token = execSync(`bash ${tokenScript} ${email}${userArg}`, { encoding: 'utf8' }).trim();

	const baseURL = process.env.WEB_BASE_URL ?? 'http://localhost:3000';
	const urlObj = new URL(baseURL);

	await context.addCookies([
		{
			name: 'access_token',
			value: token,
			domain: urlObj.hostname,
			path: '/',
			httpOnly: false,
			secure: false,
			sameSite: 'Lax'
		}
	]);
}
