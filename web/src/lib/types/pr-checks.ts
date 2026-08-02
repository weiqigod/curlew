/** A single PR check row returned by the backend. */
export interface PrCheck {
	id: string;
	repo: string;
	pr: number;
	state: 'success' | 'failure';
	result_id: string | null;
	created_at: string; // ISO 8601 UTC
	posted_at?: string | null; // ISO 8601 UTC; set when GitHub Checks API POST succeeded (M14-021)
	check_run_id?: number | null; // GitHub check_run.id; set alongside posted_at (M14-021)
}
