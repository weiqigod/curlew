# Implementation Plan: M2-030

## Overview

Extend the GraphQL protocol adapter so collections can reference external `.graphql` query files via `graphql.query_file:` and concatenate reusable fragments via `graphql.fragments:`. Fragments are loaded, topologically ordered by their `...FragmentName` references, concatenated onto the query, and any circular dependency is reported as a parse error. Loaded file contents are placed into `GraphQLConfig.Query`, which means existing `{{var}}` interpolation works unchanged.

## Task Details
- **ID:** M2-030
- **Title:** GraphQL external query files and fragment support
- **Phase:** M2: GraphQL
- **Priority:** 4
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-029 | GraphQL protocol adapter | done |

## Key Architectural Decisions

1. **Loading happens in the parser**, not the runner. After `yaml.Unmarshal` and before `validateRequests`, `ParseFile` walks every request's `GraphQLConfig` and, if `QueryFile` or `Fragments` are set, reads the files, concatenates them into `GraphQLConfig.Query`, and appends absolute paths to `col.ExternalFiles`. Rationale: Variable interpolation, feature gating, and the rest of the runner pipeline already consume `GraphQLConfig.Query` unchanged. This gives us interpolation of `{{var}}` in loaded files "for free" (behavior 6) and preserves the existing runner tests.

2. **Mechanics live in `internal/graphql/files.go`**, not the parser. The parser just orchestrates; the graphql package owns the domain logic (read, extract fragment names, resolve dependency order, detect cycles, concatenate). This keeps the parser package thin and the graphql package self-contained for unit testing.

3. **Fragment ordering is topological**. Each loaded fragment is parsed with a regex to find its own `fragment <Name> on <Type>` declaration and any `...<OtherName>` spread references. The resulting directed graph is sorted so dependencies appear before consumers in the final concatenated output. Cycles (a -> b -> a) are rejected with `ErrGraphQLFragmentCycle`.

4. **Fragment name discovery is by file, not by scanning directories**. Only the fragments explicitly listed in `graphql.fragments:` are loaded; the topological sort only considers references between those loaded fragments. References to unlisted fragments are passed through (they may be defined in the query file itself) - they are not treated as errors.

5. **Paths are resolved relative to the collection file directory**, mirroring `resolveExternalReferences` in `internal/parser/external.go`. Absolute paths are honored as-is.

6. **`query_file` and `query` are mutually exclusive.** Specifying both is a parse error (`ErrMutuallyExclusive`). At least one must be set when `protocol: graphql`.

7. **Variable interpolation applies to loaded content** because it flows through `GraphQLConfig.Query` which `requtil.InterpolateRequest` already processes (line 61 of requtil.go). No change to requtil needed.

## Implementation Steps

### Step 1: Add `QueryFile` and `Fragments` fields to `GraphQLConfig`

**Rationale:** Smallest blast radius - a pure data struct change. Touches one file and no tests break because the fields are optional and zero-valued by default.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `QueryFile` and `Fragments` yaml fields to `GraphQLConfig` |

#### Current Code
```go
// GraphQLConfig holds the GraphQL-specific request configuration.
type GraphQLConfig struct {
	Query         string         `yaml:"query"`
	Variables     map[string]any `yaml:"variables,omitempty"`
	ErrorHandling string         `yaml:"error_handling,omitempty"` // "fail" (default), "warn"
}
```

#### New Code
```go
// GraphQLConfig holds the GraphQL-specific request configuration.
type GraphQLConfig struct {
	Query         string         `yaml:"query,omitempty"`
	QueryFile     string         `yaml:"query_file,omitempty"`     // path to an external .graphql file (mutually exclusive with query)
	Fragments     []string       `yaml:"fragments,omitempty"`      // paths to .graphql fragment files, concatenated onto the query
	Variables     map[string]any `yaml:"variables,omitempty"`
	ErrorHandling string         `yaml:"error_handling,omitempty"` // "fail" (default), "warn"
}
```

#### Tests to Write FIRST (RED phase)
No tests written at this step - this is a struct shape change. The first failing tests arrive in Step 3 (parser) and Step 4 (graphql.LoadQuery).

#### Impact on Existing Tests
- None. `Query` becomes `omitempty`, but all existing YAML files set `query:` explicitly and all existing tests construct `GraphQLConfig` with `Query` set. The YAML tag change from `yaml:"query"` to `yaml:"query,omitempty"` only affects serialization (which the parser does not perform).

---

### Step 2: Add sentinel errors for the new failure modes

