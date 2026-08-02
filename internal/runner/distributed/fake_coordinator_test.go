package distributed_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/peterlindqvist/apitest/internal/worker"
)

// fakeCoordinatorOpts configures the fake coordinator.
type fakeCoordinatorOpts struct {
	org   string
	token string // if non-empty, validates Authorization header

	// maxWorkerCount: if > 0, worker_count is capped — never advances past this.
	maxWorkerCount int
	// finalWorkerCount is the worker count the fake will report once stable.
	finalWorkerCount int
	// reassignActive causes the shard at reassignShardIndex to report
	// "reassigned" on the first poll, then "completed" on subsequent polls.
	reassignShardIndex int
	reassignActive     bool
}

// fakeCoordinatorState records coordinator interactions.
type fakeCoordinatorState struct {
	PollCount  int
	jobCreated *createJobCapture
}

// createJobCapture holds the body received by POST /jobs.
type createJobCapture struct {
	CollectionSha string `json:"collection_sha"`
	ShardCount    int    `json:"shard_count"`
}

// shardPayloadWire is used only for decoding in the fake.
type shardPayloadWire struct {
	Index        int    `json:"index"`
	RequestsJson string `json:"requests_json"`
}

// shardStatusWire mirrors ShardStatus for the fake's JSON encoding.
type shardStatusWire struct {
	ShardID    string              `json:"shard_id"`
	ShardIndex int                 `json:"shard_index"`
	State      string              `json:"state"`
	Items      []worker.SubmitItem `json:"items,omitempty"`
	PassCount  int                 `json:"pass_count,omitempty"`
	FailCount  int                 `json:"fail_count,omitempty"`
}

// jobResponseWire mirrors JobResponse for the fake's JSON encoding.
type jobResponseWire struct {
	JobID       string            `json:"job_id"`
	State       string            `json:"state"`
	ShardCount  int               `json:"shard_count"`
	Shards      []shardStatusWire `json:"shards"`
	WorkerCount int               `json:"worker_count,omitempty"`
}

