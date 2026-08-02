package cel

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("cel",
		apierrors.RegisteredError{
			Name: "ErrCelParse",
			Err:  ErrCelParse,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "ERR_CEL_PARSE",
				Hint:     "Fix the CEL expression syntax. See docs/MANUAL.md § Expression Language (CEL).",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrCelType",
			Err:  ErrCelType,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "ERR_CEL_TYPE",
				Hint:     "Ensure the CEL expression's result type matches the field's expected type (bool for assertions and if:).",
			},
		},
	)
}
