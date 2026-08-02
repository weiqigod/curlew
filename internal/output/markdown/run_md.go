package markdown

import (
	"bytes"
	"fmt"
	"sort"
	"time"
)

// renderRunMD produces the complete byte content for the run.md index file.
// The sentinel block is the deterministic region; the H1 and ## Analysis
// sections are outside and preserved on splice.
func renderRunMD(report *Report) []byte {
	var buf bytes.Buffer

	// H1: collection name
	fmt.Fprintf(&buf, "# %s\n\n", report.CollectionName)

	// ## Notes (agent-owned space above the sentinel)
	fmt.Fprintf(&buf, "## Notes\n\n")

	// Sentinel block for run.md uses "run" as the reserved slug.
	runEntry := &RequestEntry{
		RequestID: "run",
		Slug:      RunMDSentinelSlug,
	}
	buf.Write(renderRunSentinelBlock(report, runEntry))

	// ## Analysis (below sentinel; agent-owned)
	fmt.Fprintf(&buf, "\n## Analysis\n\n")

	return buf.Bytes()
}

// renderRunSentinelBlock renders the deterministic sentinel block for run.md.
func renderRunSentinelBlock(report *Report, entry *RequestEntry) []byte {
	var buf bytes.Buffer

	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelBeginPrefix, entry.RequestID, entry.Slug, report.RunID, SentinelSuffix)

	fmt.Fprintf(&buf, "## Run Summary\n")

	// Summary table
	fmt.Fprintf(&buf, "\n")
	fmt.Fprintf(&buf, "| | |\n")
	fmt.Fprintf(&buf, "|---|---|\n")
	fmt.Fprintf(&buf, "| Total | %d |\n", report.Summary.Total)
	fmt.Fprintf(&buf, "| Passed | %d |\n", report.Summary.Passed)
	fmt.Fprintf(&buf, "| Failed | %d |\n", report.Summary.Failed)
	fmt.Fprintf(&buf, "| Skipped | %d |\n", report.Summary.Skipped)

	// Run metadata
	fmt.Fprintf(&buf, "\n")
	if report.EnvName != "" {
		fmt.Fprintf(&buf, "environment: %s\n", report.EnvName)
	}
	fmt.Fprintf(&buf, "run_id: %s\n", report.RunID)
	fmt.Fprintf(&buf, "started_at: %s\n", report.StartedAt.UTC().Format(time.RFC3339Nano))

	// Per-request links
	if len(report.Requests) == 0 {
		fmt.Fprintf(&buf, "\n_No requests executed._\n")
	} else if report.IsParallel {
		renderRunMDByWave(&buf, report)
	} else {
		fmt.Fprintf(&buf, "\n## Requests\n\n")
		for i := range report.Requests {
			renderRunMDEntry(&buf, &report.Requests[i])
		}
	}

	fmt.Fprintf(&buf, "%sid=%s slug=%s run=%s%s\n",
		SentinelEndPrefix, entry.RequestID, entry.Slug, report.RunID, SentinelSuffix)

	return buf.Bytes()
}

// renderRunMDByWave groups report.Requests by WaveIndex and emits an H2 wave
// header per group. Entries with WaveIndex < 0 (data-driven aggregates,
// sequential interleave) are emitted under ## Sequential at the end. Wave
// indexes are sorted ascending; ties keep input order (slice append order =
// runner emission order).
func renderRunMDByWave(buf *bytes.Buffer, report *Report) {
	waves := map[int][]*RequestEntry{}
	waveOrder := []int{}
	sequential := []*RequestEntry{}
	for i := range report.Requests {
		req := &report.Requests[i]
		if req.WaveIndex < 0 {
			sequential = append(sequential, req)
			continue
		}
		if _, seen := waves[req.WaveIndex]; !seen {
			waveOrder = append(waveOrder, req.WaveIndex)
		}
		waves[req.WaveIndex] = append(waves[req.WaveIndex], req)
	}
	sort.Ints(waveOrder)
	for _, w := range waveOrder {
		fmt.Fprintf(buf, "\n## Wave %d\n\n", w)
		for _, req := range waves[w] {
			renderRunMDEntry(buf, req)
		}
	}
	if len(sequential) > 0 {
		fmt.Fprintf(buf, "\n## Sequential\n\n")
		for _, req := range sequential {
			renderRunMDEntry(buf, req)
		}
	}
}

// renderRunMDEntry renders one bullet line for run.md.
// Data-driven entries (len(Iterations) > 0) collapse to a link to
// <slug>/index.md with aggregate counts. Single-request entries link
// directly to <slug>.md.
func renderRunMDEntry(buf *bytes.Buffer, req *RequestEntry) {
	if len(req.Iterations) > 0 {
		passed, failed, skipped := iterationOutcomeCounts(req.Iterations)
		fmt.Fprintf(buf, "- [%s](%s/index.md) (%d iterations, %d passed, %d failed",
			req.Name, req.Slug, len(req.Iterations), passed, failed)
		if skipped > 0 {
			fmt.Fprintf(buf, ", %d skipped", skipped)
		}
		fmt.Fprintln(buf, ")")
		return
	}
	status := "pass"
	if req.Err != nil || (req.Assertions != nil && !req.Assertions.Passed) {
		status = "fail"
	}
	if req.Skipped {
		status = "skip"
	}
	fmt.Fprintf(buf, "- %s: [%s](%s.md)\n", status, req.Name, req.Slug)
}
