// Package fuzzseed loads fixtures already in the repository as fuzz seed
// corpora. A seed corpus of real inputs finds interesting mutations far faster
// than a corpus of empty strings, and every source here is a file some other
// test already depends on, so a seed set that silently empties is a moved
// fixture rather than a design choice.
package fuzzseed

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// ErrNoSeeds is returned when a named source set matched no files. Returning
// an error rather than an empty slice is deliberate: a fuzz target seeded
// with nothing still passes, which is the false clear internal/backlog
// exists to prevent.
var ErrNoSeeds = errors.New("fuzz seed set is empty")

// Seed is one fixture: its repo-relative path and its bytes. The path is
// carried so a failing seed names the file it came from.
type Seed struct {
	Path string
	Data []byte
}

// Root returns the repository root, found by walking up from dir until a
// directory containing go.mod is reached.
func Root(dir string) (string, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return "", fmt.Errorf("resolving %q: %w", dir, err)
	}
	for d := abs; ; {
		if _, err := os.Stat(filepath.Join(d, "go.mod")); err == nil {
			return d, nil
		}
		parent := filepath.Dir(d)
		if parent == d {
			return "", fmt.Errorf("no go.mod found above %q", abs)
		}
		d = parent
	}
}

// collectionSource is one directory Collections reads from, relative to the
// repository root.
type collectionSource struct {
	dir       string
	recursive bool
}

// collectionSources lists every directory that holds collection-shaped YAML
// fixtures in this repository. Keep in sync with the plan's source list
// (management/plans/M27-002-plan.md, Step 1): parser testdata, the mudflat
// dogfood suite, its environment and gaps files, and the worked examples.
var collectionSources = []collectionSource{
	{"internal/parser/testdata", true},
	{"testapi/collections", true},
	{"testapi", false},
	{"testapi/environments", true},
	{"testapi/gaps", true},
	{"examples", true},
}

// Collections returns every collection-shaped YAML fixture in the
// repository: internal/parser/testdata/**/*.yaml, testapi/collections/**/*.yaml,
// testapi/*.yaml, testapi/environments/*.yaml, testapi/gaps/*.yaml and
// examples/**/*.yaml.
func Collections(root string) ([]Seed, error) {
	var seeds []Seed
	for _, src := range collectionSources {
		dir := filepath.Join(root, src.dir)
		files, err := yamlFilesIn(dir, src.recursive)
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", src.dir, err)
		}
		for _, path := range files {
			data, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("reading %s: %w", path, err)
			}
			seeds = append(seeds, Seed{Path: relPath(root, path), Data: data})
		}
	}
	if len(seeds) == 0 {
		return nil, ErrNoSeeds
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].Path < seeds[j].Path })
	return seeds, nil
}

// Templates returns every scalar containing "{{" found in Collections.
func Templates(root string) ([]string, error) {
	cols, err := Collections(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cols {
		doc, ok := parseYAML(c.Data)
		if !ok {
			continue
		}
		for _, v := range collectAllScalars(doc) {
			if strings.Contains(v, "{{") {
				out = append(out, v)
			}
		}
	}
	out = dedupe(out)
	if len(out) == 0 {
		return nil, ErrNoSeeds
	}
	sort.Strings(out)
	return out, nil
}

// CELExpressions returns every scalar appearing under a cel: key in
// Collections, plus the CEL expressions in docs/MANUAL.md's tables.
func CELExpressions(root string) ([]string, error) {
	cols, err := Collections(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cols {
		doc, ok := parseYAML(c.Data)
		if !ok {
			continue
		}
		walkMappings(doc, func(key, val *yaml.Node) {
			if key.Kind == yaml.ScalarNode && key.Value == "cel" {
				out = append(out, collectAllScalars(val)...)
			}
		})
	}
	manual, err := celSeedsFromManual(root)
	if err != nil {
		return nil, err
	}
	out = append(out, manual...)
	out = dedupe(out)
	if len(out) == 0 {
		return nil, ErrNoSeeds
	}
	sort.Strings(out)
	return out, nil
}

// JSONPaths returns every JSONPath used in Collections: the mapping keys
// under an assertions.body block, and any scalar beginning with "$.".
func JSONPaths(root string) ([]string, error) {
	cols, err := Collections(root)
	if err != nil {
		return nil, err
	}
	var out []string
	for _, c := range cols {
		doc, ok := parseYAML(c.Data)
		if !ok {
			continue
		}
		walkMappings(doc, func(key, val *yaml.Node) {
			if key.Kind != yaml.ScalarNode || key.Value != "assertions" || val.Kind != yaml.MappingNode {
				return
			}
			for i := 0; i+1 < len(val.Content); i += 2 {
				bkey, bval := val.Content[i], val.Content[i+1]
				if bkey.Kind == yaml.ScalarNode && bkey.Value == "body" && bval.Kind == yaml.MappingNode {
					for j := 0; j+1 < len(bval.Content); j += 2 {
						pkey := bval.Content[j]
						if pkey.Kind == yaml.ScalarNode {
							out = append(out, pkey.Value)
						}
					}
				}
			}
		})
		for _, v := range collectAllScalars(doc) {
			if strings.HasPrefix(v, "$.") {
				out = append(out, v)
			}
		}
	}
	out = dedupe(out)
	if len(out) == 0 {
		return nil, ErrNoSeeds
	}
	sort.Strings(out)
	return out, nil
}

// JSONBodies returns the bytes of every *.json fixture under internal/.
func JSONBodies(root string) ([]Seed, error) {
	dir := filepath.Join(root, "internal")
	var seeds []Seed
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNoSeeds
		}
		return nil, fmt.Errorf("reading internal: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("internal: %w", fmt.Errorf("not a directory"))
	}
	err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		if !strings.EqualFold(filepath.Ext(path), ".json") {
			return nil
		}
		data, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		seeds = append(seeds, Seed{Path: relPath(root, path), Data: data})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking internal: %w", err)
	}
	if len(seeds) == 0 {
		return nil, ErrNoSeeds
	}
	sort.Slice(seeds, func(i, j int) bool { return seeds[i].Path < seeds[j].Path })
	return seeds, nil
}

