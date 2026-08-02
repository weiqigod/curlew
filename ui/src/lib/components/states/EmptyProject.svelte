<script lang="ts">
  // Empty-project onboarding (§10.6.5) — auto-refreshes via files.changed →
  // tree refetch, which swaps this screen out live.
  import { meta } from '../../stores/meta';
  import Icon from '../atoms/Icon.svelte';

  $: dir = $meta?.project.collection_filter ?? 'collections/';
</script>

<div class="empty">
  <div class="inner">
    <div>
      <h1 class="hl" tabindex="-1">No collections in this repo yet</h1>
      <div class="sub">
        apitest reads YAML files from <span class="at-mono">{dir}</span> — your files in git are
        the only source of truth. Scaffold a starting point from your terminal:
      </div>
    </div>
    <div class="term at-mono">
      <div><span class="p">$ </span><span class="c">apitest init</span></div>
      <div>&nbsp;</div>
      <div><span class="ok">&nbsp;&nbsp;created&nbsp;&nbsp;</span><span class="f">apitest.yaml</span></div>
      <div><span class="ok">&nbsp;&nbsp;created&nbsp;&nbsp;</span><span class="f">collections/example.yaml</span></div>
      <div><span class="ok">&nbsp;&nbsp;created&nbsp;&nbsp;</span><span class="f">environments/dev.yaml</span></div>
      <div>&nbsp;</div>
      <div class="dim">&nbsp;&nbsp;3 files written. Edit collections/example.yaml, then:</div>
      <div>&nbsp;</div>
      <div><span class="p">$ </span><span class="c">apitest run</span><span class="cursor"></span></div>
    </div>
    <div class="foot">
      <Icon name="file" size={13} />
      this view refreshes automatically when files appear — apitest is watching the repo
    </div>
  </div>
</div>

<style>
  .empty {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    min-height: 0;
  }
  .inner {
    width: 560px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }
  .hl {
    font-size: 15px;
    font-weight: 600;
    margin: 0;
    outline: none;
  }
  .sub {
    font-size: var(--fs-sm);
    color: var(--fg2);
    margin-top: 4px;
    line-height: 1.5;
  }
  .term {
    background: var(--bg1);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 14px 16px;
    font-size: var(--fs-sm);
    line-height: 1.75;
    white-space: pre;
  }
  .p {
    color: var(--fg3);
  }
  .c {
    color: var(--fg0);
  }
  .ok {
    color: var(--ok);
  }
  .f {
    color: var(--fg1);
  }
  .dim {
    color: var(--fg2);
  }
  .cursor {
    display: inline-block;
    width: 7px;
    height: 14px;
    background: var(--fg2);
    margin-left: 6px;
    vertical-align: middle;
  }
  .foot {
    font-size: var(--fs-xs);
    color: var(--fg3);
    display: flex;
    gap: 6px;
    align-items: center;
  }
</style>
