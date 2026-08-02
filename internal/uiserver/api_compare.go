package uiserver

import (
	"net/http"
	"sort"
)

// compareSide is one run's view of a paired request (§4.12).
type compareSide struct {
	RequestID  string      `json:"request_id"`
	Outcome    *string     `json:"outcome"`
	StatusCode int         `json:"status_code,omitempty"`
	DurationMs int64       `json:"duration_ms"`
	Timing     *timingJSON `json:"timing"`
	FailMsg    *string     `json:"fail_message"`
	Error      *errorJSON  `json:"error"`
}

type compareDelta struct {
	DurationMs     int64            `json:"duration_ms"`
	OutcomeChanged bool             `json:"outcome_changed"`
	StatusChanged  bool             `json:"status_changed"`
	TimingUs       map[string]int64 `json:"timing_us,omitempty"`
}

type comparePair struct {
	Slug      string        `json:"slug"`
	Iteration *int          `json:"iteration"`
	Name      string        `json:"name"`
	Method    string        `json:"method"`
	Base      *compareSide  `json:"base"`
	Target    *compareSide  `json:"target"`
	Delta     *compareDelta `json:"delta"`
}

type compareOnly struct {
	Slug      string `json:"slug"`
	Name      string `json:"name"`
	Method    string `json:"method"`
	RequestID string `json:"request_id"`
}

// pairKey aligns requests across runs (slug, iteration index) — slugs are
// canonical, stable, and frozen by the events schema's derivation rule.
type pairKey struct {
	slug string
	iter int // -1 when not data-driven
}

func keyOf(e requestListEntry) pairKey {
	iter := -1
	if e.Iteration != nil {
		iter = e.Iteration.Index
	}
	return pairKey{slug: e.Slug, iter: iter}
}

func sideOf(d *DetailCollector, e requestListEntry) *compareSide {
	side := &compareSide{
		RequestID:  e.RequestID,
		Outcome:    e.Outcome,
		StatusCode: e.StatusCode,
		DurationMs: e.DurationMs,
		FailMsg:    e.FailMsg,
		Error:      e.Error,
	}
	if det := d.Get(e.RequestID); det != nil {
		side.Timing = det.Timing
	}
	return side
}

// handleCompare serves GET /api/v1/compare?base=&target= (§4.12).
// The server computes alignment and numeric deltas; body text diffs are the
// client's job.
func (s *Server) handleCompare(w http.ResponseWriter, r *http.Request) {
	baseID := r.URL.Query().Get("base")
	targetID := r.URL.Query().Get("target")
	if baseID == "" || targetID == "" {
		writeAPIError(w, http.StatusBadRequest, "bad_request", "base and target run ids are required", "", nil)
		return
	}
	baseRec := s.findRun(baseID)
	targetRec := s.findRun(targetID)
	if baseRec == nil || baseRec.details == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run "+baseID, "", nil)
		return
	}
	if targetRec == nil || targetRec.details == nil {
		writeAPIError(w, http.StatusNotFound, "not_found", "unknown run "+targetID, "", nil)
		return
	}

	baseList := baseRec.details.List()
	targetList := targetRec.details.List()
	targetByKey := make(map[pairKey]requestListEntry, len(targetList))
	for _, e := range targetList {
		targetByKey[keyOf(e)] = e
	}

	pairs := []comparePair{}
	onlyBase := []compareOnly{}
	matched := map[pairKey]bool{}
	for _, be := range baseList {
		k := keyOf(be)
		te, ok := targetByKey[k]
		if !ok {
			onlyBase = append(onlyBase, compareOnly{Slug: be.Slug, Name: be.Name, Method: be.Method, RequestID: be.RequestID})
			continue
		}
		matched[k] = true
		var iter *int
		if k.iter >= 0 {
			i := k.iter
			iter = &i
		}
		baseSide := sideOf(baseRec.details, be)
		targetSide := sideOf(targetRec.details, te)
		delta := &compareDelta{
			DurationMs:     targetSide.DurationMs - baseSide.DurationMs,
			OutcomeChanged: !equalOutcome(baseSide.Outcome, targetSide.Outcome),
			StatusChanged:  baseSide.StatusCode != targetSide.StatusCode,
		}
		if baseSide.Timing != nil && targetSide.Timing != nil {
			delta.TimingUs = map[string]int64{
				"dns":      usDelta(baseSide.Timing.DNSUs, targetSide.Timing.DNSUs),
				"connect":  usDelta(baseSide.Timing.ConnectUs, targetSide.Timing.ConnectUs),
				"tls":      usDelta(baseSide.Timing.TLSUs, targetSide.Timing.TLSUs),
				"ttfb":     usDelta(baseSide.Timing.TTFBUs, targetSide.Timing.TTFBUs),
				"download": usDelta(baseSide.Timing.DownloadUs, targetSide.Timing.DownloadUs),
				"total":    targetSide.Timing.TotalUs - baseSide.Timing.TotalUs,
			}
		}
		pairs = append(pairs, comparePair{
			Slug: be.Slug, Iteration: iter, Name: be.Name, Method: be.Method,
			Base: baseSide, Target: targetSide, Delta: delta,
		})
	}
	onlyTarget := []compareOnly{}
	for _, te := range targetList {
		if !matched[keyOf(te)] {
			onlyTarget = append(onlyTarget, compareOnly{Slug: te.Slug, Name: te.Name, Method: te.Method, RequestID: te.RequestID})
		}
	}
	sort.SliceStable(onlyTarget, func(i, j int) bool { return onlyTarget[i].Slug < onlyTarget[j].Slug })

	resp := map[string]any{
		"pairs":          pairs,
		"only_in_base":   onlyBase,
		"only_in_target": onlyTarget,
	}
	if baseRec.meta != nil {
		resp["base"] = baseRec.meta
	}
	if targetRec.meta != nil {
		resp["target"] = targetRec.meta
	}
	writeJSON(w, http.StatusOK, resp)
}

func equalOutcome(a, b *string) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func usDelta(a, b *int64) int64 {
	var av, bv int64
	if a != nil {
		av = *a
	}
	if b != nil {
		bv = *b
	}
	return bv - av
}
