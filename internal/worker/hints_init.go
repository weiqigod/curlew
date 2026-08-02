package worker

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("worker",
		apierrors.RegisteredError{
			Name: "ErrCoordinatorURLMissing",
			Err:  ErrCoordinatorURLMissing,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "WORKER_COORDINATOR_URL_MISSING", Hint: "Set CURLEW_COORDINATOR_URL or pass --coordinator-url."},
		},
		apierrors.RegisteredError{
			Name: "ErrTokenMissing",
			Err:  ErrTokenMissing,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "WORKER_TOKEN_MISSING", Hint: "Set CURLEW_BACKEND_TOKEN or pass --token."},
		},
		apierrors.RegisteredError{
			Name: "ErrUnauthorized",
			Err:  ErrUnauthorized,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryAuth, Code: "WORKER_UNAUTHORIZED", Hint: "The backend token is missing or expired. Refresh it."},
		},
		apierrors.RegisteredError{
			Name: "ErrNetworkExhausted",
			Err:  ErrNetworkExhausted,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryNetwork, Code: "WORKER_NETWORK_EXHAUSTED", Hint: "The coordinator was unreachable after retries. Check network and coordinator status."},
		},
	)
}
