package openapi

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("openapi",
		apierrors.RegisteredError{
			Name: "ErrSpecInvalid",
			Err:  ErrSpecInvalid,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "OPENAPI_SPEC_INVALID",
				Hint:     "Validate the OpenAPI spec file against a 3.x schema before importing.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrNoOperations",
			Err:  ErrNoOperations,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "OPENAPI_NO_OPERATIONS",
				Hint:     "The OpenAPI spec contains no path operations to import. Add at least one operation.",
			},
		},
	)
}
