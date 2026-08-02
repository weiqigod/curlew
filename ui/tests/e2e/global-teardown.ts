// Global teardown: kill the curlew ui process, stop the echo server, and
// remove the temp fixture/binary directories + state file.
import * as fs from 'node:fs';
import type { ChildProcess } from 'node:child_process';
import type { EchoServer } from './echo-server';
import { STATE_FILE, readState } from './helpers';

interface GlobalStash {
  echo: EchoServer;
  children: ChildProcess[];
}

function killPid(pid: number): void {
  try {
    process.kill(pid, 'SIGTERM');
  } catch {
    // already gone
  }
}

export default async function globalTeardown(): Promise<void> {
  const stash = (globalThis as unknown as { __curlewE2E?: GlobalStash }).__curlewE2E;

  let state;
  try {
    state = readState();
  } catch {
    state = null;
  }

  if (state !== null) {
    killPid(state.server.pid);
  }
  // Give the server a moment for its graceful shutdown (history flush).
  await new Promise((r) => setTimeout(r, 500));
  if (stash !== undefined) {
    for (const c of stash.children) {
      if (c.exitCode === null) c.kill('SIGKILL');
    }
    await stash.echo.close();
  }

  if (state !== null) {
    fs.rmSync(state.server.dir, { recursive: true, force: true });
    fs.rmSync(state.binDir, { recursive: true, force: true });
  }
  fs.rmSync(STATE_FILE, { force: true });
}
