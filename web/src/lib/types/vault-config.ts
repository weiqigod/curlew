/** Response from GET/PUT /api/v1/organizations/{orgId}/vault-config. */
export interface VaultConfigResponse {
	/** Raw YAML source, preserving comments and ordering. */
	template: string;
	/** Monotonic per-org version number. */
	version: number;
	/** UTC timestamp of the last modification. */
	updated_at: string;
	/** Email of the actor who last modified the config, if available. */
	updated_by_email: string | null;
	/** Warning paths when the validator ran in warn mode. */
	warnings?: string[] | null;
}

/** PUT body — raw YAML sent as text/yaml. The API client sends the string as-is. */
export type VaultConfigPutBody = string;
