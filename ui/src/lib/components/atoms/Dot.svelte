<script lang="ts">
  // Status dot (§10.5.3): pending / running (spinner) / passed / failed /
  // skipped / error (hollow --err2 ring + "!") / mixed (diagonal split).
  // Color is never the sole channel — the glyph + title carry the state.
  export let state:
    | 'pending'
    | 'running'
    | 'passed'
    | 'failed'
    | 'skipped'
    | 'error'
    | 'mixed' = 'pending';
  export let title: string | undefined = undefined;

  const CLS: Record<string, string> = {
    pending: 'pending',
    passed: 'pass',
    failed: 'fail',
    skipped: 'skip',
    mixed: 'mixed',
  };
</script>

{#if state === 'running'}
  <span class="at-spin" role="img" title={title ?? 'running'} aria-label={title ?? 'running'}
  ></span>
{:else if state === 'error'}
  <span
    class="at-dot error errdot"
    role="img"
    title={title ?? 'error'}
    aria-label={title ?? 'error'}><span class="bang" aria-hidden="true">!</span></span
  >
{:else}
  <span
    class="at-dot {CLS[state]}"
    role="img"
    title={title ?? state}
    aria-label={title ?? state}
  ></span>
{/if}

<style>
  .errdot {
    display: inline-flex;
    align-items: center;
    justify-content: center;
  }
  .bang {
    font: 700 7px var(--font-mono);
    line-height: 1;
    color: var(--err2);
  }
</style>
