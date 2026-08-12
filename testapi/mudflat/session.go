package mudflat

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sync"
	"time"
)

// ErrInvalidSessionID is returned for an id outside the character set the
// specification allows. Sessions are created implicitly, so a typo would
// otherwise become a silently-empty namespace rather than an error.
var ErrInvalidSessionID = errors.New("invalid session id")

const (
	defaultSessionTTL = time.Hour
	defaultSessionMax = 1000
	maxSessionIDLen   = 64
)

// Resource is one stored object in a session's collection. Bodies are held as
// raw bytes, never re-serialised, so what a client stored is what it reads back.
type Resource struct {
	ID   string
	Seq  int
	Body []byte
	ETag string
}

// Session holds all mutable state for one caller. Every field is scoped here,
// which is what makes the isolation invariant (§6.2) structural rather than a
// matter of handler discipline.
type Session struct {
	ID string

	mu        sync.Mutex
	attempts  map[string]int
	resources map[string]*Resource
	order     []string
	seq       int
	idem      map[string][]byte
	docs      map[string][]byte
}

func newSession(id string) *Session {
	return &Session{
		ID:        id,
		attempts:  make(map[string]int),
		resources: make(map[string]*Resource),
		idem:      make(map[string][]byte),
		docs:      make(map[string][]byte),
	}
}

// Attempt increments and returns the 1-based attempt count for key. Failure
// injection keys on (endpoint, query) so two different flaky endpoints in one
// session do not share a sequence.
func (s *Session) Attempt(key string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.attempts[key]++
	return s.attempts[key]
}

// CreateResource stores body and returns the resource. The id is derived from
// (session, sequence) rather than randomly, so a failing assertion quotes a
// value a rerun reproduces.
//
// Returns a copy rather than the stored pointer. Handing back the pointer let a
// later UpdateResource mutate what the caller was still holding, so a test
// comparing an ETag before and after an update saw the same value twice.
func (s *Session) CreateResource(body []byte) (Resource, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	s.seq++
	stored := make([]byte, len(body))
	copy(stored, body)

	r := &Resource{
		ID:   fmt.Sprintf("res_%s_%d", s.ID, s.seq),
		Seq:  s.seq,
		Body: stored,
		ETag: etagFor(stored),
	}
	s.resources[r.ID] = r
	s.order = append(s.order, r.ID)
	return *r, nil
}

// GetResource returns a copy, so a caller cannot mutate stored state by holding
// the returned value.
func (s *Session) GetResource(id string) (Resource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	r, ok := s.resources[id]
	if !ok {
		return Resource{}, false
	}
	return *r, true
}

// UpdateResource replaces the body and recomputes the ETag. A stale ETag is how
// a conditional-request test silently passes, so the recomputation is the point.
func (s *Session) UpdateResource(id string, body []byte) (Resource, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	r, ok := s.resources[id]
	if !ok {
		return Resource{}, false
	}
	stored := make([]byte, len(body))
	copy(stored, body)
	r.Body = stored
	r.ETag = etagFor(stored)
	return *r, true
}

func (s *Session) DeleteResource(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()

	if _, ok := s.resources[id]; !ok {
		return false
	}
	delete(s.resources, id)
	for i, existing := range s.order {
		if existing == id {
			s.order = append(s.order[:i], s.order[i+1:]...)
			break
		}
	}
	return true
}

// ListResources returns resources in creation order. Map iteration order would
// make pagination non-deterministic, which §6.1 forbids.
func (s *Session) ListResources() []Resource {
	s.mu.Lock()
	defer s.mu.Unlock()

	out := make([]Resource, 0, len(s.order))
	for _, id := range s.order {
		if r, ok := s.resources[id]; ok {
			out = append(out, *r)
		}
	}
	return out
}

// Idempotent runs produce for a key exactly once. The second and later calls
// replay the first result and report replayed=true.
func (s *Session) Idempotent(key string, produce func() []byte) (body []byte, replayed bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if stored, ok := s.idem[key]; ok {
		out := make([]byte, len(stored))
		copy(out, stored)
		return out, true
	}

	produced := produce()
	stored := make([]byte, len(produced))
	copy(stored, produced)
	s.idem[key] = stored
	return produced, false
}

func etagFor(body []byte) string {
	sum := sha256.Sum256(body)
	return `"` + hex.EncodeToString(sum[:])[:32] + `"`
}

