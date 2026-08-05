import { describe, expect, it } from 'vitest';
import { applyEvent, seedFromList, syntheticId, type RequestMap } from './event-reducer';
import type {
  AssertionResultEvent,
  RequestEndEvent,
  RequestStartEvent,
  RunEndEvent,
  RunEvent,
  RunStartEvent,
} from './types/events';
import type { RequestListEntry } from './types/run';

const RUN_ID = 'a1b2c3d4e5f6a1b2c3d4e5f6a1b2c3d4';

function header(id: number, at_ms: number) {
  return { schema_version: '1.3', run_id: RUN_ID, id, at_ms };
}

function entry(overrides: Partial<RequestListEntry> & { request_id: string; slug: string }): RequestListEntry {
  return {
    name: overrides.slug,
    phase: 'main',
    method: 'GET',
    outcome: null,
    duration_ms: 0,
    wave_index: -1,
    retry_count: 0,
    skip_reason: null,
    fail_message: null,
    error: null,
    iteration: null,
    source_file: 'collections/users.yaml',
    source_line: 1,
    ...overrides,
  };
}

/** A planned (pending) light list as the server returns it right after start. */
function plannedList(): RequestListEntry[] {
  return [
    entry({ request_id: 'slug:get-token', slug: 'get-token', name: 'Get token', phase: 'setup', wave_index: 0, source_line: 3 }),
    entry({ request_id: 'slug:create-user', slug: 'create-user', name: 'Create user', method: 'POST', wave_index: 0, source_line: 12 }),
    entry({ request_id: 'slug:get-user', slug: 'get-user', name: 'Get user', wave_index: 1, source_line: 20 }),
  ];
}

function reqStart(id: number, at_ms: number, request_id: string, slug: string): RequestStartEvent {
  return {
    ...header(id, at_ms),
    kind: 'request.start',
    request_id,
    request_slug: slug,
    method: 'POST',
    url: 'https://api.example.com/users',
    phase: 'main',
  };
}

function reqEnd(
  id: number,
  at_ms: number,
  request_id: string,
  overrides: Partial<RequestEndEvent> = {},
): RequestEndEvent {
  return {
    ...header(id, at_ms),
    kind: 'request.end',
    request_id,
    outcome: 'passed',
    duration_ms: 42,
    ...overrides,
  };
}

function assertion(
  id: number,
  request_id: string,
  overrides: Partial<AssertionResultEvent> = {},
): AssertionResultEvent {
  return {
    ...header(id, 40),
    kind: 'assertion.result',
    request_id,
    type: 'status',
    label: 'status',
    passed: false,
    expected: '201',
    actual: '422',
    ...overrides,
  };
}

function applyAll(map: RequestMap, events: RunEvent[]): RequestMap {
  return events.reduce(applyEvent, map);
}

describe('seedFromList', () => {
  it('keys rows by request_id and maps null outcome to pending', () => {
    const map = seedFromList(plannedList());
    expect([...map.keys()]).toEqual(['slug:get-token', 'slug:create-user', 'slug:get-user']);
    for (const row of map.values()) {
      expect(row.status).toBe('pending');
      expect(row.duration_ms).toBeUndefined();
    }
    expect(map.get('slug:create-user')).toMatchObject({
      slug: 'create-user',
      name: 'Create user',
      method: 'POST',
      phase: 'main',
      wave_index: 0,
      source_file: 'collections/users.yaml',
      source_line: 12,
    });
  });

  it('maps completed entries (historical run) to terminal rows', () => {
    const map = seedFromList([
      entry({
        request_id: 'req-1',
        slug: 'create-user',
        outcome: 'failed',
        status_code: 422,
        duration_ms: 184,
        fail_message: 'status: expected 201, got 422',
        wave_index: 1,
      }),
      entry({
        request_id: 'req-2',
        slug: 'get-user',
        outcome: 'skipped',
        skip_reason: 'dependency "Create user" failed',
      }),
    ]);
    expect(map.get('req-1')).toMatchObject({
      status: 'failed',
      status_code: 422,
      duration_ms: 184,
      fail_message: 'status: expected 201, got 422',
      wave_index: 1,
    });
    expect(map.get('req-2')).toMatchObject({
      status: 'skipped',
      skip_reason: 'dependency "Create user" failed',
    });
  });
});

