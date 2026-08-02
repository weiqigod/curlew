package output

import (
	"io"
	"os"
)

// ANSI SGR escape codes.
const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiRed      = "\033[31m"
	ansiGreen    = "\033[32m"
	ansiYellow   = "\033[33m"
	ansiCyan     = "\033[36m"
	ansiGray     = "\033[90m"
	ansiBoldCyan = "\033[1;36m"
)

// IsTerminal reports whether w is connected to a terminal device.
// Returns false for *bytes.Buffer, pipes, and redirected files.
func IsTerminal(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// colorize wraps s with an ANSI code and reset when enabled.
// Returns s unchanged when enabled is false or s is empty.
func colorize(s, code string, enabled bool) string {
	if !enabled || s == "" {
		return s
	}
	return code + s + ansiReset
}
