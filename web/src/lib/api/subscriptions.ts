import { api, type RequestOptions } from './client';
import type {
	GetSubscriptionResponse,
	UpdateSubscriptionRequest,
	UpdateSubscriptionResponse,
	PortalRequest,
	PortalResponse
} from '$lib/types/subscriptions';

/** Typed client for /api/v1/subscriptions endpoints. */
export const subscriptionsApi = {
	/**
	 * Fetches the subscription for an org (owner or admin).
	 * Passes org_id as a query parameter.
	 */
	async get(orgId: string, opts?: RequestOptions): Promise<GetSubscriptionResponse> {
		return api.get<GetSubscriptionResponse>('/api/v1/subscriptions', {
			...opts,
			params: { ...opts?.params, org_id: orgId }
		});
	},

	/**
	 * Updates a subscription (owner only).
	 * Returns both the updated subscription and proration breakdown.
	 */
	async update(
		id: string,
		body: UpdateSubscriptionRequest,
		opts?: RequestOptions
	): Promise<UpdateSubscriptionResponse> {
		return api.patch<UpdateSubscriptionResponse>(
			`/api/v1/subscriptions/${encodeURIComponent(id)}`,
			body,
			opts
		);
	},

	/**
	 * Creates a Stripe customer-portal session URL (owner only).
	 * Returns a portal_url to redirect the user to.
	 */
	async portal(orgId: string, returnUrl: string, opts?: RequestOptions): Promise<PortalResponse> {
		const body: PortalRequest = { org_id: orgId, return_url: returnUrl };
		return api.post<PortalResponse>('/api/v1/subscriptions/portal', body, opts);
	}
};
