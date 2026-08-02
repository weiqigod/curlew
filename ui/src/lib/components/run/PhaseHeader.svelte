<script lang="ts">
  // setup / teardown section header (§10.6.2.3); main renders no header.
  export let phase: 'setup' | 'teardown';
</script>

<div class="at-group-h" role="presentation">{phase}</div>
