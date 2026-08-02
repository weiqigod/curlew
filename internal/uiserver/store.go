package uiserver

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// Store is the persisted run-history store under <root>/.apitest/ui/ (spec
// §8). Constructed only when config has
// not disabled it; below Solo nothing is ever written to .apitest/ui/.
type Store struct {
	mu      sync.Mutex
	dir     string // <root>/.apitest/ui
	maxRuns int
}

// storedDetail is the on-disk detail.json entry: the §4.9 shape plus the
// raw redacted bodies (capped at StoredBodyLimit each, base64).
type storedDetail struct {
	RequestDetail
	RawRequestBody  string `json:"raw_request_body,omitempty"`  // base64, ≤ 1 MiB
	RawResponseBody string `json:"raw_response_body,omitempty"` // base64, ≤ 1 MiB
}

// newStore creates the store directory and its self-ignoring .gitignore.
// The .gitignore is required, not belt-and-braces: the project scaffolder
// only ignores .apitest/ when --skill was used (spec §8.1).
func newStore(root string, maxRuns int) (*Store, error) {
	dir := filepath.Join(root, ".apitest", "ui")
	if err := os.MkdirAll(filepath.Join(dir, "runs"), 0o755); err != nil {
		return nil, err
	}
	gi := filepath.Join(dir, ".gitignore")
	if _, err := os.Stat(gi); os.IsNotExist(err) {
		if err := os.WriteFile(gi, []byte("*\n"), 0o644); err != nil {
			return nil, err
		}
	}
	return &Store{dir: dir, maxRuns: maxRuns}, nil
}

// Persist writes a completed run atomically: write <run_id>.tmp/, then
// rename. Bodies above StoredBodyLimit are stored truncated (spec §8.3).
func (st *Store) Persist(run *CompletedRun) error {
	st.mu.Lock()
	defer st.mu.Unlock()

	runsDir := filepath.Join(st.dir, "runs")
	tmp := filepath.Join(runsDir, run.RunID+".tmp")
	final := filepath.Join(runsDir, run.RunID)
	if err := os.MkdirAll(tmp, 0o755); err != nil {
		return err
	}
	defer func() { _ = os.RemoveAll(tmp) }()

	metaBytes, err := json.MarshalIndent(run.Meta, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "meta.json"), metaBytes, 0o644); err != nil {
		return err
	}

	var events []byte
	for _, line := range run.Log.Lines() {
		events = append(events, line...)
		events = append(events, '\n')
	}
	if err := os.WriteFile(filepath.Join(tmp, "events.ndjson"), events, 0o644); err != nil {
		return err
	}

	details := run.Details.Details()
	stored := make([]storedDetail, 0, len(details))
	for _, d := range details {
		sd := storedDetail{RequestDetail: *d}
		sd.RawRequestBody, sd.Request = capStoredBody(d.rawRequestBody, d.Request)
		sd.RawResponseBody, sd.Response = capStoredResponse(d.rawResponseBody, d.Response)
		stored = append(stored, sd)
	}
	detailBytes, err := json.Marshal(stored)
	if err != nil {
		return err
	}
	if err := os.WriteFile(filepath.Join(tmp, "detail.json"), detailBytes, 0o644); err != nil {
		return err
	}

	_ = os.RemoveAll(final)
	if err := os.Rename(tmp, final); err != nil {
		return err
	}
	st.pruneLocked()
	return nil
}

// capStoredBody applies the 1 MiB store cap to a request payload.
func capStoredBody(raw []byte, req *requestPayloadJSON) (string, *requestPayloadJSON) {
	if raw == nil || req == nil || req.Body == nil {
		return "", req
	}
	if len(raw) > StoredBodyLimit {
		cp := *req
		body := *req.Body
		body.Truncated = true
		body.Content = ""
		cp.Body = &body
		return base64.StdEncoding.EncodeToString(raw[:StoredBodyLimit]), &cp
	}
	return base64.StdEncoding.EncodeToString(raw), req
}

func capStoredResponse(raw []byte, resp *responsePayloadJSON) (string, *responsePayloadJSON) {
	if raw == nil || resp == nil || resp.Body == nil {
		return "", resp
	}
	if len(raw) > StoredBodyLimit {
		cp := *resp
		body := *resp.Body
		body.Truncated = true
		body.Content = ""
		cp.Body = &body
		return base64.StdEncoding.EncodeToString(raw[:StoredBodyLimit]), &cp
	}
	return base64.StdEncoding.EncodeToString(raw), resp
}

