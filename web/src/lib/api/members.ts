import { api, type RequestOptions } from './client';
import type { Member, ListMembersResponse } from '$lib/types/members';

/** Typed API client for organization members endpoints. */
export const membersApi = {
	/** Lists active members for the given organization. Requires Admin or Owner role. */
	async list(orgId: string, opts?: RequestOptions): Promise<Member[]> {
		const { members } = await api.get<ListMembersResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/members`,
			opts
		);
		return members;
	}
};
