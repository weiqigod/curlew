package parser

import "errors"

// Sentinel errors for collection parsing failures.
var (
	ErrFileNotFound          = errors.New("collection file not found")
	ErrInvalidYAML           = errors.New("invalid YAML syntax")
	ErrEmptyCollection       = errors.New("collection has no name")
	ErrUnsupportedMethod     = errors.New("unsupported HTTP method")
	ErrMissingRequiredField  = errors.New("missing required field")
	ErrExternalFileNotFound  = errors.New("external request file not found")
	ErrCircularFileReference = errors.New("circular file reference")
	ErrMutuallyExclusive     = errors.New("mutually exclusive fields")
	ErrUnsupportedProtocol   = errors.New("unsupported protocol")
	ErrInvalidFieldValue     = errors.New("invalid field value")

	// ErrCircularInclude is returned when an include: directive creates a cycle
	// (e.g. A includes B, B includes A).
	ErrCircularInclude = errors.New("circular include")
	// ErrIncludeNotFound is returned when an include: path cannot be found on
	// disk.
	ErrIncludeNotFound = errors.New("include file not found")

	// ErrDuplicateRequestName is returned when two or more main requests share
	// the same name within a collection (including items spliced in via
	// include:). Duplicate-name rejection happens at load time so every
	// subcommand (run, watch, validate) inherits it unconditionally.
	ErrDuplicateRequestName = errors.New("duplicate request name")

	// ErrSlugEmpty is returned when a request name slugifies to an empty string
	// (i.e., contains no ASCII letters or digits after Unicode NFKD normalization
	// and combining-mark removal). Callers surface this as a load-time error
	// matching the early-reject posture of duplicate-name detection.
	ErrSlugEmpty = errors.New("request name slugifies to empty")

	// ErrUnknownDependsOn is returned when a depends_on: list references a
	// request name that does not exist in any phase of the collection.
	ErrUnknownDependsOn = errors.New("depends_on references unknown request name")

	// ErrCelAndOperatorMutuallyExclusive is returned when a cel: assertion entry
	// also carries an operator key (e.g. eq:). A single entry may carry either a
	// cel: expression or an operator assertion, never both.
	ErrCelAndOperatorMutuallyExclusive = errors.New("cel: and operator assertion are mutually exclusive on the same entry")
)
