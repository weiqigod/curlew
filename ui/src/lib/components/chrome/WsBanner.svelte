<script lang="ts">
  // WS reconnecting banner (§10.6.7.2): slim bar under the top bar while
  // reconnecting; green "reconnected" flash for 1.5 s on recovery.
  import { wsStatus } from '../../stores/connection';
  import { runState } from '../../stores/run';

  let wasReconnecting = false;
  let flash = false;
  let timer: ReturnType<typeof setTimeout> | null = null;

  $: {
    if ($wsStatus === 'reconnecting') {
      wasReconnecting = true;
    } else if ($wsStatus === 'open' && wasReconnecting) {
      wasReconnecting = false;
      flash = true;
      if (timer !== null) clearTimeout(timer);
      timer = setTimeout(() => (flash = false), 1500);
    }
  }

  $: runLive = $runState === 'running' || $runState === 'cancelling';
</script>

{#if $wsStatus === 'reconnecting'}
  <div class="at-banner warn banner" aria-live="polite">
    <span class="at-spin"></span>
    <span>reconnecting to apitest…</span>
    {#if runLive}<span class="dim">run continues — events will catch up</span>{/if}
  </div>
{:else if flash}
  <div class="at-banner ok banner" aria-live="polite">reconnected</div>
{/if}

<style>
  .banner {
    flex: none;
  }
  .banner.warn {
    background: color-mix(in srgb, var(--warn) 8%, transparent);
  }
  .banner.ok {
    background: color-mix(in srgb, var(--ok) 8%, transparent);
    border-bottom-color: var(--ok);
    color: var(--fg1);
  }
  .dim {
    color: var(--fg2);
    font-weight: 400;
  }
</style>
