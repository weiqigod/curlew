/* apitest UI — response inspector: Body / Headers / Assertions / Timing / Request */

const atInspectorCss = `
.jt-row { display:flex; align-items:center; gap:4px; min-height:20px; padding-right:8px;
  font:400 var(--fs-sm) var(--font-mono); white-space:nowrap; border-radius:3px; }
.jt-row:hover { background: var(--bg2); }
.jt-row .jt-copy { visibility:hidden; margin-left:2px; }
.jt-row:hover .jt-copy { visibility:visible; }
.jt-key { color: var(--fg0); }
.jt-val-str { color: var(--fg1); }
.jt-val-num { color: var(--fg1); }
.jt-val-null { color: var(--fg3); font-style: italic; }
.jt-pn { color: var(--fg3); }
.jt-preview { color: var(--fg3); cursor: pointer; }
.jt-preview:hover { color: var(--fg2); }
.jt-chev { all:unset; cursor:pointer; width:14px; height:14px; display:inline-flex;
  align-items:center; justify-content:center; color:var(--fg3); flex:none; }
.jt-chev:hover { color: var(--fg1); }
.jt-hit { background: color-mix(in srgb, var(--warn) 14%, transparent); }
.jt-more { all:unset; cursor:pointer; font:400 var(--fs-xs) var(--font-mono); color:var(--fg2);
  padding:2px 6px; border:1px dashed var(--bd1); border-radius:3px; margin:2px 0; }
.jt-more:hover { color:var(--fg0); border-color:var(--bd2); }
.at-kv { display:grid; grid-template-columns: 220px 1fr; gap: 0; }
.at-kv > div { padding: 5px 10px; border-bottom: 1px solid var(--bd0); font:400 var(--fs-sm) var(--font-mono); }
.at-kv .k { color: var(--fg2); }
.at-seg { display:inline-flex; border:1px solid var(--bd1); border-radius:var(--rad); overflow:hidden; }
.at-seg button { all:unset; cursor:pointer; padding:3px 9px; font:500 var(--fs-xs) var(--font-sans); color:var(--fg2); }
.at-seg button.on { background:var(--bg4); color:var(--fg0); }
`;

/* ---------- JSON tree ---------- */
function jtMatches(data, q) {
  const matches = new Set(), force = new Set();
  if (!q) return { matches, force };
  const walk = (node, path) => {
    let any = false;
    const entries = Array.isArray(node) ? node.map((v, i) => [i, v]) : Object.entries(node);
    for (const [k, v] of entries) {
      const p = path + (Array.isArray(node) ? '[' + k + ']' : '.' + k);
      const isObj = v !== null && typeof v === 'object';
      let childAny = false;
      if (isObj) childAny = walk(v, p);
      const hit = String(k).toLowerCase().includes(q) || (!isObj && JSON.stringify(v).toLowerCase().includes(q));
      if (hit) { matches.add(p); any = true; }
      if (childAny) { force.add(p); any = true; }
    }
    return any;
  };
  walk(data, '$');
  return { matches, force };
}

function JtVal({ v }) {
  if (v === null) return <span className="jt-val-null">null</span>;
  if (typeof v === 'string') {
    const s = v.length > 64 ? v.slice(0, 64) + '…' : v;
    return <span className="jt-val-str">"{s}"</span>;
  }
  if (typeof v === 'boolean' || typeof v === 'number') return <span className="jt-val-num">{String(v)}</span>;
  return <span className="jt-val-str">{String(v)}</span>;
}

function JtNode({ k, v, path, depth, ctx }) {
  const isObj = v !== null && typeof v === 'object';
  const isArr = Array.isArray(v);
  const expanded = isObj && ctx.isExpanded(path, depth, v);
  const hit = ctx.matches.has(path);
  const indent = { paddingLeft: depth * 14 + 8 };
  const keyEl = k != null && <span className="jt-key">{isArr || typeof k === 'number' ? <span className="jt-pn">{k}</span> : '"' + k + '"'}<span className="jt-pn">: </span></span>;

  if (!isObj) {
    return (
      <div className={'jt-row' + (hit ? ' jt-hit' : '')} style={indent}>
        <span style={{ width: 14, flex: 'none' }}></span>
        {keyEl}<JtVal v={v} />
        <span className="jt-copy"><AtCopyBtn text={'body.' + path} label="path" /></span>
      </div>
    );
  }

  const entries = isArr ? v.map((x, i) => [i, x]) : Object.entries(v);
  const count = entries.length;
  const truncated = isArr && expanded && count > 20 && !ctx.showAll[path];
  const visible = truncated ? entries.slice(0, 20) : entries;

  return (
    <React.Fragment>
      <div className={'jt-row' + (hit ? ' jt-hit' : '')} style={indent}>
        <button className="jt-chev" onClick={() => ctx.toggle(path, expanded)}>{AtIcons.chevron(expanded)}</button>
        {keyEl}
        {expanded
          ? <span className="jt-pn">{isArr ? '[' : '{'}</span>
          : <span className="jt-preview" onClick={() => ctx.toggle(path, false)}>
              {isArr ? '[…] ' + count + ' items' : '{…} ' + count + ' keys'}
            </span>}
        <span className="jt-copy"><AtCopyBtn text={'body.' + path} label="path" /></span>
      </div>
      {expanded && visible.map(([ck, cv]) => (
        <JtNode key={ck} k={ck} v={cv} depth={depth + 1}
          path={path + (isArr ? '[' + ck + ']' : '.' + ck)} ctx={ctx} />
      ))}
      {expanded && truncated && (
        <div style={{ paddingLeft: (depth + 1) * 14 + 8 }}>
          <button className="jt-more" onClick={() => ctx.setShowAll({ ...ctx.showAll, [path]: true })}>
            … show {count - 20} more items
          </button>
        </div>
      )}
      {expanded && (
        <div className="jt-row" style={indent}>
          <span style={{ width: 14, flex: 'none' }}></span>
          <span className="jt-pn">{isArr ? ']' : '}'}</span>
        </div>
      )}
    </React.Fragment>
  );
}

