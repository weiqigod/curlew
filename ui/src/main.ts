// Boot sequence (UI_SPECIFICATION.md §10.4.3).
// Step 1 — token strip: move ?token=… into sessionStorage and rewrite the URL
// with history.replaceState, preserving the deep-link hash.
// Steps 3–5 — fetch /meta, /tree, /environments in parallel, open the WS, and
// resolve the route (controller.boot). No token at all → Disconnected screen.

import './styles/fonts.css';
import './styles/tokens.css';
import App from './App.svelte';
import { getToken, setToken } from './lib/api/client';
import { boot } from './lib/controller';
import { disconnected } from './lib/stores/connection';

const params = new URLSearchParams(location.search);
const token = params.get('token');
if (token !== null) {
  setToken(token);
  params.delete('token');
  const qs = params.toString();
  history.replaceState(null, '', location.pathname + (qs === '' ? '' : `?${qs}`) + location.hash);
}

if (getToken() === null) {
  // No token in the URL or sessionStorage — there is no login form (§10.4.3).
  disconnected.set(true);
} else {
  void boot();
}

const app = new App({
  target: document.getElementById('app') as HTMLElement,
});

export default app;
