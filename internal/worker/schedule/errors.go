package schedule

import "errors"

// Sentinel errors for the schedule-pull worker lifecycle.
var (
	// ErrNoRunAvailable is returned when GET /api/v1/schedules/next-run
	// responds with HTTP 204 (no pending run to claim).
	ErrNoRunAvailable = errors.New("schedule: no runs available (HTTP 204)")

	// ErrClaimReaped is returned when a heartbeat or post call receives HTTP
	// 409 with problem type "claim-reaped" or "stale-claim", indicating the
	// ShardReaper has reclaimed the run.
	ErrClaimReaped = errors.New("schedule: claim reaped (HTTP 409)")

	// ErrAlreadyCompleted is returned when posting a result receives HTTP 409
	// with problem type "already-completed", meaning the server already has a
	// result for this run_id (idempotent guard from M16-010).
	ErrAlreadyCompleted = errors.New("schedule: run already completed (HTTP 409)")

	// ErrUnsupportedCollectionRef is returned when collection_ref uses a
	// scheme other than "file:" (e.g. "git:").
	ErrUnsupportedCollectionRef = errors.New("schedule: unsupported collection_ref scheme")

	// ErrPostExhausted is returned when result posting has failed after
	// exhausting exponential backoff; the caller should queue locally.
	ErrPostExhausted = errors.New("schedule: result post failed after retries; queued locally")
)
