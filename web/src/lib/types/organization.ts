/** A user's role within an organization. */
export type OrgRole = 'owner' | 'admin' | 'member';

/** The subscription tier of an organization. */
export type OrgTier = 'free' | 'solo' | 'professional' | 'team' | 'enterprise';

/** An organization the authenticated user belongs to. */
export interface Organization {
	id: string;
	name: string;
	slug: string;
	role: OrgRole;
	seat_count: number;
	seat_limit: number;
	status: string;
	created_at: string; // ISO 8601 UTC
	tier: OrgTier;
}
