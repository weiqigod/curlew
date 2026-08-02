package files

import apierrors "github.com/weiqigod/curlew/internal/errors"

func init() {
	apierrors.RegisterPackage("graphql/files",
		apierrors.RegisteredError{
			Name: "ErrMissingQuery",
			Err:  ErrMissingQuery,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_MISSING_QUERY",
				Hint:     "Provide either query: or query_file: under the graphql: block.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrQueryFileNotFound",
			Err:  ErrQueryFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_QUERY_FILE_NOT_FOUND",
				Hint:     "Verify the query_file: path resolves relative to the collection file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrFragmentFileNotFound",
			Err:  ErrFragmentFileNotFound,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_FRAGMENT_FILE_NOT_FOUND",
				Hint:     "Verify the fragment file path. Paths resolve relative to the including query file.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrFragmentCycle",
			Err:  ErrFragmentCycle,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_FRAGMENT_CYCLE",
				Hint:     "Break the circular fragment dependency. A fragment must not transitively include itself.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrQueryMutuallyExclusive",
			Err:  ErrQueryMutuallyExclusive,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_QUERY_MUTUALLY_EXCLUSIVE",
				Hint:     "Use either query: or query_file:, not both.",
			},
		},
		apierrors.RegisteredError{
			Name: "ErrDuplicateFragment",
			Err:  ErrDuplicateFragment,
			Hint: apierrors.ClassifiedHint{
				Category: apierrors.CategoryParse,
				Code:     "GRAPHQL_DUPLICATE_FRAGMENT",
				Hint:     "Fragment names must be unique. Rename or remove the duplicate.",
			},
		},
	)
}
