package prcheck

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("prcheck",
		apierrors.RegisteredError{
			Name: "ErrBackendURLMissing",
			Err:  ErrBackendURLMissing,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "PRCHECK_BACKEND_URL_MISSING", Hint: "Configure the backend URL via APITEST_BACKEND_URL or project config."},
		},
		apierrors.RegisteredError{
			Name: "ErrUnauthorized",
			Err:  ErrUnauthorized,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "PRCHECK_UNAUTHORIZED", Hint: "Refresh APITEST_BACKEND_TOKEN."},
		},
		apierrors.RegisteredError{
			Name: "ErrNetworkFailure",
			Err:  ErrNetworkFailure,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryNetwork, Code: "PRCHECK_NETWORK_FAILURE", Hint: "Backend unreachable after retries. Check network and backend status."},
		},
	)
}
