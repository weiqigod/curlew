/* curlew UI — shared atoms: icons, dots, chips, tabs, open-in-editor */
const AtIcon = ({ d, size = 14, sw = 1.5, style }) => (
  <svg width={size} height={size} viewBox="0 0 16 16" fill="none" stroke="currentColor"
    strokeWidth={sw} strokeLinecap="round" strokeLinejoin="round" style={{ flex: 'none', ...style }}>
    {d}
  </svg>
);

const AtIcons = {
  chevron: (open) => <AtIcon d={<polyline points={open ? '4,6 8,10 12,6' : '6,4 10,8 6,12'} />} size={12} />,
  caret: <AtIcon d={<polyline points="4,6 8,10 12,6" />} size={12} />,
  search: <AtIcon d={<g><circle cx="7" cy="7" r="4.5" /><line x1="10.5" y1="10.5" x2="14" y2="14" /></g>} size={13} />,
  play: <AtIcon d={<path d="M5 3.5 L12.5 8 L5 12.5 Z" fill="currentColor" stroke="none" />} size={12} />,
  branch: <AtIcon d={<g><circle cx="4.5" cy="4" r="2" /><circle cx="4.5" cy="12" r="2" /><circle cx="11.5" cy="6" r="2" /><path d="M4.5 6 v4 M11.5 8 c0 3 -4 2 -5 3" /></g>} size={12} />,
  clock: <AtIcon d={<g><circle cx="8" cy="8" r="5.5" /><polyline points="8,5 8,8 10.5,9.5" /></g>} size={13} />,
  external: <AtIcon d={<g><path d="M6.5 4 H4 a1 1 0 0 0 -1 1 v7 a1 1 0 0 0 1 1 h7 a1 1 0 0 0 1 -1 V9.5" /><path d="M9 3 h4 v4 M13 3 L7.5 8.5" /></g>} size={12} />,
  copy: <AtIcon d={<g><rect x="5.5" y="5.5" width="7" height="7" rx="1" /><path d="M10.5 5.5 V4 a1 1 0 0 0 -1 -1 H4 a1 1 0 0 0 -1 1 v5.5 a1 1 0 0 0 1 1 h1.5" /></g>} size={12} />,
  check: <AtIcon d={<polyline points="3,8.5 6.5,12 13,4.5" />} size={12} sw={1.8} />,
  x: <AtIcon d={<g><line x1="4" y1="4" x2="12" y2="12" /><line x1="12" y1="4" x2="4" y2="12" /></g>} size={12} sw={1.8} />,
  warn: <AtIcon d={<g><path d="M8 2.5 L14.5 13.5 H1.5 Z" /><line x1="8" y1="7" x2="8" y2="10" /><circle cx="8" cy="12" r="0.4" fill="currentColor" /></g>} size={13} />,
  back: <AtIcon d={<g><line x1="13" y1="8" x2="3.5" y2="8" /><polyline points="7.5,4 3.5,8 7.5,12" /></g>} size={13} />,
  file: <AtIcon d={<g><path d="M4 2.5 h5.5 L13 6 v7.5 a1 1 0 0 1 -1 1 H4 a1 1 0 0 1 -1 -1 v-10 a1 1 0 0 1 1 -1 Z" /><polyline points="9.5,2.5 9.5,6 13,6" /></g>} size={13} />,
  skip: <AtIcon d={<line x1="4" y1="8" x2="12" y2="8" />} size={12} />,
};

function AtDot({ state, title }) {
  if (state === 'running') return <span className="at-spin" title={title || 'running'}></span>;
  return <span className={'at-dot ' + (state || 'pending')} title={title || state}></span>;
}

const AtMethod = ({ m }) => <span className="at-method">{m}</span>;

const AtRedacted = () => (
  <span className="at-redacted at-mono">
    <span className="mask">•••••••</span>
    <span className="tag">redacted</span>
  </span>
);

const fmtMs = (ms) => (ms >= 1000 ? (ms / 1000).toFixed(1) + ' s' : ms + ' ms');

function AtTabs({ tabs, active, onChange }) {
  return (
    <div className="at-tabs">
      {tabs.map((t) => (
        <button key={t.id} className={'at-tab' + (active === t.id ? ' active' : '')} onClick={() => onChange(t.id)}>
          {t.label}
          {t.count != null && <span className="n">{t.count}</span>}
        </button>
      ))}
    </div>
  );
}

/* "Open in editor" — the single editing affordance. Shows transient feedback. */
function AtOpenInEditor({ file, line, compact }) {
  const [hit, setHit] = React.useState(false);
  const fire = () => { setHit(true); setTimeout(() => setHit(false), 1400); };
  return (
    <button className="at-btn sm ghost" onClick={fire} title={file + (line ? ':' + line : '')}
      style={{ fontFamily: 'var(--font-mono)', fontSize: 'var(--fs-xs)' }}>
      {AtIcons.external}
      {hit ? <span>→ $EDITOR</span> : <span>{compact ? 'Open in editor' : 'Open in editor'}</span>}
      {!compact && line != null && <span style={{ color: 'var(--fg3)' }}>:{line}</span>}
    </button>
  );
}

/* status-code text — neutral; pass/fail color lives on the dot */
const AtCode = ({ code }) => (
  <span className="at-mono" style={{ fontSize: 'var(--fs-xs)', color: 'var(--fg1)', whiteSpace: 'nowrap' }}>{code}</span>
);

function AtCopyBtn({ text, label }) {
  const [ok, setOk] = React.useState(false);
  return (
    <button className="at-btn sm ghost" style={{ height: 18, padding: '0 4px' }}
      title={'copy ' + (label || '')}
      onClick={(e) => {
        e.stopPropagation();
        try { navigator.clipboard && navigator.clipboard.writeText(text); } catch (err) { /* noop */ }
        setOk(true); setTimeout(() => setOk(false), 1100);
      }}>
      {ok ? AtIcons.check : AtIcons.copy}
      {ok && <span style={{ fontSize: 10 }}>copied</span>}
    </button>
  );
}

Object.assign(window, { AtIcon, AtIcons, AtDot, AtMethod, AtRedacted, AtTabs, AtOpenInEditor, AtCode, AtCopyBtn, fmtMs });