describe('applyEvent — request.start', () => {
  it('re-keys a pending "slug:<slug>" row to the real id (server adoption)', () => {
    const map = seedFromList(plannedList());
    const next = applyEvent(map, reqStart(3, 17, 'req-2', 'create-user'));

    expect(next.has(syntheticId('create-user'))).toBe(false);
    const row = next.get('req-2');
    expect(row).toBeDefined();
    expect(row).toMatchObject({
      request_id: 'req-2',
      slug: 'create-user',
      name: 'Create user', // structural fields preserved from the seed
      method: 'POST',
      status: 'running',
      wave_index: 0,
      at_ms: 17,
    });
    // Display order is preserved across the re-key.
    expect([...next.keys()]).toEqual(['slug:get-token', 'req-2', 'slug:get-user']);
  });

  it('marks an already-keyed row running', () => {
    const map = seedFromList([entry({ request_id: 'req-1', slug: 'create-user' })]);
    const next = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    expect(next.get('req-1')).toMatchObject({ status: 'running', at_ms: 5 });
  });

  it('inserts unknown requests fresh with wave_index -1 (unplanned expansion)', () => {
    const map = seedFromList(plannedList());
    const ev: RequestStartEvent = {
      ...header(9, 80),
      kind: 'request.start',
      request_id: 'req-7',
      request_slug: 'create-user-2',
      name: 'Create user #2',
      method: 'POST',
      url: 'https://api.example.com/users',
      phase: 'main',
      source_file: 'collections/users.yaml',
      source_line: 12,
    };
    const next = applyEvent(map, ev);
    expect(next.get('req-7')).toMatchObject({
      status: 'running',
      wave_index: -1,
      retry_count: 0,
      name: 'Create user #2',
      slug: 'create-user-2',
      source_line: 12,
    });
    expect(next.size).toBe(4);
  });
});

describe('applyEvent — request.end', () => {
  it('sets outcome, status_code and duration_ms', () => {
    let map = seedFromList(plannedList());
    map = applyEvent(map, reqStart(3, 17, 'req-2', 'create-user'));
    map = applyEvent(map, reqEnd(5, 60, 'req-2', { request_slug: 'create-user', outcome: 'passed', status_code: 201, duration_ms: 43 }));
    expect(map.get('req-2')).toMatchObject({
      status: 'passed',
      status_code: 201,
      duration_ms: 43,
    });
  });

  it('IGNORES the frame wave_index — omitted-when-0 trap', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user', wave_index: 2 })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));

    // Event WITH wave_index 0 must not affect grouping…
    const withWave = applyEvent(map, reqEnd(4, 50, 'req-1', { wave_index: 0 }));
    expect(withWave.get('req-1')?.wave_index).toBe(2);

    // …and an event WITHOUT the field must produce the identical row.
    const withoutWave = applyEvent(map, reqEnd(4, 50, 'req-1'));
    expect(withoutWave.get('req-1')).toEqual(withWave.get('req-1'));
  });

  it('ignores timing and truncated bodies from the event', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(
      map,
      reqEnd(4, 50, 'req-1', {
        timing: { total_us: 41100, ttfb_us: 21200 },
        response_body: '{"id":1}',
        response_body_truncated: true,
        response_body_size: 32768,
      }),
    );
    const row = map.get('req-1') as unknown as Record<string, unknown>;
    expect(row.timing).toBeUndefined();
    expect(row.response_body).toBeUndefined();
    expect(row.response_body_size).toBeUndefined();
  });

  it('stores the compact error object for error outcomes', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(
      map,
      reqEnd(4, 5001, 'req-1', {
        outcome: 'error',
        duration_ms: 5001,
        error: {
          category: 'network',
          code: 'NETWORK_TIMEOUT',
          message: 'request timed out after 5000ms',
          hint: 'increase the timeout',
        },
      }),
    );
    expect(map.get('req-1')?.status).toBe('error');
    expect(map.get('req-1')?.error).toEqual({
      category: 'network',
      code: 'NETWORK_TIMEOUT',
      message: 'request timed out after 5000ms',
    });
  });

  it('tolerates request.end on an already-terminal row (overwrite, no duplicate)', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(map, reqEnd(4, 50, 'req-1', { outcome: 'passed', status_code: 200 }));
    map = applyEvent(map, reqEnd(4, 50, 'req-1', { outcome: 'passed', status_code: 200 }));
    expect(map.size).toBe(1);
    expect(map.get('req-1')?.status).toBe('passed');
  });

  it('adopts the planned row when request.end arrives without a seen request.start', () => {
    const map = seedFromList(plannedList());
    const next = applyEvent(
      map,
      reqEnd(4, 50, 'req-2', { request_slug: 'create-user', outcome: 'passed', status_code: 201 }),
    );
    expect(next.has(syntheticId('create-user'))).toBe(false);
    expect(next.get('req-2')).toMatchObject({ name: 'Create user', status: 'passed' });
    expect(next.size).toBe(3);
  });
});

