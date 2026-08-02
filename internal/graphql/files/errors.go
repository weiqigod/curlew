// Package files resolves external GraphQL query and fragment files for
// collection parsing. It is intentionally dependency-free (beyond the standard
// library) so that it can be imported from internal/parser without creating
// an import cycle with internal/graphql.
package files

import "errors"

// Sentinel errors returned by LoadQuery.
var (
	// ErrMissingQuery is returned when neither an inline query nor a query_file
	// was supplied (or the query_file exists but is empty).
	ErrMissingQuery = errors.New("graphql query is required")

	// ErrQueryFileNotFound is returned when graphql.query_file points to a
	// file that does not exist on disk.
	ErrQueryFileNotFound = errors.New("graphql query_file not found")

	// ErrFragmentFileNotFound is returned when an entry in graphql.fragments
	// points to a file that does not exist on disk.
	ErrFragmentFileNotFound = errors.New("graphql fragment file not found")

	// ErrFragmentCycle is returned when loaded fragments form a circular
	// dependency via fragment spread references.
	ErrFragmentCycle = errors.New("graphql fragment circular dependency")

	// ErrQueryMutuallyExclusive is returned when both graphql.query and
	// graphql.query_file are specified on the same request.
	ErrQueryMutuallyExclusive = errors.New("graphql query and query_file are mutually exclusive")

	// ErrDuplicateFragment is returned when two loaded fragment files declare
	// the same fragment name.
	ErrDuplicateFragment = errors.New("graphql duplicate fragment name")
)
