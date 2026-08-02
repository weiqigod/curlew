// Hand-rolled Myers O(ND) line diff (UI_SPECIFICATION.md §10.6.4.5).
// Pipeline: canonicalize (sortKeysDeep + 2-space JSON.stringify, arrays NOT
// sorted) → Myers line diff → intra-line common-prefix/suffix marking for
// paired -/+ lines → optional changes-only hunking with 2 context lines.
// Guard: canonicalized inputs > 1.5 MB → 'too_large'.

export type DiffLineType = 'context' | 'del' | 'add';

export interface DiffLine {
  type: DiffLineType;
  text: string;
  /** 1-based line number in A (context and del lines). */
  aLine?: number;
  /** 1-based line number in B (context and add lines). */
  bLine?: number;
  /** Intra-line changed span [markStart, markEnd) for paired del/add lines. */
  markStart?: number;
  markEnd?: number;
}

export interface DiffHunk {
  lines: DiffLine[];
}

export type DiffResult =
  | { kind: 'too_large' }
  | {
      kind: 'diff';
      identical: boolean;
      /** True when both inputs parsed as JSON and were canonicalized. */
      isJson: boolean;
      added: number;
      removed: number;
      hunks: DiffHunk[];
    };

export interface DiffOptions {
  /** Filter to changed hunks, keeping `context` lines around each change. */
  changesOnly?: boolean;
  /** Context lines per hunk in changes-only mode (spec: 2). */
  context?: number;
}

const MAX_DIFF_BYTES = 1.5 * 1024 * 1024;

/** Recursively sorts object keys; arrays keep their order (NOT sorted). */
export function sortKeysDeep(v: unknown): unknown {
  if (Array.isArray(v)) {
    return v.map(sortKeysDeep);
  }
  if (v !== null && typeof v === 'object') {
    const src = v as Record<string, unknown>;
    const out: Record<string, unknown> = {};
    for (const key of Object.keys(src).sort()) {
      out[key] = sortKeysDeep(src[key]);
    }
    return out;
  }
  return v;
}

function tryParseJson(text: string): { ok: true; value: unknown } | { ok: false } {
  try {
    return { ok: true, value: JSON.parse(text) };
  } catch {
    return { ok: false };
  }
}

/**
 * Canonicalizes a body pair: when BOTH parse as JSON, key-sorted 2-space
 * stringification (so key-order churn doesn't pollute the diff); otherwise
 * the raw texts are diffed as-is.
 */
export function canonicalizePair(
  aText: string,
  bText: string,
): { a: string; b: string; isJson: boolean } {
  const pa = tryParseJson(aText);
  const pb = tryParseJson(bText);
  if (pa.ok && pb.ok) {
    return {
      a: JSON.stringify(sortKeysDeep(pa.value), null, 2),
      b: JSON.stringify(sortKeysDeep(pb.value), null, 2),
      isJson: true,
    };
  }
  return { a: aText, b: bText, isJson: false };
}

/**
 * Myers O(ND) greedy diff on line arrays. Returns the edit script as a flat
 * list of context/del/add lines with 1-based line numbers.
 */
