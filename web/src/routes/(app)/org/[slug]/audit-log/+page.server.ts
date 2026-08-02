import type { PageServerLoad } from './$types';
import { requireAuth, requireEnterpriseTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { auditLogApi } from '$lib/api/audit-log';
import { AUDIT_LOG_PAGE_SIZE, type AuditLogEntry } from '$lib/types/audit-log';

const FETCH_LIMIT = 100;

export const load: PageServerLoad = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname + url.search);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const entOrg = requireEnterpriseTier(org, params.slug);
	const adminOrg = requireOrgAdmin(entOrg, params.slug);

	const event_type = url.searchParams.get('event_type') ?? null;
	const from = url.searchParams.get('from') ?? null;
	const to = url.searchParams.get('to') ?? null;
	const page = Math.max(1, Number(url.searchParams.get('page') ?? '1') || 1);

	try {
		const all = await auditLogApi.list(adminOrg.id, {
			token,
			fetch,
			limit: FETCH_LIMIT,
			event_type: event_type ?? undefined,
			from: from ?? undefined,
			to: to ?? undefined
		});
		const totalPages = Math.max(1, Math.ceil(all.length / AUDIT_LOG_PAGE_SIZE));
		const clamped = Math.min(page, totalPages);
		const start = (clamped - 1) * AUDIT_LOG_PAGE_SIZE;
		const pageRows: AuditLogEntry[] = all.slice(start, start + AUDIT_LOG_PAGE_SIZE);
		return {
			org: adminOrg,
			entries: pageRows,
			totalCount: all.length,
			page: clamped,
			totalPages,
			filter: { event_type, from, to },
			error: null as string | null
		};
	} catch (e) {
		const message = e instanceof Error ? e.message : 'Failed to load audit log.';
		return {
			org: adminOrg,
			entries: [] as AuditLogEntry[],
			totalCount: 0,
			page: 1,
			totalPages: 1,
			filter: { event_type, from, to },
			error: message
		};
	}
};
