import { test, expect } from '@playwright/test';

/**
 * E2e specs for the password-reset flow.
 * All backend calls are route-mocked — no live stack needed.
 * These pages are unauthenticated; no seedAuthCookie required.
 */
test.describe('Password reset flow', () => {
	test('request page submits email and shows generic confirmation', async ({ page, context }) => {
		await context.route('**/api/v1/auth/password-reset/request', (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ ok: true, message: 'If that email exists, a reset link has been sent.' })
			})
		);

		await page.goto('/auth/password-reset/request');
		await expect(page.getByTestId('email-input')).toBeVisible();

		await page.getByTestId('email-input').fill('user@example.com');
		await page.getByTestId('submit-button').click();

		await expect(page.getByTestId('confirmation-message')).toBeVisible();
	});

	test('confirm page with strong password redirects to /login on success', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/auth/password-reset/confirm', (route) =>
			route.fulfill({
				status: 200,
				contentType: 'application/json',
				body: JSON.stringify({ ok: true })
			})
		);

		await page.goto('/auth/password-reset/confirm?token=prst_validtoken');
		await expect(page.getByTestId('password-input')).toBeVisible();

		await page.getByTestId('password-input').fill('Tr0ub4dor&3-very-strong');
		await page.getByTestId('submit-button').click();

		await page.waitForURL(/\/login\?toast=password_reset_success/);
	});

	test('confirm page with weak password shows inline 422 score error', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/auth/password-reset/confirm', (route) =>
			route.fulfill({
				status: 422,
				contentType: 'application/problem+json',
				body: JSON.stringify({
					type: 'https://apitool.dev/errors/password-too-weak',
					title: 'Password rejected',
					status: 422,
					detail: 'score 1/4',
					score: 1
				})
			})
		);

		await page.goto('/auth/password-reset/confirm?token=prst_xxx');
		await page.getByTestId('password-input').fill('weak');
		await page.getByTestId('submit-button').click();

		await expect(page.getByTestId('error-weak-password')).toBeVisible();
		await expect(page.getByTestId('error-weak-password')).toContainText('1/4');
	});

	test('confirm page with invalid token shows 400 error and new-link', async ({
		page,
		context
	}) => {
		await context.route('**/api/v1/auth/password-reset/confirm', (route) =>
			route.fulfill({
				status: 400,
				contentType: 'application/problem+json',
				body: JSON.stringify({
					type: 'https://apitool.dev/errors/password-reset-token-invalid',
					title: 'Token invalid',
					status: 400
				})
			})
		);

		await page.goto('/auth/password-reset/confirm?token=bad_token');
		await page.getByTestId('password-input').fill('StrongP@ss1!');
		await page.getByTestId('submit-button').click();

		await expect(page.getByTestId('error-token-invalid')).toBeVisible();
		await expect(page.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/password-reset/request'
		);
	});

	test('confirm page without token shows missing-token state with new-link', async ({ page }) => {
		await page.goto('/auth/password-reset/confirm');

		// No form should be rendered when no token is present
		await expect(page.getByTestId('password-input')).not.toBeVisible();
		await expect(page.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/password-reset/request'
		);
	});
});
