/* apitest UI — run comparison: history rail + side-by-side run header + unified JSON diff */

function AtRunRow({ r, role, onPick }) {
  return (
    <div onClick={() => onPick && onPick(r.id)}
      style={{
        display: 'flex', flexDirection: 'column', gap: 3, padding: '7px 9px',
        borderRadius: 'var(--rad)', cursor: r.current ? 'default' : 'pointer',
        background: role ? 'var(--acc-dim)' : 'transparent',
        border: '1px solid ' + (role ? 'var(--bd2)' : 'transparent'),
      }}
      onMouseEnter={(e) => { if (!role) e.currentTarget.style.background = 'var(--bg2)'; }}
      onMouseLeave={(e) => { if (!role) e.currentTarget.style.background = 'transparent'; }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 7 }}>
        <span className="at-mono" style={{ fontSize: 'var(--fs-sm)', color: 'var(--fg0)' }}>{r.time}</span>
        <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>{r.day}</span>
        <span style={{ flex: 1 }}></span>
        {role && <span style={{
          fontSize: 9, fontWeight: 700, letterSpacing: '0.08em', color: 'var(--acc-fg)',
          background: 'var(--acc)', borderRadius: 3, padding: '1px 5px',
        }}>{role}</span>}
      </div>
      <div className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg2)', display: 'flex', alignItems: 'center', gap: 4 }}>
        {AtIcons.branch}<span style={{ overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>{r.branch}</span>
      </div>
      <div className="at-mono" style={{ fontSize: 'var(--fs-xs)', display: 'flex', gap: 8, whiteSpace: 'nowrap' }}>
        <span style={{ color: 'var(--ok)' }}>{r.pass}✓</span>
        {r.fail > 0 && <span style={{ color: 'var(--err)' }}>{r.fail}✗</span>}
        {r.skip > 0 && <span style={{ color: 'var(--fg3)' }}>{r.skip}–</span>}
        <span style={{ color: 'var(--fg3)' }}>· {r.total}</span>
        {r.current && <span style={{ color: 'var(--fg3)' }}>· current</span>}
      </div>
    </div>
  );
}

