// See https://svelte.dev/docs/kit/types#app.d.ts
declare global {
	namespace App {
		// interface Error {}
		// interface Locals {}
		// interface PageData {}
		// interface PageState {}
		// interface Platform {}
	}
}

// mdsvex compiles `.svx` files to Svelte components with frontmatter metadata.
declare module '*.svx' {
	import type { ComponentType } from 'svelte';
	const component: ComponentType;
	export default component;
	export const metadata: Record<string, unknown>;
}

export {};
