import { describe, expect, it } from 'vitest';
import { transition, type RunAction, type RunUiState } from './run-machine';

const STATES: RunUiState[] = [
  'idle',
  'starting',
  'running',
  'cancelling',
  'completed',
  'cancelled',
  'error',
];

type Expected = Record<RunUiState, RunUiState>;

/** Asserts the full column of the transition table for one action. */
function expectColumn(action: RunAction, expected: Expected): void {
  for (const state of STATES) {
    expect(transition(state, action), `${state} + ${action.type}`).toBe(expected[state]);
  }
}

describe('transition — full table', () => {
  it('run_click starts from idle and from every terminal state', () => {
    expectColumn(
      { type: 'run_click' },
      {
        idle: 'starting',
        starting: 'starting',
        running: 'running',
        cancelling: 'cancelling',
        completed: 'starting',
        cancelled: 'starting',
        error: 'starting',
      },
    );
  });

  it('start_ok (202) only advances starting', () => {
    expectColumn(
      { type: 'start_ok' },
      {
        idle: 'idle',
        starting: 'running',
        running: 'running',
        cancelling: 'cancelling',
        completed: 'completed',
        cancelled: 'cancelled',
        error: 'error',
      },
    );
  });

  for (const failure of [
    'start_conflict',
    'start_invalid',
    'start_network_error',
  ] as const) {
    it(`${failure} (409/422/network) returns starting to idle`, () => {
      expectColumn(
        { type: failure },
        {
          idle: 'idle',
          starting: 'idle',
          running: 'running',
          cancelling: 'cancelling',
          completed: 'completed',
          cancelled: 'cancelled',
          error: 'error',
        },
      );
    });
  }

  it('cancel_click only applies while running', () => {
    expectColumn(
      { type: 'cancel_click' },
      {
        idle: 'idle',
        starting: 'starting',
        running: 'cancelling',
        cancelling: 'cancelling',
        completed: 'completed',
        cancelled: 'cancelled',
        error: 'error',
      },
    );
  });

  it('run_state:running confirms an in-flight start only', () => {
    expectColumn(
      { type: 'run_state', state: 'running' },
      {
        idle: 'idle',
        starting: 'running',
        running: 'running',
        cancelling: 'cancelling',
        completed: 'completed',
        cancelled: 'cancelled',
        error: 'error',
      },
    );
  });

  it('run_state:cancelling moves any active state to cancelling', () => {
    expectColumn(
      { type: 'run_state', state: 'cancelling' },
      {
        idle: 'idle',
        starting: 'cancelling',
        running: 'cancelling',
        cancelling: 'cancelling',
        completed: 'completed',
        cancelled: 'cancelled',
        error: 'error',
      },
    );
  });

  for (const terminal of ['completed', 'cancelled', 'error'] as const) {
    it(`run_state:${terminal} terminates active states and re-enters terminals idempotently`, () => {
      expectColumn(
        { type: 'run_state', state: terminal },
        {
          idle: 'idle', // historical viewing ignores replayed terminal frames
          starting: terminal,
          running: terminal,
          cancelling: terminal,
          completed: terminal,
          cancelled: terminal,
          error: terminal,
        },
      );
    });
  }

  for (const adopted of ['running', 'cancelling'] as const) {
    it(`adopt:${adopted} applies from any state (409 View / hello run)`, () => {
      expectColumn(
        { type: 'adopt', state: adopted },
        {
          idle: adopted,
          starting: adopted,
          running: adopted,
          cancelling: adopted,
          completed: adopted,
          cancelled: adopted,
          error: adopted,
        },
      );
    });
  }

  it('view_historical returns any state to idle', () => {
    expectColumn(
      { type: 'view_historical' },
      {
        idle: 'idle',
        starting: 'idle',
        running: 'idle',
        cancelling: 'idle',
        completed: 'idle',
        cancelled: 'idle',
        error: 'idle',
      },
    );
  });
});

describe('transition — spec walk-throughs (§10.3.2)', () => {
  function walk(start: RunUiState, actions: RunAction[]): RunUiState {
    return actions.reduce(transition, start);
  }

  it('happy path: idle → starting → running → completed → (new run) starting', () => {
    expect(
      walk('idle', [
        { type: 'run_click' },
        { type: 'start_ok' },
        { type: 'run_state', state: 'completed' },
        { type: 'run_click' },
      ]),
    ).toBe('starting');
  });

  it('cancel path: running → cancelling → cancelled', () => {
    expect(
      walk('running', [{ type: 'cancel_click' }, { type: 'run_state', state: 'cancelled' }]),
    ).toBe('cancelled');
  });

  it('409 with View action: starting → idle → adopt running', () => {
    expect(
      walk('idle', [
        { type: 'run_click' },
        { type: 'start_conflict' },
        { type: 'adopt', state: 'running' },
      ]),
    ).toBe('running');
  });

  it('run infrastructure error: running → error', () => {
    expect(walk('running', [{ type: 'run_state', state: 'error' }])).toBe('error');
  });
});
