package graphql

import apierrors "github.com/peterlindqvist/apitest/internal/errors"

func init() {
	apierrors.RegisterPackage("graphql",
		apierrors.RegisteredError{
			Name: "ErrMissingConfig",
			Err:  ErrMissingConfig,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_MISSING_CONFIG",
				Hint:     "Add a graphql: block with at least query: or query_file: when protocol is graphql.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrEmptyQuery",
			Err:  ErrEmptyQuery,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_EMPTY_QUERY",
				Hint:     "Provide a non-empty GraphQL query under query: or query_file:.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrInvalidErrorHandling",
			Err:  ErrInvalidErrorHandling,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_INVALID_ERROR_HANDLING",
				Hint:     "Set error_handling: to strict, warn, or ignore.",
			},
		},
	)
}
