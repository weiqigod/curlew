# API Error Codes

This document is the stable reference for machine-readable error codes returned by the ApiTool Backend.
All error responses use RFC 7807 problem-details format (`Content-Type: application/problem+json`).

Every problem-details response includes a `code` extension field for machine-readable identification
and a `request_id` extension field for server-side correlation (also echoed as the `X-Request-Id` header).

## Format

```json
{
  "type":       "https://api.apitool.dev/errors/<slug>",
  "title":      "Human-readable title",
  "status":     401,
  "detail":     "Actionable message for the developer.",
  "code":       "AUTH_REFRESH_REUSED",
  "request_id": "0HMV6G1234:00000001"
}
```

---

## Auth — Refresh Token Errors

These codes are returned by `POST /api/v1/auth/refresh`.
Refs docs/SPECIFICATION.md:8243–8267.

| Code | HTTP Status | Type URL | Description | Recommended client action |
|------|-------------|----------|-------------|--------------------------|
| `AUTH_REFRESH_REUSED` | 401 | `.../errors/refresh-token-reused` | Refresh token has already been rotated. The entire token family has been revoked as a security precaution. The account owner receives an `account_security_alert` email. | Discard all stored tokens and redirect the user to `apitest login`. |
| `AUTH_REFRESH_EXPIRED` | 401 | `.../errors/refresh-token-expired` | Refresh token has exceeded its absolute 365-day maximum lifetime. | Discard all stored tokens and redirect the user to `apitest login`. |
| `AUTH_DEVICE_MISMATCH` | 401 | `.../errors/auth-device-mismatch` | The `device_id` in the request does not match the device the refresh token was issued to. | Discard the stored refresh token for this device and redirect to `apitest login`. |
| `AUTH_INVALID_REFRESH` | 401 | `.../errors/auth-invalid-refresh` | The refresh token is not recognised, has been revoked, or the request body is malformed (missing `refresh_token` or `device_id`). | Discard all stored tokens and redirect the user to `apitest login`. |

### Notes

- All four codes map to HTTP 401 Unauthorized. Clients MUST NOT retry the same token after receiving any of these codes — the token is no longer usable.
- `AUTH_REFRESH_REUSED` triggers a family-wide revocation. If the user is legitimately logged in on another device, they will also be logged out and need to re-authenticate.
- `AUTH_DEVICE_MISMATCH` does NOT rotate the presented token. The token remains valid for the device it was issued to.

---

---

## PR-Check Errors

These codes are returned by `POST /api/v1/pr-checks`.
Refs docs/SPECIFICATION.md:8537–8549.

The base type URL prefix is `https://api.apitool.dev/errors`.

