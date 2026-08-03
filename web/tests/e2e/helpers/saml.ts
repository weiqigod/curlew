import { execSync } from 'node:child_process';
import type { Page } from '@playwright/test';
import path from 'path';

const REPO_ROOT = path.resolve(import.meta.dirname, '../../../..');
// CURLEW_BACKEND_URL is what the E2E gate and the other helpers export;
// BACKEND_URL is kept as a fallback for the shell scripts that set it.
const BACKEND_URL =
	process.env.CURLEW_BACKEND_URL ?? process.env.BACKEND_URL ?? 'http://localhost:5000';
const OWNER_USER_ID = '00000000-0000-0000-0000-000000000001';

/**
 * Resolves the org's internal GUID (without the `org_` prefix) by making
 * an authenticated request as the seeded owner and filtering by slug.
 *
 * @param slug - The org slug (e.g. "acme")
 * @returns The raw hex GUID used in SAML endpoint paths
 */
export async function lookupOrgGuid(slug: string): Promise<string> {
	const tokenScript = path.join(REPO_ROOT, 'scripts', 'test-token.sh');
	const token = execSync(`bash ${tokenScript} owner@example.com ${OWNER_USER_ID}`, {
		encoding: 'utf8'
	}).trim();

	const resp = await fetch(`${BACKEND_URL}/api/v1/organizations`, {
		headers: { Authorization: `Bearer ${token}` }
	});
	if (!resp.ok) {
		throw new Error(`lookupOrgGuid: GET /organizations returned ${resp.status}`);
	}
	const body = (await resp.json()) as { organizations: Array<{ id: string; slug: string }> };
	const org = body.organizations.find((o) => o.slug === slug);
	if (!org) {
		throw new Error(`lookupOrgGuid: org with slug "${slug}" not found`);
	}
	// Wire format: "org_<32-hex-chars>" — strip the prefix
	return org.id.replace(/^org_/, '');
}

/**
 * Performs the full SAML SP-initiated login flow via the fake IdP:
 *
 *   1. Navigate to the backend SAML login endpoint
 *      → backend 302-redirects to the fake IdP
 *   2. The fake IdP auto-submits a signed SAMLResponse form
 *      → backend ACS validates and sets the access_token cookie
 *   3. Browser follows the redirect to Saml:WebPortalUrl (/org/acme)
 *
 * After this call, the Playwright page is on /org/<slug> and the browser
 * context holds a valid `access_token` cookie.
 *
 * @param page - The Playwright page to use
 * @param slug - The org slug (e.g. "acme")
 */
export async function triggerSamlLogin(page: Page, slug: string): Promise<void> {
	const guid = await lookupOrgGuid(slug);
	// Navigate to the SP-initiated login endpoint — backend redirects to fake IdP,
	// fake IdP auto-submits form, backend ACS sets cookie and redirects to web portal.
	await page.goto(`${BACKEND_URL}/api/v1/sso/saml/${guid}/login`);
	await page.waitForURL(new RegExp(`/org/${slug}(\\?|$)`), { timeout: 15_000 });
}
