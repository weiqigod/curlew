<script lang="ts">
  // Timing tab (§10.6.3.4): µs waterfall with sequential offsets, absent
  // phases omitted, >5% unaccounted → hatched "other", reused-connection and
  // nil-timing degraded views, Attempts section with backoff chips.
  import { fmtMs, fmtUs } from '../../format';
  import type { Retry, Timing } from '../../types/run';
  import Code from '../atoms/Code.svelte';

  export let timing: Timing | null;
  export let retry: Retry | null = null;
  export let durationMs: number | undefined = undefined;

  interface PhaseRow {
    label: string;
    us: number;
    strong?: boolean;
    hatched?: boolean;
  }

  function buildRows(t: Timing): PhaseRow[] {
    const rows: PhaseRow[] = [];
    if (!t.connection_reused) {
      if (t.dns_us !== undefined) rows.push({ label: 'DNS lookup', us: t.dns_us });
      if (t.connect_us !== undefined) rows.push({ label: 'TCP connect', us: t.connect_us });
      if (t.tls_us !== undefined) rows.push({ label: 'TLS handshake', us: t.tls_us });
    }
    if (t.ttfb_us !== undefined) rows.push({ label: 'Waiting (TTFB)', us: t.ttfb_us, strong: true });
    if (t.download_us !== undefined) rows.push({ label: 'Content download', us: t.download_us });
    const accounted = rows.reduce((s, r) => s + r.us, 0);
    const other = t.total_us - accounted;
    if (other > 0.05 * t.total_us) {
      rows.push({ label: 'other', us: other, hatched: true });
    }
    return rows;
  }

  $: rows = timing !== null ? buildRows(timing) : [];
  $: total = timing?.total_us ?? 0;

  function offsets(list: PhaseRow[]): number[] {
    const out: number[] = [];
    let acc = 0;
    for (const r of list) {
      out.push(acc);
      acc += r.us;
    }
    return out;
  }
  $: offs = offsets(rows);

  function pct(us: number): number {
    return total > 0 ? (us / total) * 100 : 0;
  }
</script>

<div class="tt">
  {#if timing === null}
    <div class="nil">
      <div class="wf">
        <div class="row">
          <span class="lbl at-mono">total</span>
          <div class="track"><div class="seg full"></div></div>
          <span class="val at-mono">{durationMs !== undefined ? fmtMs(durationMs) : '—'}</span>
        </div>
      </div>
      <div class="caption at-mono">phase timing not available for this request type</div>
    </div>
  {:else}
    {#if timing.connection_reused}
      <div class="caption at-mono reused">connection reused — no DNS/connect/TLS phases</div>
    {/if}
    <div class="wf">
      {#each rows as r, i (r.label)}
        <div class="row">
          <span class="lbl at-mono">{r.label}</span>
          <div class="track">
            <div
              class="seg"
              class:strong={r.strong}
              class:hatched={r.hatched}
              style={`left:${pct(offs[i])}%;width:${Math.max(0.6, pct(r.us))}%`}
            ></div>
          </div>
          <span class="val at-mono">{fmtUs(r.us)}</span>
        </div>
      {/each}
      <div class="totalrow">
        <span class="lbl at-mono total">total</span>
        <span></span>
        <span class="val at-mono total">{fmtUs(total)}</span>
      </div>
    </div>
  {/if}

  {#if retry !== null && retry.count > 0}
    <div class="attempts">
      <div class="ah">Attempts ({retry.attempts.length})</div>
      {#each retry.attempts as a, i (a.number)}
        {#if i > 0 && a.delay_ms > 0}
          <div class="backoffrow">
            <span class="at-chip">+{a.delay_ms} ms backoff</span>
          </div>
        {/if}
        <div class="arow at-mono">
          <span class="anum">#{a.number}</span>
          {#if a.error !== null && a.error !== ''}
            <span class="aerr">{a.error}</span>
          {:else if a.status_code > 0}
            <Code code={a.status_code} />
          {:else}
            <span class="adim">—</span>
          {/if}
          <span class="sp"></span>
          <span class="adur">{fmtMs(a.duration_ms)}</span>
        </div>
      {/each}
      <div class="caption at-mono">the waterfall above describes the final attempt</div>
      {#each retry.warnings as w}
        <div class="warnline at-mono">{w}</div>
      {/each}
    </div>
  {/if}
</div>

<style>
  .tt {
    overflow: auto;
    flex: 1;
    padding: var(--pad);
  }
  .wf {
    display: flex;
    flex-direction: column;
    gap: 2px;
    max-width: 760px;
  }
  .row {
    display: grid;
    grid-template-columns: 150px 1fr 72px;
    gap: 10px;
    align-items: center;
    height: 24px;
  }
  .lbl {
    font-size: var(--fs-xs);
    color: var(--fg2);
    text-align: right;
  }
  .track {
    position: relative;
    height: 10px;
    background: var(--bg2);
    border-radius: 2px;
  }
  .seg {
    position: absolute;
    top: 0;
    bottom: 0;
    background: var(--bd2);
    border-radius: 2px;
  }
  .seg.strong {
    background: var(--fg1);
  }
  .seg.hatched {
    background: repeating-linear-gradient(
      45deg,
      var(--bd2),
      var(--bd2) 3px,
      transparent 3px,
      transparent 6px
    );
  }
  .seg.full {
    left: 0;
    width: 100%;
  }
  .val {
    font-size: var(--fs-xs);
    color: var(--fg1);
    text-align: right;
  }
  .totalrow {
    display: grid;
    grid-template-columns: 150px 1fr 72px;
    gap: 10px;
    margin-top: 6px;
    padding-top: 8px;
    border-top: 1px solid var(--bd0);
  }
  .lbl.total,
  .val.total {
    color: var(--fg0);
    font-weight: 600;
  }
  .caption {
    font-size: var(--fs-xs);
    color: var(--fg3);
    margin-top: 8px;
  }
  .caption.reused {
    margin: 0 0 8px;
    color: var(--fg2);
  }
  .nil .caption {
    margin-top: 6px;
  }
  .attempts {
    margin-top: calc(var(--pad) * 1.6);
    max-width: 760px;
    display: flex;
    flex-direction: column;
    gap: 3px;
  }
  .ah {
    font: 600 var(--fs-xs) var(--font-sans);
    text-transform: uppercase;
    letter-spacing: 0.07em;
    color: var(--fg2);
    margin-bottom: 4px;
  }
  .arow {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: var(--fs-xs);
    border: 1px solid var(--bd0);
    background: var(--bg1);
    border-radius: var(--rad);
    padding: 5px 10px;
  }
  .anum {
    color: var(--fg2);
    width: 26px;
  }
  .aerr {
    color: var(--err2);
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .adim {
    color: var(--fg3);
  }
  .sp {
    flex: 1;
  }
  .adur {
    color: var(--fg2);
  }
  .backoffrow {
    display: flex;
    padding-left: 10px;
  }
  .warnline {
    font-size: var(--fs-xs);
    color: var(--warn);
  }
</style>
