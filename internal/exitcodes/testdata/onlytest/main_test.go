// Package onlytest has a run function that exists only inside a _test.go
// file. Reachable must treat this package as having no non-test sources —
// a code that exists only in a test is not one the shipped binary can
// return.
package onlytest

func run(mode int) int {
	if mode == 0 {
		return 0
	}
	return 1
}
