package graphql

import "errors"

// Sentinel errors for GraphQL-specific failures.
var (
	// ErrMissingConfig is returned when a graphql request has no GraphQL config.
	ErrMissingConfig = errors.New("graphql config is required for protocol: graphql")

	// ErrEmptyQuery is returned when the GraphQL query string is empty.
	ErrEmptyQuery = errors.New("graphql query must not be empty")

	// ErrInvalidErrorHandling is returned for unrecognized error handling modes.
	ErrInvalidErrorHandling = errors.New("invalid graphql error_handling mode")
)
