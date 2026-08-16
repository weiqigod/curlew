// Package unresolvable exercises followCall's two silent-skip paths: a call
// to a package-qualified function (not a bare identifier, so it can never be
// a call to a function this package parsed) and a call to a bare identifier
// that resolves to something other than a locally declared function (here,
// the builtin len).
package unresolvable

import "os"

// run's only literal exit code is its own `return 0`; neither os.Getpid()
// nor len(items) may contribute a value, since neither is a locally
// declared function this package can walk into.
func run(items []int) int {
	switch {
	case len(items) == 0:
		return 0
	case len(items) == 1:
		return os.Getpid()
	default:
		return len(items)
	}
}
