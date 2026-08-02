package uiserver

import (
	"encoding/json"
	"fmt"
	"mime"
	"net/http"
	"strconv"
	"time"
)

// handleRunStart serves POST /api/v1/runs (spec §4.7).
func (s *Server) handleRunStart(w http.ResponseWriter, r *http.Request) {
	var params StartParams
	if err := json.NewDecoder(r.Body).Decode(&params); err != nil {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "malformed JSON body: "+err.Error(), "", nil)
		return
	}
	runID, serr := s.orch.Start(params)
	if serr != nil {
		writeAPIError(w, serr.status, serr.code, serr.message, serr.hint, serr.details)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"run_id": runID, "state": "running"})
}

// handleRunCurrent serves GET /api/v1/runs/current (spec §4.8).
func (s *Server) handleRunCurrent(w http.ResponseWriter, r *http.Request) {
	active := s.orch.Current()
	if active == nil {
		writeJSON(w, http.StatusOK, map[string]any{"run": nil})
		return
	}
	total, passed, failed, skipped, errored, completed := active.Details.Progress()
	writeJSON(w, http.StatusOK, map[string]any{"run": map[string]any{
		"run_id":        active.RunID,
		"state":         active.State,
		"params":        active.Params,
		"started_at":    active.StartedAt.Format(time.RFC3339),
		"last_event_id": active.Log.LastID(),
		"progress": map[string]int{
			"total": total, "passed": passed, "failed": failed,
			"skipped": skipped, "error": errored, "completed": completed,
		},
	}})
}

// runRecord resolves a run id across active run, memory ring, and store.
type runRecord struct {
	source  string // "active" | "memory" | "store"
	state   string
	meta    *RunMeta
	summary *runSummaryJSON
	details *DetailCollector
	active  *ActiveRun
}

func (s *Server) findRun(runID string) *runRecord {
	if active := s.orch.Current(); active != nil && active.RunID == runID {
		return &runRecord{source: "active", state: active.State, details: active.Details, active: active}
	}
	if c := s.orch.FromRing(runID); c != nil {
		summary := c.Meta.Summary
		return &runRecord{source: "memory", state: c.State, meta: &c.Meta, summary: &summary, details: c.Details}
	}
	if s.store != nil {
		if meta, err := s.store.Meta(runID); err == nil {
			state := "completed"
			if meta.ExitStatus == "cancelled" || meta.ExitStatus == "error" {
				state = meta.ExitStatus
			}
			summary := meta.Summary
			rec := &runRecord{source: "store", state: state, meta: meta, summary: &summary}
			if d, err := s.store.LoadDetails(runID, s.opts.Root); err == nil {
				rec.details = d
			}
			return rec
		}
	}
	return nil
}

// handleRunGet serves GET /api/v1/runs/{run_id} (spec §4.8).
func (s *Server) handleRunGet(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if runID == "current" { // routed separately, defensive
		s.handleRunCurrent(w, r)
		return
	}
	rec := s.findRun(runID)
	if rec == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run "+runID, "", nil)
		return
	}
	resp := map[string]any{
		"run_id": runID,
		"state":  rec.state,
		"source": rec.source,
	}
	if rec.meta != nil {
		resp["meta"] = rec.meta
		resp["exit_status"] = rec.meta.ExitStatus
	}
	if rec.summary != nil {
		resp["summary"] = rec.summary
	} else if rec.active != nil {
		total, passed, failed, skipped, errored, _ := rec.active.Details.Progress()
		resp["summary"] = runSummaryJSON{
			Total: total, Passed: passed, Failed: failed, Skipped: skipped, Error: errored,
			Parallel: rec.active.Params.Parallel,
		}
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleRunCancel serves POST /api/v1/runs/{run_id}/cancel (spec §4.8).
func (s *Server) handleRunCancel(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	if !s.orch.Cancel(runID) {
		writeAPIError(w, http.StatusNotFound, "not_found", "not the active run", "", nil)
		return
	}
	writeJSON(w, http.StatusAccepted, map[string]string{"state": "cancelling"})
}

// handleRunRequests serves GET /api/v1/runs/{run_id}/requests — the light
// list driving the run view (spec §4.8). Returned for the active run too,
// seeded from the planner with outcome null.
func (s *Server) handleRunRequests(w http.ResponseWriter, r *http.Request) {
	rec := s.findRun(r.PathValue("run_id"))
	if rec == nil || rec.details == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"requests": rec.details.List()})
}