**Rationale:** Errors must exist before the code that returns them; tests in later steps assert via `errors.Is`. This is still a small, isolated change.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/graphql/errors.go` | modify | Add three new sentinel errors |

#### Current Code
```go
var (
	// ErrMissingConfig is returned when a graphql request has no GraphQL config.
	ErrMissingConfig = errors.New("graphql config is required for protocol: graphql")

	// ErrEmptyQuery is returned when the GraphQL query string is empty.
	ErrEmptyQuery = errors.New("graphql query must not be empty")

	// ErrInvalidErrorHandling is returned for unrecognized error handling modes.
	ErrInvalidErrorHandling = errors.New("invalid graphql error_handling mode")
)
```

#### New Code
```go
var (
	// ErrMissingConfig is returned when a graphql request has no GraphQL config.
	ErrMissingConfig = errors.New("graphql config is required for protocol: graphql")

	// ErrEmptyQuery is returned when the GraphQL query string is empty.
	ErrEmptyQuery = errors.New("graphql query must not be empty")

	// ErrInvalidErrorHandling is returned for unrecognized error handling modes.
	ErrInvalidErrorHandling = errors.New("invalid graphql error_handling mode")

	// ErrQueryFileNotFound is returned when graphql.query_file points to a missing file.
	ErrQueryFileNotFound = errors.New("graphql query_file not found")

	// ErrFragmentFileNotFound is returned when an entry in graphql.fragments points to a missing file.
	ErrFragmentFileNotFound = errors.New("graphql fragment file not found")

	// ErrFragmentCycle is returned when loaded fragments form a circular dependency.
	ErrFragmentCycle = errors.New("graphql fragment circular dependency")

	// ErrQueryMutuallyExclusive is returned when both graphql.query and graphql.query_file are set.
	ErrQueryMutuallyExclusive = errors.New("graphql query and query_file are mutually exclusive")
)
```

#### Impact on Existing Tests
- None.

---

### Step 3: Implement fragment loading + topological concatenation in the graphql package

**Rationale:** Pure, self-contained logic with no external dependencies beyond `os` and `regexp`. Fully unit-testable with table-driven tests. Must be done before parser integration so we can TDD the domain logic in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/graphql/files.go` | create | `LoadQuery` function + fragment parsing, ordering, cycle detection |
| `internal/graphql/files_test.go` | create | Table-driven unit tests |

#### New Code

```go
// Package graphql - new file files.go

package graphql

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// fragmentDeclPattern matches "fragment <Name> on <Type>" at the start of a fragment definition.
var fragmentDeclPattern = regexp.MustCompile(`(?m)^\s*fragment\s+(\w+)\s+on\s+\w+`)

// fragmentSpreadPattern matches "...<Name>" fragment spread references.
var fragmentSpreadPattern = regexp.MustCompile(`\.\.\.(\w+)`)

// LoadQueryInput holds the inputs for loading a GraphQL query from external files.
type LoadQueryInput struct {
	// BaseDir is the directory that relative QueryFile / Fragments paths are resolved against.
	BaseDir string
	// InlineQuery is the graphql.query value from the YAML (may be empty).
	InlineQuery string
	// QueryFile is the graphql.query_file value (may be empty).
	QueryFile string
	// Fragments are the graphql.fragments paths (may be empty).
	Fragments []string
}

// LoadQueryResult holds the concatenated query and the absolute paths of every
// file that was read (for watch-mode rerun tracking).
type LoadQueryResult struct {
	Query     string
	FilePaths []string
}

// LoadQuery resolves graphql.query_file and graphql.fragments into a single
// concatenated query string. Fragments are topologically ordered so that
// dependencies appear before dependents. Circular dependencies return
// ErrFragmentCycle.
//
// Rules:
//   - If InlineQuery and QueryFile are both set, returns ErrQueryMutuallyExclusive.
//   - If neither is set, returns ErrEmptyQuery.
//   - If QueryFile is set, reads the file and uses it as the base query.
//   - Fragments are read, parsed for their declared name and spread references,
//     ordered by dependency, and appended to the query (separated by "\n\n").
//   - Relative paths resolve against BaseDir.
func LoadQuery(input LoadQueryInput) (*LoadQueryResult, error) {
	if input.InlineQuery != "" && input.QueryFile != "" {
		return nil, ErrQueryMutuallyExclusive
	}

	var filePaths []string
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

	if baseQuery == "" {
		return nil, ErrEmptyQuery
	}

	if len(input.Fragments) == 0 {
		return &LoadQueryResult{Query: baseQuery, FilePaths: filePaths}, nil
	}

	type fragment struct {
		name     string
		body     string
		deps     []string
		origPath string
	}

	// Read every fragment file and parse its declared name + spread refs.
	loaded := make([]fragment, 0, len(input.Fragments))
	byName := make(map[string]int) // name -> index in loaded
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
		deps := extractFragmentSpreads(body, name)
		loaded = append(loaded, fragment{name: name, body: body, deps: deps, origPath: p})
		byName[name] = len(loaded) - 1
		filePaths = append(filePaths, abs)
	}

	// Topological sort over the loaded fragments only. Unknown spread targets
	// (e.g. fragments defined inside the query file) are ignored.
	order, err := topoSort(loaded, byName)
	if err != nil {
		return nil, err
	}

	var b strings.Builder
	for _, idx := range order {
		b.WriteString(strings.TrimRight(loaded[idx].body, "\n"))
		b.WriteString("\n\n")
	}
	b.WriteString(baseQuery)

	return &LoadQueryResult{Query: b.String(), FilePaths: filePaths}, nil
}

// extractFragmentSpreads returns the fragment names referenced via "..." inside
// body, excluding self-references.
func extractFragmentSpreads(body, selfName string) []string {
	matches := fragmentSpreadPattern.FindAllStringSubmatch(body, -1)
	seen := make(map[string]bool)
	var deps []string
	for _, m := range matches {
		if len(m) < 2 {
			continue
		}
		name := m[1]
		if name == selfName || seen[name] {
			continue
		}
		seen[name] = true
		deps = append(deps, name)
	}
	return deps
}

// topoSort performs a DFS-based topological sort and returns indices in
// dependency-first order. Cycles return ErrFragmentCycle.
func topoSort[F any](nodes []F, byName map[string]int) ([]int, error) {
	// Function-local type assertion: nodes are fragment structs exposed via byName.
	// We need deps; since Go generics can't access fields, the caller in LoadQuery
	// will use the non-generic variant below.
	return nil, nil // placeholder - replaced by the non-generic implementation below
}
```

