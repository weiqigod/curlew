package mudflat

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// fakeClock lets the TTL and eviction tests run without sleeping. Wall-clock in
// a test that asserts on expiry is how a suite becomes flaky; the spec's
// determinism invariant (§6.1) applies to the server's own tests too.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func newFakeClock() *fakeClock {
	return &fakeClock{now: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestSessions(t *testing.T) (*Sessions, *fakeClock) {
	t.Helper()
	clk := newFakeClock()
	return NewSessions(SessionOptions{TTL: time.Hour, Max: 1000, Now: clk.Now}), clk
}

func TestSessions_CreatedImplicitlyOnFirstUse(t *testing.T) {
	s, _ := newTestSessions(t)

	got, err := s.Get("abc")
	if err != nil {
		t.Fatalf("Get(abc) = %v, want implicit creation", err)
	}
	if got.ID != "abc" {
		t.Errorf("ID = %q, want %q", got.ID, "abc")
	}
	if n := s.Len(); n != 1 {
		t.Errorf("Len() = %d, want 1", n)
	}
}

func TestSessions_SameIDReturnsSameSession(t *testing.T) {
	s, _ := newTestSessions(t)

	a, err := s.Get("abc")
	if err != nil {
		t.Fatalf("first Get: %v", err)
	}
	a.Attempt("k")

	b, err := s.Get("abc")
	if err != nil {
		t.Fatalf("second Get: %v", err)
	}
	if got := b.Attempt("k"); got != 2 {
		t.Errorf("Attempt on re-fetched session = %d, want 2 (state not shared)", got)
	}
}

func TestSessions_AreIsolated(t *testing.T) {
	s, _ := newTestSessions(t)

	a, _ := s.Get("one")
	b, _ := s.Get("two")

	a.Attempt("k")
	a.Attempt("k")

	if got := b.Attempt("k"); got != 1 {
		t.Errorf("session two saw session one's counter: Attempt = %d, want 1", got)
	}

	if _, err := a.CreateResource([]byte(`{"n":1}`)); err != nil {
		t.Fatalf("CreateResource: %v", err)
	}
	if got := len(b.ListResources()); got != 0 {
		t.Errorf("session two sees %d resources from session one, want 0", got)
	}
}

func TestSessions_InvalidIDRejected(t *testing.T) {
	s, _ := newTestSessions(t)

	tests := []struct {
		name string
		id   string
	}{
		{"empty", ""},
		{"slash", "a/b"},
		{"dot", "a.b"},
		{"space", "a b"},
		{"too long", string(make([]byte, 65))},
		{"percent", "a%b"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if _, err := s.Get(tc.id); err == nil {
				t.Errorf("Get(%q) = nil error, want rejection", tc.id)
			}
		})
	}

	valid := []string{"a", "ABC123", "with-dash", "with_underscore", string(make([]byte, 0)) + "x"}
	for _, id := range valid {
		if _, err := s.Get(id); err != nil {
			t.Errorf("Get(%q) = %v, want accepted", id, err)
		}
	}
}

func TestSessions_ResetEmptiesButKeepsIdentity(t *testing.T) {
	s, _ := newTestSessions(t)

	sess, _ := s.Get("abc")
	sess.Attempt("k")
	if _, err := sess.CreateResource([]byte(`{}`)); err != nil {
		t.Fatalf("CreateResource: %v", err)
	}

	s.Reset("abc")

	after, _ := s.Get("abc")
	if got := after.Attempt("k"); got != 1 {
		t.Errorf("Attempt after reset = %d, want 1", got)
	}
	if got := len(after.ListResources()); got != 0 {
		t.Errorf("resources after reset = %d, want 0", got)
	}
}

func TestSessions_ResetIsIdempotent(t *testing.T) {
	s, _ := newTestSessions(t)
	s.Reset("never-seen")
	s.Reset("never-seen")
	if n := s.Len(); n != 0 {
		t.Errorf("Len() = %d after resetting an unknown session, want 0", n)
	}
}

func TestSessions_ExpireAfterTTL(t *testing.T) {
	s, clk := newTestSessions(t)

	sess, _ := s.Get("abc")
	sess.Attempt("k")

	clk.Advance(time.Hour + time.Second)

	revived, _ := s.Get("abc")
	if got := revived.Attempt("k"); got != 1 {
		t.Errorf("Attempt after TTL expiry = %d, want 1 (session should be fresh)", got)
	}
}

