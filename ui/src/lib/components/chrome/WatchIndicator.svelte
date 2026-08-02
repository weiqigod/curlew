<script lang="ts">
  // Watching indicator (§10.6.1.2). Pulses --warn on files.changed; hidden
  // (replaced by the WS banner state) while the socket is down.
  import { wsStatus } from '../../stores/connection';
  import { watchEvent } from '../../stores/ui';

  $: active = $watchEvent !== null;
  $: socketUp = $wsStatus === 'open' || $wsStatus === 'connecting';
</script>

{#if socketUp}
  <div class="wi" title="curlew is watching the repo for file changes">
    {#key $watchEvent?.seq}
      <span class="dot" class:active></span>
    {/key}
    <span class="label at-mono" class:active aria-live="polite">
      {active ? $watchEvent?.label : 'watching files'}
    </span>
  </div>
{/if}

<style>
  .wi {
    display: flex;
    align-items: center;
    gap: 6px;
    margin-left: 4px;
    min-width: 0;
  }
  .dot {
    width: 7px;
    height: 7px;
    border-radius: 50%;
    flex: none;
    background: var(--fg3);
  }
  .dot.active {
    background: var(--warn);
    animation: at-pulse 1s ease-out 2;
  }
  .label {
    font-size: var(--fs-xs);
    color: var(--fg3);
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .label.active {
    color: var(--fg1);
  }
</style>
