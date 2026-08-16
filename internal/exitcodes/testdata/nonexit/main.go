// Package nonexit mirrors cmd/curlew/main.go's htmlResultStatusCode and
// countWaveResults: an int-returning local helper whose result is used for
// something other than the exit code. Reachable must not collect its return
// values, because they never flow into a return statement of the walked
// function.
package nonexit

func run(ok bool) int {
	code := statusCode()
	if code > 0 {
		return 1
	}
	return 0
}

// statusCode returns an HTTP-style status, not an exit code. Its 404 must
// never appear in Reachable's output for run.
func statusCode() int {
	return 404
}
