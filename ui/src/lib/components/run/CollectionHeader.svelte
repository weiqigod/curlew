<script lang="ts">
  // Collection section header for batch runs (§10.6.2.3).
  export let file: string;
  $: basename = file.split('/').pop() ?? file;
</script>

<div class="at-group-h" role="presentation">
  <span class="at-mono base">{basename}</span>
</div>

<style>
  .base {
    text-transform: none;
    letter-spacing: 0;
  }
</style>
