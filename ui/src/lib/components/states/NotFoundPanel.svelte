<script lang="ts">
  // Inline not-found panel (§10.4.2): unknown routes and expired run ids.
  export let message = 'page not found';
</script>

<div class="nf">
  <div class="title">{message}</div>
  <a href="#/" class="at-btn ghost">← back to the run view</a>
</div>

<style>
  .nf {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 12px;
    min-height: 0;
    padding: var(--pad);
  }
  .title {
    font: 600 var(--fs-lg) var(--font-sans);
    color: var(--fg1);
  }
</style>
