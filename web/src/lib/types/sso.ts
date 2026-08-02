/** Public view of an organisation's SSO configuration (secrets omitted). */
export interface SsoConfigView {
	sso_enabled: boolean;
	sso_provider: 'saml' | 'oidc' | null;
	saml_config: SamlConfigView | null;
	oidc_config: OidcConfigView | null;
}

/** Public-only SAML config view — no IdP cert. */
export interface SamlConfigView {
	idp_metadata_url: string;
	acs_url: string;
	entity_id: string;
	idp_sso_url: string | null;
}

/** Public-only OIDC config view — no client_secret. */
export interface OidcConfigView {
	issuer_url: string;
	client_id: string;
	redirect_uri: string;
	scopes: string;
}

/** Request body for PUT /organizations/{id}/sso/saml. */
export interface SamlUpsertRequest {
	idp_metadata_url: string;
	acs_url: string;
	entity_id: string;
	idp_sso_url?: string;
	idp_cert_pem?: string;
}

/** Request body for PUT /organizations/{id}/sso/oidc. */
export interface OidcUpsertRequest {
	issuer_url: string;
	client_id: string;
	client_secret: string;
	redirect_uri?: string;
	scopes?: string;
}

/** Response shape of a successful SSO config upsert — org fields plus SSO status. */
export interface SsoOrganizationResponse {
	id: string;
	name: string;
	slug: string;
	role: 'owner' | 'admin' | 'member';
	seat_count: number;
	seat_limit: number;
	status: string;
	created_at: string;
	sso_enabled: boolean;
	sso_provider: 'saml' | 'oidc';
}
