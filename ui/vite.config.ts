import { defineConfig } from 'vitest/config';
import { svelte } from '@sveltejs/vite-plugin-svelte';

// Plain Vite SPA (no SvelteKit) per UI_SPECIFICATION.md §10.1.
// Build output is embedded into the Go binary via internal/uiserver/assets.
export default defineConfig({
  plugins: [svelte()],
  base: './',
  build: {
    outDir: '../internal/uiserver/assets/dist',
    emptyOutDir: true,
  },
  server: {
    proxy: {
      // Dev workflow (§10.2): `apitest ui --no-open` on :8765, Vite on :5173.
      '/api': {
        target: 'http://127.0.0.1:8765',
        ws: true,
      },
    },
  },
  test: {
    environment: 'jsdom',
    include: ['src/**/*.test.ts'],
  },
});
