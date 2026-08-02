import { describe, it, expect, vi } from 'vitest';
import { notificationsApi } from './notifications';
import type { NotificationRule, NotificationDelivery } from '$lib/types/notifications';

function makeFetchOk(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: true,
		status,
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

function makeFetchError(status: number, code: string, description: string): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: false,
		status,
		statusText: description,
		json: () => Promise.resolve({ code, description })
	}) as unknown as typeof fetch;
}

function makeRule(overrides: Partial<NotificationRule> = {}): NotificationRule {
	return {
		id: 'nrule_abc123',
		channel: 'slack',
		target: 'https://hooks.slack.test/x',
		on: ['run_failed'],
		created_at: '2026-04-17T00:00:00Z',
		...overrides
	};
}

function makeDelivery(overrides: Partial<NotificationDelivery> = {}): NotificationDelivery {
	return {
		id: 'ndel_abc123',
		rule_id: 'nrule_abc123',
		channel: 'slack',
		status: 'failed',
		response_code: null,
		attempt_count: 1,
		error_message: 'connection refused',
		attempted_at: '2026-04-17T00:00:00Z',
		...overrides
	};
}

describe('notificationsApi.listRules', () => {
	it('returns rules array', async () => {
		const rule = makeRule();
		const fetchFn = makeFetchOk({ rules: [rule] });
		const result = await notificationsApi.listRules('org_123', { fetch: fetchFn });
		expect(result).toHaveLength(1);
		expect(result[0].id).toBe('nrule_abc123');
	});

	it('returns empty list on zero rules', async () => {
		const fetchFn = makeFetchOk({ rules: [] });
		const result = await notificationsApi.listRules('org_123', { fetch: fetchFn });
		expect(result).toHaveLength(0);
	});
});

describe('notificationsApi.createRule', () => {
	it('posts JSON body and returns created rule', async () => {
		const rule = makeRule();
		const fetchFn = makeFetchOk(rule, 201);
		const result = await notificationsApi.createRule(
			'org_123',
			{ channel: 'slack', target: 'https://hooks.slack.test/x', on: ['run_failed'] },
			{ fetch: fetchFn }
		);
		expect(result.id).toBe('nrule_abc123');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('POST');
		expect(JSON.parse(opts.body as string)).toMatchObject({ channel: 'slack' });
	});

	it('propagates ApiError on 400 invalid_target', async () => {
		const fetchFn = makeFetchError(400, 'invalid_target', 'Slack target must be an HTTPS URL.');
		await expect(
			notificationsApi.createRule(
				'org_123',
				{ channel: 'slack', target: 'http://bad.url', on: ['run_failed'] },
				{ fetch: fetchFn }
			)
		).rejects.toMatchObject({ code: 'invalid_target', status: 400 });
	});
});

describe('notificationsApi.deleteRule', () => {
	it('sends DELETE and resolves void on 204', async () => {
		const fetchFn = vi.fn().mockResolvedValue({
			ok: true,
			status: 204,
			json: () => Promise.resolve(null)
		}) as unknown as typeof fetch;
		await expect(
			notificationsApi.deleteRule('org_123', 'nrule_abc123', { fetch: fetchFn })
		).resolves.toBeUndefined();
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('DELETE');
	});

	it('throws ApiError on 404', async () => {
		const fetchFn = makeFetchError(404, 'not_found', 'Notification rule not found.');
		await expect(
			notificationsApi.deleteRule('org_123', 'nrule_abc123', { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'not_found', status: 404 });
	});
});

describe('notificationsApi.listDeliveries', () => {
	it('passes limit=25 by default', async () => {
		const fetchFn = makeFetchOk({ deliveries: [] });
		await notificationsApi.listDeliveries('org_123', { fetch: fetchFn });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const url = call[0] as string;
		expect(url).toContain('limit=25');
	});

	it('passes custom limit param', async () => {
		const delivery = makeDelivery();
		const fetchFn = makeFetchOk({ deliveries: [delivery] });
		const result = await notificationsApi.listDeliveries('org_123', { fetch: fetchFn, limit: 10 });
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const url = call[0] as string;
		expect(url).toContain('limit=10');
		expect(result).toHaveLength(1);
		expect(result[0].id).toBe('ndel_abc123');
	});
});
