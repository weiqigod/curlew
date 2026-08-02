// Package plugin provides the external-process plugin loader for apitest.
// Plugins are standalone executables that communicate with apitest over
// JSON-RPC 2.0 on stdin/stdout (one JSON object per line, newline-delimited).
//
// Discovery: set APITEST_PLUGINS to a colon-separated (semicolon on Windows)
// list of plugin executables or directories. See docs/plugins.md.
package plugin

import "errors"

// Plugin describes a successfully-loaded external plugin.
type Plugin struct {
	Name    string
	Version string
	Hooks   []string
	Path    string
}

// LoadError carries a per-plugin load failure or warning.
// Fatal=true means CLI should exit with code 2; Fatal=false means a warning
// is printed to stderr but exit remains 0.
type LoadError struct {
	Path    string
	Message string
	Fatal   bool
}

// Sentinel errors returned via LoadError context or wrapped upstream.
var (
	// ErrHandshakeTimeout is returned when a plugin does not respond within the
	// handshake deadline.
	ErrHandshakeTimeout = errors.New("handshake timeout")

	// ErrDuplicateName is returned when two plugins report the same name.
	ErrDuplicateName = errors.New("duplicate plugin name")

	// ErrNotExecutable is returned when a plugin path lacks the execute bit.
	ErrNotExecutable = errors.New("not executable")

	// ErrHandshakeProtocol is returned when a plugin's handshake response is
	// malformed or missing required fields.
	ErrHandshakeProtocol = errors.New("handshake protocol error")
)

// knownHooks is the set of hook names understood by this version of apitest.
// Unknown hooks in a plugin's hello response are dropped with a warning.
var knownHooks = map[string]struct{}{
	"on_request":  {},
	"on_response": {},
	"on_result":   {},
}
