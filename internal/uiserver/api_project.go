package uiserver

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/weiqigod/curlew/internal/config"
	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/validator"
	"github.com/weiqigod/curlew/internal/variable"
)

// resolveInRoot jails a root-relative request path to the project root.
// Returns the absolute path, or "" when the path escapes the root.
func (s *Server) resolveInRoot(rel string) string {
	if rel == "" || filepath.IsAbs(rel) {
		return ""
	}
	clean := filepath.Clean(rel)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return ""
	}
	abs := filepath.Join(s.opts.Root, clean)
	if abs != s.opts.Root && !strings.HasPrefix(abs, s.opts.Root+string(filepath.Separator)) {
		return ""
	}
	return abs
}

// collectionPaths lists the project's collection files (root-relative),
// honoring the --collection filter.
func (s *Server) collectionPaths() []string {
	paths := config.ListCollections(s.opts.Root)
	if s.opts.CollectionFilter == "" {
		return paths
	}
	for _, p := range paths {
		if p == s.opts.CollectionFilter {
			return []string{p}
		}
	}
	return nil
}

// treeEtag hashes sorted (path, mtime, size) tuples over the collection set.
func (s *Server) treeEtag(paths []string) string {
	h := sha256.New()
	sorted := append([]string(nil), paths...)
	sort.Strings(sorted)
	for _, p := range sorted {
		abs := filepath.Join(s.opts.Root, p)
		var mtime int64
		var size int64
		if fi, err := os.Stat(abs); err == nil {
			mtime = fi.ModTime().UnixNano()
			size = fi.Size()
		}
		_, _ = fmt.Fprintf(h, "%s|%d|%d\n", p, mtime, size)
	}
	return hex.EncodeToString(h.Sum(nil))
}

type treeIssue struct {
	Severity string `json:"severity"`
	Line     int    `json:"line"`
	Message  string `json:"message"`
	Hint     string `json:"hint,omitempty"`
}

type treeRequest struct {
	Name       string `json:"name"`
	Slug       string `json:"slug"`
	Phase      string `json:"phase"`
	Method     string `json:"method"`
	URL        string `json:"url"` // raw uninterpolated template — never resolved values
	SourceLine int    `json:"source_line"`
	DataDriven bool   `json:"data_driven"`
	Required   bool   `json:"required"`
}

type treeCounts struct {
	Setup    int `json:"setup"`
	Main     int `json:"main"`
	Teardown int `json:"teardown"`
}

type treeCollection struct {
	Path     string        `json:"path"`
	Name     *string       `json:"name"`
	Valid    bool          `json:"valid"`
	Counts   *treeCounts   `json:"counts"`
	Requests []treeRequest `json:"requests"`
	Issues   []treeIssue   `json:"issues"`
}

func issuesFromValidator(issues []validator.Issue) []treeIssue {
	out := make([]treeIssue, 0, len(issues))
	for _, is := range issues {
		out = append(out, treeIssue{
			Severity: is.Severity.String(),
			Line:     is.Line,
			Message:  is.Message,
			Hint:     is.Hint,
		})
	}
	return out
}

// handleTree serves GET /api/v1/tree (spec §4.3). Files are parsed without
// Read-only rendering of the user's own files
// must never gate-block; gates are enforced at run time exactly as the CLI.
func (s *Server) handleTree(w http.ResponseWriter, r *http.Request) {
	paths := s.collectionPaths()
	collections := make([]treeCollection, 0, len(paths))
	for _, rel := range paths {
		abs := filepath.Join(s.opts.Root, rel)
		col, err := parser.ParseFile(abs)
		if err != nil {
			entry := treeCollection{Path: rel, Valid: false, Requests: []treeRequest{}, Issues: []treeIssue{}}
			if res := validator.ValidateAuto(abs, nil); res != nil {
				entry.Issues = issuesFromValidator(res.Issues)
			}
			collections = append(collections, entry)
			continue
		}
		name := col.Name
		entry := treeCollection{
			Path:  rel,
			Name:  &name,
			Valid: true,
			Counts: &treeCounts{
				Setup:    len(col.Setup.Items),
				Main:     len(col.Requests.Items),
				Teardown: len(col.Teardown.Items),
			},
			Requests: []treeRequest{},
			Issues:   []treeIssue{},
		}
		appendPhase := func(items []parser.RequestItem, phase string) {
			for _, item := range items {
				entry.Requests = append(entry.Requests, treeRequest{
					Name:       item.Name,
					Slug:       item.Slug,
					Phase:      phase,
					Method:     item.Request.Method,
					URL:        item.Request.URL,
					SourceLine: item.SourceLine,
					DataDriven: item.DataDriven != nil,
					Required:   item.Required != nil && *item.Required,
				})
			}
		}
		appendPhase(col.Setup.Items, "setup")
		appendPhase(col.Requests.Items, "main")
		appendPhase(col.Teardown.Items, "teardown")
		collections = append(collections, entry)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"etag":        s.treeEtag(paths),
		"collections": collections,
	})
}

