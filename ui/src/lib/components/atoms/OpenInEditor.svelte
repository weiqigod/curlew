<script lang="ts">
  // The single editing affordance (§10.6.3.8): POST /open → 204 flash; 409
  // no_editor → toast with the server hint; 404 (older binary) → degrade to
  // copying file:line with a "copied path" flash.
  import { ApiError } from '../../api/client';
  import { openInEditor } from '../../api/open';
  import { toast } from '../../stores/toast';
  import Icon from './Icon.svelte';

  export let file: string;
  export let line: number;
  export let compact = false;

  let flash: '' | 'editor' | 'copied' = '';
  let timer: ReturnType<typeof setTimeout> | null = null;

  function setFlash(f: 'editor' | 'copied'): void {
    flash = f;
    if (timer !== null) clearTimeout(timer);
    timer = setTimeout(() => (flash = ''), 1400);
  }

  export async function open(): Promise<void> {
    try {
      await openInEditor(file, line);
      setFlash('editor');
    } catch (err) {
      if (err instanceof ApiError && err.code === 'no_editor') {
        const hint = err.hint ?? 'set ui.editor in curlew.yaml or $CURLEW_EDITOR';
        toast(`${err.message} — ${hint}`, { kind: 'error' });
        return;
      }
      if (err instanceof ApiError && err.status === 404) {
        try {
          await navigator.clipboard?.writeText(`${file}:${line}`);
        } catch {
          // clipboard unavailable
        }
        setFlash('copied');
        return;
      }
      toast('could not open the editor', { kind: 'error' });
    }
  }
</script>

<button
  class="at-btn sm ghost oie at-mono"
  title={`${file}:${line}`}
  on:click={() => void open()}
>
  <Icon name="external" size={12} />
  {#if flash === 'editor'}
    <span>→ editor</span>
  {:else if flash === 'copied'}
    <span>copied path</span>
  {:else}
    <span>Open in editor</span>
    {#if !compact}<span class="ln">:{line}</span>{/if}
  {/if}
</button>

<style>
  .oie {
    font-family: var(--font-mono);
    font-size: var(--fs-xs);
  }
  .ln {
    color: var(--fg3);
  }
</style>