**Note on generics:** Go generics cannot access fields of a type parameter. The actual implementation will use a non-generic helper that takes the `[]fragment` and `byName` directly. The above generic stub is removed; the real code is:

```go
// topoSort performs a DFS-based topological sort on the loaded fragments.
// Returns indices in dependency-first order. Cycles return ErrFragmentCycle.
func topoSortFragments(loaded []struct {
	name     string
	body     string
	deps     []string
	origPath string
}, byName map[string]int) ([]int, error) {
```

To avoid the anonymous struct leakage, the cleanest form is to declare the `fragment` type at package scope inside `files.go`:

```go
type loadedFragment struct {
	name     string
	body     string
	deps     []string
	origPath string
}

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
			return fmt.Errorf("%w: %s", ErrFragmentCycle, strings.Join(append(stack, loaded[idx].name), " -> "))
		case black:
			return nil
		}
		color[idx] = gray
		stack = append(stack, loaded[idx].name)
		for _, dep := range loaded[idx].deps {
			depIdx, ok := byName[dep]
			if !ok {
				// Reference to a fragment not in the loaded set (e.g. defined in
				// the query file itself). Skip it - not an ordering constraint.
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

// resolvePath joins a relative path with baseDir, or returns absolute paths unchanged.
func resolvePath(baseDir, p string) string {
	if filepath.IsAbs(p) {
		return p
	}
	return filepath.Join(baseDir, p)
}
```

#### Tests to Write FIRST (RED phase)

Create `internal/graphql/files_test.go`:

