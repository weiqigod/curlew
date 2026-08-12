// Package testapi holds the anti-bloat parity test (§16).
//
// The test derives both sides of the comparison from the artefacts themselves —
// the endpoint registry from the server, the URLs from the dogfood collections —
// so an endpoint nobody calls, or a collection URL that hits nothing, fails the
// build rather than sitting unnoticed.
//
// It deliberately does not import any curlew package. §13.1 forbids it: if
// curlew's own code were what proved mudflat correct, the loop this whole
// directory exists to break would close again from the other side. The YAML
// parse below is a dozen lines against gopkg.in/yaml.v3 rather than a call into
// internal/parser, for exactly that reason.
package testapi

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/weiqigod/curlew/testapi/mudflat"
)

// collectionDirs are searched for request URLs. gaps/ holds requests that
// document a curlew defect and are not expected to pass; they still count as
// coverage, because the endpoint they exercise is doing its job.
var collectionDirs = []string{"collections", "gaps"}

// allEndpoints merges the structured and raw registries. The parity rule is
// about mudflat's whole surface; how many listeners implement it is an
// implementation detail the rule should not have to know.
func allEndpoints() []mudflat.IndexEntry {
	structured := mudflat.New(mudflat.Options{}).Index()
	raw := mudflat.NewRaw(mudflat.RawOptions{}).Index()
	return append(structured, raw...)
}

// loadGoldens returns the set of golden transcript names present on disk.
func loadGoldens(t *testing.T) map[string]bool {
	t.Helper()

	out := map[string]bool{}
	matches, err := filepath.Glob(filepath.Join(repoRelative(t, "golden"), "raw", "*.txt"))
	if err != nil {
		t.Fatalf("glob goldens: %v", err)
	}
	for _, file := range matches {
		out[strings.TrimSuffix(filepath.Base(file), ".txt")] = true
	}
	return out
}

// goldenName maps an endpoint pattern to its transcript file name, matching
// goldenFile in raw_test.go.
func goldenName(pattern string) string {
	return strings.ReplaceAll(strings.TrimPrefix(pattern, "/raw/"), "/", "-")
}

// coveredByTest lists endpoints whose only possible coverage is a Go test, with
// the reason. Each entry is a deliberate exemption from §16, not an oversight.
var coveredByTest = map[string]string{
	"/raw/reset-after/{n}": "the response is cut mid-flight, so its bytes depend on TCP timing rather than on what the server wrote; TestRaw_ResetAfterNBytes asserts the reset instead",
}

// intentionallyUnrouted lists paths a collection requests on purpose without
// any endpoint behind them. Each needs a reason: the default assumption for an
// unresolvable URL is a typo, and a typo becomes a 404 that reads like a curlew
// bug rather than a collection bug.
var intentionallyUnrouted = map[string]string{
	"/no-such-endpoint-exists": "00-smoke asserts that an unknown path is a clean 404",
}

type collectionFile struct {
	Name     string `yaml:"name"`
	Requests []struct {
		Name    string `yaml:"name"`
		Request struct {
			Method   string `yaml:"method"`
			URL      string `yaml:"url"`
			Protocol string `yaml:"protocol"`
		} `yaml:"request"`
	} `yaml:"requests"`
}

type usage struct {
	method string
	path   string
	source string
}

