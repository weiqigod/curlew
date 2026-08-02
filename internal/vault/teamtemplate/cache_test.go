package teamtemplate_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/peterlindqvist/apitest/internal/backend"
	"github.com/peterlindqvist/apitest/internal/vault/teamtemplate"
)

// stubFetcher is a test double for teamtemplate.Fetcher.
type stubFetcher struct {
	result    *teamtemplate.FetchResult
	err       error
	callCount atomic.Int32
	delay     time.Duration // optional delay to simulate slow network
}

func (s *stubFetcher) GetTeamVault(ctx context.Context, _, _ string) (*teamtemplate.FetchResult, error) {
	s.callCount.Add(1)
	if s.delay > 0 {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(s.delay):
		}
	}
	return s.result, s.err
}

// newCache constructs a Cache with an injected clock.
func newCache(cfgDir string, now func() time.Time) *teamtemplate.Cache {
	c := teamtemplate.NewCache(cfgDir)
	teamtemplate.SetCacheClock(c, now)
	return c
}

// writeCacheEnvelope writes a cache envelope directly to disk for test setup.
func writeCacheEnvelope(t *testing.T, cfgDir string, env *teamtemplate.CacheEnvelope) {
	t.Helper()
	if err := teamtemplate.NewCache(cfgDir).Write(env); err != nil {
		t.Fatalf("write cache envelope: %v", err)
	}
}

func TestCache_Load(t *testing.T) {
	now := time.Date(2026, 5, 12, 10, 0, 0, 0, time.UTC)

	tests := []struct {
		name           string
		cacheOnDisk    *teamtemplate.CacheEnvelope
		cacheAge       time.Duration
		force          bool
		fetchResult    *teamtemplate.FetchResult
		fetchErr       error
		wantFetchCalls int32
		wantTemplate   string
		wantWarn       string
		wantErrIs      error
	}{
		{
			name:           "ttl_hit_no_network",
			cacheOnDisk:    &teamtemplate.CacheEnvelope{Version: 1, Template: "T1"},
			cacheAge:       1 * time.Minute,
			force:          false,
			fetchResult:    nil,
			fetchErr:       nil,
			wantFetchCalls: 0,
			wantTemplate:   "T1",
		},
		{
			name:           "ttl_miss_fetch_succeeds",
			cacheOnDisk:    &teamtemplate.CacheEnvelope{Version: 1, Template: "T1"},
			cacheAge:       10 * time.Minute,
			force:          false,
			fetchResult:    &teamtemplate.FetchResult{Version: 2, Template: "T2"},
			fetchErr:       nil,
			wantFetchCalls: 1,
			wantTemplate:   "T2",
		},
		{
			name:           "force_bypasses_ttl",
			cacheOnDisk:    &teamtemplate.CacheEnvelope{Version: 1, Template: "T1"},
			cacheAge:       30 * time.Second,
			force:          true,
			fetchResult:    &teamtemplate.FetchResult{Version: 2, Template: "T2"},
			fetchErr:       nil,
			wantFetchCalls: 1,
			wantTemplate:   "T2",
		},
		{
			name:           "stale_offline_returns_cached_with_warning",
			cacheOnDisk:    &teamtemplate.CacheEnvelope{Version: 1, Template: "T1"},
			cacheAge:       10 * time.Minute,
			force:          false,
			fetchResult:    nil,
			fetchErr:       backend.ErrNetworkFailure,
			wantFetchCalls: 1,
			wantTemplate:   "T1",
			wantWarn:       "team vault unavailable",
		},
		{
			name:           "no_cache_offline_returns_error",
			cacheOnDisk:    nil,
			cacheAge:       0,
			force:          false,
			fetchResult:    nil,
			fetchErr:       backend.ErrNetworkFailure,
			wantFetchCalls: 1,
			wantTemplate:   "",
			wantErrIs:      backend.ErrNetworkFailure,
		},
		{
			name:           "no_cache_fetch_succeeds_persists",
			cacheOnDisk:    nil,
			cacheAge:       0,
			force:          false,
			fetchResult:    &teamtemplate.FetchResult{Version: 1, Template: "T1"},
			fetchErr:       nil,
			wantFetchCalls: 1,
			wantTemplate:   "T1",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			cfgDir := t.TempDir()

			// Set up the on-disk state.
			if tc.cacheOnDisk != nil {
				fetchedAt := now.Add(-tc.cacheAge)
				env := *tc.cacheOnDisk
				env.FetchedAt = fetchedAt.Unix()
				writeCacheEnvelope(t, cfgDir, &env)
			}

			fetcher := &stubFetcher{result: tc.fetchResult, err: tc.fetchErr}
			c := newCache(cfgDir, func() time.Time { return now })

			var warnBuf strings.Builder
			got, err := c.Load(context.Background(), fetcher, "org-1", "token", tc.force, &warnBuf)

			if tc.wantErrIs != nil {
				if !errors.Is(err, tc.wantErrIs) {
					t.Fatalf("error = %v, want %v", err, tc.wantErrIs)
				}
			} else if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if tc.wantTemplate != "" {
				if got == nil {
					t.Fatal("expected non-nil result")
				}
				if got.Template != tc.wantTemplate {
					t.Errorf("Template = %q, want %q", got.Template, tc.wantTemplate)
				}
			}

			if tc.wantWarn != "" {
				if !strings.Contains(warnBuf.String(), tc.wantWarn) {
					t.Errorf("warn = %q, want substring %q", warnBuf.String(), tc.wantWarn)
				}
			}

			if got := fetcher.callCount.Load(); got != tc.wantFetchCalls {
				t.Errorf("fetch calls = %d, want %d", got, tc.wantFetchCalls)
			}
		})
	}
}

