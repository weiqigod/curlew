package exitcodes

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage(
		"exitcodes",
		apierrors.RegisteredError{
			Name: "ErrRootNotFound",
			Err:  ErrRootNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "EXITCODES_ROOT_NOT_FOUND",
				Hint:     "Check the root function name passed to Reachable — it must be a top-level function declared directly in dir.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoSources",
			Err:  ErrNoSources,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "EXITCODES_NO_SOURCES",
				Hint:     "Check dir — it must contain at least one non-test .go file directly inside it (Reachable does not recurse into subdirectories).",
			},
		},
	)
}
