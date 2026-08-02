package httpbody

import "errors"

var (
	// ErrBodyFileNotFound is returned when the file referenced by body_file or
	// body_binary_file does not exist.
	ErrBodyFileNotFound = errors.New("body file not found")

	// ErrBodyFileTooLarge is returned when the referenced file exceeds the
	// configured size limit.
	ErrBodyFileTooLarge = errors.New("body file too large")
)
