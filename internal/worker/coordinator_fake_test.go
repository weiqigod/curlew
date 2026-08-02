package worker

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

// FakeShard describes one shard the fake coordinator will serve.
type FakeShard struct {
	ID           string
	Index        int
	RequestsJson string
}

// FakeOpts configures the fake coordinator.
type FakeOpts struct {
	Org    string
	Token  string // if non-empty, validates Authorization header
	Shards []FakeShard
}

// FakeState records coordinator interactions for assertions.
type FakeState struct {
	mu                sync.Mutex
	ClaimCount        int
	SubmitsByShard    map[string]*SubmitResultBody
	HeartbeatsByShard map[string]int
}

func (s *FakeState) recordSubmit(shardID string, body *SubmitResultBody) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.SubmitsByShard == nil {
		s.SubmitsByShard = make(map[string]*SubmitResultBody)
	}
	s.SubmitsByShard[shardID] = body
}

func (s *FakeState) recordHeartbeat(shardID string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.HeartbeatsByShard == nil {
		s.HeartbeatsByShard = make(map[string]int)
	}
	s.HeartbeatsByShard[shardID]++
}

// startFakeCoordinator starts an httptest server mimicking the M5-008 coordinator API.
// Returns the server and a FakeState that records interactions.
func startFakeCoordinator(t *testing.T, opts FakeOpts) (*httptest.Server, *FakeState) {
	t.Helper()
	state := &FakeState{}
	var mu sync.Mutex
	claimIdx := 0

	mux := http.NewServeMux()

	// POST /api/v1/organizations/{org}/coordinator/jobs/{jobID}/claim
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Auth check
		if opts.Token != "" {
			auth := r.Header.Get("Authorization")
			if auth != "Bearer "+opts.Token {
				w.WriteHeader(401)
				return
			}
		}

		path := r.URL.Path

		switch {
		case strings.HasSuffix(path, "/claim") && r.Method == "POST":
			mu.Lock()
			state.ClaimCount++
			if claimIdx >= len(opts.Shards) {
				mu.Unlock()
				w.WriteHeader(204)
				return
			}
			shard := opts.Shards[claimIdx]
			claimIdx++
			mu.Unlock()

			resp := ShardResponse{
				ShardID:      shard.ID,
				JobID:        extractSegment(path, "jobs", 1),
				ShardIndex:   shard.Index,
				State:        "claimed",
				RequestsJson: shard.RequestsJson,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(200)
			_ = json.NewEncoder(w).Encode(resp)

		case strings.Contains(path, "/shards/") && strings.HasSuffix(path, "/result") && r.Method == "POST":
			shardID := extractShardID(path)
			var body SubmitResultBody
			_ = json.NewDecoder(r.Body).Decode(&body)
			state.recordSubmit(shardID, &body)
			w.WriteHeader(202)

		case strings.Contains(path, "/shards/") && strings.HasSuffix(path, "/heartbeat") && r.Method == "POST":
			shardID := extractShardID(path)
			state.recordHeartbeat(shardID)
			w.WriteHeader(204)

		default:
			w.WriteHeader(404)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

// extractSegment returns the segment after the given key in a URL path.
// e.g. extractSegment("/api/v1/organizations/acme/coordinator/jobs/job_abc/claim", "jobs", 1) → "job_abc"
func extractSegment(path, key string, offset int) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == key && i+offset < len(parts) {
			return parts[i+offset]
		}
	}
	return ""
}

// extractShardID extracts the shard ID from a path like .../shards/{id}/result
func extractShardID(path string) string {
	parts := strings.Split(path, "/")
	for i, p := range parts {
		if p == "shards" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}