// SessionOptions configures a Sessions store. The zero value of each field
// selects the specification's default.
type SessionOptions struct {
	TTL time.Duration
	Max int
	// Now is injectable so TTL and eviction can be tested without sleeping.
	Now func() time.Time
}

type sessionEntry struct {
	sess     *Session
	lastUsed time.Time
}

// Sessions is the session store: a TTL'd, LRU-bounded map of independent
// namespaces.
type Sessions struct {
	mu        sync.Mutex
	entries   map[string]*sessionEntry
	ttl       time.Duration
	max       int
	now       func() time.Time
	evictions int
}

func NewSessions(opts SessionOptions) *Sessions {
	if opts.TTL <= 0 {
		opts.TTL = defaultSessionTTL
	}
	if opts.Max <= 0 {
		opts.Max = defaultSessionMax
	}
	if opts.Now == nil {
		opts.Now = time.Now
	}
	return &Sessions{
		entries: make(map[string]*sessionEntry),
		ttl:     opts.TTL,
		max:     opts.Max,
		now:     opts.Now,
	}
}

// Get returns the session for id, creating it if absent. An expired session is
// replaced rather than revived, so a suite that outruns the TTL sees a clean
// namespace instead of stale counters.
func (s *Sessions) Get(id string) (*Session, error) {
	if err := ValidateSessionID(id); err != nil {
		return nil, err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	now := s.now()
	s.purgeExpiredLocked(now)

	if entry, ok := s.entries[id]; ok {
		entry.lastUsed = now
		return entry.sess, nil
	}

	if len(s.entries) >= s.max {
		s.evictOldestLocked()
	}

	entry := &sessionEntry{sess: newSession(id), lastUsed: now}
	s.entries[id] = entry
	return entry.sess, nil
}

// Reset discards a session's state. It is idempotent: resetting an unknown
// session succeeds and creates nothing.
func (s *Sessions) Reset(id string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.entries, id)
}

func (s *Sessions) Len() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return len(s.entries)
}

// Evictions reports how many sessions were dropped for capacity. Surfaced in
// /capabilities so a suite that trips the cap can find out rather than seeing
// mysterious state resets.
func (s *Sessions) Evictions() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.evictions
}

func (s *Sessions) purgeExpiredLocked(now time.Time) {
	for id, entry := range s.entries {
		if now.Sub(entry.lastUsed) > s.ttl {
			delete(s.entries, id)
		}
	}
}

func (s *Sessions) evictOldestLocked() {
	var oldestID string
	var oldest time.Time
	for id, entry := range s.entries {
		if oldestID == "" || entry.lastUsed.Before(oldest) {
			oldestID, oldest = id, entry.lastUsed
		}
	}
	if oldestID != "" {
		delete(s.entries, oldestID)
		s.evictions++
	}
}

// ValidateSessionID enforces the 1–64 character [A-Za-z0-9_-] rule from §7.
func ValidateSessionID(id string) error {
	if id == "" {
		return fmt.Errorf("%w: empty", ErrInvalidSessionID)
	}
	if len(id) > maxSessionIDLen {
		return fmt.Errorf("%w: %d characters, maximum %d", ErrInvalidSessionID, len(id), maxSessionIDLen)
	}
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= 'a' && c <= 'z',
			c >= 'A' && c <= 'Z',
			c >= '0' && c <= '9',
			c == '-', c == '_':
		default:
			return fmt.Errorf("%w: character %q at position %d", ErrInvalidSessionID, string(c), i)
		}
	}
	return nil
}

// Doc returns the document stored at name. The ETag endpoint needs one mutable
// slot per session, which resources cannot provide: their ids are
// sequence-addressed, so there is no fixed well-known key.
//
// Returns a copy. Handing back the stored slice would let a caller mutate
// session state through a read, which is the same class of bug CreateResource
// had.
func (s *Session) Doc(name string) ([]byte, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored, ok := s.docs[name]
	if !ok {
		return nil, false
	}
	out := make([]byte, len(stored))
	copy(out, stored)
	return out, true
}

// SetDoc stores body at name, copying it so a later mutation by the caller does
// not reach into the session.
func (s *Session) SetDoc(name string, body []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()

	stored := make([]byte, len(body))
	copy(stored, body)
	s.docs[name] = stored
}
