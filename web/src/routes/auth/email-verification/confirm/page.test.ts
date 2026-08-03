import { render, screen } from '@testing-library/svelte';
import { describe, expect, it } from 'vitest';
import Page from './+page.svelte';

describe('email-verification confirm page', () => {
	it.each(['missing_token', 'token_invalid'])(
		'shows the invalid-link state with a new-link route for %s',
		(error) => {
			render(Page, { props: { data: { error } } });

			expect(screen.getByTestId('error-token-invalid')).toBeVisible();
			expect(screen.getByTestId('request-new-link')).toHaveAttribute(
				'href',
				'/auth/email-verification/request'
			);
		}
	);

	it('shows a generic message plus a recovery link for a server error', () => {
		render(Page, { props: { data: { error: 'server_error' } } });

		expect(screen.getByRole('heading')).toHaveTextContent('Something went wrong');
		expect(screen.queryByTestId('error-token-invalid')).not.toBeInTheDocument();
		expect(
			screen.getByRole('link', { name: 'Request a new verification email' })
		).toHaveAttribute('href', '/auth/email-verification/request');
	});

	// NOTE: the template's final `{:else}` ("Verifying your email…") is
	// unreachable — a successful load throws a redirect and every other path
	// returns an `error`, so PageData always carries one. Left untested
	// deliberately rather than asserted against a state that cannot occur.
});
