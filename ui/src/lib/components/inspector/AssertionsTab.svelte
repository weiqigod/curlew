<script lang="ts">
  // Assertions tab (§10.6.3.3): type badge, client-rendered expectation line,
  // failed cards expand expected/actual with >120-char collapse + copy.
  import type { Assertions } from '../../types/run';
  import CopyBtn from '../atoms/CopyBtn.svelte';
  import Icon from '../atoms/Icon.svelte';

  export let assertions: Assertions | null;

  const LONG = 120;
  let expandedValues: Record<string, boolean> = {};

  $: items = assertions?.items ?? [];

  function valKey(i: number, which: string): string {
    return `${i}:${which}`;
  }

  function shown(value: string, key: string): string {
    if (value.length <= LONG || expandedValues[key]) return value;
    return value.slice(0, LONG) + '…';
  }
</script>

<div class="at2">
  {#each items as a, i}
    <div class="card" class:failcard={!a.passed}>
      <div class="head">
        <span class="ico" class:ok={a.passed} class:err={!a.passed}>
          <Icon name={a.passed ? 'check' : 'x'} size={12} sw={1.8} />
        </span>
        <span class="typechip">{a.type}</span>
        <span class="expect at-mono">{a.label} · expected {a.expected}</span>
        <span class="affix" class:ok={a.passed} class:err={!a.passed}>
          {a.passed ? 'passed' : 'failed'}
        </span>
      </div>
      {#if !a.passed}
        <div class="detail at-mono">
          <div class="vrow">
            <span class="vlbl">expected</span>
            <span class="vexp">{shown(a.expected, valKey(i, 'e'))}</span>
            {#if a.expected.length > LONG}
              <button
                class="jt-more"
                on:click={() =>
                  (expandedValues = {
                    ...expandedValues,
                    [valKey(i, 'e')]: !expandedValues[valKey(i, 'e')],
                  })}
              >
                {expandedValues[valKey(i, 'e')] ? 'collapse' : 'expand'}
              </button>
            {/if}
            <CopyBtn text={a.expected} label="expected" />
          </div>
          <div class="vrow">
            <span class="vlbl">actual</span>
            <span class="vact">{shown(a.actual, valKey(i, 'a'))}</span>
            {#if a.actual.length > LONG}
              <button
                class="jt-more"
                on:click={() =>
                  (expandedValues = {
                    ...expandedValues,
                    [valKey(i, 'a')]: !expandedValues[valKey(i, 'a')],
                  })}
              >
                {expandedValues[valKey(i, 'a')] ? 'collapse' : 'expand'}
              </button>
            {/if}
            <CopyBtn text={a.actual} label="actual" />
          </div>
        </div>
      {/if}
    </div>
  {:else}
    <div class="empty at-mono">
      no assertions for this request — only transport success was checked
    </div>
  {/each}
</div>

<style>
  .at2 {
    overflow: auto;
    flex: 1;
    padding: var(--pad);
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .card {
    border: 1px solid var(--bd0);
    background: var(--bg1);
    border-radius: var(--rad);
    padding: 7px 10px;
  }
  .card.failcard {
    border-color: color-mix(in srgb, var(--err) 30%, transparent);
    background: color-mix(in srgb, var(--err) 7%, transparent);
  }
  .head {
    display: flex;
    align-items: center;
    gap: 9px;
  }
  .ico {
    display: flex;
  }
  .ico.ok {
    color: var(--ok);
  }
  .ico.err {
    color: var(--err);
  }
  .typechip {
    font: 600 9px var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    color: var(--fg1);
    background: var(--bg3);
    border: 1px solid var(--bd1);
    border-radius: 3px;
    padding: 1px 5px;
    flex: none;
  }
  .expect {
    font-size: var(--fs-sm);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .affix {
    font-size: 10px;
    font-weight: 600;
    text-transform: uppercase;
    letter-spacing: 0.06em;
  }
  .affix.ok {
    color: var(--ok);
  }
  .affix.err {
    color: var(--err);
  }
  .detail {
    margin-top: 6px;
    margin-left: 21px;
    font-size: var(--fs-xs);
    line-height: 1.7;
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .vrow {
    display: flex;
    align-items: baseline;
    gap: 8px;
  }
  .vlbl {
    color: var(--fg2);
    width: 70px;
    flex: none;
  }
  .vexp {
    color: var(--fg0);
    overflow-wrap: anywhere;
  }
  .vact {
    color: var(--err);
    overflow-wrap: anywhere;
  }
  .empty {
    font-size: var(--fs-sm);
    color: var(--fg3);
  }
</style>