function AtRunHeaderCard({ r, status, code, ms, delta, failMsg }) {
  return (
    <div style={{ flex: 1, minWidth: 0, background: 'var(--bg2)', border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', padding: '9px 12px', display: 'flex', flexDirection: 'column', gap: 5 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8 }}>
        <span className="at-mono" style={{ fontSize: 'var(--fs-sm)', fontWeight: 600 }}>{r.time}</span>
        <span style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', whiteSpace: 'nowrap' }}>{r.day} on</span>
        <span className="at-chip">{AtIcons.branch}{r.branch}</span>
        <span style={{ flex: 1 }}></span>
        <span style={{ display: 'inline-flex', alignItems: 'center', gap: 5, fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em', color: status === 'pass' ? 'var(--ok)' : 'var(--err)' }}>
          <AtDot state={status} /> {status === 'pass' ? 'passed' : 'failed'}
        </span>
      </div>
      <div className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg1)', display: 'flex', gap: 8, alignItems: 'baseline', whiteSpace: 'nowrap' }}>
        <AtCode code={code} />
        <span style={{ color: 'var(--fg3)' }}>·</span>
        <span style={{ whiteSpace: 'nowrap' }}>{fmtMs(ms)}</span>
        {delta != null && (
          <span style={{ color: 'var(--fg2)', background: 'var(--bg3)', border: '1px solid var(--bd1)', borderRadius: 3, padding: '0 5px', whiteSpace: 'nowrap' }}>
            {delta > 0 ? '+' : ''}{delta} ms
          </span>
        )}
      </div>
      {failMsg && <div className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--err)' }}>{failMsg}</div>}
    </div>
  );
}

function AtDiffLine({ line, i }) {
  const bg = line.t === '-' ? 'color-mix(in srgb, var(--err) 10%, transparent)'
    : line.t === '+' ? 'color-mix(in srgb, var(--ok) 10%, transparent)' : 'transparent';
  const sign = line.t === ' ' ? '' : line.t;
  let content = line.s;
  if (line.k) {
    const idx = line.s.lastIndexOf(line.k);
    content = (
      <React.Fragment>
        {line.s.slice(0, idx)}
        <span style={{
          background: line.t === '-' ? 'color-mix(in srgb, var(--err) 28%, transparent)' : 'color-mix(in srgb, var(--ok) 28%, transparent)',
          borderRadius: 2, padding: '0 1px',
        }}>{line.k}</span>
        {line.s.slice(idx + line.k.length)}
      </React.Fragment>
    );
  }
  return (
    <div style={{ display: 'flex', background: bg, minHeight: 19 }}>
      <span style={{ width: 38, flex: 'none', textAlign: 'right', paddingRight: 10, color: 'var(--fg3)', userSelect: 'none' }}>{i + 1}</span>
      <span style={{ width: 16, flex: 'none', color: line.t === '-' ? 'var(--err)' : line.t === '+' ? 'var(--ok)' : 'var(--fg3)', userSelect: 'none' }}>{sign}</span>
      <span style={{ whiteSpace: 'pre', color: 'var(--fg1)' }}>{content}</span>
    </div>
  );
}

function AtCompare({ onOpenRequest }) {
  const [baseline, setBaseline] = React.useState('run-2');
  const [reqId, setReqId] = React.useState(AT.COMPARE.request);
  const [reqOpen, setReqOpen] = React.useState(false);
  const [onlyChanges, setOnlyChanges] = React.useState(false);

  const current = AT.RUNS.find((r) => r.current);
  const a = AT.RUNS.find((r) => r.id === baseline);
  const isCanonical = reqId === AT.COMPARE.request && baseline === 'run-2';
  const req = AT.REQUESTS[reqId];
  const cmp = AT.COMPARE;
  const lines = onlyChanges ? cmp.diff.filter((l) => l.t !== ' ') : cmp.diff;

  return (
    <div data-screen-label="Run comparison" style={{ display: 'flex', flex: 1, minHeight: 0 }}>
      {/* history rail */}
      <div style={{ width: 224, flex: 'none', borderRight: '1px solid var(--bd0)', background: 'var(--bg1)', display: 'flex', flexDirection: 'column', minHeight: 0 }}>
        <div style={{ padding: '9px 12px 5px', fontSize: 'var(--fs-xs)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.07em', color: 'var(--fg2)' }}>
          Recent runs
        </div>
        <div style={{ padding: '0 12px 7px', fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>
          pick a baseline to diff against the current run
        </div>
        <div style={{ flex: 1, overflowY: 'auto', padding: 6, display: 'flex', flexDirection: 'column', gap: 2 }}>
          {AT.RUNS.map((r) => (
            <AtRunRow key={r.id} r={r}
              role={r.current ? 'B' : r.id === baseline ? 'A' : null}
              onPick={(id) => { if (!r.current) setBaseline(id); }} />
          ))}
        </div>
      </div>

      {/* diff area */}
      <div style={{ flex: 1, minWidth: 0, overflow: 'auto', padding: 'var(--pad)', display: 'flex', flexDirection: 'column', gap: 'var(--pad)' }}>
        {/* request picker */}
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <div style={{ position: 'relative' }}>
            <button className="at-btn" onClick={() => setReqOpen(!reqOpen)}>
              <AtMethod m={req.method} />
              <span style={{ fontWeight: 600 }}>{req.name}</span>
              {AtIcons.caret}
            </button>
            {reqOpen && (
              <React.Fragment>
                <div className="at-overlay" onClick={() => setReqOpen(false)}></div>
                <div className="at-menu" style={{ left: 0, top: 32, maxHeight: 320, overflowY: 'auto', minWidth: 260 }}>
                  {AT.REQ_LIST.map((r) => (
                    <button key={r.id} className="at-menu-item" onClick={() => { setReqId(r.id); setReqOpen(false); }}>
                      <span style={{ width: 12 }}>{r.id === reqId ? AtIcons.check : null}</span>
                      <AtMethod m={r.method} />
                      <span>{r.name}</span>
                    </button>
                  ))}
                </div>
              </React.Fragment>
            )}
          </div>
          <span className="at-mono at-xs at-dim">{req.file}:{req.line}</span>
          <AtOpenInEditor file={req.file} line={req.line} compact />
          <span style={{ flex: 1 }}></span>
          {isCanonical && (
            <label style={{ display: 'flex', alignItems: 'center', gap: 6, fontSize: 'var(--fs-xs)', color: 'var(--fg2)', cursor: 'pointer', userSelect: 'none' }}>
              <input type="checkbox" checked={onlyChanges} onChange={(e) => setOnlyChanges(e.target.checked)} style={{ accentColor: 'var(--acc)' }} />
              changes only
            </label>
          )}
        </div>

        {/* run header cards */}
        <div style={{ display: 'flex', gap: 'var(--pad)', alignItems: 'stretch' }}>
          <AtRunHeaderCard r={a} status={isCanonical ? cmp.a.status : 'pass'} code={isCanonical ? cmp.a.code : AT.OUTCOMES[reqId].code} ms={isCanonical ? cmp.a.ms : AT.OUTCOMES[reqId].ms} />
          <div style={{ alignSelf: 'center', color: 'var(--fg3)', flex: 'none' }}>{AtIcons.chevron(false)}</div>
          <AtRunHeaderCard r={current}
            status={isCanonical ? cmp.b.status : 'pass'}
            code={isCanonical ? cmp.b.code : AT.OUTCOMES[reqId].code}
            ms={isCanonical ? cmp.b.ms : AT.OUTCOMES[reqId].ms + 12}
            delta={isCanonical ? cmp.b.ms - cmp.a.ms : 12}
            failMsg={isCanonical ? cmp.b.failMsg : null} />
        </div>

        {/* body diff */}
        {isCanonical ? (
          <div style={{ border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', background: 'var(--bg1)', overflow: 'hidden' }}>
            <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: '6px 10px', borderBottom: '1px solid var(--bd0)', background: 'var(--bg2)' }}>
              <span style={{ fontSize: 'var(--fs-xs)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.07em', color: 'var(--fg2)' }}>Response body diff</span>
              <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>{cmp.changedKeys} keys changed</span>
              <span style={{ flex: 1 }}></span>
              <span className="at-mono" style={{ fontSize: 'var(--fs-xs)' }}>
                <span style={{ color: 'var(--err)' }}>−{cmp.diff.filter((l) => l.t === '-').length}</span>
                {' '}
                <span style={{ color: 'var(--ok)' }}>+{cmp.diff.filter((l) => l.t === '+').length}</span>
              </span>
            </div>
            <div className="at-mono" style={{ fontSize: 'var(--fs-sm)', lineHeight: 1.5, padding: '6px 0', overflowX: 'auto' }}>
              {lines.map((l, i) => <AtDiffLine key={i} line={l} i={i} />)}
            </div>
          </div>
        ) : (
          <div style={{
            border: '1px dashed var(--bd1)', borderRadius: 'var(--rad)', padding: '28px 20px',
            display: 'flex', flexDirection: 'column', alignItems: 'center', gap: 6,
          }}>
            <span style={{ color: 'var(--ok)', display: 'flex' }}>{AtIcons.check}</span>
            <span style={{ fontSize: 'var(--fs-sm)', color: 'var(--fg1)' }}>Response bodies are identical between these runs</span>
            <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>status, headers and assertions unchanged</span>
          </div>
        )}
      </div>
    </div>
  );
}

Object.assign(window, { AtCompare, AtRunRow, AtRunHeaderCard });
