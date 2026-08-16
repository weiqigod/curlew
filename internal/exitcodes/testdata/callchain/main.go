// Package callchain exercises a helper called in return position, the
// pattern Strategy B (call graph, no ident resolution) already handles.
package callchain

func run(mode int) int {
	switch mode {
	case 0:
		return 0
	case 1:
		return 1
	default:
		return helper()
	}
}

func helper() int {
	return 3
}
