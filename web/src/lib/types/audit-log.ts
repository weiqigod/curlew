/** Known organization audit-log event types emitted by the M5-004 middleware. */
export type AuditEventType =
	| 'member.invited'
	| 'member.removed'
	| 'member.role_changed'
	| 'org.settings.updated'
	| 'sso.login'
	| 'sso.login_failed'
	| 'subscription.updated';

/** JSON entry returned by GET /organizations/{id}/audit-log. */
export interface AuditLogEntry {
	event_type: string;
	user_id: string | null;
	/**
	 * Email address of the actor at the time of the event, when known.
	 * Preferred over {@link user_id} for display because admins recognize
	 * emails over raw user GUIDs. May be `null` for anonymous failure rows
	 * (bogus SSO callbacks before any user is identified) or for legacy
	 * rows written before the column was added.
	 */
	user_email: string | null;
	target_type: string | null;
	target_id: string | null;
	created_at: string; // ISO 8601 UTC
	ip_address: string | null;
	success: boolean;
	failure_reason: string | null;
}

/** Loader-side filter parsed from URL search params. */
export interface AuditLogFilter {
	event_type: string | null;
	from: string | null; // ISO 8601
	to: string | null; // ISO 8601
	page: number; // 1-based
}

/** Preset date-range keys exposed by the UI picker. */
export type AuditRangePreset = '24h' | '7d' | '30d' | 'all';

/** Page size for client-side pagination of audit log rows. */
export const AUDIT_LOG_PAGE_SIZE = 10;

/** Event types shown in the filter dropdown, plus an 'all' sentinel. */
export const AUDIT_EVENT_TYPES: readonly AuditEventType[] = [
	'member.invited',
	'member.removed',
	'member.role_changed',
	'org.settings.updated',
	'sso.login',
	'sso.login_failed',
	'subscription.updated'
] as const;
