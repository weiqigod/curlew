<script lang="ts">
  // compact | columns | lanes toggle (§10.6.2.1). Default compact; columns
  // and lanes disabled for sequential runs (control stays visible).
  import { prefs } from '../../stores/ui';

  /** False when the focused run was sequential. */
  export let parallel: boolean;

  const OPTIONS = ['compact', 'columns', 'lanes'] as const;
  const seqTip = 'this run was sequential — waves apply to parallel runs only';

  function pick(layout: (typeof OPTIONS)[number]): void {
    prefs.update((p) => ({ ...p, runLayout: layout }));
  }

  $: effective = parallel ? $prefs.runLayout : 'compact';
</script>

<div class="at-seg" role="group" aria-label="run layout">
  {#each OPTIONS as o}
    <button
      class:on={effective === o}
      disabled={o !== 'compact' && !parallel}
      title={o !== 'compact' && !parallel ? seqTip : undefined}
      on:click={() => pick(o)}
    >
      {o}
    </button>
  {/each}
</div>
