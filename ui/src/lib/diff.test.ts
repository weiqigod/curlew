import { describe, expect, it } from 'vitest';
import { canonicalizePair, computeDiff, myersDiff, type DiffLine } from './diff';

/** Deterministic PRNG (mulberry32) so randomized cases are reproducible. */
function mulberry32(seed: number): () => number {
  let a = seed;
  return () => {
    a |= 0;
    a = (a + 0x6d2b79f5) | 0;
    let t = Math.imul(a ^ (a >>> 15), 1 | a);
    t = (t + Math.imul(t ^ (t >>> 7), 61 | t)) ^ t;
    return ((t ^ (t >>> 14)) >>> 0) / 4294967296;
  };
}

/** Brute-force LCS length via DP — the reference for edit-script minimality. */
function lcsLength(a: string[], b: string[]): number {
  const dp: number[][] = Array.from({ length: a.length + 1 }, () =>
    new Array<number>(b.length + 1).fill(0),
  );
  for (let i = 1; i <= a.length; i++) {
    for (let j = 1; j <= b.length; j++) {
      dp[i][j] = a[i - 1] === b[j - 1] ? dp[i - 1][j - 1] + 1 : Math.max(dp[i - 1][j], dp[i][j - 1]);
    }
  }
  return dp[a.length][b.length];
}

/** Applies the edit script to A and returns the reconstructed B. */
function reconstruct(lines: DiffLine[]): string[] {
  return lines.filter((l) => l.type !== 'del').map((l) => l.text);
}

function keptFromA(lines: DiffLine[]): string[] {
  return lines.filter((l) => l.type !== 'add').map((l) => l.text);
}

describe('myersDiff — correctness vs brute-force LCS', () => {
  it('matches the brute-force LCS on randomized small inputs', () => {
    const rand = mulberry32(0xa91e57);
    const alphabet = ['a', 'b', 'c'];
    for (let trial = 0; trial < 300; trial++) {
      const n = Math.floor(rand() * 9);
      const m = Math.floor(rand() * 9);
      const a = Array.from({ length: n }, () => alphabet[Math.floor(rand() * alphabet.length)]);
      const b = Array.from({ length: m }, () => alphabet[Math.floor(rand() * alphabet.length)]);

      const lines = myersDiff(a, b);

      // The edit script must transform A into B…
      expect(keptFromA(lines), `A mismatch: [${a}] vs [${b}]`).toEqual(a);
      expect(reconstruct(lines), `B mismatch: [${a}] vs [${b}]`).toEqual(b);

      // …with the minimal number of edits (Myers optimality = LCS distance).
      const dels = lines.filter((l) => l.type === 'del').length;
      const adds = lines.filter((l) => l.type === 'add').length;
      const lcs = lcsLength(a, b);
      expect(dels + adds, `edit count: [${a}] vs [${b}]`).toBe(n + m - 2 * lcs);
    }
  });

  it('handles empty inputs', () => {
    expect(myersDiff([], [])).toEqual([]);
    expect(myersDiff([], ['x', 'y'])).toEqual([
      { type: 'add', text: 'x', bLine: 1 },
      { type: 'add', text: 'y', bLine: 2 },
    ]);
    expect(myersDiff(['x'], [])).toEqual([{ type: 'del', text: 'x', aLine: 1 }]);
  });

  it('assigns 1-based line numbers to context lines on both sides', () => {
    const lines = myersDiff(['same', 'old'], ['same', 'new']);
    expect(lines[0]).toEqual({ type: 'context', text: 'same', aLine: 1, bLine: 1 });
  });
});

describe('sortKeysDeep / canonicalization', () => {
  it('is key-order invariant (objects sorted, arrays NOT sorted)', () => {
    const a = '{"b": 2, "a": {"y": [3, 1, 2], "x": 0}}';
    const b = '{"a": {"x": 0, "y": [3, 1, 2]}, "b": 2}';
    const result = computeDiff(a, b);
    expect(result).toMatchObject({ kind: 'diff', identical: true, isJson: true });
  });

  it('does NOT sort arrays', () => {
    const result = computeDiff('[2, 1]', '[1, 2]');
    expect(result).toMatchObject({ kind: 'diff', identical: false, isJson: true });
  });

  it('falls back to raw text when either side is not JSON', () => {
    const { isJson } = canonicalizePair('{"a":1}', 'plain text');
    expect(isJson).toBe(false);
  });

  it('canonicalizes with 2-space indentation', () => {
    const { a } = canonicalizePair('{"b":1,"a":2}', '{}');
    expect(a).toBe('{\n  "a": 2,\n  "b": 1\n}');
  });
});

