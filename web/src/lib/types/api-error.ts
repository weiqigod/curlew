/** Represents a typed error returned by the ApiTool backend API. */
export class ApiError extends Error {
	/** Machine-readable error code from the backend (e.g. "permission_denied"). */
	readonly code: string;
	/** HTTP status code of the response that caused this error. */
	readonly status: number;
	/**
	 * Optional field name identifying the offending input field (e.g. "idp_metadata_url").
	 * Present only when the backend returns a field-level validation error.
	 */
	readonly field?: string;
	/**
	 * Optional map of non-standard top-level fields from the error response body.
	 * For example, a 409 role_in_use response includes `{ member_count: N }`.
	 */
	readonly details?: Record<string, unknown>;
	/**
	 * Optional RFC 7807 problem type URI from the `type` field of a problem-detail body.
	 * Use `endsWith('/slug')` to match specific problem types without coupling to the full URI.
	 * For example: `err.problemType?.endsWith('/password-too-weak')`.
	 */
	readonly problemType?: string;

	constructor(
		code: string,
		message: string,
		status: number,
		field?: string,
		details?: Record<string, unknown>,
		problemType?: string
	) {
		super(message);
		this.name = 'ApiError';
		this.code = code;
		this.status = status;
		this.field = field;
		this.details = details;
		this.problemType = problemType;
	}
}

/** Type guard: returns true if `value` is an `ApiError` instance. */
export function isApiError(value: unknown): value is ApiError {
	return value instanceof ApiError;
}
