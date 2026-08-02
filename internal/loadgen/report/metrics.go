package report

import (
	"math"
	"sort"
	"sync"
	"time"
)

// Sample is the report-facing view of one completed request.
type Sample struct {
	StartOffsetNs int64
	LatencyNs     int64
	Success       bool
}

// Bucket is the per-second time-series entry.
type Bucket struct {
	SecondOffset int
	Requests     int
	Errors       int
	P50          time.Duration
	P95          time.Duration
}

// Metrics is the aggregated result of a perf run.
type Metrics struct {
	Requests           int
	Successes          int
	Failures           int
	Elapsed            time.Duration
	P50, P95, P99      time.Duration
	Min, Max, Mean     time.Duration
	ErrorRate          float64 // 0..1
	Throughput         float64 // req/s
	SmallSampleWarning bool
	Buckets            []Bucket
}

// Aggregator collects samples from a running perf load test. It is safe for
// concurrent use by multiple VU goroutines.
//
// Memory bound: at 10k RPS for 60s, the samples slice holds ~600k entries at
// 8 bytes each (~4.8 MB). If memory becomes a constraint in future milestones,
// replace with a streaming quantile digest (e.g. influxdata/tdigest).
type Aggregator struct {
	mu         sync.Mutex
	samples    []time.Duration      // all latencies, for global percentiles
	buckets    map[int]*bucketState // keyed by second offset
	requests   int
	successes  int
	failures   int
	lastOffset int64 // ns; largest start offset observed
}

type bucketState struct {
	requests  int
	errors    int
	latencies []time.Duration
}

// New returns a ready-to-use Aggregator.
func New() *Aggregator {
	return &Aggregator{buckets: make(map[int]*bucketState)}
}

// Observe records one sample. Thread-safe; VU goroutines may call concurrently.
func (a *Aggregator) Observe(s Sample) {
	a.mu.Lock()
	defer a.mu.Unlock()

	lat := time.Duration(s.LatencyNs)
	a.samples = append(a.samples, lat)
	a.requests++
	if s.Success {
		a.successes++
	} else {
		a.failures++
	}
	if s.StartOffsetNs > a.lastOffset {
		a.lastOffset = s.StartOffsetNs
	}

	sec := int(s.StartOffsetNs / int64(time.Second))
	b, ok := a.buckets[sec]
	if !ok {
		b = &bucketState{}
		a.buckets[sec] = b
	}
	b.requests++
	if !s.Success {
		b.errors++
	}
	b.latencies = append(b.latencies, lat)
}

// Metrics returns an immutable snapshot. elapsed should be the actual run
// duration; it is used to compute Throughput.
func (a *Aggregator) Metrics(elapsed time.Duration) Metrics {
	a.mu.Lock()
	defer a.mu.Unlock()

	m := Metrics{
		Requests:  a.requests,
		Successes: a.successes,
		Failures:  a.failures,
		Elapsed:   elapsed,
	}
	if a.requests == 0 {
		return m
	}

	sorted := make([]time.Duration, len(a.samples))
	copy(sorted, a.samples)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i] < sorted[j] })

	m.Min = sorted[0]
	m.Max = sorted[len(sorted)-1]
	m.P50 = percentile(sorted, 0.50)
	m.P95 = percentile(sorted, 0.95)
	if len(sorted) < 100 {
		m.P99 = m.Max
		m.SmallSampleWarning = true
	} else {
		m.P99 = percentile(sorted, 0.99)
	}

	var total time.Duration
	for _, d := range sorted {
		total += d
	}
	m.Mean = total / time.Duration(len(sorted))

	m.ErrorRate = float64(a.failures) / float64(a.requests)
	if elapsed > 0 {
		m.Throughput = float64(a.requests) / elapsed.Seconds()
	}

	// Buckets: sorted by SecondOffset for deterministic JSON / HTML.
	keys := make([]int, 0, len(a.buckets))
	for k := range a.buckets {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	m.Buckets = make([]Bucket, 0, len(keys))
	for _, k := range keys {
		bs := a.buckets[k]
		local := make([]time.Duration, len(bs.latencies))
		copy(local, bs.latencies)
		sort.Slice(local, func(i, j int) bool { return local[i] < local[j] })
		m.Buckets = append(m.Buckets, Bucket{
			SecondOffset: k,
			Requests:     bs.requests,
			Errors:       bs.errors,
			P50:          percentile(local, 0.50),
			P95:          percentile(local, 0.95),
		})
	}
	return m
}

// percentile uses nearest-rank on a pre-sorted slice. p must be in (0, 1].
// For an empty slice it returns 0. For all valid inputs the computed rank is
// in [0, len(sorted)-1]:
//
//   - rank = ceil(p * n) - 1; with p > 0 and n >= 1, rank >= 0.
//   - with p <= 1 and n >= 1, ceil(p*n) <= n so rank <= n-1.
func percentile(sorted []time.Duration, p float64) time.Duration {
	if len(sorted) == 0 {
		return 0
	}
	rank := int(math.Ceil(p*float64(len(sorted)))) - 1
	return sorted[rank]
}
