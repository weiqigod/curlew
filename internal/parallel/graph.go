package parallel

// RequestNode represents a request in the dependency graph.
type RequestNode struct {
	Index          int
	Name           string
	ProducedVars   map[string]bool // variables extracted by this request
	ReferencedVars map[string]bool // variables referenced by this request (excluding pre-exec and dynamic)
	Dependencies   map[int]bool    // indices of requests this depends on
}

// Edge represents a dependency between two requests.
type Edge struct {
	From      int      // index of producer request
	To        int      // index of consumer request
	Variables []string // variable names creating this dependency
	Explicit  bool     // true when declared via depends_on rather than inferred from variables
}

// DependencyGraph holds the full analysis result.
type DependencyGraph struct {
	Nodes    []RequestNode
	Edges    []Edge
	Waves    [][]int // execution wave groupings
	IsValid  bool
	Errors   []string
	Warnings []string
}

// ImpactEntry describes how many requests were skipped due to a specific failed request.
type ImpactEntry struct {
	FailedIndex  int
	FailedName   string
	SkippedCount int
}
