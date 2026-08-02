<script lang="ts">
  // Run-infrastructure error banner (§10.3.2: run.error event).
  import type { EventError } from '../../types/events';
  import Icon from '../atoms/Icon.svelte';

  export let error: EventError;
</script>

<div class="reb" role="alert">
  <span class="ico"><Icon name="warn" size={14} /></span>
  <span class="cat">{error.category}</span>
  <span class="msg at-mono">{error.message}</span>
</div>

<style>
  .reb {
    display: flex;
    align-items: baseline;
    gap: 8px;
    margin: var(--pad-sm) var(--pad) 0;
    padding: 8px 12px;
    background: color-mix(in srgb, var(--err2) 8%, transparent);
    border: 1px solid color-mix(in srgb, var(--err2) 30%, transparent);
    border-radius: var(--rad);
    font-size: var(--fs-sm);
  }
  .ico {
    color: var(--err2);
    align-self: center;
    display: flex;
  }
  .cat {
    font-weight: 600;
    color: var(--err2);
  }
  .msg {
    color: var(--fg1);
    overflow-wrap: anywhere;
  }
</style>