```go
package graphql

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadQuery(t *testing.T) {
	dir := t.TempDir()

	// Setup files
	writeFile := func(name, contents string) string {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(contents), 0o644); err != nil {
			t.Fatal(err)
		}
		return p
	}

	writeFile("queries/get_user.graphql", "query GetUser($id: ID!) { user(id: $id) { ...UserFields } }")
	writeFile("fragments/user_fields.graphql", "fragment UserFields on User { id name email }")
	writeFile("fragments/post_fields.graphql", "fragment PostFields on Post { id title author { ...UserFields } }")
	writeFile("fragments/cycle_a.graphql", "fragment A on T { ...B }")
	writeFile("fragments/cycle_b.graphql", "fragment B on T { ...A }")
	writeFile("fragments/no_decl.graphql", "{ not a fragment }")
	writeFile("fragments/self_ref.graphql", "fragment Self on T { id ...Self }")
	writeFile("queries/with_var.graphql", "query { user(id: \"{{user_id}}\") { id } }")

	tests := []struct {
		name       string
		input      LoadQueryInput
		wantErr    error
		wantSubstr []string // strings that must appear in result.Query
		wantOrder  []string // optional: substrings in the order they should appear
		wantFiles  int
	}{
		{
			name: "inline query no files",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ me { id } }",
			},
			wantSubstr: []string{"{ me { id } }"},
			wantFiles:  0,
		},
		{
			name: "query_file loads from disk",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
			},
			wantSubstr: []string{"query GetUser", "UserFields"},
			wantFiles:  1,
		},
		{
			name: "query_file with single fragment concatenates",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
				Fragments: []string{"fragments/user_fields.graphql"},
			},
			wantSubstr: []string{"fragment UserFields", "query GetUser"},
			wantOrder:  []string{"fragment UserFields", "query GetUser"},
			wantFiles:  2,
		},
		{
			name: "fragment dependency ordered before dependent",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/get_user.graphql",
				Fragments: []string{
					"fragments/post_fields.graphql", // depends on UserFields
					"fragments/user_fields.graphql",
				},
			},
			// UserFields must appear before PostFields in the concatenated output
			wantOrder: []string{"fragment UserFields", "fragment PostFields", "query GetUser"},
			wantFiles: 3,
		},
		{
			name: "circular fragment dependency returns ErrFragmentCycle",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...A } }",
				Fragments: []string{
					"fragments/cycle_a.graphql",
					"fragments/cycle_b.graphql",
				},
			},
			wantErr: ErrFragmentCycle,
		},
		{
			name: "self-referencing fragment is not a cycle",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x { ...Self } }",
				Fragments:   []string{"fragments/self_ref.graphql"},
			},
			wantSubstr: []string{"fragment Self"},
			wantFiles:  1,
		},
		{
			name: "missing query_file returns ErrQueryFileNotFound",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/missing.graphql",
			},
			wantErr: ErrQueryFileNotFound,
		},
		{
			name: "missing fragment file returns ErrFragmentFileNotFound",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x }",
				Fragments:   []string{"fragments/missing.graphql"},
			},
			wantErr: ErrFragmentFileNotFound,
		},
		{
			name: "both query and query_file returns ErrQueryMutuallyExclusive",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ a }",
				QueryFile:   "queries/get_user.graphql",
			},
			wantErr: ErrQueryMutuallyExclusive,
		},
		{
			name: "no query and no query_file returns ErrEmptyQuery",
			input: LoadQueryInput{
				BaseDir: dir,
			},
			wantErr: ErrEmptyQuery,
		},
		{
			name: "fragment without declaration returns error",
			input: LoadQueryInput{
				BaseDir:     dir,
				InlineQuery: "{ x }",
				Fragments:   []string{"fragments/no_decl.graphql"},
			},
			wantErr: nil, // will use raw strings.Contains check for the error message
		},
		{
			name: "variable placeholders preserved in loaded content",
			input: LoadQueryInput{
				BaseDir:   dir,
				QueryFile: "queries/with_var.graphql",
			},
			wantSubstr: []string{"{{user_id}}"},
			wantFiles:  1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := LoadQuery(tt.input)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Fatalf("LoadQuery() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if tt.name == "fragment without declaration returns error" {
				if err == nil || !strings.Contains(err.Error(), "does not contain a 'fragment") {
					t.Fatalf("expected fragment declaration error, got %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("LoadQuery() unexpected error: %v", err)
			}
			for _, s := range tt.wantSubstr {
				if !strings.Contains(got.Query, s) {
					t.Errorf("result.Query missing substring %q\nGot:\n%s", s, got.Query)
				}
			}
			if len(tt.wantOrder) > 0 {
				prev := -1
				for _, s := range tt.wantOrder {
					idx := strings.Index(got.Query, s)
					if idx < 0 {
						t.Errorf("result.Query missing expected substring %q\nGot:\n%s", s, got.Query)
						continue
					}
					if idx < prev {
						t.Errorf("substring %q appears before previous; wanted order %v\nGot:\n%s", s, tt.wantOrder, got.Query)
					}
					prev = idx
				}
			}
			if tt.wantFiles > 0 && len(got.FilePaths) != tt.wantFiles {
				t.Errorf("FilePaths count = %d, want %d (%v)", len(got.FilePaths), tt.wantFiles, got.FilePaths)
			}
		})
	}
}

func TestExtractFragmentSpreads(t *testing.T) {
	tests := []struct {
		name, body, self string
		want             []string
	}{
		{"no spreads", "fragment X on T { id }", "X", nil},
		{"one spread", "fragment X on T { ...Y }", "X", []string{"Y"}},
		{"multiple spreads deduped", "fragment X on T { ...Y ...Z ...Y }", "X", []string{"Y", "Z"}},
		{"self reference excluded", "fragment X on T { ...X ...Y }", "X", []string{"Y"}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractFragmentSpreads(tt.body, tt.self)
			if len(got) != len(tt.want) {
				t.Fatalf("got %v, want %v", got, tt.want)
			}
			for i, g := range got {
				if g != tt.want[i] {
					t.Errorf("got[%d] = %q, want %q", i, g, tt.want[i])
				}
			}
		})
	}
}
```

#### Impact on Existing Tests
- None. New file, new test function, no changes to existing exported symbols.

---

### Step 4: Wire external file resolution into the parser

**Rationale:** Once `LoadQuery` is green, integrate it into `ParseFile` so that every GraphQL request has its `Query` field populated by the time validation and the runner see it. This is the step where user-visible behavior starts working end-to-end.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/parser.go` | modify | After `yaml.Unmarshal` and external request resolution, walk every request and call `graphql.LoadQuery` when `query_file` or `fragments` are set |
| `internal/parser/parser_test.go` | modify | Add end-to-end tests via `ParseFile` |
| `internal/parser/testdata/graphql_query_file.yaml` | create | Fixture: `query_file:` only |
| `internal/parser/testdata/graphql_fragments.yaml` | create | Fixture: `query_file:` + multiple fragments |
| `internal/parser/testdata/graphql_fragment_cycle.yaml` | create | Fixture: circular fragments |
| `internal/parser/testdata/graphql_missing_query_file.yaml` | create | Fixture: missing file |
| `internal/parser/testdata/graphql_query_and_file.yaml` | create | Fixture: both `query` and `query_file` set |
| `internal/parser/testdata/graphql/queries/get_user.graphql` | create | Fixture query file |
| `internal/parser/testdata/graphql/fragments/user_fields.graphql` | create | Fixture fragment |
| `internal/parser/testdata/graphql/fragments/post_fields.graphql` | create | Fixture fragment with dep on UserFields |
| `internal/parser/testdata/graphql/fragments/cycle_a.graphql` | create | Cycle fixture |
| `internal/parser/testdata/graphql/fragments/cycle_b.graphql` | create | Cycle fixture |

#### Current Code (parser.go, around line 165-168)

```go
			// GraphQL requests don't require method validation - BuildRequest forces POST
			if protocol == "graphql" && (*section)[i].Request.Method == "" {
				(*section)[i].Request.Method = "POST"
			}
		}
	}
