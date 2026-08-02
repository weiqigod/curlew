// Tiny deterministic echo API for the e2e suite (UI_SPECIFICATION.md §13.3).
// Runs in-process inside the Playwright global setup (the setup/teardown both
// execute in the runner process, so the server stays alive for the workers).
//
// Endpoints:
//   GET /json  → small JSON with an incrementing `counter` (so two runs of the
//                same collection produce different bodies → compare diff shows
//                changed lines) plus a `stable` field for pass/fail assertions
//   GET /slow  → ~1.8 s delay, then a STATIC JSON body (identical across runs
//                → the compare identical-state panel; also the mid-run-reload
//                window)
//   GET /500   → always 500 (retry fixture target)
//   GET /big   → static JSON > 256 KiB (the server's InlineBodyLimit) → the
//                Body tab load-on-demand panel
//   GET /bin   → PNG bytes, image/png → the binary panel
//   anything else → 404 JSON
import * as http from 'node:http';
import type { AddressInfo } from 'node:net';

const SLOW_MS = 1800;

// 1×1 transparent PNG (contains NUL bytes → the UI's binary heuristic).
const PNG_1PX = Buffer.from(
  'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg==',
  'base64',
);

// Static payload comfortably above the 256 KiB inline cap (~300 KB).
const BIG_BODY = JSON.stringify({
  data: Array.from({ length: 4000 }, (_, i) => ({
    id: i,
    name: `item-${i}`,
    value: Math.floor(i * 7.3),
    tags: ['alpha', 'beta', 'gamma'],
  })),
});

export interface EchoServer {
  port: number;
  close(): Promise<void>;
}

export async function startEchoServer(): Promise<EchoServer> {
  let counter = 0;

  const server = http.createServer((req, res) => {
    const url = new URL(req.url ?? '/', 'http://localhost');
    switch (url.pathname) {
      case '/json': {
        counter += 1;
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ counter, stable: 'yes', items: [1, 2, 3] }));
        return;
      }
      case '/slow': {
        setTimeout(() => {
          res.writeHead(200, { 'Content-Type': 'application/json' });
          res.end(JSON.stringify({ ok: true, kind: 'slow' }));
        }, SLOW_MS);
        return;
      }
      case '/500': {
        res.writeHead(500, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'boom' }));
        return;
      }
      case '/big': {
        res.writeHead(200, { 'Content-Type': 'application/json' });
        res.end(BIG_BODY);
        return;
      }
      case '/bin': {
        res.writeHead(200, { 'Content-Type': 'image/png' });
        res.end(PNG_1PX);
        return;
      }
      default: {
        res.writeHead(404, { 'Content-Type': 'application/json' });
        res.end(JSON.stringify({ error: 'not found' }));
      }
    }
  });

  await new Promise<void>((resolve) => server.listen(0, '127.0.0.1', resolve));
  const port = (server.address() as AddressInfo).port;
  return {
    port,
    close: () =>
      new Promise<void>((resolve) => {
        server.closeAllConnections();
        server.close(() => resolve());
      }),
  };
}

/**
 * Reserves an ephemeral port and releases it immediately: nothing listens on
 * it afterwards, so connections are refused → the `error` outcome fixture.
 */
export async function refusedPort(): Promise<number> {
  const srv = http.createServer();
  await new Promise<void>((resolve) => srv.listen(0, '127.0.0.1', resolve));
  const port = (srv.address() as AddressInfo).port;
  await new Promise<void>((resolve) => srv.close(() => resolve()));
  return port;
}
