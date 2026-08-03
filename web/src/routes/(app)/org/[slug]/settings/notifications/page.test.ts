import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import type { NotificationRule, NotificationDelivery } from '$lib/types/notifications';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const invalidateAll = vi.fn();
vi.mock('$app/navigation', () => ({ invalidateAll: () => invalidateAll() }));

const createRule = vi.fn();
const deleteRule = vi.fn();
vi.mock('$lib/api/notifications', () => ({
	notificationsApi: {
		createRule: (...a: unknown[]) => createRule(...a),
		deleteRule: (...a: unknown[]) => deleteRule(...a)
	}
}));

const RULE_ID = 'nrule_aabbccddeeff00112233445566778899';

const RULE = {
	id: RULE_ID,
	channel: 'slack',
	target: 'https://hooks.slack.test/del',
	on: ['run_failed'],
	created_at: '2026-04-17T00:00:00Z'
} as NotificationRule;

const DELIVERIES = [
	{
		id: 'ndel_111',
		rule_id: 'nrule_aaa',
		channel: 'slack',
		status: 'delivered',
		response_code: 200,
		attempt_count: 1,
		error_message: null,
		attempted_at: '2026-04-17T10:00:00Z'
	},
	{
		id: 'ndel_222',
		rule_id: 'nrule_aaa',
		channel: 'slack',
		status: 'failed',
		response_code: null,
		attempt_count: 1,
		error_message: 'connection refused',
		attempted_at: '2026-04-17T09:00:00Z'
	}
] as NotificationDelivery[];

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'team' }),
		rules: [RULE],
		deliveries: DELIVERIES,
		error: null,
		...overrides
	};
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('notifications page', () => {
	it('renders the rules table and delivery log', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByRole('heading', { name: 'Notification Settings' })).toBeVisible();
		expect(screen.getByTestId('rules-table')).toBeVisible();
		expect(screen.getByTestId('delivery-log')).toBeVisible();
		expect(screen.getByTestId(`rule-row-${RULE_ID}`)).toBeVisible();
	});

	it('gives delivered and failed deliveries visually distinct badges', () => {
		render(Page, { props: { data: makeData() } });

		const delivered = screen.getByTestId('delivery-status-ndel_111');
		const failed = screen.getByTestId('delivery-status-ndel_222');

		expect(delivered).toHaveTextContent('delivered');
		expect(failed).toHaveTextContent('failed');
		expect(delivered.getAttribute('class')).not.toBe(failed.getAttribute('class'));
	});

	it('creates a slack rule through the modal and reloads', async () => {
		const user = userEvent.setup();
		createRule.mockResolvedValue({});

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		expect(screen.getByTestId('notif-modal')).toBeVisible();

		await user.selectOptions(screen.getByTestId('notif-modal-channel'), 'slack');
		await user.type(screen.getByTestId('notif-modal-target'), 'https://hooks.slack.test/e2e');
		await user.click(screen.getByTestId('notif-modal-on-run_failed'));
		await user.click(screen.getByTestId('notif-modal-submit'));

		await waitFor(() => expect(createRule).toHaveBeenCalledTimes(1));
		expect(createRule).toHaveBeenCalledWith('org_test', {
			channel: 'slack',
			target: 'https://hooks.slack.test/e2e',
			on: ['run_failed']
		});
		await waitFor(() => expect(screen.queryByTestId('notif-modal')).not.toBeInTheDocument());
		expect(invalidateAll).toHaveBeenCalledTimes(1);
	});

	it('blocks a non-https slack target client-side without calling the API', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		await user.selectOptions(screen.getByTestId('notif-modal-channel'), 'slack');
		await user.type(screen.getByTestId('notif-modal-target'), 'http://not-https.example.com/hook');
		await user.click(screen.getByTestId('notif-modal-on-run_failed'));
		await user.click(screen.getByTestId('notif-modal-submit'));

		expect(screen.getByTestId('notif-modal-error')).toHaveTextContent('https');
		expect(screen.getByTestId('notif-modal')).toBeVisible();
		expect(createRule).not.toHaveBeenCalled();
	});

	it('rejects a malformed email target client-side', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		await user.selectOptions(screen.getByTestId('notif-modal-channel'), 'email');
		await user.type(screen.getByTestId('notif-modal-target'), 'not-an-email');
		await user.click(screen.getByTestId('notif-modal-on-run_failed'));
		await user.click(screen.getByTestId('notif-modal-submit'));

		expect(screen.getByTestId('notif-modal-error')).toHaveTextContent(
			'Email target must be a valid email address.'
		);
		expect(createRule).not.toHaveBeenCalled();
	});

	it('requires at least one trigger event', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		await user.type(screen.getByTestId('notif-modal-target'), 'https://hooks.slack.test/e2e');
		await user.click(screen.getByTestId('notif-modal-submit'));

		expect(screen.getByTestId('notif-modal-error')).toHaveTextContent(
			'Select at least one event.'
		);
		expect(createRule).not.toHaveBeenCalled();
	});

	it('surfaces an API failure in the modal and keeps it open', async () => {
		const user = userEvent.setup();
		createRule.mockRejectedValue(new Error('backend refused'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		await user.type(screen.getByTestId('notif-modal-target'), 'https://hooks.slack.test/e2e');
		await user.click(screen.getByTestId('notif-modal-on-run_failed'));
		await user.click(screen.getByTestId('notif-modal-submit'));

		await waitFor(() =>
			expect(screen.getByTestId('notif-modal-error')).toHaveTextContent('backend refused')
		);
		expect(screen.getByTestId('notif-modal')).toBeVisible();
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('deletes a rule behind an inline confirmation banner', async () => {
		const user = userEvent.setup();
		deleteRule.mockResolvedValue(undefined);

		const { rerender } = render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`rule-delete-${RULE_ID}`));
		expect(screen.getByTestId('delete-confirm-banner')).toBeVisible();
		// Nothing is deleted until the banner is confirmed.
		expect(deleteRule).not.toHaveBeenCalled();

		await user.click(screen.getByTestId('delete-confirm-yes'));

		await waitFor(() => expect(deleteRule).toHaveBeenCalledWith('org_test', RULE_ID));
		expect(invalidateAll).toHaveBeenCalledTimes(1);

		// The re-load is what removes the row.
		await rerender({ data: makeData({ rules: [] }) });
		expect(screen.queryByTestId(`rule-row-${RULE_ID}`)).not.toBeInTheDocument();
	});

	it('cancels a pending delete without calling the API', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`rule-delete-${RULE_ID}`));
		await user.click(screen.getByTestId('delete-confirm-no'));

		expect(screen.queryByTestId('delete-confirm-banner')).not.toBeInTheDocument();
		expect(deleteRule).not.toHaveBeenCalled();
	});

	it('shows an inline banner when the delete request fails', async () => {
		const user = userEvent.setup();
		deleteRule.mockRejectedValue(new Error('delete blew up'));

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId(`rule-delete-${RULE_ID}`));
		await user.click(screen.getByTestId('delete-confirm-yes'));

		await waitFor(() =>
			expect(screen.getByTestId('delete-error-banner')).toHaveTextContent('delete blew up')
		);
	});

	it('labels the modal inputs and closes on Escape', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('add-rule-button'));
		expect(screen.getByLabelText('Channel')).toBeVisible();
		expect(screen.getByLabelText('Webhook URL')).toBeVisible();

		// The target label tracks the selected channel.
		await user.selectOptions(screen.getByTestId('notif-modal-channel'), 'email');
		expect(screen.getByLabelText('Email address')).toBeVisible();

		await user.keyboard('{Escape}');
		await waitFor(() => expect(screen.queryByTestId('notif-modal')).not.toBeInTheDocument());
	});

	it('shows an error state instead of the tables when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load notifications.' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent('Failed to load notifications.');
		expect(screen.queryByTestId('rules-table')).not.toBeInTheDocument();
		expect(screen.queryByTestId('delivery-log')).not.toBeInTheDocument();
	});

	it('shows a no-deliveries message when the log is empty', () => {
		render(Page, { props: { data: makeData({ deliveries: [] }) } });

		expect(screen.getByTestId('delivery-log')).toHaveTextContent('No deliveries yet.');
	});
});
