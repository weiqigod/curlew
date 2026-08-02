/* apitest UI — live run view: summary strip + wave layouts (columns / lanes / compact) */

function runCounts(run) {
  const c = { pass: 0, fail: 0, skip: 0, running: 0, pending: 0, total: 0 };
  AT.WAVES.flat().forEach((id) => {
    c.total++;
    const p = run.phases[id] || 'pending';
    c[p] = (c[p] || 0) + 1;
  });
  return c;
}

function AtSummaryStrip({ run, elapsedMs }) {
  const c = runCounts(run);
  const done = c.pass + c.fail + c.skip;
  const t = run.running ? elapsedMs : run.wallMs;
  const seg = (n) => (n / c.total) * 100 + '%';
  const item = (n, label, color) => (
    <span style={{ display: 'inline-flex', alignItems: 'baseline', gap: 5 }}>
      <span className="at-mono" style={{ fontWeight: 600, color: n > 0 ? color || 'var(--fg0)' : 'var(--fg3)' }}>{n}</span>
      <span style={{ color: 'var(--fg2)', fontSize: 'var(--fs-sm)' }}>{label}</span>
    </span>
  );
  return (
    <div style={{ flex: 'none', padding: 'var(--pad) var(--pad) 0' }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 14, paddingBottom: 'var(--pad-sm)' }}>
        {item(c.pass, 'passed', 'var(--ok)')}
        {item(c.fail, 'failed', 'var(--err)')}
        {item(c.skip, 'skipped')}
        {run.running && item(c.running, 'running', 'var(--warn)')}
        <span style={{ color: 'var(--fg3)' }}>·</span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-sm)', color: 'var(--fg1)', whiteSpace: 'nowrap' }}>
          {(t / 1000).toFixed(1)}s {run.running ? 'elapsed' : 'total'}
        </span>
        <span style={{ flex: 1 }}></span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', whiteSpace: 'nowrap' }}>
          {done}/{c.total} {run.running ? '· streaming' : '· run finished'}
        </span>
      </div>
      <div style={{ height: 3, borderRadius: 2, background: 'var(--bg3)', display: 'flex', overflow: 'hidden' }}>
        <div style={{ width: seg(c.pass), background: 'var(--ok)', transition: 'width .3s' }}></div>
        <div style={{ width: seg(c.fail), background: 'var(--err)', transition: 'width .3s' }}></div>
        <div style={{ width: seg(c.skip), background: 'var(--fg3)', transition: 'width .3s' }}></div>
        <div style={{ width: seg(c.running), background: 'var(--warn)', opacity: 0.7, transition: 'width .3s' }}></div>
      </div>
    </div>
  );
}