// startFakeCoordinator starts an httptest server for the distributed runner tests.
// The fake is deterministic and poll-count-driven: on first GET, all shards are
// in their initial state (plus optional reassignment); on subsequent GETs, all
// reassigned shards transition to completed.
func startFakeCoordinator(
	t *testing.T,
	opts fakeCoordinatorOpts,
) (*httptest.Server, *fakeCoordinatorState) {
	t.Helper()

	state := &fakeCoordinatorState{}
	var mu sync.Mutex
	var shardCount int
	var shards []shardPayloadWire
	pollCount := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// Auth check.
		if opts.token != "" && r.Header.Get("Authorization") != "Bearer "+opts.token {
			w.WriteHeader(401)
			return
		}

		path := r.URL.Path

		switch {
		// POST /api/v1/organizations/{org}/coordinator/jobs
		case strings.Contains(path, "/coordinator/jobs") && !strings.Contains(path, "/jobs/") && r.Method == "POST":
			var body struct {
				CollectionSha string             `json:"collection_sha"`
				ShardCount    int                `json:"shard_count"`
				Shards        []shardPayloadWire `json:"shards"`
				Variables     map[string]string  `json:"variables"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)

			mu.Lock()
			shardCount = body.ShardCount
			shards = body.Shards
			state.jobCreated = &createJobCapture{
				CollectionSha: body.CollectionSha,
				ShardCount:    body.ShardCount,
			}
			mu.Unlock()

			resp := map[string]interface{}{
				"job_id":      "job_fake",
				"state":       "pending",
				"shard_count": body.ShardCount,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(resp)

		// GET /api/v1/organizations/{org}/coordinator/jobs/{jobID}
		case strings.Contains(path, "/coordinator/jobs/") && r.Method == "GET":
			mu.Lock()
			pollCount++
			state.PollCount = pollCount
			currentPollCount := pollCount
			mu.Unlock()

			workerCount := opts.finalWorkerCount
			if workerCount == 0 {
				workerCount = shardCount
			}
			if opts.maxWorkerCount > 0 && workerCount > opts.maxWorkerCount {
				workerCount = opts.maxWorkerCount
			}

			shardStatuses := make([]shardStatusWire, shardCount)
			for i := range shardStatuses {
				reqCount := 3
				if i < len(shards) {
					n := countRequestsInShard(shards[i])
					if n > 0 {
						reqCount = n
					}
				}

				shardState := "completed"
				var items []worker.SubmitItem
				pass, fail := 0, 0

				if opts.reassignActive && i == opts.reassignShardIndex {
					// Poll 1 (join check): complete so workers appear joined. This is the trick:
					// the Run code polls once for workers then polls for shards. We want the
					// first SHARD poll (poll 2) to show "reassigned", and poll 3+ to show "completed".
					if currentPollCount == 2 {
						// First shard-status poll: report reassigned (no items yet).
						shardState = "reassigned"
					} else {
						// Poll 1 (join check) or poll 3+ (after reassignment): completed.
						shardState = "completed"
						for j := 0; j < reqCount; j++ {
							items = append(items, worker.SubmitItem{Name: fakeName(i, j), Status: "pass"})
							pass++
						}
					}
				} else {
					for j := 0; j < reqCount; j++ {
						items = append(items, worker.SubmitItem{Name: fakeName(i, j), Status: "pass"})
						pass++
					}
				}

				shardStatuses[i] = shardStatusWire{
					ShardID:    fakeShardID(i),
					ShardIndex: i,
					State:      shardState,
					Items:      items,
					PassCount:  pass,
					FailCount:  fail,
				}
			}

			allDone := true
			for _, s := range shardStatuses {
				if s.State != "completed" {
					allDone = false
					break
				}
			}
			jobState := "running"
			if allDone {
				jobState = "completed"
			}

			resp := jobResponseWire{
				JobID:       "job_fake",
				State:       jobState,
				ShardCount:  shardCount,
				Shards:      shardStatuses,
				WorkerCount: workerCount,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(404)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, state
}

func fakeShardID(idx int) string {
	return fmt.Sprintf("shd_%d", idx+1)
}

func fakeName(shardIdx, reqIdx int) string {
	return fmt.Sprintf("req_s%d_r%d", shardIdx, reqIdx)
}

func countRequestsInShard(sp shardPayloadWire) int {
	if sp.RequestsJson == "" || sp.RequestsJson == "[]" {
		return 3
	}
	var reqs []interface{}
	if err := json.Unmarshal([]byte(sp.RequestsJson), &reqs); err != nil {
		return 3
	}
	return len(reqs)
}

// startFakeCoordinatorWithFailure creates a coordinator where shard 0 has
// failCount failing items.
func startFakeCoordinatorWithFailure(t *testing.T, org, token string, workerCount, failCount int) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	var shardCount int

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/coordinator/jobs") && !strings.Contains(path, "/jobs/") && r.Method == "POST":
			var body struct {
				ShardCount int `json:"shard_count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			shardCount = body.ShardCount
			mu.Unlock()
			resp := map[string]interface{}{
				"job_id":      "job_fake",
				"state":       "pending",
				"shard_count": body.ShardCount,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(resp)

		case strings.Contains(path, "/coordinator/jobs/") && r.Method == "GET":
			mu.Lock()
			sc := shardCount
			mu.Unlock()

			shardStatuses := make([]shardStatusWire, sc)
			for i := range shardStatuses {
				if i == 0 {
					// Shard 0 has failCount failures.
					items := make([]worker.SubmitItem, failCount)
					for j := range items {
						items[j] = worker.SubmitItem{Name: fmt.Sprintf("fail_%d", j), Status: "fail", Message: "HTTP 500"}
					}
					shardStatuses[i] = shardStatusWire{
						ShardID:    fakeShardID(i),
						ShardIndex: i,
						State:      "completed",
						Items:      items,
						PassCount:  0,
						FailCount:  failCount,
					}
				} else {
					shardStatuses[i] = shardStatusWire{
						ShardID:    fakeShardID(i),
						ShardIndex: i,
						State:      "completed",
						Items:      []worker.SubmitItem{{Name: fakeName(i, 0), Status: "pass"}},
						PassCount:  1,
						FailCount:  0,
					}
				}
			}
			resp := jobResponseWire{
				JobID:       "job_fake",
				State:       "completed",
				ShardCount:  sc,
				Shards:      shardStatuses,
				WorkerCount: workerCount,
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(404)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}

// startFakeCoordinatorNeverComplete starts a fake that always reports workers
// joined but shards never complete.
func startFakeCoordinatorNeverComplete(t *testing.T, org, token string) *httptest.Server {
	t.Helper()
	var mu sync.Mutex
	var shardCount int

	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if token != "" && r.Header.Get("Authorization") != "Bearer "+token {
			w.WriteHeader(401)
			return
		}
		path := r.URL.Path
		switch {
		case strings.Contains(path, "/coordinator/jobs") && !strings.Contains(path, "/jobs/") && r.Method == "POST":
			var body struct {
				ShardCount int `json:"shard_count"`
			}
			_ = json.NewDecoder(r.Body).Decode(&body)
			mu.Lock()
			shardCount = body.ShardCount
			mu.Unlock()
			resp := map[string]interface{}{
				"job_id":      "job_fake",
				"state":       "pending",
				"shard_count": body.ShardCount,
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(201)
			_ = json.NewEncoder(w).Encode(resp)

		case strings.Contains(path, "/coordinator/jobs/") && r.Method == "GET":
			mu.Lock()
			sc := shardCount
			mu.Unlock()
			// Workers have joined but shards are always "pending" (never complete).
			shards := make([]shardStatusWire, sc)
			for i := range shards {
				shards[i] = shardStatusWire{
					ShardID:    fakeShardID(i),
					ShardIndex: i,
					State:      "pending",
				}
			}
			resp := jobResponseWire{
				JobID:       "job_fake",
				State:       "running",
				ShardCount:  sc,
				Shards:      shards,
				WorkerCount: sc, // Workers joined.
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(resp)

		default:
			w.WriteHeader(404)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv
}