```

#### New Code

Add a new pass in `ParseFile` that runs **after** `resolveExternalReferences` (so requests pulled from external files are also processed) and **before** the existing GraphQL validation block. This resolves `query_file` + `fragments` into `Query`. The existing validation that rejects empty `Query` then correctly covers the case where both are missing.

```go
// Resolve external query / fragment files for GraphQL requests. Must run
// before validation below, which expects GraphQLConfig.Query to be populated.
collectionDir := filepath.Dir(absPath)
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        gql := (*section)[i].Request.GraphQL
        if gql == nil {
            continue
        }
        if gql.QueryFile == "" && len(gql.Fragments) == 0 {
            continue
        }
        res, loadErr := graphql.LoadQuery(graphql.LoadQueryInput{
            BaseDir:     collectionDir,
            InlineQuery: gql.Query,
            QueryFile:   gql.QueryFile,
            Fragments:   gql.Fragments,
        })
        if loadErr != nil {
            return nil, &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: path,
                Message:  fmt.Sprintf("graphql request %q: %s", (*section)[i].Name, loadErr),
                Hint:     "Check graphql.query_file and graphql.fragments paths",
                Inner:    loadErr,
            }
        }
        gql.Query = res.Query
        col.ExternalFiles = append(col.ExternalFiles, res.FilePaths...)
    }
}
```

Also update the existing GraphQL validation block so it no longer hard-rejects when `Query` is empty **before** external files have been resolved. After external loading, the current check at line 143 (`gql.Query == ""`) becomes the final backstop that fires only when neither inline nor external content was provided. No change needed to the existing check - the ordering ensures it runs after loading.

Import addition:
```go
import (
    // ... existing ...
    "github.com/weiqigod/curlew/internal/graphql"
)
```

**Import cycle check:** `internal/graphql` currently imports `internal/parser` (for `parser.Request`). Adding `internal/parser` -> `internal/graphql` creates a cycle. **Resolution:** Move `LoadQuery` / `LoadQueryInput` / `LoadQueryResult` and the sentinel errors into a new package `internal/graphqlfiles` that has **no dependency on parser**. The existing `graphql` package continues to import parser; the new `graphqlfiles` package is parser-free.

**Revised file location:**

| Original plan | Revised plan |
|---------------|--------------|
| `internal/graphql/files.go` | `internal/graphql/files.go` (stays) |
| `internal/graphql/files_test.go` | `internal/graphql/files_test.go` (stays) |

The `files.go` code as drafted does **not** import parser - it only uses `os`, `fmt`, `path/filepath`, `regexp`, `strings`. The current `graphql/graphql.go` imports parser, but Go only checks package-level imports, not file-level. However the graphql package as a whole would still import parser, so `parser` importing `graphql` is the cycle.

**Final resolution:** Create a new subpackage `internal/graphql/files` that is parser-free. Parser imports `internal/graphql/files`; the existing `internal/graphql` package is untouched and continues to import parser.

#### Revised Step 3 location

| File | Action | Description |
|------|--------|-------------|
| `internal/graphql/files/files.go` | create | `LoadQuery` + fragment logic, no parser dependency |
| `internal/graphql/files/errors.go` | create | Sentinel errors (`ErrQueryFileNotFound`, `ErrFragmentFileNotFound`, `ErrFragmentCycle`, `ErrQueryMutuallyExclusive`, `ErrEmptyQuery`) |
| `internal/graphql/files/files_test.go` | create | Unit tests |

The existing `internal/graphql/errors.go` keeps `ErrEmptyQuery`. The new `files` package defines its own `ErrEmptyQuery` as `= graphql.ErrEmptyQuery`? No - that reintroduces the cycle. Instead: define a **new sentinel** in `internal/graphql/files/errors.go` (e.g. `ErrMissingQuery`) with a different name. Parser uses `files.ErrMissingQuery`. Existing graphql package still uses its own `ErrEmptyQuery` for `BuildRequest`. Both are separate sentinels; tests reference the correct one per layer.

**Step 2 revision:** The sentinels live in `internal/graphql/files/errors.go`, not `internal/graphql/errors.go`. The existing `internal/graphql/errors.go` is untouched.

#### Testdata fixtures

`testdata/graphql/queries/get_user.graphql`:
```graphql
query GetUser($id: ID!) {
  user(id: $id) {
    ...UserFields
    posts {
      ...PostFields
    }
  }
}
```

`testdata/graphql/fragments/user_fields.graphql`:
```graphql
fragment UserFields on User {
  id
  name
  email
}
```

`testdata/graphql/fragments/post_fields.graphql`:
```graphql
fragment PostFields on Post {
  id
  title
  author {
    ...UserFields
  }
}
```

`testdata/graphql/fragments/cycle_a.graphql`:
```graphql
fragment A on T { ...B }
```

`testdata/graphql/fragments/cycle_b.graphql`:
```graphql
fragment B on T { ...A }
```

`testdata/graphql_query_file.yaml`:
```yaml
name: GraphQL With Query File
requests:
  - name: Get User
    request:
      protocol: graphql
      url: "https://api.example.com/graphql"
      graphql:
        query_file: graphql/queries/get_user.graphql
        variables:
          id: "{{user_id}}"
