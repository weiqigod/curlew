import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { GitLabInstallation } from '$lib/types/gitlab-integrations';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const create = vi.fn();
const remove = vi.fn();
vi.mock('$lib/api/gitlab-integrations', () => ({
	gitlabIntegrationsApi: {
		create: (...a: unknown[]) => create(...a),
		remove: (...a: unknown[]) => remove(...a)
	}
}));

const INST_ID = 'aaaaaaaa-1111-2222-3333-444444444444';

function installation(overrides: Partial<GitLabInstallation> = {}): GitLabInstallation {
	return {
		id: INST_ID,
		project_id: 42,
		project_path: 'group/project',
		gitlab_base_url: 'https://gitlab.com',
		created_at: '2026-05-10T10:00:00Z',
		access_token_revoked_at: null,
		last_status_post_at: null,
		...overrides
	} as GitLabInstallation;
}

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		installations: [installation()],
		loadError: null,
		...overrides
	};
}

/** Fills the modal's required fields and submits. */
async function connect(
	user: ReturnType<typeof userEvent.setup>,
	{ path = 'new/project', pat = 'glpat-testtoken' } = {}
) {
	await user.type(screen.getByTestId('gitlab-modal-path'), path);
	await user.type(screen.getByTestId('gitlab-modal-pat'), pat);
	await user.click(screen.getByTestId('gitlab-modal-submit'));
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('gitlab integrations page', () => {
	it('shows the empty state with a Connect button when nothing is connected', () => {
		render(Page, { props: { data: makeData({ installations: [] }) } });

		expect(screen.getByTestId('gitlab-integration-page')).toBeVisible();
		expect(screen.getByTestId('gitlab-empty-state')).toBeVisible();
		expect(screen.getByTestId('connect-gitlab-button')).toBeVisible();
		expect(screen.queryByTestId('gitlab-integrations-table')).not.toBeInTheDocument();
	});

	it('lists each installation with its project, instance and connected badge', () => {
		render(Page, { props: { data: makeData() } });

		const row = screen.getByTestId(`gitlab-row-${INST_ID}`);
		expect(row).toHaveTextContent('group/project');
		expect(row).toHaveTextContent('https://gitlab.com');
		expect(row).toHaveTextContent('connected');
		// Never posted yet.
		expect(row).toHaveTextContent('—');
	});

	it('renders the last status post as relative time', () => {
		const twoMinutesAgo = new Date(Date.now() - 2 * 60 * 1000).toISOString();
		render(
			Page,
			{ props: { data: makeData({ installations: [installation({ last_status_post_at: twoMinutesAgo })] }) } }
		);

		expect(screen.getByTestId(`gitlab-row-${INST_ID}`)).toHaveTextContent('2m ago');
	});

	it('creates an installation and appends it to the table', async () => {
		const user = userEvent.setup();
		const created = installation({ id: 'new-id', project_path: 'new/project' });
		create.mockResolvedValue(created);

		render(Page, { props: { data: makeData({ installations: [] }) } });

		await user.click(screen.getByTestId('connect-gitlab-button'));
		expect(screen.getByTestId('gitlab-modal')).toBeVisible();

		await connect(user);

		await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
		expect(create).toHaveBeenCalledWith('org_test', {
			project_path: 'new/project',
			access_token: 'glpat-testtoken',
			gitlab_base_url: 'https://gitlab.com',
			gitlab_ca_bundle: undefined
		});
		await waitFor(() => expect(screen.queryByTestId('gitlab-modal')).not.toBeInTheDocument());
		expect(screen.getByTestId('gitlab-integrations-table')).toBeVisible();
	});

	it('sends a self-managed base URL together with a CA bundle', async () => {
		const user = userEvent.setup();
		create.mockResolvedValue(installation());
		const pem = '-----BEGIN CERTIFICATE-----\nMIIFake\n-----END CERTIFICATE-----';

		render(Page, { props: { data: makeData({ installations: [] }) } });
		await user.click(screen.getByTestId('connect-gitlab-button'));

		const baseUrl = screen.getByTestId('gitlab-modal-base-url');
		await user.clear(baseUrl);
		await user.type(baseUrl, 'https://gitlab.example.com');

		await user.click(screen.getByText(/Advanced: Custom CA Bundle/));
		await user.type(screen.getByTestId('gitlab-modal-ca'), pem);

		await connect(user, { path: 'corp/project', pat: 'glpat-selfmanaged' });

		await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
		const body = (create.mock.calls[0] as [string, Record<string, unknown>])[1];
		expect(body.gitlab_base_url).toBe('https://gitlab.example.com');
		expect(body.gitlab_ca_bundle).toBe(pem);
	});

	it.each([
		['project_not_found', 'gitlab-modal-error-project_path', /not found/i],
		['pat_unauthorized', 'gitlab-modal-error-access_token', /rejected by GitLab/i],
		['insecure_base_url', 'gitlab-modal-error-gitlab_base_url', /https:\/\//i],
		['duplicate_integration', 'gitlab-modal-error-project_path', /already exists/i]
	])('pins the %s error to its field', async (code, testId, pattern) => {
		const user = userEvent.setup();
		create.mockRejectedValue(new ApiError(code, 'rejected', 400));

		render(Page, { props: { data: makeData({ installations: [] }) } });
		await user.click(screen.getByTestId('connect-gitlab-button'));
		await connect(user, { path: 'group/missing' });

		const err = await screen.findByTestId(testId);
		expect(err).toHaveTextContent(pattern);
		// The modal stays open for correction.
		expect(screen.getByTestId('gitlab-modal')).toBeVisible();
		expect(screen.getByTestId('gitlab-modal-submit')).toBeEnabled();
	});

	it('falls back to a general error for an unrecognised API code', async () => {
		const user = userEvent.setup();
		create.mockRejectedValue(new ApiError('server_error', 'gitlab unreachable', 500));

		render(Page, { props: { data: makeData({ installations: [] }) } });
		await user.click(screen.getByTestId('connect-gitlab-button'));
		await connect(user);

		expect(await screen.findByTestId('gitlab-modal-error-general')).toHaveTextContent(
			'gitlab unreachable'
		);
	});

	it('rejects a URL-encoded project path client-side', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData({ installations: [] }) } });

		await user.click(screen.getByTestId('connect-gitlab-button'));
		await connect(user, { path: 'group%2Fproject' });

		expect(screen.getByTestId('gitlab-modal-error-project_path')).toHaveTextContent(
			'without URL-encoding'
		);
		expect(create).not.toHaveBeenCalled();
	});

	it('requires both a project path and an access token', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData({ installations: [] }) } });

		await user.click(screen.getByTestId('connect-gitlab-button'));
		await user.click(screen.getByTestId('gitlab-modal-submit'));
		expect(screen.getByTestId('gitlab-modal-error-project_path')).toHaveTextContent(
			'Project path is required'
		);

		await user.type(screen.getByTestId('gitlab-modal-path'), 'group/project');
		await user.click(screen.getByTestId('gitlab-modal-submit'));
		expect(screen.getByTestId('gitlab-modal-error-access_token')).toHaveTextContent(
			'Access token is required.'
		);

		expect(create).not.toHaveBeenCalled();
	});

	it('flags a revoked token and offers a pre-filled re-paste flow', async () => {
		const user = userEvent.setup();
		render(
			Page,
			{
				props: {
					data: makeData({
						installations: [installation({ access_token_revoked_at: '2026-05-09T12:00:00Z' })]
					})
				}
			}
		);

		expect(screen.getByTestId('gitlab-token-revoked-badge')).toBeVisible();
		await user.click(screen.getByTestId('gitlab-repaste-pat-button'));

		// The project path is carried over so only the token must be re-entered.
		expect(screen.getByTestId('gitlab-modal-path')).toHaveValue('group/project');
	});

	it('replaces the old installation when a re-pasted token is accepted', async () => {
		const user = userEvent.setup();
		remove.mockResolvedValue(undefined);
		create.mockResolvedValue(installation({ id: 'fresh-id' }));

		render(
			Page,
			{
				props: {
					data: makeData({
						installations: [installation({ access_token_revoked_at: '2026-05-09T12:00:00Z' })]
					})
				}
			}
		);

		await user.click(screen.getByTestId('gitlab-repaste-pat-button'));
		await user.type(screen.getByTestId('gitlab-modal-pat'), 'glpat-fresh');
		await user.click(screen.getByTestId('gitlab-modal-submit'));

		// The stale row is removed before the replacement is created.
		await waitFor(() => expect(remove).toHaveBeenCalledWith('org_test', INST_ID));
		await waitFor(() => expect(create).toHaveBeenCalledTimes(1));
		await waitFor(() => expect(screen.getByTestId('gitlab-row-fresh-id')).toBeVisible());
		expect(screen.queryByTestId(`gitlab-row-${INST_ID}`)).not.toBeInTheDocument();
	});

	it('disconnects only after confirmation, then drops the row', async () => {
		const user = userEvent.setup();
		remove.mockResolvedValue(undefined);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('disconnect-gitlab-button'));
		expect(screen.getByTestId('disconnect-confirm-button')).toBeVisible();
		expect(remove).not.toHaveBeenCalled();

		await user.click(screen.getByTestId('disconnect-confirm-button'));

		await waitFor(() => expect(remove).toHaveBeenCalledWith('org_test', INST_ID));
		await waitFor(() =>
			expect(screen.queryByTestId(`gitlab-row-${INST_ID}`)).not.toBeInTheDocument()
		);
	});

	it('keeps the row when the disconnect confirmation is cancelled', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('disconnect-gitlab-button'));
		await user.click(screen.getByRole('button', { name: 'Cancel' }));

		expect(screen.getByTestId(`gitlab-row-${INST_ID}`)).toBeVisible();
		expect(remove).not.toHaveBeenCalled();
	});

	it('surfaces a failed disconnect and keeps the row', async () => {
		const user = userEvent.setup();
		remove.mockRejectedValue(new Error('disconnect refused'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('disconnect-gitlab-button'));
		await user.click(screen.getByTestId('disconnect-confirm-button'));

		expect(await screen.findByTestId('gitlab-load-error')).toHaveTextContent('disconnect refused');
		expect(screen.getByTestId(`gitlab-row-${INST_ID}`)).toBeVisible();
	});

	it('shows the load error and suppresses the empty state', () => {
		render(
			Page,
			{ props: { data: makeData({ installations: [], loadError: 'Failed to load integrations' }) } }
		);

		expect(screen.getByTestId('gitlab-load-error')).toHaveTextContent(
			'Failed to load integrations'
		);
		expect(screen.queryByTestId('gitlab-empty-state')).not.toBeInTheDocument();
	});
});
