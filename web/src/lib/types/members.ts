import type { OrgRole } from './organization';

export interface Member {
	user_id: string;
	role: OrgRole;
	/** Populated when the member is assigned a custom role (M5-006/M5-007). */
	role_id?: string;
	joined_at: string;
}

export interface ListMembersResponse {
	members: Member[];
}
