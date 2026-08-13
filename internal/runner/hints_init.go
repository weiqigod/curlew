package runner

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("runner",
		apierrors.RegisteredError{
			Name: "ErrAuthProfileNotFound",
			Err:  ErrAuthProfileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAuth,
				Code:     "RUNNER_AUTH_PROFILE_NOT_FOUND",
				Hint:     "Define the auth profile named in the message, or fix the profile reference on the request.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoMatchingRequests",
			Err:  ErrNoMatchingRequests,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "ONLY_NO_MATCH",
				Hint:     "Pass --only <name> with a name that matches one of the available main requests. --only does not target setup or teardown items.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrLargeDataset",
			Err:  ErrLargeDataset,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATASET_TOO_LARGE",
				Hint:     "Pass --confirm-large-dataset to run every row, or set store_results: summary|failed_only to bound what the run retains.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrParallelAnalysis",
			Err:  ErrParallelAnalysis,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryConfig,
				Code:     "PARALLEL_ANALYSIS_REJECTED",
				Hint:     "The message names the requests involved. Break the cycle, give each concurrent item its own extract name, replace a dynamic extract key with a literal one, or drop a depends_on that reaches into another phase. Running without --parallel is not a fix: the collection says something the ordering cannot honour.",
			},
		},
	)
}
