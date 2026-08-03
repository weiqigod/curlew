import { render, screen } from '@testing-library/svelte';
import { describe, expect, it, vi } from 'vitest';
import Page from './+page.svelte';

vi.mock('$app/forms', () => ({ enhance: () => ({ destroy() {} }) }));

describe('email-verification request page', () => {
	it('renders the email form before submission', () => {
		render(Page, { props: { form: null } });

		const email = screen.getByTestId('email-input');
		expect(email).toBeVisible();
		expect(email).toBeRequired();
		expect(email).toHaveAttribute('type', 'email');
		expect(screen.getByTestId('submit-button')).toBeVisible();
		expect(screen.queryByTestId('confirmation-message')).not.toBeInTheDocument();
	});

	it('replaces the form with an enumeration-safe confirmation after submission', () => {
		render(Page, { props: { form: { submitted: true } } });

		const confirmation = screen.getByTestId('confirmation-message');
		expect(confirmation).toBeVisible();
		// Must not reveal whether the address is registered or already verified.
		expect(confirmation).toHaveTextContent('If that email address is registered and unverified');
		expect(screen.queryByTestId('email-input')).not.toBeInTheDocument();
	});
});