func TestSessions_TTLIsRefreshedByUse(t *testing.T) {
	s, clk := newTestSessions(t)

	sess, _ := s.Get("abc")
	sess.Attempt("k")

	// Three-quarters of the way through, touch it again.
	clk.Advance(45 * time.Minute)
	if _, err := s.Get("abc"); err != nil {
		t.Fatalf("Get: %v", err)
	}

	// Another 45 minutes: 90 total, but only 45 since last use.
	clk.Advance(45 * time.Minute)
	again, _ := s.Get("abc")
	if got := again.Attempt("k"); got != 2 {
		t.Errorf("Attempt = %d, want 2 — use should refresh the TTL", got)
	}
}

func TestSessions_EvictsLeastRecentlyUsedAtCapacity(t *testing.T) {
	clk := newFakeClock()
	s := NewSessions(SessionOptions{TTL: time.Hour, Max: 3, Now: clk.Now})

	for _, id := range []string{"a", "b", "c"} {
		sess, _ := s.Get(id)
		sess.Attempt("k")
		clk.Advance(time.Minute)
	}

	// Touch "a" so "b" becomes least-recently-used.
	if _, err := s.Get("a"); err != nil {
		t.Fatalf("Get(a): %v", err)
	}
	clk.Advance(time.Minute)

	sess, _ := s.Get("d")
	sess.Attempt("k")

	if n := s.Len(); n != 3 {
		t.Errorf("Len() = %d, want 3 (capacity)", n)
	}
	if s.Evictions() != 1 {
		t.Errorf("Evictions() = %d, want 1", s.Evictions())
	}

	// "b" should be gone: fetching it yields a fresh session.
	b, _ := s.Get("b")
	if got := b.Attempt("k"); got != 1 {
		t.Errorf("b.Attempt = %d, want 1 — b should have been evicted", got)
	}

	// "a" should have survived, because it was touched.
	a, _ := s.Get("a")
	if got := a.Attempt("k"); got != 2 {
		t.Errorf("a.Attempt = %d, want 2 — a was touched and should not be evicted", got)
	}
}

func TestSession_AttemptCountsPerKey(t *testing.T) {
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	if got := sess.Attempt("one"); got != 1 {
		t.Errorf("first Attempt(one) = %d, want 1", got)
	}
	if got := sess.Attempt("one"); got != 2 {
		t.Errorf("second Attempt(one) = %d, want 2", got)
	}
	if got := sess.Attempt("two"); got != 1 {
		t.Errorf("Attempt(two) = %d, want 1 — counters are per key", got)
	}
}

func TestSession_ResourceIDsAreDeterministic(t *testing.T) {
	// The spec (§9.H) requires ids derived from (session, sequence) so a failing
	// assertion quotes a stable value and a rerun reproduces it.
	for run := range 2 {
		s, _ := newTestSessions(t)
		sess, _ := s.Get("fixed")

		var ids []string
		for range 3 {
			r, err := sess.CreateResource([]byte(`{"n":1}`))
			if err != nil {
				t.Fatalf("run %d: CreateResource: %v", run, err)
			}
			ids = append(ids, r.ID)
		}

		want := []string{"res_fixed_1", "res_fixed_2", "res_fixed_3"}
		for i := range want {
			if ids[i] != want[i] {
				t.Errorf("run %d: id[%d] = %q, want %q", run, i, ids[i], want[i])
			}
		}
	}
}

func TestSession_ResourceLifecycle(t *testing.T) {
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	created, err := sess.CreateResource([]byte(`{"name":"first"}`))
	if err != nil {
		t.Fatalf("CreateResource: %v", err)
	}

	got, ok := sess.GetResource(created.ID)
	if !ok {
		t.Fatalf("GetResource(%q) not found after create", created.ID)
	}
	if string(got.Body) != `{"name":"first"}` {
		t.Errorf("body = %q, want the created body", got.Body)
	}
	if got.ETag == "" {
		t.Error("ETag is empty; conditional requests need one")
	}

	updated, ok := sess.UpdateResource(created.ID, []byte(`{"name":"second"}`))
	if !ok {
		t.Fatal("UpdateResource returned not-found for an existing id")
	}
	if updated.ETag == created.ETag {
		t.Error("ETag did not change after update; If-None-Match would wrongly 304")
	}

	if !sess.DeleteResource(created.ID) {
		t.Fatal("DeleteResource returned false for an existing id")
	}
	if _, ok := sess.GetResource(created.ID); ok {
		t.Error("GetResource found a deleted resource")
	}
	if sess.DeleteResource(created.ID) {
		t.Error("DeleteResource returned true for an already-deleted id")
	}
}

