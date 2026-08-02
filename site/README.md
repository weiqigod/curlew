# Curlew — Examples Cookbook (site)

A standalone, fully-static showcase site for Curlew: a filterable gallery of
sophisticated, end-to-end example scenarios that combine many CLI features at once
(auth chaining, data-driven, parallel waves, GraphQL/WebSocket, signing, vaults, CI,
distributed/perf, compliance).

This is **separate** from `../web` (the authenticated team dashboard).

## Stack

- SvelteKit 2 + Svelte 4 + Vite 5 (matches `../web`)
- `@sveltejs/adapter-static` — pure prerendered SSG output
- mdsvex — scenario bodies authored as Markdown (`.svx`)
- Shiki — build-time syntax highlighting (zero highlighter JS shipped to the client)
- Tailwind CSS 3 + `@tailwindcss/typography`

No runtime dependencies — everything is build-time.

## Develop

```bash
npm install
npm run dev        # http://localhost:5173 (or: npm run dev -- --port 4321)
npm run build      # prerenders to build/  (strict: fails on any dead link / bad frontmatter)
npm run preview    # serve the built output on :4173
npm run check      # svelte-check
npm run lint       # eslint
```

## Add a new example

Drop one file in `src/content/examples/<slug>.svx`. It is auto-discovered,
indexed, routed, and prerendered — no wiring needed.

```markdown
---
title: My scenario
intent: One-sentence "what this proves" hook.
group: core            # core | resilience | security | automation  (see src/lib/content/taxonomy.ts)
features: [auth, cel]  # ids from taxonomy.ts — validated at build time
command: curlew run collections/mine.yaml
order: 25              # global sort order
runnable: false        # true if the captured output is from a real run
---

Prose, then fenced ```yaml / ```bash / ```json code blocks (auto-highlighted,
with copy buttons). Use <Callout type="note|tip|warn|output"> for asides
(`import Callout from '$lib/components/Callout.svelte'` at the top).

Escape literal `{{...}}` inside <Callout>/<code> tags as `{'{{...}}'}` — they're
parsed as Svelte mustaches there (Markdown backtick spans are auto-escaped).
```

Feature tags and groups are defined once in
[`src/lib/content/taxonomy.ts`](src/lib/content/taxonomy.ts) — badges and filters
derive from it. Adding a feature there makes it available everywhere.

## Output captured vs representative

Examples that target free public endpoints (e.g. `httpbin.org`) are marked
`runnable: true` and show output captured from a real `curlew` run. Scenarios that
need a private API, paid provider, or backend show output modeled on the real tool's
documented format, labelled with a `<Callout type="output">`.
