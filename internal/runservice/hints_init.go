package runservice

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("runservice",
		apierrors.RegisteredError{
			Name: "ErrCollectionInvalid",
			Err:  ErrCollectionInvalid,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryParse, Code: "RUNSERVICE_COLLECTION_INVALID", Hint: "Fix the collection file; run `apitest validate <file>` for line-level diagnostics."},
		},
	)
}
