export type InvitationRole = 'admin' | 'member';

export interface Invitation {
	id: string;
	org_id: string;
	email: string;
	role: InvitationRole;
	expires_at: string;
	created_at: string;
	accepted_at: string | null;
	revoked_at: string | null;
}

export interface CreateInvitationRequest {
	email: string;
	role: InvitationRole;
}

export interface CreateInvitationResponse {
	invitation: Invitation;
	token: string;
}

export interface ListInvitationsResponse {
	invitations: Invitation[];
}