func TestParity_EveryEndpointIsExercised(t *testing.T) {
	endpoints := allEndpoints()
	uses := loadUsages(t)
	goldens := loadGoldens(t)

	if len(uses) == 0 {
		t.Fatal("no request URLs found; the parity test would pass vacuously")
	}

	var orphans []string
	for _, ep := range endpoints {
		if anyUsageMatches(ep, uses) {
			continue
		}
		// §16 accepts either form of coverage. Most raw endpoints cannot appear
		// in a collection at all — their responses are unparseable by design —
		// so a golden transcript is how they prove they are exercised.
		if goldens[goldenName(ep.Pattern)] {
			continue
		}
		if reason, ok := coveredByTest[ep.Pattern]; ok {
			t.Logf("%s: covered by a Go test (%s)", ep.Pattern, reason)
			continue
		}
		orphans = append(orphans, fmt.Sprintf("  %-34s %v  (%s)", ep.Pattern, ep.Methods, ep.Summary))
	}

	if len(orphans) > 0 {
		sort.Strings(orphans)
		t.Errorf("%d endpoint(s) are not exercised by any dogfood collection.\n"+
			"Either add a request that uses them, or delete them — §16 keeps the\n"+
			"endpoint set tied to what the suite actually calls:\n%s",
			len(orphans), strings.Join(orphans, "\n"))
	}
}

func TestParity_EveryCollectionURLResolves(t *testing.T) {
	endpoints := allEndpoints()
	uses := loadUsages(t)

	var unresolved []string
	for _, use := range uses {
		if reason, ok := intentionallyUnrouted[use.path]; ok {
			t.Logf("skipping %s: %s", use.path, reason)
			continue
		}
		matched := false
		for _, ep := range endpoints {
			if endpointMatches(ep, use) {
				matched = true
				break
			}
		}
		if !matched {
			unresolved = append(unresolved,
				fmt.Sprintf("  %s %s\n    in %s", use.method, use.path, use.source))
		}
	}

	if len(unresolved) > 0 {
		t.Errorf("%d request URL(s) do not resolve to a documented endpoint.\n"+
			"A typo here becomes a 404 that looks like a curlew bug:\n%s",
			len(unresolved), strings.Join(unresolved, "\n"))
	}
}

func TestParity_EveryEndpointCitesACurlewBehaviour(t *testing.T) {
	// §P3. The citation is what stops the endpoint set from drifting into a
	// museum of HTTP trivia.
	for _, ep := range allEndpoints() {
		if strings.TrimSpace(ep.Exercises) == "" {
			t.Errorf("endpoint %s has no Exercises citation", ep.Pattern)
		}
		if strings.TrimSpace(ep.Summary) == "" {
			t.Errorf("endpoint %s has no Summary", ep.Pattern)
		}
	}
}

