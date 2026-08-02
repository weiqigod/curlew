package backend

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("backend",
		apierrors.RegisteredError{
			Name: "ErrNotProblem",
			Err:  ErrNotProblem,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "BACKEND_NOT_PROBLEM_JSON", Hint: "The backend response was not application/problem+json. Check backend service health."},
		},
		apierrors.RegisteredError{
			Name: "ErrNetworkFailure",
			Err:  ErrNetworkFailure,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryNetwork, Code: "BACKEND_NETWORK_FAILURE", Hint: "The backend is unreachable. Check your network connection."},
		},
		apierrors.RegisteredError{
			Name: "ErrServerError",
			Err:  ErrServerError,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInternal, Code: "BACKEND_SERVER_ERROR", Hint: "The backend returned a 5xx error. Retry later or check service status."},
		},
		apierrors.RegisteredError{
			Name: "ErrEncryptedFileTampered",
			Err:  ErrEncryptedFileTampered,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_ENCRYPTED_FILE_TAMPERED", Hint: "The stored refresh token file is corrupted or the encryption key changed. Re-authenticate via apitest login."},
		},
		apierrors.RegisteredError{
			Name: "ErrTokenNotFound",
			Err:  ErrTokenNotFound,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_TOKEN_NOT_FOUND", Hint: "No refresh token is stored. Authenticate via apitest login."},
		},
		apierrors.RegisteredError{
			Name: "ErrAuthorizationPending",
			Err:  ErrAuthorizationPending,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_DEVICE_AUTHORIZATION_PENDING", Hint: "The device code is awaiting authorization. Keep polling."},
		},
		apierrors.RegisteredError{
			Name: "ErrSlowDown",
			Err:  ErrSlowDown,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_DEVICE_SLOW_DOWN", Hint: "Polling too fast. Increase the polling interval by 5 seconds per RFC 8628 §3.5."},
		},
		apierrors.RegisteredError{
			Name: "ErrExpiredToken",
			Err:  ErrExpiredToken,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_DEVICE_EXPIRED_TOKEN", Hint: "The device code has expired. Run apitest login again to restart the flow."},
		},
		apierrors.RegisteredError{
			Name: "ErrAccessDenied",
			Err:  ErrAccessDenied,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_DEVICE_ACCESS_DENIED", Hint: "The authorization was denied. Run apitest login to try again."},
		},
		apierrors.RegisteredError{
			Name: "ErrRefreshExpired",
			Err:  ErrRefreshExpired,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_REFRESH_EXPIRED", Hint: "The refresh token has expired. Run apitest login to re-authenticate."},
		},
		apierrors.RegisteredError{
			Name: "ErrRefreshReused",
			Err:  ErrRefreshReused,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_REFRESH_REUSED", Hint: "Refresh token reuse detected — the token family has been revoked. Run apitest login to re-authenticate."},
		},
		apierrors.RegisteredError{
			Name: "ErrDeviceMismatch",
			Err:  ErrDeviceMismatch,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_DEVICE_MISMATCH", Hint: "Device mismatch. Run apitest login to re-register this device."},
		},
		apierrors.RegisteredError{
			Name: "ErrTrialAlreadyConsumed",
			Err:  ErrTrialAlreadyConsumed,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_TRIAL_ALREADY_CONSUMED", Hint: "A trial for this feature has already been used. Subscribe to continue using this feature."},
		},
		apierrors.RegisteredError{
			Name: "ErrTrialFeatureUnknown",
			Err:  ErrTrialFeatureUnknown,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "BACKEND_TRIAL_FEATURE_UNKNOWN", Hint: "The requested feature slug is not recognized. Check the feature name and try again."},
		},
		apierrors.RegisteredError{
			Name: "ErrTeamVaultNotFound",
			Err:  ErrTeamVaultNotFound,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryConfig, Code: "BACKEND_TEAM_VAULT_NOT_FOUND", Hint: "The organization has no shared vault configuration. Configure one via the team admin dashboard."},
		},
		apierrors.RegisteredError{
			Name: "ErrOrgIDRequired",
			Err:  ErrOrgIDRequired,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryConfig, Code: "BACKEND_ORG_ID_REQUIRED", Hint: "An organization ID is required for this operation. Ensure your license JWT contains a valid org_id claim (re-authenticate if missing)."},
		},
	)
}
