package parser

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/variable"
	"gopkg.in/yaml.v3"
)

// includeContext threads the cumulative variable overrides, visited file set,
// and accumulated external file list through recursive include resolution.
type includeContext struct {
	cumulativeVars map[string]string // overrides accumulated from enclosing includes
	visited        map[string]bool   // absolute paths visited on the current recursion path
	extFiles       *[]string         // appended-to: every successfully loaded include path
}

// resolveIncludes walks col.Include, parses each child collection, recursively
// resolves its includes, stamps cumulativeVars onto each child request item,
// and appends the spliced items into col.Setup/Requests/Teardown in include
// order. cumulativeVars at the root level should be nil.
//
// The function mutates col in place. col.Include is left intact for caller
// inspection (e.g. tests); runtime code ignores it.
func resolveIncludes(col *Collection, parentPath string, ctx *includeContext) error {
	if len(col.Include) == 0 {
		return nil
	}
	parentDir := filepath.Dir(parentPath)
	for idx, raw := range col.Include {
		childPath := raw
		if !filepath.IsAbs(childPath) {
			childPath = filepath.Join(parentDir, childPath)
		}
		absChild, absErr := filepath.Abs(childPath)
		if absErr != nil {
			return &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: parentPath,
				Message:  fmt.Sprintf("include[%d]: cannot resolve path %q: %s", idx, raw, absErr),
				Inner:    absErr,
			}
		}

		// Resolve symlinks for cycle detection accuracy.
		if resolved, symlinkErr := filepath.EvalSymlinks(absChild); symlinkErr == nil {
			absChild = resolved
		}

		if ctx.visited[absChild] {
			return &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: parentPath,
				Message: fmt.Sprintf(
					"circular include: %q includes %q which is already on the include path",
					parentPath, absChild,
				),
				Inner: ErrCircularInclude,
			}
		}

		// Read child raw bytes and parse without re-entering ParseFileWithOptions
		// (we don't want to re-fire the gate on a child).
		data, readErr := os.ReadFile(absChild)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				return &apierrors.Structured{
					Category: apierrors.CategoryParse,
					FilePath: parentPath,
					Line:     findIncludeLine(parentPath, idx),
					Message:  fmt.Sprintf("include[%d]: file not found: %s", idx, raw),
					Inner:    ErrIncludeNotFound,
				}
			}
			return &apierrors.Structured{
				Category: apierrors.CategoryParse,
				FilePath: parentPath,
				Message:  fmt.Sprintf("include[%d]: reading %s: %s", idx, raw, readErr),
				Inner:    readErr,
			}
		}

		// Parse child as a Collection. parseCollectionBytes performs structural
		// unmarshal + per-request validation + resolveExternalReferences
		// against the child's directory. Include directives inside the child
		// are resolved recursively by the explicit resolveIncludes call below,
		// not by parseCollectionBytes (which does not call resolveIncludes).
		child, parseErr := parseCollectionBytes(absChild, data)
		if parseErr != nil {
			return parseErr
		}

		// Compute the override map this child contributes to its descendants
		// and to its own spliced request items.
		childOverrides := mergeStringMaps(ctx.cumulativeVars, child.Variables.Values)
		childCtx := &includeContext{
			cumulativeVars: childOverrides,
			visited:        cloneVisited(ctx.visited, absChild),
			extFiles:       ctx.extFiles,
		}

		// Recursively resolve the child's own include: entries with the
		// child's augmented context.
		if err := resolveIncludes(child, absChild, childCtx); err != nil {
			return err
		}

		// Stamp overrides onto every spliced request item, then append.
		// Note: sensitivity is NOT propagated through the include snapshot;
		// the runtime base scope's SensitiveSet handles redaction for any
		// parent variables. We stamp only values.
		stampOverrides(child.Setup.Items, childOverrides)
		stampOverrides(child.Requests.Items, childOverrides)
		stampOverrides(child.Teardown.Items, childOverrides)

		col.Setup.Items = append(col.Setup.Items, child.Setup.Items...)
		col.Requests.Items = append(col.Requests.Items, child.Requests.Items...)
		col.Teardown.Items = append(col.Teardown.Items, child.Teardown.Items...)

		// Track the loaded child file (and its transitively loaded includes).
		*ctx.extFiles = append(*ctx.extFiles, absChild)
		*ctx.extFiles = append(*ctx.extFiles, child.ExternalFiles...)
	}
	return nil
}

// stampOverrides merges overrides into each item's Variables.Values.
// Existing per-request overrides on the item win over include-supplied
// overrides (request-site specificity). Sensitivity flags are NOT propagated
// through the include snapshot; the parent's runtime SensitiveSet covers them.
func stampOverrides(items []RequestItem, overrides map[string]string) {
	if len(overrides) == 0 {
		return
	}
	for i := range items {
		merged := make(map[string]string, len(overrides)+len(items[i].Variables.Values))
		for k, v := range overrides {
			merged[k] = v
		}
		for k, v := range items[i].Variables.Values {
			merged[k] = v // request-site override wins
		}
		items[i].Variables.Values = merged
		if items[i].Variables.Sensitive == nil {
			// Ensure a non-nil SensitiveSet so downstream code can inspect it
			// without nil checks.
			items[i].Variables.Sensitive = variable.NewSensitiveSet()
		}
	}
}

// mergeStringMaps returns a new map containing all keys from base, overridden
// by all keys from over. Returns nil when both inputs are empty.
func mergeStringMaps(base, over map[string]string) map[string]string {
	if len(base) == 0 && len(over) == 0 {
		return nil
	}
	out := make(map[string]string, len(base)+len(over))
	for k, v := range base {
		out[k] = v
	}
	for k, v := range over {
		out[k] = v
	}
	return out
}

// cloneVisited returns a copy of v with extra added.
func cloneVisited(v map[string]bool, extra string) map[string]bool {
	out := make(map[string]bool, len(v)+1)
	for k := range v {
		out[k] = true
	}
	out[extra] = true
	return out
}

// findIncludeLine returns the YAML line number of the include[idx] entry in
// the parent file. Returns 0 on any parse error or out-of-range index.
// Only called on the error path; the extra file I/O is acceptable.
func findIncludeLine(parentPath string, idx int) int {
	data, err := os.ReadFile(parentPath)
	if err != nil {
		return 0
	}
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return 0
	}
	if doc.Kind != yaml.DocumentNode || len(doc.Content) == 0 {
		return 0
	}
	mapping := doc.Content[0]
	if mapping.Kind != yaml.MappingNode {
		return 0
	}
	for i := 0; i+1 < len(mapping.Content); i += 2 {
		if mapping.Content[i].Value == "include" {
			seq := mapping.Content[i+1]
			if seq.Kind != yaml.SequenceNode || idx < 0 || idx >= len(seq.Content) {
				return mapping.Content[i].Line
			}
			return seq.Content[idx].Line
		}
	}
	return 0
}