func TestParity_NoCurlewImportsUnderTestapi(t *testing.T) {
	// §13.1, enforced here as well as in ci-local.sh so a developer running
	// `go test ./testapi/...` finds out immediately.
	root := repoRelative(t, ".")

	err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() || !strings.HasSuffix(path, ".go") {
			return nil
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		// Assembled at run time rather than written as literals: spelling them
		// out here made this guard flag its own source file, which is a real
		// enough failure mode that the workaround is worth a comment.
		modulePath := "github.com/weiqigod/" + "curlew/"
		for _, banned := range []string{
			`"` + modulePath + "internal/",
			`"` + modulePath + "cmd/",
		} {
			if strings.Contains(string(src), banned) {
				t.Errorf("%s imports a curlew package (%s…).\n"+
					"§13.1: nothing under testapi/ may depend on curlew, or curlew's own\n"+
					"code becomes what proves the test API correct.", path, banned)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
}

// loadUsages collects every (method, path) a dogfood collection requests.
func loadUsages(t *testing.T) []usage {
	t.Helper()

	var out []usage
	for _, dir := range collectionDirs {
		root := repoRelative(t, dir)
		if _, err := os.Stat(root); os.IsNotExist(err) {
			continue
		}

		// Walk rather than glob: collections/parallel/ holds the requests that
		// only pass under --parallel, and they still count as coverage.
		var matches []string
		err := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if !d.IsDir() && strings.HasSuffix(path, ".yaml") {
				matches = append(matches, path)
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", root, err)
		}
		sort.Strings(matches)

		for _, file := range matches {
			raw, err := os.ReadFile(file)
			if err != nil {
				t.Fatalf("read %s: %v", file, err)
			}
			var parsed collectionFile
			if err := yaml.Unmarshal(raw, &parsed); err != nil {
				t.Fatalf("parse %s: %v", file, err)
			}
			for _, req := range parsed.Requests {
				if req.Request.URL == "" {
					continue
				}
				method := strings.ToUpper(req.Request.Method)
				if method == "" {
					// A graphql request carries no method: curlew sets POST
					// and the Content-Type itself, which is exactly the
					// behaviour §9.K checks. Defaulting it to GET here would
					// report every GraphQL request as an unresolved URL.
					method = defaultMethodFor(req.Request.Protocol)
				}
				out = append(out, usage{
					method: method,
					path:   pathOf(req.Request.URL),
					source: fmt.Sprintf("%s → %q", filepath.Base(file), req.Name),
				})
			}
		}
	}
	return out
}

// pathOf reduces a collection URL to its path. The leading {{var}} that carries
// the host is dropped, and the query string is discarded.
//
// This is why collections write full paths after a single base variable: a URL
// assembled from several variables cannot be resolved without evaluating them,
// and evaluating them would mean importing curlew.
func pathOf(url string) string {
	path := url
	if idx := strings.Index(path, "}}"); idx >= 0 && strings.HasPrefix(strings.TrimSpace(path), "{{") {
		path = path[idx+2:]
	}
	if idx := strings.IndexAny(path, "?#"); idx >= 0 {
		path = path[:idx]
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	return path
}

func anyUsageMatches(ep mudflat.IndexEntry, uses []usage) bool {
	for _, use := range uses {
		if endpointMatches(ep, use) {
			return true
		}
	}
	return false
}

func endpointMatches(ep mudflat.IndexEntry, use usage) bool {
	if !methodAllowed(ep, use.method) {
		return false
	}
	if pathMatches(ep.Pattern, use.path) {
		return true
	}
	// Stateless endpoints are mounted under a session prefix as well, so a
	// collection may reach /echo as /s/{sid}/echo.
	if ep.SessionOptional && pathMatches("/s/{sid}"+ep.Pattern, use.path) {
		return true
	}
	return false
}

func methodAllowed(ep mudflat.IndexEntry, method string) bool {
	for _, m := range ep.Methods {
		if m == "ANY" || m == method {
			return true
		}
	}
	return false
}

// pathMatches compares a ServeMux-style pattern against a concrete path.
// A {name} segment matches one segment; {name...} matches the rest; a {{var}}
// in the concrete path is a placeholder and matches any single segment.
func pathMatches(pattern, path string) bool {
	// "/{$}" is Go's exact-root pattern.
	if pattern == "/{$}" {
		return path == "/"
	}

	patternParts := splitPath(pattern)
	pathParts := splitPath(path)

	for i, want := range patternParts {
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "...}") {
			return len(pathParts) >= i
		}
		if i >= len(pathParts) {
			return false
		}
		got := pathParts[i]
		if strings.HasPrefix(want, "{") && strings.HasSuffix(want, "}") {
			continue
		}
		if strings.Contains(got, "{{") {
			// A variable placeholder in the collection: it cannot be compared
			// to a literal segment, so treat it as a wildcard.
			continue
		}
		if want != got {
			return false
		}
	}
	return len(patternParts) == len(pathParts)
}

func splitPath(p string) []string {
	trimmed := strings.Trim(p, "/")
	if trimmed == "" {
		return nil
	}
	return strings.Split(trimmed, "/")
}

// repoRelative resolves a path under testapi/, independent of where the test
// binary was started from.
func repoRelative(t *testing.T, rel string) string {
	t.Helper()
	wd, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	return filepath.Join(wd, rel)
}

// defaultMethodFor returns the method curlew uses when a request declares none.
// http requests default to GET; a graphql request is always a POST, chosen by
// the adapter rather than by the collection author.
func defaultMethodFor(protocol string) string {
	if strings.EqualFold(protocol, "graphql") {
		return "POST"
	}
	return "GET"
}
