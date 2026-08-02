// WS client (UI_SPECIFICATION.md §10.3.5, §5).
// - connect ws(s)://host/api/v1/ws?token=… (token via query param — browsers
//   cannot set headers on WS upgrades)
// - tracks lastEventId (highest run.event id seen) for gapless re-subscription
// - exponential backoff 0.5 s → 8 s cap, jittered; after 3 failed attempts the
//   status flips to 'reconnecting' (banner); after 30 s of failure a /meta
//   probe decides 'dead' (Disconnected screen)
// - on reconnect: re-subscribe {run_id, from_id: lastEventId} — the server
//   replays gaplessly (§5.2)
//
// Frame *interpretation* (hello etag comparison, tree refresh, reducer
// dispatch) belongs to the store layer via onFrame — this class is transport.

import { writable, type Writable } from 'svelte/store';
import { apiFetch, getToken } from './api/client';
import type { ClientFrame, WsFrame } from './types/events';

export type WsStatus = 'connecting' | 'open' | 'reconnecting' | 'dead';

const BACKOFF_BASE_MS = 500;
const BACKOFF_CAP_MS = 8000;
const RECONNECT_BANNER_AFTER_ATTEMPTS = 3;
const DEAD_PROBE_AFTER_MS = 30_000;

function defaultUrl(): string {
  const proto = location.protocol === 'https:' ? 'wss' : 'ws';
  const token = getToken() ?? '';
  return `${proto}://${location.host}/api/v1/ws?token=${encodeURIComponent(token)}`;
}

async function defaultProbe(): Promise<boolean> {
  try {
    await apiFetch('/meta');
    return true;
  } catch {
    return false;
  }
}

export interface WsClientOptions {
  onFrame?: (frame: WsFrame) => void;
  /** URL factory — overridable for tests/dev. */
  makeUrl?: () => string;
  /** Reachability probe used after 30 s of failed reconnects. */
  probe?: () => Promise<boolean>;
  /** External status store (e.g. stores/connection.wsStatus). */
  status?: Writable<WsStatus>;
}

export class WsClient {
  readonly status: Writable<WsStatus>;

  private ws: WebSocket | null = null;
  private opts: WsClientOptions;
  private closed = false;
  private attempts = 0;
  private failingSince: number | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private subscribedRunId: string | null = null;
  private lastEventId = 0;

  constructor(opts: WsClientOptions = {}) {
    this.opts = opts;
    this.status = opts.status ?? writable<WsStatus>('connecting');
  }

  /** Highest run.event id seen for the subscribed run (the replay cursor). */
  getLastEventId(): number {
    return this.lastEventId;
  }

  isSubscribedTo(runId: string): boolean {
    return this.subscribedRunId === runId;
  }

  connect(): void {
    this.closed = false;
    this.open();
  }

  /** Subscribe to a run's event stream from a given event id (0 = everything). */
  subscribe(runId: string, fromId = 0): void {
    if (this.subscribedRunId !== runId) {
      this.lastEventId = fromId;
    }
    this.subscribedRunId = runId;
    this.send({ type: 'subscribe', run_id: runId, from_id: fromId });
  }

  ping(): void {
    this.send({ type: 'ping' });
  }

  close(): void {
    this.closed = true;
    if (this.reconnectTimer !== null) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.ws?.close();
    this.ws = null;
  }

  private send(frame: ClientFrame): void {
    if (this.ws !== null && this.ws.readyState === WebSocket.OPEN) {
      this.ws.send(JSON.stringify(frame));
    }
  }

  private open(): void {
    const makeUrl = this.opts.makeUrl ?? defaultUrl;
    let ws: WebSocket;
    try {
      ws = new WebSocket(makeUrl());
    } catch {
      this.scheduleReconnect();
      return;
    }
    this.ws = ws;

    ws.onopen = () => {
      this.attempts = 0;
      this.failingSince = null;
      this.status.set('open');
      // Re-subscribe after a reconnect — the server replays from lastEventId
      // gaplessly (§5.2). On a fresh connect the consumer subscribes after hello.
      if (this.subscribedRunId !== null) {
        this.send({ type: 'subscribe', run_id: this.subscribedRunId, from_id: this.lastEventId });
      }
    };

    ws.onmessage = (msg: MessageEvent) => {
      let frame: WsFrame;
      try {
        frame = JSON.parse(String(msg.data)) as WsFrame;
      } catch {
        return; // malformed frame — ignore
      }
      if (
        frame.type === 'run.event' &&
        frame.run_id === this.subscribedRunId &&
        frame.data.id > this.lastEventId
      ) {
        this.lastEventId = frame.data.id;
      }
      this.opts.onFrame?.(frame);
    };

    ws.onclose = () => {
      if (this.ws === ws) this.ws = null;
      this.scheduleReconnect();
    };
    ws.onerror = () => {
      // onclose follows; nothing to do here.
    };
  }

  private scheduleReconnect(): void {
    if (this.closed || this.reconnectTimer !== null) return;
    this.attempts++;
    if (this.failingSince === null) this.failingSince = Date.now();
    if (this.attempts > RECONNECT_BANNER_AFTER_ATTEMPTS) {
      this.status.set('reconnecting');
    }
    if (Date.now() - this.failingSince > DEAD_PROBE_AFTER_MS) {
      void (this.opts.probe ?? defaultProbe)().then((reachable) => {
        if (!reachable) this.status.set('dead');
      });
    }
    // 0.5 → 1 → 2 → 4 → 8 s cap, jittered ±25%.
    const base = Math.min(BACKOFF_CAP_MS, BACKOFF_BASE_MS * 2 ** (this.attempts - 1));
    const delay = base * (0.75 + Math.random() * 0.5);
    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      if (!this.closed) this.open();
    }, delay);
  }
}
