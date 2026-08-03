import { render, screen, waitFor } from '@testing-library/svelte';
import userEvent from '@testing-library/user-event';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { ApiError } from '$lib/types/api-error';
import type { SsoConfigView } from '$lib/types/sso';
import { makeOrg } from '$lib/testing/fixtures';
import Page from './+page.svelte';

const invalidateAll = vi.fn();
vi.mock('$app/navigation', () => ({ invalidateAll: () => invalidateAll() }));

const upsertSaml = vi.fn();
const upsertOidc = vi.fn();
vi.mock('$lib/api/sso', () => ({
	ssoApi: {
		upsertSaml: (...a: unknown[]) => upsertSaml(...a),
		upsertOidc: (...a: unknown[]) => upsertOidc(...a)
	}
}));

const EMPTY_CONFIG: SsoConfigView = {
	sso_enabled: false,
	sso_provider: null,
	saml_config: null,
	oidc_config: null
};

const SAML_ENABLED: SsoConfigView = {
	sso_enabled: true,
	sso_provider: 'saml',
	saml_config: {
		idp_metadata_url: 'https://idp.example.com/metadata',
		acs_url: 'https://sp.example.com/acs',
		entity_id: 'https://sp.example.com',
		idp_sso_url: 'https://idp.example.com/sso'
	},
	oidc_config: null
} as SsoConfigView;

/** Mirrors the shape returned by this route's server `load`. */
function makeData(overrides: Record<string, unknown> = {}) {
	return {
		organizations: [],
		org: makeOrg({ id: 'org_test', tier: 'enterprise' }),
		config: EMPTY_CONFIG,
		error: null,
		...overrides
	};
}

/** Fills the three required SAML fields. */
async function fillSaml(user: ReturnType<typeof userEvent.setup>) {
	await user.type(screen.getByTestId('saml-idp-metadata-url'), 'https://idp.example.com/metadata');
	await user.type(screen.getByTestId('saml-acs-url'), 'https://sp.example.com/acs');
	await user.type(screen.getByTestId('saml-entity-id'), 'https://sp.example.com');
}

beforeEach(() => {
	vi.clearAllMocks();
});

