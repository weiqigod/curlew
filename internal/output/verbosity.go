package output

// Verbosity controls how much detail is printed per request.
type Verbosity int

const (
	VerbosityQuiet   Verbosity = -1 // -q: summary line only
	VerbosityDefault Verbosity = 0  // no flag: name, status, assertions
	VerbosityVerbose Verbosity = 1  // -v: + request/response headers
	VerbosityDebug   Verbosity = 2  // -vv: + full body dump
)

// String returns the string representation of the verbosity level.
func (v Verbosity) String() string {
	switch v {
	case VerbosityQuiet:
		return "quiet"
	case VerbosityDefault:
		return "default"
	case VerbosityVerbose:
		return "verbose"
	case VerbosityDebug:
		return "debug"
	default:
		return "unknown"
	}
}
