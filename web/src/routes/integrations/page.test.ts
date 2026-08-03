import { render, screen } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { describe, expect, it } from 'vitest';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const INSTALL_URL =
	'https://github.com/apps/curlew-checks-test/installations/new?state=abc';

/** Builds a GitHub App installation in the requested lifecycle state. */
function installation(
	opts: { claimed?: boolean; suspended?: boolean; repoCount?: number } = {}
) {
	const { claimed = true, suspended = false, repoCount = 3 } = opts;
	return {
		installation_id: 12345,
		account_login: 'curlew-checks-test',
		account_type: 'Organization',
		repo_set: Array.from({ length: repoCount }, (_, i) => ({
			owner: 'acme',
			name: `repo-${i}`,
			id: 1000 + i
		})),
		claimed_at: claimed ? '2026-05-01T00:00:00Z' : null,
		suspended_at: suspended ? '2026-05-04T00:00:00Z' : null
	};
}

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		org: makeOrg(),
		isAdmin: true,
		installUrl: INSTALL_URL,
		installation: null,
		stateError: null,
		showInstalledToast: false,
		...overrides
	};
}

describe('integrations page', () => {
	it('offers admins a Connect button pointing at the minted install URL', () => {
		render(Page, { props: { data: makeData() } });

		const btn = screen.getByTestId('github-connect-button');
		expect(btn).toBeVisible();
		expect(btn).toHaveAttribute('href', INSTALL_URL);
		expect(btn.getAttribute('rel')).toContain('noopener');
		// Help text explains the two install paths.
		expect(screen.getByTestId('github-install-help')).toBeVisible();
	});

	it('hides the Connect button when no install URL could be minted', () => {
		render(Page, { props: { data: makeData({ installUrl: null }) } });

		expect(screen.queryByTestId('github-connect-button')).not.toBeInTheDocument();
		expect(screen.queryByTestId('github-install-help')).not.toBeInTheDocument();
	});

	it('gives members a read-only card with an admin-only note', () => {
		render(Page, { props: { data: makeData({ isAdmin: false, installUrl: null }) } });

		expect(screen.getByTestId('github-integration-card')).toBeVisible();
		expect(screen.queryByTestId('github-connect-button')).not.toBeInTheDocument();
		expect(screen.getByTestId('github-admin-only-tooltip')).toHaveTextContent(
			'Only owners and admins can connect GitHub.'
		);
	});

	it('reports a connected install with its account and repo count', () => {
		render(Page, { props: { data: makeData({ installation: installation({ repoCount: 3 }) }) } });

		const status = screen.getByTestId('github-install-status');
		expect(status).toHaveTextContent('GitHub connected');
		expect(status).toHaveTextContent('curlew-checks-test');
		expect(status).toHaveTextContent('3 repos covered');
		expect(screen.queryByTestId('github-connect-button')).not.toBeInTheDocument();
	});

	it('badges a suspended install and steers away from reconnecting', () => {
		render(
			Page,
			{ props: { data: makeData({ installation: installation({ suspended: true }) }) } }
		);

		const badge = screen.getByTestId('github-suspended-badge');
		expect(badge).toHaveTextContent('Suspended on GitHub');
		expect(screen.getByTestId('github-suspended-description')).toHaveTextContent('suspended');
		// Reconnecting is the wrong remedy — the fix is on GitHub's side.
		expect(screen.queryByTestId('github-connect-button')).not.toBeInTheDocument();
		expect(screen.queryByTestId('github-install-status')).not.toBeInTheDocument();
	});

	it('offers admins a claim action for a webhook-first install', () => {
		render(Page, { props: { data: makeData({ installation: installation({ claimed: false }) }) } });

		expect(screen.getByTestId('github-claim-button')).toBeVisible();
		expect(screen.queryByTestId('github-connect-button')).not.toBeInTheDocument();
	});

	it('does not offer the claim action to members', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						installation: installation({ claimed: false }),
						isAdmin: false,
						installUrl: null
					})
				}
			}
		);

		expect(screen.queryByTestId('github-claim-button')).not.toBeInTheDocument();
		expect(screen.getByTestId('github-admin-only-tooltip')).toBeVisible();
	});

	it('shows a dismissable success toast after the install callback', async () => {
		const user = userEvent.setup();
		render(
			Page,
			{ props: { data: makeData({ showInstalledToast: true, installation: installation() }) } }
		);

		const toast = screen.getByTestId('toast-github-connected');
		expect(toast).toHaveTextContent('GitHub connected');

		await user.click(screen.getByRole('button', { name: 'Dismiss notification' }));
		expect(screen.queryByTestId('toast-github-connected')).not.toBeInTheDocument();
	});

	it('hides the toast when the callback flag is absent', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.queryByTestId('toast-github-connected')).not.toBeInTheDocument();
	});

	it('surfaces a state error alongside the card', () => {
		render(Page, { props: { data: makeData({ stateError: 'install state unavailable' }) } });

		expect(screen.getByText('install state unavailable')).toBeVisible();
		expect(screen.getByTestId('github-integration-card')).toBeVisible();
	});

	it('stays bounded to GitHub — no billing or license surface', () => {
		render(Page, { props: { data: makeData({ installation: installation() }) } });

		expect(screen.queryByTestId('subscription-card')).not.toBeInTheDocument();
		expect(screen.queryByTestId('license-display')).not.toBeInTheDocument();
	});
});
