package parser

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/fuzzseed"
)

// fuzzParse runs everything ParseFile runs except os.ReadFile (the fuzzer
// supplies the bytes directly) and resolveIncludes (which needs real files on
// disk, which a fuzzed collection.Include list does not have). See
// management/plans/M27-002-plan.md design decision D2:
// TestFuzzParseCollection_matches_ParseFile holds this function to the real
// pipeline, so a validation step added to ParseFile without a matching
// addition here is caught rather than silently going unfuzzed.
func fuzzParse(path string, data []byte) (*Collection, error) {
	col, err := parseCollectionBytes(path, data)
	if err != nil {
		return nil, err
	}
	if collectionHasSchema(col) {
		if err := compileSchemas(col, path); err != nil {
			return nil, err
		}
	}
	if err := checkDuplicateNames(col, path); err != nil {
		return nil, err
	}
	if err := populateSlugs(col, path); err != nil {
		return nil, err
	}
	if err := validateDependsOn(col, path); err != nil {
		return nil, err
	}
	return col, nil
}

// FuzzParseCollection fuzzes the collection-parsing pipeline every collection
// a user hand-writes goes through, short of include resolution and reading
// the file from disk (see fuzzParse).
func FuzzParseCollection(f *testing.F) {
	root, err := fuzzseed.Root(".")
	if err != nil {
		f.Fatalf("locating repo root: %v", err)
	}
	seeds, err := fuzzseed.Collections(root)
	if err != nil {
		f.Fatalf("loading collection seeds: %v", err)
	}
	for _, s := range seeds {
		f.Add(s.Data)
	}

	// One empty directory for the whole target, not one per iteration:
	// parseCollectionBytes never reads `path` itself, it only takes its
	// filepath.Dir as the base for external references.
	path := filepath.Join(f.TempDir(), "collection.yaml")

	f.Fuzz(func(t *testing.T, data []byte) {
		col, err := fuzzParse(path, data)
		switch {
		case err != nil && col != nil:
			t.Fatalf("fuzzParse returned both a collection and an error: %v", err)
		case err == nil && col == nil:
			t.Fatalf("fuzzParse returned neither a collection nor an error")
		case err == nil && col.Name == "":
			// parseCollectionBytes rejects an empty name with
			// ErrEmptyCollection, so a nameless success is a hole in that check.
			t.Fatalf("fuzzParse accepted a collection with no name")
		}
	})
}

// TestFuzzParseCollection_matches_ParseFile is the anti-drift guard for D2.
// Without it, a validation step added to ParseFile is a step the fuzzer
// silently stops covering, because fuzzParse would keep mirroring a stale
// snapshot of the pipeline.
//
// For every *.yaml fixture under testdata that declares no include: key (the
// one pipeline step fuzzParse cannot run without real files on disk),
// fuzzParse(path, data) and ParseFile(path) must agree on error-vs-success,
// and on Name when both succeed.
func TestFuzzParseCollection_matches_ParseFile(t *testing.T) {
	testdataDir := "testdata"
	var fixtures []string
	err := filepath.Walk(testdataDir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() || filepath.Ext(path) != ".yaml" {
			return nil
		}
		fixtures = append(fixtures, path)
		return nil
	})
	if err != nil {
		t.Fatalf("walking %s: %v", testdataDir, err)
	}
	if len(fixtures) == 0 {
		t.Fatalf("no fixtures found under %s; the parity guard is not testing anything", testdataDir)
	}

	for _, path := range fixtures {
		data, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("reading %s: %v", path, err)
		}
		if strings.Contains(string(data), "include:") {
			continue // needs resolveIncludes, which fuzzParse deliberately skips
		}

		t.Run(path, func(t *testing.T) {
			viaFuzz, fuzzErr := fuzzParse(path, data)
			viaParseFile, parseFileErr := ParseFile(path)

			if (fuzzErr == nil) != (parseFileErr == nil) {
				t.Fatalf("fuzzParse err=%v, ParseFile err=%v -- disagree on success", fuzzErr, parseFileErr)
			}
			if fuzzErr == nil {
				if viaFuzz.Name != viaParseFile.Name {
					t.Fatalf("fuzzParse Name=%q, ParseFile Name=%q -- disagree", viaFuzz.Name, viaParseFile.Name)
				}
			}
		})
	}
}
