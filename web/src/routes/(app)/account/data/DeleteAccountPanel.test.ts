import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import Panel from './DeleteAccountPanel.svelte';

const issueReauthToken = vi.fn();
const requestDeletion = vi.fn();
vi.mock('$lib/api/user-deletion', () => ({
	userDeletionApi: {
		issueReauthToken: (...a: unknown[]) => issueReauthToken(...a),
		requestDeletion: (...a: unknown[]) => requestDeletion(...a)
	}
}));

const DELETION_202 = {
	finalizes_at: '2026-06-17T03:00:00Z',
	cancellable_until: '2026-06-17T03:00:00Z',
	cancel_url: 'http://web.test/account/data/cancel-deletion'
};

/** Walks idle → confirm → reauth and submits the password. */
async function reachSubmit(user: ReturnType<typeof userEvent.setup>, password = 'TestPassword!1') {
	await user.click(screen.getByRole('button', { name: 'Delete my account' }));
	await user.click(screen.getByRole('button', { name: 'Yes, delete my account' }));
	await user.type(screen.getByLabelText('Password'), password);
	await user.click(screen.getByRole('button', { name: 'Confirm deletion' }));
}

beforeEach(() => {
	vi.clearAllMocks();
	issueReauthToken.mockResolvedValue({ reauth_token: 'drto_e2etest123' });
});

describe('delete account panel', () => {
	it('starts idle behind a single delete button', () => {
		render(Panel);

		expect(screen.getByRole('region', { name: 'Delete account' })).toBeVisible();
		expect(screen.getByRole('button', { name: 'Delete my account' })).toBeVisible();
		expect(screen.queryByLabelText('Password')).not.toBeInTheDocument();
	});

	it('requires confirmation and re-authentication before deleting', async () => {
		const user = userEvent.setup();
		requestDeletion.mockResolvedValue(DELETION_202);

		render(Panel);

		await user.click(screen.getByRole('button', { name: 'Delete my account' }));
		expect(screen.getByRole('alert')).toHaveTextContent('Are you sure?');
		expect(issueReauthToken).not.toHaveBeenCalled();

		await user.click(screen.getByRole('button', { name: 'Yes, delete my account' }));
		expect(screen.getByRole('dialog')).toBeVisible();
		expect(requestDeletion).not.toHaveBeenCalled();

		await user.type(screen.getByLabelText('Password'), 'TestPassword!1');
		await user.click(screen.getByRole('button', { name: 'Confirm deletion' }));

		await waitFor(() => expect(issueReauthToken).toHaveBeenCalledWith('TestPassword!1'));
		// The deletion request rides the freshly issued re-auth token.
		expect(requestDeletion).toHaveBeenCalledWith('drto_e2etest123');
	});

	it('shows the scheduled state with a cancellation link on success', async () => {
		const user = userEvent.setup();
		requestDeletion.mockResolvedValue(DELETION_202);

		render(Panel);
		await reachSubmit(user);

		const status = await screen.findByRole('status');
		expect(status).toHaveTextContent('scheduled for deletion');
		expect(screen.getByRole('link', { name: 'Cancel deletion' })).toHaveAttribute(
			'href',
			'/account/data/cancel-deletion'
		);
	});

	it('lists blocking organizations on a 409 owner_cannot_leave', async () => {
		const user = userEvent.setup();
		requestDeletion.mockRejectedValue(
			new ApiError('owner_cannot_leave', 'Owner cannot leave', 409, undefined, {
				blocking_orgs: [{ slug: 'my-org', name: 'My Org' }]
			})
		);

		render(Panel);
		await reachSubmit(user);

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('sole owner');
		expect(screen.getByText('My Org')).toBeVisible();
		expect(screen.getByRole('link', { name: 'Transfer ownership' })).toHaveAttribute(
			'href',
			'/org/my-org/members'
		);
	});

	it('recovers from the blocked state via Back', async () => {
		const user = userEvent.setup();
		requestDeletion.mockRejectedValue(
			new ApiError('owner_cannot_leave', 'Owner cannot leave', 409, undefined, {
				blocking_orgs: [{ slug: 'my-org', name: 'My Org' }]
			})
		);

		render(Panel);
		await reachSubmit(user);
		await screen.findByRole('alert');

		await user.click(screen.getByRole('button', { name: 'Back' }));

		expect(screen.getByRole('button', { name: 'Delete my account' })).toBeVisible();
	});

	it('keeps the modal open and names the cause when the password is wrong', async () => {
		const user = userEvent.setup();
		issueReauthToken.mockRejectedValue(new ApiError('unauthorized', 'nope', 401));

		render(Panel);
		await reachSubmit(user, 'wrong-password');

		expect(await screen.findByText('Incorrect password. Please try again.')).toBeVisible();
		expect(screen.getByRole('dialog')).toBeVisible();
		expect(requestDeletion).not.toHaveBeenCalled();
	});

	it('reports a non-401 re-auth failure distinctly', async () => {
		const user = userEvent.setup();
		issueReauthToken.mockRejectedValue(new ApiError('server_error', 'boom', 500));

		render(Panel);
		await reachSubmit(user);

		expect(await screen.findByText('Failed to verify password. Please try again.')).toBeVisible();
	});

	it('falls back to an error state when the deletion request fails outright', async () => {
		const user = userEvent.setup();
		requestDeletion.mockRejectedValue(new ApiError('server_error', 'boom', 500));

		render(Panel);
		await reachSubmit(user);

		expect(
			await screen.findByText(/Failed to submit deletion request/)
		).toBeVisible();
	});

	it('abandons the flow when the confirmation is cancelled', async () => {
		const user = userEvent.setup();
		render(Panel);

		await user.click(screen.getByRole('button', { name: 'Delete my account' }));
		await user.click(screen.getByRole('button', { name: 'Cancel' }));

		expect(screen.getByRole('button', { name: 'Delete my account' })).toBeVisible();
		expect(issueReauthToken).not.toHaveBeenCalled();
	});

	it('disables the submit button until a password is entered', async () => {
		const user = userEvent.setup();
		render(Panel);

		await user.click(screen.getByRole('button', { name: 'Delete my account' }));
		await user.click(screen.getByRole('button', { name: 'Yes, delete my account' }));

		expect(screen.getByRole('button', { name: 'Confirm deletion' })).toBeDisabled();
		await user.type(screen.getByLabelText('Password'), 'x');
		expect(screen.getByRole('button', { name: 'Confirm deletion' })).toBeEnabled();
	});
});