```

`testdata/graphql_fragments.yaml`:
```yaml
name: GraphQL With Fragments
requests:
  - name: Get User With Posts
    request:
      protocol: graphql
      url: "https://api.example.com/graphql"
      graphql:
        query_file: graphql/queries/get_user.graphql
        fragments:
          - graphql/fragments/post_fields.graphql
          - graphql/fragments/user_fields.graphql
        variables:
          id: "123"
```

`testdata/graphql_fragment_cycle.yaml`:
```yaml
name: GraphQL Fragment Cycle
requests:
  - name: Cyclic
    request:
      protocol: graphql
      url: "https://api.example.com/graphql"
      graphql:
        query: "{ x { ...A } }"
        fragments:
          - graphql/fragments/cycle_a.graphql
          - graphql/fragments/cycle_b.graphql
```

`testdata/graphql_missing_query_file.yaml`:
```yaml
name: Missing Query File
requests:
  - name: Missing
    request:
      protocol: graphql
      url: "https://api.example.com/graphql"
      graphql:
        query_file: graphql/queries/does_not_exist.graphql
```

`testdata/graphql_query_and_file.yaml`:
```yaml
name: Mutually Exclusive
requests:
  - name: Both
    request:
      protocol: graphql
      url: "https://api.example.com/graphql"
      graphql:
        query: "{ me { id } }"
        query_file: graphql/queries/get_user.graphql
```

#### Tests to Write FIRST (RED phase)

Add to the existing `TestParseFile_GraphQL` block in `parser_test.go`:

```go
t.Run("graphql query_file loads external query", func(t *testing.T) {
    col, err := ParseFile("testdata/graphql_query_file.yaml")
    if err != nil {
        t.Fatalf("ParseFile error: %v", err)
    }
    gql := col.Requests.Items[0].Request.GraphQL
    if !strings.Contains(gql.Query, "query GetUser") {
        t.Errorf("Query missing loaded content: %q", gql.Query)
    }
    if len(col.ExternalFiles) == 0 {
        t.Error("ExternalFiles not populated from query_file")
    }
})

t.Run("graphql fragments concatenated in dependency order", func(t *testing.T) {
    col, err := ParseFile("testdata/graphql_fragments.yaml")
    if err != nil {
        t.Fatalf("ParseFile error: %v", err)
    }
    q := col.Requests.Items[0].Request.GraphQL.Query
    userIdx := strings.Index(q, "fragment UserFields")
    postIdx := strings.Index(q, "fragment PostFields")
    queryIdx := strings.Index(q, "query GetUser")
    if userIdx < 0 || postIdx < 0 || queryIdx < 0 {
        t.Fatalf("concatenated query missing parts:\n%s", q)
    }
    if userIdx > postIdx {
        t.Errorf("UserFields should come before PostFields (dep order); got UserFields@%d PostFields@%d", userIdx, postIdx)
    }
    if postIdx > queryIdx {
        t.Errorf("fragments should come before query; got PostFields@%d query@%d", postIdx, queryIdx)
    }
    if len(col.ExternalFiles) != 3 {
        t.Errorf("ExternalFiles count = %d, want 3", len(col.ExternalFiles))
    }
})

t.Run("graphql circular fragment dependency returns ErrFragmentCycle", func(t *testing.T) {
    _, err := ParseFile("testdata/graphql_fragment_cycle.yaml")
    if err == nil {
        t.Fatal("expected error for circular fragments")
    }
    if !errors.Is(err, files.ErrFragmentCycle) {
        t.Errorf("expected ErrFragmentCycle, got %v", err)
    }
})

t.Run("graphql missing query_file returns clear error", func(t *testing.T) {
    _, err := ParseFile("testdata/graphql_missing_query_file.yaml")
    if err == nil {
        t.Fatal("expected error for missing query_file")
    }
    if !errors.Is(err, files.ErrQueryFileNotFound) {
        t.Errorf("expected ErrQueryFileNotFound, got %v", err)
    }
    // Error message must include the expected path for easy debugging.
    if !strings.Contains(err.Error(), "does_not_exist.graphql") {
        t.Errorf("error missing expected path: %v", err)
    }
})

