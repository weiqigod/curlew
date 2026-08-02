<script lang="ts">
  // Transient toast host (stores/toast.ts).
  import { dismissToast, toasts } from '../../stores/toast';
  import Icon from '../atoms/Icon.svelte';
</script>

{#if $toasts.length > 0}
  <div class="host" role="status" aria-live="polite">
    {#each $toasts as t (t.id)}
      <div class="toast" class:error={t.kind === 'error'}>
        <span class="msg">{t.message}</span>
        {#if t.action !== undefined}
          <button
            class="at-btn sm"
            on:click={() => {
              t.action?.fn();
              dismissToast(t.id);
            }}>{t.action.label}</button
          >
        {/if}
        <button class="at-btn sm ghost close" aria-label="dismiss" on:click={() => dismissToast(t.id)}>
          <Icon name="x" size={11} />
        </button>
      </div>
    {/each}
  </div>
{/if}

<style>
  .host {
    position: fixed;
    bottom: 14px;
    right: 14px;
    z-index: 80;
    display: flex;
    flex-direction: column;
    gap: 6px;
    max-width: 420px;
  }
  .toast {
    display: flex;
    align-items: center;
    gap: 10px;
    background: var(--bg2);
    border: 1px solid var(--bd1);
    border-radius: var(--rad);
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.4);
    padding: 8px 10px;
    font-size: var(--fs-sm);
    animation: at-fadein 0.18s ease-out;
  }
  .toast.error {
    border-color: color-mix(in srgb, var(--err) 45%, var(--bd1));
  }
  .msg {
    flex: 1;
    line-height: 1.4;
    overflow-wrap: anywhere;
  }
  .close {
    padding: 0 4px;
  }
</style>
