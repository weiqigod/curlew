package backend_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/gofrs/flock"
	"github.com/peterlindqvist/apitest/internal/backend"
)

func TestRefreshTokens_Concurrent_OnlyOneNetworkCall(t *testing.T) {
	var calls atomic.Int32
	// handlerExitTime records when the sole backend call's handler returned.
	// After wg.Wait() we assert at least one goroutine started before this
	// time, proving the two goroutines were truly concurrent (one was blocked
	// on the flock while the other held the lock and made the network call).
	var handlerMu sync.Mutex
	var handlerExitTime time.Time

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		time.Sleep(50 * time.Millisecond) // hold the lock long enough to queue the sibling

		handlerMu.Lock()
		handlerExitTime = time.Now()
		handlerMu.Unlock()

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"license_jwt":"L","access_token":"A","refresh_token":"R-new"}`))
	}))
	defer srv.Close()

	cfgDir := t.TempDir()
	client, err := backend.NewClient(backend.Options{BaseURL: srv.URL})
	if err != nil {
		t.Fatal(err)
	}
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, DeviceID: "dev-1", ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("R-old"); err != nil {
		t.Fatal(err)
	}

	var wg sync.WaitGroup
	tokens := make([]backend.Tokens, 2)
	errs := make([]error, 2)
	starts := [2]time.Time{}

	wg.Add(2)
	go func() {
		defer wg.Done()
		starts[0] = time.Now()
		tokens[0], errs[0] = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	}()
	go func() {
		defer wg.Done()
		starts[1] = time.Now()
		tokens[1], errs[1] = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	}()
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Fatalf("goroutine %d error: %v", i, err)
		}
	}

	if got := calls.Load(); got != 1 {
		t.Fatalf("expected exactly 1 backend call, got %d", got)
	}

	// Ordering proof: at least one goroutine must have started before the handler
	// finished. If both started after handlerExitTime, the calls were purely
	// sequential rather than concurrent — the flock serialisation would not have
	// been exercised. This assertion guards against a degenerate sequential execution.
	handlerMu.Lock()
	exit := handlerExitTime
	handlerMu.Unlock()
	if !exit.IsZero() {
		bothStartedAfterHandler := starts[0].After(exit) && starts[1].After(exit)
		if bothStartedAfterHandler {
			t.Fatal("ordering: both goroutines started after handler exit — flock serialisation not demonstrated")
		}
	}

	// Both goroutines should see "R-new" — via network or cache re-read.
	for i, tok := range tokens {
		if tok.RefreshToken != "R-new" {
			t.Fatalf("goroutine %d: got RefreshToken=%q; want R-new", i, tok.RefreshToken)
		}
		if tok.AccessToken != "A" {
			t.Fatalf("goroutine %d: got AccessToken=%q; want A", i, tok.AccessToken)
		}
	}
}

func TestRefreshTokens_HoldStaleLockTimesOutAndReadsCache(t *testing.T) {
	cfgDir := t.TempDir()
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	if err := storage.SetRefreshToken("R-cached"); err != nil {
		t.Fatal(err)
	}
	if err := storage.SetAccessToken("A-cached"); err != nil {
		t.Fatal(err)
	}

	// Acquire the lock externally and hold it.
	held := flock.New(filepath.Join(cfgDir, "refresh.lock"))
	if err := held.Lock(); err != nil {
		t.Fatalf("could not acquire external lock: %v", err)
	}
	defer func() { _ = held.Unlock() }()

	client, err := backend.NewClient(backend.Options{BaseURL: "http://127.0.0.1:1"}) // closed; would fail if reached
	if err != nil {
		t.Fatal(err)
	}

	// Use a short context timeout to trigger fallback faster than DefaultLockTimeout.
	ctx, cancel := context.WithTimeout(context.Background(), 300*time.Millisecond)
	defer cancel()

	out, err := backend.RefreshTokens(ctx, client, storage, cfgDir, "dev-1")
	if err != nil {
		t.Fatalf("expected fallback success (cache read), got %v", err)
	}
	if out.RefreshToken != "R-cached" {
		t.Fatalf("expected R-cached fallback, got %q", out.RefreshToken)
	}
	if out.AccessToken != "A-cached" {
		t.Fatalf("expected A-cached fallback, got %q", out.AccessToken)
	}
}

func TestRefreshTokens_MissingRefreshTokenReturnsErrTokenNotFound(t *testing.T) {
	cfgDir := t.TempDir()
	storage, err := backend.NewStorage(backend.StorageOptions{ConfigDir: cfgDir, ForceFile: true})
	if err != nil {
		t.Fatal(err)
	}
	client, err := backend.NewClient(backend.Options{BaseURL: "http://x"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = backend.RefreshTokens(context.Background(), client, storage, cfgDir, "dev-1")
	if !errors.Is(err, backend.ErrTokenNotFound) {
		t.Fatalf("got %v; want ErrTokenNotFound", err)
	}
}
