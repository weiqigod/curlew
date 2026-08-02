<script lang="ts">
  // Compare screen (§10.6.4): history rail, pair picker
  // (changed-first), run header cards with deltas, client-side body diff.
  // #/compare?base=&target=&slug=&iter=&changes=1 round-trips.
  import { onDestroy, onMount } from 'svelte';
  import { registerKey } from '../../keyboard';
  import { fmtUs } from '../../format';
  import { navigate } from '../../router';
  import { comparePairs, loadCompare } from '../../stores/compare';
  import { loadHistory, runs } from '../../stores/history';
  import { meta } from '../../stores/meta';
  import { prefs } from '../../stores/ui';
  import type { ComparePair, CompareSide } from '../../types/compare';
  import Icon from '../atoms/Icon.svelte';
  import DiffView from './DiffView.svelte';
  import HistoryRail from './HistoryRail.svelte';
  import RequestPicker, { type PickerEntry } from './RequestPicker.svelte';
  import RunHeaderCard from './RunHeaderCard.svelte';

  export let base: string | undefined = undefined;
  export let target: string | undefined = undefined;
  export let slug: string | undefined = undefined;
  export let iter: number | undefined = undefined;
  export let changes: boolean | undefined = undefined;

  $: configDisabled = $meta?.history.enabled === false;

  // URL ?changes=1 wins over the persisted pref on entry.
  let changesApplied = false;
  $: if (changes !== undefined && !changesApplied) {
    changesApplied = true;
    prefs.update((p) => ({ ...p, changesOnly: changes === true }));
  }

  onMount(() => {
    if (!configDisabled) {
      void loadHistory().catch(() => undefined);
    }
  });

  // Default selection: newest run = B (target).
  $: effectiveTarget = target ?? ($runs.length > 0 ? $runs[0].run_id : undefined);
  $: effectiveBase = base;

  function go(opts: {
    base?: string;
    target?: string;
    slug?: string;
    iter?: number;
  }): void {
    navigate({
      name: 'compare',
      ...(opts.base !== undefined ? { base: opts.base } : {}),
      ...(opts.target !== undefined ? { target: opts.target } : {}),
      ...(opts.slug !== undefined ? { slug: opts.slug } : {}),
      ...(opts.iter !== undefined ? { iter: opts.iter } : {}),
      ...($prefs.changesOnly ? { changes: true } : {}),
    });
  }

  function pickRun(id: string): void {
    if (id === effectiveBase && effectiveTarget !== undefined) {
      // Second click on A swaps A↔B.
      go({ base: effectiveTarget, target: id });
    } else if (id !== effectiveTarget) {
      go({ base: id, ...(effectiveTarget !== undefined ? { target: effectiveTarget } : {}) });
    }
  }

  // Load /compare when both ends are set.
  let lastCompareKey = '';
  $: compareKey = `${effectiveBase ?? ''}|${effectiveTarget ?? ''}`;
  $: if (!configDisabled && effectiveBase !== undefined && effectiveTarget !== undefined && compareKey !== lastCompareKey) {
    lastCompareKey = compareKey;
    comparePairs.set(null);
    void loadCompare(effectiveBase, effectiveTarget).catch(() => undefined);
  }

  // ---- pair list (changed-first, §10.6.4.3) --------------------------------
  function isChanged(p: ComparePair): boolean {
    if (p.delta === null) return false;
    if (p.delta.outcome_changed || p.delta.status_changed) return true;
    const baseDur = p.base?.duration_ms ?? 0;
    return baseDur > 0 && Math.abs(p.delta.duration_ms) > 0.2 * baseDur;
  }

  function sym(side: CompareSide | null): string {
    switch (side?.outcome) {
      case 'passed':
        return '✓';
      case 'failed':
        return '✗';
      case 'error':
        return '!';
      case 'skipped':
        return '–';
      default:
        return '·';
    }
  }

  function glyph(p: ComparePair): string {
    if (p.delta === null) return '';
    if (p.delta.outcome_changed) return `${sym(p.base)}→${sym(p.target)}`;
    if (isChanged(p)) {
      return `${p.delta.duration_ms > 0 ? '+' : ''}${p.delta.duration_ms}ms`;
    }
    return '';
  }

  function keyOf(s: string, i: number | null): string {
    return `${s}|${i ?? ''}`;
  }

  $: entries = (() => {
    const result = $comparePairs;
    if (result === null) return [] as PickerEntry[];
    const pairs = [...result.pairs].sort((a, b) => Number(isChanged(b)) - Number(isChanged(a)));
    const out: PickerEntry[] = pairs.map((p) => ({
      key: keyOf(p.slug, p.iteration),
      slug: p.slug,
      iteration: p.iteration,
      name: p.name,
      method: p.method,
      kind: 'pair' as const,
      changed: isChanged(p),
      glyph: glyph(p),
    }));
    for (const e of result.only_in_base) {
      out.push({
        key: `ob:${e.slug}`,
        slug: e.slug,
        iteration: null,
        name: e.name,
        method: e.method,
        kind: 'only_base',
        changed: false,
        glyph: 'A only',
      });
    }
    for (const e of result.only_in_target) {
      out.push({
        key: `ot:${e.slug}`,
        slug: e.slug,
        iteration: null,
        name: e.name,
        method: e.method,
        kind: 'only_target',
        changed: false,
        glyph: 'B only',
      });
    }
    return out;
  })();

  $: selectedKey = (() => {
    if (slug !== undefined) {
      const k = keyOf(slug, iter ?? null);
      if (entries.some((e) => e.key === k)) return k;
      const single = entries.find(
        (e) => (e.kind === 'only_base' || e.kind === 'only_target') && e.slug === slug,
      );
      if (single !== undefined) return single.key;
    }
    const firstChanged = entries.find((e) => e.changed);
    return firstChanged?.key ?? entries[0]?.key ?? null;
  })();

  $: selectedEntry = entries.find((e) => e.key === selectedKey) ?? null;
  $: selectedPair =
    selectedEntry !== null && selectedEntry.kind === 'pair'
      ? ($comparePairs?.pairs.find(
          (p) => keyOf(p.slug, p.iteration) === selectedEntry.key,
        ) ?? null)
      : null;

  function pickPair(key: string): void {
    const e = entries.find((x) => x.key === key);
    if (e === undefined || effectiveBase === undefined || effectiveTarget === undefined) return;
    go({
      base: effectiveBase,
      target: effectiveTarget,
      slug: e.slug,
      ...(e.iteration !== null ? { iter: e.iteration } : {}),
    });
  }

  function step(d: number): void {
    if (entries.length === 0 || selectedKey === null) return;
    const i = entries.findIndex((e) => e.key === selectedKey);
    const next = (i + d + entries.length) % entries.length;
    pickPair(entries[next].key);
  }

  // Timing chips: delta.timing_us entries exceeding 10% of the base phase.
  $: timingChips = (() => {
    const p = selectedPair;
    if (p === null || p.delta?.timing_us === undefined || p.base?.timing == null) return [];
    const baseT = p.base.timing;
    const baseVals: Record<string, number | undefined> = {
      dns: baseT.dns_us,
      connect: baseT.connect_us,
      tls: baseT.tls_us,
      ttfb: baseT.ttfb_us,
      download: baseT.download_us,
      total: baseT.total_us,
    };
    const out: string[] = [];
    for (const [k, dv] of Object.entries(p.delta.timing_us)) {
      const bv = baseVals[k];
      if (bv !== undefined && bv > 0 && Math.abs(dv) > 0.1 * bv) {
        out.push(`${k} ${dv > 0 ? '+' : '−'}${fmtUs(Math.abs(dv))}`);
      }
    }
    return out;
  })();

  let unregister: (() => void) | null = null;
  onMount(() => {
    unregister = registerKey('comparePair', (d) => step(d ?? 1));
  });
  onDestroy(() => unregister?.());
