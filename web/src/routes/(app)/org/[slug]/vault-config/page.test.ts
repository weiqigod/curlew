import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { AuditLogEntry } from '$lib/types/audit-log';
import type { VaultConfigResponse } from '$lib/types/vault-config';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const put = vi.fn();
const remove = vi.fn();
vi.mock('$lib/api/vault-config', () => ({
	vaultConfigApi: {
		put: (...a: unknown[]) => put(...a),
		remove: (...a: unknown[]) => remove(...a)
	}
}));

const SPEC_EXAMPLE_YAML =
	'team_secrets:\n  provider: aws-secrets-manager\n  keys:\n    api_key: prod/api-key\n';

const CONFIG: VaultConfigResponse = {
	template: SPEC_EXAMPLE_YAML,
	version: 1,
	updated_at: '2026-05-12T10:00:00Z',
	updated_by_email: 'owner@test.com',
	warnings: null
} as VaultConfigResponse;

const AUDIT_ENTRY = {
	event_type: 'vault_config.upserted',
	user_id: 'user-123',
	user_email: 'owner@test.com',
	target_type: null,
	target_id: null,
	created_at: '2026-05-12T10:00:00Z',
	ip_address: null,
	success: true,
	failure_reason: null
} as AuditLogEntry;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_vaulttest', tier: 'team' }),
		currentConfig: CONFIG,
		auditEntries: [AUDIT_ENTRY],
		...overrides
	};
}

function editor() {
	return screen.getByTestId('vault-editor') as HTMLTextAreaElement;
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('vault-config page', () => {
	it('pre-fills the editor with the stored template', () => {
		render(Page, { props: { data: makeData() } });

		expect(editor()).toHaveValue(SPEC_EXAMPLE_YAML);
	});

	it('shows an empty editor when the org has no template yet', () => {
		render(Page, { props: { data: makeData({ currentConfig: null }) } });

		expect(editor()).toBeVisible();
		expect(editor()).toHaveValue('');
	});

	it('saves the edited YAML and reports the new version', async () => {
		const user = userEvent.setup();
		put.mockResolvedValue(CONFIG);

		render(Page, { props: { data: makeData({ currentConfig: null }) } });

		await user.type(editor(), 'team_secrets:\n');
		await user.click(screen.getByTestId('vault-save-button'));

		await waitFor(() => expect(put).toHaveBeenCalledTimes(1));
		expect(put).toHaveBeenCalledWith('org_vaulttest', 'team_secrets:\n');
		expect(await screen.findByText('Updated to version 1')).toBeVisible();
		// The canonical template returned by the backend replaces the local text.
		expect(editor()).toHaveValue(SPEC_EXAMPLE_YAML);
	});

	it('folds backend warnings into the success toast', async () => {
		const user = userEvent.setup();
		put.mockResolvedValue({ ...CONFIG, version: 2, warnings: ['unknown provider'] });

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('vault-save-button'));

		expect(
			await screen.findByText('Updated to version 2 (warnings: unknown provider)')
		).toBeVisible();
	});

	it('lists the offending paths from a 422 suspicious-value response', async () => {
		const user = userEvent.setup();
		put.mockRejectedValue(
			new ApiError('vault_template_suspicious_value', 'Template contains likely-secret values', 422, undefined, {
				offending_paths: ['team_secrets.password']
			})
		);

		render(Page, { props: { data: makeData({ currentConfig: null }) } });

		await user.type(editor(), 'team_secrets:\n');
		await user.click(screen.getByTestId('vault-save-button'));

		const warning = await screen.findByTestId('vault-suspicious-warning');
		expect(warning).toBeVisible();
		expect(warning).toHaveTextContent('team_secrets.password');
	});

	it('clears a previous suspicious-value warning on the next save', async () => {
		const user = userEvent.setup();
		put.mockRejectedValueOnce(
			new ApiError('vault_template_suspicious_value', 'nope', 422, undefined, {
				offending_paths: ['team_secrets.password']
			})
		).mockResolvedValueOnce(CONFIG);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('vault-save-button'));
		expect(await screen.findByTestId('vault-suspicious-warning')).toBeVisible();

		await user.click(screen.getByTestId('vault-save-button'));
		await waitFor(() =>
			expect(screen.queryByTestId('vault-suspicious-warning')).not.toBeInTheDocument()
		);
	});

	it('reports a non-422 save failure as an error toast', async () => {
		const user = userEvent.setup();
		put.mockRejectedValue(new ApiError('server_error', 'save blew up', 500));

		render(Page, { props: { data: makeData() } });
		await user.click(screen.getByTestId('vault-save-button'));

		expect(await screen.findByText('save blew up')).toBeVisible();
		expect(screen.queryByTestId('vault-suspicious-warning')).not.toBeInTheDocument();
	});

	it('deletes only after the inline confirmation, then empties the editor', async () => {
		const user = userEvent.setup();
		remove.mockResolvedValue(undefined);

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('vault-delete-button'));
		expect(screen.getByTestId('vault-delete-confirm')).toBeVisible();
		expect(remove).not.toHaveBeenCalled();

		await user.click(screen.getByTestId('vault-delete-confirm'));

		await waitFor(() => expect(remove).toHaveBeenCalledWith('org_vaulttest'));
		await waitFor(() => expect(editor()).toHaveValue(''));
		expect(await screen.findByText('Vault configuration deleted')).toBeVisible();
		expect(screen.queryByTestId('vault-delete-confirm')).not.toBeInTheDocument();
	});

	it('keeps the template when the delete request fails', async () => {
		const user = userEvent.setup();
		remove.mockRejectedValue(new Error('delete refused'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('vault-delete-button'));
		await user.click(screen.getByTestId('vault-delete-confirm'));

		expect(await screen.findByText('delete refused')).toBeVisible();
		expect(editor()).toHaveValue(SPEC_EXAMPLE_YAML);
	});

	it('toggles the CLI snippet panel', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		// NOTE: `curlew license` no longer exists — the CLI's licensing system was
		// removed. This asserts today's (stale) copy so the test fails loudly when
		// the snippet is corrected. See the follow-up task on this route.
		expect(screen.queryByText('curlew license --refresh')).not.toBeInTheDocument();

		await user.click(screen.getByTestId('vault-cli-snippet-button'));
		expect(screen.getByText('curlew license --refresh')).toBeVisible();

		await user.click(screen.getByTestId('vault-cli-snippet-button'));
		expect(screen.queryByText('curlew license --refresh')).not.toBeInTheDocument();
	});

	it('lists vault_config audit entries with actor and event type', () => {
		render(Page, { props: { data: makeData() } });

		const log = screen.getByTestId('vault-audit-log');
		expect(log).toBeVisible();
		expect(log).toHaveTextContent('vault_config.upserted');
		expect(log).toHaveTextContent('owner@test.com');
	});

	it('falls back to the user id, then a dash, when the actor email is absent', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						auditEntries: [
							{ ...AUDIT_ENTRY, user_email: null },
							{ ...AUDIT_ENTRY, user_email: null, user_id: null }
						]
					})
				}
			}
		);

		const rows = screen.getByTestId('vault-audit-log').querySelectorAll('tbody tr');
		expect(rows[0]).toHaveTextContent('user-123');
		expect(rows[1]).toHaveTextContent('—');
	});

	it('shows a placeholder when there are no vault_config events', () => {
		render(Page, { props: { data: makeData({ auditEntries: [] }) } });

		expect(screen.getByTestId('vault-audit-log')).toHaveTextContent(
			'No vault configuration events yet.'
		);
	});
});
