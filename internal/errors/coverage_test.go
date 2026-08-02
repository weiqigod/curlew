package errors_test

// This test enforces the hint-table completeness contract for the agent event
// stream: every exported Err* sentinel in the internal/ tree must be
// registered via apierrors.RegisterPackage so ClassifyError can classify it.
// The blank imports below trigger each owning package's init() registration.
//
// When a new sentinel is added to the codebase, the test will fail until the
// owning package registers it.

import (
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	apierrors "github.com/peterlindqvist/apitest/internal/errors"

	// Blank imports force each owning package's init() to run, populating the
	// registry. The list must cover every internal package that declares an
	// Err* sentinel.
	_ "github.com/peterlindqvist/apitest/internal/assertion"
	_ "github.com/peterlindqvist/apitest/internal/auth"
	_ "github.com/peterlindqvist/apitest/internal/backend"
	_ "github.com/peterlindqvist/apitest/internal/backend/device"
	_ "github.com/peterlindqvist/apitest/internal/cel"
	_ "github.com/peterlindqvist/apitest/internal/config"
	_ "github.com/peterlindqvist/apitest/internal/datadriven"
	_ "github.com/peterlindqvist/apitest/internal/discovery"
	_ "github.com/peterlindqvist/apitest/internal/graphql"
	_ "github.com/peterlindqvist/apitest/internal/graphql/files"
	_ "github.com/peterlindqvist/apitest/internal/httpbody"
	_ "github.com/peterlindqvist/apitest/internal/httpexec"
	_ "github.com/peterlindqvist/apitest/internal/jsonpath"
	_ "github.com/peterlindqvist/apitest/internal/loadgen"
	_ "github.com/peterlindqvist/apitest/internal/loadgen/report"
	_ "github.com/peterlindqvist/apitest/internal/openapi"
	_ "github.com/peterlindqvist/apitest/internal/output"
	_ "github.com/peterlindqvist/apitest/internal/output/events"
	_ "github.com/peterlindqvist/apitest/internal/parser"
	_ "github.com/peterlindqvist/apitest/internal/plugin"
	_ "github.com/peterlindqvist/apitest/internal/plugin/hooks"
	_ "github.com/peterlindqvist/apitest/internal/prcheck"
	_ "github.com/peterlindqvist/apitest/internal/runner"
	_ "github.com/peterlindqvist/apitest/internal/runner/distributed"
	_ "github.com/peterlindqvist/apitest/internal/runservice"
	_ "github.com/peterlindqvist/apitest/internal/scaffold"
	_ "github.com/peterlindqvist/apitest/internal/signer"
	_ "github.com/peterlindqvist/apitest/internal/telemetry"
	_ "github.com/peterlindqvist/apitest/internal/variable"
	_ "github.com/peterlindqvist/apitest/internal/vault"
	_ "github.com/peterlindqvist/apitest/internal/vault/teamtemplate"
	_ "github.com/peterlindqvist/apitest/internal/websocket"
	_ "github.com/peterlindqvist/apitest/internal/websocket/templates"
	_ "github.com/peterlindqvist/apitest/internal/worker/schedule"
)

// internalRoot returns the absolute path to the internal/ directory of this
// project, computed from the test working directory (internal/errors).
func internalRoot(t *testing.T) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	// wd == .../internal/errors
	root := filepath.Dir(wd)
	if filepath.Base(root) != "internal" {
		t.Fatalf("unexpected test cwd %q (expected inside internal/errors)", wd)
	}
	return root
}

