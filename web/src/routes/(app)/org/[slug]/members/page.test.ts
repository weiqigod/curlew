import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { Invitation } from '$lib/types/invitations';
import type { Member } from '$lib/types/members';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const invalidateAll = vi.fn();
vi.mock('$app/navigation', () => ({ invalidateAll: () => invalidateAll() }));

const create = vi.fn();
const revoke = vi.fn();
vi.mock('$lib/api/invitations', () => ({
	invitationsApi: {
		create: (...a: unknown[]) => create(...a),
		revoke: (...a: unknown[]) => revoke(...a)
	}
}));

const INV_ID = 'inv_tocancel';

const INVITATION = {
	id: INV_ID,
	org_id: 'org_test',
	email: 'pending@example.com',
	role: 'member',
	expires_at: '2026-04-24T00:00:00Z',
	created_at: '2026-04-17T00:00:00Z',
	accepted_at: null,
	revoked_at: null
} as Invitation;

const MEMBER = { user_id: 'u1', role: 'owner', joined_at: '2026-01-01T00:00:00Z' } as Member;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		members: [MEMBER],
		invitations: [INVITATION],
		error: null,
		...overrides
	};
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('members page', () => {
	it('renders the members and invitations tables', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('members-table')).toBeVisible();
		expect(screen.getByTestId('invitations-table')).toBeVisible();
		expect(screen.getByTestId('member-row-u1')).toBeVisible();
		expect(screen.getByTestId(`invitation-row-${INV_ID}`)).toBeVisible();
		expect(screen.getByTestId('invite-button')).toBeVisible();
	});

	it('sends an invitation and reloads, surfacing the new row', async () => {
		const user = userEvent.setup();
		create.mockResolvedValue({});
		const newInv = { ...INVITATION, id: 'inv_test123', email: 'newuser@example.com' };

		const { rerender } = render(Page, { props: { data: makeData({ invitations: [] }) } });

		await user.click(screen.getByTestId('invite-button'));
		expect(screen.getByTestId('invite-modal')).toBeVisible();

		await user.type(screen.getByTestId('invite-modal-email'), 'newuser@example.com');
		await user.click(screen.getByTestId('invite-modal-submit'));

		await waitFor(() =>
			expect(create).toHaveBeenCalledWith('org_test', {
				email: 'newuser@example.com',
				role: 'member'
			})
		);
		await waitFor(() => expect(screen.queryByTestId('invite-modal')).not.toBeInTheDocument());
		expect(invalidateAll).toHaveBeenCalledTimes(1);

		await rerender({ data: makeData({ invitations: [newInv] }) });
		expect(screen.getByTestId('invitation-row-inv_test123')).toBeVisible();
	});

	it('carries the selected role on the invitation', async () => {
		const user = userEvent.setup();
		create.mockResolvedValue({});

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('invite-button'));
		await user.type(screen.getByTestId('invite-modal-email'), 'admin@example.com');
		await user.selectOptions(screen.getByTestId('invite-modal-role'), 'admin');
		await user.click(screen.getByTestId('invite-modal-submit'));

		await waitFor(() =>
			expect(create).toHaveBeenCalledWith('org_test', {
				email: 'admin@example.com',
				role: 'admin'
			})
		);
	});

	it('rejects a malformed email client-side', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('invite-button'));
		await user.type(screen.getByTestId('invite-modal-email'), 'not-an-email');
		await user.click(screen.getByTestId('invite-modal-submit'));

		expect(screen.getByTestId('invite-modal-error')).toHaveTextContent(
			'Enter a valid email address.'
		);
		expect(create).not.toHaveBeenCalled();
	});

	it('keeps the modal open and shows the reason when the invite is rejected', async () => {
		const user = userEvent.setup();
		create.mockRejectedValue(new Error('seat limit reached'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('invite-button'));
		await user.type(screen.getByTestId('invite-modal-email'), 'newuser@example.com');
		await user.click(screen.getByTestId('invite-modal-submit'));

		await waitFor(() =>
			expect(screen.getByTestId('invite-modal-error')).toHaveTextContent('seat limit reached')
		);
		expect(screen.getByTestId('invite-modal')).toBeVisible();
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('revokes an invitation only after the inline confirmation', async () => {
		const user = userEvent.setup();
		revoke.mockResolvedValue(undefined);

		const { rerender } = render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`invitation-cancel-${INV_ID}`));
		expect(screen.getByTestId('cancel-confirm-banner')).toBeVisible();
		expect(revoke).not.toHaveBeenCalled();

		await user.click(screen.getByTestId('cancel-confirm-yes'));

		await waitFor(() => expect(revoke).toHaveBeenCalledWith('org_test', INV_ID));
		expect(invalidateAll).toHaveBeenCalledTimes(1);

		await rerender({ data: makeData({ invitations: [] }) });
		expect(screen.queryByTestId(`invitation-row-${INV_ID}`)).not.toBeInTheDocument();
	});

	it('keeps the invitation when the confirmation is dismissed', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`invitation-cancel-${INV_ID}`));
		await user.click(screen.getByTestId('cancel-confirm-no'));

		expect(screen.queryByTestId('cancel-confirm-banner')).not.toBeInTheDocument();
		expect(revoke).not.toHaveBeenCalled();
		expect(screen.getByTestId(`invitation-row-${INV_ID}`)).toBeVisible();
	});

	it('shows an inline banner when the revoke fails', async () => {
		const user = userEvent.setup();
		revoke.mockRejectedValue(new Error('revoke refused'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`invitation-cancel-${INV_ID}`));
		await user.click(screen.getByTestId('cancel-confirm-yes'));

		await waitFor(() =>
			expect(screen.getByTestId('cancel-error-banner')).toHaveTextContent('revoke refused')
		);
	});

	it('shows an error state instead of the tables when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load members.' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('Failed to load members.');
		expect(screen.queryByTestId('members-table')).not.toBeInTheDocument();
		expect(screen.queryByTestId('invitations-table')).not.toBeInTheDocument();
	});
});
