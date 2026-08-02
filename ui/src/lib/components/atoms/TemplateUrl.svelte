<script lang="ts">
  // Raw template URL renderer (§10.5.3): {{…}} spans styled --fg2 italic on
  // --acc-dim, the rest mono --fg1. Never resolves values.
  export let url: string;

  interface Part {
    text: string;
    tpl: boolean;
  }

  function split(u: string): Part[] {
    const out: Part[] = [];
    const re = /\{\{[^}]*\}\}/g;
    let last = 0;
    for (const m of u.matchAll(re)) {
      const i = m.index ?? 0;
      if (i > last) out.push({ text: u.slice(last, i), tpl: false });
      out.push({ text: m[0], tpl: true });
      last = i + m[0].length;
    }
    if (last < u.length) out.push({ text: u.slice(last), tpl: false });
    return out;
  }

  $: parts = split(url);
</script>

<span class="turl at-mono">
  {#each parts as p}{#if p.tpl}<span class="tpl">{p.text}</span>{:else}{p.text}{/if}{/each}
</span>

<style>
  .turl {
    color: var(--fg1);
    font-size: var(--fs-xs);
    overflow-wrap: anywhere;
  }
  .tpl {
    color: var(--fg2);
    font-style: italic;
    background: var(--acc-dim);
    border-radius: 2px;
  }
</style>
