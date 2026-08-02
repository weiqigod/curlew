// Run lifecycle state machine (UI_SPECIFICATION.md §10.3.2) as a pure
// transition(state, action) function. Side effects (toasts, navigation,
// seeding, WS subscribe) live in the store layer — never here.
//
//   idle ──run_click──► starting ──start_ok──► running ──run.state:cancelling──► cancelling
//     ▲                    │                      │                                  │
//     │              409/403/422/network          ├─run.state:completed──► completed │
//     │                    ▼                      ├─run.state:error──────► error ◄───┤
//     └──── run_click ◄── idle                    └─run.state:cancelled──► cancelled◄┘

export type RunUiState =
  | 'idle'
  | 'starting'
  | 'running'
  | 'cancelling'
  | 'completed'
  | 'cancelled'
  | 'error';

/** run.state values carried by WS run.state frames (§5.1). */
export type RunWireState = 'running' | 'cancelling' | 'completed' | 'cancelled' | 'error';

export type RunAction =
  /** Run button clicked — POST /runs goes in flight (optimistic UI). */
  | { type: 'run_click' }
  /** POST /runs answered 202. */
  | { type: 'start_ok' }
  /** 409 run_active — back to idle (+ toast with View action). */
  | { type: 'start_conflict' }
  /** 422 collection_invalid / env_not_found — back to idle (+ navigation/toast). */
  | { type: 'start_invalid' }
  /** Network failure on POST /runs — back to idle (+ reachability flow). */
  | { type: 'start_network_error' }
  /** Cancel button clicked while running — POST cancel goes in flight. */
  | { type: 'cancel_click' }
  /** Adopt an already-active run (409 View action, or hello with a live run). */
  | { type: 'adopt'; state: 'running' | 'cancelling' }
  /** WS run.state frame for the subscribed run. */
  | { type: 'run_state'; state: RunWireState }
  /** Navigated to a historical run — the machine returns to idle. */
  | { type: 'view_historical' };

const TERMINAL: ReadonlySet<RunUiState> = new Set(['completed', 'cancelled', 'error']);

/** Pure transition function — unit-tested exhaustively. */
export function transition(state: RunUiState, action: RunAction): RunUiState {
  switch (action.type) {
    case 'run_click':
      // A new run can start from idle or any terminal state.
      return state === 'idle' || TERMINAL.has(state) ? 'starting' : state;
    case 'start_ok':
      return state === 'starting' ? 'running' : state;
    case 'start_conflict':
    case 'start_invalid':
    case 'start_network_error':
      return state === 'starting' ? 'idle' : state;
    case 'cancel_click':
      return state === 'running' ? 'cancelling' : state;
    case 'adopt':
      return action.state;
    case 'view_historical':
      return 'idle';
    case 'run_state':
      switch (action.state) {
        case 'running':
          // Confirms an in-flight start (WS may beat the 202 handler).
          return state === 'starting' || state === 'running' ? 'running' : state;
        case 'cancelling':
          return state === 'starting' || state === 'running' || state === 'cancelling'
            ? 'cancelling'
            : state;
        case 'completed':
        case 'cancelled':
        case 'error':
          // Terminal frames apply from any active state; replays re-enter
          // terminal states idempotently. idle (historical viewing) ignores
          // replayed terminal frames.
          return state === 'idle' ? state : action.state;
      }
  }
}
