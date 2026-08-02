package output

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	apierrors "github.com/weiqigod/curlew/internal/errors"
	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/retry"
)

// Printer renders test output to a writer with optional ANSI color support.
type Printer struct {
	w         io.Writer
	color     bool
	verbosity Verbosity
}

// NewPrinter creates a Printer. Pass VerbosityVerbose or VerbosityDebug for
// verbose output, VerbosityQuiet for quiet mode.
func NewPrinter(w io.Writer, color bool, verbosity ...Verbosity) *Printer {
	v := VerbosityDefault
	if len(verbosity) > 0 {
		v = verbosity[0]
	}
	return &Printer{w: w, color: color, verbosity: v}
}

// Result writes a single request result line with pass/fail indicator.
// When retryCount > 0, a "(retry: N)" suffix is appended.
func (p *Printer) Result(name string, result *httpexec.Result, passed bool, retryCount int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	indicator := "✓"
	code := ansiGreen
	if !passed {
		indicator = "✗"
		code = ansiRed
	}
	if retryCount > 0 {
		_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms (retry: %d)\n",
			colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds(), retryCount)
	} else {
		_, _ = fmt.Fprintf(p.w, "  %s %s  %d  %dms\n",
			colorize(indicator, code, p.color), name, result.StatusCode, result.Duration.Milliseconds())
	}
}

// RetryAttemptDetails writes per-attempt retry information (shown at -v and -vv).
// Each line shows attempt number, status/error, duration, and delay.
// Skipped when there is only one attempt (no retry occurred).
func (p *Printer) RetryAttemptDetails(details []retry.AttemptDetail) {
	if p.verbosity < VerbosityVerbose || len(details) <= 1 {
		return
	}
	for _, d := range details {
		if d.Err != nil {
			_, _ = fmt.Fprintf(p.w, "    %s\n",
				colorize(fmt.Sprintf("Attempt %d: error (%dms, delay %dms)",
					d.Number, d.Duration.Milliseconds(), d.Delay.Milliseconds()), ansiGray, p.color))
		} else {
			_, _ = fmt.Fprintf(p.w, "    %s\n",
				colorize(fmt.Sprintf("Attempt %d: %d (%dms, delay %dms)",
					d.Number, d.StatusCode, d.Duration.Milliseconds(), d.Delay.Milliseconds()), ansiGray, p.color))
		}
	}
}

// AssertionDetail writes an assertion failure detail line.
func (p *Printer) AssertionDetail(assertType, expected, actual string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	detail := fmt.Sprintf("✗ %s: expected %s, got %s", assertType, expected, actual)
	_, _ = fmt.Fprintf(p.w, "    %s\n", colorize(detail, ansiRed, p.color))
}

// CollectionHeader writes the collection name header.
func (p *Printer) CollectionHeader(name string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Collection: "+name, ansiBold, p.color))
}

// SectionHeader writes a phase section header (e.g., "Setup:", "Teardown:").
func (p *Printer) SectionHeader(section string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(section+":", ansiBoldCyan, p.color))
}

// Warning writes a warning message.
func (p *Printer) Warning(msg string) {
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Warning: "+msg, ansiYellow, p.color))
}

// Skipped writes a skipped request line.
func (p *Printer) Skipped(name string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  %s  SKIPPED\n", colorize(name, ansiGray, p.color))
}

// SkippedWithReason writes a skipped request line with a human-readable reason.
// Format: "  SKIPPED  <name>  (<reason>)" when reason is non-empty, or
// "  <name>  SKIPPED" when reason is empty.
func (p *Printer) SkippedWithReason(name, reason string) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	if reason != "" {
		_, _ = fmt.Fprintf(p.w, "  SKIPPED  %s  (%s)\n",
			colorize(name, ansiGray, p.color),
			colorize(reason, ansiGray, p.color))
	} else {
		_, _ = fmt.Fprintf(p.w, "  %s  SKIPPED\n", colorize(name, ansiGray, p.color))
	}
}

// ImpactLine holds a single impact analysis entry for display.
type ImpactLine struct {
	FailedName   string
	SkippedCount int
}

// ImpactSummary writes the parallel execution impact analysis.
func (p *Printer) ImpactSummary(entries []ImpactLine) {
	if len(entries) == 0 || p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize("Impact Analysis:", ansiBold, p.color))
	for _, e := range entries {
		_, _ = fmt.Fprintf(p.w, "  %s\n",
			colorize(fmt.Sprintf("%d request(s) skipped due to dependency on '%s'",
				e.SkippedCount, e.FailedName), ansiYellow, p.color))
	}
}

