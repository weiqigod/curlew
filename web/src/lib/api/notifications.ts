import { api, type RequestOptions } from './client';
import type {
	NotificationRule,
	NotificationDelivery,
	CreateRuleRequest
} from '$lib/types/notifications';

interface ListRulesResponse {
	rules: NotificationRule[];
}

interface ListDeliveriesResponse {
	deliveries: NotificationDelivery[];
}

/** Typed API client for notification rules and deliveries. */
export const notificationsApi = {
	/**
	 * Lists notification rules for the given organization.
	 * Any member may call this.
	 */
	async listRules(orgId: string, opts?: RequestOptions): Promise<NotificationRule[]> {
		const { rules } = await api.get<ListRulesResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules`,
			opts
		);
		return rules;
	},

	/**
	 * Creates a notification rule for the given organization.
	 * Requires Owner or Admin role.
	 */
	async createRule(
		orgId: string,
		body: CreateRuleRequest,
		opts?: RequestOptions
	): Promise<NotificationRule> {
		return api.post<NotificationRule>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules`,
			body,
			opts
		);
	},

	/**
	 * Deletes a notification rule.
	 * Requires Owner or Admin role.
	 */
	async deleteRule(orgId: string, ruleId: string, opts?: RequestOptions): Promise<void> {
		await api.delete<void>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/notification-rules/${encodeURIComponent(ruleId)}`,
			opts
		);
	},

	/**
	 * Lists notification deliveries for the given organization, newest first.
	 * Any member may call this.
	 */
	async listDeliveries(
		orgId: string,
		opts?: RequestOptions & { limit?: number }
	): Promise<NotificationDelivery[]> {
		const { limit = 25, ...rest } = opts ?? {};
		const { deliveries } = await api.get<ListDeliveriesResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/notification-deliveries`,
			{ ...rest, params: { ...rest.params, limit } }
		);
		return deliveries;
	}
};
