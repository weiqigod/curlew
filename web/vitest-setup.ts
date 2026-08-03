import '@testing-library/jest-dom/vitest';
import { cleanup } from '@testing-library/svelte';
import { afterEach } from 'vitest';

// @testing-library/svelte auto-cleans only when a global afterEach exists at
// import time; register it explicitly so component trees from one test never
// leak into the next one's queries.
afterEach(() => {
	cleanup();
});