describe('sso settings page', () => {
	it('opens on the SAML tab with the OIDC panel hidden and no provider badge', () => {
		render(Page, { props: { data: makeData() } });

		expect(screen.getByTestId('sso-heading')).toBeVisible();
		expect(screen.getByTestId('tab-saml')).toBeVisible();
		expect(screen.getByTestId('tab-oidc')).toBeVisible();
		expect(screen.getByTestId('tabpanel-saml')).toBeVisible();
		expect(screen.getByTestId('tabpanel-oidc')).not.toBeVisible();
		expect(screen.getByTestId('tab-saml')).toHaveAttribute('aria-selected', 'true');
		expect(screen.queryByTestId('sso-provider-badge')).not.toBeInTheDocument();
	});

	it('opens on the OIDC tab when OIDC is the configured provider', () => {
		render(
			Page,
			{
				props: {
					data: makeData({
						config: { ...EMPTY_CONFIG, sso_enabled: true, sso_provider: 'oidc' }
					})
				}
			}
		);

		expect(screen.getByTestId('tabpanel-oidc')).toBeVisible();
		expect(screen.getByTestId('tabpanel-saml')).not.toBeVisible();
		expect(screen.getByTestId('sso-provider-badge')).toHaveTextContent('OIDC enabled');
	});

	it('switches panels when a tab is clicked', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('tab-oidc'));

		expect(screen.getByTestId('tabpanel-oidc')).toBeVisible();
		expect(screen.getByTestId('tabpanel-saml')).not.toBeVisible();
		expect(screen.getByTestId('tab-oidc')).toHaveAttribute('aria-selected', 'true');
	});

	it('submits the SAML form and shows the SSO enabled toast', async () => {
		const user = userEvent.setup();
		upsertSaml.mockResolvedValue({});

		render(Page, { props: { data: makeData() } });

		await fillSaml(user);
		await user.click(screen.getByTestId('saml-submit'));

		await waitFor(() => expect(upsertSaml).toHaveBeenCalledTimes(1));
		expect(upsertSaml).toHaveBeenCalledWith('org_test', {
			idp_metadata_url: 'https://idp.example.com/metadata',
			acs_url: 'https://sp.example.com/acs',
			entity_id: 'https://sp.example.com'
		});
		expect(await screen.findByTestId('toast-message')).toHaveTextContent('SSO enabled');
		expect(invalidateAll).toHaveBeenCalledTimes(1);
	});

	it('includes the optional SAML fields only when filled', async () => {
		const user = userEvent.setup();
		upsertSaml.mockResolvedValue({});

		render(Page, { props: { data: makeData() } });

		await fillSaml(user);
		await user.type(screen.getByTestId('saml-idp-sso-url'), 'https://idp.example.com/sso');
		await user.click(screen.getByTestId('saml-submit'));

		await waitFor(() => expect(upsertSaml).toHaveBeenCalledTimes(1));
		const body = (upsertSaml.mock.calls[0] as [string, Record<string, unknown>])[1];
		expect(body.idp_sso_url).toBe('https://idp.example.com/sso');
		expect(body).not.toHaveProperty('idp_cert_pem');
	});

	it('submits the OIDC form and shows the SSO enabled toast', async () => {
		const user = userEvent.setup();
		upsertOidc.mockResolvedValue({});

		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('tab-oidc'));
		await user.type(screen.getByTestId('oidc-issuer-url'), 'https://login.example.com');
		await user.type(screen.getByTestId('oidc-client-id'), 'my_client_id');
		await user.type(screen.getByTestId('oidc-client-secret'), 'super_secret');
		await user.click(screen.getByTestId('oidc-submit'));

		await waitFor(() => expect(upsertOidc).toHaveBeenCalledTimes(1));
		expect(upsertOidc).toHaveBeenCalledWith('org_test', {
			issuer_url: 'https://login.example.com',
			client_id: 'my_client_id',
			client_secret: 'super_secret',
			scopes: 'openid email profile'
		});
		expect(await screen.findByTestId('toast-message')).toHaveTextContent('SSO enabled');
	});

	it('pins an invalid_sso_config 400 to the offending field', async () => {
		const user = userEvent.setup();
		upsertSaml.mockRejectedValue(
			new ApiError('invalid_sso_config', 'Invalid metadata URL', 400, 'idp_metadata_url')
		);

		render(Page, { props: { data: makeData() } });

		await fillSaml(user);
		await user.click(screen.getByTestId('saml-submit'));

		const alert = await screen.findByRole('alert');
		expect(alert).toHaveTextContent('Invalid metadata URL');
		// Field-scoped errors do not fall through to the form-level slot.
		expect(screen.queryByTestId('saml-server-error')).not.toBeInTheDocument();
		expect(invalidateAll).not.toHaveBeenCalled();
	});

	it('shows a field-less server error at form level', async () => {
		const user = userEvent.setup();
		upsertSaml.mockRejectedValue(new ApiError('server_error', 'upstream exploded', 500));

		render(Page, { props: { data: makeData() } });

		await fillSaml(user);
		await user.click(screen.getByTestId('saml-submit'));

		expect(await screen.findByTestId('saml-server-error')).toHaveTextContent('upstream exploded');
	});

	it('blocks submission client-side when a required SAML field is missing', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('saml-submit'));

		expect(await screen.findByRole('alert')).toHaveTextContent('IdP Metadata URL is required.');
		expect(upsertSaml).not.toHaveBeenCalled();
	});

	it('blocks OIDC submission when the client secret is missing', async () => {
		const user = userEvent.setup();
		render(Page, { props: { data: makeData() } });

		await user.click(screen.getByTestId('tab-oidc'));
		await user.type(screen.getByTestId('oidc-issuer-url'), 'https://login.example.com');
		await user.type(screen.getByTestId('oidc-client-id'), 'my_client_id');
		await user.click(screen.getByTestId('oidc-submit'));

		expect(await screen.findByRole('alert')).toHaveTextContent('Client Secret is required.');
		expect(upsertOidc).not.toHaveBeenCalled();
	});

	it('pre-populates the SAML form from the stored config', () => {
		render(Page, { props: { data: makeData({ config: SAML_ENABLED }) } });

		expect(screen.getByTestId('saml-idp-metadata-url')).toHaveValue(
			'https://idp.example.com/metadata'
		);
		expect(screen.getByTestId('saml-acs-url')).toHaveValue('https://sp.example.com/acs');
		expect(screen.getByTestId('saml-entity-id')).toHaveValue('https://sp.example.com');
		// Secrets are never echoed back into the form.
		expect(screen.getByTestId('saml-idp-cert-pem')).toHaveValue('');
	});

	it('offers a Test SSO login link only once SSO is enabled', () => {
		const { rerender } = render(Page, { props: { data: makeData() } });
		expect(screen.queryByTestId('sso-test-login')).not.toBeInTheDocument();

		rerender({ data: makeData({ config: SAML_ENABLED }) });

		const link = screen.getByTestId('sso-test-login');
		expect(link).toHaveAttribute('href', '/api/v1/sso/saml/org_test/login');
		expect(link).toHaveAttribute('target', '_blank');
		expect(link.getAttribute('rel')).toContain('noopener');
	});

	it('shows an error state instead of the tabs when load failed', () => {
		render(Page, { props: { data: makeData({ error: 'Failed to load SSO configuration.' }) } });

		expect(screen.getByTestId('error-state')).toHaveTextContent(
			'Failed to load SSO configuration.'
		);
		expect(screen.queryByTestId('tab-saml')).not.toBeInTheDocument();
	});
});
