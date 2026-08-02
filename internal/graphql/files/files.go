package files

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fragmentDeclPattern matches "fragment <Name> on <Type>" at the start of a
// fragment definition. Only the first match is used to record the fragment's
// declared name; files containing multiple fragment declarations are not
// recommended and only the first name participates in dependency resolution.
var fragmentDeclPattern = regexp.MustCompile(`(?m)^\s*fragment\s+(\w+)\s+on\s+\w+`)

// fragmentSpreadPattern matches "...<Name>" fragment spread references. The
// optional whitespace between the ellipsis and the name lets us distinguish
// spread syntax (`...UserFields`) from inline fragment syntax (`... on Type`);
// inline-fragment matches that capture the literal keyword `on` are filtered
// out in [extractFragmentSpreads]. This is a lightweight lexical approximation
// and does not attempt to parse GraphQL comments or string literals.
var fragmentSpreadPattern = regexp.MustCompile(`\.\.\.\s*([A-Za-z_]\w*)`)

// LoadQueryInput holds the inputs for loading a GraphQL query from external
// files. All paths are resolved against BaseDir unless absolute.
type LoadQueryInput struct {
	// BaseDir is the directory that relative QueryFile / Fragments paths are
	// resolved against.
	BaseDir string
	// InlineQuery is the graphql.query value from the YAML (may be empty).
	InlineQuery string
	// QueryFile is the graphql.query_file value (may be empty).
	QueryFile string
	// Fragments are the graphql.fragments paths (may be empty).
	Fragments []string
}

// LoadQueryResult holds the concatenated query and the absolute paths of every
// file that was read. FilePaths is suitable for appending to a collection's
// ExternalFiles list so watch mode can rerun on edits.
type LoadQueryResult struct {
	Query     string
	FilePaths []string
}

// loadedFragment is an internal record for one fragment file read from disk.
type loadedFragment struct {
	name string
	body string
	deps []string
	path string // original path from the input, used in error messages
}