t.Run("graphql query and query_file are mutually exclusive", func(t *testing.T) {
    _, err := ParseFile("testdata/graphql_query_and_file.yaml")
    if err == nil {
        t.Fatal("expected error for both query and query_file")
    }
    if !errors.Is(err, files.ErrQueryMutuallyExclusive) {
        t.Errorf("expected ErrQueryMutuallyExclusive, got %v", err)
    }
})
```

Add the import `"github.com/weiqigod/curlew/internal/graphql/files"` and `"strings"` (if not present) to `parser_test.go`.

#### Impact on Existing Tests
- The existing `graphql without query field` subtest still passes: the parser's final `gql.Query == ""` check still fires when neither inline nor external content was supplied.
- `TestParseFile_ExternalFiles` is unaffected - it tests the `path:` (external request YAML) mechanism, not GraphQL files.
- All `internal/runner/runner_test.go` GraphQL tests continue to pass because they construct `GraphQLConfig{Query: "..."}` directly, bypassing the parser.

---

### Step 5: Integration test with a variable placeholder in a loaded query file

**Rationale:** Behavior 6 ("variable interpolation in external query files") crosses parser -> requtil -> runner. We need an end-to-end assertion that `{{var}}` inside a loaded .graphql file is resolved at run time. The simplest level is to test `requtil.InterpolateRequest` directly, using a `Request` whose `GraphQL.Query` was pre-populated from a file (simulating what the parser produced).

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/requtil/requtil_test.go` | modify | Add a new test: given `gql.Query` containing `{{user_id}}`, after interpolation the placeholder is replaced |

#### Tests to Write FIRST (RED phase)

```go
func TestInterpolateRequest_GraphQLQueryFromFile(t *testing.T) {
    scope := variable.NewScope()
    scope.Set("user_id", "abc-123")

    req := &parser.Request{
        URL:      "https://api.example.com/graphql",
        Protocol: "graphql",
        GraphQL: &parser.GraphQLConfig{
            // Simulates a query loaded from an external file that contains
            // a {{var}} placeholder.
            Query: "query { user(id: \"{{user_id}}\") { id } }",
        },
    }

    out, err := InterpolateRequest(scope, req)
    if err != nil {
        t.Fatalf("InterpolateRequest error: %v", err)
    }
    if strings.Contains(out.GraphQL.Query, "{{user_id}}") {
        t.Errorf("placeholder not interpolated: %q", out.GraphQL.Query)
    }
    if !strings.Contains(out.GraphQL.Query, "abc-123") {
        t.Errorf("value not substituted: %q", out.GraphQL.Query)
    }
}
```

This test may already pass given the existing implementation in `requtil.go` line 61. If so, it becomes a regression guard for M2-030; if it fails because the existing behavior has a gap, we fix it.

#### Impact on Existing Tests
- None.

---

### Step 6: Update the smoke test to exercise the observable

**Rationale:** The completeness contract requires the smoke test to exercise the real binary for new capabilities. Adds a tiny fixture that reproduces the observable in the task YAML.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add a GraphQL `query_file` + `fragments` parse test that expects success (parse ok, then hits feature gate = Free tier) |

#### New Code (appended to the existing GraphQL smoke block)

```bash
echo "--- GraphQL query_file + fragments (expect parse success, then feature gate) ---"
GQL_DIR=$(mktemp -d /tmp/curlew_gql_files_XXXXXX)
mkdir -p "$GQL_DIR/graphql/queries" "$GQL_DIR/graphql/fragments"
cat > "$GQL_DIR/graphql/queries/get_user.graphql" << 'GQL'
query GetUser {
  user { ...UserFields }
}
GQL
cat > "$GQL_DIR/graphql/fragments/user_fields.graphql" << 'GQL'
fragment UserFields on User {
  id
  name
}
GQL
cat > "$GQL_DIR/tests.yaml" << 'YAML'
name: GraphQL Files Test
requests:
  - name: Get User
    request:
      protocol: graphql
      url: "https://example.com/graphql"
      graphql:
        query_file: graphql/queries/get_user.graphql
        fragments:
          - graphql/fragments/user_fields.graphql
YAML
GQL_FILES_OUT=$(./curlew run "$GQL_DIR/tests.yaml" 2>&1 || true)
if echo "$GQL_FILES_OUT" | grep -q "Professional tier"; then
  echo "PASS: graphql query_file + fragments parsed successfully (gated at Free tier)"
else
  echo "FAIL: expected Professional tier gate after successful parse, got: $GQL_FILES_OUT"
  rm -rf "$GQL_DIR"
  exit 1
fi
rm -rf "$GQL_DIR"
echo
```

**Design decision:** We check for the Professional tier gate because GraphQL is gated at Free. Hitting the gate means the parser successfully loaded `query_file` and the fragments (otherwise a parse error would have fired first). This is the same pattern used by the existing GraphQL smoke block.

#### Impact on Existing Tests
- None.

---

### Step 7: CHANGELOG entry

**Rationale:** Required for task completion per quality gates.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `CHANGELOG.md` | modify | Add an entry under the M2 section for M2-030 |

