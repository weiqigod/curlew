// Svelte action: progressively enhance every Shiki <pre> inside `node` with a
// language label and a copy-to-clipboard button. Highlighting itself is done at
// build time by mdsvex + Shiki; this only adds the client-side affordance.

export function codeCopy(node: HTMLElement) {
	const timers = new Set<ReturnType<typeof setTimeout>>();

	function copyText(text: string): Promise<void> {
		if (navigator.clipboard?.writeText) {
			return navigator.clipboard.writeText(text);
		}
		// Fallback for older / insecure contexts.
		return new Promise((resolve, reject) => {
			try {
				const ta = document.createElement('textarea');
				ta.value = text;
				ta.style.position = 'fixed';
				ta.style.opacity = '0';
				document.body.appendChild(ta);
				ta.select();
				document.execCommand('copy');
				ta.remove();
				resolve();
			} catch (err) {
				reject(err);
			}
		});
	}

	function enhance() {
		const pres = node.querySelectorAll<HTMLPreElement>('pre.shiki:not([data-enhanced])');
		pres.forEach((pre) => {
			pre.setAttribute('data-enhanced', 'true');

			const wrap = document.createElement('div');
			wrap.className = 'code-wrap';
			pre.parentNode?.insertBefore(wrap, pre);
			wrap.appendChild(pre);

			const lang = pre.getAttribute('data-lang');
			if (lang && lang !== 'text') {
				const tag = document.createElement('span');
				tag.className = 'code-lang';
				tag.textContent = lang;
				wrap.appendChild(tag);
			}

			const btn = document.createElement('button');
			btn.type = 'button';
			btn.className = 'copy-btn';
			btn.setAttribute('aria-label', 'Copy to clipboard');
			btn.textContent = 'Copy';

			btn.addEventListener('click', async () => {
				try {
					await copyText(pre.textContent ?? '');
					btn.textContent = 'Copied';
					btn.classList.add('copied');
				} catch {
					btn.textContent = 'Failed';
				}
				const t = setTimeout(() => {
					btn.textContent = 'Copy';
					btn.classList.remove('copied');
				}, 1800);
				timers.add(t);
			});

			wrap.appendChild(btn);
		});
	}

	enhance();

	return {
		update: enhance,
		destroy() {
			timers.forEach(clearTimeout);
		}
	};
}
