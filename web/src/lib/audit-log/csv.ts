import type { AuditLogEntry } from '$lib/types/audit-log';

const CSV_HEADER = 'event_type,user,target,timestamp,ip';

/**
 * Formats a list of audit-log entries as CSV matching the M5-005 observable header.
 * Uses RFC 4180 quoting: wraps fields in double-quotes if they contain comma,
 * double-quote, newline, or carriage return; escapes internal quotes by doubling.
 * Fields starting with =, +, -, or @ are prefixed with a single quote to
 * neutralise spreadsheet formula injection.
 */
export function formatAuditLogCsv(entries: readonly AuditLogEntry[]): string {
	const lines = [CSV_HEADER];
	for (const e of entries) {
		const target = [e.target_type, e.target_id].filter(Boolean).join(':');
		lines.push(
			[
				quote(e.event_type),
				quote(e.user_id ?? ''),
				quote(target),
				quote(e.created_at),
				quote(e.ip_address ?? '')
			].join(',')
		);
	}
	return lines.join('\r\n') + '\r\n';
}

function quote(field: string): string {
	let f = field;
	if (f.length > 0 && (f[0] === '=' || f[0] === '+' || f[0] === '-' || f[0] === '@')) {
		f = "'" + f;
	}
	if (f.includes(',') || f.includes('"') || f.includes('\n') || f.includes('\r')) {
		return '"' + f.replace(/"/g, '""') + '"';
	}
	return f;
}