// yamlFilesIn returns every *.yaml file under dir, sorted. A missing dir
// yields zero files rather than an error -- Collections turns an
// all-sources-empty result into ErrNoSeeds itself, and treating one missing
// source directory as fatal would make the aggregate reader too brittle to
// tolerate a directory rename in a source list this task does not own.
func yamlFilesIn(dir string, recursive bool) ([]string, error) {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", dir)
	}

	var files []string
	if recursive {
		err = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				return nil
			}
			if strings.EqualFold(filepath.Ext(path), ".yaml") {
				files = append(files, path)
			}
			return nil
		})
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(dir)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if strings.EqualFold(filepath.Ext(e.Name()), ".yaml") {
				files = append(files, filepath.Join(dir, e.Name()))
			}
		}
	}
	sort.Strings(files)
	return files, nil
}

// relPath returns path relative to root, or path unchanged if it cannot be
// made relative -- the fallback keeps Seed.Path usable even for a source
// outside root, which should not happen but must not panic if it does.
func relPath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

// parseYAML parses data as a YAML document node. It returns ok=false rather
// than an error for a fixture that fails to parse: such a fixture is still a
// legitimate seed for FuzzParseCollection (Collections returns it raw), it
// just contributes nothing to the structural readers (Templates,
// CELExpressions, JSONPaths) that need a walkable tree.
func parseYAML(data []byte) (*yaml.Node, bool) {
	var doc yaml.Node
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return nil, false
	}
	return &doc, true
}

// walkNodes calls visit on n and every node reachable from it.
func walkNodes(n *yaml.Node, visit func(*yaml.Node)) {
	if n == nil {
		return
	}
	visit(n)
	for _, c := range n.Content {
		walkNodes(c, visit)
	}
}

// collectAllScalars returns the value of every scalar node reachable from n.
func collectAllScalars(n *yaml.Node) []string {
	var out []string
	walkNodes(n, func(x *yaml.Node) {
		if x.Kind == yaml.ScalarNode {
			out = append(out, x.Value)
		}
	})
	return out
}

// walkMappings calls fn on every (key, value) pair of every mapping node
// reachable from n, then continues walking into value so mappings nested
// inside sequences or other mappings are visited too.
func walkMappings(n *yaml.Node, fn func(key, value *yaml.Node)) {
	if n == nil {
		return
	}
	if n.Kind == yaml.MappingNode {
		for i := 0; i+1 < len(n.Content); i += 2 {
			key, val := n.Content[i], n.Content[i+1]
			fn(key, val)
			walkMappings(val, fn)
		}
		return
	}
	for _, c := range n.Content {
		walkMappings(c, fn)
	}
}

// dedupe returns in with exact duplicates removed, order preserved by first
// occurrence.
func dedupe(in []string) []string {
	seen := make(map[string]struct{}, len(in))
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, ok := seen[s]; ok {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

// celSection is the heading celSeedsFromManual scans docs/MANUAL.md for.
const celSection = "### 3.10 Expression Language (CEL)"

var (
	singleQuoted = regexp.MustCompile(`'([^'\n]+)'`)
	backtickCall = regexp.MustCompile("`([^`\n]*\\([^`\n]*\\)[^`\n]*)`")
)

// celSeedsFromManual extracts example CEL expressions from the "Expression
// Language (CEL)" section of docs/MANUAL.md: single-quoted inline examples
// (e.g. 'response.body.items.size() == response.body.total') and backtick
// code spans that look like a function call (e.g. `now()`). A missing
// MANUAL.md or missing section yields no seeds, not an error -- this reader
// is a supplement to the cel: keys already read from Collections, not the
// only source, so its absence alone must not empty the whole corpus.
func celSeedsFromManual(root string) ([]string, error) {
	path := filepath.Join(root, "docs", "MANUAL.md")
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("reading %s: %w", path, err)
	}
	text := string(data)
	start := strings.Index(text, celSection)
	if start < 0 {
		return nil, nil
	}
	section := text[start+len(celSection):]
	if end := strings.Index(section, "\n### "); end >= 0 {
		section = section[:end]
	}

	var out []string
	for _, m := range singleQuoted.FindAllStringSubmatch(section, -1) {
		if len(m[1]) > 3 {
			out = append(out, m[1])
		}
	}
	for _, m := range backtickCall.FindAllStringSubmatch(section, -1) {
		out = append(out, m[1])
	}
	return out, nil
}
