package markdown

import (
	"errors"
	"regexp"
	"strings"
)

// spliceAction describes what the writer should do when it encounters an
// existing file at the target path.
type spliceAction int

const (
	// actionSplice: BEGIN/END both present and slug matches; rewrite only the
	// bytes between the sentinel lines; content above and below is preserved.
	actionSplice spliceAction = iota

	// actionAppendOrphan: BEGIN/END both present but slug differs (request renamed).
	// Append a new sentinel block below the existing content; emit a stderr warning.
	actionAppendOrphan

	// actionDotNew: malformed sentinels (BEGIN without END, or no sentinels).
	// Write <slug>.md.new alongside; original is never overwritten.
	actionDotNew
)

// sentinelLocation records where the BEGIN and END lines are in the existing file.
type sentinelLocation struct {
	BeginLine int // 0-based line index of the BEGIN line
	EndLine   int // 0-based line index of the END line
	Begin     sentinelAttrs
	End       sentinelAttrs
}

// sentinelAttrs holds the three correlation IDs parsed from a sentinel line.
type sentinelAttrs struct {
	RequestID string
	Slug      string
	RunID     string
}

// errMalformedSentinel is returned when BEGIN exists without END, or vice versa,
// or sentinels are nested.
var errMalformedSentinel = errors.New("markdown: malformed sentinel pair")

// sentinelLineRE matches a complete sentinel line (BEGIN or END).
// Captures: 1=marker (BEGIN|END), 2=request_id, 3=slug, 4=run_id.
var sentinelLineRE = regexp.MustCompile(
	`^<!-- (BEGIN|END) curlew:response id=(\S+) slug=([a-z0-9-]+) run=(\S+) -->$`,
)

// parseSentinels scans the existing file bytes and returns the location of
// the first complete BEGIN/END pair. A file may contain multiple complete
// pairs (the normal result after actionAppendOrphan); this function returns
// only the first. Use parseSentinelForSlug to locate a specific pair.
//
// Returns:
//   - (loc, nil) when at least one complete BEGIN/END pair is present.
//   - (nil, errMalformedSentinel) when a BEGIN has no matching END before the
//     next BEGIN (i.e. a partial/nested sentinel exists in the file).
//   - (nil, nil) when no sentinel lines are present at all.
func parseSentinels(existing []byte) (*sentinelLocation, error) {
	pairs, err := parseAllSentinelPairs(existing)
	if err != nil {
		return nil, err
	}
	if len(pairs) == 0 {
		return nil, nil
	}
	return pairs[0], nil
}

// parseAllSentinelPairs scans existing and returns all complete BEGIN/END
// pairs in file order. An incomplete pair (BEGIN without a following END
// before the next BEGIN, or a lone END) is treated as malformed.
func parseAllSentinelPairs(existing []byte) ([]*sentinelLocation, error) {
	lines := splitLines(existing)

	var pairs []*sentinelLocation
	var pending *sentinelLocation // BEGIN seen but END not yet found

	for i, raw := range lines {
		line := strings.TrimRight(string(raw), "\r\n")
		m := sentinelLineRE.FindStringSubmatch(line)
		if m == nil {
			continue
		}
		marker, reqID, slug, runID := m[1], m[2], m[3], m[4]
		attrs := sentinelAttrs{RequestID: reqID, Slug: slug, RunID: runID}

		switch marker {
		case "BEGIN":
			if pending != nil {
				// A new BEGIN arrived before the previous one was closed — malformed.
				return nil, errMalformedSentinel
			}
			pending = &sentinelLocation{
				BeginLine: i,
				Begin:     attrs,
			}
		case "END":
			if pending == nil {
				// END without a preceding BEGIN — malformed.
				return nil, errMalformedSentinel
			}
			pending.EndLine = i
			pending.End = attrs
			pairs = append(pairs, pending)
			pending = nil
		}
	}

	if pending != nil {
		// A BEGIN with no closing END — malformed.
		return nil, errMalformedSentinel
	}
	return pairs, nil
}

// decideAction inspects the existing file and the current slug to choose one
// of the four splice actions.
//
// The run= attribute on the sentinel is intentionally not part of the
// identity check: two runs against the same collection may carry different
// run_ids; the slug is the stable identity.
//
// Files that have been through actionAppendOrphan contain two or more complete
// sentinel pairs. decideAction finds the pair matching currentSlug (splice) or,
// if no matching pair exists, returns actionAppendOrphan to add one below.
func decideAction(existing []byte, currentSlug string) (spliceAction, *sentinelLocation, error) {
	pairs, err := parseAllSentinelPairs(existing)
	if err != nil {
		// Malformed sentinels (incomplete pair): write .md.new.
		return actionDotNew, nil, err
	}
	if len(pairs) == 0 {
		// No sentinels: treat as user-owned; write .md.new.
		return actionDotNew, nil, nil
	}

	// Look for a pair whose slug matches currentSlug.
	for _, p := range pairs {
		if p.Begin.Slug == currentSlug {
			return actionSplice, p, nil
		}
	}

	// No pair matches: this is an orphan-append situation. Return the last
	// pair as the loc (used only for its slug in the warning message).
	return actionAppendOrphan, pairs[len(pairs)-1], nil
}
