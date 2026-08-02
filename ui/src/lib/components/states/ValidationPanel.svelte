<script lang="ts">
  // Validation panel for an invalid collection (§10.6.6, #/file/<path>).
  // Renders ALL issues with severity coloring; the `hint` field is its own
  // line — never fabricate inline annotations.
  import { validate } from '../../api/validate';
  import { lastFilesChanged } from '../../stores/ui';
  import { tree } from '../../stores/tree';
  import type { TreeIssue } from '../../types/tree';
  import Icon from '../atoms/Icon.svelte';
  import OpenInEditor from '../atoms/OpenInEditor.svelte';

  export let path: string;

  let issues: TreeIssue[] = [];
  let valid = false;
  let loaded = false;

  // Seed from the tree entry, then refresh via GET /validate on mount and
  // whenever files.changed includes this path.
  $: treeEntry = $tree?.collections.find((c) => c.path === path);
  $: if (!loaded && treeEntry !== undefined) {
    issues = treeEntry.issues;
    valid = treeEntry.valid;
  }

  async function refresh(p: string): Promise<void> {
    try {
      const res = await validate(p);
      const f = res.files.find((x) => x.file === p) ?? res.files[0];
      if (f !== undefined) {
        issues = f.issues;
        valid = f.valid;
      }
      loaded = true;
    } catch {
      // tree data remains the fallback
    }
  }

  $: void refresh(path);
  $: if ($lastFilesChanged.paths.includes(path)) void refresh(path);

  $: firstLine = issues.length > 0 ? issues[0].line : 1;
</script>

<div class="vp">
  <div class="inner">
    <div class="head">
      <span class="warnico"><Icon name="warn" size={14} /></span>
      <h1 class="hl" tabindex="-1">{valid ? 'Collection file' : 'Invalid collection file'}</h1>
      <span class="at-mono at-xs at-dim">{path}</span>
      <span class="sp"></span>
      <OpenInEditor file={path} line={firstLine} />
    </div>

    <div class="body">
      This file was skipped — its requests won't run until the file parses. Fix it in your editor;
      the tree reloads the moment you save.
    </div>

    {#each issues as issue}
      <div class="diag at-mono" class:err={issue.severity === 'error'} class:warn={issue.severity !== 'error'}>
        <div>
          <span class="sev">{issue.severity}</span>
          <span class="msg">
            {#if issue.line > 0}Line {issue.line}: {/if}{issue.message}
          </span>
        </div>
        {#if issue.hint !== undefined && issue.hint !== ''}
          <div class="hint">hint: {issue.hint}</div>
        {/if}
      </div>
    {:else}
      <div class="diag at-mono ok">no issues reported — the file may have been fixed already</div>
    {/each}

    <div class="foot">apitest never edits your files — there is deliberately no fix-it form here.</div>

    <div>
      <a class="at-btn ghost sm" href="#/"><Icon name="back" size={13} /> back to run view</a>
    </div>
  </div>
</div>

<style>
  .vp {
    flex: 1;
    overflow: auto;
    padding: calc(var(--pad) * 2);
    min-height: 0;
  }
  .inner {
    max-width: 720px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .head {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .warnico {
    color: var(--warn);
    display: flex;
  }
  .hl {
    font-size: 15px;
    font-weight: 600;
    margin: 0;
    outline: none;
  }
  .sp {
    flex: 1;
  }
  .body {
    font-size: var(--fs-sm);
    color: var(--fg2);
    line-height: 1.5;
  }
  .diag {
    border-radius: var(--rad);
    padding: 10px 14px;
    font-size: var(--fs-sm);
    line-height: 1.6;
  }
  .diag.err {
    background: color-mix(in srgb, var(--err) 7%, transparent);
    border: 1px solid color-mix(in srgb, var(--err) 30%, transparent);
  }
  .diag.err .sev {
    color: var(--err);
    font-weight: 600;
  }
  .diag.warn {
    background: color-mix(in srgb, var(--warn) 7%, transparent);
    border: 1px solid color-mix(in srgb, var(--warn) 30%, transparent);
  }
  .diag.warn .sev {
    color: var(--warn);
    font-weight: 600;
  }
  .diag.ok {
    background: var(--bg1);
    border: 1px solid var(--bd0);
    color: var(--fg2);
  }
  .msg {
    color: var(--fg1);
  }
  .hint {
    color: var(--fg2);
    font-style: italic;
    margin-top: 2px;
  }
  .foot {
    font-size: var(--fs-xs);
    color: var(--fg3);
  }
</style>