#### New Code
```markdown
- GraphQL external query files (`graphql.query_file:`) and fragment loading
  (`graphql.fragments:`) with automatic dependency ordering and circular
  dependency detection (M2-030)
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/graphql/files/files_test.go` | `TestLoadQuery` | new | write (Step 3) |
| `internal/graphql/files/files_test.go` | `TestExtractFragmentSpreads` | new | write (Step 3) |
| `internal/parser/parser_test.go` | `TestParseFile_GraphQL` | extend | add 5 new subtests (Step 4) |
| `internal/parser/parser_test.go` | `graphql without query field` | none | still passes (final backstop) |
| `internal/requtil/requtil_test.go` | `TestInterpolateRequest_GraphQLQueryFromFile` | new | write (Step 5) |
| `internal/runner/runner_test.go` | all GraphQL tests | none | construct `GraphQLConfig{Query: ...}` directly, bypass parser |
| `internal/parser/parser_test.go` | `TestParseFile_ExternalFiles` | none | tests `path:` not `query_file:` |

## Risks and Edge Cases

- **Risk:** Import cycle between `internal/parser` and `internal/graphql` (graphql already imports parser for `parser.Request`).  
  **Mitigation:** Put the file-loading logic in a new subpackage `internal/graphql/files` that has no dependency on parser. Parser imports `files`; the existing `graphql` package stays as-is.

- **Risk:** Fragment name collision - two loaded fragments declaring the same name.  
  **Mitigation:** Detect it in `LoadQuery` and return a clear error (`fmt.Errorf("duplicate fragment name %q in files %q and %q", ...)`). Not called out in the spec but would cause confusing runtime failures otherwise. **Decision:** implement this check; include a test case.

- **Edge case:** A fragment declares itself via `...Self` (self-reference). GraphQL syntax technically allows this but it has no effect; our topo sort would treat it as a cycle if not handled.  
  **Handling:** `extractFragmentSpreads` excludes references matching the declaring fragment's own name. Test case included.

- **Edge case:** A fragment references another fragment that isn't in the loaded set (e.g., it's defined inline in `query_file`).  
  **Handling:** Unknown references are silently ignored by `topoSortFragments` - they impose no ordering constraint. This matches the spec language "dependencies resolved automatically" and the principle that we only reorder what we control.

- **Edge case:** `query_file` is an absolute path.  
  **Handling:** `resolvePath` returns absolute paths unchanged.

- **Edge case:** `query_file` path contains `..` and escapes the collection directory.  
  **Handling:** We do not restrict this - user explicitly opted into file-based queries. Consistent with how `resolveExternalReferences` handles `path:` for request YAML files.

- **Edge case:** The loaded `query_file` is empty.  
  **Handling:** Falls through to the existing `ErrEmptyQuery` / `ErrMissingQuery` check (inside `LoadQuery`). Test case included.

- **Edge case:** Fragment file contains multiple fragment declarations.  
  **Handling:** The regex `FindStringSubmatch` (singular) captures only the first declaration's name. Spread refs from the whole file body are still collected. This is good enough for M2-030: declaring multiple fragments per file is uncommon and not called out in the spec. **Decision:** document as a known limitation in doc comments.

- **Risk:** The `ExternalFiles` list is used by watch mode. Adding loaded `.graphql` files means watch rerun will trigger on fragment edits, which is the desired behavior.  
  **Mitigation:** Verified by tracing `internal/watch/paths.go` usage of `col.ExternalFiles`. No code change needed.

## Open Questions Resolved

1. **Should fragments be interpolated for `{{var}}` placeholders?** Spec behavior 6 says "variable interpolation in external query files" — this naturally covers both the query file and fragments because after loading, everything is concatenated into `GraphQLConfig.Query`, which `requtil.InterpolateRequest` interpolates whole. **Resolved:** yes, for free via the existing pipeline.

2. **Where should file loading happen — parser or runner?** Parser, so the loaded content participates in the existing interpolation pipeline without special-casing in the runner. Also ensures parse-time errors (missing file, cycle) surface during `curlew validate`, which is a nicer UX.

3. **What package owns the logic?** New subpackage `internal/graphql/files` to avoid import cycles with parser.

4. **Are multiple fragment declarations in one file supported?** Only the first declaration's name is recorded for dependency resolution; all spread refs are still scanned. Documented as a limitation.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification (from task YAML):
```bash
# Create the directory structure
mkdir -p /tmp/m2-030-demo/graphql/{queries,fragments}
cd /tmp/m2-030-demo

cat > graphql/queries/get_user.graphql << 'GQL'
query GetUser($id: ID!) {
  user(id: $id) {
    ...UserFields
  }
}
GQL

cat > graphql/fragments/user_fields.graphql << 'GQL'
fragment UserFields on User {
  id
  name
  email
}
GQL

cat > tests.yaml << 'YAML'
name: GraphQL Files Demo
requests:
  - name: Get User
    request:
      protocol: graphql
      url: "https://example.com/graphql"
      graphql:
        query_file: graphql/queries/get_user.graphql
        fragments:
          - graphql/fragments/user_fields.graphql
        variables:
          id: "{{user_id}}"
YAML

# Run (will hit the Professional-tier feature gate on Free,
# proving the parse + load pipeline works end-to-end)
curlew run tests.yaml --var user_id=abc-123

# Run the graphql package tests
go test ./internal/graphql/...
```