func TestCache_LoadCacheOnlyWithoutFetcherOrCoordinates(t *testing.T) {
	t.Run("stale cache remains available without network inputs", func(t *testing.T) {
		dir := t.TempDir()
		c := teamtemplate.NewCache(dir)
		teamtemplate.SetCacheClock(c, func() time.Time { return time.Unix(10_000, 0) })
		want := &teamtemplate.CacheEnvelope{
			FetchedAt: 1,
			Version:   7,
			Template:  backendYAML,
		}
		if err := c.Write(want); err != nil {
			t.Fatalf("Write: %v", err)
		}

		got, err := c.Load(context.Background(), nil, "", "", false, nil)
		if err != nil {
			t.Fatalf("Load cache-only: %v", err)
		}
		if got == nil || got.Version != want.Version || got.Template != want.Template {
			t.Fatalf("Load cache-only = %#v; want version %d template preserved", got, want.Version)
		}
	})

	t.Run("missing cache returns no template without network inputs", func(t *testing.T) {
		c := teamtemplate.NewCache(t.TempDir())
		got, err := c.Load(context.Background(), nil, "", "", false, nil)
		if err != nil {
			t.Fatalf("Load cache-only: %v", err)
		}
		if got != nil {
			t.Fatalf("Load cache-only = %#v; want nil", got)
		}
	})
}

