// Display formatting helpers (UI_SPECIFICATION.md §10.2).

/** Last path segment for display, accepting API paths and native Windows paths. */
export function fileBasename(path: string): string {
  return path.split(/[\\/]/).pop() ?? '';
}

/** Milliseconds: "412 ms", "4.8 s", "1m 23s". */
export function fmtMs(ms: number): string {
  if (ms < 1000) return `${Math.round(ms)} ms`;
  if (ms < 60_000) return `${(ms / 1000).toFixed(1)} s`;
  const min = Math.floor(ms / 60_000);
  const sec = Math.round((ms % 60_000) / 1000);
  return `${min}m ${sec}s`;
}

/** Microseconds: < 1000 µs → "412 µs", else ms with one decimal → "48.2 ms". */
export function fmtUs(us: number): string {
  if (us < 1000) return `${Math.round(us)} µs`;
  return `${(us / 1000).toFixed(1)} ms`;
}

/** Bytes (binary units): "512 B", "47.1 KB", "1.5 MB". */
export function fmtBytes(n: number): string {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${trimZero((n / 1024).toFixed(1))} KB`;
  return `${trimZero((n / (1024 * 1024)).toFixed(1))} MB`;
}

function trimZero(s: string): string {
  return s.endsWith('.0') ? s.slice(0, -2) : s;
}

/** Relative time for run timestamps: "just now", "4m ago", "3h ago", "2d ago". */
export function relTime(iso: string, now: number = Date.now()): string {
  const t = Date.parse(iso);
  if (Number.isNaN(t)) return iso;
  const diff = Math.max(0, now - t);
  if (diff < 60_000) return 'just now';
  if (diff < 3_600_000) return `${Math.floor(diff / 60_000)}m ago`;
  if (diff < 86_400_000) return `${Math.floor(diff / 3_600_000)}h ago`;
  return `${Math.floor(diff / 86_400_000)}d ago`;
}

/** "14:32" local clock time for history-rail rows. */
export function clockTime(iso: string): string {
  const t = new Date(iso);
  if (Number.isNaN(t.getTime())) return iso;
  return t.toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' });
}
