import { sveltekit } from '@sveltejs/kit/vite';
import { defineConfig } from 'vite';

export default defineConfig({
	plugins: [sveltekit()],
	test: {
		include: ['src/**/*.{test,spec}.{js,ts}'],
		environment: 'jsdom',
		globals: true,
		setupFiles: ['./vitest-setup.ts']
	},
	// Component tests render Svelte client-side under jsdom. Without the browser
	// condition Vite resolves svelte's server entry and @testing-library/svelte's
	// `render` gets an SSR-only component with no mount().
	resolve: process.env.VITEST ? { conditions: ['browser'] } : undefined
});
