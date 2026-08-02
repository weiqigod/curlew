package markdown

import (
	"bytes"
	"fmt"
	"io"
	"path/filepath"
	"time"
)

// renderDataDrivenRequest writes <dir>/<slug>/index.md plus one
// <dir>/<slug>/iter-<n>.md per iteration. The slug subdirectory is
// created via EnsureReportDir(dir, slug) before any file is written.
//
// Splice discipline: each iter-N.md and the index.md are independent
// splice targets; agent edits outside the sentinels are preserved on
// re-run. The base slug is identical across all files in the
// directory; this is intentional and the splice machinery handles it
// correctly because each path is a distinct file.
func renderDataDrivenRequest(req *RequestEntry, dir, runID string, errW io.Writer) error {
	if err := EnsureReportDir(dir, req.Slug); err != nil {
		return err
	}
	base := filepath.Join(dir, req.Slug)

	// Per-iteration files.
	for i := range req.Iterations {
		it := &req.Iterations[i]
		content := renderIterationFile(req, it, runID)
		path := filepath.Join(base, fmt.Sprintf("iter-%d.md", it.Index))
		if err := writeFile(path, req.Slug, content, errW); err != nil {
			return fmt.Errorf("write iter-%d.md: %w", it.Index, err)
		}
	}

	// Index file.
	indexContent := renderIndex(req, runID)
	indexPath := filepath.Join(base, "index.md")
	if err := writeFile(indexPath, req.Slug, indexContent, errW); err != nil {
		return fmt.Errorf("write index.md: %w", err)
	}
	return nil
}

// renderIterationFile returns the byte content of <slug>/iter-<n>.md.
// Mirrors renderFullFile shape but writes a sentinel with
// request_id=<iter.RequestID>-iter-<index> and slug=<base slug>.
func renderIterationFile(parent *RequestEntry, it *IterationEntry, runID string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# %s [%d/%d]\n\n", parent.DataDrivenName, it.Index+1, parent.IterationTotal)
	fmt.Fprintf(&buf, "## Notes\n\n")
	buf.Write(renderIterationSentinelBlock(parent, it, runID))
	fmt.Fprintf(&buf, "\n## Analysis\n\n")
	return buf.Bytes()
}

// renderIterationSentinelBlock renders the deterministic sentinel block for an
// iter-N.md file. The sentinel uses id=<iterID> slug=<baseSlug> run=<runID>.
func renderIterationSentinelBlock(parent *RequestEntry, it *IterationEntry, runID string) []byte {
	var buf bytes.Buffer
	iterID := fmt.Sprintf("%s-iter-%d", it.RequestID, it.Index)
	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelBeginPrefix, iterID, parent.Slug, runID, SentinelSuffix)
	fmt.Fprintf(&buf, "## Response (deterministic)\n")

	// Re-use the same Section 6-10 helpers used by renderSentinelBlock.
	// The shared helpers operate on a temporary RequestEntry built from the
	// iteration data so we don't duplicate body/header/timing/assertion
	// rendering code.
	synth := iterationToEntry(parent, it)
	fmt.Fprintf(&buf, "\n### Request\n\n")
	renderRequest(&buf, synth)
	fmt.Fprintf(&buf, "\n### Response %d\n\n", synth.StatusCode)
	renderResponse(&buf, synth)
	fmt.Fprintf(&buf, "\n### Response metadata\n\n")
	renderResponseMetadata(&buf, synth)
	fmt.Fprintf(&buf, "\n### Timing\n\n")
	renderTiming(&buf, synth)
	fmt.Fprintf(&buf, "\n### Assertions\n\n")
	renderAssertions(&buf, synth)
	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelEndPrefix, iterID, parent.Slug, runID, SentinelSuffix)
	return buf.Bytes()
}