function AtRequestCard({ id, run, onOpen, compactRow }) {
  const req = AT.REQUESTS[id];
  const ph = run.phases[id] || 'pending';
  const out = run.resolved[id] || {};
  const clickable = ph === 'pass' || ph === 'fail';
  const dim = ph === 'pending' || ph === 'skip';

  if (compactRow) {
    return (
      <div onClick={() => clickable && onOpen(id)}
        style={{
          display: 'flex', alignItems: 'center', gap: 10, height: 'var(--row-h)', padding: '0 10px',
          borderBottom: '1px solid var(--bd0)', cursor: clickable ? 'pointer' : 'default',
          opacity: dim ? 0.55 : 1, background: 'transparent',
        }}
        onMouseEnter={(e) => { if (clickable) e.currentTarget.style.background = 'var(--bg2)'; }}
        onMouseLeave={(e) => { e.currentTarget.style.background = 'transparent'; }}>
        <AtDot state={ph} />
        <AtMethod m={req.method} />
        <span style={{ fontSize: 'var(--fs-sm)', whiteSpace: 'nowrap' }}>{req.name}</span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', whiteSpace: 'nowrap' }}>
          {req.file.replace('collections/', '')}
        </span>
        {ph === 'fail' && (
          <span className="at-mono" style={{
            fontSize: 'var(--fs-xs)', color: 'var(--err)', overflow: 'hidden',
            textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1,
          }}>{out.msg}</span>
        )}
        <span style={{ flex: ph === 'fail' ? 'none' : 1 }}></span>
        {(ph === 'pass' || ph === 'fail') && (
          <React.Fragment>
            <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg2)' }}>{fmtMs(out.ms)}</span>
            <AtCode code={out.code} />
          </React.Fragment>
        )}
        {ph === 'skip' && <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>skipped</span>}
      </div>
    );
  }

  return (
    <div onClick={() => clickable && onOpen(id)}
      data-comment-anchor={'card-' + id}
      style={{
        background: 'var(--bg2)', border: '1px solid ' + (ph === 'running' ? 'var(--bd2)' : 'var(--bd0)'),
        borderRadius: 'var(--rad)', padding: 'var(--pad-sm) 10px',
        cursor: clickable ? 'pointer' : 'default', opacity: dim ? 0.55 : 1,
        display: 'flex', flexDirection: 'column', gap: 4, transition: 'opacity .25s, border-color .25s',
      }}
      onMouseEnter={(e) => { if (clickable) e.currentTarget.style.borderColor = 'var(--bd2)'; }}
      onMouseLeave={(e) => { e.currentTarget.style.borderColor = ph === 'running' ? 'var(--bd2)' : 'var(--bd0)'; }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <AtDot state={ph} />
        <span style={{
          fontSize: 'var(--fs-sm)', fontWeight: 500, flex: 1, overflow: 'hidden',
          textOverflow: 'ellipsis', whiteSpace: 'nowrap',
        }}>{req.name}</span>
        <AtMethod m={req.method} />
      </div>
      <div className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', paddingLeft: 16 }}>
        {req.file.replace('collections/', '')}
      </div>
      {(ph === 'pass' || ph === 'fail') && (
        <div className="at-mono" style={{ display: 'flex', gap: 8, paddingLeft: 16, fontSize: 'var(--fs-xs)', color: 'var(--fg2)', whiteSpace: 'nowrap', animation: run.animate ? 'at-fadein .25s' : 'none' }}>
          <span>{fmtMs(out.ms)}</span>
          <span style={{ color: 'var(--fg3)' }}>·</span>
          <AtCode code={out.code} />
        </div>
      )}
      {ph === 'running' && (
        <div className="at-mono" style={{ paddingLeft: 16, fontSize: 'var(--fs-xs)', color: 'var(--warn)' }}>running…</div>
      )}
      {ph === 'skip' && (
        <div className="at-mono" style={{ paddingLeft: 16, fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>{out.msg || 'skipped'}</div>
      )}
      {ph === 'fail' && out.msg && (
        <div className="at-mono" style={{
          margin: '2px 0 1px 16px', padding: '4px 7px', fontSize: 'var(--fs-xs)', lineHeight: 1.45,
          color: 'var(--err)', background: 'color-mix(in srgb, var(--err) 9%, transparent)',
          border: '1px solid color-mix(in srgb, var(--err) 25%, transparent)', borderRadius: 3,
          animation: run.animate ? 'at-fadein .25s' : 'none',
        }}>{out.msg}</div>
      )}
    </div>
  );
}

function AtRunView({ run, elapsedMs, layout, fileFilter, onOpen, onClearFilter }) {
  const waves = AT.WAVES.map((w) => (fileFilter ? w.filter((id) => AT.REQUESTS[id].file === fileFilter) : w));
  const waveLabel = (i, w) => (
    <div style={{ display: 'flex', alignItems: 'center', gap: 8, whiteSpace: 'nowrap' }}>
      <span style={{ fontSize: 'var(--fs-xs)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.07em', color: 'var(--fg2)' }}>
        Wave {i + 1}
      </span>
      <span className="at-mono" style={{ fontSize: 10, color: 'var(--fg3)' }}>{w.length} req</span>
    </div>
  );

  return (
    <div data-screen-label="Live run view" style={{ display: 'flex', flexDirection: 'column', minHeight: 0, flex: 1 }}>
      <AtSummaryStrip run={run} elapsedMs={elapsedMs} />
      {fileFilter && (
        <div style={{ padding: 'var(--pad-sm) var(--pad) 0', display: 'flex' }}>
          <span className="at-chip" style={{ gap: 6 }}>
            filtered: {fileFilter.replace('collections/', '')}
            <button onClick={onClearFilter} style={{ all: 'unset', cursor: 'pointer', display: 'flex', color: 'var(--fg2)' }}>{AtIcons.x}</button>
          </span>
        </div>
      )}

      {layout === 'columns' && (
        <div style={{ flex: 1, overflow: 'auto', display: 'flex', gap: 0, padding: 'var(--pad)', alignItems: 'stretch', minHeight: 0 }}>
          {waves.map((w, i) => w.length > 0 && (
            <React.Fragment key={i}>
              {i > 0 && (
                <div style={{ flex: 'none', display: 'flex', alignItems: 'flex-start', padding: '34px 6px 0', color: 'var(--fg3)' }}>
                  {AtIcons.chevron(false)}
                </div>
              )}
              <div style={{ width: 252, flex: 'none', display: 'flex', flexDirection: 'column', gap: 'var(--pad-sm)' }}>
                {waveLabel(i, w)}
                {w.map((id) => <AtRequestCard key={id} id={id} run={run} onOpen={onOpen} />)}
              </div>
            </React.Fragment>
          ))}
        </div>
      )}

      {layout === 'lanes' && (
        <div style={{ flex: 1, overflow: 'auto', padding: 'var(--pad)', display: 'flex', flexDirection: 'column', gap: 'var(--pad)' }}>
          {waves.map((w, i) => w.length > 0 && (
            <div key={i} style={{ display: 'flex', gap: 'var(--pad)', alignItems: 'flex-start' }}>
              <div style={{ width: 76, flex: 'none', paddingTop: 6 }}>{waveLabel(i, w)}</div>
              <div style={{
                flex: 1, display: 'grid', gridTemplateColumns: 'repeat(auto-fill, minmax(236px, 1fr))',
                gap: 'var(--pad-sm)', borderLeft: '1px solid var(--bd0)', paddingLeft: 'var(--pad)',
              }}>
                {w.map((id) => <AtRequestCard key={id} id={id} run={run} onOpen={onOpen} />)}
              </div>
            </div>
          ))}
        </div>
      )}

      {layout === 'compact' && (
        <div style={{ flex: 1, overflow: 'auto', padding: 'var(--pad)' }}>
          <div style={{ border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', overflow: 'hidden', background: 'var(--bg1)' }}>
            {waves.map((w, i) => w.length > 0 && (
              <React.Fragment key={i}>
                <div style={{
                  display: 'flex', alignItems: 'center', gap: 8, padding: '5px 10px',
                  background: 'var(--bg2)', borderBottom: '1px solid var(--bd0)',
                  borderTop: i > 0 ? '1px solid var(--bd0)' : 'none',
                }}>
                  {waveLabel(i, w)}
                </div>
                {w.map((id) => <AtRequestCard key={id} id={id} run={run} onOpen={onOpen} compactRow />)}
              </React.Fragment>
            ))}
          </div>
        </div>
      )}
    </div>
  );
}

Object.assign(window, { AtRunView, AtSummaryStrip, AtRequestCard, runCounts });