</script>

<div class="cmp">
  {#if configDisabled}
    <div class="confighint">
      <div class="ch-title">run history is disabled</div>
      <div class="ch-body at-mono">ui.history.enabled: false</div>
      <div class="ch-sub">enable it in curlew.yaml to record and compare runs</div>
    </div>
  {:else}
    <HistoryRail base={effectiveBase ?? null} target={effectiveTarget ?? null} on:pick={(e) => pickRun(e.detail)} />

    <div class="main">
      {#if effectiveBase === undefined || effectiveTarget === undefined}
        <div class="pickhint">
          <h1 class="hl" tabindex="-1">Compare runs</h1>
          <div class="sub">pick a baseline (A) in the rail — the newest run is B</div>
        </div>
      {:else if $comparePairs === null}
        <div class="center"><span class="at-spin"></span></div>
      {:else}
        <div class="pickrow">
          <RequestPicker {entries} selected={selectedKey} on:pick={(e) => pickPair(e.detail)} />
          <span class="sp"></span>
        </div>

        {#if selectedEntry !== null && selectedEntry.kind === 'pair' && selectedPair !== null}
          <div class="cards">
            <RunHeaderCard run={$comparePairs.base} side={selectedPair.base} />
            <div class="chev"><Icon name="chevron-right" size={12} /></div>
            <RunHeaderCard
              run={$comparePairs.target}
              side={selectedPair.target}
              deltaMs={selectedPair.delta?.duration_ms ?? null}
              {timingChips}
            />
          </div>
          {#if selectedPair.base !== null && selectedPair.target !== null}
            <DiffView baseRunId={effectiveBase} targetRunId={effectiveTarget} pair={selectedPair} />
          {/if}
        {:else if selectedEntry !== null}
          <div class="single">
            <div class="single-note at-mono">
              {selectedEntry.kind === 'only_base'
                ? 'this request exists only in run A — it was removed or renamed since'
                : 'this request exists only in run B — it was added since run A'}
            </div>
            <div class="cards">
              {#if selectedEntry.kind === 'only_base'}
                <RunHeaderCard run={$comparePairs.base} side={null} />
              {:else}
                <RunHeaderCard run={$comparePairs.target} side={null} />
              {/if}
            </div>
          </div>
        {:else}
          <div class="pickhint">
            <div class="sub">no overlapping requests between these runs</div>
          </div>
        {/if}
      {/if}
    </div>
  {/if}
</div>

<style>
  .cmp {
    display: flex;
    flex: 1;
    min-height: 0;
  }
  .main {
    flex: 1;
    min-width: 0;
    overflow: auto;
    padding: var(--pad);
    display: flex;
    flex-direction: column;
    gap: var(--pad);
  }
  .pickrow {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .sp {
    flex: 1;
  }
  .cards {
    display: flex;
    gap: var(--pad);
    align-items: stretch;
  }
  .chev {
    align-self: center;
    color: var(--fg3);
    flex: none;
    display: flex;
  }
  .center {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .pickhint {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }
  .hl {
    font: 600 var(--fs-lg) var(--font-sans);
    margin: 0;
    outline: none;
  }
  .sub {
    font-size: var(--fs-sm);
    color: var(--fg2);
  }
  .single {
    display: flex;
    flex-direction: column;
    gap: var(--pad);
  }
  .single-note {
    font-size: var(--fs-xs);
    color: var(--fg2);
  }
  .confighint {
    flex: 1;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
  }
  .ch-title {
    font: 600 var(--fs-lg) var(--font-sans);
  }
  .ch-body {
    font-size: var(--fs-sm);
    color: var(--fg2);
    background: var(--bg1);
    border: 1px solid var(--bd0);
    border-radius: var(--rad);
    padding: 6px 12px;
  }
  .ch-sub {
    font-size: var(--fs-sm);
    color: var(--fg2);
  }
</style>
