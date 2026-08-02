// Package schedule implements the self-hosted worker side of the M16 schedule
// execution model. It polls GET /api/v1/schedules/next-run, resolves the
// file: collection ref locally, executes it through the existing runner.Run
// engine, sends heartbeats every 30s via POST /api/v1/schedules/runs/{id}/heartbeat,
// and posts results via POST /api/v1/schedules/runs/{id}/result.
//
// On unrecoverable post failure the result payload is queued under
// ~/.config/curlew/pending-uploads/<run_id>.json and drained on the next
// successful poll cycle.
//
// References:
//   - docs/SPECIFICATION.md "Schedule Execution Model"
//   - M16-009: backend endpoints (next-run, heartbeat, result)
//   - M16-010: result atomicity guard
//   - M16-011: this package
//
// Combining --schedule-pull with --perf-pull / --all-modes is reserved for a
// future release; this slice supports --schedule-pull alone.
package schedule
