package runner

import (
	"context"
	"testing"

	"github.com/weiqigod/curlew/internal/httpexec"
	"github.com/weiqigod/curlew/internal/parser"
)

// TestRun_Parallel_ResultsCarryRequestIDs locks the contract that parallel
// RequestResults carry the same RequestID/RequestSlug pairing as the events
// stream — consumers (curlew ui) reconcile authoritative results against
// event-collected state by request id, and an empty id creates phantom
// entries.
func TestRun_Parallel_ResultsCarryRequestIDs(t *testing.T) {
	col := &parser.Collection{
		Name: "P",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "One", Slug: "one", Request: parser.Request{Method: "GET", URL: "http://x.test/1"}},
			{
				Name: "Two", Slug: "two", Request: parser.Request{Method: "GET", URL: "http://x.test/2"},
				DependsOn: []string{"One"},
			},
		}},
	}
	exec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Body: []byte("{}")}, nil
	}
	sink := &recordingSink{}
	results, _, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
		OnEvent:  sink,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	eventIDs := map[string]string{} // slug → request id from events
	for _, e := range sink.ends {
		eventIDs[e.RequestSlug] = e.RequestID
	}
	for _, rr := range results {
		if rr.RequestSlug == "" {
			t.Errorf("result %q has empty RequestSlug", rr.Name)
			continue
		}
		if rr.RequestID == "" {
			t.Errorf("result %q has empty RequestID", rr.Name)
			continue
		}
		if want := eventIDs[rr.RequestSlug]; want != rr.RequestID {
			t.Errorf("result %q RequestID = %q, events emitted %q", rr.Name, rr.RequestID, want)
		}
	}
}
