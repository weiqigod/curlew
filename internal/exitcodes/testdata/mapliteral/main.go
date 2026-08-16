// Package mapliteral mirrors cmd/curlew/discovery_run.go's exitCodeSeverity
// map: a composite literal whose keys and values include 6. Reachable must
// not collect 6, because a map literal is not a return statement.
package mapliteral

// severity mimics the dead exitCodeSeverity map left by the removed
// licensing system: `6: 6, // feature gate` appears as both a key and a
// value here, exactly as it does in the real map.
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