// SummaryWithDuration writes the execution summary including total duration.
// When skipped is 0, the skipped segment is omitted.
func (p *Printer) SummaryWithDuration(total, passed, failed, skipped int, duration time.Duration) {
	sep := strings.Repeat("─", 32)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(sep, ansiGray, p.color))

	passedStr := fmt.Sprintf("%d passed", passed)
	failedStr := fmt.Sprintf("%d failed", failed)
	durationStr := fmt.Sprintf("(%dms)", duration.Milliseconds())

	if p.color {
		passedStr = colorize(passedStr, ansiGreen, true)
		if failed > 0 {
			failedStr = colorize(failedStr, ansiRed, true)
		}
		durationStr = colorize(durationStr, ansiGray, true)
	}

	if skipped > 0 {
		skippedStr := fmt.Sprintf("%d skipped", skipped)
		if p.color {
			skippedStr = colorize(skippedStr, ansiYellow, true)
		}
		_, _ = fmt.Fprintf(p.w, "  %d request(s): %s, %s, %s %s\n",
			total, passedStr, failedStr, skippedStr, durationStr)
	} else {
		_, _ = fmt.Fprintf(p.w, "  %d request(s): %s, %s %s\n",
			total, passedStr, failedStr, durationStr)
	}
}

// Error writes an error message.
func (p *Printer) Error(msg string) {
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize("Error: "+msg, ansiRed, p.color))
}

// StructuredError writes a formatted error using the [ERROR] format.
func (p *Printer) StructuredError(err error) {
	formatted := apierrors.Format(err)
	_, _ = fmt.Fprintln(p.w, colorize(formatted, ansiRed, p.color))
}

// RequestError writes an error for a named request.
// Format: [ERROR] <request-name> — <classified message>
func (p *Printer) RequestError(name string, err error) {
	var netErr *apierrors.NetworkError
	if errors.As(err, &netErr) {
		_, _ = fmt.Fprintf(p.w, "%s\n", colorize(fmt.Sprintf("[ERROR] %s — %s", name, netErr.Message), ansiRed, p.color))
		if netErr.Hint != "" {
			_, _ = fmt.Fprintf(p.w, "  Hint: %s\n", colorize(netErr.Hint, ansiGray, p.color))
		}
		return
	}
	_, _ = fmt.Fprintf(p.w, "%s\n", colorize(fmt.Sprintf("[ERROR] %s — %s", name, err), ansiRed, p.color))
}

// RequestDetail writes request method, URL, and headers (shown at -v and -vv).
func (p *Printer) RequestDetail(method, url string, headers map[string]string) {
	if p.verbosity < VerbosityVerbose {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  > %s %s\n", method, url)
	for k, v := range headers {
		_, _ = fmt.Fprintf(p.w, "  > %s: %s\n", k, v)
	}
}

// ResponseDetail writes response status and headers (shown at -v and -vv).
func (p *Printer) ResponseDetail(statusCode int, headers http.Header) {
	if p.verbosity < VerbosityVerbose {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  < %d\n", statusCode)
	for k, vs := range headers {
		for _, v := range vs {
			_, _ = fmt.Fprintf(p.w, "  < %s: %s\n", k, v)
		}
	}
}

// RequestBodyDump writes full request body (shown at -vv only).
func (p *Printer) RequestBodyDump(body any) {
	if p.verbosity < VerbosityDebug || body == nil {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  > (body): %v\n", body)
}

// ResponseBodyDump writes full response body (shown at -vv only).
// Truncates at 10KB with indicator.
func (p *Printer) ResponseBodyDump(body []byte) {
	if p.verbosity < VerbosityDebug || len(body) == 0 {
		return
	}
	const maxBytes = 10 * 1024
	if len(body) > maxBytes {
		_, _ = fmt.Fprintf(p.w, "  < (body, truncated): %s\n  < [%d bytes total]\n", body[:maxBytes], len(body))
	} else {
		_, _ = fmt.Fprintf(p.w, "  < (body): %s\n", body)
	}
}

// WaveHeader writes a wave section header for parallel execution.
// Shows wave number (1-based) and count of concurrent requests.
func (p *Printer) WaveHeader(waveNum, concurrentCount int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	label := fmt.Sprintf("Wave %d (%d concurrent):", waveNum, concurrentCount)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(label, ansiBoldCyan, p.color))
}

// ParallelSummary writes parallel execution metadata: wave count,
// max parallelism, and speedup factor.
func (p *Printer) ParallelSummary(waveCount, maxParallelism int, duration time.Duration, waveDurations []time.Duration) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "  Waves: %d, Max parallelism: %d\n", waveCount, maxParallelism)
	// Speedup: sum of wave durations (sequential estimate) vs actual duration
	var seqEstimate time.Duration
	for _, d := range waveDurations {
		seqEstimate += d
	}
	// Only show speedup when meaningful (more than 1 wave)
	if waveCount > 1 && duration > 0 {
		speedup := float64(seqEstimate) / float64(duration)
		_, _ = fmt.Fprintf(p.w, "  Speedup: %.1fx\n", speedup)
	}
}