describe('computeDiff — intra-line marking', () => {
  it('marks the changed span on paired -/+ lines via common prefix/suffix', () => {
    const result = computeDiff('{"amount": 100}', '{"amount": 95}');
    if (result.kind !== 'diff') throw new Error('expected diff');
    const del = result.hunks[0].lines.find((l) => l.type === 'del');
    const add = result.hunks[0].lines.find((l) => l.type === 'add');
    //   "amount": 100   vs   "amount": 95
    expect(del?.text).toBe('  "amount": 100');
    expect(add?.text).toBe('  "amount": 95');
    expect(del?.markStart).toBe(12); // after '  "amount": '
    expect(del?.markEnd).toBe(15);
    expect(add?.markStart).toBe(12);
    expect(add?.markEnd).toBe(14);
    // Marked spans are exactly the differing text.
    expect(del?.text.slice(del.markStart, del.markEnd)).toBe('100');
    expect(add?.text.slice(add.markStart, add.markEnd)).toBe('95');
  });

  it('leaves unpaired del/add lines unmarked', () => {
    const result = computeDiff('a\nb', 'a');
    if (result.kind !== 'diff') throw new Error('expected diff');
    const del = result.hunks[0].lines.find((l) => l.type === 'del');
    expect(del?.markStart).toBeUndefined();
  });
});

describe('computeDiff — changes-only hunking', () => {
  const aLines = ['l1', 'l2', 'l3', 'l4', 'l5', 'l6', 'l7', 'l8', 'l9', 'l10'];

  it('keeps 2 context lines around each change', () => {
    const b = [...aLines];
    b[4] = 'CHANGED'; // l5
    const result = computeDiff(aLines.join('\n'), b.join('\n'), { changesOnly: true });
    if (result.kind !== 'diff') throw new Error('expected diff');
    expect(result.hunks).toHaveLength(1);
    expect(result.hunks[0].lines.map((l) => l.text)).toEqual([
      'l3',
      'l4',
      'l5',
      'CHANGED',
      'l6',
      'l7',
    ]);
  });

  it('splits far-apart changes into separate hunks', () => {
    const b = [...aLines];
    b[0] = 'FIRST';
    b[9] = 'LAST';
    const result = computeDiff(aLines.join('\n'), b.join('\n'), { changesOnly: true });
    if (result.kind !== 'diff') throw new Error('expected diff');
    expect(result.hunks).toHaveLength(2);
  });

  it('merges overlapping context into one hunk', () => {
    const b = [...aLines];
    b[3] = 'X'; // l4
    b[6] = 'Y'; // l7 — context windows (±2) touch
    const result = computeDiff(aLines.join('\n'), b.join('\n'), { changesOnly: true });
    if (result.kind !== 'diff') throw new Error('expected diff');
    expect(result.hunks).toHaveLength(1);
  });

  it('returns the full edit script as one hunk when changesOnly is off', () => {
    const b = [...aLines];
    b[4] = 'CHANGED';
    const result = computeDiff(aLines.join('\n'), b.join('\n'));
    if (result.kind !== 'diff') throw new Error('expected diff');
    expect(result.hunks).toHaveLength(1);
    expect(result.hunks[0].lines).toHaveLength(11); // 10 lines + 1 (del+add for the change)
  });
});

describe('computeDiff — totals and guards', () => {
  it('counts added and removed lines', () => {
    const result = computeDiff('a\nb\nc', 'a\nX\nc\nd');
    if (result.kind !== 'diff') throw new Error('expected diff');
    expect(result.removed).toBe(1);
    expect(result.added).toBe(2);
  });

  it('reports identical inputs', () => {
    const result = computeDiff('same\ntext', 'same\ntext');
    expect(result).toMatchObject({ kind: 'diff', identical: true, added: 0, removed: 0 });
    if (result.kind === 'diff') expect(result.hunks).toEqual([]);
  });

  it('guards against inputs > 1.5 MB after canonicalization', () => {
    const big = 'x'.repeat(Math.ceil(1.5 * 1024 * 1024) + 1);
    expect(computeDiff(big, 'small')).toEqual({ kind: 'too_large' });
    expect(computeDiff('small', big)).toEqual({ kind: 'too_large' });
  });
});
