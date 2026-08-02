<script lang="ts">
  export let title: string | undefined = undefined;
</script>

<span class="at-chip" {title}><slot /></span>
