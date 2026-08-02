package output

import (
	"errors"
	"fmt"
	"strings"
)

// ErrUnknownFormat is returned when an output.format value is not one of the
// supported formats.
var ErrUnknownFormat = errors.New("unknown output format")

// SupportedFormats lists the valid output.format values (matches the CLI
// --format enum).
var SupportedFormats = []string{"terminal", "json", "tap", "junit", "html", "markdown"}

// Config is the typed form of a YAML `output:` block declared at either the
// project (curlew.yaml) or collection level. All fields are optional; zero
// values mean "not declared" and inherit from lower-precedence scopes.
type Config struct {
	Format    string `yaml:"format,omitempty"`    // terminal|json|tap|junit|html|markdown
	Report    string `yaml:"report,omitempty"`    // file path
	Events    string `yaml:"events,omitempty"`    // file path
	Verbosity string `yaml:"verbosity,omitempty"` // quiet|normal|verbose|debug
}

// Validate returns a non-nil error when Config declares an unsupported format.
// Unset fields are permitted. Path emptiness (report/events) is enforced by the
// JSON Schema (minLength: 1) before Validate is reached in the run path.
func (c *Config) Validate() error {
	if c == nil {
		return nil
	}
	if c.Format != "" && !IsSupportedFormat(c.Format) {
		return fmt.Errorf("%w %q (supported: %s)", ErrUnknownFormat, c.Format, FormatList())
	}
	return nil
}

// ParseVerbosity converts a YAML verbosity string to output.Verbosity.
// Empty string returns VerbosityDefault with ok=false so callers can treat it
// as "not declared" (same as a missing key).
func ParseVerbosity(s string) (v Verbosity, ok bool, err error) {
	switch s {
	case "":
		return VerbosityDefault, false, nil
	case "quiet":
		return VerbosityQuiet, true, nil
	case "normal":
		return VerbosityDefault, true, nil
	case "verbose":
		return VerbosityVerbose, true, nil
	case "debug":
		return VerbosityDebug, true, nil
	default:
		return VerbosityDefault, false, fmt.Errorf("unknown output verbosity %q (supported: quiet, normal, verbose, debug)", s)
	}
}

// IsSupportedFormat reports whether s is one of the known output formats.
func IsSupportedFormat(s string) bool {
	for _, f := range SupportedFormats {
		if s == f {
			return true
		}
	}
	return false
}

// FormatList returns a comma-separated list of supported output format names
// suitable for use in error messages.
func FormatList() string {
	return strings.Join(SupportedFormats, ", ")
}
