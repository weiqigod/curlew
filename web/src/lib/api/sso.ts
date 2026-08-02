import { api, type RequestOptions } from './client';
import type {
	SsoConfigView,
	SamlUpsertRequest,
	OidcUpsertRequest,
	SsoOrganizationResponse
} from '$lib/types/sso';

/** Typed API client for SSO configuration endpoints. */
export const ssoApi = {
	/**
	 * Reads the current SSO configuration for an organisation.
	 * Owner-only. Secrets (IdP cert, OIDC client_secret) are never returned.
	 */
	async get(orgId: string, opts?: RequestOptions): Promise<SsoConfigView> {
		return api.get<SsoConfigView>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/sso`,
			opts
		);
	},

	/**
	 * Creates or updates the SAML SSO configuration for an organisation.
	 * Owner-only.
	 */
	async upsertSaml(
		orgId: string,
		body: SamlUpsertRequest,
		opts?: RequestOptions
	): Promise<SsoOrganizationResponse> {
		return api.put<SsoOrganizationResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/sso/saml`,
			body,
			opts
		);
	},

	/**
	 * Creates or updates the OIDC SSO configuration for an organisation.
	 * Owner-only.
	 */
	async upsertOidc(
		orgId: string,
		body: OidcUpsertRequest,
		opts?: RequestOptions
	): Promise<SsoOrganizationResponse> {
		return api.put<SsoOrganizationResponse>(
			`/api/v1/organizations/${encodeURIComponent(orgId)}/sso/oidc`,
			body,
			opts
		);
	}
};
