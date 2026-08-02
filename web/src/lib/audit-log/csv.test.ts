import { describe, it, expect } from 'vitest';
import { formatAuditLogCsv } from './csv';
import type { AuditLogEntry } from '$lib/types/audit-log';

function entry(partial: Partial<AuditLogEntry> = {}): AuditLogEntry {
	return {
		event_type: 'member.invited',
		user_id: 'aaaa1111',
		user_email: null,
		target_type: 'invitation',
		target_id: 'inv_1',
		created_at: '2026-04-18T10:00:00Z',
		ip_address: '10.0.0.1',
		success: true,
		failure_reason: null,
		...partial
	};
}

describe('formatAuditLogCsv', () => {
	const cases = [
		{ name: 'header only when no entries', entries: [], expectLines: 1 },
		{ name: 'single entry produces header + one row', entries: [entry()], expectLines: 2 },
		{
			name: 'header matches observable spec',
			entries: [],
			expectHeader: 'event_type,user,target,timestamp,ip'
		},
		{
			name: 'combines target_type and target_id with colon',
			entries: [entry()],
			expectSubstring: 'invitation:inv_1'
		},
		{
			name: 'empty target when both null',
			entries: [entry({ target_type: null, target_id: null })],
			expectSubstring: ',,'
		},
		{
			name: 'quotes fields containing comma',
			entries: [entry({ ip_address: '1.2,3.4' })],
			expectSubstring: '"1.2,3.4"'
		},
		{
			name: 'escapes internal double quotes',
			entries: [entry({ event_type: 'a"b' })],
			expectSubstring: '"a""b"'
		},
		{
			name: 'neutralises formula-injection prefix',
			entries: [entry({ event_type: '=1+1' })],
			expectSubstring: "'=1+1"
		},
		{
			name: 'quotes fields with newlines',
			entries: [entry({ ip_address: 'a\nb' })],
			expectSubstring: '"a\nb"'
		},
		{
			name: 'quotes fields with carriage returns',
			entries: [entry({ ip_address: 'a\rb' })],
			expectSubstring: '"a\rb"'
		},
		{
			name: 'null user_id renders as empty column',
			entries: [entry({ user_id: null })],
			expectSubstring: 'member.invited,,invitation:inv_1'
		}
	];

	for (const c of cases) {
		it(c.name, () => {
			const csv = formatAuditLogCsv(c.entries);
			if (c.expectLines !== undefined) {
				expect(csv.trimEnd().split('\r\n')).toHaveLength(c.expectLines);
			}
			if (c.expectHeader) {
				expect(csv.split('\r\n')[0]).toBe(c.expectHeader);
			}
			if (c.expectSubstring) {
				expect(csv).toContain(c.expectSubstring);
			}
		});
	}
});
