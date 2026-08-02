/** Subscription tier identifier (backend mirror). */
export type SubscriptionTier = 'solo' | 'professional' | 'team';

/** Lifecycle state of a subscription. */
export type SubscriptionStatus = 'active' | 'past_due' | 'canceled' | 'trialing' | 'incomplete';

/** Billing interval. */
export type BillingInterval = 'month' | 'year';

/** Wire model mirroring backend SubscriptionDto. */
export interface Subscription {
	id: string;
	org_id: string;
	tier: SubscriptionTier;
	status: SubscriptionStatus;
	interval: BillingInterval;
	seat_count: number;
	seat_limit: number;
	current_period_start: string; // ISO 8601 UTC
	current_period_end: string;
	cancel_at_period_end: boolean;
	created_at: string;
}

/** Proration breakdown (amounts in cents). */
export interface ProrationResult {
	credit: number;
	charge: number;
	net: number;
}

export interface GetSubscriptionResponse {
	subscription: Subscription | null;
	tier: string;
}

export interface UpdateSubscriptionRequest {
	tier?: SubscriptionTier;
	seat_count?: number;
	interval?: BillingInterval;
}

export interface UpdateSubscriptionResponse {
	subscription: Subscription;
	proration: ProrationResult;
}

export interface PortalRequest {
	org_id: string;
	return_url: string;
}

export interface PortalResponse {
	portal_url: string;
}
