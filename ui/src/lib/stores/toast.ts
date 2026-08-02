// Transient notices — copy confirmations, 409 conflicts, open-in-editor
// results (UI_SPECIFICATION.md §10.3.1).

import { writable } from 'svelte/store';

export interface ToastAction {
  label: string;
  fn: () => void;
}

export interface Toast {
  id: number;
  message: string;
  kind: 'info' | 'error';
  action?: ToastAction;
}

export interface ToastOptions {
  kind?: 'info' | 'error';
  action?: ToastAction;
  /** Auto-dismiss delay; 0 disables auto-dismiss. */
  durationMs?: number;
}

const DEFAULT_DURATION_MS = 5000;

let nextId = 1;

export const toasts = writable<Toast[]>([]);

export function toast(message: string, opts: ToastOptions = {}): number {
  const t: Toast = { id: nextId++, message, kind: opts.kind ?? 'info' };
  if (opts.action !== undefined) t.action = opts.action;
  toasts.update((list) => [...list, t]);
  const duration = opts.durationMs ?? DEFAULT_DURATION_MS;
  if (duration > 0) {
    setTimeout(() => dismissToast(t.id), duration);
  }
  return t.id;
}

export function dismissToast(id: number): void {
  toasts.update((list) => list.filter((t) => t.id !== id));
}
