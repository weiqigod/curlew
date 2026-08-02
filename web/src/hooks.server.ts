import type { Handle } from '@sveltejs/kit';

/** Server hook: extracts the access_token cookie and populates locals. */
export const handle: Handle = async ({ event, resolve }) => {
	const token = event.cookies.get('access_token');
	event.locals.accessToken = token ?? null;
	event.locals.user = token ? decodeJwtClaims(token) : null;
	return resolve(event);
};

/**
 * Lightly decodes the payload of a JWT to extract `sub` and `email` claims.
 * Does NOT verify the signature — verification happens at the backend.
 * Returns null if the token is malformed.
 */
function decodeJwtClaims(token: string): { sub: string; email: string } | null {
	try {
		const parts = token.split('.');
		if (parts.length !== 3) return null;
		const json = Buffer.from(parts[1], 'base64url').toString('utf8');
		const claims = JSON.parse(json) as { sub?: unknown; email?: unknown };
		if (typeof claims.sub !== 'string' || typeof claims.email !== 'string') return null;
		return { sub: claims.sub, email: claims.email };
	} catch {
		return null;
	}
}
