/** Lifecycle status of a user data export request. */
export type UserExportStatus = 'queued' | 'building' | 'ready' | 'failed' | 'expired';

/** A per-user GDPR data export request, as returned by the backend API. */
export interface UserExportRequest {
	id: string;
	status: UserExportStatus;
	created_at: string; // ISO 8601 UTC
	ready_at?: string | null;
	expires_at?: string | null;
	signed_url?: string | null;
	failure_reason?: string | null;
}

/** Terminal states — the request won't change after reaching one of these. */
export const TERMINAL_STATUSES: ReadonlySet<UserExportStatus> = new Set([
	'ready',
	'failed',
	'expired',
]);