type envVariable struct {
	Name      string `json:"name"`
	Value     string `json:"value"`
	Sensitive bool   `json:"sensitive"`
}

type envEntry struct {
	Name      string        `json:"name"`
	File      string        `json:"file"`
	Variables []envVariable `json:"variables"`
}

// handleEnvironments serves GET /api/v1/environments (spec §4.4). Values pass
// the redaction layer with a SensitiveSet built from heuristic name patterns
// plus the project's configured secret names. .env contents are never served.
func (s *Server) handleEnvironments(w http.ResponseWriter, r *http.Request) {
	names := config.ListAvailableEnvironments(s.opts.Root)
	projectCfg, _, _ := config.LoadProjectConfig(s.opts.Root)

	envs := make([]envEntry, 0, len(names))
	for _, name := range names {
		vars, err := config.LoadEnvironment(name, s.opts.Root)
		if err != nil {
			continue // unreadable env files are simply absent from the menu
		}
		sens := variable.NewSensitiveSet()
		sens.AddHeuristicNames(vars)
		if projectCfg != nil && projectCfg.Secrets != nil {
			sens.Merge(projectCfg.Secrets.SensitiveNames())
		}
		file := filepath.Join("environments", name+".yaml")
		if _, err := os.Stat(filepath.Join(s.opts.Root, file)); err != nil {
			alt := filepath.Join("environments", name+".yml")
			if _, err := os.Stat(filepath.Join(s.opts.Root, alt)); err == nil {
				file = alt
			}
		}
		keys := make([]string, 0, len(vars))
		for k := range vars {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		vlist := make([]envVariable, 0, len(keys))
		for _, k := range keys {
			sensitive := sens.IsSensitive(k)
			vlist = append(vlist, envVariable{
				Name:      k,
				Value:     variable.RedactValue(k, vars[k], sens, false),
				Sensitive: sensitive,
			})
		}
		envs = append(envs, envEntry{Name: name, File: file, Variables: vlist})
	}
	writeJSON(w, http.StatusOK, map[string]any{"environments": envs})
}

type validateFile struct {
	File   string      `json:"file"`
	Valid  bool        `json:"valid"`
	Issues []treeIssue `json:"issues"`
}

// handleValidate serves GET /api/v1/validate?path= (spec §4.5).
func (s *Server) handleValidate(w http.ResponseWriter, r *http.Request) {
	var rels []string
	if p := r.URL.Query().Get("path"); p != "" {
		if s.resolveInRoot(p) == "" {
			writeAPIError(w, http.StatusBadRequest, "bad_request", "path outside project root", "", nil)
			return
		}
		rels = []string{filepath.Clean(p)}
	} else {
		rels = s.collectionPaths()
	}
	allValid := true
	files := make([]validateFile, 0, len(rels))
	for _, rel := range rels {
		abs := filepath.Join(s.opts.Root, rel)
		res := validator.ValidateAuto(abs, nil)
		vf := validateFile{File: rel, Valid: true, Issues: []treeIssue{}}
		if res != nil {
			vf.Valid = res.Valid
			vf.Issues = issuesFromValidator(res.Issues)
		}
		if !vf.Valid {
			allValid = false
		}
		files = append(files, vf)
	}
	writeJSON(w, http.StatusOK, map[string]any{"valid": allValid, "files": files})
}

// handleFiles serves GET /api/v1/files?path= (spec §4.6): read-only fetch for
// the inspector's view-source affordance. Jailed to the project root,
// .yaml/.yml only, 1 MiB cap, raw text/plain.
func (s *Server) handleFiles(w http.ResponseWriter, r *http.Request) {
	rel := r.URL.Query().Get("path")
	abs := s.resolveInRoot(rel)
	if abs == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "path outside project root", "", nil)
		return
	}
	ext := filepath.Ext(abs)
	if ext != ".yaml" && ext != ".yml" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "only .yaml/.yml files are served", "", nil)
		return
	}
	fi, err := os.Stat(abs)
	if err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "file not found", "", nil)
		return
	}
	if fi.Size() > 1024*1024 {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "file exceeds the 1 MiB limit", "", nil)
		return
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		writeAPIError(w, http.StatusInternalServerError, "internal", "reading file failed", "", nil)
		return
	}
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	_, _ = w.Write(data)
}