// handleRequestDetail serves GET /api/v1/runs/{run_id}/requests/{request_id}
// — the full inspector payload (spec §4.9). Works mid-run.
func (s *Server) handleRequestDetail(w http.ResponseWriter, r *http.Request) {
	rec := s.findRun(r.PathValue("run_id"))
	if rec == nil || rec.details == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run", "", nil)
		return
	}
	d := rec.details.Get(r.PathValue("request_id"))
	if d == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown request", "", nil)
		return
	}
	writeJSON(w, http.StatusOK, d)
}

// handleRequestBody serves
// GET /api/v1/runs/{run_id}/requests/{request_id}/body?which=request|response
// — the full redacted body bytes with original Content-Type (spec §4.10).
func (s *Server) handleRequestBody(w http.ResponseWriter, r *http.Request) {
	rec := s.findRun(r.PathValue("run_id"))
	if rec == nil || rec.details == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run", "", nil)
		return
	}
	which := r.URL.Query().Get("which")
	if which != "request" && which != "response" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "which must be request or response", "", nil)
		return
	}
	requestID := r.PathValue("request_id")
	raw, meta, ok := rec.details.RawBody(requestID, which)
	if !ok || raw == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "no such body", "", nil)
		return
	}
	d := rec.details.Get(requestID)
	contentType := meta.ContentType
	if contentType == "" {
		contentType = "application/octet-stream"
	}
	ext := extensionFor(contentType)
	w.Header().Set("Content-Type", contentType)
	w.Header().Set("Content-Disposition",
		fmt.Sprintf("attachment; filename=%q", d.Slug+"-"+which+ext))
	w.Header().Set("Content-Length", strconv.Itoa(len(raw)))
	_, _ = w.Write(raw)
}

// extensionFor guesses a download extension from the content type.
func extensionFor(contentType string) string {
	mt, _, err := mime.ParseMediaType(contentType)
	if err != nil {
		return ".bin"
	}
	switch mt {
	case "application/json":
		return ".json"
	case "text/plain":
		return ".txt"
	case "text/html":
		return ".html"
	case "application/xml", "text/xml":
		return ".xml"
	}
	if exts, err := mime.ExtensionsByType(mt); err == nil && len(exts) > 0 {
		return exts[0]
	}
	return ".bin"
}

// handleRunsList serves GET /api/v1/runs (spec §4.11). When config-disabled,
// an empty list is returned (history.enabled:false is already visible in
// /meta — spec §8.4).
func (s *Server) handleRunsList(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeJSON(w, http.StatusOK, map[string]any{"runs": []RunMeta{}, "total": 0})
		return
	}
	limit, offset := 50, 0
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if v := r.URL.Query().Get("offset"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n >= 0 {
			offset = n
		}
	}
	metas, total := s.store.List(limit, offset)
	if metas == nil {
		metas = []RunMeta{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"runs": metas, "total": total})
}

// handleRunDelete serves DELETE /api/v1/runs/{run_id} (spec §4.11).
func (s *Server) handleRunDelete(w http.ResponseWriter, r *http.Request) {
	if s.store == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "history is disabled (ui.history.enabled: false)", "", nil)
		return
	}
	if err := s.store.Delete(r.PathValue("run_id")); err != nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown persisted run", "", nil)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleRunEvents serves GET /api/v1/runs/{run_id}/events — the raw NDJSON
// stream. Free for ring runs; persisted runs are implicitly Solo (spec §4.11).
func (s *Server) handleRunEvents(w http.ResponseWriter, r *http.Request) {
	runID := r.PathValue("run_id")
	var lines [][]byte
	if active := s.orch.Current(); active != nil && active.RunID == runID {
		lines = active.Log.Lines()
	} else if c := s.orch.FromRing(runID); c != nil {
		lines = c.Log.Lines()
	} else if s.store != nil {
		if data, err := s.store.Events(runID); err == nil {
			w.Header().Set("Content-Type", "application/x-ndjson")
			_, _ = w.Write(data)
			return
		}
	}
	if lines == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run", "", nil)
		return
	}
	w.Header().Set("Content-Type", "application/x-ndjson")
	for _, line := range lines {
		_, _ = w.Write(line)
		_, _ = w.Write([]byte("\n"))
	}
}
