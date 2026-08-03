// Registers @testing-library/jest-dom's matcher types (toBeVisible,
// toHaveTextContent, …) on vitest's Assertion interface for svelte-check.
// The runtime registration lives in vitest-setup.ts, which sits outside the
// tsconfig include list, so the augmentation has to be pulled in from src/.
import '@testing-library/jest-dom/vitest';
