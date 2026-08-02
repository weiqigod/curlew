<script lang="ts">
  // Status code + client-side reason text ("201 Created") — the server sends
  // only the number (§10.5.3). Neutral color; pass/fail lives on the Dot.
  import { statusText } from '../../status';
  export let code: number;
</script>

<span class="code at-mono">{statusText(code)}</span>

<style>
  .code {
    font-size: var(--fs-xs);
    color: var(--fg1);
    white-space: nowrap;
  }
</style>