// DataDrivenHeader writes the data-driven section header.
// Format: "Data-Driven: <name> (<total> iterations)"
func (p *Printer) DataDrivenHeader(name string, total int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	label := fmt.Sprintf("Data-Driven: %s (%d iterations)", name, total)
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize(label, ansiBoldCyan, p.color))
}

// DataDrivenCompactSummary writes a one-line compact summary for >= 10 iterations.
// Format: "  <passed> passed, <failed> failed (<total> total, avg <avg>ms)"
func (p *Printer) DataDrivenCompactSummary(passed, failed, total int, avgDurationMs int64, failedIndices []int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	passedStr := fmt.Sprintf("%d passed", passed)
	failedStr := fmt.Sprintf("%d failed", failed)
	if p.color {
		passedStr = colorize(passedStr, ansiGreen, true)
		if failed > 0 {
			failedStr = colorize(failedStr, ansiRed, true)
		}
	}
	_, _ = fmt.Fprintf(p.w, "  %s, %s (%d total, avg %dms)\n", passedStr, failedStr, total, avgDurationMs)
	if len(failedIndices) > 0 {
		idxStrs := make([]string, len(failedIndices))
		for i, idx := range failedIndices {
			idxStrs[i] = fmt.Sprintf("%d", idx)
		}
		_, _ = fmt.Fprintf(p.w, "  %s\n", colorize("Failed iterations: "+strings.Join(idxStrs, ", "), ansiRed, p.color))
	}
}

// DataDrivenVerboseResult writes a single verbose iteration result line.
// Format: "  ✓ Iteration <n>: <label> (<duration>ms)" or "  ✗ Iteration <n>: <label> (<duration>ms)"
func (p *Printer) DataDrivenVerboseResult(iterationNum int, label string, durationMs int64, passed bool) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	indicator := "✓"
	code := ansiGreen
	if !passed {
		indicator = "✗"
		code = ansiRed
	}
	_, _ = fmt.Fprintf(p.w, "  %s Iteration %d: %s (%dms)\n",
		colorize(indicator, code, p.color), iterationNum, label, durationMs)
}

// DataDrivenSummary writes the data-driven summary line.
// When failedIndices is non-empty, it also shows which iterations failed.
func (p *Printer) DataDrivenSummary(passed, failed, total int, avgDurationMs int64, failedIndices []int) {
	if p.verbosity <= VerbosityQuiet {
		return
	}
	_, _ = fmt.Fprintf(p.w, "\n  Summary: %d passed, %d failed (%d total)\n", passed, failed, total)
	_, _ = fmt.Fprintf(p.w, "  Average duration: %dms\n", avgDurationMs)
	if len(failedIndices) > 0 {
		idxStrs := make([]string, len(failedIndices))
		for i, idx := range failedIndices {
			idxStrs[i] = fmt.Sprintf("%d", idx)
		}
		_, _ = fmt.Fprintf(p.w, "  %s\n", colorize("Failed iterations: "+strings.Join(idxStrs, ", "), ansiRed, p.color))
	}
}

// GuardRail writes the guard rail limit exceeded message.
func (p *Printer) GuardRail(executed, limit int) {
	_, _ = fmt.Fprintf(p.w, "\n%s\n", colorize("✗ Request limit exceeded", ansiRed, p.color))
	_, _ = fmt.Fprintf(p.w, "  Executed %d requests (limit: %d)\n", executed, limit)
	_, _ = fmt.Fprintf(p.w, "  %s\n", colorize("Hint: Split this collection into multiple smaller collections", ansiGray, p.color))
}
