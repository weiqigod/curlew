<script lang="ts">
  // Tiny uppercase phase chip (§10.5.3) — `main` renders nothing (default).
  export let phase: 'setup' | 'main' | 'teardown';
</script>

{#if phase !== 'main'}
  <span class="pb">{phase}</span>
{/if}

<style>
  .pb {
    font: 600 9px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--fg2);
    background: var(--bg3);
    border: 1px solid var(--bd1);
    border-radius: 3px;
    padding: 1px 4px;
    flex: none;
  }
</style>
