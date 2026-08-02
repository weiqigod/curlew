<script lang="ts">
  // Binary body panel (§10.6.3.1): content type, size, Download via Blob;
  // inline preview for image/* (object URL, checkerboard background).
  import { onDestroy } from 'svelte';
  import { fmtBytes } from '../../format';
  import Icon from '../atoms/Icon.svelte';

  export let base64: string;
  export let contentType: string;
  export let size: number;
  export let filename: string;

  let url: string | null = null;

  function toBlob(): Blob {
    const bin = atob(base64);
    const bytes = new Uint8Array(bin.length);
    for (let i = 0; i < bin.length; i++) bytes[i] = bin.charCodeAt(i);
    return new Blob([bytes], { type: contentType || 'application/octet-stream' });
  }

  $: isImage = contentType.startsWith('image/');
  $: {
    if (url !== null) URL.revokeObjectURL(url);
    try {
      url = URL.createObjectURL(toBlob());
    } catch {
      url = null;
    }
  }

  onDestroy(() => {
    if (url !== null) URL.revokeObjectURL(url);
  });

  function download(): void {
    if (url === null) return;
    const a = document.createElement('a');
    a.href = url;
    a.download = filename;
    a.click();
  }
</script>

<div class="bp">
  <div class="meta at-mono">
    <span>{contentType || 'application/octet-stream'}</span>
    <span class="dotsep">·</span>
    <span>{fmtBytes(size)}</span>
    <span class="dotsep">·</span>
    <span class="dim">binary content</span>
  </div>
  <div>
    <button class="at-btn" on:click={download} disabled={url === null}>
      <Icon name="download" size={13} />
      Download
    </button>
  </div>
  {#if isImage && url !== null}
    <div class="preview">
      <img src={url} alt="response body preview" />
    </div>
  {/if}
</div>

<style>
  .bp {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: var(--pad);
  }
  .meta {
    font-size: var(--fs-sm);
    color: var(--fg1);
    display: flex;
    gap: 8px;
  }
  .dotsep {
    color: var(--fg3);
  }
  .dim {
    color: var(--fg2);
  }
  .preview {
    background:
      repeating-conic-gradient(var(--bg3) 0% 25%, var(--bg1) 0% 50%) 0 0 / 16px 16px;
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 8px;
    align-self: flex-start;
  }
  .preview img {
    max-height: 480px;
    max-width: 100%;
    display: block;
  }
</style>
