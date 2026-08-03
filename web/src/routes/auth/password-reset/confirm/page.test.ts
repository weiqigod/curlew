import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

vi.mock('$app/forms', () => ({ enhance: () => ({ destroy() {} }) }));

const WITH_TOKEN = { token: 'prst_validtoken', hasToken: true };

describe('password-reset confirm page', () => {
	it('renders the password form when a token is present', () => {
		render(Page, { props: { data: WITH_TOKEN, form: null } });

		const password = screen.getByTestId('password-input');
		expect(password).toBeVisible();
		expect(password).toHaveAttribute('type', 'password');
		expect(password).toBeRequired();
		// The token rides along in a hidden field so the action can read it.
		expect(document.querySelector('input[name="token"]')).toHaveValue('prst_validtoken');
	});

	it('shows the missing-token state with a new-link route and no form', () => {
		render(Page, { props: { data: { token: '', hasToken: false }, form: null } });

		expect(screen.queryByTestId('password-input')).not.toBeInTheDocument();
		expect(screen.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/password-reset/request'
		);
	});

	it('shows the weak-password error with the returned score', () => {
		// The action returns `{ error, score }` for a 422, but SvelteKit collapses
		// the ActionData union to `{ error: string }` — hence the component's
		// `'score' in form` guard, and this cast.
		const form = { error: 'weak_password', score: 1 } as unknown as { error: string };
		render(Page, { props: { data: WITH_TOKEN, form } });

		const err = screen.getByTestId('error-weak-password');
		expect(err).toBeVisible();
		expect(err).toHaveTextContent('1/4');
		// The form stays available for a retry.
		expect(screen.getByTestId('password-input')).toBeVisible();
	});

	it('shows the invalid-token error alongside a link to request a new one', () => {
		render(Page, { props: { data: WITH_TOKEN, form: { error: 'token_invalid' } } });

		expect(screen.getByTestId('error-token-invalid')).toBeVisible();
		expect(screen.getByTestId('request-new-link')).toHaveAttribute(
			'href',
			'/auth/password-reset/request'
		);
	});

	it('shows a generic message for a server error', () => {
		render(Page, { props: { data: WITH_TOKEN, form: { error: 'server_error' } } });

		expect(screen.getByRole('alert')).toHaveTextContent('Something went wrong.');
		expect(screen.queryByTestId('error-weak-password')).not.toBeInTheDocument();
		expect(screen.queryByTestId('error-token-invalid')).not.toBeInTheDocument();
	});
});