// LoadQuery resolves graphql.query_file and graphql.fragments into a single
// concatenated query string. Fragments are topologically ordered so that
// dependencies appear before dependents, then the base query follows.
//
// Rules:
//   - If InlineQuery and QueryFile are both set, returns ErrQueryMutuallyExclusive.
//   - If neither is set, returns ErrMissingQuery.
//   - If QueryFile is set, reads the file and uses it as the base query.
//   - An empty base query (after loading) returns ErrMissingQuery.
//   - Fragments are read, parsed for their declared name and spread references,
//     ordered by dependency, and prepended to the query (separated by "\n\n").
//   - Duplicate fragment names across files return ErrDuplicateFragment.
//   - Circular fragment dependencies return ErrFragmentCycle.
//   - Relative paths resolve against BaseDir; absolute paths are honored as-is.
//   - Variable placeholders ({{name}}) in the loaded content are preserved for
//     the caller to interpolate later.
//
// A fragment file that declares multiple fragments is accepted, but only the
// first declaration's name is recorded for dependency resolution. Spread
// references (`...Name`) inside the file are still walked across all its
// declarations, so secondary fragments in the same file cannot be targeted by
// other files - keep one fragment per file to avoid surprises.
func LoadQuery(input LoadQueryInput) (*LoadQueryResult, error) {
	if input.InlineQuery != "" && input.QueryFile != "" {
		return nil, ErrQueryMutuallyExclusive
	}

	filePaths := make([]string, 0, 1+len(input.Fragments))
	var baseQuery string

	if input.QueryFile != "" {
		path := resolvePath(input.BaseDir, input.QueryFile)
		data, err := os.ReadFile(path)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %s", ErrQueryFileNotFound, input.QueryFile)
			}
			return nil, fmt.Errorf("reading graphql query_file %q: %w", input.QueryFile, err)
		}
		baseQuery = string(data)
		filePaths = append(filePaths, path)
	} else {
		baseQuery = input.InlineQuery
	}

	if strings.TrimSpace(baseQuery) == "" {
		return nil, ErrMissingQuery
	}

	if len(input.Fragments) == 0 {
		return &LoadQueryResult{Query: baseQuery, FilePaths: filePaths}, nil
	}

	loaded := make([]loadedFragment, 0, len(input.Fragments))
	byName := make(map[string]int, len(input.Fragments))
	for _, p := range input.Fragments {
		abs := resolvePath(input.BaseDir, p)
		data, err := os.ReadFile(abs)
		if err != nil {
			if os.IsNotExist(err) {
				return nil, fmt.Errorf("%w: %s", ErrFragmentFileNotFound, p)
			}
			return nil, fmt.Errorf("reading fragment %q: %w", p, err)
		}
		body := string(data)
		m := fragmentDeclPattern.FindStringSubmatch(body)
		if len(m) < 2 {
			return nil, fmt.Errorf("fragment file %q does not contain a 'fragment <Name> on <Type>' declaration", p)
		}
		name := m[1]
		if existing, ok := byName[name]; ok {
			return nil, fmt.Errorf("%w: %q declared in %q and %q", ErrDuplicateFragment, name, loaded[existing].path, p)
		}
		deps := extractFragmentSpreads(body, name)
		loaded = append(loaded, loadedFragment{name: name, body: body, deps: deps, path: p})
		byName[name] = len(loaded) - 1
		filePaths = append(filePaths, abs)
	}

	order, err := topoSortFragments(loaded, byName)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	// Pre-size roughly: sum of fragment bodies + query + separators.
	approx := len(baseQuery)
	for i := range loaded {
		approx += len(loaded[i].body) + 2
	}
	b.Grow(approx)
	for _, idx := range order {
		b.WriteString(strings.TrimRight(loaded[idx].body, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString(baseQuery)

	return &LoadQueryResult{Query: b.String(), FilePaths: filePaths}, nil
}

// extractFragmentSpreads returns the fragment names referenced via "..." inside
// body, excluding self-references to selfName. The order of the result matches
// the order of first appearance in the source.
func extractFragmentSpreads(body, selfName string) []string {
	matches := fragmentSpreadPattern.FindAllStringSubmatch(body, -1)
	if len(matches) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(matches))
	deps := make([]string, 0, len(matches))
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := m[1]
		if name == selfName {
			continue
		}
		// Skip inline-fragment syntax: "... on Type" captures the literal
		// keyword "on", which is never a valid fragment name.
		if name == "on" {
			continue
		}
		if _, ok := seen[name]; ok {
			continue
		}
		seen[name] = struct{}{}
		deps = append(deps, name)
	}
	return deps
}

// topoSortFragments performs a DFS-based topological sort on the loaded
// fragments. It returns indices in dependency-first order. Cycles return
// ErrFragmentCycle with the offending chain in the error message. References
// to fragments that are not in the loaded set (for example, fragments declared
// inline in the query file) are ignored and impose no ordering constraint.
func topoSortFragments(loaded []loadedFragment, byName map[string]int) ([]int, error) {
	const (
		white = 0 // unvisited
		gray  = 1 // on current DFS stack
		black = 2 // finished
	)
	color := make([]int, len(loaded))
	order := make([]int, 0, len(loaded))

	var visit func(idx int, stack []string) error
	visit = func(idx int, stack []string) error {
		switch color[idx] {
		case gray:
			chain := append(append([]string(nil), stack...), loaded[idx].name)
			return fmt.Errorf("%w: %s", ErrFragmentCycle, strings.Join(chain, " -> "))
		case black:
			return nil
		}
		color[idx] = gray
		stack = append(stack, loaded[idx].name)
		for _, dep := range loaded[idx].deps {
			depIdx, ok := byName[dep]
			if !ok {
				continue
			}
			if err := visit(depIdx, stack); err != nil {
				return err
			}
		}
		color[idx] = black
		order = append(order, idx)
		return nil
	}

	for i := range loaded {
		if color[i] == white {
			if err := visit(i, nil); err != nil {
				return nil, err
			}
		}
	}
	return order, nil
}

// resolvePath joins a relative path with baseDir, or returns absolute paths
// unchanged.
func resolvePath(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}