function AtJsonBody({ body }) {
  const [q, setQ] = React.useState('');
  const [raw, setRaw] = React.useState(false);
  const [mode, setMode] = React.useState('default'); // default | all | none
  const [overrides, setOverrides] = React.useState({});
  const [showAll, setShowAll] = React.useState({});
  const query = q.trim().toLowerCase();
  const { matches, force } = React.useMemo(() => jtMatches(body, query), [body, query]);

  const ctx = {
    matches, force, showAll, setShowAll,
    isExpanded: (path, depth, v) => {
      if (force.has(path)) return true;
      if (path in overrides) return overrides[path];
      if (mode === 'all') return true;
      if (mode === 'none') return depth === 0;
      return depth < 2 && !(Array.isArray(v) && v.length > 10);
    },
    toggle: (path, cur) => setOverrides({ ...overrides, [path]: !cur }),
  };

  return (
    <div style={{ display: 'flex', flexDirection: 'column', minHeight: 0, flex: 1 }}>
      <div style={{ display: 'flex', alignItems: 'center', gap: 8, padding: 'var(--pad-sm) var(--pad)', flex: 'none' }}>
        <div style={{ position: 'relative', width: 240 }}>
          <span style={{ position: 'absolute', left: 7, top: 6, color: 'var(--fg3)' }}>{AtIcons.search}</span>
          <input className="at-input" style={{ paddingLeft: 26 }} placeholder="Search keys & values…"
            value={q} onChange={(e) => setQ(e.target.value)} />
        </div>
        {query && <span className="at-mono at-xs at-dim">{matches.size} match{matches.size === 1 ? '' : 'es'}</span>}
        <span style={{ flex: 1 }}></span>
        <button className="at-btn sm ghost" onClick={() => { setMode('all'); setOverrides({}); }}>Expand all</button>
        <button className="at-btn sm ghost" onClick={() => { setMode('none'); setOverrides({}); }}>Collapse all</button>
        <div className="at-seg">
          <button className={!raw ? 'on' : ''} onClick={() => setRaw(false)}>Pretty</button>
          <button className={raw ? 'on' : ''} onClick={() => setRaw(true)}>Raw</button>
        </div>
        <AtCopyBtn text={JSON.stringify(body, null, 2)} label="body" />
      </div>
      <div style={{ flex: 1, overflow: 'auto', padding: '0 var(--pad) var(--pad)' }}>
        {raw ? (
          <pre className="at-mono" style={{
            margin: 0, fontSize: 'var(--fs-sm)', lineHeight: 1.5, color: 'var(--fg1)',
            whiteSpace: 'pre-wrap', wordBreak: 'break-word',
          }}>{JSON.stringify(body, null, 2)}</pre>
        ) : (
          <JtNode v={body} path="$" depth={0} ctx={ctx} />
        )}
      </div>
    </div>
  );
}