export function myersDiff(a: string[], b: string[]): DiffLine[] {
  const n = a.length;
  const m = b.length;
  const max = n + m;
  if (max === 0) return [];

  const offset = max;
  const v = new Array<number>(2 * max + 2).fill(0);
  const trace: number[][] = [];

  let found = false;
  for (let d = 0; d <= max && !found; d++) {
    trace.push(v.slice());
    for (let k = -d; k <= d; k += 2) {
      let x: number;
      if (k === -d || (k !== d && v[offset + k - 1] < v[offset + k + 1])) {
        x = v[offset + k + 1]; // move down (insertion from b)
      } else {
        x = v[offset + k - 1] + 1; // move right (deletion from a)
      }
      let y = x - k;
      while (x < n && y < m && a[x] === b[y]) {
        x++;
        y++;
      }
      v[offset + k] = x;
      if (x >= n && y >= m) {
        found = true;
        break;
      }
    }
  }

  // Backtrack: trace[d] is the V state before depth d was processed.
  const out: DiffLine[] = [];
  let x = n;
  let y = m;
  for (let d = trace.length - 1; d >= 0; d--) {
    const vd = trace[d];
    const k = x - y;
    let prevK: number;
    if (k === -d || (k !== d && vd[offset + k - 1] < vd[offset + k + 1])) {
      prevK = k + 1;
    } else {
      prevK = k - 1;
    }
    const prevX = vd[offset + prevK];
    const prevY = prevX - prevK;
    while (x > prevX && y > prevY) {
      out.push({ type: 'context', text: a[x - 1], aLine: x, bLine: y });
      x--;
      y--;
    }
    if (d > 0) {
      if (x === prevX) {
        out.push({ type: 'add', text: b[prevY], bLine: prevY + 1 });
      } else {
        out.push({ type: 'del', text: a[prevX], aLine: prevX + 1 });
      }
      x = prevX;
      y = prevY;
    }
  }
  out.reverse();
  return out;
}

/** Marks the changed span on paired -/+ lines via common prefix/suffix trim. */
function markIntraLine(lines: DiffLine[]): void {
  let i = 0;
  while (i < lines.length) {
    if (lines[i].type !== 'del') {
      i++;
      continue;
    }
    let j = i;
    while (j < lines.length && lines[j].type === 'del') j++;
    let k = j;
    while (k < lines.length && lines[k].type === 'add') k++;
    const pairs = Math.min(j - i, k - j);
    for (let p = 0; p < pairs; p++) {
      markPair(lines[i + p], lines[j + p]);
    }
    i = k;
  }
}

function markPair(del: DiffLine, add: DiffLine): void {
  const a = del.text;
  const b = add.text;
  const shorter = Math.min(a.length, b.length);
  let prefix = 0;
  while (prefix < shorter && a[prefix] === b[prefix]) prefix++;
  let suffix = 0;
  while (suffix < shorter - prefix && a[a.length - 1 - suffix] === b[b.length - 1 - suffix]) {
    suffix++;
  }
  del.markStart = prefix;
  del.markEnd = a.length - suffix;
  add.markStart = prefix;
  add.markEnd = b.length - suffix;
}

/** Changes-only hunking: keep `context` lines around each -/+ run. */
function hunkify(lines: DiffLine[], context: number): DiffHunk[] {
  const keep = new Array<boolean>(lines.length).fill(false);
  for (let i = 0; i < lines.length; i++) {
    if (lines[i].type !== 'context') {
      const from = Math.max(0, i - context);
      const to = Math.min(lines.length - 1, i + context);
      for (let j = from; j <= to; j++) keep[j] = true;
    }
  }
  const hunks: DiffHunk[] = [];
  let current: DiffLine[] | null = null;
  for (let i = 0; i < lines.length; i++) {
    if (keep[i]) {
      if (current === null) current = [];
      current.push(lines[i]);
    } else if (current !== null) {
      hunks.push({ lines: current });
      current = null;
    }
  }
  if (current !== null) hunks.push({ lines: current });
  return hunks;
}

/** Full §10.6.4.5 pipeline over two (already redacted) body texts. */
export function computeDiff(aText: string, bText: string, opts: DiffOptions = {}): DiffResult {
  const { changesOnly = false, context = 2 } = opts;
  const { a, b, isJson } = canonicalizePair(aText, bText);
  if (a.length > MAX_DIFF_BYTES || b.length > MAX_DIFF_BYTES) {
    return { kind: 'too_large' };
  }
  if (a === b) {
    return { kind: 'diff', identical: true, isJson, added: 0, removed: 0, hunks: [] };
  }
  const lines = myersDiff(a.split('\n'), b.split('\n'));
  markIntraLine(lines);
  let added = 0;
  let removed = 0;
  for (const line of lines) {
    if (line.type === 'add') added++;
    else if (line.type === 'del') removed++;
  }
  const hunks = changesOnly ? hunkify(lines, context) : [{ lines }];
  return { kind: 'diff', identical: false, isJson, added, removed, hunks };
}
