package datadriven

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("datadriven",
		apierrors.RegisteredError{
			Name: "ErrFileNotFound",
			Err:  ErrFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_FILE_NOT_FOUND",
				Hint:     "The data file referenced by data: does not exist. Check the path relative to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEmptyDataFile",
			Err:  ErrEmptyDataFile,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_FILE_EMPTY",
				Hint:     "Add at least one row (after the header) to the data file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnsupportedFormat",
			Err:  ErrUnsupportedFormat,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_UNSUPPORTED_FORMAT",
				Hint:     "Use a supported data file format: csv, tsv, or json.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrMalformedData",
			Err:  ErrMalformedData,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_MALFORMED",
				Hint:     "Fix the malformed row described in the message. All rows must match the header's column count.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrUnsupportedFilter",
			Err:  ErrUnsupportedFilter,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_UNSUPPORTED_FILTER",
				Hint:     "Use a supported type conversion filter (e.g. int, float, bool).",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidFilter",
			Err:  ErrInvalidFilter,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryInput,
				Code:     "DATA_INVALID_FILTER",
				Hint:     "Fix the filter expression syntax. Expected format: <column> | <filter>.",
			},
		},
	)
}
