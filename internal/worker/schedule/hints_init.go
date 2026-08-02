package schedule

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("worker/schedule",
		apierrors.RegisteredError{
			Name: "ErrNoRunAvailable",
			Err:  ErrNoRunAvailable,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "SCHEDULE_NO_RUN_AVAILABLE",
				Hint:     "No scheduled runs are currently pending. The worker will retry on the next poll interval.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrClaimReaped",
			Err:  ErrClaimReaped,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "SCHEDULE_CLAIM_REAPED",
				Hint:     "The schedule run claim was reaped by the server (heartbeat timeout). The run will be re-queued automatically.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrAlreadyCompleted",
			Err:  ErrAlreadyCompleted,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "SCHEDULE_ALREADY_COMPLETED",
				Hint:     "The schedule run result was already submitted (idempotent guard). No action required.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnsupportedCollectionRef",
			Err:  ErrUnsupportedCollectionRef,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "SCHEDULE_UNSUPPORTED_COLLECTION_REF",
				Hint:     "The collection_ref scheme is not supported. Use 'file:./path/to/collection.yaml'. git: refs are deferred to a future release.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrPostExhausted",
			Err:  ErrPostExhausted,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryNetwork,
				Code:     "SCHEDULE_POST_EXHAUSTED",
				Hint:     "Result posting failed after retries. The payload has been queued locally under ~/.config/curlew/pending-uploads/. It will be retried on the next poll cycle.",
			},
		},
	)
}
