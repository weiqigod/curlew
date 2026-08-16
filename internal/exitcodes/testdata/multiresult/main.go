// Package multiresult exercises two shapes real cmd/curlew source uses that
// the other fixtures do not reach: a full-value forward, where a return
// statement is a single call supplying every one of the enclosing function's
// results at once, and a named, grouped result list.
package multiresult

func run(mode int) int {
	if mode == 0 {
		return 0
	}
	code, _ := forward()
	return code
}

// forward has two results and forwards both from a single call — the
// "return f()" full-value-forward shape examineReturn exists to detect.
// cmd/curlew's own runCmdWithWriters is `code, _ := runCmdInner(...); return
// code` (the multi-value assignment run's own body uses), and runCmdInner's
// internal call chain includes exactly this full-forward shape one level
// deeper.
func forward() (int, int) {
	return delegate()
}

// delegate uses a named, grouped result list ("(code, spare int)") to
// exercise resultCount's other counting branch: most of cmd/curlew's
// exit-code functions return a single unnamed int, but the counting rule
// must hold for a named group too.
func delegate() (code, spare int) {
	return 9, 0
}

// noop has no results at all, exercising resultCount's nil-Results branch.
// It is never reached by any walk rooted at run; parseDir calls resultCount
// on every top-level function it parses, regardless of whether the walk
// ever visits it.
func noop() {}
