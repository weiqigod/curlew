/**
 * M18-004: GDPR /account/data page — Playwright e2e smoke spec.
 *
 * Covers the happy-path: authenticate → click Request data export → mock the
 * builder tick via route interception → poll status → see Ready → verify
 * the Download link appears and the footer cross-link is present.
 *
 * The spec uses Playwright route mocking so it runs without a live stack.
 * Full convergence (MinIO, real signed URL, live backend) is deferred to M18-012.
 *
 * DoD item: "Playwright smoke covers the happy-path web flow."
 */
import { test, expect } from '@playwright/test';
import { seedAuthCookie } from './helpers/auth';
import { OWNER_EMAIL, OWNER_USER_ID } from './helpers/fixtures';

const ACCOUNT_DATA_URL = '/account/data';

const QUEUED_REQUEST = {
	id: 'e2e-req-001',
	status: 'queued',
	created_at: '2026-05-18T12:00:00Z',
};

const READY_REQUEST = {
	id: 'e2e-req-001',
	status: 'ready',
	created_at: '2026-05-18T12:00:00Z',
	ready_at: '2026-05-18T12:01:00Z',
	expires_at: '2026-05-19T12:01:00Z',
	signed_url: 'http://test/objects/bundle.json?exp=1748001660',
};

test.describe('account/data export page', () => {
	test.beforeEach(async ({ context }) => {
		await seedAuthCookie(context, OWNER_EMAIL, OWNER_USER_ID);
	});

	// Happy-path: authenticate → click → queued → ready → download link visible
	test('happy-path: request export → queued → ready → download link', async ({
		page,
		context
	}) => {
		// POST /api/v1/users/me/export-requests returns 202 queued
		await context.route('**/api/v1/users/me/export-requests', (route) => {
			if (route.request().method() === 'POST') {
				return route.fulfill({
					status: 202,
					contentType: 'application/json',
					body: JSON.stringify(QUEUED_REQUEST)
				});
			}
			return route.continue();
		});

		// GET /api/v1/users/me/export-requests/:id → returns ready after first poll
		let pollCount = 0;
		await context.route('**/api/v1/users/me/export-requests/e2e-req-001', (route) => {
			pollCount++;
			return route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify(pollCount === 1 ? QUEUED_REQUEST : READY_REQUEST)
			});
		});

		await page.goto(ACCOUNT_DATA_URL);
		await expect(page.getByRole('button', { name: 'Request data export' })).toBeVisible();

		// Click the button — triggers POST
		await page.getByRole('button', { name: 'Request data export' }).click();

		// After POST responds with queued, the Queued badge should appear
		await expect(page.locator('[data-status="queued"]')).toBeVisible({ timeout: 5000 });

		// Polling eventually transitions to ready; wait for the Download link
		await expect(page.getByRole('link', { name: 'Download bundle' })).toBeVisible({
			timeout: 15000
		});

		// Download link href is the signed URL
		const href = await page.getByRole('link', { name: 'Download bundle' }).getAttribute('href');
		expect(href).toContain('bundle.json');
	});

	// 429 rate-limit path: a second request within 24h shows an error message
	test('429 rate-limit: shows error message when re-requesting within 24h', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/users/me/export-requests', (route) => {
			if (route.request().method() === 'POST') {
				return route.fulfill({
					status: 429,
					contentType: 'application/problem+json',
					headers: { 'Retry-After': '86399' },
					body: JSON.stringify({
						status: 429,
						title: 'Export rate limited',
						detail: 'You can request one export per 24 hours.',
						extensions: { code: 'export_rate_limited' }
					})
				});
			}
			return route.continue();
		});

		await page.goto(ACCOUNT_DATA_URL);
		await page.getByRole('button', { name: 'Request data export' }).click();

		// Should display rate-limit error message
		await expect(page.locator('[role="alert"]')).toBeVisible({ timeout: 5000 });
		await expect(page.locator('[role="alert"]')).toContainText('24 hours');
	});

	// Footer cross-link to data-inventory.md
	test('footer links to data-inventory document', async ({ page, context }) => {
		// No export requests in flight
		await context.route('**/api/v1/users/me/export-requests', (route) => route.continue());

		await page.goto(ACCOUNT_DATA_URL);

		// The footer anchor must link to the data-inventory doc
		const footerLink = page.locator('footer a[href*="data-inventory"]');
		await expect(footerLink).toBeVisible();
		const href = await footerLink.getAttribute('href');
		expect(href).toContain('data-inventory');
	});
});