/* ---------- other tabs ---------- */
function AtHeadersTab({ headers }) {
  return (
    <div style={{ overflow: 'auto', flex: 1, padding: 'var(--pad)' }}>
      <div className="at-kv" style={{ border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', overflow: 'hidden', background: 'var(--bg1)' }}>
        {headers.map(([k, v]) => (
          <React.Fragment key={k}>
            <div className="k">{k}</div>
            <div>{v === null ? <AtRedacted /> : v}</div>
          </React.Fragment>
        ))}
      </div>
    </div>
  );
}

function AtAssertionsTab({ assertions }) {
  return (
    <div style={{ overflow: 'auto', flex: 1, padding: 'var(--pad)', display: 'flex', flexDirection: 'column', gap: 4 }}>
      {assertions.map((a, i) => (
        <div key={i} style={{
          border: '1px solid ' + (a.pass ? 'var(--bd0)' : 'color-mix(in srgb, var(--err) 30%, transparent)'),
          background: a.pass ? 'var(--bg1)' : 'color-mix(in srgb, var(--err) 7%, transparent)',
          borderRadius: 'var(--rad)', padding: '7px 10px',
        }}>
          <div style={{ display: 'flex', alignItems: 'center', gap: 9 }}>
            <span style={{ color: a.pass ? 'var(--ok)' : 'var(--err)', display: 'flex' }}>
              {a.pass ? AtIcons.check : AtIcons.x}
            </span>
            <span className="at-mono" style={{ fontSize: 'var(--fs-sm)', flex: 1 }}>{a.expr}</span>
            <span style={{
              fontSize: 10, fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.06em',
              color: a.pass ? 'var(--ok)' : 'var(--err)',
            }}>{a.pass ? 'passed' : 'failed'}</span>
          </div>
          {!a.pass && (
            <div className="at-mono" style={{ marginTop: 6, marginLeft: 21, fontSize: 'var(--fs-xs)', lineHeight: 1.7 }}>
              <div><span style={{ color: 'var(--fg2)', display: 'inline-block', width: 70 }}>expected</span><span style={{ color: 'var(--fg0)' }}>{a.expected}</span></div>
              <div><span style={{ color: 'var(--fg2)', display: 'inline-block', width: 70 }}>actual</span><span style={{ color: 'var(--err)' }}>{a.actual}</span></div>
            </div>
          )}
        </div>
      ))}
    </div>
  );
}

function AtTimingTab({ timing }) {
  const total = timing.reduce((s, [, ms]) => s + ms, 0);
  let acc = 0;
  return (
    <div style={{ overflow: 'auto', flex: 1, padding: 'var(--pad)' }}>
      <div style={{ display: 'flex', flexDirection: 'column', gap: 2, maxWidth: 760 }}>
        {timing.map(([label, ms], i) => {
          const left = (acc / total) * 100; acc += ms;
          const w = Math.max(0.6, (ms / total) * 100);
          const strong = label.indexOf('TTFB') >= 0;
          return (
            <div key={label} style={{ display: 'grid', gridTemplateColumns: '150px 1fr 64px', gap: 10, alignItems: 'center', height: 24 }}>
              <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg2)', textAlign: 'right' }}>{label}</span>
              <div style={{ position: 'relative', height: 10, background: 'var(--bg2)', borderRadius: 2 }}>
                <div style={{
                  position: 'absolute', left: left + '%', width: w + '%', top: 0, bottom: 0,
                  background: strong ? 'var(--fg1)' : 'var(--bd2)', borderRadius: 2,
                }}></div>
              </div>
              <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg1)', textAlign: 'right' }}>{ms} ms</span>
            </div>
          );
        })}
        <div style={{ display: 'grid', gridTemplateColumns: '150px 1fr 64px', gap: 10, marginTop: 6, paddingTop: 8, borderTop: '1px solid var(--bd0)' }}>
          <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg0)', textAlign: 'right', fontWeight: 600 }}>total</span>
          <span></span>
          <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg0)', textAlign: 'right', fontWeight: 600 }}>{total} ms</span>
        </div>
      </div>
    </div>
  );
}

