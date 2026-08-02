package jsonpath

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("jsonpath",
		apierrors.RegisteredError{
			Name: "ErrNotFound",
			Err:  ErrNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "JSONPATH_NOT_FOUND",
				Hint:     "The JSONPath did not match. Verify the path against the actual response body.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidPath",
			Err:  ErrInvalidPath,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "JSONPATH_INVALID_PATH",
				Hint:     "Fix the JSONPath syntax. Supported forms are $, $.field, $.arr[0], $..descendant, $.arr[*].",
			},
		},
	)
}
