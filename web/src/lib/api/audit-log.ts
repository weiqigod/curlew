import { api, type RequestOptions } from './client';
import type { AuditLogEntry } from '$lib/types/audit-log';

interface ListAuditLogResponse {
	items: AuditLogEntry[];
}

/** Typed API client for GET /organizations/{id}/audit-log. */
export const auditLogApi = {
	/**
	 * Lists audit-log entries for an organisation, newest-first.
	 * Admin/owner only — raises 403 permission_denied otherwise.
	 * Backend clamps limit to [1, 200].
	 */
	async list(
		orgId: string,
		opts: RequestOptions & {
			event_type?: string;
			from?: string;
			to?: string;
			limit?: number;
		} = {}
	): Promise<AuditLogEntry[]> {
		const { event_type, from, to, limit, ...rest } = opts;
		const params: Record<string, string | number> = {};
		if (event_type) params.event_type = event_type;
		if (from) params.from = from;
		if (to) params.to = to;
		if (limit !== undefined) params.limit = limit;

		const { items } = await api.get<ListAuditLogResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/audit-log`,
			{ ...rest, params: { ...rest.params, ...params } }
		);
		return items;
	}
};
