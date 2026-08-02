/* apitest UI — chrome: top bar + sidebar collection tree */

function AtTopBar({ env, setEnv, onRun, running, view, setView, watchEvent, interactive }) {
  const [envOpen, setEnvOpen] = React.useState(false);
  const [runOpen, setRunOpen] = React.useState(false);

  const bar = {
    display: 'flex', alignItems: 'center', gap: 10, height: 44, padding: '0 12px',
    background: 'var(--bg1)', borderBottom: '1px solid var(--bd0)', flex: 'none', position: 'relative', zIndex: 30,
  };

  return (
    <div style={bar}>
      {/* project + branch */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, minWidth: 0 }}>
        <span style={{ fontWeight: 600, fontSize: 'var(--fs-md)', whiteSpace: 'nowrap' }}>{AT.project}</span>
        <span className="at-chip" title="current git branch">{AtIcons.branch}{AT.branch}</span>
      </div>

      {/* watching indicator */}
      <div style={{ display: 'flex', alignItems: 'center', gap: 6, marginLeft: 4 }}
        title="apitest is watching the repo for file changes">
        <span style={{
          width: 7, height: 7, borderRadius: '50%', flex: 'none',
          background: watchEvent ? 'var(--warn)' : 'var(--fg3)',
          animation: watchEvent ? 'at-pulse 1s ease-out 2' : 'none',
        }}></span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: watchEvent ? 'var(--fg1)' : 'var(--fg3)', whiteSpace: 'nowrap' }}>
          {watchEvent ? watchEvent + ' changed · tree reloaded' : 'watching files'}
        </span>
      </div>

      <div style={{ flex: 1 }}></div>

      {/* history */}
      <button className={'at-btn ghost' + (view === 'compare' ? '' : '')}
        style={view === 'compare' ? { background: 'var(--bg3)', color: 'var(--fg0)' } : null}
        onClick={() => setView(view === 'compare' ? { screen: 'run' } : { screen: 'compare' })}>
        {AtIcons.clock} History
      </button>

      {/* environment switcher */}
      <div style={{ position: 'relative' }}>
        <button className="at-btn" onClick={() => setEnvOpen(!envOpen)} title="environments/*.yaml">
          <span style={{ color: 'var(--fg2)', fontWeight: 400 }}>env</span>
          <span className="at-mono" style={{ fontSize: 'var(--fs-xs)' }}>{env}</span>
          {AtIcons.caret}
        </button>
        {envOpen && (
          <React.Fragment>
            <div className="at-overlay" onClick={() => setEnvOpen(false)}></div>
            <div className="at-menu" style={{ right: 0, top: 32 }}>
              {AT.ENVS.map((e) => (
                <button key={e.id} className="at-menu-item" onClick={() => { setEnv(e.id); setEnvOpen(false); }}>
                  <span style={{ width: 12 }}>{env === e.id ? AtIcons.check : null}</span>
                  <span className="at-mono" style={{ fontSize: 'var(--fs-sm)' }}>{e.id}</span>
                  <span className="hint at-mono">{e.base.replace('https://', '')}</span>
                </button>
              ))}
              <div style={{ borderTop: '1px solid var(--bd0)', margin: '4px 0' }}></div>
              <div style={{ padding: '4px 8px', fontSize: 'var(--fs-xs)', color: 'var(--fg3)', fontFamily: 'var(--font-mono)' }}>
                environments/{env}.yaml
              </div>
            </div>
          </React.Fragment>
        )}
      </div>

      {/* run split button */}
      <div style={{ display: 'flex', position: 'relative' }}>
        <button className="at-btn primary" disabled={running} onClick={() => onRun('all')}
          style={{ borderRadius: 'var(--rad) 0 0 var(--rad)', opacity: running ? 0.6 : 1 }}>
          {running ? <span className="at-spin" style={{ borderTopColor: 'var(--acc-fg)' }}></span> : AtIcons.play}
          {running ? 'Running…' : 'Run all'}
        </button>
        <button className="at-btn primary" onClick={() => setRunOpen(!runOpen)}
          style={{ borderRadius: '0 var(--rad) var(--rad) 0', borderLeft: '1px solid color-mix(in srgb, var(--acc-fg) 25%, transparent)', padding: '0 5px' }}>
          {AtIcons.caret}
        </button>
        {runOpen && (
          <React.Fragment>
            <div className="at-overlay" onClick={() => setRunOpen(false)}></div>
            <div className="at-menu" style={{ right: 0, top: 32 }}>
              <button className="at-menu-item" onClick={() => { setRunOpen(false); onRun('all'); }}>Run all<span className="hint at-mono">14 requests</span></button>
              <button className="at-menu-item" onClick={() => { setRunOpen(false); onRun('file'); }}>Run current file</button>
              <button className="at-menu-item" onClick={() => { setRunOpen(false); onRun('selection'); }}>Run selection</button>
              <button className="at-menu-item" onClick={() => { setRunOpen(false); onRun('failed'); }}>Re-run failed</button>
            </div>
          </React.Fragment>
        )}
      </div>
      {!interactive && <div className="at-overlay" style={{ position: 'absolute', inset: 0, zIndex: 35 }}></div>}
    </div>
  );
}

