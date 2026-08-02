package openapi

import "errors"

// Sentinel errors for well-known OpenAPI import failure modes.
var (
	// ErrSpecInvalid is returned when the spec file cannot be loaded or fails
	// OpenAPI validation.
	ErrSpecInvalid = errors.New("openapi spec is invalid")

	// ErrNoOperations is returned when the spec contains no operations.
	ErrNoOperations = errors.New("openapi spec contains no operations")
)
