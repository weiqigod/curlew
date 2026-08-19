package fuzzseed

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage(
		"fuzzseed",
		apierrors.RegisteredError{
			Name: "ErrNoSeeds",
			Err:  ErrNoSeeds,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInternal,
				Code:     "FUZZSEED_NO_SEEDS",
				Hint:     "A fixture source directory matched zero files. Check that the source list in fuzzseed.go still matches real directories on disk.",
			},
		},
	)
}
