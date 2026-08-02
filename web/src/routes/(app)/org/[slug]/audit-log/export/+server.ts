import type { RequestHandler } from './$types';
import { error } from '@sveltejs/kit';
import { requireAuth, requireEnterpriseTier, requireOrgAdmin } from '$lib/server/guards';
import { organizationsApi } from '$lib/api/organizations';
import { auditLogApi } from '$lib/api/audit-log';
import { formatAuditLogCsv } from '$lib/audit-log/csv';

export const GET: RequestHandler = async ({ locals, params, url, fetch }) => {
	const token = requireAuth(locals.accessToken, url.pathname);
	const org = await organizationsApi.findBySlug(params.slug, { token, fetch });
	const entOrg = requireEnterpriseTier(org, params.slug);
	const adminOrg = requireOrgAdmin(entOrg, params.slug);

	const event_type = url.searchParams.get('event_type') ?? undefined;
	const from = url.searchParams.get('from') ?? undefined;
	const to = url.searchParams.get('to') ?? undefined;

	let entries;
	try {
		entries = await auditLogApi.list(adminOrg.id, {
			token,
			fetch,
			limit: 200,
			event_type,
			from,
			to
		});
	} catch {
		throw error(502, 'Upstream audit-log fetch failed');
	}

	const csv = formatAuditLogCsv(entries);
	const stamp = new Date().toISOString().slice(0, 10).replace(/-/g, '');
	const filename = `audit-log-${params.slug}-${stamp}.csv`;
	return new Response(csv, {
		status: 200,
		headers: {
			'Content-Type': 'text/csv; charset=utf-8',
			'Content-Disposition': `attachment; filename="${filename}"`
		}
	});
};
