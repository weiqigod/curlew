/** An organization that blocks the user's account deletion (sole owner + other members). */
export interface BlockingOrg {
	slug: string;
	name: string;
}

/** Response from POST /api/v1/users/me/deletion-requests (202 Accepted). */
export interface DeletionRequestResponse {
	finalizes_at: string;       // ISO 8601 UTC
	cancellable_until: string;  // ISO 8601 UTC
	cancel_url: string;
}

/** Response from GET /api/v1/users/me/deletion-requests/status (200 OK). */
export interface UserDeletionStatus {
	pending_deletion_at?: string | null;  // ISO 8601 UTC
	finalizes_at?: string | null;         // ISO 8601 UTC
	anonymised_at?: string | null;        // ISO 8601 UTC
}

/** Response from POST /api/v1/auth/reauth (200 OK). */
export interface ReauthTokenResponse {
	reauth_token: string;  // drto_ prefix
}

/** Shape of the 409 OwnerCannotLeave error body. */
export interface OwnerCannotLeaveError {
	error_code: 'OwnerCannotLeave';
	code: 'owner_cannot_leave';
	blocking_orgs: BlockingOrg[];
}
