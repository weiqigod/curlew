/* apitest UI — design canvas: wave-layout variations + key screens & states */

function AtBoard({ w = 1440, h = 900, tweaks, initial }) {
  return (
    <div style={{ width: w, height: h, overflow: 'hidden' }}>
      <AtApp interactive={false} tweaks={tweaks} initial={initial} />
    </div>
  );
}

function AtCanvasMain() {
  const dark = (extra) => Object.assign({ theme: 'dark', accent: '#e9e9e9', density: 'dense', layout: 'columns' }, extra);
  return (
    <DesignCanvas>
      <DCSection id="runview" title="Screen 1 · Live run view — wave layout variations"
        subtitle="Requests execute in dependency-ordered waves. A is the proposed default, shown mid-flight (waves 1–2 done, wave 3 running, 4–5 pending). B and C show the finished run: 11 passed · 2 failed · 1 skipped.">
        <DCArtboard id="run-a" label="A · Waves as columns — mid-flight" width={1440} height={900}>
          <AtBoard tweaks={dark({ layout: 'columns' })} initial={{ run: 'midflight' }} />
        </DCArtboard>
        <DCArtboard id="run-b" label="B · Swimlane rows per wave" width={1440} height={900}>
          <AtBoard tweaks={dark({ layout: 'lanes' })} initial={{}} />
        </DCArtboard>
        <DCArtboard id="run-c" label="C · Compact CI-log list" width={1440} height={900}>
          <AtBoard tweaks={dark({ layout: 'compact' })} initial={{}} />
        </DCArtboard>
        <DCPostIt id="note-run">
          Color is reserved for status: green pass, red fail, amber running, hollow gray pending, dash skipped.
          Failed cards quote the first failing assertion inline. The summary strip + thin segmented progress bar
          stream live; cards fade their result rows in as the event stream lands.
        </DCPostIt>
      </DCSection>

      <DCSection id="inspector" title="Screen 2 · Response inspector"
        subtitle="Click a request card to open it. Tabs: Body / Headers / Assertions / Timing / Request. Secrets are •••••• with a redacted chip — no reveal toggle, by design.">
        <DCArtboard id="ins-body" label="Body — collapsible JSON tree, search, copy-path, raw toggle (50-item payload truncated)" width={1440} height={900}>
          <AtBoard tweaks={dark()} initial={{ view: { screen: 'inspector', req: 'chg-list' } }} />
        </DCArtboard>
        <DCArtboard id="ins-assert" label="Assertions — failed request, expected vs actual" width={1440} height={760}>
          <AtBoard h={760} tweaks={dark()} initial={{ view: { screen: 'inspector', req: 'ref-partial', tab: 'assertions' } }} />
        </DCArtboard>
        <DCArtboard id="ins-timing" label="Timing — request waterfall" width={1440} height={620}>
          <AtBoard h={620} tweaks={dark()} initial={{ view: { screen: 'inspector', req: 'chg-create', tab: 'timing' } }} />
        </DCArtboard>
        <DCArtboard id="ins-request" label="Request — resolved headers (redacted), body, YAML source + open-in-editor" width={1440} height={900}>
          <AtBoard tweaks={dark()} initial={{ view: { screen: 'inspector', req: 'chg-create', tab: 'request' } }} />
        </DCArtboard>
      </DCSection>

      <DCSection id="compare" title="Screen 3 · Run comparison"
        subtitle="History rail picks a baseline (A) to diff against the current run (B): status, duration delta, unified JSON body diff with changed keys highlighted.">
        <DCArtboard id="cmp" label="10:32 on main  vs  10:47 on feature/fix-totals" width={1440} height={900}>
          <AtBoard tweaks={dark()} initial={{ view: { screen: 'compare' } }} />
        </DCArtboard>
      </DCSection>

      <DCSection id="states" title="States & themes"
        subtitle="Empty repo onboarding, YAML validation error (read-only diagnostic + open-in-editor), and the secondary light theme.">
        <DCArtboard id="st-empty" label="Empty project — terminal-flavored onboarding" width={1440} height={760}>
          <AtBoard h={760} tweaks={dark({ appState: 'empty' })} initial={{}} />
        </DCArtboard>
        <DCArtboard id="st-invalid" label="Validation error — webhooks.yaml, quoted CLI diagnostic" width={1440} height={760}>
          <AtBoard h={760} tweaks={dark()} initial={{ view: { screen: 'error', file: 'collections/webhooks.yaml' } }} />
        </DCArtboard>
        <DCArtboard id="st-light" label="Light theme (secondary)" width={1440} height={900}>
          <AtBoard tweaks={dark({ theme: 'light' })} initial={{ run: 'midflight' }} />
        </DCArtboard>
        <DCPostIt id="note-system">
          System: IBM Plex Sans for chrome, IBM Plex Mono for paths/YAML/JSON/timings. Pure-neutral grays
          (#151515 → #303030 surfaces). Monochrome accent by default — selection, focus and the Run button
          are near-white; an optional blue/teal/violet accent is a tweak in the prototype.
        </DCPostIt>
      </DCSection>
    </DesignCanvas>
  );
}

ReactDOM.createRoot(document.getElementById('root')).render(<AtCanvasMain />);