| Code | HTTP Status | Type URL | Description | Recommended client action |
|------|-------------|----------|-------------|--------------------------|
| `PRCHECK_TOKEN_LEAK_DETECTED` | 400 | `.../errors/prcheck-token-leak-detected` | The request body contains a pattern matching a GitHub installation token (`ghs_…`). Sending tokens in the body is a security violation. | Remove the token from the payload and retry. Never include raw GitHub tokens in pr-check payloads. |
| `PRCHECK_INVALID_STATE` | 400 | `.../errors/prcheck-invalid-state` | The `state` field is not one of the six valid conclusion values (`success`, `failure`, `cancelled`, `timed_out`, `neutral`, `skipped`). | Correct the `state` field and retry. Note: `action_required` is not supported in M14. |
| `PRCHECK_NO_INSTALLATION` | 404 | `.../errors/prcheck-no-installation` | No GitHub App installation exists for this organisation. | Install the ApiTool GitHub App from the dashboard before posting PR checks. |
| `PRCHECK_REPO_NOT_COVERED` | 403 | `.../errors/prcheck-repo-not-covered` | The `repo` is not covered by the installation's repository selection. | Grant access to the repository in the GitHub App settings, then retry after the next reconciliation cycle (≤ 24 h). |
| `PRCHECK_INSTALLATION_SUSPENDED` | 423 | `.../errors/prcheck-installation-suspended` | The GitHub App installation is suspended. | Resume the installation from GitHub App settings. |
| `PRCHECK_INSTALLATION_DELETED` | 410 | `.../errors/prcheck-installation-deleted` | The GitHub App installation was deleted. The check cannot be posted. | Reinstall the ApiTool GitHub App from the dashboard. |
| `PRCHECK_GITHUB_RATE_LIMITED` | 429 | `.../errors/prcheck-github-rate-limited` | GitHub's API rate-limited the outbound POST. The row is marked `queued` and will be retried automatically. | No immediate action required. The CLI can poll for status if needed. |
| `PRCHECK_GITHUB_UNAVAILABLE` | 502 | `.../errors/prcheck-github-unavailable` | GitHub returned a 5xx error. The row is marked `queued` and will be retried automatically. | No immediate action required. If the problem persists, check [githubstatus.com](https://www.githubstatus.com). |
| `PRCHECK_PERMANENT_FAILURE` | 500 | `.../errors/prcheck-permanent-failure` | Posting the check run failed permanently after exhausting retries. | Contact support with the `request_id` from the response. |

### Notes

- `PRCHECK_TOKEN_LEAK_DETECTED` is enforced at the raw-body level before JSON deserialisation. No partial processing occurs.
- `PRCHECK_GITHUB_RATE_LIMITED` and `PRCHECK_GITHUB_UNAVAILABLE` both result in the pr-check row being persisted with `status=queued`. A future drain worker (M14+) will retry these rows. The CLI receives the error code so it can inform the user without blocking the run.
- The `pr_check_id` field in the response body identifies the persisted row, which can be used for status polling in a future API (M14+).

---

## Auth — Password Reset / Email Verification Errors

These codes are returned by `POST /api/v1/auth/password-reset/request`,
`POST /api/v1/auth/password-reset/confirm`, `POST /api/v1/auth/email-verification/resend`,
`POST /api/v1/auth/email-verification/confirm`, and any endpoint protected by
the `RequireVerifiedEmail` filter (e.g. `POST /api/v1/subscriptions/checkout`,
`POST /api/v1/invitations/accept`).
Refs docs/SPECIFICATION.md:8462-8580.

The base type URL prefix is `https://apitool.dev/errors`.

| Code | HTTP Status | Type URL | Description | Recommended client action |
|------|-------------|----------|-------------|--------------------------|
| `PASSWORD_RESET_TOKEN_INVALID` | 400 | `.../errors/password-reset-token-invalid` | The password-reset token is not recognised, has already been consumed, is expired, or has been revoked. | Discard the token and ask the user to request a new password-reset email. |
| `PASSWORD_TOO_WEAK` | 422 | `.../errors/password-too-weak` | The proposed new password scores below 3 on the zxcvbn scale. | Show the user the strength feedback and ask them to choose a stronger password. |
| `EMAIL_VERIFICATION_TOKEN_INVALID` | 400 | `.../errors/email-verification-token-invalid` | The email-verification token is not recognised, has already been consumed, is expired, or has been revoked. | Discard the token and ask the user to request a new verification email via the resend endpoint. |
| `EMAIL_NOT_VERIFIED` | 403 | `.../errors/email-not-verified` | The authenticated user has not yet verified their email address. Access to this endpoint requires a verified email. | Redirect the user to the email-verification flow. The resend endpoint can issue a new verification email if needed. |

### Notes

- `POST /api/v1/auth/password-reset/request` always returns 200 OK regardless of whether the
  email address exists or the per-email rate limit has been reached — this prevents user enumeration.
- `POST /api/v1/auth/email-verification/resend` follows the same silent-200 policy on
  per-email rate limit.
- On successful password-reset confirm, **all refresh-token families** for the user are
  revoked. The user will be logged out on all devices and must re-authenticate.
- The `EMAIL_NOT_VERIFIED` filter applies to `POST /api/v1/subscriptions/checkout` and
  `POST /api/v1/invitations/accept` only. Other endpoints are not gated.

---

## Trials (M16-007)

Returned by `POST /api/v1/trials/{feature}`.

| Code | Status | Meaning | CLI exit |
|------|--------|---------|---------|
| `TRIAL_ALREADY_CONSUMED` | 409 | A trial row already exists for this (user, feature) pair. The `previous_grant` extension carries `kind`, `granted_at`, and `expires_at`. | 5 |
| `TRIAL_FEATURE_UNKNOWN` | 404 | The `{feature}` slug is not in the registered trialable-feature list. | 6 |
| `TRIAL_INVALID_REQUEST` | 400 | The request body is missing `refresh_token` or `device_id`. | 1 |

---

## Vault Configuration Errors (M16-017)

These codes are returned by `GET/PUT/DELETE /api/v1/organizations/{orgId}/vault-config`.
Refs docs/SPECIFICATION.md :5654-5715.

| Code | HTTP Status | Type URL | Description | Recommended client action |
|------|-------------|----------|-------------|--------------------------|
| `VAULT_CONFIG_SUSPICIOUS_VALUE` | 422 | `.../errors/vault-template-suspicious-value` | One or more template fields match the literal-secret heuristic (≥16 alphanumeric chars under a sensitive key name that is not a recognized provider coordinate). The `offending_paths` extension lists the dot-separated JSON paths. | Replace the literal values with provider coordinate references (ARN, vault:// URI, etc.) and retry. In development mode (APITOOL__VAULTCONFIG__VALIDATORMODE=warn) the save succeeds with a warning instead. |

### Example Response (422)

```json
{
  "type": "https://api.apitool.dev/errors/vault-template-suspicious-value",
  "title": "Template contains likely-secret values",
  "status": 422,
  "detail": "One or more template fields match the literal-secret heuristic...",
  "instance": "/api/v1/organizations/org_abc123/vault-config",
  "offending_paths": ["team_secrets.vault_configs.prod.password"]
}
```

---

## Future Sections

Additional error-code families will be appended here as M14 slices land:

- Stripe / Billing errors (M14-005+)
- OIDC / SSO errors
