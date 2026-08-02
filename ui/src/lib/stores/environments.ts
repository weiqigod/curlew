// /environments payload + per-session env selection (UI_SPECIFICATION.md §10.3.1).
// Environment selection is per-session (sessionStorage), defaulting to
// meta.project.default_env at boot (the boot sequence sets it when unset).

import { writable } from 'svelte/store';
import { getEnvironments } from '../api/environments';
import type { Environment } from '../types/tree';

const ENV_KEY = 'curlew.env';

export const environments = writable<Environment[]>([]);

function initialEnv(): string | null {
  try {
    return sessionStorage.getItem(ENV_KEY);
  } catch {
    return null;
  }
}

export const selectedEnv = writable<string | null>(initialEnv());

selectedEnv.subscribe((name) => {
  try {
    if (name === null) {
      sessionStorage.removeItem(ENV_KEY);
    } else {
      sessionStorage.setItem(ENV_KEY, name);
    }
  } catch {
    // sessionStorage unavailable — selection just won't survive reload
  }
});

export async function loadEnvironments(): Promise<void> {
  environments.set(await getEnvironments());
}
