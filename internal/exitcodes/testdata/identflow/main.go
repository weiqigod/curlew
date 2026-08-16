// Package identflow exercises the pattern Strategy B misses and Strategy C
// exists to add: a code produced by a local function call, threaded through
// a local variable, and returned by name rather than by direct call. This is
// the shape of cmd/curlew/main.go's `code := runErrorExitCode(varErr);
// return code, summary`.
package identflow

func run(mode int) int {
	if mode == 0 {
		return 0
	}
	code := classify(mode)
	return code
}

func classify(mode int) int {
	if mode == 1 {
		return 2
	}
	return 5
}