// pruneLocked deletes the oldest runs beyond maxRuns by created_at (fallback
// directory mtime). Corrupt directories count toward pruning.
func (st *Store) pruneLocked() {
	metas := st.listLocked()
	if len(metas) <= st.maxRuns {
		return
	}
	for _, m := range metas[st.maxRuns:] {
		_ = os.RemoveAll(filepath.Join(st.dir, "runs", m.RunID))
	}
}

type storedRun struct {
	RunID     string
	CreatedAt string
	Meta      *RunMeta // nil when meta.json is unreadable
}

// listLocked returns stored runs newest-first.
func (st *Store) listLocked() []storedRun {
	entries, err := os.ReadDir(filepath.Join(st.dir, "runs"))
	if err != nil {
		return nil
	}
	var runs []storedRun
	for _, e := range entries {
		if !e.IsDir() || filepath.Ext(e.Name()) == ".tmp" {
			continue
		}
		sr := storedRun{RunID: e.Name()}
		data, err := os.ReadFile(filepath.Join(st.dir, "runs", e.Name(), "meta.json"))
		if err == nil {
			var meta RunMeta
			if json.Unmarshal(data, &meta) == nil && meta.SchemaVersion == 1 {
				sr.Meta = &meta
				sr.CreatedAt = meta.CreatedAt
			}
		}
		if sr.CreatedAt == "" {
			if fi, err := e.Info(); err == nil {
				sr.CreatedAt = fi.ModTime().UTC().Format("2006-01-02T15:04:05Z")
			}
		}
		runs = append(runs, sr)
	}
	sort.Slice(runs, func(i, j int) bool { return runs[i].CreatedAt > runs[j].CreatedAt })
	return runs
}

// List returns readable run metas, newest first, with the total count.
func (st *Store) List(limit, offset int) ([]RunMeta, int) {
	st.mu.Lock()
	defer st.mu.Unlock()
	all := st.listLocked()
	var metas []RunMeta
	for _, sr := range all {
		if sr.Meta != nil {
			metas = append(metas, *sr.Meta)
		}
	}
	total := len(metas)
	if offset > len(metas) {
		offset = len(metas)
	}
	metas = metas[offset:]
	if limit > 0 && limit < len(metas) {
		metas = metas[:limit]
	}
	return metas, total
}

// Meta loads one run's meta.json.
func (st *Store) Meta(runID string) (*RunMeta, error) {
	data, err := os.ReadFile(filepath.Join(st.dir, "runs", runID, "meta.json"))
	if err != nil {
		return nil, err
	}
	var meta RunMeta
	if err := json.Unmarshal(data, &meta); err != nil {
		return nil, err
	}
	if meta.SchemaVersion != 1 {
		return nil, fmt.Errorf("unknown store schema version %d", meta.SchemaVersion)
	}
	return &meta, nil
}

// Events returns the raw NDJSON stream bytes.
func (st *Store) Events(runID string) ([]byte, error) {
	return os.ReadFile(filepath.Join(st.dir, "runs", runID, "events.ndjson"))
}

// LoadDetails rebuilds a DetailCollector from detail.json so persisted runs
// serve through the same read paths as ring runs.
func (st *Store) LoadDetails(runID, root string) (*DetailCollector, error) {
	data, err := os.ReadFile(filepath.Join(st.dir, "runs", runID, "detail.json"))
	if err != nil {
		return nil, err
	}
	var stored []storedDetail
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil, err
	}
	c := NewDetailCollector(root)
	for i := range stored {
		sd := &stored[i]
		d := sd.RequestDetail
		if sd.RawRequestBody != "" {
			if raw, err := base64.StdEncoding.DecodeString(sd.RawRequestBody); err == nil {
				d.rawRequestBody = raw
			}
		}
		if sd.RawResponseBody != "" {
			if raw, err := base64.StdEncoding.DecodeString(sd.RawResponseBody); err == nil {
				d.rawResponseBody = raw
			}
		}
		if d.Assertions != nil {
			for _, item := range d.Assertions.Items {
				if !item.Passed {
					d.failMessage = fmt.Sprintf("%s: expected %s, got %s", item.Type, item.Expected, item.Actual)
					break
				}
			}
		}
		if d.Retry != nil {
			d.retryCount = d.Retry.Count
		}
		dd := d
		c.byID[d.RequestID] = &dd
		c.order = append(c.order, d.RequestID)
	}
	return c, nil
}

// Delete removes a persisted run directory.
func (st *Store) Delete(runID string) error {
	st.mu.Lock()
	defer st.mu.Unlock()
	dir := filepath.Join(st.dir, "runs", runID)
	if _, err := os.Stat(dir); err != nil {
		return err
	}
	return os.RemoveAll(dir)
}
