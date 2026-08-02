package runner

import (
	"context"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/parser"
)

// TestRun_RequestIDPrefix verifies the additive VarSources.RequestIDPrefix:
// when non-empty, request ids are "<prefix>req-N" so multi-collection batch
// runs (apitest ui) never collide; when empty, the existing "req-N" format is
// unchanged.
func TestRun_RequestIDPrefix(t *testing.T) {
	col := &parser.Collection{
		Name: "P",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "One", Slug: "one", Request: parser.Request{Method: "GET", URL: "http://x.test/1"}},
			{Name: "Two", Slug: "two", Request: parser.Request{Method: "GET", URL: "http://x.test/2"}},
		}},
	}
	exec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Body: []byte("{}")}, nil
	}

	for _, tc := range []struct {
		prefix string
		want   string
	}{
		{prefix: "", want: "req-1"},
		{prefix: "c2-", want: "c2-req-1"},
	} {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, exec, VarSources{
			OnEvent:         sink,
			RequestIDPrefix: tc.prefix,
		})
		if err != nil {
			t.Fatalf("Run(prefix=%q): %v", tc.prefix, err)
		}
		if len(sink.ends) != 2 {
			t.Fatalf("prefix=%q: got %d request.end events, want 2", tc.prefix, len(sink.ends))
		}
		if sink.ends[0].RequestID != tc.want {
			t.Errorf("prefix=%q: first request id = %q, want %q", tc.prefix, sink.ends[0].RequestID, tc.want)
		}
		for _, e := range sink.ends {
			if !strings.HasPrefix(e.RequestID, tc.prefix+"req-") {
				t.Errorf("prefix=%q: request id %q lacks expected prefix", tc.prefix, e.RequestID)
			}
		}
	}
}
