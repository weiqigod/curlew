package report

import (
	"sync"
	"testing"
	"time"
)

func TestAggregator_EmptyReturnsZeroMetrics(t *testing.T) {
	a := New()
	m := a.Metrics(0)
	if m.Requests != 0 {
		t.Errorf("Requests = %d, want 0", m.Requests)
	}
	if m.P50 != 0 {
		t.Errorf("P50 = %v, want 0", m.P50)
	}
	if m.SmallSampleWarning {
		t.Error("SmallSampleWarning = true, want false for empty set")
	}
}

func TestAggregator_PercentilesNearestRank(t *testing.T) {
	// 100 samples 1ms..100ms; p50 -> 50ms; p95 -> 95ms; p99 -> 99ms
	a := New()
	for i := 1; i <= 100; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Duration(i) * time.Millisecond),
			Success:       true,
		})
	}
	m := a.Metrics(2 * time.Second)
	if m.P50 != 50*time.Millisecond {
		t.Errorf("P50 = %v, want 50ms", m.P50)
	}
	if m.P95 != 95*time.Millisecond {
		t.Errorf("P95 = %v, want 95ms", m.P95)
	}
	if m.P99 != 99*time.Millisecond {
		t.Errorf("P99 = %v, want 99ms", m.P99)
	}
	if m.SmallSampleWarning {
		t.Error("SmallSampleWarning = true, want false for 100-sample set")
	}
}

func TestAggregator_SmallSampleFallback(t *testing.T) {
	// 10 samples 1ms..10ms -> P99 == Max and SmallSampleWarning == true
	a := New()
	for i := 1; i <= 10; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Duration(i) * time.Millisecond),
			Success:       true,
		})
	}
	m := a.Metrics(1 * time.Second)
	if !m.SmallSampleWarning {
		t.Error("SmallSampleWarning = false, want true for <100 samples")
	}
	if m.P99 != m.Max {
		t.Errorf("P99 = %v, want Max = %v for small sample", m.P99, m.Max)
	}
	if m.Max != 10*time.Millisecond {
		t.Errorf("Max = %v, want 10ms", m.Max)
	}
}

func TestAggregator_ErrorRate(t *testing.T) {
	// 10 samples, 3 failures -> ErrorRate == 0.3
	a := New()
	for i := 0; i < 10; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Millisecond),
			Success:       i >= 3, // first 3 are failures
		})
	}
	m := a.Metrics(1 * time.Second)
	if m.Failures != 3 {
		t.Errorf("Failures = %d, want 3", m.Failures)
	}
	// ErrorRate == 0.3
	const want = 0.3
	const epsilon = 0.0001
	if m.ErrorRate < want-epsilon || m.ErrorRate > want+epsilon {
		t.Errorf("ErrorRate = %v, want %v", m.ErrorRate, want)
	}
}

func TestAggregator_Throughput(t *testing.T) {
	// 100 requests across 2s elapsed -> 50 req/s
	a := New()
	for i := 0; i < 100; i++ {
		a.Observe(Sample{
			StartOffsetNs: 0,
			LatencyNs:     int64(time.Millisecond),
			Success:       true,
		})
	}
	m := a.Metrics(2 * time.Second)
	if m.Throughput != 50.0 {
		t.Errorf("Throughput = %v, want 50.0", m.Throughput)
	}
}

func TestAggregator_BucketsByStartOffset(t *testing.T) {
	// Samples at 0ms, 500ms, 999ms -> bucket 0 has 3 requests
	// Samples at 1000ms, 1500ms -> bucket 1 has 2 requests
	// Verify SecondOffset ordering is 0 then 1.
	a := New()
	offsets := []int64{
		0,
		int64(500 * time.Millisecond),
		int64(999 * time.Millisecond),
		int64(1000 * time.Millisecond),
		int64(1500 * time.Millisecond),
	}
	for _, off := range offsets {
		a.Observe(Sample{
			StartOffsetNs: off,
			LatencyNs:     int64(time.Millisecond),
			Success:       true,
		})
	}
	m := a.Metrics(2 * time.Second)
	if len(m.Buckets) != 2 {
		t.Fatalf("len(Buckets) = %d, want 2", len(m.Buckets))
	}
	if m.Buckets[0].SecondOffset != 0 {
		t.Errorf("Buckets[0].SecondOffset = %d, want 0", m.Buckets[0].SecondOffset)
	}
	if m.Buckets[0].Requests != 3 {
		t.Errorf("Buckets[0].Requests = %d, want 3", m.Buckets[0].Requests)
	}
	if m.Buckets[1].SecondOffset != 1 {
		t.Errorf("Buckets[1].SecondOffset = %d, want 1", m.Buckets[1].SecondOffset)
	}
	if m.Buckets[1].Requests != 2 {
		t.Errorf("Buckets[1].Requests = %d, want 2", m.Buckets[1].Requests)
	}
}

func TestAggregator_ConcurrentObserve(t *testing.T) {
	// 8 goroutines each calling Observe 100x; assert total count after wg.Wait.
	// Run under go test -race.
	a := New()
	const goroutines = 8
	const perGoroutine = 100
	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := 0; i < perGoroutine; i++ {
				a.Observe(Sample{
					StartOffsetNs: 0,
					LatencyNs:     int64(time.Millisecond),
					Success:       true,
				})
			}
		}()
	}
	wg.Wait()
	m := a.Metrics(1 * time.Second)
	if m.Requests != goroutines*perGoroutine {
		t.Errorf("Requests = %d, want %d", m.Requests, goroutines*perGoroutine)
	}
}