func TestCache_Load_FetchedFileMode0600(t *testing.T) {
	cfgDir := t.TempDir()
	now := time.Now()

	fetcher := &stubFetcher{result: &teamtemplate.FetchResult{Version: 1, Template: "T"}}
	c := newCache(cfgDir, func() time.Time { return now })

	_, err := c.Load(context.Background(), fetcher, "org-1", "tok", false, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	info, err := os.Stat(filepath.Join(cfgDir, "team_vault.json"))
	if err != nil {
		t.Fatalf("stat team_vault.json: %v", err)
	}
	mode := info.Mode().Perm()
	if mode != 0o600 {
		t.Errorf("file mode = %o, want 0600", mode)
	}
}

func TestCache_Refresh_AlwaysFetches(t *testing.T) {
	cfgDir := t.TempDir()
	now := time.Now()

	// Write a fresh cache (within TTL).
	env := &teamtemplate.CacheEnvelope{
		FetchedAt: now.Unix(),
		Version:   1,
		Template:  "old",
	}
	writeCacheEnvelope(t, cfgDir, env)

	fetcher := &stubFetcher{result: &teamtemplate.FetchResult{Version: 2, Template: "new"}}
	c := newCache(cfgDir, func() time.Time { return now })

	var warnBuf strings.Builder
	if err := c.Refresh(context.Background(), fetcher, "org-1", "tok", &warnBuf); err != nil {
		t.Fatalf("Refresh error: %v", err)
	}

	if got := fetcher.callCount.Load(); got != 1 {
		t.Errorf("Refresh() fetch calls = %d, want 1", got)
	}

	// Verify the new value was written.
	got, err := teamtemplate.NewCache(cfgDir).Read()
	if err != nil {
		t.Fatalf("Read after Refresh: %v", err)
	}
	if got.Template != "new" {
		t.Errorf("cache after Refresh: Template = %q, want %q", got.Template, "new")
	}
}

func TestCache_FetchTimeout_FallsBackToStale(t *testing.T) {
	cfgDir := t.TempDir()
	staleTime := time.Now().Add(-10 * time.Minute)

	env := &teamtemplate.CacheEnvelope{
		FetchedAt: staleTime.Unix(),
		Version:   1,
		Template:  "stale",
	}
	writeCacheEnvelope(t, cfgDir, env)

	// Slow fetcher: blocks for 3s, longer than the 2s fetch timeout.
	fetcher := &stubFetcher{
		result: &teamtemplate.FetchResult{Version: 2, Template: "fresh"},
		delay:  3 * time.Second,
	}
	c := newCache(cfgDir, func() time.Time { return time.Now() })

	var warnBuf strings.Builder
	start := time.Now()
	got, err := c.Load(context.Background(), fetcher, "org-1", "tok", false, &warnBuf)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if elapsed > 2500*time.Millisecond {
		t.Errorf("Load took %v, want ≤2.5s (timeout should have fired)", elapsed)
	}
	if got == nil || got.Template != "stale" {
		t.Errorf("template = %v, want stale", got)
	}
	if !strings.Contains(warnBuf.String(), "team vault unavailable") {
		t.Errorf("expected warning about vault unavailable, got: %q", warnBuf.String())
	}
}

func TestCache_Concurrent_OneNetworkCallUnderFlock(t *testing.T) {
	cfgDir := t.TempDir()
	now := time.Now()

	// No cache on disk initially (so every goroutine would want to fetch).
	var fetchCalls atomic.Int32
	fetcher := &stubFetcherFunc{
		fn: func(ctx context.Context, orgID, tok string) (*teamtemplate.FetchResult, error) {
			fetchCalls.Add(1)
			time.Sleep(50 * time.Millisecond) // hold the lock long enough to queue siblings
			return &teamtemplate.FetchResult{Version: 1, Template: "T"}, nil
		},
	}

	const goroutines = 5
	var wg sync.WaitGroup
	errs := make([]error, goroutines)
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		go func() {
			defer wg.Done()
			c := newCache(cfgDir, func() time.Time { return now })
			_, errs[i] = c.Load(context.Background(), fetcher, "org-1", "tok", false, nil)
		}()
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("goroutine %d error: %v", i, err)
		}
	}

	// Under flock, only one goroutine should call the backend; the rest should
	// find a fresh cache after the lock is released.
	if got := fetchCalls.Load(); got != 1 {
		t.Errorf("fetch calls = %d, want 1 (flock serialization failed)", got)
	}
}

// stubFetcherFunc is a Fetcher backed by a plain function, for the concurrent test.
type stubFetcherFunc struct {
	fn func(ctx context.Context, orgID, accessToken string) (*teamtemplate.FetchResult, error)
}

func (s *stubFetcherFunc) GetTeamVault(ctx context.Context, orgID, accessToken string) (*teamtemplate.FetchResult, error) {
	return s.fn(ctx, orgID, accessToken)
}