describe('applyEvent — assertion.result', () => {
  it('sets fail_message "<type>: expected <e>, got <a>" on failure', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(map, assertion(3, 'req-1'));
    expect(map.get('req-1')?.fail_message).toBe('status: expected 201, got 422');
  });

  it('only sets fail_message when unset (first failing assertion wins)', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(map, assertion(3, 'req-1'));
    map = applyEvent(map, assertion(4, 'req-1', { type: 'body', expected: '100', actual: '95' }));
    expect(map.get('req-1')?.fail_message).toBe('status: expected 201, got 422');
  });

  it('ignores passing assertions', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    const next = applyEvent(map, assertion(3, 'req-1', { passed: true }));
    expect(next.get('req-1')?.fail_message).toBeUndefined();
  });

  it('tolerates out-of-order assertion.result after request.end', () => {
    let map = seedFromList([entry({ request_id: 'slug:create-user', slug: 'create-user' })]);
    map = applyEvent(map, reqStart(2, 5, 'req-1', 'create-user'));
    map = applyEvent(map, reqEnd(4, 50, 'req-1', { outcome: 'failed', status_code: 422 }));
    map = applyEvent(map, assertion(3, 'req-1'));
    expect(map.get('req-1')).toMatchObject({
      status: 'failed',
      fail_message: 'status: expected 201, got 422',
    });
  });

  it('tolerates assertions for unknown request ids', () => {
    const map = seedFromList(plannedList());
    expect(applyEvent(map, assertion(3, 'req-99'))).toBe(map);
  });
});

describe('applyEvent — run-level events', () => {
  const runStart: RunStartEvent = {
    ...header(1, 0),
    kind: 'run.start',
    started_at: '2026-06-11T09:30:00Z',
    curlew_version: '1.0.0',
    cli_args: ['run', 'collections/users.yaml'],
  };
  const runEnd: RunEndEvent = {
    ...header(9, 123),
    kind: 'run.end',
    duration_ms: 123,
    total: 3,
    passed: 3,
    failed: 0,
    skipped: 0,
    exit_code: 0,
    event_count: 9,
  };

  it('run.start, run.end and run.error never mutate rows', () => {
    const map = seedFromList(plannedList());
    expect(applyEvent(map, runStart)).toBe(map);
    expect(applyEvent(map, runEnd)).toBe(map);
    expect(
      applyEvent(map, {
        ...header(2, 0),
        kind: 'run.error',
        error: { category: 'parse', message: 'unexpected token at line 5' },
      }),
    ).toBe(map);
  });
});

describe('replay idempotency', () => {
  // A realistic stream: setup passes, main fails an assertion, dependent skips.
  function stream(): RunEvent[] {
    return [
      {
        ...header(1, 0),
        kind: 'run.start',
        started_at: '2026-06-11T09:30:00Z',
        curlew_version: '1.0.0',
        cli_args: ['run', 'collections/users.yaml'],
      },
      reqStart(2, 1, 'req-1', 'get-token'),
      assertion(3, 'req-1', { passed: true }),
      reqEnd(4, 30, 'req-1', { request_slug: 'get-token', status_code: 200, duration_ms: 29 }),
      reqStart(5, 31, 'req-2', 'create-user'),
      assertion(6, 'req-2'),
      reqEnd(7, 90, 'req-2', { request_slug: 'create-user', outcome: 'failed', status_code: 422, duration_ms: 59 }),
      reqEnd(8, 91, 'req-3', { request_slug: 'get-user', outcome: 'skipped', duration_ms: 0 }),
      {
        ...header(9, 95),
        kind: 'run.end',
        duration_ms: 95,
        total: 3,
        passed: 1,
        failed: 1,
        skipped: 1,
        exit_code: 1,
        event_count: 9,
      },
    ];
  }

  it('replaying the full stream a second time leaves the map deep-equal', () => {
    const seed = seedFromList(plannedList());
    const once = applyAll(seed, stream());
    const twice = applyAll(once, stream());
    expect(Object.fromEntries(twice)).toEqual(Object.fromEntries(once));
    expect([...twice.keys()]).toEqual([...once.keys()]);
  });

  it('checkpoint statuses match the stream', () => {
    const seed = seedFromList(plannedList());
    const events = stream();

    let map = applyAll(seed, events.slice(0, 3));
    expect(map.get('req-1')?.status).toBe('running');
    expect(map.get('slug:create-user')?.status).toBe('pending');

    map = applyAll(map, events.slice(3, 7));
    expect(map.get('req-1')?.status).toBe('passed');
    expect(map.get('req-2')).toMatchObject({
      status: 'failed',
      fail_message: 'status: expected 201, got 422',
    });

    map = applyAll(map, events.slice(7));
    expect(map.get('req-3')?.status).toBe('skipped');
    expect(map.size).toBe(3);
    expect([...map.values()].map((r) => r.status)).toEqual(['passed', 'failed', 'skipped']);
  });
});
