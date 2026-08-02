<script lang="ts">
	import '../../app.css';
	import { page } from '$app/stores';
	import { derived } from 'svelte/store';
	import type { LayoutData } from './$types';

	export let data: LayoutData;

	/** Current org slug from the URL, if inside an /org/[slug] route. */
	const currentSlug = derived(page, ($p) => $p.params.slug as string | undefined);

	/** The organization matching the current route slug, if any. */
	$: currentOrg = $currentSlug
		? (data.organizations ?? []).find((o) => o.slug === $currentSlug)
		: undefined;

	/** Whether the current org can use Team-tier features. */
	$: isTeamTier =
		!currentOrg || currentOrg.tier === 'team' || currentOrg.tier === 'enterprise';

	/** Whether the current org can use Enterprise-only features. */
	$: isEnterpriseTier = !currentOrg || currentOrg.tier === 'enterprise';

	/** Whether the current org user has admin/owner role. */
	$: isAdmin = !!currentOrg && (currentOrg.role === 'owner' || currentOrg.role === 'admin');

	/** Toast message from query param (e.g. ?toast=team_tier_required). */
	$: toastParam = $page.url.searchParams.get('toast');
	$: showTeamTierToast = toastParam === 'team_tier_required';
	$: showEnterpriseTierToast = toastParam === 'enterprise_tier_required';
	$: showAdminRequiredToast = toastParam === 'admin_required';
	$: showOwnerRequiredToast = toastParam === 'owner_required';

	/** Dismiss the toast by removing the query param (client-side navigation). */
	let dismissed = false;
	$: if (
		toastParam !== 'team_tier_required' &&
		toastParam !== 'enterprise_tier_required' &&
		toastParam !== 'admin_required' &&
		toastParam !== 'owner_required'
	)
		dismissed = false;
</script>

{#if showTeamTierToast && !dismissed}
	<div
		data-testid="toast-team-tier-required"
		role="alert"
		class="fixed inset-x-0 top-4 z-50 mx-auto max-w-md rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 shadow-md"
	>
		<div class="flex items-center justify-between gap-4">
			<p class="text-sm font-medium text-amber-800">Team tier required</p>
			<button
				type="button"
				aria-label="Dismiss notification"
				class="text-amber-600 hover:text-amber-900"
				on:click={() => (dismissed = true)}
			>
				&times;
			</button>
		</div>
	</div>
{/if}

{#if showEnterpriseTierToast && !dismissed}
	<div
		data-testid="toast-enterprise-tier-required"
		role="alert"
		class="fixed inset-x-0 top-4 z-50 mx-auto max-w-md rounded-lg border border-amber-300 bg-amber-50 px-4 py-3 shadow-md"
	>
		<div class="flex items-center justify-between gap-4">
			<p class="text-sm font-medium text-amber-800">Enterprise tier required</p>
			<button
				type="button"
				aria-label="Dismiss notification"
				class="text-amber-600 hover:text-amber-900"
				on:click={() => (dismissed = true)}
			>
				&times;
			</button>
		</div>
	</div>
{/if}

{#if showAdminRequiredToast && !dismissed}
	<div
		data-testid="toast-admin-required"
		role="alert"
		class="fixed inset-x-0 top-4 z-50 mx-auto max-w-md rounded-lg border border-red-300 bg-red-50 px-4 py-3 shadow-md"
	>
		<div class="flex items-center justify-between gap-4">
			<p class="text-sm font-medium text-red-800">Admin role required</p>
			<button
				type="button"
				aria-label="Dismiss notification"
				class="text-red-600 hover:text-red-900"
				on:click={() => (dismissed = true)}
			>
				&times;
			</button>
		</div>
	</div>
{/if}

{#if showOwnerRequiredToast && !dismissed}
	<div
		data-testid="toast-owner-required"
		role="alert"
		class="fixed inset-x-0 top-4 z-50 mx-auto max-w-md rounded-lg border border-red-300 bg-red-50 px-4 py-3 shadow-md"
	>
		<div class="flex items-center justify-between gap-4">
			<p class="text-sm font-medium text-red-800">Owner role required</p>
			<button
				type="button"
				aria-label="Dismiss notification"
				class="text-red-600 hover:text-red-900"
				on:click={() => (dismissed = true)}
			>
				&times;
			</button>
		</div>
	</div>
{/if}

{#if currentOrg}
	<nav
		data-testid="org-subnav"
		class="border-b border-gray-200 bg-white"
		aria-label="Organisation navigation"
	>
		<div class="mx-auto flex max-w-6xl items-center gap-6 px-4 py-2 text-sm font-medium">
			<span class="text-gray-900">{currentOrg.name}</span>
			{#if isTeamTier}
				<a
					href="/org/{currentOrg.slug}/dashboard"
					data-testid="subnav-dashboard-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Dashboard
				</a>
				<a
					href="/org/{currentOrg.slug}/results"
					data-testid="subnav-results-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Test Results
				</a>
				{#if isAdmin}
					<a
						href="/org/{currentOrg.slug}/schedules"
						data-testid="subnav-schedules-link"
						class="text-gray-600 hover:text-gray-900"
					>
						Schedules
					</a>
					<a
						href="/org/{currentOrg.slug}/integrations/gitlab"
						data-testid="subnav-gitlab-link"
						class="text-gray-600 hover:text-gray-900"
					>
						GitLab
					</a>
				{/if}
			{/if}
			{#if isTeamTier && isAdmin}
				<a
					href="/org/{currentOrg.slug}/settings/notifications"
					data-testid="subnav-notifications-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Notifications
				</a>
			{/if}
			{#if isTeamTier && currentOrg?.role === 'owner'}
				<a
					href="/org/{currentOrg.slug}/billing"
					data-testid="subnav-billing-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Billing
				</a>
			{/if}
			{#if isTeamTier && isAdmin}
				<a
					href="/org/{currentOrg.slug}/members"
					data-testid="subnav-members-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Members
				</a>
			{/if}
			{#if isEnterpriseTier && currentOrg?.role === 'owner'}
				<a
					href="/org/{currentOrg.slug}/settings/sso"
					data-testid="subnav-sso-link"
					class="text-gray-600 hover:text-gray-900"
				>
					SSO
				</a>
				<a
					href="/org/{currentOrg.slug}/settings/roles"
					data-testid="subnav-roles-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Roles
				</a>
			{/if}
			{#if isEnterpriseTier && isAdmin}
				<a
					href="/org/{currentOrg.slug}/audit-log"
					data-testid="subnav-audit-log-link"
					class="text-gray-600 hover:text-gray-900"
				>
					Audit Log
				</a>
			{/if}
		</div>
	</nav>
{/if}

<slot />
