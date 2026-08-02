package output

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("output",
		apierrors.RegisteredError{
			Name: "ErrUnknownFormat",
			Err:  ErrUnknownFormat,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "OUTPUT_UNKNOWN_FORMAT",
				Hint:     "Set output.format to one of: terminal, json, tap, junit, html",
			},
		},
	)
}
