<script lang="ts">
  // Thin .at-btn wrapper forwarding click (variant: default | primary | ghost).
  export let variant: 'default' | 'primary' | 'ghost' = 'default';
  export let sm = false;
  export let disabled = false;
  export let title: string | undefined = undefined;
  export let ariaLabel: string | undefined = undefined;
</script>

<button
  class="at-btn"
  class:primary={variant === 'primary'}
  class:ghost={variant === 'ghost'}
  class:sm
  {disabled}
  {title}
  aria-label={ariaLabel}
  on:click
>
  <slot />
</button>
