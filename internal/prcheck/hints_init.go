package prcheck

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("prcheck",
		apierrors.RegisteredError{
			Name: "ErrResultsFileMissing",
			Err:  ErrResultsFileMissing,
			Hint: apierrors.ClassifiedHint{Category: apierrors.CategoryInput, Code: "PRCHECK_RESULTS_MISSING", Hint: "Pass --results <file>, produced by `curlew run --format json --report <file>`."},
		},
	)
}
