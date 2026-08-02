<script lang="ts">
  // Clipboard copy with 1.1 s check feedback (§10.5.3).
  import Icon from './Icon.svelte';

  export let text: string;
  export let label = '';

  let ok = false;
  let timer: ReturnType<typeof setTimeout> | null = null;

  function copy(e: MouseEvent): void {
    e.stopPropagation();
    try {
      void navigator.clipboard?.writeText(text);
    } catch {
      // clipboard unavailable — nothing useful to do
    }
    ok = true;
    if (timer !== null) clearTimeout(timer);
    timer = setTimeout(() => (ok = false), 1100);
  }
</script>

<button
  class="at-btn sm ghost cbtn"
  title={'copy ' + label}
  aria-label={'copy ' + (label || 'value')}
  on:click={copy}
>
  <Icon name={ok ? 'check' : 'copy'} size={12} />
  {#if ok}<span class="copied">copied</span>{/if}
</button>

<style>
  .cbtn {
    height: 18px;
    padding: 0 4px;
  }
  .copied {
    font-size: 10px;
  }
</style>
