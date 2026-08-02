import { test, expect } from '@playwright/test';

/**
 * E2e specs for the email-verification flow.
 * All backend calls are route-mocked — no live stack needed.
 * These pages are unauthenticated; no seedAuthCookie required.
 */
test.describe('Email verification flow', () => {
	test('request page submits email and shows generic confirmation', async ({ page, context }) => {
		await context.route('**/api/v1/auth/email-verification/resend', (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ ok: true })
			})
		);

		await page.goto('/auth/email-verification/request');
		await expect(page.getByTestId('email-input')).toBeVisible();

		await page.getByTestId('email-input').fill('user@example.com');
		await page.getByTestId('submit-button').click();

		await expect(page.getByTestId('confirmation-message')).toBeVisible();
	});

	test('confirm page with valid token auto-redirects to /?toast=email_verified', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/auth/email-verification/confirm', (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ ok: true })
			})
		);

		// After server-side redirect, we end up at / with the toast param
		await page.goto('/auth/email-verification/confirm?token=evtk_validtoken');

		await page.waitForURL(/\/\?toast=email_verified/);
	});

	test('confirm page with invalid token shows error and request-new-link', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/auth/email-verification/confirm', (route) =>
			route.fulfill({
				status: 400,
				contentType: 'application/problem+json',
				body: JSON.stringify({
					type: 'https://apitool.dev/errors/email-verification-token-invalid',
					title: 'Token invalid',
					status: 400
				})
			})
		);

		await page.goto('/auth/email-verification/confirm?token=bad_token');

		await expect(page.getByTestId('error-token-invalid')).toBeVisible();
		await expect(page.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/email-verification/request'
		);
	});

	test('confirm page without token shows missing-token state', async ({ page }) => {
		await page.goto('/auth/email-verification/confirm');

		// Should show an error state with the request-new-link
		await expect(page.getByTestId('error-token-invalid')).toBeVisible();
		await expect(page.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/email-verification/request'
		);
	});
});
