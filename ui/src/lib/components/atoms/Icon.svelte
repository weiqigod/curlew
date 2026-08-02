<script lang="ts" context="module">
  // Inline stroke icons ported from the design prototype (at-bits.jsx).
  const ICONS: Record<string, string> = {
    'chevron-down': '<polyline points="4,6 8,10 12,6" />',
    'chevron-right': '<polyline points="6,4 10,8 6,12" />',
    caret: '<polyline points="4,6 8,10 12,6" />',
    search: '<circle cx="7" cy="7" r="4.5" /><line x1="10.5" y1="10.5" x2="14" y2="14" />',
    play: '<path d="M5 3.5 L12.5 8 L5 12.5 Z" fill="currentColor" stroke="none" />',
    branch:
      '<circle cx="4.5" cy="4" r="2" /><circle cx="4.5" cy="12" r="2" /><circle cx="11.5" cy="6" r="2" /><path d="M4.5 6 v4 M11.5 8 c0 3 -4 2 -5 3" />',
    clock: '<circle cx="8" cy="8" r="5.5" /><polyline points="8,5 8,8 10.5,9.5" />',
    external:
      '<path d="M6.5 4 H4 a1 1 0 0 0 -1 1 v7 a1 1 0 0 0 1 1 h7 a1 1 0 0 0 1 -1 V9.5" /><path d="M9 3 h4 v4 M13 3 L7.5 8.5" />',
    copy: '<rect x="5.5" y="5.5" width="7" height="7" rx="1" /><path d="M10.5 5.5 V4 a1 1 0 0 0 -1 -1 H4 a1 1 0 0 0 -1 1 v5.5 a1 1 0 0 0 1 1 h1.5" />',
    check: '<polyline points="3,8.5 6.5,12 13,4.5" />',
    x: '<line x1="4" y1="4" x2="12" y2="12" /><line x1="12" y1="4" x2="4" y2="12" />',
    warn: '<path d="M8 2.5 L14.5 13.5 H1.5 Z" /><line x1="8" y1="7" x2="8" y2="10" /><circle cx="8" cy="12" r="0.4" fill="currentColor" />',
    back: '<line x1="13" y1="8" x2="3.5" y2="8" /><polyline points="7.5,4 3.5,8 7.5,12" />',
    file: '<path d="M4 2.5 h5.5 L13 6 v7.5 a1 1 0 0 1 -1 1 H4 a1 1 0 0 1 -1 -1 v-10 a1 1 0 0 1 1 -1 Z" /><polyline points="9.5,2.5 9.5,6 13,6" />',
    skip: '<line x1="4" y1="8" x2="12" y2="8" />',
    lock: '<rect x="3.5" y="7" width="9" height="6.5" rx="1" /><path d="M5.5 7 V5 a2.5 2.5 0 0 1 5 0 v2" />',
    parallel: '<path d="M3 5.5 h8 M3 10.5 h8" /><polyline points="8.5,3 11,5.5 8.5,8" /><polyline points="8.5,8 11,10.5 8.5,13" />',
    swap: '<path d="M4 6 h8 M12 10 h-8" /><polyline points="9.5,3.5 12,6 9.5,8.5" /><polyline points="6.5,7.5 4,10 6.5,12.5" />',
    download:
      '<path d="M8 2.5 v8 M4.5 7 L8 10.5 L11.5 7" /><path d="M3 12.5 h10" />',
    stack: '<ellipse cx="8" cy="4.5" rx="5" ry="2" /><path d="M3 4.5 v3 c0 1.1 2.2 2 5 2 s5 -0.9 5 -2 v-3 M3 7.5 v3 c0 1.1 2.2 2 5 2 s5 -0.9 5 -2 v-3" />',
  };
</script>

<script lang="ts">
  export let name: string;
  export let size = 14;
  export let sw = 1.5;
</script>

<svg
  width={size}
  height={size}
  viewBox="0 0 16 16"
  fill="none"
  stroke="currentColor"
  stroke-width={sw}
  stroke-linecap="round"
  stroke-linejoin="round"
  style="flex:none"
  aria-hidden="true"
>
  <!-- eslint-disable-next-line svelte/no-at-html-tags — static path data only -->
  {@html ICONS[name] ?? ''}
</svg>
