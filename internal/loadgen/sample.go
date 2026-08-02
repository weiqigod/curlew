package loadgen

// Sample is one completed request in a perf run. StartOffsetNs is measured
// from the start of Run (not wall time). Zero value is a valid "not yet set"
// sentinel used only inside tests.
type Sample struct {
	StartOffsetNs int64
	LatencyNs     int64
	Success       bool
}