func TestSession_ETagIsContentAddressed(t *testing.T) {
	s, _ := newTestSessions(t)
	a, _ := s.Get("one")
	b, _ := s.Get("two")

	ra, _ := a.CreateResource([]byte(`{"same":true}`))
	rb, _ := b.CreateResource([]byte(`{"same":true}`))

	if ra.ETag != rb.ETag {
		t.Errorf("identical bodies produced different ETags: %q vs %q", ra.ETag, rb.ETag)
	}
}

func TestSession_ListResourcesIsOrderedByCreation(t *testing.T) {
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	for i := range 5 {
		if _, err := sess.CreateResource(fmt.Appendf(nil, `{"n":%d}`, i)); err != nil {
			t.Fatalf("CreateResource: %v", err)
		}
	}

	list := sess.ListResources()
	if len(list) != 5 {
		t.Fatalf("ListResources len = %d, want 5", len(list))
	}
	for i, r := range list {
		want := fmt.Sprintf("res_abc_%d", i+1)
		if r.ID != want {
			t.Errorf("list[%d].ID = %q, want %q", i, r.ID, want)
		}
	}
}

func TestSession_IdempotencyKeyReplaysFirstResult(t *testing.T) {
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	first, replayed := sess.Idempotent("key-1", func() []byte { return []byte("first") })
	if replayed {
		t.Error("first call reported as a replay")
	}
	if string(first) != "first" {
		t.Errorf("first = %q, want %q", first, "first")
	}

	second, replayed := sess.Idempotent("key-1", func() []byte { return []byte("second") })
	if !replayed {
		t.Error("second call with the same key was not reported as a replay")
	}
	if string(second) != "first" {
		t.Errorf("replay = %q, want the first result %q", second, "first")
	}

	other, replayed := sess.Idempotent("key-2", func() []byte { return []byte("other") })
	if replayed {
		t.Error("a different key was reported as a replay")
	}
	if string(other) != "other" {
		t.Errorf("other = %q, want %q", other, "other")
	}
}

func TestSessions_ConcurrentUseIsSafe(t *testing.T) {
	// Runs under -race in the gate. The isolation invariant (§6.2) is only
	// meaningful if the store itself is safe under concurrency.
	s, _ := newTestSessions(t)

	var wg sync.WaitGroup
	for i := range 16 {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			id := fmt.Sprintf("sess-%d", i%4)
			sess, err := s.Get(id)
			if err != nil {
				t.Errorf("Get(%q): %v", id, err)
				return
			}
			sess.Attempt("k")
			if _, err := sess.CreateResource([]byte(`{}`)); err != nil {
				t.Errorf("CreateResource: %v", err)
			}
			sess.ListResources()
		}(i)
	}
	wg.Wait()

	if n := s.Len(); n != 4 {
		t.Errorf("Len() = %d, want 4 distinct sessions", n)
	}
}

func TestSession_DocumentStoreRoundTrips(t *testing.T) {
	// The ETag endpoint needs one mutable document per session. Resources are
	// sequence-addressed, so they cannot serve a fixed well-known slot.
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	if _, ok := sess.Doc("etag"); ok {
		t.Error("Doc returned present for a slot never written")
	}

	sess.SetDoc("etag", []byte(`{"v":1}`))
	got, ok := sess.Doc("etag")
	if !ok {
		t.Fatal("Doc reported absent after SetDoc")
	}
	if string(got) != `{"v":1}` {
		t.Errorf("Doc = %q, want the stored bytes", got)
	}

	sess.SetDoc("etag", []byte(`{"v":2}`))
	got, _ = sess.Doc("etag")
	if string(got) != `{"v":2}` {
		t.Errorf("Doc after overwrite = %q, want the second value", got)
	}

	if _, ok := sess.Doc("other"); ok {
		t.Error("a different slot returned the first slot's value")
	}
}

func TestSession_DocumentStoreIsClearedByReset(t *testing.T) {
	s, _ := newTestSessions(t)

	sess, _ := s.Get("abc")
	sess.SetDoc("etag", []byte("x"))
	s.Reset("abc")

	after, _ := s.Get("abc")
	if _, ok := after.Doc("etag"); ok {
		t.Error("document survived a session reset")
	}
}

func TestSession_DocumentCopiesOnReadAndWrite(t *testing.T) {
	s, _ := newTestSessions(t)
	sess, _ := s.Get("abc")

	original := []byte(`{"v":1}`)
	sess.SetDoc("etag", original)
	original[2] = 'X'

	got, _ := sess.Doc("etag")
	if string(got) != `{"v":1}` {
		t.Errorf("stored document = %q; mutating the caller's slice changed it", got)
	}

	got[2] = 'Y'
	again, _ := sess.Doc("etag")
	if string(again) != `{"v":1}` {
		t.Errorf("stored document = %q; mutating the returned slice changed it", again)
	}
}
