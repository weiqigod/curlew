import adapter from '@sveltejs/adapter-static';
import { vitePreprocess } from '@sveltejs/vite-plugin-svelte';
import { mdsvex, escapeSvelte } from 'mdsvex';
import { createHighlighter } from 'shiki';

// Dual light/dark themes. Shiki emits CSS variables; app.css flips them on `.dark`.
const themes = { light: 'github-light', dark: 'github-dark-dimmed' };
const langs = ['yaml', 'bash', 'json', 'graphql'];

// Lazily create a single Shiki highlighter, reused across every fenced block.
let highlighterPromise;
function getHighlighter() {
	if (!highlighterPromise) {
		highlighterPromise = createHighlighter({ themes: Object.values(themes), langs });
	}
	return highlighterPromise;
}

/** @type {import('mdsvex').MdsvexOptions} */
const mdsvexConfig = {
	extensions: ['.svx'],
	highlight: {
		async highlighter(code, lang = 'text') {
			const hl = await getHighlighter();
			const safeLang = langs.includes(lang) ? lang : 'text';
			const html = hl.codeToHtml(code, { lang: safeLang, themes });
			// Tag the <pre> with its language so the copy action can show a label.
			const tagged = html.replace('<pre class="', `<pre data-lang="${safeLang}" class="`);
			return `{@html \`${escapeSvelte(tagged)}\`}`;
		}
	}
};

/** @type {import('@sveltejs/kit').Config} */
const config = {
	extensions: ['.svelte', '.svx'],
	preprocess: [vitePreprocess(), mdsvex(mdsvexConfig)],
	kit: {
		adapter: adapter({
			pages: 'build',
			assets: 'build',
			fallback: undefined, // pure SSG — every page is prerendered, no SPA fallback
			precompress: true,
			strict: true // build fails if anything isn't prerenderable
		}),
		alias: {
			$lib: 'src/lib',
			$content: 'src/content'
		},
		prerender: {
			handleHttpError: 'fail' // dead internal links fail the build
		}
	}
};

export default config;
