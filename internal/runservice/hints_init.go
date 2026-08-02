package runservice

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("runservice",
		apierrors.RegisteredError{
			Name: "ErrCollectionInvalid",
			Err:  ErrCollectionInvalid,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryParse, Code: "RUNSERVICE_COLLECTION_INVALID", Hint: "Fix the collection file; run `curlew validate <file>` for line-level diagnostics."},
		},
	)
}
