import { api, type RequestOptions } from './client';
import type {
	Invitation,
	CreateInvitationRequest,
	CreateInvitationResponse,
	ListInvitationsResponse
} from '$lib/types/invitations';

/** Typed API client for organization invitation endpoints. */
export const invitationsApi = {
	/** Lists pending invitations for the given organization. Requires Admin or Owner role. */
	async list(orgId: string, opts?: RequestOptions): Promise<Invitation[]> {
		const { invitations } = await api.get<ListInvitationsResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/invitations`,
			opts
		);
		return invitations;
	},

	/** Creates a new invitation for the given organization. Requires Admin or Owner role. */
	async create(
		orgId: string,
		body: CreateInvitationRequest,
		opts?: RequestOptions
	): Promise<CreateInvitationResponse> {
		return api.post<CreateInvitationResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/invitations`,
			body,
			opts
		);
	},

	/** Revokes (cancels) a pending invitation. Requires Admin or Owner role. */
	async revoke(orgId: string, invitationId: string, opts?: RequestOptions): Promise<void> {
		await api.delete<void>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/invitations/${encodeURIComponent(invitationId)}`,
			opts
		);
	}
};