/* ---------- sidebar ---------- */
function AtSidebar({ run, selection, onSelect, interactive }) {
  const [filter, setFilter] = React.useState('');
  const [collapsed, setCollapsed] = React.useState({});
  const q = filter.trim().toLowerCase();

  const fileAgg = (f) => {
    if (f.invalid) return null;
    const states = f.requests.map((id) => run.phases[id]);
    if (states.some((s) => s === 'running')) return 'running';
    if (states.every((s) => s === 'pending' || !s)) return 'pending';
    if (states.includes('fail')) return states.includes('pass') ? 'mixed' : 'fail';
    return 'pass';
  };

  const rowBase = {
    display: 'flex', alignItems: 'center', gap: 7, height: 'var(--row-h)',
    padding: '0 8px', borderRadius: 'var(--rad)', cursor: 'pointer', userSelect: 'none', minWidth: 0,
  };

  return (
    <div style={{
      width: 264, flex: 'none', display: 'flex', flexDirection: 'column',
      background: 'var(--bg1)', borderRight: '1px solid var(--bd0)', minHeight: 0,
    }}>
      <div style={{ padding: 8, borderBottom: '1px solid var(--bd0)' }}>
        <div style={{ position: 'relative' }}>
          <span style={{ position: 'absolute', left: 7, top: 6, color: 'var(--fg3)' }}>{AtIcons.search}</span>
          <input className="at-input" style={{ paddingLeft: 26 }} placeholder="Filter requests…"
            value={filter} onChange={(e) => setFilter(e.target.value)} />
        </div>
      </div>

      <div style={{ flex: 1, overflowY: 'auto', padding: 6 }}>
        {AT.FILES.map((f) => {
          const short = f.path.replace('collections/', '');
          const reqs = f.requests
            .map((id) => AT.REQUESTS[id])
            .filter((r) => !q || r.name.toLowerCase().includes(q) || f.path.toLowerCase().includes(q));
          if (q && reqs.length === 0 && !f.path.toLowerCase().includes(q)) return null;
          const open = !collapsed[f.path];
          const agg = fileAgg(f);
          const selectedFile = selection && selection.type === 'file' && selection.id === f.path;
          return (
            <div key={f.path} style={{ marginBottom: 2 }}>
              <div
                style={{
                  ...rowBase,
                  background: selectedFile ? 'var(--acc-dim)' : 'transparent',
                  color: f.invalid ? 'var(--fg1)' : 'var(--fg0)',
                }}
                onMouseEnter={(e) => { if (!selectedFile) e.currentTarget.style.background = 'var(--bg2)'; }}
                onMouseLeave={(e) => { if (!selectedFile) e.currentTarget.style.background = 'transparent'; }}
                onClick={() => {
                  if (!interactive) return;
                  if (f.invalid) onSelect({ type: 'file', id: f.path });
                  else setCollapsed({ ...collapsed, [f.path]: open });
                }}
                onDoubleClick={() => interactive && !f.invalid && onSelect({ type: 'file', id: f.path })}
              >
                <span style={{ color: 'var(--fg3)', display: 'flex' }}>{f.invalid ? null : AtIcons.chevron(open)}</span>
                <span className="at-mono" style={{
                  fontSize: 'var(--fs-sm)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap', flex: 1,
                }}>{short}</span>
                {f.invalid ? (
                  <span title={f.invalid.diag} style={{
                    display: 'inline-flex', alignItems: 'center', gap: 3, color: 'var(--warn)',
                    fontSize: 10, fontWeight: 600,
                  }}>{AtIcons.warn} invalid</span>
                ) : (
                  <React.Fragment>
                    <span style={{ fontSize: 10, color: 'var(--fg3)', fontFamily: 'var(--font-mono)' }}>{f.requests.length}</span>
                    {agg && agg !== 'pending' && <AtDot state={agg === 'running' ? 'running' : agg} />}
                  </React.Fragment>
                )}
              </div>

              {open && !f.invalid && reqs.map((r) => {
                const ph = run.phases[r.id] || 'pending';
                const sel = selection && selection.type === 'request' && selection.id === r.id;
                return (
                  <div key={r.id}
                    style={{
                      ...rowBase, paddingLeft: 27, gap: 8,
                      background: sel ? 'var(--acc-dim)' : 'transparent',
                    }}
                    onMouseEnter={(e) => { if (!sel) e.currentTarget.style.background = 'var(--bg2)'; }}
                    onMouseLeave={(e) => { if (!sel) e.currentTarget.style.background = 'transparent'; }}
                    onClick={() => interactive && onSelect({ type: 'request', id: r.id })}
                  >
                    <AtDot state={ph} />
                    <span style={{
                      fontSize: 'var(--fs-sm)', flex: 1, overflow: 'hidden', textOverflow: 'ellipsis',
                      whiteSpace: 'nowrap', color: sel ? 'var(--fg0)' : 'var(--fg1)',
                    }}>{r.name}</span>
                    <AtMethod m={r.method} />
                  </div>
                );
              })}
            </div>
          );
        })}
      </div>

      <div style={{
        padding: '7px 12px', borderTop: '1px solid var(--bd0)', display: 'flex', gap: 8,
        alignItems: 'center', color: 'var(--fg3)', fontSize: 'var(--fs-xs)', fontFamily: 'var(--font-mono)',
        whiteSpace: 'nowrap',
      }}>
        <span>5 files</span><span>·</span><span>14 requests</span>
        <span style={{ flex: 1 }}></span>
        <span title="read-only: files are the source of truth">read-only</span>
      </div>
    </div>
  );
}

Object.assign(window, { AtTopBar, AtSidebar });
