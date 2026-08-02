// Global setup (UI_SPECIFICATION.md §13.3): owns the full stack lifecycle.
//
//  1. Rebuilds the SPA when internal/uiserver/assets/dist is stale relative
//     to ui/src (the Go binary embeds those assets, so this must come first).
//  2. Builds the real apitest binary to a temp dir.
//  3. Starts the in-process echo API + reserves a guaranteed-refused port.
//  4. Writes the fixture project and launches one
//     `apitest ui --port 0 --no-open --env dev`, parsing the printed
//     URL + token from stdout.
//  5. Persists everything the tests/teardown need to a state file in tmpdir.
import { execSync } from 'node:child_process';
import { spawn, type ChildProcess } from 'node:child_process';
import * as fs from 'node:fs';
import * as os from 'node:os';
import * as path from 'node:path';
import { fileURLToPath } from 'node:url';
import { startEchoServer, refusedPort, type EchoServer } from './echo-server';
import { writeFixtureProject } from './fixture';
import { STATE_FILE, type E2EState, type UiServer } from './helpers';

const HERE = path.dirname(fileURLToPath(import.meta.url));
const UI_DIR = path.resolve(HERE, '..', '..');
const REPO_ROOT = path.resolve(UI_DIR, '..');
const DIST_INDEX = path.join(REPO_ROOT, 'internal', 'uiserver', 'assets', 'dist', 'index.html');

interface GlobalStash {
  echo: EchoServer;
  children: ChildProcess[];
}

function newestMtime(dir: string): number {
  let newest = 0;
  for (const entry of fs.readdirSync(dir, { withFileTypes: true })) {
    const p = path.join(dir, entry.name);
    if (entry.isDirectory()) {
      newest = Math.max(newest, newestMtime(p));
    } else {
      newest = Math.max(newest, fs.statSync(p).mtimeMs);
    }
  }
  return newest;
}

function ensureSpaBuilt(): void {
  const srcNewest = Math.max(
    newestMtime(path.join(UI_DIR, 'src')),
    fs.statSync(path.join(UI_DIR, 'index.html')).mtimeMs,
  );
  const distFresh = fs.existsSync(DIST_INDEX) && fs.statSync(DIST_INDEX).mtimeMs >= srcNewest;
  if (!distFresh) {
    console.log('[e2e setup] dist assets stale — running npm run build');
    execSync('npm run build', { cwd: UI_DIR, stdio: 'inherit' });
  }
}

function buildBinary(binDir: string): string {
  const bin = path.join(binDir, 'apitest');
  console.log('[e2e setup] building apitest binary');
  execSync(`go build -o ${JSON.stringify(bin)} ./cmd/apitest`, { cwd: REPO_ROOT, stdio: 'inherit' });
  return bin;
}

function launchUiServer(bin: string, dir: string): Promise<{ child: ChildProcess; server: UiServer }> {
  return new Promise((resolve, reject) => {
    const child = spawn(bin, ['ui', '--port', '0', '--no-open', '--env', 'dev'], {
      cwd: dir,
      env: { ...process.env },
      stdio: ['ignore', 'pipe', 'pipe'],
    });
    let out = '';
    let err = '';
    const timer = setTimeout(() => {
      child.kill('SIGKILL');
      reject(new Error(`apitest ui did not print its URL in time.\nstdout: ${out}\nstderr: ${err}`));
    }, 20_000);
    child.stderr?.on('data', (d: Buffer) => {
      err += d.toString();
    });
    child.stdout?.on('data', (d: Buffer) => {
      out += d.toString();
      const m = /listening on (http:\/\/127\.0\.0\.1:\d+)\/\?token=([0-9a-f]+)/.exec(out);
      if (m !== null) {
        clearTimeout(timer);
        const pid = child.pid;
        if (pid === undefined) {
          reject(new Error('apitest ui has no pid'));
          return;
        }
        resolve({ child, server: { origin: m[1], token: m[2], pid, dir } });
      }
    });
    child.on('exit', (code) => {
      clearTimeout(timer);
      reject(new Error(`apitest ui exited early (code ${code}).\nstdout: ${out}\nstderr: ${err}`));
    });
  });
}

async function waitReady(server: UiServer): Promise<void> {
  const deadline = Date.now() + 10_000;
  for (;;) {
    try {
      const res = await fetch(`${server.origin}/api/v1/meta`, {
        headers: { Authorization: `Bearer ${server.token}` },
      });
      if (res.ok) return;
    } catch {
      // not up yet
    }
    if (Date.now() > deadline) throw new Error(`server at ${server.origin} never became ready`);
    await new Promise((r) => setTimeout(r, 100));
  }
}

export default async function globalSetup(): Promise<void> {
  ensureSpaBuilt();

  const binDir = fs.mkdtempSync(path.join(os.tmpdir(), 'apitest-e2e-bin-'));
  const bin = buildBinary(binDir);

  const echo = await startEchoServer();
  const refused = await refusedPort();
  const fixtureOpts = {
    echoUrl: `http://127.0.0.1:${echo.port}`,
    refusedUrl: `http://127.0.0.1:${refused}`,
  };

  const children: ChildProcess[] = [];
  let server: UiServer;
  try {
    const dir = fs.mkdtempSync(path.join(os.tmpdir(), 'apitest-e2e-'));
    writeFixtureProject(dir, fixtureOpts);
    const launched = await launchUiServer(bin, dir);
    children.push(launched.child);
    server = launched.server;
    await waitReady(server);
    console.log(`[e2e setup] server ready at ${server.origin}`);
  } catch (e) {
    for (const c of children) c.kill('SIGKILL');
    await echo.close();
    throw e;
  }

  const state: E2EState = {
    binDir,
    echoPort: echo.port,
    refusedPort: refused,
    server,
  };
  fs.writeFileSync(STATE_FILE, JSON.stringify(state, null, 2));

  // The teardown runs in this same process: stash live handles globally.
  (globalThis as unknown as { __apitestE2E?: GlobalStash }).__apitestE2E = { echo, children };
}
