package httpbody

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("httpbody",
		apierrors.RegisteredError{
			Name: "ErrBodyFileNotFound",
			Err:  ErrBodyFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_BODY_FILE_NOT_FOUND",
				Hint:     "The file referenced by body_file: or body_binary_file: does not exist. Check the path relative to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrBodyFileTooLarge",
			Err:  ErrBodyFileTooLarge,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "PARSE_BODY_FILE_TOO_LARGE",
				Hint:     "Reduce the file size below the configured body-file limit, or raise the limit via config.",
			},
		},
	)
}
