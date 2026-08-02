// Connection state (UI_SPECIFICATION.md §10.3.1) — drives the WS banner and
// the Disconnected screen (§10.6.7).

import { writable } from 'svelte/store';
import { setUnauthorizedHandler } from '../api/client';
import type { WsStatus } from '../ws';

export const wsStatus = writable<WsStatus>('connecting');

/** False once the /meta probe fails after sustained WS failure. */
export const serverReachable = writable(true);

/**
 * Set on any 401 — the token is stale (server restarted). There is no login
 * form; the Disconnected screen tells the user to reopen the printed URL.
 */
export const disconnected = writable(false);

setUnauthorizedHandler(() => {
  disconnected.set(true);
});
