package distributed

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("runner/distributed",
		apierrors.RegisteredError{
			Name: "ErrWorkerJoinTimeout",
			Err:  ErrWorkerJoinTimeout,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "DISTRIBUTED_WORKER_JOIN_TIMEOUT", Hint: "No workers joined before the timeout. Ensure workers are running and point to the same coordinator URL."},
		},
	)
}
