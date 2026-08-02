/* apitest UI — app shell: routing, run simulation, theme/density/accent wiring */

const AT_ACCENTS = { '#e9e9e9': 'mono', '#6494e4': 'blue', '#3aa8b8': 'teal', '#9d85e8': 'violet' };

function AtApp({ tweaks, interactive = true, initial }) {
  const T = Object.assign({ theme: 'dark', accent: '#e9e9e9', density: 'dense', layout: 'columns', speed: 1, failRate: 15, appState: 'normal' }, tweaks || {});
  const init = initial || {};
  const failRate = (T.failRate == null ? 15 : T.failRate) / 100;

  const [view, setView] = React.useState(init.view || { screen: 'run' });
  const [env, setEnv] = React.useState(init.env || 'dev');
  const [run, setRun] = React.useState(() => (init.run === 'midflight' ? AT.midflightRun() : AT.completeRun(failRate)));
  const [elapsed, setElapsed] = React.useState(init.run === 'midflight' ? 1240 : 0);
  const [watchEvent, setWatchEvent] = React.useState(null);
  const timersRef = React.useRef([]);
  const watchFiles = ['collections/charges.yaml', 'collections/refunds.yaml', 'collections/disputes.yaml'];

  const clearTimers = () => { timersRef.current.forEach(clearTimeout); timersRef.current = []; };
  React.useEffect(() => clearTimers, []);

  /* live-reload pulse, every ~9s */
  React.useEffect(() => {
    if (!interactive) return;
    let i = 0, t2;
    const t = setInterval(() => {
      setWatchEvent(watchFiles[i++ % watchFiles.length].replace('collections/', ''));
      t2 = setTimeout(() => setWatchEvent(null), 2600);
    }, 9000);
    return () => { clearInterval(t); clearTimeout(t2); };
  }, [interactive]);

  /* failure-rate tweak re-resolves a finished run */
  React.useEffect(() => {
    if (!interactive) return;
    setRun((r) => (r.running ? r : AT.completeRun(failRate)));
  }, [failRate, interactive]);

  /* elapsed ticker while streaming */
  React.useEffect(() => {
    if (!run.running || !run.startT) return;
    const t = setInterval(() => setElapsed(Math.round((Date.now() - run.startT) * (T.speed || 1))), 100);
    return () => clearInterval(t);
  }, [run.running, run.startT, T.speed]);

  const startRun = (mode) => {
    if (!interactive || run.running) return;
    clearTimers();
    const speed = T.speed || 1;
    const resolved = AT.resolve(failRate);
    const phases = {};
    AT.WAVES.flat().forEach((id) => (phases[id] = 'pending'));
    let wallActual = 0;
    AT.WAVES.forEach((w) => (wallActual += Math.max(...w.map((id) => (resolved[id].result === 'skip' ? 0 : resolved[id].ms)))));
    setElapsed(0);
    setRun({ phases: { ...phases }, resolved, running: true, animate: true, wallMs: wallActual, startT: Date.now() });
    setView((v) => {
      const f = mode === 'all' ? null : v.file;
      const ff = f && AT.FILES.find((x) => x.path === f);
      return { screen: 'run', file: ff && ff.requests.length > 0 ? f : null };
    });

    const upd = (id, ph) => setRun((s) => ({ ...s, phases: { ...s.phases, [id]: ph } }));
    let cum = 250 / speed;
    AT.WAVES.forEach((w) => {
      let waveMax = 0;
      w.forEach((id) => {
        const r = resolved[id];
        if (r.result === 'skip') {
          timersRef.current.push(setTimeout(() => upd(id, 'skip'), cum + 60));
          return;
        }
        timersRef.current.push(setTimeout(() => upd(id, 'running'), cum));
        const dur = r.ms / speed;
        waveMax = Math.max(waveMax, dur);
        timersRef.current.push(setTimeout(() => upd(id, r.result), cum + dur));
      });
      cum += waveMax + 130 / speed;
    });
    timersRef.current.push(setTimeout(() => setRun((s) => ({ ...s, running: false })), cum + 40));
  };

  const onSelect = (sel) => {
    if (sel.type === 'request') setView({ screen: 'inspector', req: sel.id, file: view.file });
    else {
      const f = AT.FILES.find((x) => x.path === sel.id);
      if (f && f.invalid) setView({ screen: 'error', file: sel.id });
      else setView({ screen: 'run', file: sel.id });
    }
  };

  const selection = view.screen === 'inspector' ? { type: 'request', id: view.req }
    : view.screen === 'error' ? { type: 'file', id: view.file }
    : view.file ? { type: 'file', id: view.file } : null;

  const empty = T.appState === 'empty';
  let content;
  if (empty) content = <AtEmptyState />;
  else if (view.screen === 'error') content = <AtValidationError file={view.file} onBack={() => setView({ screen: 'run' })} />;
  else if (view.screen === 'inspector') content = (
    <AtInspector key={view.req} id={view.req} run={run} env={env} initialTab={view.tab}
      onBack={() => setView({ screen: 'run', file: view.file })} />
  );
  else if (view.screen === 'compare') content = <AtCompare />;
  else content = (
    <AtRunView run={run} elapsedMs={elapsed} layout={T.layout} fileFilter={view.file}
      onOpen={(id) => setView({ screen: 'inspector', req: id, file: view.file })}
      onClearFilter={() => setView({ screen: 'run' })} />
  );

  return (
    <div className="at-root" data-theme={T.theme} data-density={T.density}
      data-accent={AT_ACCENTS[T.accent] || 'mono'}
      style={{ width: '100%', height: '100%', display: 'flex', flexDirection: 'column', overflow: 'hidden' }}>
      <AtTopBar env={env} setEnv={setEnv} running={run.running} watchEvent={empty ? null : watchEvent}
        interactive={interactive}
        view={view.screen} setView={setView}
        onRun={(mode) => startRun(mode)} />
      <div style={{ display: 'flex', flex: 1, minHeight: 0 }}>
        {!empty && <AtSidebar run={run} selection={selection} onSelect={onSelect} interactive={interactive} />}
        <main style={{ flex: 1, minWidth: 0, display: 'flex', flexDirection: 'column', minHeight: 0, background: 'var(--bg0)' }}>
          {content}
        </main>
      </div>
    </div>
  );
}

Object.assign(window, { AtApp, AT_ACCENTS });
