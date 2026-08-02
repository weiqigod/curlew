// Unit tests for gitlabIntegrationsApi — RED phase.
// These will fail until gitlab-integrations.ts is created.
// Refs: M16-016
import { describe, it, expect, vi } from 'vitest';
import { gitlabIntegrationsApi } from './gitlab-integrations';
import type { GitLabInstallation } from '$lib/types/gitlab-integrations';

function makeInstallation(overrides: Partial<GitLabInstallation> = {}): GitLabInstallation {
	return {
		id: 'aaaaaaaa-bbbb-cccc-dddd-eeeeeeeeeeee',
		project_id: 42,
		project_path: 'group/project',
		gitlab_base_url: 'https://gitlab.com',
		created_at: '2026-05-11T00:00:00Z',
		access_token_revoked_at: null,
		last_status_post_at: null,
		...overrides
	};
}

function mockFetch(body: unknown, status = 200): typeof fetch {
	return vi.fn().mockResolvedValue({
		ok: status >= 200 && status < 300,
		status,
		statusText: status === 200 ? 'OK' : 'Error',
		json: () => Promise.resolve(body)
	}) as unknown as typeof fetch;
}

const TEST_ORG_ID = 'org_00000000-0000-0000-0000-000000000001';

describe('gitlabIntegrationsApi', () => {
	it('list parses {installations: [...]} envelope and forwards org_id', async () => {
		const inst = makeInstallation({ project_path: 'g/p' });
		const fetchFn = mockFetch({ installations: [inst] });

		const result = await gitlabIntegrationsApi.list(TEST_ORG_ID, { fetch: fetchFn });

		expect(result).toHaveLength(1);
		expect(result[0].project_path).toBe('g/p');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/api/v1/integrations/gitlab');
		expect(call[0]).toContain('org_id=');
	});

	it('create POSTs body with org_id and returns dto without echoing the PAT', async () => {
		const created = makeInstallation({ project_path: 'group/new' });
		const fetchFn = mockFetch(created, 201);

		const result = await gitlabIntegrationsApi.create(
			TEST_ORG_ID,
			{ project_path: 'group/new', access_token: 'secret-token' },
			{ fetch: fetchFn }
		);

		expect(result.project_path).toBe('group/new');
		// The result DTO should not contain access_token
		expect(JSON.stringify(result)).not.toContain('secret-token');
		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('org_id=');
	});

	it('remove DELETEs the id-scoped path with org_id', async () => {
		const fetchFn = mockFetch(undefined, 204);

		await gitlabIntegrationsApi.remove(TEST_ORG_ID, 'inst-uuid-123', { fetch: fetchFn });

		const call = (fetchFn as ReturnType<typeof vi.fn>).mock.calls[0];
		expect(call[0]).toContain('/api/v1/integrations/gitlab/inst-uuid-123');
		expect(call[0]).toContain('org_id=');
		expect(call[1].method).toBe('DELETE');
	});

	it('list propagates ApiError on 402 with problem+json type', async () => {
		const fetchFn = mockFetch(
			{
				type: 'https://api.apitool.dev/errors/tier-ineligible',
				code: 'gitlab_integrations_tier_ineligible',
				title: 'Tier ineligible'
			},
			402
		);

		await expect(
			gitlabIntegrationsApi.list(TEST_ORG_ID, { fetch: fetchFn })
		).rejects.toMatchObject({
			status: 402,
			problemType: 'https://api.apitool.dev/errors/tier-ineligible'
		});
	});
});
