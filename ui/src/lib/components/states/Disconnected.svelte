<script lang="ts">
  // Full-screen server-unreachable state (§10.6.7.1). Auto-retries GET /meta
  // every 5 s with a countdown; on success, full re-boot preserving the hash.
  import { onDestroy, onMount } from 'svelte';
  import { apiFetch } from '../../api/client';

  let countdown = 5;
  let probing = false;
  let timer: ReturnType<typeof setInterval> | null = null;

  async function probe(): Promise<void> {
    if (probing) return;
    probing = true;
    try {
      await apiFetch('/meta');
      // Reachable again — full re-boot (§10.4.3); location.reload keeps the
      // hash and the sessionStorage token.
      location.reload();
    } catch {
      countdown = 5;
    } finally {
      probing = false;
    }
  }

  onMount(() => {
    timer = setInterval(() => {
      countdown--;
      if (countdown <= 0) {
        countdown = 5;
        void probe();
      }
    }, 1000);
  });

  onDestroy(() => {
    if (timer !== null) clearInterval(timer);
  });
</script>

<div class="disc at-root" data-theme="dark">
  <div class="panel">
    <div class="title">curlew ui is not running</div>
    <div class="body">
      the server at <span class="at-mono">{location.host}</span> stopped or this tab's session
      expired. Restart it and reopen the printed URL:
    </div>
    <div class="term at-mono">
      <span class="prompt">$ </span><span>curlew ui</span>
    </div>
    <div class="actions">
      <button class="at-btn ghost" on:click={() => void probe()} disabled={probing}>
        {probing ? 'probing…' : 'Retry now'}
      </button>
      <span class="count at-mono">retrying in {countdown}s</span>
    </div>
  </div>
</div>

<style>
  .disc {
    position: fixed;
    inset: 0;
    display: flex;
    align-items: center;
    justify-content: center;
    background: var(--bg0);
    z-index: 100;
  }
  .panel {
    width: 480px;
    display: flex;
    flex-direction: column;
    gap: 12px;
  }
  .title {
    font: 600 var(--fs-lg) var(--font-sans);
  }
  .body {
    font-size: var(--fs-sm);
    color: var(--fg2);
    line-height: 1.5;
  }
  .term {
    background: var(--bg1);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 12px 16px;
    font-size: var(--fs-sm);
  }
  .prompt {
    color: var(--fg3);
  }
  .actions {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .count {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
</style>
