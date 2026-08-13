package main

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"sync"
	"testing"

	"github.com/weiqigod/curlew/internal/docs"
)

// §4.3's glob tokens and §4.4's file-reference table.
//
// Both are about where the CLI looks for a file, which is the kind of claim
// that is never wrong in the common case and often wrong in the one that
// matters: a glob token that quietly crosses a directory boundary, or a
// reference that resolves against the working directory instead of the
// collection's. So every §4.4 row is run with a decoy of the same name sitting
// in the working directory. Resolving from the wrong place then finds a file
// and uses it, which is exactly the failure a "file not found" test cannot see.

// pathRecorder collects the request paths and bodies a run produced.
type pathRecorder struct {
	mu     sync.Mutex
	paths  []string
	bodies []string
}

func (p *pathRecorder) server(t *testing.T) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		buf := make([]byte, 4096)
		n, _ := r.Body.Read(buf)
		p.mu.Lock()
		p.paths = append(p.paths, r.URL.Path)
		p.bodies = append(p.bodies, string(buf[:n]))
		p.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"id":"ok","name":"ok"}`))
	}))
}

func (p *pathRecorder) sortedPaths() []string {
	p.mu.Lock()
	defer p.mu.Unlock()
	out := append([]string{}, p.paths...)
	sort.Strings(out)
	return out
}

func (p *pathRecorder) allBodies() string {
	p.mu.Lock()
	defer p.mu.Unlock()
	return strings.Join(p.bodies, "\n")
}

// ---------------------------------------------------------------- §4.3 globs

// globCase is one token's discriminating pattern: what it must match, and what
// it must not. A token is only proven by the second half — `*` that also
// crossed a directory boundary would satisfy any "matches these" check.
type globCase struct {
	pattern string
	hits    []string
	misses  []string
}

// globCases are keyed by the token as the table spells it.
var globCases = map[string]globCase{
	"*":     {"a*.yaml", []string{"ab", "abc"}, []string{"sub-ab"}},
	"?":     {"a?.yaml", []string{"ab"}, []string{"abc"}},
	"[abc]": {"[bc].yaml", []string{"b", "c"}, []string{"ab", "x"}},
	"**":    {"sub/**/x.yaml", []string{"deep-x", "deeper-x"}, []string{"x"}},
}

// globTree is every collection the glob fixture contains: name → relative path.
var globTree = map[string]string{
	"ab":       "ab.yaml",
	"abc":      "abc.yaml",
	"b":        "b.yaml",
	"c":        "c.yaml",
	"x":        "x.yaml",
	"sub-ab":   "sub/ab.yaml",
	"deep-x":   "sub/deep/x.yaml",
	"deeper-x": "sub/deep/nest/x.yaml",
}

func writeGlobTree(t *testing.T, dir, base string) {
	t.Helper()
	for name, rel := range globTree {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		body := fmt.Sprintf("name: %s\nrequests:\n  - name: One\n    request:\n      method: GET\n      url: \"%s/hit/%s\"\n",
			name, base, name)
		if err := os.WriteFile(full, []byte(body), 0o600); err != nil {
			t.Fatalf("write %s: %v", rel, err)
		}
	}
}

func TestDocTables_globTokensMatchWhatTheyClaim(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Token", "Meaning")
	if err != nil {
		t.Fatalf("glob table: %v", err)
	}
	tokenCol := docs.Column(hdr, "Token")
	if tokenCol < 0 {
		t.Fatalf("glob table lost its Token column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= tokenCol {
			continue
		}
		token := docs.FirstName(row[tokenCol])
		gc, ok := globCases[token]
		if !ok {
			t.Errorf("the glob table documents token %q, which no case exercises; write one rather "+
				"than leaving the token unproven", token)
			continue
		}
		seen++

		t.Run(token, func(t *testing.T) {
			rec := &pathRecorder{}
			srv := rec.server(t)
			defer srv.Close()

			dir := t.TempDir()
			writeGlobTree(t, dir, srv.URL)

			cmd := exec.Command(bin, "run", gc.pattern)
			cmd.Dir = dir
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			out, runErr := cmd.CombinedOutput()
			if runErr != nil {
				t.Fatalf("run %q: %v\n%s", gc.pattern, runErr, out)
			}

			got := map[string]bool{}
			for _, p := range rec.sortedPaths() {
				got[strings.TrimPrefix(p, "/hit/")] = true
			}
			for _, want := range gc.hits {
				if !got[want] {
					t.Errorf("%q means %q, and pattern %q did not match %s",
						token, row[tokenCol+1], gc.pattern, globTree[want])
				}
			}
			for _, never := range gc.misses {
				if got[never] {
					t.Errorf("%q means %q, and pattern %q also matched %s",
						token, row[tokenCol+1], gc.pattern, globTree[never])
				}
			}
		})
	}
	if seen == 0 {
		t.Fatal("no glob tokens read from the table")
	}
}

// ------------------------------------------------------ §4.4 file references

// referenceShape recognises the field names §4.4's left column lists.
var referenceShape = regexp.MustCompile(`[a-z_]+(?:\.[a-z_]+)?:`)

// refFixture writes one kind of reference two ways: the real file beside the
// collection, and a decoy of the same relative name in the working directory.
// A resolver that looks in the wrong place finds the decoy rather than nothing.
type refFixture struct {
	// build writes the collection at collectionDir and returns its path. The
	// referenced file is written at both collectionDir and cwd, carrying marker
	// and decoyMarker respectively.
	build func(t *testing.T, collectionDir, cwd, base string) string
	// found reports whether the run used the file beside the collection.
	// Nil when the proof is the exit code instead: a schema is not visible on
	// the wire, so the decoy is written to reject the response and the run
	// passing is what shows which file was compiled.
	found func(rec *pathRecorder) bool
	// args are extra arguments the run needs.
	args []string
}

const (
	realMarker  = "BESIDE_THE_COLLECTION"
	decoyMarker = "BESIDE_THE_WORKING_DIRECTORY"
)

func writeBoth(t *testing.T, collectionDir, cwd, rel, real, decoy string) {
	t.Helper()
	for dir, content := range map[string]string{collectionDir: real, cwd: decoy} {
		full := filepath.Join(dir, rel)
		if err := os.MkdirAll(filepath.Dir(full), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(full, []byte(content), 0o600); err != nil {
			t.Fatalf("write %s: %v", full, err)
		}
	}
}

func writeCollectionFile(t *testing.T, dir, body string) string {
	t.Helper()
	path := filepath.Join(dir, "c.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("write collection: %v", err)
	}
	return path
}

func markerInPaths(rec *pathRecorder) bool {
	return strings.Contains(strings.Join(rec.sortedPaths(), " "), realMarker)
}

func markerInBodies(rec *pathRecorder) bool {
	return strings.Contains(rec.allBodies(), realMarker)
}

// refFixtures maps each row of §4.4 to a way of exercising it.
var refFixtures = map[string]refFixture{
	"path:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "req.yaml",
				fmt.Sprintf("name: Ext\nrequest:\n  method: GET\n  url: \"%s/%s\"\n", base, realMarker),
				fmt.Sprintf("name: Ext\nrequest:\n  method: GET\n  url: \"%s/%s\"\n", base, decoyMarker))
			return writeCollectionFile(t, collectionDir,
				"name: refs\nrequests:\n  - path: \"./req.yaml\"\n")
		},
		found: markerInPaths,
	},
	"include:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "inc.yaml",
				fmt.Sprintf("name: Inc\nrequests:\n  - name: I\n    request:\n      method: GET\n      url: \"%s/%s\"\n", base, realMarker),
				fmt.Sprintf("name: Inc\nrequests:\n  - name: I\n    request:\n      method: GET\n      url: \"%s/%s\"\n", base, decoyMarker))
			return writeCollectionFile(t, collectionDir,
				"name: refs\ninclude:\n  - \"./inc.yaml\"\n")
		},
		found: markerInPaths,
	},
	"body_file:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "body.txt", realMarker, decoyMarker)
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
					"      url: \"%s/b\"\n      body_file: \"./body.txt\"\n", base))
		},
		found: markerInBodies,
	},
	"body_binary_file:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "raw.bin", realMarker, decoyMarker)
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
					"      url: \"%s/b\"\n      body_binary_file: \"./raw.bin\"\n", base))
		},
		found: markerInBodies,
	},
	"assertions.schema:": {
		// The schema beside the collection accepts the response; the decoy
		// demands a property it does not have. A run that passes used the
		// former, and one that used the decoy fails its assertion.
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "s.json",
				`{"type":"object","required":["id"]}`,
				`{"type":"object","required":["absent_on_purpose"]}`)
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: GET\n"+
					"      url: \"%s/one\"\n    assertions:\n      schema: \"./s.json\"\n", base))
		},
	},
	"data_driven.source:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "d.csv",
				"where\n"+realMarker+"\n", "where\n"+decoyMarker+"\n")
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    data_driven:\n      source: \"./d.csv\"\n"+
					"    request:\n      method: GET\n      url: \"%s/{{where}}\"\n", base))
		},
		found: markerInPaths,
	},
	"graphql.query_file:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "q.graphql",
				"query { "+realMarker+" }", "query { "+decoyMarker+" }")
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
					"      url: \"%s/gql\"\n      protocol: graphql\n      graphql:\n        query_file: \"./q.graphql\"\n", base))
		},
		found: markerInBodies,
	},
	"graphql.fragments:": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, "f.graphql",
				"fragment F on T { "+realMarker+" }", "fragment F on T { "+decoyMarker+" }")
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: POST\n"+
					"      url: \"%s/gql\"\n      protocol: graphql\n      graphql:\n"+
					"        query: \"query { a }\"\n        fragments:\n          - \"./f.graphql\"\n", base))
		},
		found: markerInBodies,
	},
	"environments/": {
		build: func(t *testing.T, collectionDir, cwd, base string) string {
			writeBoth(t, collectionDir, cwd, filepath.Join("environments", "e.yaml"),
				"variables:\n  where: \""+realMarker+"\"\n",
				"variables:\n  where: \""+decoyMarker+"\"\n")
			return writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: refs\nrequests:\n  - name: One\n    request:\n      method: GET\n"+
					"      url: \"%s/{{where}}\"\n", base))
		},
		found: markerInPaths,
		args:  []string{"--env", "e"},
	},
}

func TestDocTables_fileReferencesResolveWhereTheTableSays(t *testing.T) {
	bin := buildBinary(t)

	hdr, rows, err := docs.Table("CLI_SPECIFICATION.md", "Reference", "Resolved relative to")
	if err != nil {
		t.Fatalf("file reference table: %v", err)
	}
	refCol := docs.Column(hdr, "Reference")
	if refCol < 0 {
		t.Fatalf("file reference table lost its Reference column: %v", hdr)
	}

	seen := 0
	for _, row := range rows {
		if len(row) <= refCol {
			continue
		}
		names := referenceShape.FindAllString(row[refCol], -1)
		if strings.Contains(row[refCol], "environments/") {
			names = append(names, "environments/")
		}
		if len(names) == 0 {
			t.Errorf("file-reference row %q names no reference", row[refCol])
			continue
		}

		for _, name := range names {
			fx, ok := refFixtures[name]
			if !ok {
				t.Errorf("§4.4 lists %q, which no fixture exercises; write one rather than leaving "+
					"the resolution rule unproven", name)
				continue
			}
			seen++

			t.Run(strings.Trim(name, ":/"), func(t *testing.T) {
				rec := &pathRecorder{}
				srv := rec.server(t)
				defer srv.Close()

				root := t.TempDir()
				collectionDir := filepath.Join(root, "collections")
				cwd := filepath.Join(root, "elsewhere")
				for _, d := range []string{collectionDir, cwd} {
					if mkErr := os.MkdirAll(d, 0o700); mkErr != nil {
						t.Fatalf("mkdir: %v", mkErr)
					}
				}

				collection := fx.build(t, collectionDir, cwd, srv.URL)

				args := append([]string{"run", collection}, fx.args...)
				cmd := exec.Command(bin, args...)
				cmd.Dir = cwd
				cmd.Env = append(os.Environ(), "NO_COLOR=1")
				out, runErr := cmd.CombinedOutput()

				if fx.found == nil {
					// The decoy is written to reject the response, so a passing
					// run is the proof and a failing one names the decoy.
					if runErr != nil {
						t.Errorf("§4.4 says %s resolves relative to the collection's directory. The run "+
							"did not pass, which is what the copy beside the working directory is written "+
							"to cause:\n%s", name, out)
					}
					return
				}
				if runErr != nil {
					t.Fatalf("run with %s: %v\n%s", name, runErr, out)
				}
				if !fx.found(rec) {
					t.Errorf("§4.4 says %s resolves relative to the collection's directory; the run "+
						"used the copy beside the working directory instead.\npaths: %v\nbodies: %s\n%s",
						name, rec.sortedPaths(), rec.allBodies(), out)
				}
			})
		}
	}
	if seen == 0 {
		t.Fatal("no references read from §4.4")
	}
}

// "Collection directory, then project root" is two rules in one row, and the
// second is the one nobody notices is missing: a project-wide environment file
// is the ordinary way to keep one, and it sits nowhere near the collection.
func TestDocTables_environmentsResolveCollectionDirThenProjectRoot(t *testing.T) {
	bin := buildBinary(t)

	// The row must still say both, or this test is checking a rule the
	// specification has dropped.
	_, rows, err := docs.Table("CLI_SPECIFICATION.md", "Reference", "Resolved relative to")
	if err != nil {
		t.Fatalf("file reference table: %v", err)
	}
	said := false
	for _, row := range rows {
		joined := strings.ToLower(strings.Join(row, " "))
		if strings.Contains(joined, "environments/") &&
			strings.Contains(joined, "collection") && strings.Contains(joined, "project root") {
			said = true
		}
	}
	if !said {
		t.Fatal("§4.4 no longer says environment files fall back to the project root")
	}

	for _, tc := range []struct {
		name       string
		alsoBeside bool
		want       string
	}{
		{"only at the project root", false, "PROJECT_ROOT"},
		{"beside the collection wins", true, "COLLECTION_DIR"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rec := &pathRecorder{}
			srv := rec.server(t)
			defer srv.Close()

			root := t.TempDir()
			collectionDir := filepath.Join(root, "collections")
			cwd := filepath.Join(root, "elsewhere")
			for _, d := range []string{collectionDir, cwd, filepath.Join(root, "environments")} {
				if mkErr := os.MkdirAll(d, 0o700); mkErr != nil {
					t.Fatalf("mkdir: %v", mkErr)
				}
			}
			// curlew.yaml is what makes root the project root.
			if wErr := os.WriteFile(filepath.Join(root, "curlew.yaml"),
				[]byte("project_name: envs\n"), 0o600); wErr != nil {
				t.Fatalf("write curlew.yaml: %v", wErr)
			}
			if wErr := os.WriteFile(filepath.Join(root, "environments", "e.yaml"),
				[]byte("variables:\n  where: \"PROJECT_ROOT\"\n"), 0o600); wErr != nil {
				t.Fatalf("write project environment: %v", wErr)
			}
			if tc.alsoBeside {
				if mkErr := os.MkdirAll(filepath.Join(collectionDir, "environments"), 0o700); mkErr != nil {
					t.Fatalf("mkdir: %v", mkErr)
				}
				if wErr := os.WriteFile(filepath.Join(collectionDir, "environments", "e.yaml"),
					[]byte("variables:\n  where: \"COLLECTION_DIR\"\n"), 0o600); wErr != nil {
					t.Fatalf("write collection environment: %v", wErr)
				}
			}

			collection := writeCollectionFile(t, collectionDir, fmt.Sprintf(
				"name: envs\nrequests:\n  - name: One\n    request:\n      method: GET\n      url: \"%s/{{where}}\"\n",
				srv.URL))

			cmd := exec.Command(bin, "run", collection, "--env", "e")
			cmd.Dir = cwd
			cmd.Env = append(os.Environ(), "NO_COLOR=1")
			out, runErr := cmd.CombinedOutput()
			if runErr != nil {
				t.Fatalf("run: %v\n%s", runErr, out)
			}
			if got := rec.sortedPaths(); len(got) != 1 || got[0] != "/"+tc.want {
				t.Errorf("§4.4 says environments resolve in the collection directory then the project "+
					"root; the request went to %v, want /%s", got, tc.want)
			}
		})
	}
}