// iterationToEntry projects iteration-level data onto a RequestEntry so
// the existing Section 6-10 helpers can render it without duplication.
// The returned entry intentionally has empty Iterations (the iteration
// itself never recurses into the DD code path).
func iterationToEntry(parent *RequestEntry, it *IterationEntry) *RequestEntry {
	return &RequestEntry{
		RequestID:   it.RequestID,
		Slug:        parent.Slug,
		Name:        parent.DataDrivenName,
		Method:      it.Method,
		URL:         it.URL,
		StatusCode:  it.StatusCode,
		DurationMs:  it.DurationMs,
		WaveIndex:   -1, // iterations always sequential within their group
		StartedAt:   it.StartedAt,
		RequestHdr:  it.RequestHdr,
		RequestBody: it.RequestBody,
		RespHeaders: it.RespHeaders,
		RespBody:    it.RespBody,
		Assertions:  it.Assertions,
		Err:         it.Err,
		Skipped:     it.Skipped,
		SkipReason:  it.SkipReason,
	}
}

// renderIndex returns the byte content of <slug>/index.md.
func renderIndex(req *RequestEntry, runID string) []byte {
	var buf bytes.Buffer
	fmt.Fprintf(&buf, "# %s\n\n", req.DataDrivenName)
	fmt.Fprintf(&buf, "## Notes\n\n")
	buf.Write(renderIndexSentinelBlock(req, runID))
	fmt.Fprintf(&buf, "\n## Analysis\n\n")
	return buf.Bytes()
}

// renderIndexSentinelBlock renders the deterministic sentinel block for index.md.
func renderIndexSentinelBlock(req *RequestEntry, runID string) []byte {
	var buf bytes.Buffer
	indexID := req.RequestID + "-index"
	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelBeginPrefix, indexID, req.Slug, runID, SentinelSuffix)

	fmt.Fprintf(&buf, "## Iterations\n\n")
	passed, failed, skipped := iterationOutcomeCounts(req.Iterations)
	total := len(req.Iterations)
	fmt.Fprintf(&buf, "%s — %d iterations (%d passed, %d failed",
		req.DataDrivenName, total, passed, failed)
	if skipped > 0 {
		fmt.Fprintf(&buf, ", %d skipped", skipped)
	}
	fmt.Fprintf(&buf, ")\n")
	fmt.Fprintf(&buf, "run_id: %s\n", runID)
	fmt.Fprintf(&buf, "started_at: %s\n", req.StartedAt.UTC().Format(time.RFC3339Nano))

	fmt.Fprintf(&buf, "\n| iter | status | duration_ms | link |\n")
	fmt.Fprintf(&buf, "|------|--------|-------------|------|\n")
	for i := range req.Iterations {
		it := &req.Iterations[i]
		status := iterationStatus(it)
		fmt.Fprintf(&buf, "| %d | %s | %d | [iter-%d.md](iter-%d.md) |\n",
			it.Index, status, it.DurationMs, it.Index, it.Index)
	}
	if req.IterationTotal > total {
		fmt.Fprintf(&buf, "| ... | _truncated_ | _runner cap reached at row %d (of %d)_ | — |\n",
			total, req.IterationTotal)
	}

	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelEndPrefix, indexID, req.Slug, runID, SentinelSuffix)
	return buf.Bytes()
}

// iterationOutcomeCounts returns (passed, failed, skipped) for a slice
// of iterations. Used by run.md's aggregate bullet AND index.md's header.
func iterationOutcomeCounts(iters []IterationEntry) (passed, failed, skipped int) {
	for i := range iters {
		switch iterationStatus(&iters[i]) {
		case "pass":
			passed++
		case "fail":
			failed++
		case "skip":
			skipped++
		}
	}
	return passed, failed, skipped
}

// iterationStatus returns "pass" | "fail" | "skip" for one iteration.
func iterationStatus(it *IterationEntry) string {
	if it.Skipped {
		return "skip"
	}
	if it.Err != nil || (it.Assertions != nil && !it.Assertions.Passed) {
		return "fail"
	}
	return "pass"
}
