import { createHighlighter, type Highlighter } from 'shiki';

// Build-time Shiki highlighting for code samples rendered on .svelte routes
// (mdsvex handles `.svx` scenario bodies separately). Server-only, so Shiki
// never ships to the client. Dual themes flip via CSS variables under `.dark`.
const themes = { light: 'github-light', dark: 'github-dark-dimmed' };
const langs = ['yaml', 'bash', 'json', 'graphql'];

let highlighterPromise: Promise<Highlighter> | undefined;
function getHighlighter() {
	if (!highlighterPromise) {
		highlighterPromise = createHighlighter({ themes: Object.values(themes), langs });
	}
	return highlighterPromise;
}

export async function highlight(code: string, lang = 'text'): Promise<string> {
	const hl = await getHighlighter();
	const safeLang = langs.includes(lang) ? lang : 'text';
	const html = hl.codeToHtml(code, { lang: safeLang, themes });
	return html.replace('<pre class="', `<pre data-lang="${safeLang}" class="`);
}
