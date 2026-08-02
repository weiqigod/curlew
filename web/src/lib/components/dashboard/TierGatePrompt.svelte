<script lang="ts">
	/** Org slug used to build the billing upgrade link href. */
	export let slug: string;
</script>

<div
	data-testid="dashboard-tier-gate"
	role="region"
	aria-label="Upgrade required"
	class="rounded-lg border border-amber-300 bg-amber-50 p-8 text-center"
>
	<h2 class="text-lg font-semibold text-amber-900">Team tier required</h2>
	<p class="mt-2 text-sm text-amber-800">
		The dashboard is a Team-tier feature. Upgrade your organization to access overview cards, trend
		charts, and failure analytics.
	</p>
	<a
		href="/org/{slug}/billing"
		class="mt-4 inline-block rounded bg-amber-600 px-4 py-2 text-sm font-medium text-white hover:bg-amber-700"
		data-testid="dashboard-upgrade-link"
	>
		Upgrade plan
	</a>
</div>
