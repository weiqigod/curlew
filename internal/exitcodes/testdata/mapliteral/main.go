// Package mapliteral mirrors the shape cmd/curlew/discovery_run.go's
// exitCodeSeverity map once had: a composite literal whose keys and values
// include 6. Reachable must not collect 6, because a map literal is not a
// return statement.
package mapliteral

// severity mimics exitCodeSeverity's former shape, before M26-002 deleted its
// dead `6: 6, // feature gate` entry left by the removed licensing system:
// the same key and value appear here, entirely apart from whether
// discovery_run.go currently has such an entry.
var severity = map[int]int{
	0: 0, // success
	6: 6, // feature gate
}

func run(ok bool) int {
	if ok {
		return 0
	}
	return 1
}
