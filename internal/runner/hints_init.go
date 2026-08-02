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
	)
}