function AtRequestTab({ ins, env }) {
  const base = AT.ENVS.find((e) => e.id === env).base;
  const h3 = { margin: '0 0 6px', fontSize: 'var(--fs-xs)', fontWeight: 600, textTransform: 'uppercase', letterSpacing: '0.07em', color: 'var(--fg2)' };
  return (
    <div style={{ overflow: 'auto', flex: 1, padding: 'var(--pad)', display: 'flex', flexDirection: 'column', gap: 'calc(var(--pad) * 1.4)' }}>
      <div className="at-mono" style={{ fontSize: 'var(--fs-sm)', display: 'flex', gap: 10, alignItems: 'center' }}>
        <span className="at-method" style={{ fontSize: 11, color: 'var(--fg0)' }}>{ins.req.method}</span>
        <span style={{ color: 'var(--fg1)', wordBreak: 'break-all' }}>{base}{ins.req.path.replace('{{charge.id}}', 'ch_3NqR8eK2xT0a1Vb9').replace('{{customer.id}}', 'cus_QfT81LbNa2').replace('{{refund.id}}', 're_9HtQ3yMrXw2c').replace('{{dispute.id}}', 'dp_5KwE9rT2nM')}</span>
      </div>

      <div>
        <h3 style={h3}>Request headers</h3>
        <div className="at-kv" style={{ border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', overflow: 'hidden', background: 'var(--bg1)', maxWidth: 760 }}>
          {ins.reqHeaders.map(([k, v]) => (
            <React.Fragment key={k}>
              <div className="k">{k}</div>
              <div>{v === null ? <AtRedacted /> : v}</div>
            </React.Fragment>
          ))}
        </div>
      </div>

      {ins.reqBody && (
        <div>
          <h3 style={h3}>Request body <span style={{ fontWeight: 400, textTransform: 'none', letterSpacing: 0 }}>(variables resolved)</span></h3>
          <pre className="at-mono" style={{
            margin: 0, padding: 10, fontSize: 'var(--fs-sm)', lineHeight: 1.5, color: 'var(--fg1)',
            background: 'var(--bg1)', border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', maxWidth: 760, overflow: 'auto',
          }}>{JSON.stringify(ins.reqBody, null, 2)}</pre>
        </div>
      )}

      <div>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10, marginBottom: 6 }}>
          <h3 style={{ ...h3, margin: 0 }}>Source</h3>
          <span className="at-mono at-xs at-dim">{ins.req.file}:{ins.req.line}</span>
          <AtOpenInEditor file={ins.req.file} line={ins.req.line} compact />
        </div>
        {ins.yaml ? (
          <div className="at-mono" style={{
            fontSize: 'var(--fs-sm)', lineHeight: 1.55, background: 'var(--bg1)',
            border: '1px solid var(--bd0)', borderRadius: 'var(--rad)', padding: '8px 0', maxWidth: 760, overflow: 'auto',
          }}>
            {ins.yaml.map(([n, line]) => (
              <div key={n} style={{ display: 'flex' }}>
                <span style={{ width: 44, flex: 'none', textAlign: 'right', paddingRight: 14, color: 'var(--fg3)', userSelect: 'none' }}>{n}</span>
                <span style={{ whiteSpace: 'pre', color: 'var(--fg1)' }}>{line}</span>
              </div>
            ))}
          </div>
        ) : (
          <div className="at-mono at-xs at-dim" style={{ padding: '6px 0' }}>
            definition in {ins.req.file}:{ins.req.line} — open in your editor to view
          </div>
        )}
      </div>
    </div>
  );
}

/* ---------- inspector shell ---------- */
function AtInspector({ id, run, env, onBack, initialTab }) {
  const ins = AT.inspectFor(id, run.resolved);
  const [tab, setTab] = React.useState(initialTab || 'body');
  const failCount = ins.assertions.filter((a) => !a.pass).length;
  const ph = run.phases[id];

  return (
    <div data-screen-label="Response inspector" style={{ display: 'flex', flexDirection: 'column', flex: 1, minHeight: 0 }}>
      <style>{atInspectorCss}</style>
      <div style={{ display: 'flex', alignItems: 'center', gap: 10, padding: 'var(--pad-sm) var(--pad)', flex: 'none', borderBottom: '1px solid var(--bd0)', background: 'var(--bg1)' }}>
        <button className="at-btn ghost sm" onClick={onBack} style={{ padding: '0 5px' }}>{AtIcons.back}</button>
        <AtDot state={ph} />
        <span style={{ fontWeight: 600, fontSize: 'var(--fs-md)', whiteSpace: 'nowrap' }}>{ins.req.name}</span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', overflow: 'hidden', textOverflow: 'ellipsis', whiteSpace: 'nowrap' }}>
          {ins.req.method} {ins.req.path}
        </span>
        <span style={{ flex: 1 }}></span>
        <AtCode code={ins.out.code} />
        <span style={{ color: 'var(--fg3)' }}>·</span>
        <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg1)', whiteSpace: 'nowrap' }}>{fmtMs(ins.out.ms)}</span>
        <AtOpenInEditor file={ins.req.file} line={ins.req.line} compact />
      </div>
      <AtTabs active={tab} onChange={setTab} tabs={[
        { id: 'body', label: 'Body' },
        { id: 'headers', label: 'Headers', count: ins.headers.length },
        { id: 'assertions', label: 'Assertions', count: failCount > 0 ? failCount + '✗' : ins.assertions.length },
        { id: 'timing', label: 'Timing' },
        { id: 'request', label: 'Request' },
      ]} />
      {tab === 'body' && <AtJsonBody body={ins.body} />}
      {tab === 'headers' && <AtHeadersTab headers={ins.headers} />}
      {tab === 'assertions' && <AtAssertionsTab assertions={ins.assertions} />}
      {tab === 'timing' && <AtTimingTab timing={ins.timing} />}
      {tab === 'request' && <AtRequestTab ins={ins} env={env} />}
    </div>
  );
}

Object.assign(window, { AtInspector, AtJsonBody, AtHeadersTab, AtAssertionsTab, AtTimingTab, AtRequestTab });
