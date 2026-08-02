// Package report aggregates per-request samples from a perf run and emits
// the results as a summary line, a JSON time-series document, or a self-
// contained HTML page with a Chart.js latency-vs-time chart.
package report

import (
	"errors"
	"fmt"
	"path/filepath"
	"strings"
)

// Format identifies the report output format selected by the user.
type Format int

const (
	// FormatStdout is the default — prints a summary line only; no file is written.
	FormatStdout Format = iota
	// FormatJSON writes a JSON time-series + metrics document to the given path.
	FormatJSON
	// FormatHTML writes a self-contained HTML page (Chart.js CDN) to the given path.
	FormatHTML
)

// ErrUnsupportedFormat is returned by DetectFormat for extensions other than
// .json, .html, or the literal string "stdout".
var ErrUnsupportedFormat = errors.New("unsupported report format")

// DetectFormat parses --output and returns the format plus the cleaned file
// path (empty for FormatStdout). An empty flag value defaults to stdout.
func DetectFormat(flag string) (Format, string, error) {
	if flag == "" || flag == "stdout" {
		return FormatStdout, "", nil
	}
	ext := strings.ToLower(filepath.Ext(flag))
	switch ext {
	case ".json":
		return FormatJSON, flag, nil
	case ".html":
		return FormatHTML, flag, nil
	default:
		return FormatStdout, "", fmt.Errorf("%w %s", ErrUnsupportedFormat, ext)
	}
}