// discoverSentinels walks internal/*.go and returns a map of short package
// name (relative to internal/) to a sorted slice of exported Err* var names
// declared in that package. Test files are skipped.
func discoverSentinels(t *testing.T, internalDir string) map[string][]string {
	t.Helper()
	out := map[string]map[string]struct{}{}

	err := filepath.Walk(internalDir, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		if strings.HasSuffix(path, "_test.go") {
			return nil
		}

		fset := token.NewFileSet()
		f, parseErr := parser.ParseFile(fset, path, nil, parser.AllErrors)
		if parseErr != nil {
			t.Fatalf("parse %s: %v", path, parseErr)
		}

		// Package short name: path relative to internal/, minus filename.
		rel, relErr := filepath.Rel(internalDir, filepath.Dir(path))
		if relErr != nil {
			t.Fatalf("rel %s: %v", path, relErr)
		}
		rel = filepath.ToSlash(rel)
		if rel == "errors" {
			return nil // the errors package itself: no Err* declarations tracked here
		}

		for _, decl := range f.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.VAR {
				continue
			}
			for _, spec := range gen.Specs {
				v, ok := spec.(*ast.ValueSpec)
				if !ok {
					continue
				}
				for _, name := range v.Names {
					if !strings.HasPrefix(name.Name, "Err") {
						continue
					}
					if len(name.Name) < 4 || name.Name[3] < 'A' || name.Name[3] > 'Z' {
						continue
					}
					if out[rel] == nil {
						out[rel] = map[string]struct{}{}
					}
					out[rel][name.Name] = struct{}{}
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}

	res := make(map[string][]string, len(out))
	for pkg, set := range out {
		names := make([]string, 0, len(set))
		for n := range set {
			names = append(names, n)
		}
		sort.Strings(names)
		res[pkg] = names
	}
	return res
}

func TestCoverage_EverySentinelIsRegistered(t *testing.T) {
	root := internalRoot(t)
	discovered := discoverSentinels(t, root)
	registered := apierrors.RegisteredNames()

	var missing []string
	for pkg, names := range discovered {
		regSet := map[string]struct{}{}
		for _, n := range registered[pkg] {
			regSet[n] = struct{}{}
		}
		for _, n := range names {
			if _, ok := regSet[n]; !ok {
				missing = append(missing, pkg+"."+n)
			}
		}
	}

	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("unregistered sentinels (add a RegisterPackage entry in each package's hints_init.go):\n  %s",
			strings.Join(missing, "\n  "))
	}
}

func TestCoverage_EveryRegisteredHasCategoryAndCode(t *testing.T) {
	var bad []string
	for _, pkg := range apierrors.RegisteredPackages() {
		for _, entry := range apierrors.PackageRegistrations(pkg) {
			if entry.Hint.Category == "" {
				bad = append(bad, pkg+"."+entry.Name+" (missing Category)")
			}
			if entry.Hint.Code == "" {
				bad = append(bad, pkg+"."+entry.Name+" (missing Code)")
			}
		}
	}
	if len(bad) > 0 {
		sort.Strings(bad)
		t.Fatalf("registry entries missing Category/Code:\n  %s", strings.Join(bad, "\n  "))
	}
}

func TestCoverage_ClassifiedEntriesHaveHints(t *testing.T) {
	// Every sentinel registered with a non-internal Category should carry a
	// hint. The hint quality bar is part of the v0.1 design: empty hints are
	// only acceptable for CategoryInternal (platform/transient errors).
	var missing []string
	for _, pkg := range apierrors.RegisteredPackages() {
		for _, entry := range apierrors.PackageRegistrations(pkg) {
			if entry.Hint.Category == apierrors.CategoryInternal {
				continue
			}
			if strings.TrimSpace(entry.Hint.Hint) == "" {
				missing = append(missing, pkg+"."+entry.Name+" (Category="+string(entry.Hint.Category)+")")
			}
		}
	}
	if len(missing) > 0 {
		sort.Strings(missing)
		t.Fatalf("non-internal registrations missing Hint (every classified sentinel must name a concrete next action):\n  %s",
			strings.Join(missing, "\n  "))
	}
}

func TestCoverage_CodesAreUnique(t *testing.T) {
	// Stability promise: Codes are machine-readable identifiers that agents
	// may pattern-match against. Duplicates would mean two different error
	// semantics resolve to the same code.
	seen := map[string]string{}
	var dup []string
	for _, pkg := range apierrors.RegisteredPackages() {
		for _, entry := range apierrors.PackageRegistrations(pkg) {
			if entry.Hint.Code == "" {
				continue
			}
			key := entry.Hint.Code
			origin := pkg + "." + entry.Name
			if prev, ok := seen[key]; ok {
				dup = append(dup, key+" shared by "+prev+" and "+origin)
			} else {
				seen[key] = origin
			}
		}
	}
	if len(dup) > 0 {
		sort.Strings(dup)
		t.Fatalf("duplicate Codes:\n  %s", strings.Join(dup, "\n  "))
	}
}
