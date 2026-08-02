<script lang="ts">
	import { userDeletionApi } from '$lib/api/user-deletion';
	import { isApiError } from '$lib/types/api-error';
	import type { BlockingOrg, DeletionRequestResponse } from '$lib/types/user-deletion';

	type Phase =
		| 'idle'
		| 'confirm'
		| 'reauth'
		| 'pending'
		| 'blocked'
		| 'error';

	let phase: Phase = 'idle';
	let password = '';
	let isSubmitting = false;
	let errorMessage: string | null = null;
	let blockingOrgs: BlockingOrg[] = [];
	let deletionResponse: DeletionRequestResponse | null = null;
	let reauthToken: string | null = null;

	function startConfirm() {
		phase = 'confirm';
		errorMessage = null;
	}

	function cancelConfirm() {
		phase = 'idle';
		password = '';
		errorMessage = null;
	}

	function openReauthModal() {
		phase = 'reauth';
		errorMessage = null;
	}

	async function submitReauth() {
		if (isSubmitting) return;
		isSubmitting = true;
		errorMessage = null;
		try {
			const resp = await userDeletionApi.issueReauthToken(password);
			reauthToken = resp.reauth_token;
			password = '';
			await submitDeletionRequest();
		} catch (err) {
			if (isApiError(err) && err.status === 401) {
				errorMessage = 'Incorrect password. Please try again.';
			} else {
				errorMessage = 'Failed to verify password. Please try again.';
			}
			isSubmitting = false;
		}
	}

	async function submitDeletionRequest() {
		if (!reauthToken) return;
		isSubmitting = true;
		errorMessage = null;
		try {
			deletionResponse = await userDeletionApi.requestDeletion(reauthToken);
			reauthToken = null;
			phase = 'pending';
		} catch (err) {
			reauthToken = null;
			if (isApiError(err) && err.status === 409 && err.code === 'owner_cannot_leave') {
				blockingOrgs = (err.details?.blocking_orgs as BlockingOrg[]) ?? [];
				phase = 'blocked';
			} else {
				errorMessage = 'Failed to submit deletion request. Please try again.';
				phase = 'error';
			}
		} finally {
			isSubmitting = false;
		}
	}
</script>

<section class="delete-account-panel" aria-label="Delete account" data-testid="delete-account-panel">
	<h2>Delete account</h2>
	<p class="delete-warning">
		Deleting your account is permanent. After a 30-day grace period, all your data will be
		irreversibly removed.
	</p>

	{#if phase === 'idle'}
		<button type="button" class="btn-danger" on:click={startConfirm}>
			Delete my account
		</button>

	{:else if phase === 'confirm'}
		<div class="confirm-banner" role="alert">
			<p>Are you sure? This action cannot be undone.</p>
			<div class="confirm-actions">
				<button type="button" class="btn-danger" on:click={openReauthModal}>
					Yes, delete my account
				</button>
				<button type="button" class="btn-secondary" on:click={cancelConfirm}>
					Cancel
				</button>
			</div>
		</div>

	{:else if phase === 'reauth'}
		<div class="reauth-modal" role="dialog" aria-modal="true" aria-labelledby="reauth-title">
			<h3 id="reauth-title">Confirm your password</h3>
			<p>Enter your current password to continue.</p>
			<form on:submit|preventDefault={submitReauth}>
				<label>
					Password
					<input
						type="password"
						bind:value={password}
						autocomplete="current-password"
						disabled={isSubmitting}
						required
					/>
				</label>
				{#if errorMessage}
					<div class="alert alert-error" role="alert">{errorMessage}</div>
				{/if}
				<div class="modal-actions">
					<button type="submit" class="btn-danger" disabled={isSubmitting || !password}>
						{isSubmitting ? 'Verifying…' : 'Confirm deletion'}
					</button>
					<button type="button" class="btn-secondary" on:click={cancelConfirm} disabled={isSubmitting}>
						Cancel
					</button>
				</div>
			</form>
		</div>

	{:else if phase === 'pending' && deletionResponse}
		<div class="deletion-scheduled" role="status">
			<p>
				Your account has been scheduled for deletion on
				<strong>{new Date(deletionResponse.finalizes_at).toLocaleDateString()}</strong>.
			</p>
			<p>
				You can cancel within the 30-day grace period:
				<a href="/account/data/cancel-deletion">Cancel deletion</a>.
			</p>
		</div>

	{:else if phase === 'blocked'}
		<div class="blocked-orgs" role="alert">
			<p>
				You cannot delete your account while you are the sole owner of the following
				organizations:
			</p>
			<ul class="blocking-orgs-list">
				{#each blockingOrgs as org}
					<li>
						<strong>{org.name}</strong>
						(<a href="/org/{org.slug}/members">Transfer ownership</a>)
					</li>
				{/each}
			</ul>
			<p>Transfer ownership or remove all members before requesting deletion.</p>
			<button type="button" class="btn-secondary" on:click={cancelConfirm}>
				Back
			</button>
		</div>

	{:else if phase === 'error'}
		<div class="alert alert-error" role="alert">
			{errorMessage ?? 'An unexpected error occurred. Please try again.'}
			<button type="button" class="btn-secondary" on:click={cancelConfirm}>Back</button>
		</div>
	{/if}
</section>
