/** A cron-scheduled test run configuration scoped to an organization. */
export interface Schedule {
	/** Wire-format schedule id (e.g. `sched_<hex>`). */
	id: string;
	/** Human-readable schedule name (unique per org). */
	name: string;
	/** Standard 5-field cron expression (e.g. "0 2 * * *"). */
	cron_expression: string;
	/** IANA TZ identifier the cron expression is evaluated in. */
	timezone: string;
	/** Reference to the collection file to run. */
	collection_ref: string;
	/** Whether this schedule is active. */
	enabled: boolean;
	/** Computed next UTC fire time, or null if not yet computed. */
	next_run_at: string | null;
	/** UTC time of the last queued/completed run, or null if never fired. */
	last_run_at: string | null;
	/** UTC creation timestamp. */
	created_at: string;
}

/** A single execution record for a cron schedule. */
export interface ScheduledRun {
	/** Wire-format run id (e.g. `run_<hex>`). */
	run_id: string;
	/** Execution status. */
	status: 'queued' | 'running' | 'completed' | 'failed';
	/** UTC time the run was enqueued. */
	created_at: string;
	/** UTC time the worker started executing, or null if not yet started. */
	started_at: string | null;
	/** UTC time the run finished, or null if still running. */
	completed_at: string | null;
	/** Wire-format result id, populated once result is ingested (M16-010). */
	result_id: string | null;
}

/** Request body for creating a new cron schedule. */
export interface CreateScheduleRequest {
	/** Human-readable schedule name (unique per org). */
	name: string;
	/** Standard 5-field cron expression. */
	cron: string;
	/** IANA TZ identifier (defaults to "UTC" on backend if omitted). */
	timezone: string;
	/** Reference to the collection file to run. */
	collection_ref: string;
}
