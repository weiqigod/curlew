import { describe, it, expect, vi } from 'vitest';
import { subscriptionsApi } from './subscriptions';
import type { Subscription, ProrationResult } from '$lib/types/subscriptions';

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

function makeSub(overrides: Partial<Subscription> = {}): Subscription {
	return {
		id: 'sub_test',
		org_id: 'org_test',
		tier: 'team',
		status: 'active',
		interval: 'month',
		seat_count: 3,
		seat_limit: 10,
		current_period_start: '2026-04-01T00:00:00Z',
		current_period_end: '2026-05-01T00:00:00Z',
		cancel_at_period_end: false,
		created_at: '2026-04-01T00:00:00Z',
		...overrides
	};
}

function makeProration(overrides: Partial<ProrationResult> = {}): ProrationResult {
	return { credit: 0, charge: 5400, net: 5400, ...overrides };
}

describe('subscriptionsApi.get', () => {
	it('fetches subscription with org_id query param', async () => {
		const sub = makeSub();
		const fetchFn = makeFetchOk({ subscription: sub, tier: 'team' });
		const result = await subscriptionsApi.get('org_test', { fetch: fetchFn });
		expect(result.subscription).toMatchObject({ id: 'sub_test' });
		expect(result.tier).toBe('team');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const url = call[0] as string;
		expect(url).toContain('org_id=org_test');
	});

	it('returns { subscription: null, tier } when org has no subscription', async () => {
		const fetchFn = makeFetchOk({ subscription: null, tier: 'free' });
		const result = await subscriptionsApi.get('org_free', { fetch: fetchFn });
		expect(result.subscription).toBeNull();
		expect(result.tier).toBe('free');
	});

	it('propagates ApiError on 403 permission_denied', async () => {
		const fetchFn = makeFetchError(403, 'permission_denied', 'Forbidden');
		await expect(subscriptionsApi.get('org_test', { fetch: fetchFn })).rejects.toMatchObject({
			code: 'permission_denied',
			status: 403
		});
	});
});

describe('subscriptionsApi.update', () => {
	it('sends PATCH with body and returns subscription + proration', async () => {
		const sub = makeSub({ seat_count: 6 });
		const proration = makeProration();
		const fetchFn = makeFetchOk({ subscription: sub, proration });
		const result = await subscriptionsApi.update('sub_test', { seat_count: 6 }, { fetch: fetchFn });
		expect(result.subscription.seat_count).toBe(6);
		expect(result.proration.net).toBe(5400);
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('PATCH');
		expect(JSON.parse(opts.body as string)).toMatchObject({ seat_count: 6 });
	});

	it('propagates ApiError on 403 permission_denied', async () => {
		const fetchFn = makeFetchError(403, 'permission_denied', 'Forbidden');
		await expect(
			subscriptionsApi.update('sub_test', { seat_count: 6 }, { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'permission_denied', status: 403 });
	});

	it('propagates ApiError on 409 subscription_downgrade_blocked', async () => {
		const fetchFn = makeFetchError(409, 'subscription_downgrade_blocked', 'Cannot downgrade');
		await expect(
			subscriptionsApi.update('sub_test', { seat_count: 1 }, { fetch: fetchFn })
		).rejects.toMatchObject({ code: 'subscription_downgrade_blocked', status: 409 });
	});
});

describe('subscriptionsApi.portal', () => {
	it('POSTs { org_id, return_url } and returns { portal_url }', async () => {
		const fetchFn = makeFetchOk({ portal_url: 'https://billing.stripe.com/session/xyz' });
		const result = await subscriptionsApi.portal('org_test', 'https://app.example.com/org/acme/billing', {
			fetch: fetchFn
		});
		expect(result.portal_url).toBe('https://billing.stripe.com/session/xyz');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		const opts = call[1] as RequestInit;
		expect(opts.method).toBe('POST');
		expect(JSON.parse(opts.body as string)).toMatchObject({
			org_id: 'org_test',
			return_url: 'https://app.example.com/org/acme/billing'
		});
	});

	it('propagates ApiError on 403 permission_denied', async () => {
		const fetchFn = makeFetchError(403, 'permission_denied', 'Forbidden');
		await expect(
			subscriptionsApi.portal('org_test', 'https://app.example.com/org/acme/billing', {
				fetch: fetchFn
			})
		).rejects.toMatchObject({ code: 'permission_denied', status: 403 });
	});
});
