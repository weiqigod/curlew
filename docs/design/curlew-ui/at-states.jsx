/* curlew UI — empty project onboarding + YAML validation error panel */

function AtEmptyState() {
  const line = (txt, color) => (
    <div style={{ color: color || 'var(--fg1)', whiteSpace: 'pre' }}>{txt}</div>
  );
  return (
    <div data-screen-label="Empty project" style={{ flex: 1, display: 'flex', alignItems: 'center', justifyContent: 'center', minHeight: 0 }}>
      <div style={{ width: 560, display: 'flex', flexDirection: 'column', gap: 14 }}>
        <div>
          <div style={{ fontSize: 15, fontWeight: 600 }}>No collections in this repo yet</div>
          <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--fg2)', marginTop: 4, lineHeight: 1.5 }}>
            curlew reads YAML files from <span className="at-mono">collections/</span> — your files in git are the only
            source of truth. Scaffold a starting point from your terminal:
          </div>
        </div>
        <div className="at-mono" style={{
          background: 'var(--bg1)', border: '1px solid var(--bd0)', borderRadius: 'var(--rad)',
          padding: '14px 16px', fontSize: 'var(--fs-sm)', lineHeight: 1.75,
        }}>
          {line(<span><span style={{ color: 'var(--fg3)' }}>$ </span><span style={{ color: 'var(--fg0)' }}>curlew init</span></span>)}
          {line('')}
          <div style={{ whiteSpace: 'pre' }}><span style={{ color: 'var(--ok)' }}>  created  </span><span style={{ color: 'var(--fg1)' }}>curlew.yaml</span></div>
          <div style={{ whiteSpace: 'pre' }}><span style={{ color: 'var(--ok)' }}>  created  </span><span style={{ color: 'var(--fg1)' }}>collections/example.yaml</span></div>
          <div style={{ whiteSpace: 'pre' }}><span style={{ color: 'var(--ok)' }}>  created  </span><span style={{ color: 'var(--fg1)' }}>environments/dev.yaml</span></div>
          {line('')}
          {line('  3 files written. Edit collections/example.yaml, then:', 'var(--fg2)')}
          {line('')}
          {line(<span><span style={{ color: 'var(--fg3)' }}>$ </span><span style={{ color: 'var(--fg0)' }}>curlew run</span><span className="at-cursor" style={{ display: 'inline-block', width: 7, height: 14, background: 'var(--fg2)', marginLeft: 6, verticalAlign: 'middle' }}></span></span>)}
        </div>
        <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)', display: 'flex', gap: 6, alignItems: 'center' }}>
          {AtIcons.file}
          this view refreshes automatically when files appear — curlew is watching the repo
        </div>
      </div>
    </div>
  );
}

function AtValidationError({ file, onBack }) {
  const f = AT.FILES.find((x) => x.path === file);
  const inv = f && f.invalid;
  if (!inv) return null;
  return (
    <div data-screen-label="Validation error" style={{ flex: 1, overflow: 'auto', padding: 'calc(var(--pad) * 2)', minHeight: 0 }}>
      <div style={{ maxWidth: 720, display: 'flex', flexDirection: 'column', gap: 14 }}>
        <div style={{ display: 'flex', alignItems: 'center', gap: 10 }}>
          <span style={{ color: 'var(--warn)', display: 'flex' }}>{AtIcons.warn}</span>
          <span style={{ fontSize: 15, fontWeight: 600 }}>Invalid collection file</span>
          <span className="at-mono at-xs at-dim">{f.path}</span>
          <span style={{ flex: 1 }}></span>
          <AtOpenInEditor file={f.path} line={inv.line} />
        </div>

        <div style={{ fontSize: 'var(--fs-sm)', color: 'var(--fg2)', lineHeight: 1.5 }}>
          This file was skipped — its requests won't run until the file parses. Fix it in your editor;
          the tree reloads the moment you save.
        </div>

        {/* CLI diagnostic, quoted verbatim */}
        <div className="at-mono" style={{
          background: 'color-mix(in srgb, var(--warn) 7%, transparent)',
          border: '1px solid color-mix(in srgb, var(--warn) 30%, transparent)',
          borderRadius: 'var(--rad)', padding: '10px 14px', fontSize: 'var(--fs-sm)', lineHeight: 1.6,
        }}>
          <span style={{ color: 'var(--warn)', fontWeight: 600 }}>error</span>
          <span style={{ color: 'var(--fg1)' }}> {inv.diag}</span>
        </div>

        <div className="at-mono" style={{
          background: 'var(--bg1)', border: '1px solid var(--bd0)', borderRadius: 'var(--rad)',
          padding: '8px 0', fontSize: 'var(--fs-sm)', lineHeight: 1.6, overflow: 'auto',
        }}>
          {inv.snippet.map(([n, lineTxt]) => (
            <div key={n} style={{
              display: 'flex',
              background: n === inv.line ? 'color-mix(in srgb, var(--warn) 9%, transparent)' : 'transparent',
            }}>
              <span style={{ width: 44, flex: 'none', textAlign: 'right', paddingRight: 14, color: n === inv.line ? 'var(--warn)' : 'var(--fg3)', userSelect: 'none' }}>{n}</span>
              <span style={{ whiteSpace: 'pre', color: n === inv.line ? 'var(--fg0)' : 'var(--fg1)' }}>{lineTxt}</span>
              {n === inv.line && (
                <span style={{ marginLeft: 16, color: 'var(--warn)', whiteSpace: 'nowrap' }}>← unknown operator 'eq'</span>
              )}
            </div>
          ))}
        </div>

        <div style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg3)' }}>
          curlew never edits your files — there is deliberately no fix-it form here.
        </div>

        {onBack && (
          <div>
            <button className="at-btn ghost sm" onClick={onBack}>{AtIcons.back} back to run view</button>
          </div>
        )}
      </div>
    </div>
  );
}

Object.assign(window, { AtEmptyState, AtValidationError });
