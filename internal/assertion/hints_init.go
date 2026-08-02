package assertion

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("assertion",
		apierrors.RegisteredError{
			Name: "ErrAssertionFailed",
			Err:  ErrAssertionFailed,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "ASSERTION_FAILED",
				Hint:     "Inspect the assertion.result events for this request and adjust either the assertion or the request to make them agree.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrSchemaFileNotFound",
			Err:  ErrSchemaFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "ASSERTION_SCHEMA_FILE_NOT_FOUND",
				Hint:     "Verify the schema file path is correct and the file exists. Paths resolve relative to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrSchemaInvalid",
			Err:  ErrSchemaInvalid,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryAssertion,
				Code:     "ASSERTION_SCHEMA_INVALID",
				Hint:     "The JSON Schema file is malformed. Validate it against json-schema.org/draft-07 or similar.",
			},
		},
	)
}
