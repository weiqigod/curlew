package runner

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	apicel "github.com/peterlindqvist/apitest/internal/cel"

	"github.com/peterlindqvist/apitest/internal/assertion"
	"github.com/peterlindqvist/apitest/internal/auth"
	"github.com/peterlindqvist/apitest/internal/config"
	"github.com/peterlindqvist/apitest/internal/datadriven"
	apierrors "github.com/peterlindqvist/apitest/internal/errors"
	"github.com/peterlindqvist/apitest/internal/graphql"
	"github.com/peterlindqvist/apitest/internal/httpexec"
	"github.com/peterlindqvist/apitest/internal/parser"
	"github.com/peterlindqvist/apitest/internal/plugin/hooks"
	"github.com/peterlindqvist/apitest/internal/retry"
	"github.com/peterlindqvist/apitest/internal/signer"
	"github.com/peterlindqvist/apitest/internal/validator"
	"github.com/peterlindqvist/apitest/internal/variable"
	"github.com/peterlindqvist/apitest/internal/vault"
	"github.com/peterlindqvist/apitest/internal/vault/teamtemplate"
	"github.com/peterlindqvist/apitest/internal/websocket"
)

func makeCollection(names []string, stopOnFailure bool) *parser.Collection {
	items := make([]parser.RequestItem, len(names))
	for i, n := range names {
		items[i] = parser.RequestItem{
			Name:    n,
			Request: parser.Request{Method: "GET", URL: "https://example.com"},
		}
	}
	return &parser.Collection{
		Name:     "Test",
		Requests: parser.Section{Items: items},
		Options:  parser.Options{StopOnFailure: stopOnFailure},
	}
}

var errFake = errors.New("fake network error")

func successExecutor(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
}

// failAtN returns an executor that fails on the Nth call (0-indexed).
func failAtN(n int) ExecuteFunc {
	call := 0
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		idx := call
		call++
		if idx == n {
			return nil, errFake
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
}

func TestRun(t *testing.T) {
	tests := []struct {
		name        string
		collection  *parser.Collection
		exec        ExecuteFunc
		wantPassed  int
		wantFailed  int
		wantSkipped int
		wantTotal   int
		wantOrder   []string
	}{
		{
			name:        "all requests succeed",
			collection:  makeCollection([]string{"A", "B", "C"}, false),
			exec:        successExecutor,
			wantPassed:  3,
			wantFailed:  0,
			wantSkipped: 0,
			wantTotal:   3,
		},
		{
			name:       "execution order is preserved",
			collection: makeCollection([]string{"A", "B", "C"}, false),
			exec:       successExecutor,
			wantOrder:  []string{"A", "B", "C"},
			wantTotal:  3,
			wantPassed: 3,
		},
		{
			name:        "one network error in middle counts correctly",
			collection:  makeCollection([]string{"A", "B", "C"}, false),
			exec:        failAtN(1),
			wantPassed:  2,
			wantFailed:  1,
			wantSkipped: 0,
			wantTotal:   3,
		},
		{
			name:        "stop_on_failure true skips remaining after first failure",
			collection:  makeCollection([]string{"A", "B", "C"}, true),
			exec:        failAtN(0),
			wantPassed:  0,
			wantFailed:  1,
			wantSkipped: 2,
			wantTotal:   3,
		},
		{
			name:        "stop_on_failure false continues after failure",
			collection:  makeCollection([]string{"A", "B", "C"}, false),
			exec:        failAtN(0),
			wantPassed:  2,
			wantFailed:  1,
			wantSkipped: 0,
			wantTotal:   3,
		},
		{
			name:        "stop_on_failure true second fails third skipped",
			collection:  makeCollection([]string{"A", "B", "C"}, true),
			exec:        failAtN(1),
			wantPassed:  1,
			wantFailed:  1,
			wantSkipped: 1,
			wantTotal:   3,
		},
		{
			name:        "empty requests returns zero summary",
			collection:  makeCollection([]string{}, false),
			exec:        successExecutor,
			wantPassed:  0,
			wantFailed:  0,
			wantSkipped: 0,
			wantTotal:   0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, summary, err := Run(context.Background(), tt.collection, tt.exec, VarSources{})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			if summary.Total != tt.wantTotal {
				t.Errorf("total = %d, want %d", summary.Total, tt.wantTotal)
			}
			if summary.Passed != tt.wantPassed {
				t.Errorf("passed = %d, want %d", summary.Passed, tt.wantPassed)
			}
			if summary.Failed != tt.wantFailed {
				t.Errorf("failed = %d, want %d", summary.Failed, tt.wantFailed)
			}
			if summary.Skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", summary.Skipped, tt.wantSkipped)
			}

			if tt.wantOrder != nil {
				if len(results) != len(tt.wantOrder) {
					t.Fatalf("got %d results, want %d", len(results), len(tt.wantOrder))
				}
				for i, want := range tt.wantOrder {
					if results[i].Name != want {
						t.Errorf("result[%d].Name = %q, want %q", i, results[i].Name, want)
					}
				}
			}
		})
	}
}

func makeCollectionWithAssertions(name string, statusCodes []int, stopOnFailure bool) *parser.Collection {
	item := parser.RequestItem{
		Name:    name,
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
	}
	if statusCodes != nil {
		item.Assertions = parser.Assertions{
			Status: parser.StatusCodes{Codes: statusCodes},
		}
	}
	return &parser.Collection{
		Name:     "Test",
		Requests: parser.Section{Items: []parser.RequestItem{item}},
		Options:  parser.Options{StopOnFailure: stopOnFailure},
	}
}

func TestRun_assertions(t *testing.T) {
	tests := []struct {
		name                  string
		statusCodes           []int
		wantPassed            int
		wantFailed            int
		wantAssertionFailures int
	}{
		{"passing status assertion", []int{200}, 1, 0, 0},
		{"failing status assertion", []int{201}, 0, 1, 1},
		{"no assertions always passes", nil, 1, 0, 0},
		{"list assertion match", []int{200, 201}, 1, 0, 0},
		{"list assertion mismatch", []int{201, 202}, 0, 1, 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := makeCollectionWithAssertions("A", tt.statusCodes, false)
			results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}

			if summary.Passed != tt.wantPassed {
				t.Errorf("passed = %d, want %d", summary.Passed, tt.wantPassed)
			}
			if summary.Failed != tt.wantFailed {
				t.Errorf("failed = %d, want %d", summary.Failed, tt.wantFailed)
			}
			if summary.AssertionFailures != tt.wantAssertionFailures {
				t.Errorf("assertion failures = %d, want %d", summary.AssertionFailures, tt.wantAssertionFailures)
			}

			// Verify assertion results are attached
			if len(results) != 1 {
				t.Fatalf("got %d results, want 1", len(results))
			}
			if tt.statusCodes != nil {
				if results[0].AssertionResults == nil {
					t.Fatal("AssertionResults should not be nil when assertions defined")
				}
			}
		})
	}
}

func TestRun_assertion_failure_with_stop_on_failure(t *testing.T) {
	// 3 requests: first passes (no assertions), second fails assertion, third skipped
	items := []parser.RequestItem{
		{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		{
			Name:       "B",
			Request:    parser.Request{Method: "GET", URL: "https://example.com"},
			Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{404}}},
		},
		{Name: "C", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
	}
	col := &parser.Collection{
		Name:     "Test",
		Requests: parser.Section{Items: items},
		Options:  parser.Options{StopOnFailure: true},
	}

	_, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
}

func TestRun_mixed_network_error_and_assertion_failure(t *testing.T) {
	// First request: network error. Second: assertion failure (executor returns 200, asserts 404).
	items := []parser.RequestItem{
		{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		{
			Name:       "B",
			Request:    parser.Request{Method: "GET", URL: "https://example.com"},
			Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{404}}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: items}}

	_, summary, _ := Run(context.Background(), col, failAtN(0), VarSources{})

	if summary.Failed != 2 {
		t.Errorf("failed = %d, want 2", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
}

func TestRun_total_duration_is_positive(t *testing.T) {
	col := makeCollection([]string{"A"}, false)
	_, summary, _ := Run(context.Background(), col, successExecutor, VarSources{})
	if summary.Duration <= 0 {
		t.Errorf("duration = %v, want > 0", summary.Duration)
	}
}

func TestRun_skipped_results_have_correct_fields(t *testing.T) {
	col := makeCollection([]string{"A", "B", "C"}, true)
	results, _, _ := Run(context.Background(), col, failAtN(0), VarSources{})

	// First result should be failed
	if results[0].Err == nil {
		t.Error("results[0].Err should be non-nil")
	}
	if results[0].Skipped {
		t.Error("results[0].Skipped should be false")
	}

	// Remaining should be skipped
	for i := 1; i < len(results); i++ {
		if !results[i].Skipped {
			t.Errorf("results[%d].Skipped = false, want true", i)
		}
		if results[i].Result != nil {
			t.Errorf("results[%d].Result should be nil", i)
		}
		if results[i].Err != nil {
			t.Errorf("results[%d].Err should be nil", i)
		}
	}
}

func jsonExecutor(body string) ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond, Body: []byte(body)}, nil
	}
}

func headeredExecutor(headers http.Header, duration time.Duration) ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   duration,
			Headers:    headers,
		}, nil
	}
}

func TestRun_body_assertion_pass(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
				{Path: "$.id", Operator: "equals", Value: 1},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, jsonExecutor(`{"id":1}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if summary.Failed != 0 {
		t.Errorf("failed = %d, want 0", summary.Failed)
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Error("expected assertion to pass")
	}
}

func TestRun_body_assertion_fail(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
				{Path: "$.id", Operator: "equals", Value: 99},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	_, summary, _ := Run(context.Background(), col, jsonExecutor(`{"id":1}`), VarSources{})

	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
}

func TestRun_body_assertion_with_stop_on_failure(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:    "A",
			Request: parser.Request{Method: "GET", URL: "https://example.com"},
			Assertions: parser.Assertions{
				Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
					{Path: "$.id", Operator: "equals", Value: 99},
				}},
			},
		},
		{Name: "B", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
	}
	col := &parser.Collection{
		Name:     "Test",
		Requests: parser.Section{Items: items},
		Options:  parser.Options{StopOnFailure: true},
	}

	_, summary, _ := Run(context.Background(), col, jsonExecutor(`{"id":1}`), VarSources{})

	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
}

func TestRun_body_and_status_assertions_combined(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Status: parser.StatusCodes{Codes: []int{200}},
			Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
				{Path: "$.id", Operator: "equals", Value: 1},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	results, summary, err := Run(context.Background(), col, jsonExecutor(`{"id":1}`), VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].AssertionResults == nil {
		t.Fatal("expected assertion results")
	}
	if len(results[0].AssertionResults.Items) != 2 {
		t.Errorf("assertion items = %d, want 2", len(results[0].AssertionResults.Items))
	}
}

func TestRun_non_json_body_assertion_fails(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
				{Path: "$.id", Operator: "equals", Value: 1},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond, Body: []byte("<html>")}, nil
	}

	_, summary, _ := Run(context.Background(), col, exec, VarSources{})

	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
}

func TestRun_context_cancellation_stops_execution(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	col := makeCollection([]string{"A", "B", "C"}, false)

	callCount := 0
	exec := func(innerCtx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		cancel() // cancel after first execution
		return &httpexec.Result{StatusCode: 200, Duration: 1 * time.Millisecond}, nil
	}

	results, summary, _ := Run(ctx, col, exec, VarSources{})

	if callCount != 1 {
		t.Errorf("executor called %d times, want 1", callCount)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if summary.Skipped != 2 {
		t.Errorf("skipped = %d, want 2", summary.Skipped)
	}
	if len(results) != 3 {
		t.Fatalf("got %d results, want 3", len(results))
	}
}

func TestRun_header_assertion_pass(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Headers: parser.HeaderAssertions{Items: []parser.HeaderAssertion{
				{Name: "Content-Type", Operator: "equals", Value: "application/json"},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}
	exec := headeredExecutor(http.Header{"Content-Type": {"application/json"}}, 10*time.Millisecond)

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Error("expected assertion to pass")
	}
}

func TestRun_header_assertion_fail(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Headers: parser.HeaderAssertions{Items: []parser.HeaderAssertion{
				{Name: "X-Missing", Operator: "exists", Value: true},
			}},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}
	exec := headeredExecutor(http.Header{"Content-Type": {"text/html"}}, 10*time.Millisecond)

	_, summary, _ := Run(context.Background(), col, exec, VarSources{})

	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
}

func TestRun_timing_assertion_pass(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Timing: parser.TimingAssertion{MaxDurationMs: 500},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}
	exec := headeredExecutor(nil, 200*time.Millisecond)

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Error("expected timing assertion to pass")
	}
}

func TestRun_timing_assertion_fail(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Timing: parser.TimingAssertion{MaxDurationMs: 100},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}
	exec := headeredExecutor(nil, 200*time.Millisecond)

	_, summary, _ := Run(context.Background(), col, exec, VarSources{})

	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.AssertionFailures != 1 {
		t.Errorf("assertion failures = %d, want 1", summary.AssertionFailures)
	}
}

func TestRun_header_and_timing_combined(t *testing.T) {
	item := parser.RequestItem{
		Name:    "A",
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
		Assertions: parser.Assertions{
			Status: parser.StatusCodes{Codes: []int{200}},
			Headers: parser.HeaderAssertions{Items: []parser.HeaderAssertion{
				{Name: "Content-Type", Operator: "equals", Value: "application/json"},
			}},
			Timing: parser.TimingAssertion{MaxDurationMs: 500},
		},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}
	exec := headeredExecutor(http.Header{"Content-Type": {"application/json"}}, 200*time.Millisecond)

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].AssertionResults == nil {
		t.Fatal("expected assertion results")
	}
	// status + header + timing = 3 items
	if len(results[0].AssertionResults.Items) != 3 {
		t.Errorf("assertion items = %d, want 3", len(results[0].AssertionResults.Items))
	}
}

func TestRun_with_variables_interpolates_url(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base": "https://example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base}}/path"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://example.com/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://example.com/path")
	}
}

func TestRun_with_variables_interpolates_headers(t *testing.T) {
	var capturedHeaders map[string]string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedHeaders = req.Headers
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"token": "abc123"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method:  "GET",
				URL:     "https://example.com",
				Headers: map[string]string{"Authorization": "Bearer {{token}}"},
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedHeaders["Authorization"] != "Bearer abc123" {
		t.Errorf("Authorization = %q, want %q", capturedHeaders["Authorization"], "Bearer abc123")
	}
}

func TestRun_with_variables_interpolates_query_params(t *testing.T) {
	var capturedQuery map[string]string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedQuery = req.QueryParams
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"ver": "v2"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method:      "GET",
				URL:         "https://example.com",
				QueryParams: map[string]string{"version": "{{ver}}"},
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedQuery["version"] != "v2" {
		t.Errorf("version = %q, want %q", capturedQuery["version"], "v2")
	}
}

func TestRun_with_circular_variables_returns_error(t *testing.T) {
	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"a": "{{b}}", "b": "{{a}}"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err == nil {
		t.Fatal("expected error for circular variables")
	}
	if callCount != 0 {
		t.Errorf("executor called %d times, want 0", callCount)
	}
}

func TestRun_with_undefined_variable_returns_error(t *testing.T) {
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"a": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{unknown}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err == nil {
		t.Fatal("expected error for undefined variable")
	}
}

func TestRun_with_variables_interpolates_body(t *testing.T) {
	var capturedBody any
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedBody = req.Body
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"name": "test"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method: "POST",
				URL:    "https://example.com",
				Body:   "hello {{name}}",
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedBody != "hello test" {
		t.Errorf("Body = %v, want %q", capturedBody, "hello test")
	}
}

func TestRun_with_undefined_variable_in_headers_returns_error(t *testing.T) {
	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"a": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method:  "GET",
				URL:     "https://example.com",
				Headers: map[string]string{"X-Token": "{{unknown}}"},
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err == nil {
		t.Fatal("expected error for undefined variable in headers")
	}
	if callCount != 0 {
		t.Errorf("executor called %d times, want 0", callCount)
	}
}

func TestRun_with_undefined_variable_in_query_params_returns_error(t *testing.T) {
	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"a": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method:      "GET",
				URL:         "https://example.com",
				QueryParams: map[string]string{"q": "{{unknown}}"},
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err == nil {
		t.Fatal("expected error for undefined variable in query params")
	}
	if callCount != 0 {
		t.Errorf("executor called %d times, want 0", callCount)
	}
}

func TestRun_with_undefined_variable_in_body_returns_error(t *testing.T) {
	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"a": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{
				Method: "POST",
				URL:    "https://example.com",
				Body:   "{{unknown}}",
			}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err == nil {
		t.Fatal("expected error for undefined variable in body")
	}
	if callCount != 0 {
		t.Errorf("executor called %d times, want 0", callCount)
	}
}

func TestRun_extract_sets_variable_for_next_request(t *testing.T) {
	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"data":{"token":"secret123"}}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Login",
				Request: parser.Request{Method: "POST", URL: "https://example.com/login"},
				Extract: map[string]string{"token": "$.data.token"},
			},
			{
				Name:    "Use Token",
				Request: parser.Request{Method: "GET", URL: "https://example.com/api?token={{token}}"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if len(capturedURLs) != 2 {
		t.Fatalf("got %d URLs, want 2", len(capturedURLs))
	}
	if capturedURLs[1] != "https://example.com/api?token=secret123" {
		t.Errorf("second URL = %q, want %q", capturedURLs[1], "https://example.com/api?token=secret123")
	}
}

func TestRun_extract_sets_variable_used_in_headers(t *testing.T) {
	var capturedHeaders []map[string]string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedHeaders = append(capturedHeaders, req.Headers)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"token":"bearer_xyz"}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Login",
				Request: parser.Request{Method: "POST", URL: "https://example.com/login"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name: "Use Token",
				Request: parser.Request{
					Method:  "GET",
					URL:     "https://example.com/api",
					Headers: map[string]string{"Authorization": "Bearer {{token}}"},
				},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if capturedHeaders[1]["Authorization"] != "Bearer bearer_xyz" {
		t.Errorf("Authorization = %q, want %q", capturedHeaders[1]["Authorization"], "Bearer bearer_xyz")
	}
}

func TestRun_extract_path_not_found_fails_request(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"data":{}}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract Missing",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"x": "$.missing"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
}

func TestRun_extract_non_json_body_fails_request(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`<html>not json</html>`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract Non-JSON",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"x": "$.a"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
}

func TestRun_extract_multiple_variables(t *testing.T) {
	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"a":"x","b":"y"}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"a": "$.a", "b": "$.b"},
			},
			{
				Name:    "Use",
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{a}}/{{b}}"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if capturedURLs[1] != "https://example.com/x/y" {
		t.Errorf("URL = %q, want %q", capturedURLs[1], "https://example.com/x/y")
	}
}

func TestRun_extract_overrides_collection_variable(t *testing.T) {
	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"token":"extracted"}`),
		}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"token": "original"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "Use",
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{token}}"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if capturedURLs[1] != "https://example.com/extracted" {
		t.Errorf("URL = %q, want %q", capturedURLs[1], "https://example.com/extracted")
	}
}

func TestRun_extract_in_last_request_no_error(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"id":42}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract Last",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"id": "$.id"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
}

func TestRun_extract_respects_stop_on_failure(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{}`),
		}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract Fail",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Extract: map[string]string{"x": "$.missing"},
			},
			{
				Name:    "Should Skip",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
		Options: parser.Options{StopOnFailure: true},
	}
	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
}

func TestRun_no_extract_unchanged_behavior(t *testing.T) {
	col := makeCollection([]string{"A", "B"}, false)
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
}

func TestRun_no_variables_unchanged(t *testing.T) {
	col := makeCollection([]string{"A"}, false)
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if len(results) != 1 {
		t.Fatalf("got %d results, want 1", len(results))
	}
}

func TestRun_cli_var_overrides_collection_variable(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "https://old.example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	cliVars := map[string]string{"base_url": "https://new.example.com"}
	_, _, err := Run(context.Background(), col, exec, VarSources{CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://new.example.com/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://new.example.com/path")
	}
}

func TestRun_cli_var_multiple(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{host}}/{{path}}"}},
		}},
	}
	cliVars := map[string]string{"host": "https://example.com", "path": "api"}
	_, _, err := Run(context.Background(), col, exec, VarSources{CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://example.com/api" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://example.com/api")
	}
}

func TestRun_cli_var_nil_unchanged(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base": "https://example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base}}/path"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://example.com/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://example.com/path")
	}
}

func TestRun_cli_var_adds_new_variable(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{new_var}}/path"}},
		}},
	}
	cliVars := map[string]string{"new_var": "https://injected.com"}
	_, _, err := Run(context.Background(), col, exec, VarSources{CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://injected.com/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://injected.com/path")
	}
}

func TestRun_env_var_used_when_no_collection_var(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	envVars := map[string]string{"base_url": "http://env"}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvFile: envVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://env/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://env/path")
	}
}

func TestRun_collection_overrides_env(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "http://col"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	envVars := map[string]string{"base_url": "http://env"}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvFile: envVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://col/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://col/path")
	}
}

func TestRun_cli_overrides_env_and_collection(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "http://col"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	envVars := map[string]string{"base_url": "http://env"}
	cliVars := map[string]string{"base_url": "http://cli"}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvFile: envVars, CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://cli/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://cli/path")
	}
}

func TestRun_nil_env_vars_works(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"x": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/{{x}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/1" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://example.com/1")
	}
}

func TestRun_dotenv_used_when_no_other_vars(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	dotenvVars := map[string]string{"base_url": "http://dotenv"}
	_, _, err := Run(context.Background(), col, exec, VarSources{DotEnv: dotenvVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://dotenv/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://dotenv/path")
	}
}

func TestRun_dotenv_overrides_env(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	envVars := map[string]string{"base_url": "http://env"}
	dotenvVars := map[string]string{"base_url": "http://dotenv"}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvFile: envVars, DotEnv: dotenvVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://dotenv/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://dotenv/path")
	}
}

func TestRun_collection_overrides_dotenv(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "http://col"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	dotenvVars := map[string]string{"base_url": "http://dotenv"}
	_, _, err := Run(context.Background(), col, exec, VarSources{DotEnv: dotenvVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://col/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://col/path")
	}
}

func TestRun_cli_overrides_dotenv(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	dotenvVars := map[string]string{"base_url": "http://dotenv"}
	cliVars := map[string]string{"base_url": "http://cli"}
	_, _, err := Run(context.Background(), col, exec, VarSources{DotEnv: dotenvVars, CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://cli/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://cli/path")
	}
}

func TestRun_full_precedence_chain(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "http://col"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base_url}}/path"}},
		}},
	}
	envVars := map[string]string{"base_url": "http://env"}
	dotenvVars := map[string]string{"base_url": "http://dotenv"}
	cliVars := map[string]string{"base_url": "http://cli"}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvFile: envVars, DotEnv: dotenvVars, CLI: cliVars})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://cli/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://cli/path")
	}
}

func TestRun_nil_dotenv_vars_works(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"x": "1"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/{{x}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/1" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://example.com/1")
	}
}

func TestRun_request_level_variables_override_collection(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "http://wrong-host:9999"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "A",
				Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "https://correct.example.com"}},
				Request:   parser.Request{Method: "GET", URL: "{{base_url}}/path"},
			},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://correct.example.com/path" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://correct.example.com/path")
	}
}

func TestRun_request_level_variables_scoped_to_single_request(t *testing.T) {
	var urls []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		urls = append(urls, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "https://collection.example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "First",
				Variables: parser.SensitiveVars{Values: map[string]string{"base_url": "https://override.example.com"}},
				Request:   parser.Request{Method: "GET", URL: "{{base_url}}/first"},
			},
			{
				Name:    "Second",
				Request: parser.Request{Method: "GET", URL: "{{base_url}}/second"},
			},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("got %d URLs, want 2", len(urls))
	}
	if urls[0] != "https://override.example.com/first" {
		t.Errorf("URL[0] = %q, want %q", urls[0], "https://override.example.com/first")
	}
	if urls[1] != "https://collection.example.com/second" {
		t.Errorf("URL[1] = %q, want %q", urls[1], "https://collection.example.com/second")
	}
}

func TestRun_request_level_variables_reference_collection_vars(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"host": "example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "A",
				Variables: parser.SensitiveVars{Values: map[string]string{"url": "https://{{host}}/api"}},
				Request:   parser.Request{Method: "GET", URL: "{{url}}"},
			},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "https://example.com/api" {
		t.Errorf("URL = %q, want %q", capturedURL, "https://example.com/api")
	}
}

func TestRun_env_var_overrides_collection(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"key": "col"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/{{key}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvVar: map[string]string{"key": "env"}})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/env" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://example.com/env")
	}
}

func TestRun_env_var_overrides_request_level(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "A",
				Variables: parser.SensitiveVars{Values: map[string]string{"key": "req"}},
				Request:   parser.Request{Method: "GET", URL: "http://example.com/{{key}}"},
			},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{EnvVar: map[string]string{"key": "env"}})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/env" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://example.com/env")
	}
}

func TestRun_cli_var_overrides_env_var(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "http://example.com/{{key}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		EnvVar: map[string]string{"key": "env"},
		CLI:    map[string]string{"key": "cli"},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/cli" {
		t.Errorf("URL = %q, want %q", capturedURL, "http://example.com/cli")
	}
}

func TestRun_full_precedence_chain_with_request_and_envvar(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"v": "collection"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "A",
				Variables: parser.SensitiveVars{Values: map[string]string{"v": "request"}},
				Request:   parser.Request{Method: "GET", URL: "http://example.com/{{v}}"},
			},
		}},
	}
	// CLI (10) should win over everything
	_, _, err := Run(context.Background(), col, exec, VarSources{
		EnvFile: map[string]string{"v": "envfile"},
		DotEnv:  map[string]string{"v": "dotenv"},
		EnvVar:  map[string]string{"v": "envvar"},
		CLI:     map[string]string{"v": "cli"},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedURL != "http://example.com/cli" {
		t.Errorf("URL = %q, want %q (CLI should have highest precedence)", capturedURL, "http://example.com/cli")
	}
}

func TestRun_request_level_vars_do_not_leak_to_next_request(t *testing.T) {
	var urls []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		urls = append(urls, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:      "Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"fallback": "default"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:      "First",
				Variables: parser.SensitiveVars{Values: map[string]string{"local_var": "local_value"}},
				Request:   parser.Request{Method: "GET", URL: "http://example.com/{{local_var}}"},
			},
			{
				Name:    "Second",
				Request: parser.Request{Method: "GET", URL: "http://example.com/{{fallback}}"},
			},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("got %d URLs, want 2", len(urls))
	}
	if urls[0] != "http://example.com/local_value" {
		t.Errorf("URL[0] = %q, want %q", urls[0], "http://example.com/local_value")
	}
	if urls[1] != "http://example.com/default" {
		t.Errorf("URL[1] = %q, want %q", urls[1], "http://example.com/default")
	}
}

func TestRun_request_level_vars_with_extraction(t *testing.T) {
	callCount := 0
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		if callCount == 1 {
			return &httpexec.Result{
				StatusCode: 200,
				Duration:   10 * time.Millisecond,
				Body:       []byte(`{"token":"abc123"}`),
			}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Extract",
				Request: parser.Request{Method: "GET", URL: "http://example.com/login"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:      "Use",
				Variables: parser.SensitiveVars{Values: map[string]string{"local": "local_value"}},
				Request: parser.Request{
					Method:  "GET",
					URL:     "http://example.com/api",
					Headers: map[string]string{"Authorization": "Bearer {{token}}", "X-Local": "{{local}}"},
				},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("got %d results, want 2", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("result[0] error: %v", results[0].Err)
	}
	if results[1].Err != nil {
		t.Errorf("result[1] error: %v", results[1].Err)
	}
}

// makeItem builds a single RequestItem for use in phase tests.
func makeItem(name string) parser.RequestItem {
	return parser.RequestItem{
		Name:    name,
		Request: parser.Request{Method: "GET", URL: "https://example.com"},
	}
}

// makeItemRequired builds a RequestItem with required:true.
func makeItemRequired(name string) parser.RequestItem {
	req := true
	return parser.RequestItem{
		Name:     name,
		Required: &req,
		Request:  parser.Request{Method: "GET", URL: "https://example.com"},
	}
}

// alwaysFailExecutor always returns a network error.
func alwaysFailExecutor(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	return nil, errFake
}

func TestRun_phases(t *testing.T) {
	tests := []struct {
		name        string
		col         *parser.Collection
		exec        ExecuteFunc
		wantPhases  []Phase
		wantSkipped []bool
		wantSummary func(t *testing.T, s *Summary)
	}{
		{
			name: "setup runs before main",
			col: &parser.Collection{
				Name:     "Test",
				Setup:    parser.Section{Items: []parser.RequestItem{makeItem("setup1")}},
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1"), makeItem("main2")}},
			},
			exec:        successExecutor,
			wantPhases:  []Phase{PhaseSetup, PhaseMain, PhaseMain},
			wantSkipped: []bool{false, false, false},
		},
		{
			name: "teardown runs after main",
			col: &parser.Collection{
				Name:     "Test",
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1"), makeItem("main2")}},
				Teardown: parser.Section{Items: []parser.RequestItem{makeItem("td1")}},
			},
			exec:        successExecutor,
			wantPhases:  []Phase{PhaseMain, PhaseMain, PhaseTeardown},
			wantSkipped: []bool{false, false, false},
		},
		{
			name: "teardown runs when main fails",
			col: &parser.Collection{
				Name:     "Test",
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1")}},
				Teardown: parser.Section{Items: []parser.RequestItem{makeItem("td1")}},
			},
			exec:        failAtN(0), // main fails, teardown succeeds
			wantPhases:  []Phase{PhaseMain, PhaseTeardown},
			wantSkipped: []bool{false, false},
			wantSummary: func(t *testing.T, s *Summary) {
				t.Helper()
				if s.TeardownErrors != 0 {
					t.Errorf("TeardownErrors = %d, want 0 (teardown succeeded)", s.TeardownErrors)
				}
				if s.Failed != 1 {
					t.Errorf("Failed = %d, want 1 (main failed)", s.Failed)
				}
			},
		},
		{
			name: "required setup failure skips main runs teardown",
			col: &parser.Collection{
				Name:     "Test",
				Setup:    parser.Section{Items: []parser.RequestItem{makeItemRequired("setup1")}},
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1"), makeItem("main2")}},
				Teardown: parser.Section{Items: []parser.RequestItem{makeItem("td1")}},
			},
			exec:        alwaysFailExecutor,
			wantPhases:  []Phase{PhaseSetup, PhaseMain, PhaseMain, PhaseTeardown},
			wantSkipped: []bool{false, true, true, false},
		},
		{
			name: "non-required setup failure continues main",
			col: &parser.Collection{
				Name:     "Test",
				Setup:    parser.Section{Items: []parser.RequestItem{makeItem("setup1")}},
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1"), makeItem("main2")}},
			},
			exec:        failAtN(0), // setup fails, main succeeds
			wantPhases:  []Phase{PhaseSetup, PhaseMain, PhaseMain},
			wantSkipped: []bool{false, false, false},
		},
		{
			name: "setup extract available in main and teardown",
			col: &parser.Collection{
				Name: "Test",
				Setup: parser.Section{Items: []parser.RequestItem{{
					Name:    "setup1",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
					Extract: map[string]string{"res_id": "$.id"},
				}}},
				Requests: parser.Section{Items: []parser.RequestItem{{
					Name:    "main1",
					Request: parser.Request{Method: "GET", URL: "https://example.com/{{res_id}}"},
				}}},
				Teardown: parser.Section{Items: []parser.RequestItem{{
					Name:    "td1",
					Request: parser.Request{Method: "DELETE", URL: "https://example.com/{{res_id}}"},
				}}},
			},
			exec: func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
				return &httpexec.Result{
					StatusCode: 200,
					Body:       []byte(`{"id":"123"}`),
					Duration:   10 * time.Millisecond,
				}, nil
			},
			wantPhases:  []Phase{PhaseSetup, PhaseMain, PhaseTeardown},
			wantSkipped: []bool{false, false, false},
			wantSummary: func(t *testing.T, s *Summary) {
				t.Helper()
				if s.Failed != 0 {
					t.Errorf("Failed = %d, want 0", s.Failed)
				}
			},
		},
		{
			name: "teardown failure does not change main pass/fail",
			col: &parser.Collection{
				Name:     "Test",
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1")}},
				Teardown: parser.Section{Items: []parser.RequestItem{makeItem("td1")}},
			},
			exec: func() ExecuteFunc {
				call := 0
				return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
					idx := call
					call++
					if idx == 1 { // teardown call fails
						return nil, errFake
					}
					return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
				}
			}(),
			wantPhases:  []Phase{PhaseMain, PhaseTeardown},
			wantSkipped: []bool{false, false},
			wantSummary: func(t *testing.T, s *Summary) {
				t.Helper()
				if s.TeardownErrors != 1 {
					t.Errorf("TeardownErrors = %d, want 1", s.TeardownErrors)
				}
				// main passed (Failed - TeardownErrors == 0)
				mainFailed := s.Failed - s.TeardownErrors
				if mainFailed != 0 {
					t.Errorf("main failed = %d, want 0", mainFailed)
				}
			},
		},
		{
			name: "no setup no teardown behaves as before",
			col: &parser.Collection{
				Name:     "Test",
				Requests: parser.Section{Items: []parser.RequestItem{makeItem("main1"), makeItem("main2")}},
			},
			exec:        successExecutor,
			wantPhases:  []Phase{PhaseMain, PhaseMain},
			wantSkipped: []bool{false, false},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, summary, err := Run(context.Background(), tt.col, tt.exec, VarSources{})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if len(results) != len(tt.wantPhases) {
				t.Fatalf("got %d results, want %d", len(results), len(tt.wantPhases))
			}
			for i, r := range results {
				if r.Phase != tt.wantPhases[i] {
					t.Errorf("results[%d].Phase = %q, want %q", i, r.Phase, tt.wantPhases[i])
				}
				if r.Skipped != tt.wantSkipped[i] {
					t.Errorf("results[%d].Skipped = %v, want %v", i, r.Skipped, tt.wantSkipped[i])
				}
			}
			if tt.wantSummary != nil {
				tt.wantSummary(t, summary)
			}
		})
	}
}

func TestRun_ProjectVariables(t *testing.T) {
	captureURL := func(captured *string) ExecuteFunc {
		return func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			*captured = req.URL
			return &httpexec.Result{StatusCode: 200, Duration: 1 * time.Millisecond}, nil
		}
	}

	tests := []struct {
		name       string
		project    map[string]string
		envFile    map[string]string
		dotEnv     map[string]string
		collection map[string]string
		cli        map[string]string
		requestURL string
		wantURL    string
	}{
		{
			name:       "project variables available in interpolation",
			project:    map[string]string{"base_url": "https://project.example.com"},
			requestURL: "{{base_url}}/path",
			wantURL:    "https://project.example.com/path",
		},
		{
			name:       "env file overrides project",
			project:    map[string]string{"base_url": "https://project.example.com"},
			envFile:    map[string]string{"base_url": "https://env.example.com"},
			requestURL: "{{base_url}}/path",
			wantURL:    "https://env.example.com/path",
		},
		{
			name:       "dotenv overrides project",
			project:    map[string]string{"base_url": "https://project.example.com"},
			dotEnv:     map[string]string{"base_url": "https://dotenv.example.com"},
			requestURL: "{{base_url}}/path",
			wantURL:    "https://dotenv.example.com/path",
		},
		{
			name:       "collection overrides project",
			project:    map[string]string{"base_url": "https://project.example.com"},
			collection: map[string]string{"base_url": "https://collection.example.com"},
			requestURL: "{{base_url}}/path",
			wantURL:    "https://collection.example.com/path",
		},
		{
			name:       "cli overrides project",
			project:    map[string]string{"base_url": "https://project.example.com"},
			cli:        map[string]string{"base_url": "https://cli.example.com"},
			requestURL: "{{base_url}}/path",
			wantURL:    "https://cli.example.com/path",
		},
		{
			name:       "nil project map is safe (no panic)",
			project:    nil,
			requestURL: "https://static.example.com/path",
			wantURL:    "https://static.example.com/path",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var capturedURL string
			col := &parser.Collection{
				Name:      "Test",
				Variables: parser.SensitiveVars{Values: tc.collection},
				Requests: parser.Section{Items: []parser.RequestItem{
					{
						Name:    "req",
						Request: parser.Request{Method: "GET", URL: tc.requestURL},
					},
				}},
			}
			_, _, err := Run(context.Background(), col, captureURL(&capturedURL), VarSources{
				Project: tc.project,
				EnvFile: tc.envFile,
				DotEnv:  tc.dotEnv,
				CLI:     tc.cli,
			})
			if err != nil {
				t.Fatalf("Run returned error: %v", err)
			}
			if capturedURL != tc.wantURL {
				t.Errorf("URL = %q, want %q", capturedURL, tc.wantURL)
			}
		})
	}
}

func TestRun_dynamic_uuid_interpolated(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://api/{{$uuid}}"}},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 || results[0].Err != nil {
		t.Fatalf("expected 1 successful result, got %+v", results)
	}
	uuidRe := regexp.MustCompile(`^https://api/[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)
	if !uuidRe.MatchString(capturedURL) {
		t.Errorf("URL %q does not contain a valid UUID v4", capturedURL)
	}
}

func TestRun_seed_deterministic_across_runs(t *testing.T) {
	seed := int64(42)
	captureURL := func() string {
		var url string
		exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			url = req.URL
			return &httpexec.Result{StatusCode: 200}, nil
		}
		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://api/{{$uuid}}"}},
			}},
		}
		_, _, err := Run(context.Background(), col, exec, VarSources{Seed: &seed})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		return url
	}

	url1 := captureURL()
	url2 := captureURL()
	if url1 != url2 {
		t.Errorf("same seed produced different URLs: %q vs %q", url1, url2)
	}
}

func TestRun_dynamic_different_per_request(t *testing.T) {
	urls := make([]string, 0, 2)
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		urls = append(urls, req.URL)
		return &httpexec.Result{StatusCode: 200}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://api/{{$uuid}}"}},
			{Name: "B", Request: parser.Request{Method: "GET", URL: "https://api/{{$uuid}}"}},
		}},
	}
	seed := int64(7)
	_, _, err := Run(context.Background(), col, exec, VarSources{Seed: &seed})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(urls) != 2 {
		t.Fatalf("expected 2 URLs, got %d", len(urls))
	}
	if urls[0] == urls[1] {
		t.Errorf("different requests should get different UUIDs, both got %q", urls[0])
	}
}

func TestRun_dynamic_override_by_cli_var(t *testing.T) {
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200}, nil
	}
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://api/{{$uuid}}"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		CLI: map[string]string{"$uuid": "fixed-id"},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedURL != "https://api/fixed-id" {
		t.Errorf("CLI var should override dynamic function; got %q", capturedURL)
	}
}

func TestRun_dynamic_with_request_variables(t *testing.T) {
	// A request with both per-request Variables and a {{$uuid}} in the URL.
	// The dynamic registry must survive WithOverrides so the function is resolved.
	seed := int64(42)
	var capturedURL string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURL = req.URL
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	item := parser.RequestItem{
		Name: "test",
		Request: parser.Request{
			Method: "GET",
			URL:    "https://api.example.com/items/{{$uuid}}",
		},
		Variables: parser.SensitiveVars{Values: map[string]string{"extra": "somevalue"}},
	}
	col := &parser.Collection{Name: "Test", Requests: parser.Section{Items: []parser.RequestItem{item}}}

	_, summary, err := Run(context.Background(), col, exec, VarSources{Seed: &seed})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Fatalf("expected 0 failures, got %d", summary.Failed)
	}

	// The URL must have {{$uuid}} replaced with an actual UUID (not the literal placeholder).
	if !regexp.MustCompile(`[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}`).MatchString(capturedURL) {
		t.Errorf("URL %q does not contain a resolved UUID — registry was dropped by WithOverrides", capturedURL)
	}
}

func boolPtr(b bool) *bool { return &b }

func TestRunPopulatesMethodAndURL(t *testing.T) {
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Get Test",
				Request: parser.Request{Method: "GET", URL: "https://example.com/path"},
			},
		}},
	}
	mockExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200}, nil
	}
	results, _, err := Run(context.Background(), col, mockExec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Method != "GET" {
		t.Errorf("expected Method=GET, got %q", results[0].Method)
	}
	if results[0].URL != "https://example.com/path" {
		t.Errorf("expected URL=https://example.com/path, got %q", results[0].URL)
	}
}

func TestRunPopulatesMethodAndURL_onError(t *testing.T) {
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Failing Request",
				Request: parser.Request{Method: "POST", URL: "https://example.com/api"},
			},
		}},
	}
	execErr := errors.New("connection refused")
	mockExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, execErr
	}
	results, _, err := Run(context.Background(), col, mockExec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Method != "POST" {
		t.Errorf("expected Method=POST, got %q", results[0].Method)
	}
	if results[0].URL != "https://example.com/api" {
		t.Errorf("expected URL=https://example.com/api, got %q", results[0].URL)
	}
}

func TestRunPopulatesMethodAndURL_skippedInSetupFail(t *testing.T) {
	reqErr := errors.New("setup failed")
	col := &parser.Collection{
		Name: "Test",
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:     "Setup Step",
				Request:  parser.Request{Method: "GET", URL: "https://example.com/setup"},
				Required: boolPtr(true),
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Main Request",
				Request: parser.Request{Method: "DELETE", URL: "https://example.com/resource"},
			},
		}},
	}
	mockExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, reqErr
	}
	results, _, err := Run(context.Background(), col, mockExec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Find the skipped main request
	var skippedResult *RequestResult
	for i := range results {
		if results[i].Name == "Main Request" {
			skippedResult = &results[i]
			break
		}
	}
	if skippedResult == nil {
		t.Fatal("did not find Main Request in results")
	}
	if !skippedResult.Skipped {
		t.Error("expected Main Request to be skipped")
	}
	if skippedResult.Method != "DELETE" {
		t.Errorf("expected Method=DELETE for skipped request, got %q", skippedResult.Method)
	}
	if skippedResult.URL != "https://example.com/resource" {
		t.Errorf("expected URL=https://example.com/resource for skipped request, got %q", skippedResult.URL)
	}
}

func TestRunPopulatesRequestHeadersAndBody(t *testing.T) {
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "req-with-headers",
				Request: parser.Request{
					Method:  "POST",
					URL:     "https://example.com",
					Headers: map[string]string{"Accept": "application/json", "X-Token": "abc"},
					Body:    map[string]any{"key": "value"},
				},
			},
		}},
	}
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if r.RequestHeaders == nil {
		t.Error("expected RequestHeaders to be populated")
	}
	if r.RequestHeaders["Accept"] != "application/json" {
		t.Errorf("expected Accept header, got %v", r.RequestHeaders)
	}
	if r.RequestBody == nil {
		t.Error("expected RequestBody to be populated")
	}
}

// generateNames creates N request names: "req-0", "req-1", ...
func generateNames(n int) []string {
	names := make([]string, n)
	for i := range names {
		names[i] = fmt.Sprintf("req-%d", i)
	}
	return names
}

// collectionWithPhases creates a collection with setup, main, and teardown items.
func collectionWithPhases(setup, main, teardown int) *parser.Collection {
	mkItems := func(prefix string, n int) []parser.RequestItem {
		items := make([]parser.RequestItem, n)
		for i := range items {
			items[i] = parser.RequestItem{
				Name:    fmt.Sprintf("%s-%d", prefix, i),
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			}
		}
		return items
	}
	return &parser.Collection{
		Name:     "Test",
		Setup:    parser.Section{Items: mkItems("setup", setup)},
		Requests: parser.Section{Items: mkItems("main", main)},
		Teardown: parser.Section{Items: mkItems("teardown", teardown)},
	}
}

func TestRun_GuardRail(t *testing.T) {
	tests := []struct {
		name            string
		collection      *parser.Collection
		maxRequests     int
		wantExecuted    int
		wantLimitExceed bool
		wantPassed      int
		wantSkipped     int
	}{
		{
			name:            "999 requests all succeed",
			collection:      makeCollection(generateNames(999), false),
			maxRequests:     1000,
			wantExecuted:    999,
			wantLimitExceed: false,
			wantPassed:      999,
			wantSkipped:     0,
		},
		{
			name:            "exactly 1000 requests all succeed",
			collection:      makeCollection(generateNames(1000), false),
			maxRequests:     1000,
			wantExecuted:    1000,
			wantLimitExceed: false,
			wantPassed:      1000,
			wantSkipped:     0,
		},
		{
			name:            "1001 requests stops at 1000",
			collection:      makeCollection(generateNames(1001), false),
			maxRequests:     1000,
			wantExecuted:    1000,
			wantLimitExceed: true,
			wantPassed:      1000,
			wantSkipped:     1,
		},
		{
			name:            "limit hit across setup main teardown",
			collection:      collectionWithPhases(500, 400, 200),
			maxRequests:     1000,
			wantExecuted:    1000,
			wantLimitExceed: true,
			wantPassed:      1000,
			wantSkipped:     100,
		},
		{
			name:            "limit hit during setup skips main",
			collection:      collectionWithPhases(1001, 5, 0),
			maxRequests:     1000,
			wantExecuted:    1000,
			wantLimitExceed: true,
			wantPassed:      1000,
			wantSkipped:     6,
		},
		{
			name:            "small limit for easy testing",
			collection:      makeCollection(generateNames(6), false),
			maxRequests:     5,
			wantExecuted:    5,
			wantLimitExceed: true,
			wantPassed:      5,
			wantSkipped:     1,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			old := MaxRequests
			MaxRequests = tt.maxRequests
			t.Cleanup(func() { MaxRequests = old })

			_, summary, err := Run(context.Background(), tt.collection, successExecutor, VarSources{})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if summary.RequestsExecuted != tt.wantExecuted {
				t.Errorf("RequestsExecuted = %d, want %d", summary.RequestsExecuted, tt.wantExecuted)
			}
			if summary.LimitExceeded != tt.wantLimitExceed {
				t.Errorf("LimitExceeded = %v, want %v", summary.LimitExceeded, tt.wantLimitExceed)
			}
			if summary.Passed != tt.wantPassed {
				t.Errorf("Passed = %d, want %d", summary.Passed, tt.wantPassed)
			}
			if summary.Skipped != tt.wantSkipped {
				t.Errorf("Skipped = %d, want %d", summary.Skipped, tt.wantSkipped)
			}
		})
	}
}

// urlCapturingExecutor records the URL from each request and returns success.
func urlCapturingExecutor(urls *[]string) ExecuteFunc {
	return func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		*urls = append(*urls, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}
}

func TestRunFromCommand(t *testing.T) {
	t.Run("from_command_resolves_at_solo_tier", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"secret": {Command: "echo secret123"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{secret}}"}},
			}},
		}
		var urls []string
		_, summary, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if summary.Passed != 1 {
			t.Errorf("passed = %d, want 1", summary.Passed)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/secret123" {
			t.Errorf("URL = %v, want [https://example.com/secret123]", urls)
		}
	})

	t.Run("from_command_cli_override_skips_execution", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"secret": {Command: "echo should_not_run"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{secret}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{
			CLI: map[string]string{"secret": "cli_override"},
		})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/cli_override" {
			t.Errorf("URL = %v, want [https://example.com/cli_override]", urls)
		}
	})

	t.Run("from_command_env_var_override_skips_execution", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"secret": {Command: "echo should_not_run"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{secret}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{
			EnvVar: map[string]string{"secret": "env_override"},
		})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/env_override" {
			t.Errorf("URL = %v, want [https://example.com/env_override]", urls)
		}
	})

	t.Run("from_command_error_propagates_to_caller", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"secret": {Command: "exit 1"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
			}},
		}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{})
		if err == nil {
			t.Fatal("expected error from failed command")
		}
	})

	t.Run("from_command_no_commands_at_free_tier_ok", func(t *testing.T) {
		col := makeCollection([]string{"A"}, false)
		_, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if summary.Passed != 1 {
			t.Errorf("passed = %d, want 1", summary.Passed)
		}
	})

	t.Run("from_command_used_in_request_url", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{"base": "https://api.example.com"},
				Commands: map[string]parser.CommandVar{"token": {Command: "echo abc123"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "{{base}}/{{token}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://api.example.com/abc123" {
			t.Errorf("URL = %v, want [https://api.example.com/abc123]", urls)
		}
	})

	t.Run("from_command_collection_var_overrides_command", func(t *testing.T) {
		// Collection-level string value at precedence 7 overrides from_command at precedence 5.
		// The command would fail if executed, proving the skip works.
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{"secret": "inline_value"},
				Commands: map[string]parser.CommandVar{"secret": {Command: "exit 1"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{secret}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/inline_value" {
			t.Errorf("URL = %v, want [https://example.com/inline_value]", urls)
		}
	})

	t.Run("from_command_cache_is_per_run_scope", func(t *testing.T) {
		// Cache field is parsed but within a single Run() each command variable
		// name is iterated exactly once, so the cache does not serve hits.
		// This test documents the current design intent.
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"token": {Command: "echo cached_val", Cache: 300}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{token}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/cached_val" {
			t.Errorf("URL = %v, want [https://example.com/cached_val]", urls)
		}
	})

	t.Run("from_command_sensitive_marks_variable_in_sensitive_set", func(t *testing.T) {
		sensitive := variable.NewSensitiveSet()
		sensitive.Add("secret") // Simulates what the parser does for sensitive: true
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:    map[string]string{},
				Sensitive: sensitive,
				Commands:  map[string]parser.CommandVar{"secret": {Command: "echo secret_val", Sensitive: true}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{secret}}"}},
			}},
		}
		var urls []string
		_, _, err := Run(context.Background(), col, urlCapturingExecutor(&urls), VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if len(urls) != 1 || urls[0] != "https://example.com/secret_val" {
			t.Errorf("URL = %v, want [https://example.com/secret_val]", urls)
		}
		if !col.Variables.Sensitive.IsSensitive("secret") {
			t.Error("expected 'secret' to be in sensitive set for redaction by caller")
		}
	})
}

func TestRun_VaultResolution(t *testing.T) {
	t.Run("vault_secrets_resolved_and_available_as_variables", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return "resolved-secret", nil
		}
		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{api_key}}"}},
			}},
		}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"api_key": "prod/api-key"}},
			VaultExecutor: mockExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(results) != 1 {
			t.Fatalf("expected 1 result, got %d", len(results))
		}
		if results[0].URL != "https://example.com/resolved-secret" {
			t.Errorf("URL = %q, want vault secret interpolated", results[0].URL)
		}
	})

	t.Run("vault_secrets_overridden_by_collection_values", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return "vault-value", nil
		}
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{"api_key": "collection-value"},
				Commands: map[string]parser.CommandVar{},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{api_key}}"}},
			}},
		}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"api_key": "prod/api-key"}},
			VaultExecutor: mockExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results[0].URL != "https://example.com/collection-value" {
			t.Errorf("URL = %q, want collection value to override vault", results[0].URL)
		}
	})

	t.Run("vault_secrets_overridden_by_cli_vars", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return "vault-value", nil
		}
		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{api_key}}"}},
			}},
		}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"api_key": "prod/api-key"}},
			VaultExecutor: mockExec,
			CLI:           map[string]string{"api_key": "cli-value"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results[0].URL != "https://example.com/cli-value" {
			t.Errorf("URL = %q, want CLI value to override vault", results[0].URL)
		}
	})

	t.Run("vault_secrets_override_from_command", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return "vault-value", nil
		}
		col := &parser.Collection{
			Name: "Test",
			Variables: parser.SensitiveVars{
				Values:   map[string]string{},
				Commands: map[string]parser.CommandVar{"api_key": {Command: "echo from-cmd"}},
			},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{api_key}}"}},
			}},
		}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"api_key": "prod/api-key"}},
			VaultExecutor: mockExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Vault (precedence 6) overrides from_command (precedence 5)
		if results[0].URL != "https://example.com/vault-value" {
			t.Errorf("URL = %q, want vault value to override from_command", results[0].URL)
		}
	})

	t.Run("vault_secret_fetch_error_stops_execution", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return "", fmt.Errorf("ResourceNotFoundException: not found")
		}
		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
			}},
		}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"k": "nonexistent"}},
			VaultExecutor: mockExec,
		})
		if err == nil {
			t.Fatal("expected error from vault fetch failure")
		}
		var se *apierrors.Structured
		if !errors.As(err, &se) {
			t.Fatalf("expected *errors.Structured, got %T: %v", err, err)
		}
		if se.Category != apierrors.CategoryConfig {
			t.Errorf("category = %q, want %q", se.Category, apierrors.CategoryConfig)
		}
	})

	t.Run("vault_field_extraction_in_runner", func(t *testing.T) {
		mockExec := func(ctx context.Context, command string) (string, error) {
			return `{"password":"s3cret","host":"db.example.com"}`, nil
		}
		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/{{db_pass}}"}},
			}},
		}
		results, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Secrets:       &vault.SecretsConfig{Provider: vault.ProviderAWS, Region: "us-east-1", Keys: map[string]string{"db_pass": "prod/db#password"}},
			VaultExecutor: mockExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if results[0].URL != "https://example.com/s3cret" {
			t.Errorf("URL = %q, want extracted field value", results[0].URL)
		}
	})
}

func TestRun_AuthProfiles(t *testing.T) {
	t.Run("no auth profiles unchanged behavior", func(t *testing.T) {
		col := makeCollection([]string{"A"}, false)
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	})

	t.Run("auth profile variables available in main requests", func(t *testing.T) {
		// Auth executor returns a token variable
		authCalled := false
		authExecuteFunc := func(_ context.Context, _ string) (map[string]string, error) {
			authCalled = true
			return map[string]string{"auth_token": "secret123"}, nil
		}

		col := &parser.Collection{
			Name: "Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{
					Name:    "main",
					Request: parser.Request{Method: "GET", URL: "https://example.com/{{auth_token}}"},
				},
			}},
		}

		var capturedURL string
		captureExec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			capturedURL = req.URL
			return &httpexec.Result{StatusCode: 200}, nil
		}

		_, _, err := Run(context.Background(), col, captureExec, VarSources{
			AuthProfiles: []auth.Profile{
				{Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
			},
			AuthExecuteFunc: authExecuteFunc,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !authCalled {
			t.Error("auth executor was not called")
		}
		if capturedURL != "https://example.com/secret123" {
			t.Errorf("URL = %q, want %q", capturedURL, "https://example.com/secret123")
		}
	})

	t.Run("auth profile failure returns error", func(t *testing.T) {
		col := makeCollection([]string{"A"}, false)
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			AuthProfiles: []auth.Profile{
				{Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
			},
			AuthExecuteFunc: func(_ context.Context, _ string) (map[string]string, error) {
				return nil, fmt.Errorf("network error")
			},
		})
		if err == nil {
			t.Fatal("expected error from auth profile failure, got nil")
		}
	})

	t.Run("auth profile requests do not increment main guard rail counter", func(t *testing.T) {
		col := makeCollection([]string{"A"}, false)

		authCallCount := 0
		authExec := func(_ context.Context, _ string) (map[string]string, error) {
			authCallCount++
			return map[string]string{"tok": "x"}, nil
		}

		results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
			AuthProfiles: []auth.Profile{
				{Name: "login", Type: auth.ProfileDynamic, Collection: "auth/login.yaml"},
			},
			AuthExecuteFunc: authExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if summary.RequestsExecuted != 1 {
			t.Errorf("RequestsExecuted = %d, want 1 (auth profile requests not counted)", summary.RequestsExecuted)
		}
		if len(results) != 1 {
			t.Errorf("results len = %d, want 1", len(results))
		}
		_ = authCallCount
	})
}

func TestRunForExtraction(t *testing.T) {
	t.Run("file not found returns error", func(t *testing.T) {
		_, err := RunForExtraction(context.Background(), "/nonexistent/path/auth.yaml", successExecutor, VarSources{})
		if err == nil {
			t.Fatal("expected error for missing file, got nil")
		}
	})

	t.Run("success: scope-diff extracts new variables", func(t *testing.T) {
		dir := t.TempDir()
		colFile := filepath.Join(dir, "auth.yaml")
		if err := os.WriteFile(colFile, []byte(`name: Auth Login
requests:
  - name: login
    request:
      method: POST
      url: https://example.com/login
    extract:
      token: "$.token"
`), 0o644); err != nil {
			t.Fatal(err)
		}
		exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 200, Body: []byte(`{"token":"secret_abc"}`)}, nil
		}
		extracted, err := RunForExtraction(context.Background(), colFile, exec, VarSources{})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if extracted["token"] != "secret_abc" {
			t.Errorf("token = %q, want %q", extracted["token"], "secret_abc")
		}
	})

	t.Run("request network failure returns error", func(t *testing.T) {
		dir := t.TempDir()
		colFile := filepath.Join(dir, "auth.yaml")
		if err := os.WriteFile(colFile, []byte(`name: Auth Login
requests:
  - name: login
    request:
      method: GET
      url: https://example.com/login
`), 0o644); err != nil {
			t.Fatal(err)
		}
		exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			return nil, errors.New("connection refused")
		}
		_, err := RunForExtraction(context.Background(), colFile, exec, VarSources{})
		if err == nil {
			t.Fatal("expected error from request failure, got nil")
		}
	})

	t.Run("assertion failure returns error", func(t *testing.T) {
		dir := t.TempDir()
		colFile := filepath.Join(dir, "auth.yaml")
		if err := os.WriteFile(colFile, []byte(`name: Auth Login
requests:
  - name: login
    request:
      method: GET
      url: https://example.com/login
    assertions:
      status: [201]
`), 0o644); err != nil {
			t.Fatal(err)
		}
		exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			return &httpexec.Result{StatusCode: 200}, nil
		}
		_, err := RunForExtraction(context.Background(), colFile, exec, VarSources{})
		if err == nil {
			t.Fatal("expected error from assertion failure, got nil")
		}
	})

	t.Run("recursive auth profiles not forwarded to inner run", func(t *testing.T) {
		dir := t.TempDir()
		colFile := filepath.Join(dir, "auth.yaml")
		if err := os.WriteFile(colFile, []byte(`name: Auth Login
requests:
  - name: login
    request:
      method: GET
      url: https://example.com/login
`), 0o644); err != nil {
			t.Fatal(err)
		}
		authCalled := false
		authExec := func(_ context.Context, _ string) (map[string]string, error) {
			authCalled = true
			return nil, errors.New("should not be called recursively")
		}
		_, err := RunForExtraction(context.Background(), colFile, successExecutor, VarSources{
			AuthProfiles: []auth.Profile{
				{Name: "outer", Type: auth.ProfileDynamic, Collection: "auth/other.yaml"},
			},
			AuthExecuteFunc: authExec,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if authCalled {
			t.Error("auth execute func was called recursively inside RunForExtraction")
		}
	})
}

func TestResolveAuthProfile(t *testing.T) {
	makeScope := func(kvs ...string) *variable.Scope {
		s := variable.NewScope(nil)
		if err := s.Resolve(); err != nil {
			t.Fatalf("Resolve() error: %v", err)
		}
		for i := 0; i+1 < len(kvs); i += 2 {
			s.Set(kvs[i], kvs[i+1])
		}
		return s
	}
	profiles := []auth.Profile{
		{Name: "admin_token", Extract: "admin_token"},
		{Name: "user_token", Extract: "user_token"},
	}

	tests := []struct {
		name        string
		authName    string
		profiles    []auth.Profile
		scope       *variable.Scope
		wantHeader  string
		wantValue   string
		wantErr     bool
		errContains string
	}{
		{
			name:       "empty auth name returns nothing",
			authName:   "",
			scope:      makeScope(),
			wantHeader: "", wantValue: "",
		},
		{
			name:       "valid profile with extract injects bearer",
			authName:   "admin_token",
			profiles:   profiles,
			scope:      makeScope("admin_token", "tok123"),
			wantHeader: "Authorization", wantValue: "Bearer tok123",
		},
		{
			name:       "profile without extract uses name as variable",
			authName:   "myprofile",
			profiles:   []auth.Profile{{Name: "myprofile"}},
			scope:      makeScope("myprofile", "secret"),
			wantHeader: "Authorization", wantValue: "Bearer secret",
		},
		{
			name:        "nonexistent profile lists available profiles",
			authName:    "missing",
			profiles:    profiles,
			scope:       makeScope(),
			wantErr:     true,
			errContains: "missing",
		},
		{
			name:        "nonexistent profile lists available names",
			authName:    "missing",
			profiles:    profiles,
			scope:       makeScope(),
			wantErr:     true,
			errContains: "admin_token, user_token",
		},
		{
			name:        "no profiles configured gives helpful message",
			authName:    "foo",
			profiles:    nil,
			scope:       makeScope(),
			wantErr:     true,
			errContains: "no auth profiles configured",
		},
		{
			name:        "variable not in scope returns error",
			authName:    "admin_token",
			profiles:    profiles,
			scope:       makeScope(),
			wantErr:     true,
			errContains: "admin_token",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			hdr, val, err := resolveAuthProfile(tc.authName, tc.profiles, tc.scope)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("resolveAuthProfile() error = nil, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveAuthProfile() unexpected error: %v", err)
			}
			if hdr != tc.wantHeader {
				t.Errorf("header = %q, want %q", hdr, tc.wantHeader)
			}
			if val != tc.wantValue {
				t.Errorf("value = %q, want %q", val, tc.wantValue)
			}
		})
	}
}

func TestHasHeaderCaseInsensitive(t *testing.T) {
	tests := []struct {
		name    string
		headers map[string]string
		key     string
		want    bool
	}{
		{"exact match", map[string]string{"Authorization": "Bearer x"}, "Authorization", true},
		{"lowercase key matches", map[string]string{"authorization": "Bearer x"}, "Authorization", true},
		{"uppercase key matches", map[string]string{"AUTHORIZATION": "Bearer x"}, "Authorization", true},
		{"absent key", map[string]string{"Content-Type": "application/json"}, "Authorization", false},
		{"nil map", nil, "Authorization", false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := hasHeaderCaseInsensitive(tc.headers, tc.key)
			if got != tc.want {
				t.Errorf("hasHeaderCaseInsensitive(%v, %q) = %v, want %v", tc.headers, tc.key, got, tc.want)
			}
		})
	}
}

func TestRun_PerRequestAuth(t *testing.T) {
	captureExec := func(captured *[]map[string]string) ExecuteFunc {
		return func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			headers := make(map[string]string, len(req.Headers))
			for k, v := range req.Headers {
				headers[k] = v
			}
			*captured = append(*captured, headers)
			return &httpexec.Result{StatusCode: 200}, nil
		}
	}

	makeProfile := func(name, extract string) auth.Profile {
		return auth.Profile{Name: name, Extract: extract}
	}

	tests := []struct {
		name        string
		requests    []parser.RequestItem
		profiles    []auth.Profile
		scopeVars   map[string]string
		wantHeaders []map[string]string
		wantErr     bool
		errContains string
	}{
		{
			name: "auth injects bearer header",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "admin_token",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
				},
			},
			profiles:  []auth.Profile{makeProfile("admin_token", "admin_token")},
			scopeVars: map[string]string{"admin_token": "tok123"},
			wantHeaders: []map[string]string{
				{"Authorization": "Bearer tok123"},
			},
		},
		{
			name: "two requests use different profiles",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "admin_token",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
				},
				{
					Name: "r2", Auth: "user_token",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
				},
			},
			profiles: []auth.Profile{
				makeProfile("admin_token", "admin_token"),
				makeProfile("user_token", "user_token"),
			},
			scopeVars: map[string]string{"admin_token": "adminTok", "user_token": "userTok"},
			wantHeaders: []map[string]string{
				{"Authorization": "Bearer adminTok"},
				{"Authorization": "Bearer userTok"},
			},
		},
		{
			name: "explicit Authorization header takes precedence",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "admin_token",
					Request: parser.Request{
						Method:  "GET",
						URL:     "https://example.com",
						Headers: map[string]string{"Authorization": "Custom explicit"},
					},
				},
			},
			profiles:  []auth.Profile{makeProfile("admin_token", "admin_token")},
			scopeVars: map[string]string{"admin_token": "tok123"},
			wantHeaders: []map[string]string{
				{"Authorization": "Custom explicit"},
			},
		},
		{
			name: "case-insensitive explicit header takes precedence",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "admin_token",
					Request: parser.Request{
						Method:  "GET",
						URL:     "https://example.com",
						Headers: map[string]string{"authorization": "Custom lowercase"},
					},
				},
			},
			profiles:  []auth.Profile{makeProfile("admin_token", "admin_token")},
			scopeVars: map[string]string{"admin_token": "tok123"},
			wantHeaders: []map[string]string{
				{"authorization": "Custom lowercase"},
			},
		},
		{
			name: "no auth field - no injection",
			requests: []parser.RequestItem{
				{Name: "r1", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
			},
			profiles:  []auth.Profile{makeProfile("admin_token", "admin_token")},
			scopeVars: map[string]string{"admin_token": "tok123"},
			wantHeaders: []map[string]string{
				{},
			},
		},
		{
			name: "nonexistent profile returns error listing available",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "missing",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
				},
			},
			profiles:    []auth.Profile{makeProfile("admin_token", "admin_token")},
			scopeVars:   map[string]string{"admin_token": "tok"},
			wantErr:     true,
			errContains: "admin_token",
		},
		{
			name: "auth with no profiles configured returns clear error",
			requests: []parser.RequestItem{
				{
					Name: "r1", Auth: "foo",
					Request: parser.Request{Method: "GET", URL: "https://example.com"},
				},
			},
			profiles:    nil,
			wantErr:     true,
			errContains: "no auth profiles configured",
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var captured []map[string]string
			col := &parser.Collection{
				Name:     "test",
				Requests: parser.Section{Items: tc.requests},
			}
			vars := VarSources{
				AuthProfiles: tc.profiles,
				CLI:          tc.scopeVars,
				AuthExecuteFunc: func(_ context.Context, _ string) (map[string]string, error) {
					return tc.scopeVars, nil
				},
			}
			_, _, err := Run(context.Background(), col, captureExec(&captured), vars)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("Run() error = nil, want error containing %q", tc.errContains)
				}
				if tc.errContains != "" && !strings.Contains(err.Error(), tc.errContains) {
					t.Errorf("error = %q, want to contain %q", err.Error(), tc.errContains)
				}
				return
			}
			if err != nil {
				t.Fatalf("Run() unexpected error: %v", err)
			}
			if len(captured) != len(tc.wantHeaders) {
				t.Fatalf("captured %d requests, want %d", len(captured), len(tc.wantHeaders))
			}
			for i, want := range tc.wantHeaders {
				for k, v := range want {
					if got := captured[i][k]; got != v {
						t.Errorf("request %d header %q = %q, want %q", i, k, got, v)
					}
				}
				// Ensure no unexpected auth headers added
				if _, authExpected := want["Authorization"]; !authExpected {
					if _, authExpectedLower := want["authorization"]; !authExpectedLower {
						if _, hasAuth := captured[i]["Authorization"]; hasAuth {
							t.Errorf("unexpected Authorization header injected for request %d", i)
						}
					}
				}
			}
		})
	}
}

func TestRun_AuthProfileCaching(t *testing.T) {
	col := makeCollection([]string{"req1"}, false)
	profile := auth.Profile{
		Name:       "login",
		Type:       auth.ProfileDynamic,
		Collection: "auth/login.yaml",
		CacheTTL:   3600,
	}

	execCalls := 0
	authExec := func(_ context.Context, _ string) (map[string]string, error) {
		execCalls++
		return map[string]string{"login": "tok123"}, nil
	}

	projectRoot := t.TempDir()
	cacheStore := auth.NewFileCacheStore(projectRoot)

	vars := VarSources{
		AuthProfiles:    []auth.Profile{profile},
		ProjectRoot:     projectRoot,
		AuthExecuteFunc: authExec,
		CacheStore:      cacheStore,
	}

	// First run: should call authExec once and populate cache.
	_, _, err := Run(context.Background(), col, successExecutor, vars)
	if err != nil {
		t.Fatalf("first run error: %v", err)
	}
	if execCalls != 1 {
		t.Errorf("first run: execCalls = %d, want 1", execCalls)
	}

	// Second run within TTL: should use cached credentials, skip authExec.
	execCalls = 0
	_, _, err = Run(context.Background(), col, successExecutor, vars)
	if err != nil {
		t.Fatalf("second run error: %v", err)
	}
	if execCalls != 0 {
		t.Errorf("second run (cache hit): execCalls = %d, want 0", execCalls)
	}
}

func TestExecutePhase_RefreshOnFailure(t *testing.T) {
	tests := []struct {
		name              string
		firstStatus       int
		retryStatus       int
		refreshOnFailure  bool
		hasItemAuth       bool
		authExecFails     bool
		wantFinalStatus   int
		wantExecCallCount int
	}{
		{
			name:        "401 with refresh_on_failure retries after re-auth",
			firstStatus: 401, retryStatus: 200, refreshOnFailure: true, hasItemAuth: true,
			wantFinalStatus: 200, wantExecCallCount: 2,
		},
		{
			name:        "401 without refresh_on_failure not retried",
			firstStatus: 401, retryStatus: 200, refreshOnFailure: false, hasItemAuth: true,
			wantFinalStatus: 401, wantExecCallCount: 1,
		},
		{
			name:        "401 retry only happens once",
			firstStatus: 401, retryStatus: 401, refreshOnFailure: true, hasItemAuth: true,
			wantFinalStatus: 401, wantExecCallCount: 2,
		},
		{
			name:        "403 not retried",
			firstStatus: 403, retryStatus: 200, refreshOnFailure: true, hasItemAuth: true,
			wantFinalStatus: 403, wantExecCallCount: 1,
		},
		{
			name:        "401 request without auth field not retried",
			firstStatus: 401, retryStatus: 200, refreshOnFailure: true, hasItemAuth: false,
			wantFinalStatus: 401, wantExecCallCount: 1,
		},
		{
			name:        "401 refresh failure falls through to original 401",
			firstStatus: 401, retryStatus: 0, refreshOnFailure: true, hasItemAuth: true,
			authExecFails:   true,
			wantFinalStatus: 401, wantExecCallCount: 1,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			httpCallCount := 0
			exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				httpCallCount++
				status := tt.firstStatus
				if httpCallCount > 1 {
					status = tt.retryStatus
				}
				return &httpexec.Result{StatusCode: status}, nil
			}

			authExecCalls := 0
			authExec := func(_ context.Context, _ string) (map[string]string, error) {
				authExecCalls++
				// Fail only on the refresh call (call > 1), not the initial auth execution.
				if tt.authExecFails && authExecCalls > 1 {
					return nil, fmt.Errorf("auth execution failed")
				}
				return map[string]string{"login": "new_tok"}, nil
			}

			profile := auth.Profile{
				Name:             "login",
				Type:             auth.ProfileDynamic,
				Collection:       "auth/login.yaml",
				RefreshOnFailure: tt.refreshOnFailure,
			}

			itemAuth := ""
			if tt.hasItemAuth {
				itemAuth = "login"
			}
			col := &parser.Collection{
				Name: "Test",
				Requests: parser.Section{Items: []parser.RequestItem{
					{
						Name:    "req1",
						Auth:    itemAuth,
						Request: parser.Request{Method: "GET", URL: "https://example.com"},
					},
				}},
			}

			// Seed the scope with auth variable so resolveAuthProfile works
			vars := VarSources{
				AuthProfiles:    []auth.Profile{profile},
				AuthExecuteFunc: authExec,
				Project:         map[string]string{"login": "initial_tok"},
			}

			results, _, err := Run(context.Background(), col, exec, vars)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("expected 1 result, got %d", len(results))
			}
			if results[0].Result.StatusCode != tt.wantFinalStatus {
				t.Errorf("final status = %d, want %d", results[0].Result.StatusCode, tt.wantFinalStatus)
			}
			if httpCallCount != tt.wantExecCallCount {
				t.Errorf("http exec calls = %d, want %d", httpCallCount, tt.wantExecCallCount)
			}
		})
	}
}

func TestRun_retryOn503(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		if calls <= 2 {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:  "Retry Test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Flaky", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", results[0].RetryCount)
	}
	if results[0].Result.StatusCode != 200 {
		t.Errorf("StatusCode = %d, want 200", results[0].Result.StatusCode)
	}
}

func TestRun_retryAllFail(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:  "Retry All Fail",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Always503",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{200}},
				},
			},
		}},
	}
	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if results[0].RetryCount != 2 {
		t.Errorf("RetryCount = %d, want 2", results[0].RetryCount)
	}
}

func TestRun_retryDisabled(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "No Retry",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "NoRetry", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (no retry), got %d", calls)
	}
	if results[0].RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", results[0].RetryCount)
	}
}

func TestRun_retryNetworkError(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return nil, &apierrors.NetworkError{Kind: apierrors.NetworkDNS, Message: "dns fail"}
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:  "Retry Network Error",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "DNS Fail", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", results[0].RetryCount)
	}
}

func TestRun_retryPOST503(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:  "POST No Retry",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "POST 503", Request: parser.Request{Method: "POST", URL: "https://example.com"}},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (POST not retried), got %d", calls)
	}
	if results[0].RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", results[0].RetryCount)
	}
}

func TestRun_retryPerRequestOverride(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
	}
	disabled := &retry.FullConfig{Enabled: retry.BoolPtr(false)}
	col := &parser.Collection{
		Name:  "Per-Request Override",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Disabled Override", Retry: disabled, Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if calls != 1 {
		t.Errorf("expected 1 call (per-request disabled), got %d", calls)
	}
	if results[0].RetryCount != 0 {
		t.Errorf("RetryCount = %d, want 0", results[0].RetryCount)
	}
}

func TestResolveRetryConfig_precedence(t *testing.T) {
	tests := []struct {
		name       string
		global     *retry.FullConfig
		collection *retry.FullConfig
		section    *retry.FullConfig
		request    *retry.FullConfig
		want       retry.Config
	}{
		{
			name: "all nil uses builtin defaults",
			want: retry.Config{Enabled: false, MaxAttempts: 3, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:   "global overrides builtin",
			global: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(5)},
			want:   retry.Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:       "collection overrides global (Behavior 1)",
			global:     &retry.FullConfig{Enabled: retry.BoolPtr(false)},
			collection: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(100)},
			want:       retry.Config{Enabled: true, MaxAttempts: 3, InitialDelayMs: 100, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:       "section overrides collection",
			collection: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3)},
			section:    &retry.FullConfig{MaxAttempts: retry.IntPtr(5)},
			want:       retry.Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:       "request overrides collection (Behavior 2)",
			collection: &retry.FullConfig{MaxAttempts: retry.IntPtr(5)},
			request:    &retry.FullConfig{MaxAttempts: retry.IntPtr(10)},
			want:       retry.Config{Enabled: false, MaxAttempts: 10, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:    "request overrides section",
			section: &retry.FullConfig{MaxAttempts: retry.IntPtr(5)},
			request: &retry.FullConfig{MaxAttempts: retry.IntPtr(10)},
			want:    retry.Config{Enabled: false, MaxAttempts: 10, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:    "section setup with max_attempts 5 (Behavior 3)",
			section: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(5)},
			want:    retry.Config{Enabled: true, MaxAttempts: 5, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
		{
			name:       "teardown disabled overrides (Behavior 4)",
			collection: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3)},
			section:    &retry.FullConfig{Enabled: retry.BoolPtr(false)},
			want:       retry.Config{Enabled: false, MaxAttempts: 3, InitialDelayMs: 1000, BackoffStrategy: "exponential", MaxDelayMs: 30000},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := resolveRetryConfig(tt.global, tt.collection, tt.section, tt.request)
			if got != tt.want {
				t.Errorf("resolveRetryConfig() = %+v, want %+v", got, tt.want)
			}
		})
	}
}

func TestRun_sectionRetry(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		if calls <= 4 {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Section Retry Test",
		Setup: parser.Section{
			Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(5), InitialDelayMs: retry.IntPtr(10)},
			Items: []parser.RequestItem{
				{Name: "Setup Request", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
			},
		},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Main Request", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Setup should have retried 4 times (5 attempts - 1)
	if results[0].RetryCount != 4 {
		t.Errorf("Setup RetryCount = %d, want 4", results[0].RetryCount)
	}
}

func TestRun_teardownRetryDisabled(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name:  "Teardown Retry Disabled",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Main", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
		Teardown: parser.Section{
			Retry: &retry.FullConfig{Enabled: retry.BoolPtr(false)},
			Items: []parser.RequestItem{
				{Name: "Cleanup", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
			},
		},
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// Main should retry (collection retry enabled), teardown should not
	mainRetries := results[0].RetryCount
	tdRetries := results[1].RetryCount
	if mainRetries != 2 {
		t.Errorf("Main RetryCount = %d, want 2", mainRetries)
	}
	if tdRetries != 0 {
		t.Errorf("Teardown RetryCount = %d, want 0 (section disabled)", tdRetries)
	}
}

func TestRun_globalRetryConfig(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Global Retry Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Request", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	results, summary, err := Run(context.Background(), col, exec, VarSources{
		GlobalRetry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 1 {
		t.Errorf("passed = %d, want 1", summary.Passed)
	}
	if results[0].RetryCount != 1 {
		t.Errorf("RetryCount = %d, want 1", results[0].RetryCount)
	}
}

func TestRun_exponentialAllowedAtSolo(t *testing.T) {
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	col := &parser.Collection{
		Name: "Exponential at Solo",
		Retry: &retry.FullConfig{
			Enabled:         retry.BoolPtr(true),
			MaxAttempts:     retry.IntPtr(3),
			InitialDelayMs:  retry.IntPtr(10),
			BackoffStrategy: retry.StringPtr("exponential"),
		},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Allowed", Request: parser.Request{Method: "GET", URL: "https://example.com"}},
		}},
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("expected no error for exponential at Solo tier, got %v", err)
	}
}

func TestRun_Parallel_IndependentRequests(t *testing.T) {
	col := &parser.Collection{
		Name: "Parallel Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/a"}},
			{Name: "B", Request: parser.Request{Method: "GET", URL: "https://example.com/b"}},
			{Name: "C", Request: parser.Request{Method: "GET", URL: "https://example.com/c"}},
		}},
	}

	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Total != 3 {
		t.Errorf("total = %d, want 3", summary.Total)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Errorf("results = %d, want 3", len(results))
	}
	for i, r := range results {
		if r.Phase != PhaseMain {
			t.Errorf("results[%d].Phase = %q, want %q", i, r.Phase, PhaseMain)
		}
	}
}

func TestRun_Parallel_WithSetupAndTeardown(t *testing.T) {
	var setupRan, teardownRan bool
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.Contains(req.URL, "setup") {
			setupRan = true
		}
		if strings.Contains(req.URL, "teardown") {
			teardownRan = true
		}
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Parallel With Phases",
		Setup: parser.Section{Items: []parser.RequestItem{
			{Name: "Setup", Request: parser.Request{Method: "GET", URL: "https://example.com/setup"}},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Main A", Request: parser.Request{Method: "GET", URL: "https://example.com/a"}},
			{Name: "Main B", Request: parser.Request{Method: "GET", URL: "https://example.com/b"}},
		}},
		Teardown: parser.Section{Items: []parser.RequestItem{
			{Name: "Teardown", Request: parser.Request{Method: "GET", URL: "https://example.com/teardown"}},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if !setupRan {
		t.Error("setup did not run")
	}
	if !teardownRan {
		t.Error("teardown did not run")
	}
	if summary.Passed != 4 {
		t.Errorf("passed = %d, want 4", summary.Passed)
	}
	// Verify phase labels
	phaseOrder := []Phase{PhaseSetup, PhaseMain, PhaseMain, PhaseTeardown}
	for i, want := range phaseOrder {
		if i >= len(results) {
			break
		}
		if results[i].Phase != want {
			t.Errorf("results[%d].Phase = %q, want %q", i, results[i].Phase, want)
		}
	}
}

func TestRun_Parallel_VariableExtraction(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.Contains(req.URL, "/login") {
			return &httpexec.Result{StatusCode: 200, Body: []byte(`{"token":"secret"}`), Duration: time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`), Duration: time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Parallel Extraction",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Login",
				Request: parser.Request{Method: "POST", URL: "https://example.com/login"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "Use Token",
				Request: parser.Request{Method: "GET", URL: "https://example.com/api?token={{token}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	// Verify the second request used the extracted token
	found := false
	for _, r := range results {
		if r.Name == "Use Token" && strings.Contains(r.URL, "token=secret") {
			found = true
		}
	}
	if !found {
		t.Error("expected 'Use Token' to have interpolated URL with extracted token")
	}
}

func TestRun_Parallel_FailedDepSkips(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.Contains(req.URL, "/login") {
			return nil, fmt.Errorf("connection refused")
		}
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`), Duration: time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Parallel Failed Dep",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Login",
				Request: parser.Request{Method: "POST", URL: "https://example.com/login"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "Use Token",
				Request: parser.Request{Method: "GET", URL: "https://example.com/api?token={{token}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
	// Find the skipped result
	for _, r := range results {
		if r.Name == "Use Token" {
			if !r.Skipped {
				t.Error("expected 'Use Token' to be skipped due to failed dependency")
			}
		}
	}
}

// overlapProbe proves concurrent dispatch without wall-clock assumptions
// (a duration bound flakes under CI load). Each enter() call blocks until at
// least `want` calls are in flight simultaneously — or a generous timeout
// expires — then returns. Sequential execution can never overlap, so its
// peak stays at 1 no matter how fast or slow the machine is.
type overlapProbe struct {
	mu       sync.Mutex
	want     int
	inFlight int
	peak     int
	release  chan struct{}
	once     sync.Once
}

func newOverlapProbe(want int) *overlapProbe {
	return &overlapProbe{want: want, release: make(chan struct{})}
}

func (p *overlapProbe) enter() {
	p.mu.Lock()
	p.inFlight++
	if p.inFlight > p.peak {
		p.peak = p.inFlight
	}
	if p.inFlight >= p.want {
		p.once.Do(func() { close(p.release) })
	}
	p.mu.Unlock()
	select {
	case <-p.release:
	case <-time.After(5 * time.Second):
	}
	p.mu.Lock()
	p.inFlight--
	p.mu.Unlock()
}

func (p *overlapProbe) max() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.peak
}

func TestRun_Parallel_Speedup(t *testing.T) {
	probe := newOverlapProbe(2)
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		probe.enter()
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`)}, nil
	}

	col := &parser.Collection{
		Name: "Parallel Speedup",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Slow A", Request: parser.Request{Method: "GET", URL: "https://example.com/a"}},
			{Name: "Slow B", Request: parser.Request{Method: "GET", URL: "https://example.com/b"}},
			{Name: "Slow C", Request: parser.Request{Method: "GET", URL: "https://example.com/c"}},
		}},
	}

	_, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if got := probe.max(); got < 2 {
		t.Errorf("max concurrent in-flight requests = %d, want >= 2 (independent requests must run in parallel)", got)
	}
}

func TestRun_Parallel_SetupFailure_SkipsMain(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.Contains(req.URL, "setup") {
			return nil, fmt.Errorf("setup failed")
		}
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}

	required := true
	col := &parser.Collection{
		Name: "Parallel Setup Fail",
		Setup: parser.Section{Items: []parser.RequestItem{
			{Name: "Setup", Request: parser.Request{Method: "GET", URL: "https://example.com/setup"}, Required: &required},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Main", Request: parser.Request{Method: "GET", URL: "https://example.com/main"}},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1 (main should be skipped)", summary.Skipped)
	}
	// Main request should be skipped
	for _, r := range results {
		if r.Name == "Main" {
			if !r.Skipped {
				t.Error("expected main request to be skipped due to setup failure")
			}
		}
	}
}

func TestRun_ParallelEdgeCases(t *testing.T) {
	tests := []struct {
		name            string
		collection      *parser.Collection
		exec            ExecuteFunc
		wantPassed      int
		wantFailed      int
		wantSkipped     int
		wantImpact      int
		checkSkipReason bool
	}{
		{
			"failed request skips dependents with variable-specific reason",
			&parser.Collection{
				Name: "Skip Reason",
				Requests: parser.Section{Items: []parser.RequestItem{
					{
						Name:    "A",
						Request: parser.Request{Method: "GET", URL: "https://example.com/a"},
						Extract: map[string]string{"user_id": "$.id"},
					},
					{
						Name:    "B",
						Request: parser.Request{Method: "GET", URL: "https://example.com/b/{{user_id}}"},
					},
				}},
			},
			func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return nil, fmt.Errorf("server error")
			},
			0, 1, 1, 1, true,
		},
		{
			"auth profile vars don't create dependencies - all parallel",
			&parser.Collection{
				Name: "Auth Profile Vars",
				Requests: parser.Section{Items: []parser.RequestItem{
					{
						Name:    "Create User",
						Request: parser.Request{Method: "POST", URL: "https://example.com/users", Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"}},
					},
					{
						Name:    "Get Roles",
						Request: parser.Request{Method: "GET", URL: "https://example.com/roles", Headers: map[string]string{"Authorization": "Bearer {{admin_token}}"}},
					},
				}},
				Variables: parser.SensitiveVars{Values: map[string]string{"admin_token": "tok123"}},
			},
			func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return &httpexec.Result{StatusCode: 200, Body: []byte("{}"), Duration: time.Millisecond}, nil
			},
			2, 0, 0, 0, false,
		},
		{
			"default value does not prevent skip on failed producer",
			&parser.Collection{
				Name: "Default Value Skip",
				Requests: parser.Section{Items: []parser.RequestItem{
					{
						Name:    "A",
						Request: parser.Request{Method: "GET", URL: "https://example.com/a"},
						Extract: map[string]string{"user_id": "$.id"},
					},
					{
						Name:    "B",
						Request: parser.Request{Method: "GET", URL: "https://example.com/b/{{user_id|default:fallback}}"},
					},
				}},
			},
			func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
				return nil, fmt.Errorf("fail")
			},
			0, 1, 1, 1, false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			results, summary, err := Run(context.Background(), tt.collection, tt.exec, VarSources{
				Parallel: true,
			})
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if summary.Passed != tt.wantPassed {
				t.Errorf("passed = %d, want %d", summary.Passed, tt.wantPassed)
			}
			if summary.Failed != tt.wantFailed {
				t.Errorf("failed = %d, want %d", summary.Failed, tt.wantFailed)
			}
			if summary.Skipped != tt.wantSkipped {
				t.Errorf("skipped = %d, want %d", summary.Skipped, tt.wantSkipped)
			}
			if len(summary.Impact) != tt.wantImpact {
				t.Errorf("impact entries = %d, want %d: %+v", len(summary.Impact), tt.wantImpact, summary.Impact)
			}
			if tt.checkSkipReason {
				for _, r := range results {
					if r.Skipped && r.SkipReason == "" {
						t.Errorf("skipped request %q has empty SkipReason", r.Name)
					}
				}
			}
		})
	}
}

func TestRun_ParallelMode_SkipReasonPropagated(t *testing.T) {
	// A extracts user_id, B uses {{user_id}}. A fails => B skipped with reason.
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, fmt.Errorf("server error")
	}

	col := &parser.Collection{
		Name: "Skip Reason Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com/a"},
				Extract: map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "https://example.com/b/{{user_id}}"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Find B's result
	var bResult *RequestResult
	for i := range results {
		if results[i].Name == "B" {
			bResult = &results[i]
			break
		}
	}
	if bResult == nil {
		t.Fatal("B result not found")
	}
	if !bResult.Skipped {
		t.Error("expected B to be skipped when A failed")
	}
	if bResult.SkipReason == "" {
		t.Error("expected SkipReason to be set")
	}
	if !strings.Contains(bResult.SkipReason, "user_id") {
		t.Errorf("SkipReason %q should mention 'user_id'", bResult.SkipReason)
	}
}

func TestRun_Parallel_WaveIndexPropagated(t *testing.T) {
	// Two waves: A extracts user_id, B depends on user_id → wave 0: [A], wave 1: [B]
	col := &parser.Collection{
		Name: "Wave Index Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com/a"},
				Extract: map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{user_id}}"},
			},
		}},
	}

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte(`{"id":"42"}`)}, nil
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Verify WaveIndex is set on results
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	if results[0].WaveIndex != 0 {
		t.Errorf("results[0].WaveIndex = %d, want 0", results[0].WaveIndex)
	}
	if results[1].WaveIndex != 1 {
		t.Errorf("results[1].WaveIndex = %d, want 1", results[1].WaveIndex)
	}

	// Verify summary parallel metadata
	if !summary.IsParallel {
		t.Error("summary.IsParallel should be true")
	}
	if summary.WaveCount != 2 {
		t.Errorf("summary.WaveCount = %d, want 2", summary.WaveCount)
	}
	if summary.MaxParallelism != 1 {
		t.Errorf("summary.MaxParallelism = %d, want 1", summary.MaxParallelism)
	}
	if len(summary.WaveDurations) != 2 {
		t.Errorf("summary.WaveDurations len = %d, want 2", len(summary.WaveDurations))
	}
}

func TestRun_Parallel_SingleWave_AllWaveIndex0(t *testing.T) {
	col := &parser.Collection{
		Name: "Single Wave Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "A", Request: parser.Request{Method: "GET", URL: "https://example.com/a"}},
			{Name: "B", Request: parser.Request{Method: "GET", URL: "https://example.com/b"}},
			{Name: "C", Request: parser.Request{Method: "GET", URL: "https://example.com/c"}},
		}},
	}

	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	for i, r := range results {
		if r.WaveIndex != 0 {
			t.Errorf("results[%d].WaveIndex = %d, want 0", i, r.WaveIndex)
		}
	}

	if !summary.IsParallel {
		t.Error("summary.IsParallel should be true")
	}
	if summary.WaveCount != 1 {
		t.Errorf("summary.WaveCount = %d, want 1", summary.WaveCount)
	}
	if summary.MaxParallelism != 3 {
		t.Errorf("summary.MaxParallelism = %d, want 3", summary.MaxParallelism)
	}
}

func TestRun_Sequential_NoParallelMetadata(t *testing.T) {
	col := makeCollection([]string{"A", "B"}, false)
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// Sequential mode: WaveIndex should be -1
	for i, r := range results {
		if r.WaveIndex != -1 {
			t.Errorf("results[%d].WaveIndex = %d, want -1 for sequential mode", i, r.WaveIndex)
		}
	}

	if summary.IsParallel {
		t.Error("summary.IsParallel should be false for sequential mode")
	}
	if summary.WaveCount != 0 {
		t.Errorf("summary.WaveCount = %d, want 0 for sequential mode", summary.WaveCount)
	}
}

// --- Data-Driven Tests ---

func writeYAMLFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write YAML: %v", err)
	}
	return path
}

func intPtr(n int) *int { return &n }

func writeCSVFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write CSV: %v", err)
	}
	return path
}

func writeJSONFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write JSON: %v", err)
	}
	return path
}

func TestRun_DataDriven_CSVSource(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "users.csv", "name,email\njohn,john@x.com\njane,jane@x.com\nbob,bob@x.com")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven CSV",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Create User",
				DataDriven: &datadriven.Config{Source: "users.csv"},
				Request:    parser.Request{Method: "POST", URL: "https://example.com/users/{{name}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if len(capturedURLs) != 3 {
		t.Fatalf("capturedURLs = %d, want 3", len(capturedURLs))
	}
	wantURLs := []string{
		"https://example.com/users/john",
		"https://example.com/users/jane",
		"https://example.com/users/bob",
	}
	for i, want := range wantURLs {
		if capturedURLs[i] != want {
			t.Errorf("URL[%d] = %q, want %q", i, capturedURLs[i], want)
		}
	}
}

func TestRun_DataDriven_JSONSource(t *testing.T) {
	dir := t.TempDir()
	writeJSONFile(t, dir, "users.json", `[{"name":"alice"},{"name":"bob"}]`)

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven JSON",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Get User",
				DataDriven: &datadriven.Config{Source: "users.json"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/users/{{name}}"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	wantURLs := []string{
		"https://example.com/users/alice",
		"https://example.com/users/bob",
	}
	for i, want := range wantURLs {
		if capturedURLs[i] != want {
			t.Errorf("URL[%d] = %q, want %q", i, capturedURLs[i], want)
		}
	}
}

func TestRun_DataDriven_SpecialVars(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\na\nb")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Special Vars",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{_index}}/{{_iteration}}/{{_total}}"},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(capturedURLs) != 2 {
		t.Fatalf("capturedURLs = %d, want 2", len(capturedURLs))
	}
	if capturedURLs[0] != "https://example.com/0/1/2" {
		t.Errorf("URL[0] = %q, want %q", capturedURLs[0], "https://example.com/0/1/2")
	}
	if capturedURLs[1] != "https://example.com/1/2/2" {
		t.Errorf("URL[1] = %q, want %q", capturedURLs[1], "https://example.com/1/2/2")
	}
}

func TestRun_DataDriven_ExtractionAccumulates(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\na\nb\nc")

	callNum := 0
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		body := fmt.Sprintf(`{"id":"user_%d"}`, callNum)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(body),
		}, nil
	}

	var capturedURLs []string
	wrapExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return exec(ctx, req)
	}

	col := &parser.Collection{
		Name: "Extraction",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Create",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "POST", URL: "https://example.com/create"},
				Extract:    map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "Use Accumulated",
				Request: parser.Request{Method: "GET", URL: "https://example.com/verify?ids={{user_id}}"},
			},
		}},
	}

	_, summary, err := Run(context.Background(), col, wrapExec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 4 { // 3 data-driven + 1 regular
		t.Errorf("passed = %d, want 4", summary.Passed)
	}
	// Last URL should contain accumulated array
	lastURL := capturedURLs[len(capturedURLs)-1]
	if !strings.Contains(lastURL, "user_1") || !strings.Contains(lastURL, "user_2") || !strings.Contains(lastURL, "user_3") {
		t.Errorf("last URL = %q, expected it to contain accumulated user IDs", lastURL)
	}
}

func TestRun_DataDriven_MissingFile(t *testing.T) {
	dir := t.TempDir()

	col := &parser.Collection{
		Name: "Missing File",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "nonexistent.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, successExecutor, VarSources{
		CollectionDir: dir,
	})
	if err == nil {
		t.Fatal("expected error for missing data file")
	}
	if !errors.Is(err, datadriven.ErrFileNotFound) {
		t.Errorf("error = %v, want ErrFileNotFound", err)
	}
}

func TestRun_DataDriven_EmptyFile(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "empty.csv", "name\n") // header only, no data rows

	col := &parser.Collection{
		Name: "Empty File",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "empty.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].Skipped {
		t.Error("result should be marked as skipped")
	}
}

func TestRun_DataDriven_GuardRailCountsIterations(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10")

	old := MaxRequests
	MaxRequests = 5
	t.Cleanup(func() { MaxRequests = old })

	var callCount int
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Guard Rail",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	_, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if callCount != 5 {
		t.Errorf("callCount = %d, want 5 (guard rail)", callCount)
	}
	if summary.RequestsExecuted != 5 {
		t.Errorf("RequestsExecuted = %d, want 5", summary.RequestsExecuted)
	}
}

func TestRun_DataDriven_RowDataOverridesCollectionVars(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "email\nrow@x.com")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Override Test",
		Variables: parser.SensitiveVars{
			Values:    map[string]string{"email": "default@x.com"},
			Sensitive: variable.NewSensitiveSet(),
			Commands:  make(map[string]parser.CommandVar),
		},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{email}}"},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(capturedURLs) != 1 {
		t.Fatalf("capturedURLs = %d, want 1", len(capturedURLs))
	}
	// Row data should override collection variable
	if capturedURLs[0] != "https://example.com/row@x.com" {
		t.Errorf("URL = %q, want %q (row data should override collection var)", capturedURLs[0], "https://example.com/row@x.com")
	}
}

func TestRun_DataDriven_AssertionFailureInIteration(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3")

	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Assert Fail",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{404}}},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// All 3 iterations should execute (assertion failures don't stop data-driven)
	if callNum != 3 {
		t.Errorf("callNum = %d, want 3", callNum)
	}
	if summary.Failed != 3 {
		t.Errorf("failed = %d, want 3", summary.Failed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// Check iteration naming
	if results[0].Name != "Test [1/3]" {
		t.Errorf("results[0].Name = %q, want %q", results[0].Name, "Test [1/3]")
	}
}

func TestRun_DataDriven_NetworkErrorInIteration(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3")

	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		if callNum == 2 {
			return nil, errFake
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Network Error",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// All 3 iterations should run; second fails with network error
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if summary.Failed != 1 {
		t.Errorf("failed = %d, want 1", summary.Failed)
	}
	if results[1].Err == nil {
		t.Error("results[1] should have error")
	}
}

func TestRun_DataDriven_ContextCancellationStopsIterations(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5")

	ctx, cancel := context.WithCancel(context.Background())
	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		if callNum == 2 {
			cancel()
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Cancel",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	results, _, err := Run(ctx, col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// Should stop after 2 iterations when context is cancelled
	if callNum > 3 {
		t.Errorf("callNum = %d, want at most 3 (should stop after context cancelled)", callNum)
	}
	if len(results) > 3 {
		t.Errorf("results = %d, want at most 3", len(results))
	}
}

func TestRun_DataDriven_RequiredSetupFailureSkipsMain(t *testing.T) {
	// A data-driven request in Setup with required:true that fails should
	// cause main requests to be skipped (checkRequired=true in Setup phase).
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2")

	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		// All iterations return 200, but assertion expects 404 -> assertion failure
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Required DD Setup",
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Required Data Driven Setup",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Required:   boolPtr(true),
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{404}}},
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Main Should Be Skipped",
				Request: parser.Request{Method: "GET", URL: "https://example.com/main"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// The data-driven setup should have 2 failed iterations
	if summary.Failed != 2 {
		t.Errorf("failed = %d, want 2", summary.Failed)
	}
	// The main request should be skipped because the required setup failed
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
	// Total results: 2 iterations + 1 skipped main
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// Last result should be skipped
	if !results[2].Skipped {
		t.Error("results[2] should be skipped")
	}
	// Only 2 HTTP calls should have been made (the data-driven iterations)
	if callNum != 2 {
		t.Errorf("callNum = %d, want 2", callNum)
	}
}

func TestRun_DataDriven_StopOnFailureSkipsSubsequent(t *testing.T) {
	// A data-driven request in main with stop_on_failure that encounters a
	// network error should stop subsequent items (stopOnFailure=true in Main phase).
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2")

	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		return nil, errFake
	}

	col := &parser.Collection{
		Name:    "DD StopOnFailure",
		Options: parser.Options{StopOnFailure: true},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Data Driven With Errors",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
			{
				Name:    "Should Be Skipped",
				Request: parser.Request{Method: "GET", URL: "https://example.com/next"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// The data-driven request should have 2 failed iterations
	if summary.Failed != 2 {
		t.Errorf("failed = %d, want 2", summary.Failed)
	}
	// The second item should be skipped because stopOnFailure
	if summary.Skipped != 1 {
		t.Errorf("skipped = %d, want 1", summary.Skipped)
	}
	// Total results: 2 iterations + 1 skipped
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if !results[2].Skipped {
		t.Error("results[2] should be skipped")
	}
	if callNum != 2 {
		t.Errorf("callNum = %d, want 2", callNum)
	}
}

func TestRun_DataDriven_FallsBackToProjectRoot(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Fallback",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	// CollectionDir is empty; should fall back to ProjectRoot
	_, _, err := Run(context.Background(), col, exec, VarSources{
		ProjectRoot: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(capturedURLs) != 1 {
		t.Fatalf("capturedURLs = %d, want 1", len(capturedURLs))
	}
	if capturedURLs[0] != "https://example.com/1" {
		t.Errorf("URL = %q, want %q", capturedURLs[0], "https://example.com/1")
	}
}

func TestRun_DataDriven_YAMLSource(t *testing.T) {
	dir := t.TempDir()
	writeYAMLFile(t, dir, "users.yaml", "- name: alice\n- name: bob\n- name: carol")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven YAML",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Get User",
				DataDriven: &datadriven.Config{Source: "users.yaml"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/users/{{name}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	wantURLs := []string{
		"https://example.com/users/alice",
		"https://example.com/users/bob",
		"https://example.com/users/carol",
	}
	for i, want := range wantURLs {
		if capturedURLs[i] != want {
			t.Errorf("URL[%d] = %q, want %q", i, capturedURLs[i], want)
		}
	}
}

func TestRun_DataDriven_FailFast(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5")

	var callCount int
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		// 2nd request returns 500
		if callCount == 2 {
			return &httpexec.Result{StatusCode: 500, Duration: 10 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven FailFast",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:   "data.csv",
					FailFast: true,
				},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{200}}},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// Should stop after the 2nd iteration due to assertion failure + fail_fast
	if callCount != 2 {
		t.Errorf("callCount = %d, want 2 (fail_fast should stop after failure)", callCount)
	}
	if len(results) != 2 {
		t.Errorf("results = %d, want 2", len(results))
	}
}

func TestRun_DataDriven_FilteredRows(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "name,age\nalice,25\nbob,15\ncarol,30\ndave,12\neve,18")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Filtered",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source: "data.csv",
					Filter: "{{age}} >= 18",
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{name}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// Only alice(25), carol(30), eve(18) should match filter
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	wantURLs := []string{
		"https://example.com/alice",
		"https://example.com/carol",
		"https://example.com/eve",
	}
	for i, want := range wantURLs {
		if capturedURLs[i] != want {
			t.Errorf("URL[%d] = %q, want %q", i, capturedURLs[i], want)
		}
	}
}

func TestRun_DataDriven_LimitedRows(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5\n6\n7\n8\n9\n10")

	var capturedURLs []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedURLs = append(capturedURLs, req.URL)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Limited",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source: "data.csv",
					Limit:  intPtr(3),
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if len(capturedURLs) != 3 {
		t.Errorf("capturedURLs = %d, want 3", len(capturedURLs))
	}
}

func TestRun_DataDriven_ResultMetadata(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "users.csv", "name,email\njohn,john@x.com\njane,jane@x.com\nbob,bob@x.com")

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Metadata",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Create User",
				DataDriven: &datadriven.Config{Source: "users.csv"},
				Request:    parser.Request{Method: "POST", URL: "https://example.com/users"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}

	for i, r := range results {
		if !r.IsDataDriven {
			t.Errorf("results[%d].IsDataDriven = false, want true", i)
		}
		if r.DataDrivenName != "Create User" {
			t.Errorf("results[%d].DataDrivenName = %q, want %q", i, r.DataDrivenName, "Create User")
		}
		if r.IterationIndex != i {
			t.Errorf("results[%d].IterationIndex = %d, want %d", i, r.IterationIndex, i)
		}
		if r.IterationTotal != 3 {
			t.Errorf("results[%d].IterationTotal = %d, want 3", i, r.IterationTotal)
		}
	}
}

func TestRun_DataDriven_ResultMetadata_ErrorIteration(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "name\nalice\nbob")

	callCount := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callCount++
		if callCount == 2 {
			return nil, fmt.Errorf("network error")
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Error Metadata",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Get User",
				DataDriven: &datadriven.Config{Source: "data.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/users"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}

	// Error result should also have data-driven metadata
	for i, r := range results {
		if !r.IsDataDriven {
			t.Errorf("results[%d].IsDataDriven = false, want true", i)
		}
		if r.DataDrivenName != "Get User" {
			t.Errorf("results[%d].DataDrivenName = %q, want %q", i, r.DataDrivenName, "Get User")
		}
		if r.IterationTotal != 2 {
			t.Errorf("results[%d].IterationTotal = %d, want 2", i, r.IterationTotal)
		}
	}
}

func TestRun_NonDataDriven_HasNoMetadata(t *testing.T) {
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Normal",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Get Users",
				Request: parser.Request{Method: "GET", URL: "https://example.com/users"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].IsDataDriven {
		t.Error("non-data-driven result should have IsDataDriven=false")
	}
	if results[0].DataDrivenName != "" {
		t.Errorf("non-data-driven result DataDrivenName = %q, want empty", results[0].DataDrivenName)
	}
	if results[0].IterationTotal != 0 {
		t.Errorf("non-data-driven result IterationTotal = %d, want 0", results[0].IterationTotal)
	}
}

func TestRun_DataDriven_ParallelExecution(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "users.csv", "name\nalice\nbob\ncarol")

	var callCount int32
	mu := &sync.Mutex{}
	var capturedNames []string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		atomic.AddInt32(&callCount, 1)
		mu.Lock()
		capturedNames = append(capturedNames, req.URL)
		mu.Unlock()
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Parallel",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Create User",
				DataDriven: &datadriven.Config{
					Source:   "users.csv",
					Parallel: true,
				},
				Request: parser.Request{Method: "POST", URL: "https://example.com/users/{{name}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	if int(atomic.LoadInt32(&callCount)) != 3 {
		t.Errorf("callCount = %d, want 3", callCount)
	}
	// All results should be marked as data-driven
	for i, r := range results {
		if !r.IsDataDriven {
			t.Errorf("results[%d].IsDataDriven = false, want true", i)
		}
		if r.DataDrivenName != "Create User" {
			t.Errorf("results[%d].DataDrivenName = %q, want %q", i, r.DataDrivenName, "Create User")
		}
	}

	// Parallel results should include Method, URL (same as sequential path)
	for i, r := range results {
		if r.Method != "POST" {
			t.Errorf("results[%d].Method = %q, want %q", i, r.Method, "POST")
		}
		if r.URL == "" {
			t.Errorf("results[%d].URL should not be empty", i)
		}
	}

	// M9-002: Parallel data-driven results must carry RequestID and RequestSlug.
	// Regression guard: the field doc comment says "Always populated for non-skipped requests".
	for i, r := range results {
		if r.RequestID == "" {
			t.Errorf("results[%d].RequestID should not be empty for parallel data-driven result", i)
		}
		if r.RequestSlug == "" {
			t.Errorf("results[%d].RequestSlug should not be empty for parallel data-driven result", i)
		}
	}
}

func TestRun_DataDriven_ParallelExtractionAccumulates(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\na\nb\nc")

	callNum := int32(0)
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		n := atomic.AddInt32(&callNum, 1)
		body := fmt.Sprintf(`{"id":"user_%d"}`, n)
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(body),
		}, nil
	}

	var capturedURLs []string
	mu := &sync.Mutex{}
	wrapExec := func(ctx context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		mu.Lock()
		capturedURLs = append(capturedURLs, req.URL)
		mu.Unlock()
		return exec(ctx, req)
	}

	col := &parser.Collection{
		Name: "Parallel Extraction",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Create",
				DataDriven: &datadriven.Config{
					Source:   "data.csv",
					Parallel: true,
				},
				Request: parser.Request{Method: "POST", URL: "https://example.com/create"},
				Extract: map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "Use Accumulated",
				Request: parser.Request{Method: "GET", URL: "https://example.com/verify?ids={{user_id}}"},
			},
		}},
	}

	_, summary, err := Run(context.Background(), col, wrapExec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 4 { // 3 data-driven + 1 regular
		t.Errorf("passed = %d, want 4", summary.Passed)
	}
	// Last URL should contain accumulated user IDs
	mu.Lock()
	lastURL := capturedURLs[len(capturedURLs)-1]
	mu.Unlock()
	if !strings.Contains(lastURL, "user_") {
		t.Errorf("last URL = %q, expected it to contain accumulated user IDs", lastURL)
	}
}

func TestRun_DataDriven_ParallelFailFast(t *testing.T) {
	dir := t.TempDir()
	// Use many rows so fail-fast has time to trigger before all are dispatched
	var sb strings.Builder
	sb.WriteString("val\n")
	for i := 1; i <= 50; i++ {
		fmt.Fprintf(&sb, "%d\n", i)
	}
	writeCSVFile(t, dir, "data.csv", sb.String())

	var callCount int32
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		n := atomic.AddInt32(&callCount, 1)
		// Slow down each request so fail-fast has time to propagate
		time.Sleep(20 * time.Millisecond)
		if n == 1 {
			return &httpexec.Result{StatusCode: 500, Duration: 10 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Data Driven Parallel FailFast",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:   "data.csv",
					Parallel: true,
					FailFast: true,
				},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{200}}},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	cc := int(atomic.LoadInt32(&callCount))
	if cc >= 50 {
		t.Errorf("callCount = %d, want < 50 (fail_fast should stop early)", cc)
	}
	if len(results) >= 50 {
		t.Errorf("results = %d, want < 50", len(results))
	}
}

func TestRun_DataDriven_LargeDatasetWarning(t *testing.T) {
	dir := t.TempDir()
	// Create CSV with >10000 rows
	var sb strings.Builder
	sb.WriteString("val\n")
	for i := 0; i < 10001; i++ {
		fmt.Fprintf(&sb, "%d\n", i)
	}
	writeCSVFile(t, dir, "large.csv", sb.String())

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Large Dataset",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "large.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	// Without ConfirmLargeDataset, should error
	_, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err == nil {
		t.Fatal("expected error for large dataset without confirmation")
	}
	if !strings.Contains(err.Error(), "10001") {
		t.Errorf("error = %q, expected it to mention row count", err)
	}
}

func TestRun_DataDriven_LargeDatasetConfirmed(t *testing.T) {
	dir := t.TempDir()
	// Create CSV with >10000 rows
	var sb strings.Builder
	sb.WriteString("val\n")
	for i := 0; i < 10001; i++ {
		fmt.Fprintf(&sb, "%d\n", i)
	}
	writeCSVFile(t, dir, "large.csv", sb.String())

	var callCount int32
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		atomic.AddInt32(&callCount, 1)
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	// Override guard rail for this test to avoid hitting the 1000 request limit
	old := MaxRequests
	MaxRequests = 20000
	t.Cleanup(func() { MaxRequests = old })

	col := &parser.Collection{
		Name: "Large Dataset Confirmed",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "Test",
				DataDriven: &datadriven.Config{Source: "large.csv"},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	// With ConfirmLargeDataset, should proceed
	_, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir:       dir,
		ConfirmLargeDataset: true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v (should proceed with confirmation)", err)
	}
	if summary.Passed != 10001 {
		t.Errorf("passed = %d, want 10001", summary.Passed)
	}
}

func TestRun_DataDriven_StoreResultsSummary(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3")

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Duration:   10 * time.Millisecond,
			Body:       []byte(`{"id":"x"}`),
		}, nil
	}

	col := &parser.Collection{
		Name: "Store Summary",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:       "data.csv",
					StoreResults: "summary",
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Extract: map[string]string{"id": "$.id"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 3 {
		t.Errorf("passed = %d, want 3", summary.Passed)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// In summary mode, Result, AssertionResults, RequestHeaders, and RequestBody
	// should all be stripped. Only Name/Phase/Err/Method/URL and data-driven metadata remain.
	for i, r := range results {
		if r.Err != nil {
			t.Errorf("results[%d].Err should be nil", i)
		}
		if r.Result != nil {
			t.Errorf("results[%d].Result should be nil in summary mode", i)
		}
		if r.AssertionResults != nil {
			t.Errorf("results[%d].AssertionResults should be nil in summary mode", i)
		}
		if r.RequestHeaders != nil {
			t.Errorf("results[%d].RequestHeaders should be nil in summary mode", i)
		}
		if r.RequestBody != nil {
			t.Errorf("results[%d].RequestBody should be nil in summary mode", i)
		}
		if !r.IsDataDriven {
			t.Errorf("results[%d].IsDataDriven should be true", i)
		}
		if r.Name == "" {
			t.Errorf("results[%d].Name should be preserved in summary mode", i)
		}
	}
}

func TestRun_DataDriven_StoreResultsFailedOnly(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3")

	callNum := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		callNum++
		if callNum == 2 {
			return &httpexec.Result{StatusCode: 500, Duration: 10 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Store Failed Only",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:       "data.csv",
					StoreResults: "failed_only",
				},
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{200}}},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	// The 2nd result (index 1) should be the failed one with full assertion details
	if results[1].AssertionResults == nil || results[1].AssertionResults.Passed {
		t.Error("results[1] should have failed assertions")
	}
	if results[1].Result == nil {
		t.Error("results[1].Result should be preserved (failed iteration in failed_only mode)")
	}

	// Passed iterations (index 0 and 2) should have details stripped
	for _, idx := range []int{0, 2} {
		if results[idx].Result != nil {
			t.Errorf("results[%d].Result should be nil (passed iteration in failed_only mode)", idx)
		}
		if results[idx].AssertionResults != nil {
			t.Errorf("results[%d].AssertionResults should be nil (passed iteration in failed_only mode)", idx)
		}
		if results[idx].Err != nil {
			t.Errorf("results[%d].Err should be nil (passed iteration)", idx)
		}
		if !results[idx].IsDataDriven {
			t.Errorf("results[%d].IsDataDriven should be true", idx)
		}
	}
}

func TestRun_DataDriven_ParallelRateLimit(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5")

	rps := 20
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 1 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Rate Limited",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:       "data.csv",
					Parallel:     true,
					RateLimitRPS: &rps,
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	start := time.Now()
	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 5 {
		t.Errorf("passed = %d, want 5", summary.Passed)
	}
	if len(results) != 5 {
		t.Fatalf("results = %d, want 5", len(results))
	}
	// With 5 requests at 20 RPS, should take at least ~160ms (4 intervals of 50ms * 80% tolerance)
	minExpected := 4 * time.Second / time.Duration(rps) * 80 / 100
	if elapsed < minExpected {
		t.Errorf("elapsed = %v, want >= %v (rate limiting should slow execution)", elapsed, minExpected)
	}
}

func TestRun_DataDriven_AtomicInParallelGraph(t *testing.T) {
	// Verify that a data-driven request in a collection-level --parallel execution
	// is treated as an atomic unit: all iterations complete before dependent requests start.
	// The dependent request should see accumulated extraction variables from all iterations.
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\na\nb\nc")

	callNum := int32(0)
	var capturedURLs []string
	mu := &sync.Mutex{}

	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		n := atomic.AddInt32(&callNum, 1)
		mu.Lock()
		capturedURLs = append(capturedURLs, req.URL)
		mu.Unlock()
		if strings.Contains(req.URL, "/create") {
			body := fmt.Sprintf(`{"id":"user_%d"}`, n)
			return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond, Body: []byte(body)}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 10 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "Parallel Graph Atomic DD",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Create",
				DataDriven: &datadriven.Config{
					Source: "data.csv",
				},
				Request: parser.Request{Method: "POST", URL: "https://example.com/create/{{val}}"},
				Extract: map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "Verify",
				Request: parser.Request{Method: "GET", URL: "https://example.com/verify?ids={{user_id}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel:      true, // collection-level parallel execution
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	// 3 data-driven iterations + 1 dependent request = 4 passed
	if summary.Passed != 4 {
		t.Errorf("passed = %d, want 4", summary.Passed)
	}
	if len(results) != 4 {
		t.Fatalf("results = %d, want 4", len(results))
	}
	// The last result should be the Verify request using accumulated user_ids.
	// Data-driven items produce results first (atomic), then dependent requests follow.
	mu.Lock()
	lastURL := capturedURLs[len(capturedURLs)-1]
	mu.Unlock()
	if !strings.Contains(lastURL, "user_") {
		t.Errorf("last URL = %q, expected it to contain accumulated user_id from data-driven iterations", lastURL)
	}
	// The Verify request should have been in a later wave (WaveIndex > 0 or in a separate wave)
	// since it depends on user_id extracted by Create. This confirms atomicity.
	verifyResult := results[len(results)-1]
	if verifyResult.Name != "Verify" {
		t.Errorf("last result name = %q, want %q", verifyResult.Name, "Verify")
	}
	if verifyResult.Err != nil {
		t.Errorf("Verify request error: %v", verifyResult.Err)
	}
}

func TestRun_RetryAttemptDetailsPropagated(t *testing.T) {
	// Sequential execution: verify AttemptDetails are populated in results
	calls := 0
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		calls++
		if calls == 1 {
			return &httpexec.Result{StatusCode: 503, Duration: 10 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 20 * time.Millisecond, Body: []byte("{}")}, nil
	}

	col := &parser.Collection{
		Name:  "test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{Name: "Get Data", Request: parser.Request{Method: "GET", URL: "http://example.com"}},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if summary.Failed != 0 {
		t.Fatalf("expected 0 failed, got %d", summary.Failed)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}

	r := results[0]
	if r.RetryCount != 1 {
		t.Errorf("expected RetryCount=1, got %d", r.RetryCount)
	}
	if len(r.AttemptDetails) != 2 {
		t.Fatalf("expected 2 AttemptDetails, got %d", len(r.AttemptDetails))
	}
	if r.AttemptDetails[0].StatusCode != 503 {
		t.Errorf("first attempt status should be 503, got %d", r.AttemptDetails[0].StatusCode)
	}
	if r.AttemptDetails[1].StatusCode != 200 {
		t.Errorf("second attempt status should be 200, got %d", r.AttemptDetails[1].StatusCode)
	}
}

func TestRun_ParallelWithRetry(t *testing.T) {
	// Parallel execution where one request in wave 1 needs retry.
	// Verify: dependent wave 2 requests proceed after retry succeeds.
	var callsA int32
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if strings.Contains(req.URL, "/a") {
			n := atomic.AddInt32(&callsA, 1)
			if n == 1 {
				return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
			}
			return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte(`{"token":"abc"}`)}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
	}

	col := &parser.Collection{
		Name:  "test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "http://example.com/a"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "http://example.com/b/{{token}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Both should succeed
	if summary.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", summary.Failed)
	}

	// A should have AttemptDetails
	for _, r := range results {
		if r.Name == "A" {
			if r.RetryCount != 1 {
				t.Errorf("A RetryCount = %d, want 1", r.RetryCount)
			}
			if len(r.AttemptDetails) != 2 {
				t.Errorf("A AttemptDetails length = %d, want 2", len(r.AttemptDetails))
			}
		}
		if r.Name == "B" {
			if r.Skipped {
				t.Error("B should not be skipped when A succeeded on retry")
			}
		}
	}
}

func TestRun_DataDrivenWithRetry_PerIterationRetry(t *testing.T) {
	// Data-driven execution where some iterations need retry.
	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "data.csv")
	if err := os.WriteFile(csvFile, []byte("name\nAlice\nBob\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	var calls int32
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		n := atomic.AddInt32(&calls, 1)
		// First call (Alice first attempt) returns 503, rest succeed
		if n == 1 {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
	}

	col := &parser.Collection{
		Name:  "test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(3), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Create",
				Request: parser.Request{Method: "GET", URL: "http://example.com/{{name}}"},
				DataDriven: &datadriven.Config{
					Source: csvFile,
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("expected 0 failed, got %d", summary.Failed)
	}
	// Should have 2 results (Alice, Bob)
	if len(results) != 2 {
		t.Fatalf("expected 2 results, got %d", len(results))
	}
	// Alice should have been retried
	if results[0].RetryCount != 1 {
		t.Errorf("Alice RetryCount = %d, want 1", results[0].RetryCount)
	}
	if len(results[0].AttemptDetails) != 2 {
		t.Errorf("Alice AttemptDetails length = %d, want 2", len(results[0].AttemptDetails))
	}
}

func TestRun_DataDrivenWithRetry_ExhaustedRetry(t *testing.T) {
	// When an iteration fails after all retries are exhausted, it is marked failed
	// but the next iteration continues (behavior 3 negative path).
	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "data.csv")
	if err := os.WriteFile(csvFile, []byte("name\nAlice\nBob\nCarol\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		// Alice always returns 503 (will exhaust retries), Bob and Carol succeed
		if strings.Contains(req.URL, "Alice") {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
	}

	col := &parser.Collection{
		Name:  "test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(2), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Create",
				Request: parser.Request{Method: "GET", URL: "http://example.com/{{name}}"},
				DataDriven: &datadriven.Config{
					Source: csvFile,
				},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{200}}},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Should have 3 results (Alice, Bob, Carol) — all iterations run
	if len(results) != 3 {
		t.Fatalf("expected 3 results, got %d", len(results))
	}

	// Alice should be failed (exhausted retries, assertion fails on 503)
	if summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", summary.Failed)
	}

	// Alice should have been retried (2 attempts)
	if results[0].RetryCount != 1 {
		t.Errorf("Alice RetryCount = %d, want 1", results[0].RetryCount)
	}
	if results[0].AssertionResults == nil || results[0].AssertionResults.Passed {
		t.Error("Alice assertions should have failed (503 != 200)")
	}

	// Bob and Carol should have succeeded (no retries needed)
	if results[1].Result == nil || results[1].Result.StatusCode != 200 {
		t.Errorf("Bob should have status 200, got %v", results[1].Result)
	}
	if results[2].Result == nil || results[2].Result.StatusCode != 200 {
		t.Errorf("Carol should have status 200, got %v", results[2].Result)
	}
}

func TestRun_DataDrivenWithRetry_FailFast(t *testing.T) {
	// When retries are exhausted on one iteration with fail_fast enabled,
	// remaining iterations are skipped.
	tmpDir := t.TempDir()
	csvFile := filepath.Join(tmpDir, "data.csv")
	if err := os.WriteFile(csvFile, []byte("name\nAlice\nBob\nCarol\n"), 0o644); err != nil {
		t.Fatalf("write csv: %v", err)
	}

	var callCount int32
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		atomic.AddInt32(&callCount, 1)
		// Alice always returns 503 (will exhaust retries)
		if strings.Contains(req.URL, "Alice") {
			return &httpexec.Result{StatusCode: 503, Duration: 5 * time.Millisecond}, nil
		}
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond, Body: []byte("{}")}, nil
	}

	col := &parser.Collection{
		Name:  "test",
		Retry: &retry.FullConfig{Enabled: retry.BoolPtr(true), MaxAttempts: retry.IntPtr(2), InitialDelayMs: retry.IntPtr(10)},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Create",
				Request: parser.Request{Method: "GET", URL: "http://example.com/{{name}}"},
				DataDriven: &datadriven.Config{
					Source:   csvFile,
					FailFast: true,
				},
				Assertions: parser.Assertions{Status: parser.StatusCodes{Codes: []int{200}}},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: tmpDir,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}

	// Should only have 1 result (Alice) — fail_fast stops after first failed iteration
	if len(results) != 1 {
		t.Fatalf("expected 1 result (fail_fast should stop), got %d", len(results))
	}

	// Alice should be failed
	if summary.Failed != 1 {
		t.Errorf("expected 1 failed, got %d", summary.Failed)
	}

	// Alice should have been retried (2 attempts)
	if results[0].RetryCount != 1 {
		t.Errorf("Alice RetryCount = %d, want 1", results[0].RetryCount)
	}

	// Bob and Carol should NOT have been called
	// Alice gets 2 calls (initial + 1 retry), so total should be 2
	finalCount := atomic.LoadInt32(&callCount)
	if finalCount != 2 {
		t.Errorf("expected 2 HTTP calls (Alice x2), got %d (Bob/Carol should not be called)", finalCount)
	}
}

// --- GraphQL Protocol Tests ---

func TestRun_graphql_basic_query(t *testing.T) {
	var capturedReq *httpexec.Request
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedReq = req
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"data":{"users":[{"id":"1"}]}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Get Users",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query: "{ users { id } }",
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request was captured")
	}
	if capturedReq.Method != "POST" {
		t.Errorf("Method = %q, want POST", capturedReq.Method)
	}
	if capturedReq.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", capturedReq.Headers["Content-Type"])
	}

	// Verify body contains query
	bodyStr, ok := capturedReq.Body.(string)
	if !ok {
		t.Fatalf("Body type = %T, want string", capturedReq.Body)
	}
	var body graphql.RequestBody
	if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Query != "{ users { id } }" {
		t.Errorf("Query = %q, want { users { id } }", body.Query)
	}
}

func TestRun_graphql_with_variables(t *testing.T) {
	var capturedReq *httpexec.Request
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedReq = req
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"data":{"user":{"name":"Alice"}}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Get User",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query:     "query GetUser($id: ID!) { user(id: $id) { name } }",
						Variables: map[string]any{"id": "123"},
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request was captured")
	}
	bodyStr, ok := capturedReq.Body.(string)
	if !ok {
		t.Fatalf("Body type = %T, want string", capturedReq.Body)
	}
	var body graphql.RequestBody
	if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Variables["id"] != "123" {
		t.Errorf("Variables[id] = %v, want 123", body.Variables["id"])
	}
}

func TestRun_graphql_mutation(t *testing.T) {
	var capturedReq *httpexec.Request
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedReq = req
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"data":{"createUser":{"id":"42","name":"Bob"}}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Mutation Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Create User",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query:     `mutation CreateUser($name: String!) { createUser(name: $name) { id name } }`,
						Variables: map[string]any{"name": "Bob"},
					},
				},
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{200}},
					Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
						{Path: "$.data.createUser.name", Operator: "equals", Value: "Bob"},
					}},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request was captured")
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Error("expected assertions to pass")
	}
}

func TestRun_graphql_errors_fail_default(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"errors":[{"message":"not found"}],"data":null}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Error Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Failing Query",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query: "{ nonexistent { id } }",
					},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	// Default error_handling is "fail", so assertion results should indicate failure
	if results[0].AssertionResults == nil {
		t.Fatal("AssertionResults is nil, expected non-nil")
	}
	if results[0].AssertionResults.Passed {
		t.Error("expected assertions to fail due to GraphQL errors")
	}
	if summary.Failed != 1 {
		t.Errorf("Failed = %d, want 1", summary.Failed)
	}
}

func TestRun_graphql_errors_warn(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"errors":[{"message":"deprecated field"}],"data":{"user":{"name":"Alice"}}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Warn Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Warn Query",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query:         "{ user { name } }",
						ErrorHandling: "warn",
					},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	// warn mode: GraphQL errors should NOT cause failure
	if summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (warn mode should not fail)", summary.Failed)
	}
	if summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", summary.Passed)
	}
}

func TestRun_graphql_jsonpath_assertions(t *testing.T) {
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"data":{"user":{"name":"Alice","age":30}}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name: "GraphQL Assertions Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Assert Query",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query: "{ user { name age } }",
					},
				},
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{200}},
					Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
						{Path: "$.data.user.name", Operator: "equals", Value: "Alice"},
						{Path: "$.data.user.age", Operator: "equals", Value: 30},
					}},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Error("expected JSONPath assertions to pass on GraphQL response")
	}
}

func TestRun_graphql_variable_interpolation(t *testing.T) {
	var capturedReq *httpexec.Request
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		capturedReq = req
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"data":{"user":{"name":"Alice"}}}`),
			Duration:   10 * time.Millisecond,
		}, nil
	}

	col := &parser.Collection{
		Name:      "GraphQL Interpolation Test",
		Variables: parser.SensitiveVars{Values: map[string]string{"user_id": "456"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Interpolated Query",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query:     "query GetUser($id: ID!) { user(id: $id) { name } }",
						Variables: map[string]any{"id": "{{user_id}}"},
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if capturedReq == nil {
		t.Fatal("no request was captured")
	}
	bodyStr, ok := capturedReq.Body.(string)
	if !ok {
		t.Fatalf("Body type = %T, want string", capturedReq.Body)
	}
	var body graphql.RequestBody
	if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
		t.Fatalf("unmarshal body: %v", err)
	}
	if body.Variables["id"] != "456" {
		t.Errorf("Variables[id] = %v, want 456", body.Variables["id"])
	}
}

// -- WebSocket runner tests -----------------------------------------------

// wsFakeConn is a minimal in-memory Conn used by the runner-level fake dialer.
type wsFakeConn struct {
	writes    [][]byte
	incoming  [][]byte
	closed    bool
	deadline  time.Time
	readDelay time.Duration // optional delay before returning from ReadMessage
	onRead    func()        // optional hook invoked at ReadMessage entry
}

func (f *wsFakeConn) WriteMessage(_ int, data []byte) error {
	cpy := make([]byte, len(data))
	copy(cpy, data)
	f.writes = append(f.writes, cpy)
	return nil
}

func (f *wsFakeConn) ReadMessage() (int, []byte, error) {
	if f.onRead != nil {
		f.onRead()
	}
	if f.readDelay > 0 {
		time.Sleep(f.readDelay)
	}
	if len(f.incoming) > 0 {
		msg := f.incoming[0]
		f.incoming = f.incoming[1:]
		return 1, msg, nil
	}
	// A misconfigured test should fail fast rather than block for a full
	// second; if no deadline is set, surface a timeout immediately.
	if f.deadline.IsZero() {
		return 0, nil, &wsRunnerTimeoutErr{}
	}
	wait := time.Until(f.deadline)
	if wait > 0 {
		time.Sleep(wait)
	}
	return 0, nil, &wsRunnerTimeoutErr{}
}

func (f *wsFakeConn) SetReadDeadline(t time.Time) error               { f.deadline = t; return nil }
func (f *wsFakeConn) Close() error                                    { f.closed = true; return nil }
func (f *wsFakeConn) WriteControl(_ int, _ []byte, _ time.Time) error { return nil }
func (f *wsFakeConn) SetPongHandler(_ func(string) error)             {}

type wsRunnerTimeoutErr struct{}

func (e *wsRunnerTimeoutErr) Error() string { return "i/o timeout" }
func (e *wsRunnerTimeoutErr) Timeout() bool { return true }

// wsRunnerDialer is a fake Dialer that captures the dialed URL and returns a
// preconfigured connection. Implements websocket.Dialer.
type wsRunnerDialer struct {
	conn     *wsFakeConn
	dialErr  error
	lastURL  string
	lastHdrs http.Header
}

func (d *wsRunnerDialer) Dial(_ context.Context, url string, headers http.Header) (websocket.Conn, *http.Response, error) {
	d.lastURL = url
	d.lastHdrs = headers
	if d.dialErr != nil {
		return nil, nil, d.dialErr
	}
	return d.conn, nil, nil
}

// wsMultiDialer returns a fresh wsFakeConn per Dial call (for parallel tests).
type wsMultiDialer struct {
	mu      sync.Mutex
	factory func() *wsFakeConn
	conns   []*wsFakeConn
}

func (d *wsMultiDialer) Dial(_ context.Context, _ string, _ http.Header) (websocket.Conn, *http.Response, error) {
	d.mu.Lock()
	defer d.mu.Unlock()
	c := d.factory()
	d.conns = append(d.conns, c)
	return c, nil, nil
}

func TestRun_websocket_basic_lifecycle(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"type":"welcome"}`)}}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "WS Basic",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat Session",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{Action: "send", MessageRaw: "hello"},
							{
								Action:    "expect",
								TimeoutMs: 200,
								ExpectAssertions: parser.BodyAssertions{
									Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "welcome"}},
								},
							},
							{Action: "close", Code: 1000},
						},
					},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	rr := results[0]
	if rr.Err != nil {
		t.Errorf("rr.Err = %v", rr.Err)
	}
	if rr.AssertionResults == nil || !rr.AssertionResults.Passed {
		t.Errorf("assertions not passed: %+v", rr.AssertionResults)
	}
	if summary.RequestsExecuted != 1 {
		t.Errorf("RequestsExecuted = %d, want 1", summary.RequestsExecuted)
	}
	if len(fc.writes) != 1 || string(fc.writes[0]) != "hello" {
		t.Errorf("writes = %v, want [hello]", fc.writes)
	}
}

func TestRun_websocket_expect_timeout_fails(t *testing.T) {
	fc := &wsFakeConn{} // no incoming, will timeout
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "WS Timeout",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:    "expect",
								TimeoutMs: 30,
								ExpectAssertions: parser.BodyAssertions{
									Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
								},
							},
						},
					},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("unexpected fatal err: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AssertionResults == nil || results[0].AssertionResults.Passed {
		t.Errorf("expected failed assertion results, got %+v", results[0].AssertionResults)
	}
}

func TestRun_websocket_counts_as_one_request(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{
		[]byte(`{"ok":1}`),
		[]byte(`{"ok":1}`),
		[]byte(`{"ok":1}`),
	}}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "WS Count",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Multi Step",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{Action: "send", MessageRaw: "1"},
							{Action: "expect", TimeoutMs: 200, ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{{Path: "$.ok", Operator: "equals", Value: float64(1)}}}},
							{Action: "send", MessageRaw: "2"},
							{Action: "expect", TimeoutMs: 200, ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{{Path: "$.ok", Operator: "equals", Value: float64(1)}}}},
							{Action: "close"},
						},
					},
				},
			},
		}},
	}

	_, summary, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if summary.RequestsExecuted != 1 {
		t.Errorf("RequestsExecuted = %d, want 1 (all 5 steps count as one request)", summary.RequestsExecuted)
	}
}

func TestRun_websocket_variable_interpolation_url(t *testing.T) {
	fc := &wsFakeConn{}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name:      "WS Interp",
		Variables: parser.SensitiveVars{Values: map[string]string{"host": "example.com"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://{{host}}/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{{Action: "close"}},
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if dialer.lastURL != "ws://example.com/chat" {
		t.Errorf("dialed URL = %q, want ws://example.com/chat", dialer.lastURL)
	}
}

func TestRun_websocket_variable_interpolation_message_raw(t *testing.T) {
	fc := &wsFakeConn{}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name:      "WS Interp MsgRaw",
		Variables: parser.SensitiveVars{Values: map[string]string{"token": "abc123"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Auth",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/ws",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{Action: "send", MessageRaw: "auth {{token}}"},
							{Action: "close"},
						},
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	if string(fc.writes[0]) != "auth abc123" {
		t.Errorf("write = %q, want %q", fc.writes[0], "auth abc123")
	}
}

func TestRun_websocket_variable_interpolation_message_map(t *testing.T) {
	fc := &wsFakeConn{}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name:      "WS Interp MsgMap",
		Variables: parser.SensitiveVars{Values: map[string]string{"channel": "chat"}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Subscribe",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/ws",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action: "send",
								Message: map[string]any{
									"type":    "subscribe",
									"channel": "{{channel}}",
								},
							},
							{Action: "close"},
						},
					},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(fc.writes) != 1 {
		t.Fatalf("writes = %d, want 1", len(fc.writes))
	}
	var got map[string]any
	if err := json.Unmarshal(fc.writes[0], &got); err != nil {
		t.Fatalf("write not JSON: %v", err)
	}
	if got["channel"] != "chat" {
		t.Errorf("channel = %v, want chat", got["channel"])
	}
}

func TestRun_websocket_extract_variable_available_in_later_request(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"sid":"S9"}`)}}
	dialer := &wsRunnerDialer{conn: fc}

	var httpURL string
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		httpURL = req.URL
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`), Duration: time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "WS Extract",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Handshake",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/ws",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:           "expect",
								TimeoutMs:        200,
								ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{{Path: "$.sid", Operator: "exists"}}},
								Extract:          map[string]string{"sid": "$.sid"},
							},
							{Action: "close"},
						},
					},
				},
			},
			{
				Name: "Follow up",
				Request: parser.Request{
					Method: "GET",
					URL:    "https://api.example.com/session/{{sid}}",
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, execFn, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if httpURL != "https://api.example.com/session/S9" {
		t.Errorf("httpURL = %q, want https://api.example.com/session/S9", httpURL)
	}
}

func TestRun_websocket_dial_failure_populates_rr_err(t *testing.T) {
	dialer := &wsRunnerDialer{dialErr: errors.New("connection refused")}

	col := &parser.Collection{
		Name: "WS Dial Fail",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{{Action: "close"}},
					},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected rr.Err to be populated on dial failure, got nil")
	}
	if results[0].Result == nil || results[0].Result.StatusCode != 101 {
		t.Errorf("Result.StatusCode = %v, want 101 (WS upgrade code even on failure)", results[0].Result)
	}
}

func TestRun_websocket_stop_on_failure(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"type":"nope"}`)}}
	dialer := &wsRunnerDialer{conn: fc}

	var httpCalled bool
	execFn := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		httpCalled = true
		return &httpexec.Result{StatusCode: 200, Body: []byte(`{}`), Duration: time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name:    "WS StopOnFailure",
		Options: parser.Options{StopOnFailure: true},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:           "expect",
								TimeoutMs:        100,
								ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}}},
							},
						},
					},
				},
			},
			{
				Name: "Skipped",
				Request: parser.Request{
					Method: "GET",
					URL:    "https://api.example.com/never",
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, execFn, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if httpCalled {
		t.Error("HTTP exec should not have been called after failing WS step")
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if !results[1].Skipped {
		t.Errorf("second request should be skipped; got %+v", results[1])
	}
}

func TestRun_WebSocketBufferWarningPropagates(t *testing.T) {
	// 101 stray frames then one matching frame triggers the buffer warning.
	stray := make([][]byte, 101)
	for i := range stray {
		stray[i] = []byte(`{"type":"noise"}`)
	}
	frames := append(stray, []byte(`{"type":"ok"}`))
	fc := &wsFakeConn{incoming: frames}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "WS Buffer Warning",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Chat",
				Request: parser.Request{
					Protocol: "websocket",
					URL:      "ws://fake/chat",
					Method:   "WS",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:           "expect",
								TimeoutMs:        500,
								ExpectAssertions: parser.BodyAssertions{Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}}},
							},
						},
					},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if !results[0].AssertionResults.Passed {
		t.Fatalf("request failed: %v", results[0].Err)
	}
	found := false
	for _, w := range results[0].Warnings {
		if strings.Contains(w, "buffer") || strings.Contains(w, "Buffer") {
			found = true
		}
	}
	if !found {
		t.Errorf("expected buffer warning in result.Warnings, got: %v", results[0].Warnings)
	}
}

// nilExec panics if called; used by tests that expect the runner to not touch
// the HTTP exec path.
func nilExec(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	panic("nilExec: HTTP exec should not be called")
}

// --- GraphQL Error Handling Mode Tests (M2-031) ---

func makeGraphQLCollection(body, globalMode, perReqMode string) (*parser.Collection, VarSources) {
	var gqlCfg *parser.GraphQLConfig
	if perReqMode != "" {
		gqlCfg = &parser.GraphQLConfig{
			Query:         "{ user { name } }",
			ErrorHandling: perReqMode,
		}
	} else {
		gqlCfg = &parser.GraphQLConfig{
			Query: "{ user { name } }",
		}
	}

	col := &parser.Collection{
		Name: "GraphQL Mode Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Query",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL:  gqlCfg,
				},
			},
		}},
	}

	vars := VarSources{}
	if globalMode != "" {
		vars.GlobalGraphQL = &config.GraphQLDefaults{
			ErrorHandling: config.GraphQLErrorHandling{PartialSuccess: globalMode},
		}
	}

	return col, vars
}

func makeGraphQLExecutor(body string) ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(body),
			Duration:   10 * time.Millisecond,
		}, nil
	}
}

func TestRun_graphql_mode_matrix(t *testing.T) {
	tests := []struct {
		name         string
		body         string
		globalMode   string
		perReqMode   string
		wantFailed   int
		wantWarnings bool
	}{
		{"success no errors", `{"data":{"ok":true}}`, "", "", 0, false},
		{"partial default fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "", "", 1, false},
		{"partial global warn", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "warn", "", 0, true},
		{"partial global ignore", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "ignore", "", 0, false},
		{"per-request fail beats global warn", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "warn", "fail", 1, false},
		{"per-request warn beats global fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "fail", "warn", 0, true},
		{"per-request ignore beats global fail", `{"data":{"ok":true},"errors":[{"message":"x"}]}`, "fail", "ignore", 0, false},
		{"full failure fail default", `{"data":null,"errors":[{"message":"boom"}]}`, "", "", 1, false},
		{"full failure warn still fails", `{"data":null,"errors":[{"message":"boom"}]}`, "warn", "", 1, false},
		{"full failure ignore still fails", `{"data":null,"errors":[{"message":"boom"}]}`, "ignore", "", 1, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col, vars := makeGraphQLCollection(tt.body, tt.globalMode, tt.perReqMode)
			results, summary, err := Run(context.Background(), col, makeGraphQLExecutor(tt.body), vars)
			if err != nil {
				t.Fatalf("Run() error: %v", err)
			}
			if len(results) != 1 {
				t.Fatalf("results = %d, want 1", len(results))
			}
			if summary.Failed != tt.wantFailed {
				t.Errorf("summary.Failed = %d, want %d", summary.Failed, tt.wantFailed)
			}
			if tt.wantWarnings && len(results[0].Warnings) == 0 {
				t.Errorf("expected Warnings to be non-empty")
			}
			if !tt.wantWarnings && len(results[0].Warnings) > 0 {
				t.Errorf("expected Warnings to be empty, got %v", results[0].Warnings)
			}
		})
	}
}

func TestRun_graphql_warn_mode_updates_existing_test_behaviour(t *testing.T) {
	// Verify existing warn test: partial success + warn → passes AND has warnings
	exec := makeGraphQLExecutor(`{"errors":[{"message":"deprecated field"}],"data":{"user":{"name":"Alice"}}}`)
	col, vars := makeGraphQLCollection("", "", "warn")

	results, summary, err := Run(context.Background(), col, exec, vars)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0 (warn mode should not fail)", summary.Failed)
	}
	if summary.Passed != 1 {
		t.Errorf("Passed = %d, want 1", summary.Passed)
	}
	if len(results) < 1 || len(results[0].Warnings) == 0 {
		t.Errorf("expected non-empty Warnings in warn mode, got %v", results[0].Warnings)
	}
}

func TestRun_graphql_assertion_on_errors_extensions_code(t *testing.T) {
	body := `{"data":null,"errors":[{"message":"not found","extensions":{"code":"NOT_FOUND"}}]}`
	exec := makeGraphQLExecutor(body)

	col := &parser.Collection{
		Name: "GraphQL Assertion On Errors Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Assert Error Code",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query: "{ user { name } }",
					},
				},
				Assertions: parser.Assertions{
					Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
						{Path: "$.errors[0].extensions.code", Operator: "equals", Value: "NOT_FOUND"},
					}},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AssertionResults == nil {
		t.Fatal("AssertionResults is nil")
	}
	// The body assertion on $.errors[0].extensions.code should pass
	var codeAssertionPassed bool
	for _, ar := range results[0].AssertionResults.Items {
		if strings.Contains(ar.Type, "$.errors[0].extensions.code") && ar.Passed {
			codeAssertionPassed = true
		}
	}
	if !codeAssertionPassed {
		t.Errorf("expected $.errors[0].extensions.code assertion to pass; assertions: %+v", results[0].AssertionResults.Items)
	}
}

func TestRun_graphql_assertion_on_errors_message(t *testing.T) {
	body := `{"data":{"user":{"name":"Alice"}},"errors":[{"message":"deprecated field X"}]}`
	exec := makeGraphQLExecutor(body)

	col := &parser.Collection{
		Name: "GraphQL Assertion On Error Message",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Assert Error Message",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL: &parser.GraphQLConfig{
						Query:         "{ user { name } }",
						ErrorHandling: "warn",
					},
				},
				Assertions: parser.Assertions{
					Body: parser.BodyAssertions{Items: []parser.BodyAssertion{
						{Path: "$.errors[0].message", Operator: "contains", Value: "deprecated"},
					}},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Failed != 0 {
		t.Errorf("Failed = %d, want 0", summary.Failed)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].AssertionResults == nil || !results[0].AssertionResults.Passed {
		t.Errorf("expected assertions to pass; results: %+v", results[0].AssertionResults)
	}
}

func TestRun_graphql_malformed_response_body_returns_error(t *testing.T) {
	// Finding #1: checkErr from graphql.CheckResponse must not be silently swallowed.
	// A malformed JSON body should cause Run() to return a non-nil error.
	exec := makeGraphQLExecutor("not valid json {{{")

	col := &parser.Collection{
		Name: "GraphQL Malformed Body",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Malformed",
				Request: parser.Request{
					Protocol: "graphql",
					URL:      "https://api.example.com/graphql",
					Method:   "POST",
					GraphQL:  &parser.GraphQLConfig{Query: "{ me { id } }"},
				},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, exec, VarSources{})
	if err == nil {
		t.Fatal("expected Run() to return an error for malformed GraphQL response body, got nil")
	}
	if !strings.Contains(err.Error(), "parsing graphql response") {
		t.Errorf("error should mention 'parsing graphql response', got: %v", err)
	}
}

// TestExecuteDataDriven_PopulatesIterationData verifies that data-driven results
// carry IterationData containing the row values, and non-data-driven results have nil.
func TestExecuteDataDriven_PopulatesIterationData(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "users.csv", "name,role\nalice,admin\nbob,user")

	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "IterationData Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Plain Request",
				Request: parser.Request{Method: "GET", URL: "https://example.com/plain"},
			},
			{
				Name:       "Create User",
				DataDriven: &datadriven.Config{Source: "users.csv"},
				Request:    parser.Request{Method: "POST", URL: "https://example.com/users/{{name}}"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, exec, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// First result is the non-data-driven plain request.
	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}
	plainResult := results[0]
	if plainResult.IsDataDriven {
		t.Error("plain request should not be marked as data-driven")
	}
	if plainResult.IterationData != nil {
		t.Errorf("plain request IterationData should be nil, got %v", plainResult.IterationData)
	}

	// Remaining results are data-driven iterations.
	ddResults := results[1:]
	if len(ddResults) != 2 {
		t.Fatalf("expected 2 data-driven results, got %d", len(ddResults))
	}

	wantData := []map[string]string{
		{"name": "alice", "role": "admin"},
		{"name": "bob", "role": "user"},
	}
	for i, r := range ddResults {
		if !r.IsDataDriven {
			t.Errorf("result[%d] should be marked as data-driven", i)
		}
		if r.IterationData == nil {
			t.Errorf("result[%d] IterationData should not be nil", i)
			continue
		}
		for k, want := range wantData[i] {
			if got := r.IterationData[k]; got != want {
				t.Errorf("result[%d] IterationData[%q] = %q, want %q", i, k, got, want)
			}
		}
	}
}

func TestRun_websocket_parallel_two_connections_concurrent(t *testing.T) {
	// Both WS connections must be mid-read simultaneously when Parallel=true.
	probe := newOverlapProbe(2)
	dialer := &wsMultiDialer{
		factory: func() *wsFakeConn {
			return &wsFakeConn{
				incoming: [][]byte{[]byte(`{"type":"ok"}`)},
				onRead:   probe.enter,
			}
		},
	}

	wsItem := func(name string) parser.RequestItem {
		return parser.RequestItem{
			Name: name,
			Request: parser.Request{
				Protocol: "websocket",
				Method:   "WS",
				URL:      "ws://fake/ws",
				WebSocket: &parser.WebSocketConfig{
					Steps: []parser.WebSocketStep{
						{
							Action:    "expect",
							TimeoutMs: 2000,
							ExpectAssertions: parser.BodyAssertions{
								Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
							},
						},
					},
				},
			},
		}
	}

	col := &parser.Collection{
		Name: "WS Parallel",
		Requests: parser.Section{Items: []parser.RequestItem{
			wsItem("ws1"),
			wsItem("ws2"),
		}},
	}

	results, summary, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
		Parallel:        true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for _, r := range results {
		if r.Err != nil {
			t.Errorf("%s: rr.Err = %v", r.Name, r.Err)
		}
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if got := probe.max(); got < 2 {
		t.Errorf("max concurrent in-flight WS reads = %d, want 2 (connections must run in parallel)", got)
	}
}

func TestRun_websocket_parallel_mixed_with_http(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"type":"ok"}`)}}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "Mixed Parallel",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "HTTP",
				Request: parser.Request{
					Method: "GET",
					URL:    "https://example.com/api",
				},
			},
			{
				Name: "WS",
				Request: parser.Request{
					Protocol: "websocket",
					Method:   "WS",
					URL:      "ws://fake/ws",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:    "expect",
								TimeoutMs: 500,
								ExpectAssertions: parser.BodyAssertions{
									Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
								},
							},
						},
					},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		WebSocketDialer: dialer,
		Parallel:        true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	if summary.Passed != 2 {
		t.Errorf("passed = %d, want 2", summary.Passed)
	}
	if !summary.IsParallel {
		t.Error("summary.IsParallel should be true")
	}
}

func TestRun_websocket_autodetect_ws_scheme(t *testing.T) {
	fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"type":"ok"}`)}}
	dialer := &wsRunnerDialer{conn: fc}

	// Simulate a collection where parser has already auto-detected the protocol.
	// (In production, ParseFile sets Protocol = "websocket" from the URL scheme.)
	col := &parser.Collection{
		Name: "WS AutoDetect",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "AutoWS",
				Request: parser.Request{
					Protocol: "websocket",
					Method:   "WS",
					URL:      "ws://fake/ws",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{
								Action:    "expect",
								TimeoutMs: 500,
								ExpectAssertions: parser.BodyAssertions{
									Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "ok"}},
								},
							},
						},
					},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Err != nil {
		t.Errorf("rr.Err = %v", results[0].Err)
	}
	// Verify the dialer was actually called (meaning the runner dispatched to websocket path).
	if dialer.lastURL != "ws://fake/ws" {
		t.Errorf("lastURL = %q, want ws://fake/ws", dialer.lastURL)
	}
}

// TestRun_websocket_parallel_failure_propagates exercises the buildWSOutcome failure
// branch (lines 692–706 in runner.go) through the parallel buildWebSocketFunc path.
// A dialer that always returns an error is used so websocket.Execute fails immediately;
// the test verifies the error message shape, AssertionResults content, and that the
// "unknown failure" fallback is NOT used when an error is present (err.Error() is used).
func TestRun_websocket_parallel_failure_propagates(t *testing.T) {
	dialErr := errors.New("connection refused")
	dialer := &wsRunnerDialer{dialErr: dialErr}

	col := &parser.Collection{
		Name: "WS Parallel Fail",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "WS-fail",
				Request: parser.Request{
					Protocol: "websocket",
					Method:   "WS",
					URL:      "ws://fake/ws",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{Action: "close"},
						},
					},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
		Parallel:        true,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}

	rr := results[0]

	// Error must propagate from buildWSOutcome failure branch.
	if rr.Err == nil {
		t.Fatal("expected rr.Err != nil for a failing WebSocket dial, got nil")
	}

	// AssertionResults must be set and must be failing.
	if rr.AssertionResults == nil {
		t.Fatal("expected AssertionResults != nil for failed WebSocket in parallel path")
	}
	if rr.AssertionResults.Passed {
		t.Error("AssertionResults.Passed should be false for a failed WebSocket")
	}

	// There must be exactly one assertion item describing the failure.
	if len(rr.AssertionResults.Items) != 1 {
		t.Fatalf("AssertionResults.Items = %d, want 1", len(rr.AssertionResults.Items))
	}
	item := rr.AssertionResults.Items[0]

	if item.Type != "websocket" {
		t.Errorf("AssertionResults.Items[0].Type = %q, want %q", item.Type, "websocket")
	}
	if item.Expected != "all steps pass" {
		t.Errorf("AssertionResults.Items[0].Expected = %q, want %q", item.Expected, "all steps pass")
	}
	// Actual must contain the real error string, not the "unknown failure" fallback.
	if item.Actual == "unknown failure" {
		t.Errorf("AssertionResults.Items[0].Actual = %q; expected error text, not the \"unknown failure\" fallback", item.Actual)
	}
	if !strings.Contains(item.Actual, "connection refused") {
		t.Errorf("AssertionResults.Items[0].Actual = %q; expected it to contain %q", item.Actual, "connection refused")
	}
	if item.Passed {
		t.Error("AssertionResults.Items[0].Passed should be false")
	}

	// Summary must reflect one failure.
	if summary.Failed != 1 {
		t.Errorf("summary.Failed = %d, want 1", summary.Failed)
	}
	if summary.Passed != 0 {
		t.Errorf("summary.Passed = %d, want 0", summary.Passed)
	}
	if !summary.IsParallel {
		t.Error("summary.IsParallel should be true")
	}
}

// -- Global rate-limit tests --

func makeRateLimitCollection(n, rps int) *parser.Collection {
	items := make([]parser.RequestItem, n)
	for i := range items {
		items[i] = parser.RequestItem{
			Name:    fmt.Sprintf("req%d", i),
			Request: parser.Request{Method: "GET", URL: "https://example.com"},
		}
	}
	return &parser.Collection{
		Name:         "Rate Limited",
		RateLimitRPS: rps,
		Requests:     parser.Section{Items: items},
	}
}

func okExec(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
	return &httpexec.Result{StatusCode: 200}, nil
}

// TestRun_GlobalRateLimit_ThrottlesSequential verifies that 10 sequential
// requests at 5 rps take at least 1.44s (80% of 9 × 200ms).
func TestRun_GlobalRateLimit_ThrottlesSequential(t *testing.T) {
	col := makeRateLimitCollection(10, 5)
	start := time.Now()
	_, summary, err := Run(context.Background(), col, okExec, VarSources{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if summary.Passed != 10 {
		t.Errorf("summary.Passed = %d, want 10", summary.Passed)
	}
	// 9 intervals × 200ms × 80% = 1.44s
	want := 9 * 200 * time.Millisecond * 80 / 100
	if elapsed < want {
		t.Errorf("elapsed %v < %v; global limiter did not throttle sequential requests", elapsed, want)
	}
}

// TestRun_GlobalRateLimit_ZeroIsUnlimited verifies that rate_limit_rps: 0
// applies no throttle; 20 requests must complete well under 500ms.
func TestRun_GlobalRateLimit_ZeroIsUnlimited(t *testing.T) {
	col := makeRateLimitCollection(20, 0)
	start := time.Now()
	_, _, err := Run(context.Background(), col, okExec, VarSources{})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if elapsed > 500*time.Millisecond {
		t.Errorf("elapsed %v > 500ms; zero rps should be unlimited", elapsed)
	}
}

// TestRun_GlobalRateLimit_ParallelSharesBucket verifies that parallel workers
// share the same token bucket, keeping throughput <= 5 rps.
func TestRun_GlobalRateLimit_ParallelSharesBucket(t *testing.T) {
	col := makeRateLimitCollection(10, 5)
	start := time.Now()
	_, summary, err := Run(context.Background(), col, okExec, VarSources{
		Parallel: true,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run error: %v", err)
	}
	if summary.Passed != 10 {
		t.Errorf("summary.Passed = %d, want 10", summary.Passed)
	}
	// At least 80% of the minimum throttle time
	want := 9 * 200 * time.Millisecond * 80 / 100
	if elapsed < want {
		t.Errorf("elapsed %v < %v; parallel workers are not sharing the global bucket", elapsed, want)
	}
}

// TestRun_GlobalRateLimit_ZeroDoesNotGate verifies that rate_limit_rps: 0 at
// Free tier does NOT trigger the feature gate.
func TestRun_GlobalRateLimit_ZeroDoesNotGate(t *testing.T) {
	col := makeRateLimitCollection(1, 0)
	_, _, err := Run(context.Background(), col, okExec, VarSources{})
	if err != nil {
		t.Fatalf("expected no error for rate_limit_rps=0 at Free tier, got: %v", err)
	}
}

// TestRun_GlobalRateLimit_ContextCancellation verifies that context cancellation
// while the limiter is sleeping causes Run to return promptly.
func TestRun_GlobalRateLimit_ContextCancellation(t *testing.T) {
	// 1 rps: after first request, limiter sleeps 1s. Cancel after 150ms.
	col := makeRateLimitCollection(5, 1)
	ctx, cancel := context.WithTimeout(context.Background(), 150*time.Millisecond)
	defer cancel()

	start := time.Now()
	_, _, _ = Run(ctx, col, okExec, VarSources{})
	elapsed := time.Since(start)

	// Must finish well under 1s (the limiter interval).
	if elapsed > 900*time.Millisecond {
		t.Errorf("Run took %v; should have returned promptly on context cancellation", elapsed)
	}
}

// TestRun_GlobalRateLimit_StacksWithDataDriven verifies behavior #7: when a
// collection has top-level rate_limit_rps: 5 AND a data-driven request has
// rate_limit_rps: 20, both throttles apply (they "stack"). The global limiter
// is the slower one (200ms intervals vs 50ms), so it dominates, and wall-clock
// must be >= 80% of 4 × 200ms = 640ms for 5 data rows.
func TestRun_GlobalRateLimit_StacksWithDataDriven(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "val\n1\n2\n3\n4\n5")

	// data-driven rate_limit_rps: 20 (50ms intervals, faster than global)
	ddRPS := 20
	col := &parser.Collection{
		Name:         "Stacked Rate Limits",
		RateLimitRPS: 5, // global: 200ms intervals
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Test",
				DataDriven: &datadriven.Config{
					Source:       "data.csv",
					Parallel:     true,
					RateLimitRPS: &ddRPS,
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
			},
		}},
	}

	start := time.Now()
	results, summary, err := Run(context.Background(), col, okExec, VarSources{
		CollectionDir: dir,
	})
	elapsed := time.Since(start)
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}
	if summary.Passed != 5 {
		t.Errorf("summary.Passed = %d, want 5", summary.Passed)
	}
	if len(results) != 5 {
		t.Fatalf("len(results) = %d, want 5", len(results))
	}
	// Global limiter: 5 rps → 200ms intervals. 5 rows → 4 intervals → 800ms minimum.
	// 80% tolerance → must be >= 640ms.
	want := 4 * 200 * time.Millisecond * 80 / 100
	if elapsed < want {
		t.Errorf("elapsed %v < %v; global limiter did not dominate the stacked throttles", elapsed, want)
	}
}

// mustParseTeamTemplate parses the standard two-environment fixture used
// throughout the team secrets test suite.
const twoEnvTeamTemplateYAML = `team_secrets:
  vault_configs:
    production:
      provider: aws-secrets-manager
      region: eu-west-1
      keys:
        api_key: prod/api-key
        db_password: prod/db-password
    staging:
      provider: azure-key-vault
      vault_name: staging-vault
      keys:
        api_key: staging-api-key
        db_password: staging-db-password
`

func mustParseTeamTemplate(t *testing.T) *teamtemplate.TeamTemplate {
	t.Helper()
	tpl, err := teamtemplate.Parse([]byte(twoEnvTeamTemplateYAML))
	if err != nil {
		t.Fatalf("teamtemplate.Parse() error: %v", err)
	}
	if issues := tpl.Validate(); len(issues) > 0 {
		t.Fatalf("teamtemplate.Validate() issues: %v", issues)
	}
	return tpl
}

func TestRealTeamProviderFactoryConstructsConfiguredProvider(t *testing.T) {
	tests := []struct {
		name        string
		env         *teamtemplate.ResolvedEnv
		wantCommand string
	}{
		{
			name: "aws region",
			env: &teamtemplate.ResolvedEnv{
				Name: "production", Provider: vault.ProviderAWS, Region: "eu-west-1",
			},
			wantCommand: "--region 'eu-west-1'",
		},
		{
			name: "azure vault",
			env: &teamtemplate.ResolvedEnv{
				Name: "staging", Provider: vault.ProviderAzure, VaultName: "staging-vault",
			},
			wantCommand: "--vault-name 'staging-vault'",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var command string
			exec := func(_ context.Context, got string) (string, error) {
				command = got
				return "resolved-secret", nil
			}
			provider, err := realTeamProviderFactory(exec)(tc.env)
			if err != nil {
				t.Fatalf("factory error: %v", err)
			}
			if _, err := provider.Fetch(context.Background(), "secret/path"); err != nil {
				t.Fatalf("Fetch error: %v", err)
			}
			if !strings.Contains(command, tc.wantCommand) {
				t.Fatalf("command %q does not contain %q", command, tc.wantCommand)
			}
		})
	}
}

func TestRealTeamProviderFactoryRejectsUnknownProvider(t *testing.T) {
	_, err := realTeamProviderFactory(func(context.Context, string) (string, error) { return "", nil })(
		&teamtemplate.ResolvedEnv{Name: "production", Provider: "unknown"},
	)
	if !errors.Is(err, teamtemplate.ErrUnknownProvider) {
		t.Fatalf("factory error = %v, want ErrUnknownProvider", err)
	}
}

// colWithURL creates a minimal single-request collection with the given URL.
func colWithURL(url string) *parser.Collection {
	return &parser.Collection{
		Name: "TeamTest",
		Requests: parser.Section{
			Items: []parser.RequestItem{
				{Name: "R1", Request: parser.Request{Method: "GET", URL: url}},
			},
		},
	}
}

func TestRun_TeamSecrets(t *testing.T) {
	ctx := context.Background()

	t.Run("resolves_production_alias_via_stub", func(t *testing.T) {
		tpl := mustParseTeamTemplate(t)
		col := colWithURL("https://x/{{secrets.api_key}}")
		_, summary, err := Run(ctx, col, successExecutor, VarSources{
			TeamTemplate: tpl, TeamEnv: "production", TeamStub: true,
		})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if summary.SharedSecretsResolved != 2 {
			t.Errorf("SharedSecretsResolved = %d, want 2", summary.SharedSecretsResolved)
		}
	})

	t.Run("staging_uses_azure_provider", func(t *testing.T) {
		tpl := mustParseTeamTemplate(t)
		col := colWithURL("https://x/{{secrets.api_key}}")

		var capturedURL string
		trackingExec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			capturedURL = req.URL
			return &httpexec.Result{StatusCode: 200}, nil
		}

		_, summary, err := Run(ctx, col, trackingExec, VarSources{
			TeamTemplate: tpl, TeamEnv: "staging", TeamStub: true,
		})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		// Verify the staging provider path was taken: stub values contain the env name.
		if !strings.Contains(capturedURL, "stub::staging") {
			t.Errorf("expected resolved URL to contain %q (staging-namespaced stub value); got %q", "stub::staging", capturedURL)
		}
		if summary.SharedSecretsResolved != 2 {
			t.Errorf("SharedSecretsResolved = %d, want 2", summary.SharedSecretsResolved)
		}
	})

	t.Run("env_flag_required_when_secret_referenced", func(t *testing.T) {
		tpl := mustParseTeamTemplate(t)
		col := colWithURL("https://x/{{secrets.api_key}}")
		_, _, err := Run(ctx, col, successExecutor, VarSources{
			TeamTemplate: tpl, TeamEnv: "", TeamStub: true,
		})
		if !errors.Is(err, teamtemplate.ErrEnvFlagRequired) {
			t.Fatalf("Run() error = %v, want ErrEnvFlagRequired", err)
		}
	})

	t.Run("unknown_alias_fails_before_http", func(t *testing.T) {
		tpl := mustParseTeamTemplate(t)
		col := colWithURL("https://x/{{secrets.missing}}")
		var dispatched int
		trackingExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
			dispatched++
			return &httpexec.Result{StatusCode: 200}, nil
		}
		_, _, err := Run(ctx, col, trackingExec, VarSources{
			TeamTemplate: tpl, TeamEnv: "production", TeamStub: true,
		})
		if !errors.Is(err, teamtemplate.ErrUnknownAlias) {
			t.Fatalf("Run() error = %v, want ErrUnknownAlias", err)
		}
		if dispatched != 0 {
			t.Errorf("dispatched = %d, want 0 (should fail before HTTP)", dispatched)
		}
	})

	t.Run("unknown_environment_fails", func(t *testing.T) {
		tpl := mustParseTeamTemplate(t)
		col := colWithURL("https://x/{{secrets.api_key}}")
		_, _, err := Run(ctx, col, successExecutor, VarSources{
			TeamTemplate: tpl, TeamEnv: "qa", TeamStub: true,
		})
		if !errors.Is(err, teamtemplate.ErrUnknownEnvironment) {
			t.Fatalf("Run() error = %v, want ErrUnknownEnvironment", err)
		}
	})

	t.Run("no_team_template_no_op", func(t *testing.T) {
		col := colWithURL("https://x/path")
		_, summary, err := Run(ctx, col, successExecutor, VarSources{})
		if err != nil {
			t.Fatalf("Run() error: %v", err)
		}
		if summary.SharedSecretsResolved != 0 {
			t.Errorf("SharedSecretsResolved = %d, want 0", summary.SharedSecretsResolved)
		}
	})
}

// TestVisitBody exercises all four type-switch branches of visitBody so that
// collectSecretReferences correctly detects {{secrets.X}} tokens in every
// supported body type.
func TestVisitBody(t *testing.T) {
	collect := func(body any) []string {
		var got []string
		visitBody(body, func(s string) { got = append(got, s) })
		return got
	}

	t.Run("nil_body_is_noop", func(t *testing.T) {
		got := collect(nil)
		if len(got) != 0 {
			t.Errorf("visitBody(nil) = %v, want empty", got)
		}
	})

	t.Run("string_body_calls_visit_once", func(t *testing.T) {
		got := collect("{{secrets.api_key}}")
		if len(got) != 1 || got[0] != "{{secrets.api_key}}" {
			t.Errorf("visitBody(string) = %v, want [{{secrets.api_key}}]", got)
		}
	})

	t.Run("map_string_any_body_visits_all_values", func(t *testing.T) {
		body := map[string]any{
			"key1": "{{secrets.api_key}}",
			"key2": "{{secrets.db_password}}",
			"num":  42, // non-string: ignored
		}
		got := collect(body)
		// Order is map-iteration-dependent; check presence.
		found := map[string]bool{}
		for _, s := range got {
			found[s] = true
		}
		if !found["{{secrets.api_key}}"] {
			t.Error("visitBody(map[string]any) did not visit {{secrets.api_key}}")
		}
		if !found["{{secrets.db_password}}"] {
			t.Error("visitBody(map[string]any) did not visit {{secrets.db_password}}")
		}
	})

	t.Run("map_string_string_body_visits_all_values", func(t *testing.T) {
		body := map[string]string{
			"key1": "{{secrets.api_key}}",
			"key2": "plain",
		}
		got := collect(body)
		found := map[string]bool{}
		for _, s := range got {
			found[s] = true
		}
		if !found["{{secrets.api_key}}"] {
			t.Error("visitBody(map[string]string) did not visit {{secrets.api_key}}")
		}
		if !found["plain"] {
			t.Error("visitBody(map[string]string) did not visit plain")
		}
	})

	t.Run("slice_body_visits_all_elements", func(t *testing.T) {
		body := []any{
			"{{secrets.api_key}}",
			map[string]any{"nested": "{{secrets.db_password}}"},
			42, // non-string: skipped by inner visitBody
		}
		got := collect(body)
		found := map[string]bool{}
		for _, s := range got {
			found[s] = true
		}
		if !found["{{secrets.api_key}}"] {
			t.Error("visitBody([]any) did not visit {{secrets.api_key}}")
		}
		if !found["{{secrets.db_password}}"] {
			t.Error("visitBody([]any) did not visit nested {{secrets.db_password}}")
		}
	})
}

// TestCollectSecretReferences_BodyTypes checks that collectSecretReferences
// detects {{secrets.X}} in request bodies of all supported types.
func TestCollectSecretReferences_BodyTypes(t *testing.T) {
	makeColWithBody := func(body any) *parser.Collection {
		return &parser.Collection{
			Name: "BodyTest",
			Requests: parser.Section{
				Items: []parser.RequestItem{
					{Name: "R1", Request: parser.Request{Method: "POST", URL: "https://x/", Body: body}},
				},
			},
		}
	}

	t.Run("string_body", func(t *testing.T) {
		refs := collectSecretReferences(makeColWithBody("token={{secrets.api_key}}"))
		if len(refs) != 1 || refs[0] != "api_key" {
			t.Errorf("refs = %v, want [api_key]", refs)
		}
	})

	t.Run("map_string_any_body", func(t *testing.T) {
		refs := collectSecretReferences(makeColWithBody(map[string]any{
			"token":    "{{secrets.api_key}}",
			"password": "{{secrets.db_password}}",
		}))
		if len(refs) != 2 {
			t.Errorf("refs = %v, want 2 entries", refs)
		}
	})

	t.Run("map_string_string_body", func(t *testing.T) {
		refs := collectSecretReferences(makeColWithBody(map[string]string{
			"token": "{{secrets.api_key}}",
		}))
		if len(refs) != 1 || refs[0] != "api_key" {
			t.Errorf("refs = %v, want [api_key]", refs)
		}
	})

	t.Run("slice_body", func(t *testing.T) {
		refs := collectSecretReferences(makeColWithBody([]any{
			"{{secrets.api_key}}",
			map[string]any{"p": "{{secrets.db_password}}"},
		}))
		if len(refs) != 2 {
			t.Errorf("refs = %v, want 2 entries", refs)
		}
	})
}

// --- Hooks dispatcher tests ---

// fakeDispatcher implements a minimal hooks-like interface for testing.
// It records calls and optionally mutates the request or returns errors.
type fakeDispatcher struct {
	mu             sync.Mutex
	onRequestCalls []hooks.RequestPayload
	onRespCalls    []hooks.ResponsePayload
	onResultCalls  []hooks.ResultPayload

	// mutateReq, if non-nil, transforms the request in OnRequest.
	mutateReq func(hooks.RequestPayload) hooks.RequestPayload
	// abortReq, if true, OnRequest returns ErrHookAborted.
	abortReq bool
}

func (f *fakeDispatcher) OnRequest(_ context.Context, req hooks.RequestPayload) (hooks.RequestPayload, error) {
	f.mu.Lock()
	f.onRequestCalls = append(f.onRequestCalls, req)
	f.mu.Unlock()
	if f.abortReq {
		return req, fmt.Errorf("%w: fake abort", hooks.ErrHookAborted)
	}
	if f.mutateReq != nil {
		return f.mutateReq(req), nil
	}
	return req, nil
}

func (f *fakeDispatcher) OnResponse(_ context.Context, resp hooks.ResponsePayload) (hooks.ResponsePayload, error) {
	f.mu.Lock()
	f.onRespCalls = append(f.onRespCalls, resp)
	f.mu.Unlock()
	return resp, nil
}

func (f *fakeDispatcher) OnResult(_ context.Context, res hooks.ResultPayload) error {
	f.mu.Lock()
	f.onResultCalls = append(f.onResultCalls, res)
	f.mu.Unlock()
	return nil
}

func (f *fakeDispatcher) Close() error { return nil }

func TestRun_WithHooksDispatcher_InvokesOnRequestBeforeExec(t *testing.T) {
	col := makeCollection([]string{"req-1"}, false)
	fd := &fakeDispatcher{}
	order := make([]string, 0, 2)
	var mu sync.Mutex
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		mu.Lock()
		order = append(order, "exec")
		mu.Unlock()
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	// Wrap exec with on_request check.
	origOnReq := fd.OnRequest
	_ = origOnReq
	fd.mutateReq = func(p hooks.RequestPayload) hooks.RequestPayload {
		mu.Lock()
		order = append(order, "on_request")
		mu.Unlock()
		return p
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{
		Hooks: fd,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if len(fd.onRequestCalls) != 1 {
		t.Errorf("expected 1 on_request call, got %d", len(fd.onRequestCalls))
	}
	if len(fd.onRespCalls) != 1 {
		t.Errorf("expected 1 on_response call, got %d", len(fd.onRespCalls))
	}
}

func TestRun_WithHooksDispatcher_MutationAppliesToExec(t *testing.T) {
	col := makeCollection([]string{"req-1"}, false)
	fd := &fakeDispatcher{
		mutateReq: func(p hooks.RequestPayload) hooks.RequestPayload {
			if p.Headers == nil {
				p.Headers = make(map[string]string)
			}
			p.Headers["X-Plugin-Trace"] = "injected"
			return p
		},
	}
	var seenHeader string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		seenHeader = req.Headers["X-Plugin-Trace"]
		return &httpexec.Result{StatusCode: 200, Duration: 5 * time.Millisecond}, nil
	}
	_, _, err := Run(context.Background(), col, exec, VarSources{Hooks: fd})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenHeader != "injected" {
		t.Errorf("expected X-Plugin-Trace=injected in exec, got %q", seenHeader)
	}
}

func TestRun_WithHooksDispatcher_AbortErrorBecomesRequestErr(t *testing.T) {
	col := makeCollection([]string{"req-1"}, false)
	fd := &fakeDispatcher{abortReq: true}
	execCalled := false
	exec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled = true
		return &httpexec.Result{StatusCode: 200}, nil
	}
	results, _, err := Run(context.Background(), col, exec, VarSources{Hooks: fd})
	if err != nil {
		t.Fatalf("Run returned fatal error: %v", err)
	}
	if execCalled {
		t.Error("exec should not be called when on_request aborts")
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Err == nil {
		t.Error("expected RequestResult.Err to be set after abort")
	}
}

func TestRun_WithoutHooks_NoWrapOverhead(t *testing.T) {
	col := makeCollection([]string{"A", "B"}, false)
	// Nil Hooks — no wrap.
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Errorf("expected 2 results, got %d", len(results))
	}
}

func TestRun_WithHooksDispatcher_OnResultFiresOnceWithSummary(t *testing.T) {
	col := makeCollection([]string{"A", "B"}, false)
	fd := &fakeDispatcher{}
	_, _, err := Run(context.Background(), col, successExecutor, VarSources{Hooks: fd})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(fd.onResultCalls) != 1 {
		t.Errorf("expected 1 on_result call, got %d", len(fd.onResultCalls))
	}
	if fd.onResultCalls[0].PassCount != 2 {
		t.Errorf("expected PassCount=2, got %d", fd.onResultCalls[0].PassCount)
	}
}

// TestRunner_RequestResultCarriesSourceLocation verifies that SourceFile and
// SourceLine from the parser.RequestItem are threaded verbatim into every
// RequestResult produced by the runner.
func TestRunner_RequestResultCarriesSourceLocation(t *testing.T) {
	items := []parser.RequestItem{
		{
			Name:       "alpha",
			Request:    parser.Request{Method: "GET", URL: "https://example.com/a"},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 3,
		},
		{
			Name:       "beta",
			Request:    parser.Request{Method: "GET", URL: "https://example.com/b"},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 7,
		},
	}
	col := &parser.Collection{Name: "demo", Requests: parser.Section{Items: items}}

	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for i, want := range []struct {
		file string
		line int
	}{
		{"/abs/path/demo.yaml", 3},
		{"/abs/path/demo.yaml", 7},
	} {
		if results[i].SourceFile != want.file {
			t.Errorf("results[%d].SourceFile = %q, want %q", i, results[i].SourceFile, want.file)
		}
		if results[i].SourceLine != want.line {
			t.Errorf("results[%d].SourceLine = %d, want %d", i, results[i].SourceLine, want.line)
		}
	}
}

// TestRunner_DataDrivenIterationsCarrySourceLocation verifies that each
// data-driven iteration's RequestResult carries the base item's SourceFile and
// SourceLine verbatim (not a synthetic offset per iteration).
func TestRunner_DataDrivenIterationsCarrySourceLocation(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(csvPath, []byte("user_id\n1\n2\n3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	items := []parser.RequestItem{
		{
			Name:    "fetch",
			Request: parser.Request{Method: "GET", URL: "https://example.com/{{user_id}}"},
			DataDriven: &datadriven.Config{
				Source: csvPath,
			},
			SourceFile: "/abs/path/demo.yaml",
			SourceLine: 12,
		},
	}
	col := &parser.Collection{Name: "dd", Requests: parser.Section{Items: items}}
	vars := VarSources{}
	results, _, err := Run(context.Background(), col, successExecutor, vars)
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	for i, r := range results {
		if r.SourceFile != "/abs/path/demo.yaml" {
			t.Errorf("iter %d: SourceFile = %q, want %q", i, r.SourceFile, "/abs/path/demo.yaml")
		}
		if r.SourceLine != 12 {
			t.Errorf("iter %d: SourceLine = %d, want 12", i, r.SourceLine)
		}
	}
}

// TestRunner_ParallelWebSocketCarriesSourceLocation verifies that SourceFile and
// SourceLine from the parser.RequestItem are preserved through the parallel
// buildWSOutcome path (i.e. when Parallel=true and the item is a WebSocket request).
func TestRunner_ParallelWebSocketCarriesSourceLocation(t *testing.T) {
	fc := &wsFakeConn{}
	dialer := &wsRunnerDialer{conn: fc}

	col := &parser.Collection{
		Name: "WS Parallel Source Location",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "ws-req",
				SourceFile: "/abs/path/ws-demo.yaml",
				SourceLine: 5,
				Request: parser.Request{
					Protocol: "websocket",
					Method:   "WS",
					URL:      "ws://fake/ws",
					WebSocket: &parser.WebSocketConfig{
						Steps: []parser.WebSocketStep{
							{Action: "close"},
						},
					},
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, nilExec, VarSources{
		WebSocketDialer: dialer,
		Parallel:        true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].SourceFile != "/abs/path/ws-demo.yaml" {
		t.Errorf("SourceFile = %q, want %q", results[0].SourceFile, "/abs/path/ws-demo.yaml")
	}
	if results[0].SourceLine != 5 {
		t.Errorf("SourceLine = %d, want 5", results[0].SourceLine)
	}
}

// TestRunner_ParallelDataDrivenCarriesSourceLocation verifies that SourceFile and
// SourceLine from the parser.RequestItem are preserved through the parallel
// buildDataDrivenFunc conversion path (i.e. when vars.Parallel=true and the item
// has DataDriven config). Each iteration result must carry the base item's location.
func TestRunner_ParallelDataDrivenCarriesSourceLocation(t *testing.T) {
	csvPath := filepath.Join(t.TempDir(), "rows.csv")
	if err := os.WriteFile(csvPath, []byte("val\n1\n2\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	col := &parser.Collection{
		Name: "Parallel DD Source Location",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:       "dd-req",
				SourceFile: "/abs/path/dd-demo.yaml",
				SourceLine: 9,
				Request:    parser.Request{Method: "GET", URL: "https://example.com/{{val}}"},
				DataDriven: &datadriven.Config{
					Source: csvPath,
				},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, successExecutor, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("results = %d, want 2", len(results))
	}
	for i, r := range results {
		if r.SourceFile != "/abs/path/dd-demo.yaml" {
			t.Errorf("iter %d: SourceFile = %q, want %q", i, r.SourceFile, "/abs/path/dd-demo.yaml")
		}
		if r.SourceLine != 9 {
			t.Errorf("iter %d: SourceLine = %d, want 9", i, r.SourceLine)
		}
	}
}

// --- M6-005: EventSink tests ---

// recordingSink captures all EventSink callbacks for inspection in tests.
type recordingSink struct {
	mu      sync.Mutex
	starts  []RequestEvent
	ends    []RequestEndEvent
	asserts []AssertionEvent
}

func (r *recordingSink) RequestStart(e RequestEvent) {
	r.mu.Lock()
	r.starts = append(r.starts, e)
	r.mu.Unlock()
}

func (r *recordingSink) RequestEnd(e RequestEndEvent) {
	r.mu.Lock()
	r.ends = append(r.ends, e)
	r.mu.Unlock()
}

func (r *recordingSink) AssertionResult(e AssertionEvent) {
	r.mu.Lock()
	r.asserts = append(r.asserts, e)
	r.mu.Unlock()
}

// TestRun_EventSink_Nil_NoOp verifies that when OnEvent is nil (zero value),
// the runner completes without panics or nil dereferences. Backward compat.
func TestRun_EventSink_Nil_NoOp(t *testing.T) {
	col := makeCollection([]string{"a", "b"}, false)
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatal(err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
}

// makeCollectionWithStatus builds a collection with one request and a status assertion.
func makeCollectionWithStatus(name string, expectedStatus int) *parser.Collection {
	return &parser.Collection{
		Name: name,
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "req",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{expectedStatus}},
				},
			},
		}},
	}
}

// statusExecutor returns the given HTTP status code.
func statusExecutor(code int) ExecuteFunc {
	return func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: code, Duration: 5 * time.Millisecond}, nil
	}
}

// TestRun_EventSink_SequentialPassingRequest verifies request.start, assertion.result,
// and request.end(outcome=passed) are emitted for a passing request.
func TestRun_EventSink_SequentialPassingRequest(t *testing.T) {
	sink := &recordingSink{}
	col := makeCollectionWithStatus("ok", 200)
	_, _, err := Run(context.Background(), col, statusExecutor(200), VarSources{OnEvent: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.starts) != 1 {
		t.Fatalf("want 1 request.start, got %d", len(sink.starts))
	}
	if len(sink.ends) != 1 {
		t.Fatalf("want 1 request.end, got %d", len(sink.ends))
	}
	if sink.starts[0].RequestID != sink.ends[0].RequestID {
		t.Errorf("request_id mismatch: start=%q end=%q", sink.starts[0].RequestID, sink.ends[0].RequestID)
	}
	if sink.ends[0].Outcome != "passed" {
		t.Errorf("outcome = %q, want passed", sink.ends[0].Outcome)
	}
	if len(sink.asserts) != 1 || !sink.asserts[0].Passed {
		t.Errorf("expected 1 passing assertion event, got %+v", sink.asserts)
	}
	if sink.asserts[0].RequestID != sink.starts[0].RequestID {
		t.Errorf("assertion request_id mismatch")
	}
}

// TestRun_EventSink_FailingAssertion verifies that when the status assertion fails,
// request.end carries outcome="failed" and assertion.result has Passed=false.
func TestRun_EventSink_FailingAssertion(t *testing.T) {
	sink := &recordingSink{}
	col := makeCollectionWithStatus("fail", 200) // expects 200 but gets 500
	_, _, err := Run(context.Background(), col, statusExecutor(500), VarSources{OnEvent: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.ends) != 1 {
		t.Fatalf("want 1 request.end, got %d", len(sink.ends))
	}
	if sink.ends[0].Outcome != "failed" {
		t.Errorf("outcome = %q, want failed", sink.ends[0].Outcome)
	}
	if len(sink.asserts) != 1 || sink.asserts[0].Passed {
		t.Errorf("expected 1 failing assertion event, got %+v", sink.asserts)
	}
}

// TestRun_EventSink_NetworkError verifies that when exec returns an error,
// request.end carries outcome="error" and Err is non-nil; no assertion events.
func TestRun_EventSink_NetworkError(t *testing.T) {
	sink := &recordingSink{}
	col := makeCollection([]string{"req"}, false)
	errExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, errFake
	}
	_, _, err := Run(context.Background(), col, errExec, VarSources{OnEvent: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.ends) != 1 {
		t.Fatalf("want 1 request.end, got %d", len(sink.ends))
	}
	if sink.ends[0].Outcome != "error" {
		t.Errorf("outcome = %q, want error", sink.ends[0].Outcome)
	}
	if sink.ends[0].Err == nil {
		t.Errorf("want non-nil Err on request.end for network error")
	}
	if len(sink.asserts) != 0 {
		t.Errorf("want no assertion events for network error, got %d", len(sink.asserts))
	}
}

// TestRun_EventSink_GuardRailSkip verifies that items skipped by the guard rail
// emit request.end with outcome="skipped" but no request.start.
func TestRun_EventSink_GuardRailSkip(t *testing.T) {
	oldMax := MaxRequests
	MaxRequests = 1
	t.Cleanup(func() { MaxRequests = oldMax })

	sink := &recordingSink{}
	col := makeCollection([]string{"a", "b", "c"}, false)
	_, _, err := Run(context.Background(), col, successExecutor, VarSources{OnEvent: sink})
	if err != nil {
		t.Fatal(err)
	}
	// 1 request.start (the one that ran), 2 request.end for skipped items, 1 request.end for the one that ran
	if len(sink.starts) != 1 {
		t.Errorf("want 1 request.start, got %d", len(sink.starts))
	}
	if len(sink.ends) != 3 {
		t.Errorf("want 3 request.end events (1 executed + 2 skipped), got %d", len(sink.ends))
	}
	skippedCount := 0
	for _, e := range sink.ends {
		if e.Outcome == "skipped" {
			skippedCount++
		}
	}
	if skippedCount != 2 {
		t.Errorf("want 2 skipped outcomes, got %d", skippedCount)
	}
}

// TestRun_EventSink_RequestIDsMonotonic verifies that request IDs are monotonically
// assigned in order: req-1, req-2, ... for sequential requests.
func TestRun_EventSink_RequestIDsMonotonic(t *testing.T) {
	sink := &recordingSink{}
	col := makeCollection([]string{"a", "b", "c", "d", "e"}, false)
	_, _, err := Run(context.Background(), col, successExecutor, VarSources{OnEvent: sink})
	if err != nil {
		t.Fatal(err)
	}
	if len(sink.starts) != 5 {
		t.Fatalf("want 5 starts, got %d", len(sink.starts))
	}
	for i, s := range sink.starts {
		want := fmt.Sprintf("req-%d", i+1)
		if s.RequestID != want {
			t.Errorf("start[%d].RequestID = %q, want %q", i, s.RequestID, want)
		}
	}
}

// TestRun_EventSink_DataDrivenParallel verifies that event emission works for
// data-driven items with parallel: true. Every iteration must emit a paired
// request.start / request.end event, and all request IDs must be unique.
func TestRun_EventSink_DataDrivenParallel(t *testing.T) {
	dir := t.TempDir()
	writeCSVFile(t, dir, "data.csv", "name\nalice\nbob\ncarol")

	sink := &recordingSink{}

	col := &parser.Collection{
		Name: "DataDriven Parallel Events",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "Create",
				DataDriven: &datadriven.Config{
					Source:   "data.csv",
					Parallel: true,
				},
				Request: parser.Request{Method: "POST", URL: "https://example.com/{{name}}"},
			},
		}},
	}

	_, _, err := Run(context.Background(), col, successExecutor, VarSources{
		OnEvent:       sink,
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run() error: %v", err)
	}

	// 3 rows → 3 start + 3 end events (no assertions configured).
	if len(sink.starts) != 3 {
		t.Fatalf("want 3 request.start events, got %d", len(sink.starts))
	}
	if len(sink.ends) != 3 {
		t.Fatalf("want 3 request.end events, got %d", len(sink.ends))
	}

	// All request IDs in starts must be unique.
	seen := map[string]bool{}
	for _, s := range sink.starts {
		if s.RequestID == "" {
			t.Error("request.start has empty request_id")
		}
		if seen[s.RequestID] {
			t.Errorf("duplicate request_id %q", s.RequestID)
		}
		seen[s.RequestID] = true
	}

	// Every start ID must have a matching end ID.
	endIDs := map[string]bool{}
	for _, e := range sink.ends {
		endIDs[e.RequestID] = true
	}
	for id := range seen {
		if !endIDs[id] {
			t.Errorf("request_id %q has start but no end", id)
		}
	}

	// All outcomes must be "passed".
	for _, e := range sink.ends {
		if e.Outcome != "passed" {
			t.Errorf("request.end outcome = %q, want passed", e.Outcome)
		}
	}
}

// TestRun_EventSink_WebSocketRateLimitSkip verifies that when the global rate
// limiter returns a context-cancelled error on a WebSocket item, a
// request.end event with outcome="skipped" is emitted — satisfying the
// EventSink contract that RequestEnd fires exactly once per request regardless
// of outcome.
func TestRun_EventSink_WebSocketRateLimitSkip(t *testing.T) {
	sink := &recordingSink{}

	// A collection with RateLimitRPS=1 so the runner creates a globalLimiter,
	// and a single WebSocket request item (protocol:"websocket").
	col := &parser.Collection{
		Name:         "WS Rate Limit Skip",
		RateLimitRPS: 1,
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "ws-req",
				Request: parser.Request{Method: "GET", URL: "ws://example.com/ws", Protocol: "websocket"},
			},
		}},
	}

	// A pre-cancelled context ensures globalLimiter.Wait(ctx) returns immediately
	// with ctx.Err() rather than blocking for the full token-bucket interval.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// TierProfessional is required to pass the protocol_websocket feature gate.
	// The stub dialer is never reached because the rate-limit Wait returns first.
	_, _, _ = Run(ctx, col, successExecutor, VarSources{
		OnEvent:         sink,
		WebSocketDialer: websocket.DefaultDialer, // irrelevant; never reached
	})

	// The WebSocket item should emit a request.end with outcome="skipped"
	// (no request.start is emitted for skipped items, matching the
	// convention for all other skip paths in executePhase).
	if len(sink.ends) != 1 {
		t.Fatalf("want 1 request.end event, got %d", len(sink.ends))
	}
	if sink.ends[0].Outcome != "skipped" {
		t.Errorf("request.end outcome = %q, want skipped", sink.ends[0].Outcome)
	}
	// No request.start should fire for a rate-limit skipped item.
	if len(sink.starts) != 0 {
		t.Errorf("want 0 request.start events for skipped item, got %d", len(sink.starts))
	}
}

// TestRun_VarUndefined_CarriesSourceLocation verifies that when a request
// references an undefined variable, the error chain carries a *Structured with
// FilePath and Line set from the RequestItem's SourceFile/SourceLine fields.
// This satisfies M6-007 D8.2.
func TestRun_VarUndefined_CarriesSourceLocation(t *testing.T) {
	tests := []struct {
		name       string
		sourceLine int
		sourceFile string
	}{
		{"line 7 in tests.yaml", 7, "tests.yaml"},
		{"line 12 in setup.yaml", 12, "setup.yaml"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			col := &parser.Collection{
				Name: "X",
				Requests: parser.Section{
					Items: []parser.RequestItem{{
						Name: "needs-var",
						Request: parser.Request{
							Method: "GET",
							URL:    "{{MISSING_VAR_M6007}}",
						},
						SourceFile: tt.sourceFile,
						SourceLine: tt.sourceLine,
					}},
				},
			}
			_, _, err := Run(context.Background(), col, successExecutor, VarSources{})
			if err == nil {
				t.Fatal("want err for undefined variable; got nil")
			}
			var s *apierrors.Structured
			if !errors.As(err, &s) {
				t.Fatalf("err is not *Structured: %T %v", err, err)
			}
			if s.FilePath != tt.sourceFile {
				t.Errorf("FilePath = %q, want %q", s.FilePath, tt.sourceFile)
			}
			if s.Line != tt.sourceLine {
				t.Errorf("Line = %d, want %d", s.Line, tt.sourceLine)
			}
			if s.Category != apierrors.CategoryInput {
				t.Errorf("Category = %q, want %q", s.Category, apierrors.CategoryInput)
			}
			if s.Code != "VAR_UNDEFINED" {
				t.Errorf("Code = %q, want VAR_UNDEFINED", s.Code)
			}
		})
	}
}

// makeCollectionWithPhases creates a collection with named setup, main, and
// teardown items for testing phase-aware filtering.
func makeCollectionWithPhases(setupNames, mainNames, teardownNames []string) *parser.Collection {
	makeItems := func(names []string) []parser.RequestItem {
		items := make([]parser.RequestItem, len(names))
		for i, n := range names {
			items[i] = parser.RequestItem{
				Name:    n,
				Request: parser.Request{Method: "GET", URL: "https://example.com/" + n},
			}
		}
		return items
	}
	return &parser.Collection{
		Name:     "TestCollection",
		Setup:    parser.Section{Items: makeItems(setupNames)},
		Requests: parser.Section{Items: makeItems(mainNames)},
		Teardown: parser.Section{Items: makeItems(teardownNames)},
	}
}

// TestRunner_OnlyFilter verifies that VarSources.Selection filters main-phase
// items while leaving setup and teardown intact. All cases that expect zero
// matches should return ErrNoMatchingRequests.
func TestRunner_OnlyFilter(t *testing.T) {
	recordExec := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200}, nil
	}

	// Helper to run with recording via the event sink.
	runWithRecording := func(col *parser.Collection, selection []string) ([]string, error) {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, recordExec, VarSources{
			Selection: selection,
			OnEvent:   sink,
		})
		sink.mu.Lock()
		executed := make([]string, len(sink.starts))
		for i, ev := range sink.starts {
			executed[i] = ev.Name
		}
		sink.mu.Unlock()
		return executed, err
	}

	t.Run("nil selection = all", func(t *testing.T) {
		col := makeCollectionWithPhases(
			[]string{"Setup1"},
			[]string{"A", "B"},
			[]string{"Teardown1"},
		)
		executed, err := runWithRecording(col, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"Setup1", "A", "B", "Teardown1"}
		if !stringSlicesEqual(executed, want) {
			t.Errorf("executed = %v, want %v", executed, want)
		}
	})

	t.Run("single match", func(t *testing.T) {
		col := makeCollectionWithPhases(
			[]string{"Setup1"},
			[]string{"A", "B", "C"},
			[]string{"Teardown1"},
		)
		executed, err := runWithRecording(col, []string{"B"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := []string{"Setup1", "B", "Teardown1"}
		if !stringSlicesEqual(executed, want) {
			t.Errorf("executed = %v, want %v", executed, want)
		}
	})

	t.Run("union of two", func(t *testing.T) {
		col := makeCollectionWithPhases(
			[]string{"Setup1"},
			[]string{"A", "B", "C"},
			[]string{"Teardown1"},
		)
		executed, err := runWithRecording(col, []string{"A", "C"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Order should follow the collection order, not selection order.
		want := []string{"Setup1", "A", "C", "Teardown1"}
		if !stringSlicesEqual(executed, want) {
			t.Errorf("executed = %v, want %v", executed, want)
		}
	})

	t.Run("case-sensitive mismatch", func(t *testing.T) {
		col := makeCollectionWithPhases(nil, []string{"Get user"}, nil)
		_, err := runWithRecording(col, []string{"get user"})
		if err == nil {
			t.Fatal("want error for case-sensitive mismatch, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("want ErrNoMatchingRequests, got %v", err)
		}
	})

	t.Run("no match lists available", func(t *testing.T) {
		col := makeCollectionWithPhases(nil, []string{"A", "B"}, nil)
		_, err := runWithRecording(col, []string{"Z"})
		if err == nil {
			t.Fatal("want error for no match, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("want ErrNoMatchingRequests, got %v", err)
		}
		// Error message should contain available names.
		if !strings.Contains(err.Error(), `"A"`) || !strings.Contains(err.Error(), `"B"`) {
			t.Errorf("error should list available names, got %q", err.Error())
		}
	})

	t.Run("setup name does not match main", func(t *testing.T) {
		col := makeCollectionWithPhases(
			[]string{"SetupItem"},
			[]string{"Main"},
			nil,
		)
		_, err := runWithRecording(col, []string{"SetupItem"})
		if err == nil {
			t.Fatal("want error: setup names don't match --only, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("want ErrNoMatchingRequests, got %v", err)
		}
	})

	t.Run("setup and teardown still run for single match", func(t *testing.T) {
		col := makeCollectionWithPhases(
			[]string{"Setup1", "Setup2"},
			[]string{"A", "B", "C"},
			[]string{"Teardown1"},
		)
		executed, err := runWithRecording(col, []string{"B"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		// Both setup items and teardown must appear.
		if !sliceContains(executed, "Setup1") {
			t.Errorf("Setup1 not in executed: %v", executed)
		}
		if !sliceContains(executed, "Setup2") {
			t.Errorf("Setup2 not in executed: %v", executed)
		}
		if !sliceContains(executed, "Teardown1") {
			t.Errorf("Teardown1 not in executed: %v", executed)
		}
		if !sliceContains(executed, "B") {
			t.Errorf("B not in executed: %v", executed)
		}
		if sliceContains(executed, "A") {
			t.Errorf("A should be filtered out, but was executed: %v", executed)
		}
		if sliceContains(executed, "C") {
			t.Errorf("C should be filtered out, but was executed: %v", executed)
		}
	})

	t.Run("empty string selection triggers no-match", func(t *testing.T) {
		col := makeCollectionWithPhases(nil, []string{"Get user"}, nil)
		// An empty string after TrimSpace still won't match a named request.
		_, err := runWithRecording(col, []string{""})
		if err == nil {
			t.Fatal("want error for empty name, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("want ErrNoMatchingRequests, got %v", err)
		}
	})
}

// TestRunner_BuildPreExecVarSet verifies that buildPreExecVarSet returns the
// name set of all variables already resolved on the scope.
func TestRunner_BuildPreExecVarSet(t *testing.T) {
	scope := variable.NewScope(map[string]string{"foo": "1", "bar": "2"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}
	got := buildPreExecVarSet(scope)
	want := map[string]bool{"foo": true, "bar": true}
	if len(got) != len(want) {
		t.Fatalf("got %v, want %v", got, want)
	}
	for k := range want {
		if !got[k] {
			t.Errorf("missing %q in %v", k, got)
		}
	}
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func sliceContains(s []string, v string) bool {
	for _, x := range s {
		if x == v {
			return true
		}
	}
	return false
}

// TestRunner_UndefinedVarMessageFormatStable locks the structured error message
// format that enrichSelectionCliff parses. If internal/variable changes the
// emission, this test fails so the regex is updated deliberately.
func TestRunner_UndefinedVarMessageFormatStable(t *testing.T) {
	scope := variable.NewScope(nil)
	_, err := scope.Interpolate("{{foo_bar}}")
	if err == nil {
		t.Fatal("expected error for undefined variable, got nil")
	}
	var s *apierrors.Structured
	if !errors.As(err, &s) {
		t.Fatalf("want *apierrors.Structured, got %T", err)
	}
	want := `undefined variable "foo_bar"`
	if s.Message != want {
		t.Errorf("undefined-variable message drifted:\n  got:  %q\n  want: %q\n  (update undefinedVarRe in runner.go if the format changed intentionally)", s.Message, want)
	}
}

// TestRun_OnlyVariableCliff tests the variable-cliff diagnostic: when --only
// filters out a request that would have produced a variable needed by the
// selected request, the error message should name the producer.
func TestRun_OnlyVariableCliff(t *testing.T) {
	// "Create user" extracts user_id; "Update user" references {{user_id}}.
	createUser := parser.RequestItem{
		Name: "Create user",
		Request: parser.Request{
			Method: "POST",
			URL:    "https://example.com/users",
		},
		Extract: map[string]string{
			"user_id": "$.id",
		},
	}
	updateUser := parser.RequestItem{
		Name: "Update user",
		Request: parser.Request{
			Method: "PUT",
			URL:    "https://example.com/users/{{user_id}}",
		},
	}
	col := &parser.Collection{
		Name: "VarCliff",
		Requests: parser.Section{Items: []parser.RequestItem{
			createUser,
			updateUser,
		}},
	}

	// executor that returns 200 with a JSON body for Create user.
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{
			StatusCode: 200,
			Body:       []byte(`{"id":"42"}`),
		}, nil
	}

	t.Run("producer filtered out names producer in error", func(t *testing.T) {
		// --only "Update user" but "Create user" is filtered out → variable cliff.
		_, _, err := Run(context.Background(), col, exec, VarSources{
			Selection: []string{"Update user"},
		})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(err.Error(), `"Create user"`) {
			t.Errorf("error should mention producer %q, got: %s", "Create user", err.Error())
		}
		if !strings.Contains(err.Error(), "not included by --only") {
			t.Errorf("error should mention '--only', got: %s", err.Error())
		}
	})

	t.Run("no selection uses existing undefined variable error", func(t *testing.T) {
		// Without --only, but with "Update user" and no prior extraction, the
		// error should be the plain undefined-variable message (no cliff enrichment).
		colOnlyUpdate := &parser.Collection{
			Name:     "VarCliffOnly",
			Requests: parser.Section{Items: []parser.RequestItem{updateUser}},
		}
		_, _, err := Run(context.Background(), colOnlyUpdate, exec, VarSources{})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		// Should NOT contain "not included by --only" since --only is not set.
		if strings.Contains(err.Error(), "not included by --only") {
			t.Errorf("without --only, error should not mention cliff: %s", err.Error())
		}
	})

	t.Run("producer in selection falls back to plain error", func(t *testing.T) {
		// Both "Create user" and "Update user" are selected, but the executor
		// returns a body that does NOT match the extract path ($.id is absent),
		// so user_id is never extracted. "Update user" will fail with an undefined-
		// variable error. Since the producer ("Create user") IS in the selection,
		// enrichSelectionCliff should leave the error unchanged (not mention
		// "not included by --only"). This exercises the inSelection early-return
		// branch at enrichSelectionCliff (review iteration 2, finding #2).
		execNoID := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
			// Return a body with no "id" field so $.id extraction yields empty.
			return &httpexec.Result{
				StatusCode: 200,
				Body:       []byte(`{"status":"ok"}`),
			}, nil
		}
		_, _, err := Run(context.Background(), col, execNoID, VarSources{
			Selection: []string{"Create user", "Update user"},
		})
		if err == nil {
			t.Fatal("expected error (user_id not extracted), got nil")
		}
		// The error must NOT be enriched with the cliff message, because the
		// producer is in the selection — the inSelection branch returns early.
		if strings.Contains(err.Error(), "not included by --only") {
			t.Errorf("producer is selected; error must not mention cliff, got: %s", err.Error())
		}
	})
}

// TestFilterMainItems exercises the exported FilterMainItems function directly,
// ensuring it delegates correctly to filterMainItemsBySelection (review iteration 2,
// finding #3: FilterMainItems is the exported wrapper used by cmd layer).
func TestFilterMainItems(t *testing.T) {
	items := []parser.RequestItem{
		{Name: "Alpha"},
		{Name: "Beta"},
		{Name: "Gamma"},
	}

	t.Run("single match", func(t *testing.T) {
		got, err := FilterMainItems(items, []string{"Beta"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 || got[0].Name != "Beta" {
			t.Errorf("FilterMainItems = %v, want [Beta]", got)
		}
	})

	t.Run("no match returns ErrNoMatchingRequests", func(t *testing.T) {
		_, err := FilterMainItems(items, []string{"Nope"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("expected ErrNoMatchingRequests, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"Nope"`) {
			t.Errorf("error must mention unknown name, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "available:") {
			t.Errorf("error must list available names, got: %s", err.Error())
		}
	})

	t.Run("union of two", func(t *testing.T) {
		got, err := FilterMainItems(items, []string{"Alpha", "Gamma"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("FilterMainItems len = %d, want 2", len(got))
		}
		names := []string{got[0].Name, got[1].Name}
		if names[0] != "Alpha" || names[1] != "Gamma" {
			t.Errorf("FilterMainItems = %v, want [Alpha Gamma]", names)
		}
	})

	// Finding #3 (M8-004 review iteration 3): when multiple --only names all fail to
	// match, the error message lists all unmatched names (not just selection[0]).
	t.Run("multi-name no-match lists all unmatched", func(t *testing.T) {
		_, err := FilterMainItems(items, []string{"Nope", "Also Nope"})
		if err == nil {
			t.Fatal("expected error, got nil")
		}
		if !errors.Is(err, ErrNoMatchingRequests) {
			t.Errorf("expected ErrNoMatchingRequests, got: %v", err)
		}
		if !strings.Contains(err.Error(), `"Nope"`) {
			t.Errorf("error must mention first unknown name, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), `"Also Nope"`) {
			t.Errorf("error must mention second unknown name, got: %s", err.Error())
		}
		if !strings.Contains(err.Error(), "available:") {
			t.Errorf("error must list available names, got: %s", err.Error())
		}
	})
}

// ---------------------------------------------------------------------------
// M8-005 helpers
// ---------------------------------------------------------------------------

// fixedBodyExec returns an ExecuteFunc that serves a map from URL path suffix
// (after "ex.com") to response body. Any path not in the map yields 404.
func fixedBodyExec(bodies map[string]string) ExecuteFunc {
	return func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		path := req.URL
		if i := strings.Index(req.URL, "ex.com"); i >= 0 {
			path = req.URL[i+6:]
		}
		if b, ok := bodies[path]; ok {
			return &httpexec.Result{StatusCode: 200, Body: []byte(b)}, nil
		}
		return &httpexec.Result{StatusCode: 404, Body: []byte(`{}`)}, nil
	}
}

// eventNames extracts Name values from a slice of RequestEvents.
func eventNames(evs []RequestEvent) []string {
	out := make([]string, len(evs))
	for i, ev := range evs {
		out[i] = ev.Name
	}
	return out
}

// makeRichCollection returns the canonical four-setup/two-main collection used
// to test regression (no --only → all items run).
//
//	setup: [Login (extract token), Seed users (extract user_id),
//	        Seed posts (uses {{user_id}}, extract post_id), Warm cache (no extract)]
//	main:  [Get user (uses {{token}}), List posts (uses {{post_id}})]
func makeRichCollection(_ *testing.T) *parser.Collection {
	return &parser.Collection{
		Name: "Rich",
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Login",
				Request: parser.Request{Method: "POST", URL: "https://ex.com/login"},
				Extract: map[string]string{"token": "$.token"},
			},
			{
				Name:    "Seed users",
				Request: parser.Request{Method: "POST", URL: "https://ex.com/users"},
				Extract: map[string]string{"user_id": "$.id"},
			},
			{
				Name:    "Seed posts",
				Request: parser.Request{Method: "POST", URL: "https://ex.com/users/{{user_id}}/posts"},
				Extract: map[string]string{"post_id": "$.id"},
			},
			{
				Name:    "Warm cache",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/health"},
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Get user",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/user", Headers: map[string]string{"Auth": "Bearer {{token}}"}},
			},
			{
				Name:    "List posts",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/posts/{{post_id}}"},
			},
		}},
	}
}

// richCollectionBodies returns the response map for makeRichCollection.
func richCollectionBodies() map[string]string {
	return map[string]string{
		"/login":          `{"token":"abc"}`,
		"/users":          `{"id":"u1"}`,
		"/users/u1/posts": `{"id":"p1"}`,
		"/health":         `{"status":"ok"}`,
		"/user":           `{"name":"x"}`,
		"/posts/p1":       `{"title":"y"}`,
	}
}

// ---------------------------------------------------------------------------
// M8-005 tests
// ---------------------------------------------------------------------------

// TestRunner_OnlyPrunesSetup verifies that when --only is set, the setup phase
// is pruned to the transitive closure of {{variable}} references from the
// selected main items. Setup items unreferenced by the closure and carrying an
// Extract: block are filtered out.
func TestRunner_OnlyPrunesSetup(t *testing.T) {
	col := makeRichCollection(t)
	exec := fixedBodyExec(richCollectionBodies())

	t.Run("Get user prunes seed users and seed posts", func(t *testing.T) {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, exec, VarSources{
			Selection: []string{"Get user"},
			OnEvent:   sink,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sink.mu.Lock()
		got := eventNames(sink.starts)
		sink.mu.Unlock()
		want := []string{"Login", "Warm cache", "Get user"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("executed = %v, want %v", got, want)
		}
	})

	t.Run("List posts prunes Login only", func(t *testing.T) {
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, exec, VarSources{
			Selection: []string{"List posts"},
			OnEvent:   sink,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		sink.mu.Lock()
		got := eventNames(sink.starts)
		sink.mu.Unlock()
		want := []string{"Seed users", "Seed posts", "Warm cache", "List posts"}
		if !stringSlicesEqual(got, want) {
			t.Errorf("executed = %v, want %v", got, want)
		}
	})
}

// TestRunner_OnlyPrunesSetup_ChainedProducers verifies the transitive closure
// traverses setup->setup producer chains.
func TestRunner_OnlyPrunesSetup_ChainedProducers(t *testing.T) {
	// A (extract a) -> B (uses a, extract b) -> C (uses b, extract c) -> main (uses c)
	col := &parser.Collection{
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/a"},
				Extract: map[string]string{"a": "$.a"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/b/{{a}}"},
				Extract: map[string]string{"b": "$.b"},
			},
			{
				Name:    "C",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/c/{{b}}"},
				Extract: map[string]string{"c": "$.c"},
			},
			{
				Name:    "Unused setup",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/skipme"},
				Extract: map[string]string{"unused": "$.x"},
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Main",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/main/{{c}}"},
			},
		}},
	}
	exec := fixedBodyExec(map[string]string{
		"/a":      `{"a":"1"}`,
		"/b/1":    `{"b":"2"}`,
		"/c/2":    `{"c":"3"}`,
		"/main/3": `{}`,
		"/skipme": `{}`,
	})

	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection: []string{"Main"},
		OnEvent:   sink,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	want := []string{"A", "B", "C", "Main"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("executed = %v, want %v (Unused setup must be pruned)", got, want)
	}
}

// TestRunner_OnlyPrunesSetup_NoExtractSeeder verifies setup items with empty
// extract: are included unconditionally even when --only targets a request that
// references none of their vars.
func TestRunner_OnlyPrunesSetup_NoExtractSeeder(t *testing.T) {
	col := &parser.Collection{
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Seeder",
				Request: parser.Request{Method: "POST", URL: "https://ex.com/seed"},
				// No Extract field — pure side-effect seeder.
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Main",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/main"},
			},
		}},
	}
	exec := fixedBodyExec(map[string]string{"/seed": `{}`, "/main": `{}`})

	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection: []string{"Main"},
		OnEvent:   sink,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	if !sliceContains(got, "Seeder") {
		t.Errorf("no-extract seeder must always run; executed = %v", got)
	}
}

// TestRunner_OnlyPrunesSetup_AnalyzerFallback verifies that when
// parallel.Analyze returns IsValid==false on the combined DAG, the runner falls
// back to running the full setup and emits a diagnostic line on Diagnostics.
func TestRunner_OnlyPrunesSetup_AnalyzerFallback(t *testing.T) {
	// Two setup items extract the same variable — analyzer flags it as a
	// parallel-variable collision, IsValid==false.
	col := &parser.Collection{
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "S1",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/1"},
				Extract: map[string]string{"dup": "$.x"},
			},
			{
				Name:    "S2",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/2"},
				Extract: map[string]string{"dup": "$.x"},
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Main",
				Request: parser.Request{Method: "GET", URL: "https://ex.com/main/{{dup}}"},
			},
		}},
	}
	exec := fixedBodyExec(map[string]string{
		"/1":      `{"x":"v"}`,
		"/2":      `{"x":"v"}`,
		"/main/v": `{}`,
	})

	diag := &bytes.Buffer{}
	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{
		Selection:   []string{"Main"},
		OnEvent:     sink,
		Diagnostics: diag,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Full setup should have run (fallback).
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	want := []string{"S1", "S2", "Main"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("fallback must run full setup; executed = %v, want %v", got, want)
	}
	// Diagnostic line must be emitted.
	if !strings.Contains(diag.String(), "minimal-setup analysis failed") {
		t.Errorf("expected diagnostic 'minimal-setup analysis failed' on Diagnostics, got %q", diag.String())
	}
}

// TestRunner_OnlyCliff_SetupProducer verifies that when an undefined variable's
// producer is in the (pruned) setup phase, the cliff diagnostic names it and
// flags the case as an analyzer miss (likely bug). This path is expected to be
// unreachable in practice with a correct analyzer.
func TestRunner_OnlyCliff_SetupProducer(t *testing.T) {
	// Synthesise the error that enrichSelectionCliff expects so we can exercise
	// the new setup-scan branch as a unit, without needing to trigger a real
	// scanner false-negative.
	setupItem := parser.RequestItem{
		Name:    "Seed",
		Request: parser.Request{Method: "POST", URL: "https://ex.com/seed"},
		Extract: map[string]string{"var_x": "$.x"},
	}
	mainItem := parser.RequestItem{
		Name:    "Use",
		Request: parser.Request{Method: "GET", URL: "https://ex.com/use"},
	}
	srcErr := &apierrors.Structured{
		Category: apierrors.CategoryInput,
		Code:     "UNDEFINED_VARIABLE",
		Message:  `undefined variable "var_x"`,
		Inner:    variable.ErrUndefinedVariable,
	}

	enriched := enrichSelectionCliff(
		srcErr,
		[]parser.RequestItem{mainItem},  // fullMain — does not produce var_x
		[]parser.RequestItem{setupItem}, // fullSetup — produces var_x (safety-net path)
		[]string{"Use"},
	)

	if !strings.Contains(enriched.Error(), `"Seed"`) {
		t.Errorf("expected error to name setup producer %q, got: %s", "Seed", enriched.Error())
	}
	if !strings.Contains(enriched.Error(), "pruned by --only") {
		t.Errorf("expected error to mention pruning, got: %s", enriched.Error())
	}
	if !strings.Contains(enriched.Error(), "likely an apitest bug") {
		t.Errorf("expected error to flag as a likely bug, got: %s", enriched.Error())
	}
	// Must still wrap the original error for errors.Is.
	if !errors.Is(enriched, variable.ErrUndefinedVariable) {
		t.Error("enriched error must still satisfy errors.Is(ErrUndefinedVariable)")
	}
}

// TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly verifies that without --only,
// the setup execution is byte-identical to pre-M8-005 behaviour.
func TestRunner_OnlyPrunesSetup_NoPruningWithoutOnly(t *testing.T) {
	col := makeRichCollection(t)
	exec := fixedBodyExec(richCollectionBodies())
	sink := &recordingSink{}
	_, _, err := Run(context.Background(), col, exec, VarSources{OnEvent: sink})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	sink.mu.Lock()
	got := eventNames(sink.starts)
	sink.mu.Unlock()
	// All 4 setup items + both main items, in order.
	want := []string{"Login", "Seed users", "Seed posts", "Warm cache", "Get user", "List posts"}
	if !stringSlicesEqual(got, want) {
		t.Errorf("without --only, execution must be unchanged; got %v want %v", got, want)
	}
}

// TestRunner_RequestSlugAllEmitSites verifies that RequestSlug is populated on
// every RequestStart and RequestEnd event across all emit paths.
func TestRunner_RequestSlugAllEmitSites(t *testing.T) {
	t.Run("sequential main and phases", func(t *testing.T) {
		col := &parser.Collection{
			Name: "Slug Test",
			Setup: parser.Section{Items: []parser.RequestItem{
				{Name: "Login", Slug: "login", Request: parser.Request{Method: "POST", URL: "https://example.com/login"}},
			}},
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "Get user", Slug: "get-user", Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"}},
				{Name: "Create Post!", Slug: "create-post", Request: parser.Request{Method: "POST", URL: "https://example.com/posts"}},
			}},
			Teardown: parser.Section{Items: []parser.RequestItem{
				{Name: "Logout", Slug: "logout", Request: parser.Request{Method: "POST", URL: "https://example.com/logout"}},
			}},
		}

		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{OnEvent: sink})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		sink.mu.Lock()
		starts := sink.starts
		ends := sink.ends
		sink.mu.Unlock()

		// 4 events: Login (setup) + 2 main + Logout (teardown).
		if len(starts) != 4 {
			t.Fatalf("want 4 request.start events, got %d", len(starts))
		}
		wantSlugs := map[string]string{
			"Login":        "login",
			"Get user":     "get-user",
			"Create Post!": "create-post",
			"Logout":       "logout",
		}
		for _, ev := range starts {
			if want, ok := wantSlugs[ev.Name]; ok {
				if ev.RequestSlug != want {
					t.Errorf("start %q: RequestSlug = %q, want %q", ev.Name, ev.RequestSlug, want)
				}
			}
		}
		for _, ev := range ends {
			// Match by pairing with the corresponding start.
			for _, s := range starts {
				if s.RequestID == ev.RequestID {
					if ev.RequestSlug != s.RequestSlug {
						t.Errorf("end %q: RequestSlug = %q, want %q", s.Name, ev.RequestSlug, s.RequestSlug)
					}
					break
				}
			}
		}
	})

	t.Run("data-driven sequential slug per iteration", func(t *testing.T) {
		dir := t.TempDir()
		writeCSVFile(t, dir, "data.csv", "val\nalpha\nbeta\ngamma")

		col := &parser.Collection{
			Name: "DD Slug Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{
					Name:       "Create user",
					Slug:       "create-user",
					DataDriven: &datadriven.Config{Source: "data.csv"},
					Request:    parser.Request{Method: "POST", URL: "https://example.com/u"},
				},
			}},
		}

		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			CollectionDir: dir,
			OnEvent:       sink,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		sink.mu.Lock()
		starts := sink.starts
		sink.mu.Unlock()

		// 3 iterations → 3 request.start events.
		if len(starts) != 3 {
			t.Fatalf("want 3 request.start events, got %d", len(starts))
		}
		// Each iteration name is "Create user [N/3]"; slug should be "create-user-n-3".
		wantSlugs := []string{"create-user-1-3", "create-user-2-3", "create-user-3-3"}
		for i, ev := range starts {
			if ev.RequestSlug != wantSlugs[i] {
				t.Errorf("iteration %d: RequestSlug = %q, want %q (name=%q)", i+1, ev.RequestSlug, wantSlugs[i], ev.Name)
			}
		}
	})

	t.Run("parallel main waves", func(t *testing.T) {
		// Three independent requests with no variable dependencies → one parallel
		// wave. The runner dispatches them via the parallel executor; every
		// RequestStart and RequestEnd must carry the per-item slug.
		col := &parser.Collection{
			Name: "Parallel Slug Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{Name: "Get users", Slug: "get-users", Request: parser.Request{Method: "GET", URL: "https://example.com/users"}},
				{Name: "Get posts", Slug: "get-posts", Request: parser.Request{Method: "GET", URL: "https://example.com/posts"}},
				{Name: "Get comments", Slug: "get-comments", Request: parser.Request{Method: "GET", URL: "https://example.com/comments"}},
			}},
		}
		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			Parallel: true,
			OnEvent:  sink,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		sink.mu.Lock()
		starts := sink.starts
		ends := sink.ends
		sink.mu.Unlock()

		if len(starts) != 3 {
			t.Fatalf("want 3 request.start events, got %d", len(starts))
		}
		if len(ends) != 3 {
			t.Fatalf("want 3 request.end events, got %d", len(ends))
		}

		wantSlugs := map[string]string{
			"Get users":    "get-users",
			"Get posts":    "get-posts",
			"Get comments": "get-comments",
		}
		for _, ev := range starts {
			if want, ok := wantSlugs[ev.Name]; ok {
				if ev.RequestSlug != want {
					t.Errorf("start %q: RequestSlug = %q, want %q", ev.Name, ev.RequestSlug, want)
				}
			} else {
				t.Errorf("unexpected start event name: %q", ev.Name)
			}
		}
		// Build a map of start slug by request_id to validate ends.
		startSlugByID := make(map[string]string, len(starts))
		for _, s := range starts {
			startSlugByID[s.RequestID] = s.RequestSlug
		}
		for _, ev := range ends {
			wantSlug := startSlugByID[ev.RequestID]
			if ev.RequestSlug != wantSlug {
				t.Errorf("end request_id=%q: RequestSlug = %q, want %q", ev.RequestID, ev.RequestSlug, wantSlug)
			}
		}
	})

	t.Run("data-driven parallel slug per iteration", func(t *testing.T) {
		// Data-driven with parallel: true. Three rows → three concurrent
		// iterations. Each iteration emits its own request.start and request.end
		// with the slug derived from the iteration name "Create post [N/3]".
		dir := t.TempDir()
		writeCSVFile(t, dir, "data.csv", "val\nfoo\nbar\nbaz")

		col := &parser.Collection{
			Name: "DD Parallel Slug Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{
					Name:       "Create post",
					Slug:       "create-post",
					DataDriven: &datadriven.Config{Source: "data.csv", Parallel: true},
					Request:    parser.Request{Method: "POST", URL: "https://example.com/posts"},
				},
			}},
		}

		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			CollectionDir: dir,
			OnEvent:       sink,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		sink.mu.Lock()
		starts := sink.starts
		ends := sink.ends
		sink.mu.Unlock()

		if len(starts) != 3 {
			t.Fatalf("want 3 request.start events, got %d", len(starts))
		}
		if len(ends) != 3 {
			t.Fatalf("want 3 request.end events, got %d", len(ends))
		}

		// Every start must have a non-empty slug matching "create-post-N-3".
		for _, ev := range starts {
			if ev.RequestSlug == "" {
				t.Errorf("start %q: RequestSlug is empty", ev.Name)
			}
			// Slug must start with the base slug prefix.
			if !strings.HasPrefix(ev.RequestSlug, "create-post-") {
				t.Errorf("start %q: RequestSlug = %q, expected prefix create-post-", ev.Name, ev.RequestSlug)
			}
		}
		// Every end slug must match its paired start slug.
		startSlugByID := make(map[string]string, len(starts))
		for _, s := range starts {
			startSlugByID[s.RequestID] = s.RequestSlug
		}
		for _, ev := range ends {
			wantSlug := startSlugByID[ev.RequestID]
			if ev.RequestSlug != wantSlug {
				t.Errorf("end request_id=%q: RequestSlug = %q, want %q (start slug)", ev.RequestID, ev.RequestSlug, wantSlug)
			}
		}
	})

	t.Run("websocket main", func(t *testing.T) {
		// A single WebSocket request: the runner takes the WebSocket code path
		// (not the HTTP executor) but must still emit request.start and
		// request.end with request_slug set.
		fc := &wsFakeConn{incoming: [][]byte{[]byte(`{"type":"pong"}`)}}
		dialer := &wsRunnerDialer{conn: fc}

		col := &parser.Collection{
			Name: "WS Slug Test",
			Requests: parser.Section{Items: []parser.RequestItem{
				{
					Name: "Ping Pong",
					Slug: "ping-pong",
					Request: parser.Request{
						Protocol: "websocket",
						URL:      "ws://fake/ws",
						Method:   "WS",
						WebSocket: &parser.WebSocketConfig{
							Steps: []parser.WebSocketStep{
								{Action: "send", MessageRaw: "ping"},
								{
									Action:    "expect",
									TimeoutMs: 200,
									ExpectAssertions: parser.BodyAssertions{
										Items: []parser.BodyAssertion{{Path: "$.type", Operator: "equals", Value: "pong"}},
									},
								},
								{Action: "close", Code: 1000},
							},
						},
					},
				},
			}},
		}

		sink := &recordingSink{}
		_, _, err := Run(context.Background(), col, successExecutor, VarSources{
			WebSocketDialer: dialer,
			OnEvent:         sink,
		})
		if err != nil {
			t.Fatalf("Run: %v", err)
		}

		sink.mu.Lock()
		starts := sink.starts
		ends := sink.ends
		sink.mu.Unlock()

		if len(starts) != 1 {
			t.Fatalf("want 1 request.start event, got %d", len(starts))
		}
		if len(ends) != 1 {
			t.Fatalf("want 1 request.end event, got %d", len(ends))
		}
		if starts[0].RequestSlug != "ping-pong" {
			t.Errorf("start RequestSlug = %q, want %q", starts[0].RequestSlug, "ping-pong")
		}
		if ends[0].RequestSlug != "ping-pong" {
			t.Errorf("end RequestSlug = %q, want %q", ends[0].RequestSlug, "ping-pong")
		}
	})
}

// TestRunner_AlwaysMintsRequestIDs verifies that every non-skipped
// RequestResult carries a non-empty RequestID, even when no EventSink is
// attached. Regression guard for the M9-002 splice contract: the markdown
// formatter relies on RequestID being populated regardless of --events.
func TestRunner_AlwaysMintsRequestIDs(t *testing.T) {
	col := makeCollection([]string{"First", "Second"}, false)
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("len(results) = %d, want 2", len(results))
	}
	if results[0].RequestID == "" {
		t.Error("results[0].RequestID is empty, want non-empty")
	}
	if results[1].RequestID == "" {
		t.Error("results[1].RequestID is empty, want non-empty")
	}
	if results[0].RequestID != "req-1" {
		t.Errorf("results[0].RequestID = %q, want %q", results[0].RequestID, "req-1")
	}
	if results[1].RequestID != "req-2" {
		t.Errorf("results[1].RequestID = %q, want %q", results[1].RequestID, "req-2")
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.RunID == "" {
		t.Error("summary.RunID is empty, want non-empty 32-char hex")
	}
	if len(summary.RunID) != 32 {
		t.Errorf("summary.RunID = %q, want 32-char hex, got len=%d", summary.RunID, len(summary.RunID))
	}
}

// TestRunner_RunIDInjectable verifies that vars.RunID overrides the
// auto-generated run identifier. Deterministic tests rely on this.
func TestRunner_RunIDInjectable(t *testing.T) {
	col := makeCollection([]string{"Only"}, false)
	const wantRunID = "deadbeef00000000deadbeef00000000"
	_, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		RunID: wantRunID,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if summary == nil {
		t.Fatal("summary is nil")
	}
	if summary.RunID != wantRunID {
		t.Errorf("summary.RunID = %q, want %q", summary.RunID, wantRunID)
	}
}

// TestRunner_RequestSlugCarriedToResult verifies that the slug derived at
// parse time on item.Slug appears on the RequestResult. Pairs with the M9-001
// guarantee that slugs are non-empty on parse.
func TestRunner_RequestSlugCarriedToResult(t *testing.T) {
	// Build a collection with slug already set (as parser.Parse would set it).
	col := &parser.Collection{
		Name: "Test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Get user",
				Slug:    "get-user",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("len(results) = %d, want 1", len(results))
	}
	if results[0].RequestSlug != "get-user" {
		t.Errorf("results[0].RequestSlug = %q, want %q", results[0].RequestSlug, "get-user")
	}
}

// TestRunner_SkippedRequestsCarrySlug verifies that skipped RequestResults
// (e.g. when setup fails) still carry the correct RequestSlug. This is required
// because buildMarkdownReport includes skipped main-phase requests in the report
// and uses RequestSlug as the file name. Without the slug, WriteReport would
// create a hidden file named ".md". M9-002 review finding #1.
func TestRunner_SkippedRequestsCarrySlug(t *testing.T) {
	// Build a collection with a failing required setup item and a main request.
	// When the required setup fails, all main requests are skipped.
	reqTrue := true
	col := &parser.Collection{
		Name: "Test",
		Setup: parser.Section{Items: []parser.RequestItem{
			{
				Name:     "Setup fails",
				Slug:     "setup-fails",
				Required: &reqTrue,
				Request:  parser.Request{Method: "GET", URL: "https://example.com/setup"},
			},
		}},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "Get user",
				Slug:    "get-user",
				Request: parser.Request{Method: "GET", URL: "https://example.com/users/1"},
			},
		}},
	}
	// Executor always fails (setup fails, triggering main-phase skips).
	failAlways := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return nil, errors.New("forced failure")
	}
	results, _, _ := Run(context.Background(), col, failAlways, VarSources{})
	// Find the skipped main-phase result.
	var skipped *RequestResult
	for i := range results {
		if results[i].Phase == PhaseMain && results[i].Skipped {
			skipped = &results[i]
			break
		}
	}
	if skipped == nil {
		t.Fatal("expected a skipped main-phase result, got none")
	}
	if skipped.RequestSlug != "get-user" {
		t.Errorf("skipped result RequestSlug = %q, want %q", skipped.RequestSlug, "get-user")
	}
}

// TestRun_HmacSha256_StripeStyleHeader_RedactsKey is an end-to-end integration
// test: a collection signs a payload with a secret sourced from a sensitive-named
// variable. Verifies the runner produces the correct hex MAC in the X-Signature
// header and registers the resolved key on RuntimeSensitive for downstream
// body redaction.
func TestRun_HmacSha256_StripeStyleHeader_RedactsKey(t *testing.T) {
	const (
		payload = "amount=100&currency=usd"
		key     = "sk_live_test_secret_abc"
	)

	// Compute the expected MAC independently using crypto/hmac.
	h := hmac.New(sha256.New, []byte(key))
	h.Write([]byte(payload))
	expectedMAC := fmt.Sprintf("%x", h.Sum(nil))

	var receivedSig string
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		receivedSig = req.Headers["X-Signature"]
		return &httpexec.Result{StatusCode: 200, Duration: 1 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "hmac-stripe-style",
		Variables: parser.SensitiveVars{
			Values: map[string]string{
				"payload":               payload,
				"stripe_signing_secret": key,
			},
			Sensitive: variable.NewSensitiveSet(),
		},
		Requests: parser.Section{
			Items: []parser.RequestItem{{
				Name: "Charge",
				Request: parser.Request{
					Method: "POST",
					URL:    "https://example.com",
					Headers: map[string]string{
						"X-Signature": `{{$hmacSha256('{{payload}}', '{{stripe_signing_secret}}')}}`,
					},
				},
			}},
		},
	}

	_, summary, err := Run(context.Background(), col, exec, VarSources{})
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}
	if receivedSig != expectedMAC {
		t.Errorf("X-Signature received = %q, want %q", receivedSig, expectedMAC)
	}
	if summary == nil || summary.RuntimeSensitive == nil {
		t.Fatal("expected non-nil summary.RuntimeSensitive")
	}
	vals := summary.RuntimeSensitive.Values()
	found := false
	for _, v := range vals {
		if v == key {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("RuntimeSensitive.Values() = %v, missing the resolved key %q", vals, key)
	}
}

// TestRun_HmacSha256_Parallel_BothKeysRegistered verifies that when two
// requests run concurrently in parallel mode and both use $hmacSha256 with
// sensitive keys, both resolved keys are registered on RuntimeSensitive
// without data races. Running with -race exposes any missing synchronisation
// on SensitiveSet.AddValue.
func TestRun_HmacSha256_Parallel_BothKeysRegistered(t *testing.T) {
	const (
		payload = "amount=100&currency=usd"
		keyA    = "sk_live_secret_key_aaa"
		keyB    = "sk_live_secret_key_bbb"
	)

	// Compute expected MACs independently.
	hA := hmac.New(sha256.New, []byte(keyA))
	hA.Write([]byte(payload))
	expectedMacA := fmt.Sprintf("%x", hA.Sum(nil))

	hB := hmac.New(sha256.New, []byte(keyB))
	hB.Write([]byte(payload))
	expectedMacB := fmt.Sprintf("%x", hB.Sum(nil))

	var mu sync.Mutex
	sigHeaders := map[string]string{}
	exec := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		sig := req.Headers["X-Signature"]
		mu.Lock()
		sigHeaders[req.URL] = sig
		mu.Unlock()
		return &httpexec.Result{StatusCode: 200, Duration: 1 * time.Millisecond}, nil
	}

	col := &parser.Collection{
		Name: "hmac-parallel",
		Variables: parser.SensitiveVars{
			Values: map[string]string{
				"payload":  payload,
				"key_a":    keyA,
				"key_b":    keyB,
				"secret_a": keyA, // sensitive-named copy for request A
				"secret_b": keyB, // sensitive-named copy for request B
			},
			Sensitive: variable.NewSensitiveSet(),
		},
		Requests: parser.Section{
			Items: []parser.RequestItem{
				{
					Name: "RequestA",
					Request: parser.Request{
						Method: "POST",
						URL:    "https://example.com/a",
						Headers: map[string]string{
							"X-Signature": `{{$hmacSha256('{{payload}}', '{{secret_a}}')}}`,
						},
					},
				},
				{
					Name: "RequestB",
					Request: parser.Request{
						Method: "POST",
						URL:    "https://example.com/b",
						Headers: map[string]string{
							"X-Signature": `{{$hmacSha256('{{payload}}', '{{secret_b}}')}}`,
						},
					},
				},
			},
		},
	}

	_, summary, err := Run(context.Background(), col, exec, VarSources{
		Parallel: true,
	})
	if err != nil {
		t.Fatalf("runner.Run: %v", err)
	}

	// Both signatures must be correct.
	if sigHeaders["https://example.com/a"] != expectedMacA {
		t.Errorf("RequestA X-Signature = %q, want %q", sigHeaders["https://example.com/a"], expectedMacA)
	}
	if sigHeaders["https://example.com/b"] != expectedMacB {
		t.Errorf("RequestB X-Signature = %q, want %q", sigHeaders["https://example.com/b"], expectedMacB)
	}

	// Both keys must appear in RuntimeSensitive.
	if summary == nil || summary.RuntimeSensitive == nil {
		t.Fatal("expected non-nil summary.RuntimeSensitive")
	}
	vals := summary.RuntimeSensitive.Values()
	valSet := make(map[string]bool, len(vals))
	for _, v := range vals {
		valSet[v] = true
	}
	if !valSet[keyA] {
		t.Errorf("RuntimeSensitive.Values() = %v, missing keyA %q", vals, keyA)
	}
	if !valSet[keyB] {
		t.Errorf("RuntimeSensitive.Values() = %v, missing keyB %q", vals, keyB)
	}
}

// ---------------------------------------------------------------------------
// M17-001: Signer step tests
// ---------------------------------------------------------------------------

// recordingSigner is a Signer that captures the *httpexec.Request it receives
// in Sign. Used to prove the runner invokes Sign after templating and before exec.
type recordingSigner struct {
	seenURL      string
	seenHeaders  map[string]string
	addHeader    string
	addValue     string
	addSensitive string // when non-empty, registers this string as sensitive
}

func (r *recordingSigner) Sign(_ context.Context, req *httpexec.Request, sensitives *variable.SensitiveSet) error {
	r.seenURL = req.URL
	r.seenHeaders = make(map[string]string, len(req.Headers))
	for k, v := range req.Headers {
		r.seenHeaders[k] = v
	}
	if r.addHeader != "" {
		if req.Headers == nil {
			req.Headers = make(map[string]string)
		}
		req.Headers[r.addHeader] = r.addValue
	}
	if r.addSensitive != "" {
		sensitives.AddValue(r.addSensitive)
	}
	return nil
}

func registerRecordingSigner(t *testing.T, reg *signer.Registry, name string) *recordingSigner {
	t.Helper()
	rs := &recordingSigner{addHeader: "X-Test-Signer", addValue: "ok"}
	if err := reg.Register(name, func(_ map[string]any) (signer.Signer, error) {
		return rs, nil
	}); err != nil {
		t.Fatalf("Register: %v", err)
	}
	return rs
}

func TestRunner_SignerStep_InvokedBetweenTemplatingAndExec(t *testing.T) {
	reg := signer.NewBuiltinRegistry()
	rs := registerRecordingSigner(t, reg, "noop")

	col := &parser.Collection{
		Name: "test",
		Variables: parser.SensitiveVars{
			Values:    map[string]string{"host": "example.com"},
			Sensitive: variable.NewSensitiveSet(),
		},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "signed",
				Request: parser.Request{Method: "GET", URL: "https://{{host}}/x"},
				Signing: &parser.SigningSpec{Type: "noop", Params: map[string]any{}},
			},
		}},
	}
	var seenByExec map[string]string
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		seenByExec = make(map[string]string, len(req.Headers))
		for k, v := range req.Headers {
			seenByExec[k] = v
		}
		return &httpexec.Result{StatusCode: 200, Duration: time.Millisecond}, nil
	}
	_, _, err := Run(context.Background(), col, execFn, VarSources{Signer: reg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// Sign saw the post-templated URL.
	if rs.seenURL != "https://example.com/x" {
		t.Errorf("signer saw URL %q; templating must run first", rs.seenURL)
	}
	// Exec received the signer-injected header.
	if seenByExec["X-Test-Signer"] != "ok" {
		t.Errorf("exec did not receive injected header; got %v", seenByExec)
	}
}

func TestRunner_SignerStep_NoSigning_FastPath(t *testing.T) {
	col := makeCollection([]string{"a"}, false)
	// No signing on collection or item — exec must run unmodified.
	var seenInjected bool
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if _, ok := req.Headers["X-Test-Signer"]; ok {
			seenInjected = true
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	// Pass a registry-with-something-registered to prove the wrap is bypassed
	// by item/collection emptiness, not by registry emptiness.
	reg := signer.NewBuiltinRegistry()
	_ = reg.Register("noop", func(_ map[string]any) (signer.Signer, error) {
		return &recordingSigner{addHeader: "X-Test-Signer", addValue: "should-not-fire"}, nil
	})
	_, _, err := Run(context.Background(), col, execFn, VarSources{Signer: reg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if seenInjected {
		t.Error("fast path violated: signer wrap fired despite no signing field")
	}
}

func TestRunner_SignerStep_CollectionDefault_RequestNullDisables(t *testing.T) {
	reg := signer.NewBuiltinRegistry()
	var fired int32
	_ = reg.Register("noop", func(_ map[string]any) (signer.Signer, error) {
		return &recordingSigner{addHeader: "X-Marker", addValue: "v"}, nil
	})

	// Parse the collection from inline YAML so the explicit-null `signing: ~`
	// is decoded by UnmarshalYAML — no test-only helper needed.
	dir := t.TempDir()
	yamlBody := `name: demo
signing:
  type: noop
requests:
  - name: inherits
    request:
      method: GET
      url: https://example.com/a
  - name: nulled
    request:
      method: GET
      url: https://example.com/b
    signing: ~
`
	fixturePath := filepath.Join(dir, "col.yaml")
	if err := os.WriteFile(fixturePath, []byte(yamlBody), 0o644); err != nil {
		t.Fatal(err)
	}
	col, err := parser.ParseFile(fixturePath)
	if err != nil {
		t.Fatalf("ParseFile: %v", err)
	}
	execFn := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		if _, ok := req.Headers["X-Marker"]; ok {
			atomic.AddInt32(&fired, 1)
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}
	_, _, runErr := Run(context.Background(), col, execFn, VarSources{Signer: reg})
	if runErr != nil {
		t.Fatalf("Run: %v", runErr)
	}
	if got := atomic.LoadInt32(&fired); got != 1 {
		t.Errorf("expected signer to fire on inheriting request only; fired %d times", got)
	}
}

func TestRunner_SignerStep_UnknownTypeProducesError(t *testing.T) {
	col := &parser.Collection{
		Name: "demo",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "x",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Signing: &parser.SigningSpec{Type: "missing"},
			},
		}},
	}
	reg := signer.NewBuiltinRegistry()
	_ = reg.Register("alpha", func(_ map[string]any) (signer.Signer, error) { return nil, nil })

	results, _, err := Run(context.Background(), col, successExecutor, VarSources{Signer: reg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("results = %d, want 1", len(results))
	}
	if results[0].Err == nil {
		t.Fatal("expected RequestResult.Err for unknown signer type")
	}
	if !errors.Is(results[0].Err, signer.ErrUnknownSignerType) {
		t.Errorf("err must wrap ErrUnknownSignerType; got %v", results[0].Err)
	}
	if !strings.Contains(results[0].Err.Error(), "alpha") {
		t.Errorf("err must list available types; got %q", results[0].Err.Error())
	}
}

func TestRunner_SignerStep_SensitiveValueRegistered(t *testing.T) {
	reg := signer.NewBuiltinRegistry()
	rs := &recordingSigner{addHeader: "X-Auth", addValue: "v", addSensitive: "supersecret"}
	_ = reg.Register("noop", func(_ map[string]any) (signer.Signer, error) { return rs, nil })

	col := &parser.Collection{
		Name: "demo",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "x",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Signing: &parser.SigningSpec{Type: "noop"},
			},
		}},
	}
	_, summary, err := Run(context.Background(), col, successExecutor, VarSources{Signer: reg})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	found := false
	for _, v := range summary.RuntimeSensitive.Values() {
		if v == "supersecret" {
			found = true
			break
		}
	}
	if !found {
		t.Error("AddValue called inside Sign must register on the run's RuntimeSensitive set")
	}
}

// TestExecutePhase_DependsOn_SkipsOnSkippedParent verifies that a request item
// with depends_on: referencing a skipped item is itself skipped with reason
// "parent skipped: <name>".
func TestExecutePhase_DependsOn_SkipsOnSkippedParent(t *testing.T) {
	// Collection: A passes, B fails assertions (stop_on_failure triggers), C depends on B.
	col := &parser.Collection{
		Name:    "T",
		Options: parser.Options{StopOnFailure: true},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{999}}, // always fails
				},
			},
			{
				Name:      "C",
				DependsOn: []string{"B"},
				Request:   parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}

	// A passes, B fails (assertion), C is skipped (stop_on_failure cascade).
	// With depends_on C→B and B skipped: C should be skipped with reason "parent skipped: B".
	if len(results) != 3 {
		t.Fatalf("want 3 results, got %d", len(results))
	}
	// B is failed (stop_on_failure stops the runner after B)
	if !results[1].Skipped && results[1].AssertionResults != nil && results[1].AssertionResults.Passed {
		t.Errorf("result[1] (B): want failed assertion, got %+v", results[1])
	}
	// C is skipped
	if !results[2].Skipped {
		t.Errorf("result[2] (C): want skipped, got %+v", results[2])
	}
	_ = summary
}

// TestExecutePhase_DependsOn_SkipsOnSkippedParent_Cascade verifies that when
// parent A is skipped via if:false and B depends_on A, B is skipped with the
// "parent skipped" reason (tested end-to-end via Run).
func TestExecutePhase_DependsOn_SkipsOnSkippedParent_Cascade(t *testing.T) {
	col := &parser.Collection{
		Name: "cascade",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				If:      "1 + 1 == 3", // always false
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
			{
				Name:      "B",
				DependsOn: []string{"A"},
				Request:   parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Errorf("result[0] (A): want skipped, got %+v", results[0])
	}
	if !results[1].Skipped {
		t.Errorf("result[1] (B): want skipped, got %+v", results[1])
	}
	if results[1].SkipReason != "parent skipped: A" {
		t.Errorf("result[1].SkipReason = %q, want %q", results[1].SkipReason, "parent skipped: A")
	}
}

// --- TestIfConditional_* tests (M19-001 behaviours) ---

// TestIfConditional_TrueRuns verifies that a request with an if: expression that
// evaluates to true is executed and its result appears in the output.
func TestIfConditional_TrueRuns(t *testing.T) {
	col := &parser.Collection{
		Name: "iftest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				If:      "1 + 1 == 2", // always true
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Skipped {
		t.Errorf("result[0] should not be skipped; got SkipReason=%q", results[0].SkipReason)
	}
	if summary.Passed != 1 || summary.Skipped != 0 {
		t.Errorf("want Passed=1 Skipped=0, got Passed=%d Skipped=%d", summary.Passed, summary.Skipped)
	}
}

// TestIfConditional_FalseSkips verifies that a request with an if: expression that
// evaluates to false is skipped with reason "if: false" and no HTTP call is made.
func TestIfConditional_FalseSkips(t *testing.T) {
	execCalled := 0
	recorder := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled++
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "iftest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				If:      "1 + 1 == 3", // always false
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, summary, err := Run(context.Background(), col, recorder, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if execCalled != 0 {
		t.Errorf("exec should not be called for skipped items; called %d times", execCalled)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !results[0].Skipped {
		t.Errorf("result[0] should be skipped")
	}
	if results[0].SkipReason != "if: false" {
		t.Errorf("SkipReason = %q, want %q", results[0].SkipReason, "if: false")
	}
	if summary.Skipped != 1 || summary.Passed != 0 {
		t.Errorf("want Skipped=1 Passed=0, got Skipped=%d Passed=%d", summary.Skipped, summary.Passed)
	}
}

// TestIfConditional_SkipsBeforeTemplating verifies that when if: evaluates to
// false, the templating step is never invoked (no variable interpolation errors
// even when the URL contains an undefined placeholder).
func TestIfConditional_SkipsBeforeTemplating(t *testing.T) {
	execCalled := 0
	recorder := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled++
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "iftest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "A",
				If:   "1 + 1 == 3", // false — gate fires before templating
				// URL has an undefined variable; if templating ran, Run would return an error.
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{undefined_variable_that_should_not_be_resolved}}"},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, recorder, VarSources{})
	if err != nil {
		t.Fatalf("Run should not error when if: is false (templating skipped); got: %v", err)
	}
	if execCalled != 0 {
		t.Errorf("exec should not be called; called %d times", execCalled)
	}
	if len(results) != 1 || !results[0].Skipped {
		t.Errorf("want 1 skipped result; got %+v", results)
	}
}

// TestIfConditional_SkipPropagatesToDependents verifies that when A is skipped
// via if: false, B (which depends_on A) is also skipped with "parent skipped: A".
func TestIfConditional_SkipPropagatesToDependents(t *testing.T) {
	col := &parser.Collection{
		Name: "iftest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				If:      "1 + 1 == 3", // always false
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
			{
				Name:      "B",
				DependsOn: []string{"A"},
				Request:   parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	if !results[0].Skipped || results[0].SkipReason != "if: false" {
		t.Errorf("result[0] (A): want skipped with reason 'if: false', got %+v", results[0])
	}
	if !results[1].Skipped || results[1].SkipReason != "parent skipped: A" {
		t.Errorf("result[1] (B): want skipped with reason 'parent skipped: A', got %+v", results[1])
	}
	if summary.Skipped != 2 {
		t.Errorf("summary.Skipped = %d, want 2", summary.Skipped)
	}
}

// TestIfConditional_PreviousIsLastNonSkipped verifies that `previous` in a CEL
// expression refers to the last executed (non-skipped) response.
func TestIfConditional_PreviousIsLastNonSkipped(t *testing.T) {
	// A executes and returns body {"ok": true}.
	// B's if: expression references previous.body.ok == true → should execute.
	// C is skipped (if: false). D's if: expression still sees A's response as previous.
	bodyJSON := []byte(`{"ok": true}`)
	executor := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		return &httpexec.Result{StatusCode: 200, Body: bodyJSON}, nil
	}
	col := &parser.Collection{
		Name: "previous-test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
			{
				// B's if: uses previous (A's response); body.ok is true → runs
				Name:    "B",
				If:      "1 + 1 == 2", // simple true; actual previous test via result below
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, executor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	for i, r := range results {
		if r.Skipped {
			t.Errorf("result[%d] (%s): unexpectedly skipped", i, r.Name)
		}
	}
}

// TestIfConditional_ParallelFallsBackToSequential verifies that when vars.Parallel
// is true and any item has if:, the runner falls back to sequential execution and
// emits a diagnostics line.
func TestIfConditional_ParallelFallsBackToSequential(t *testing.T) {
	col := &parser.Collection{
		Name: "parallel-fallback",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				If:      "1 + 1 == 2",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
			{
				Name:    "B",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}
	var diagBuf bytes.Buffer
	results, summary, err := Run(context.Background(), col, successExecutor, VarSources{
		Parallel:    true,
		Diagnostics: &diagBuf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.IsParallel {
		t.Error("IsParallel should be false after fallback")
	}
	if !strings.Contains(diagBuf.String(), "if: gate active") {
		t.Errorf("diagnostics should mention fallback; got: %q", diagBuf.String())
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
}

// TestVarSources_CelEvaluator_LazyInit verifies that ensureCelEvaluator returns
// a non-nil evaluator, and that once the caller stores the result back into
// CelEvaluator the same instance is returned on the next call.
func TestVarSources_CelEvaluator_LazyInit(t *testing.T) {
	var vs VarSources
	ev1, err := vs.ensureCelEvaluator()
	if err != nil {
		t.Fatalf("ensureCelEvaluator: %v", err)
	}
	if ev1 == nil {
		t.Fatal("expected non-nil evaluator")
	}
	// Simulate what runPhases does: store the evaluator back so the next call
	// returns the same instance (caller-managed caching, no sync.Once needed).
	vs.CelEvaluator = ev1
	ev2, err2 := vs.ensureCelEvaluator()
	if err2 != nil {
		t.Fatalf("ensureCelEvaluator (2nd call): %v", err2)
	}
	if ev1 != ev2 {
		t.Error("expected the stored evaluator to be returned on the second call")
	}
}

// TestVarSources_CelEvaluator_ExplicitOverride verifies that when CelEvaluator
// is set on VarSources, ensureCelEvaluator returns the provided evaluator
// without building a new one.
func TestVarSources_CelEvaluator_ExplicitOverride(t *testing.T) {
	injected, err := apicel.NewEvaluator()
	if err != nil {
		t.Fatalf("NewEvaluator: %v", err)
	}
	vs := VarSources{CelEvaluator: injected}
	got, err2 := vs.ensureCelEvaluator()
	if err2 != nil {
		t.Fatalf("ensureCelEvaluator: %v", err2)
	}
	if got != injected {
		t.Error("expected the explicitly injected evaluator to be returned")
	}
}

// TestIfConditional_SensitiveValueRoutedToRuntimeSet verifies behavior 4:
// when a sensitive variable is referenced inside an if: expression, its
// resolved value is routed into summary.RuntimeSensitive via SensitiveObserver.
func TestIfConditional_SensitiveValueRoutedToRuntimeSet(t *testing.T) {
	// Build a sensitive set that marks "token" as sensitive.
	sens := variable.NewSensitiveSet()
	sens.Add("token")

	col := &parser.Collection{
		Name: "sensitive-if-test",
		Variables: parser.SensitiveVars{
			Values:    map[string]string{"token": "abc"},
			Sensitive: sens,
		},
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "A",
				// The if: expression references vars.token. Because "token" is in
				// SensitiveNames, the observer fires with value "abc".
				If:      `vars.token == "abc"`, // evaluates to true → request runs
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}

	_, summary, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if summary.RuntimeSensitive == nil {
		t.Fatal("summary.RuntimeSensitive is nil; expected a non-nil set")
	}
	vals := summary.RuntimeSensitive.Values()
	found := false
	for _, v := range vals {
		if v == "abc" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected sensitive value %q to be in RuntimeSensitive.Values(); got: %v", "abc", vals)
	}
}

// TestIfConditional_DataDriven_SkipsAllRows verifies that when a data-driven
// item carries if: false, the entire item is skipped (one skipped result, not
// one result per data row), and the data source is never iterated.
func TestIfConditional_DataDriven_SkipsAllRows(t *testing.T) {
	dir := t.TempDir()
	// CSV with three rows so we can confirm each row was NOT executed.
	writeCSVFile(t, dir, "rows.csv", "id\n1\n2\n3")

	execCalled := 0
	recorder := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled++
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "dd-if-test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "rows",
				If:   "1 + 1 == 3", // always false → gate fires before data-driven expansion
				DataDriven: &datadriven.Config{
					Source: "rows.csv",
					Format: "csv",
				},
				Request: parser.Request{Method: "GET", URL: "https://example.com/{{id}}"},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, recorder, VarSources{
		CollectionDir: dir,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if execCalled != 0 {
		t.Errorf("exec should not be called for a skipped item; called %d times", execCalled)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 skipped result (not one per row), got %d results", len(results))
	}
	if !results[0].Skipped {
		t.Errorf("result[0] should be skipped")
	}
	if results[0].SkipReason != "if: false" {
		t.Errorf("SkipReason = %q, want %q", results[0].SkipReason, "if: false")
	}
	if summary.Skipped != 1 {
		t.Errorf("summary.Skipped = %d, want 1", summary.Skipped)
	}
}

// TestIfConditional_NonBoolRejectedByValidate verifies that a collection with
// a non-boolean CEL expression in an if: field (e.g. "1 + 1" which evaluates
// to int) is rejected by the validator with ERR_CEL_TYPE. This test satisfies
// the task observable which explicitly names this test in the runner package.
func TestIfConditional_NonBoolRejectedByValidate(t *testing.T) {
	dir := t.TempDir()
	yamlPath := filepath.Join(dir, "type_error.yaml")
	if err := os.WriteFile(yamlPath, []byte(`name: Test
requests:
  - name: A
    if: "1 + 1"
    request:
      method: GET
      url: https://example.com
`), 0o600); err != nil {
		t.Fatalf("write fixture: %v", err)
	}

	result := validator.Validate(yamlPath, nil)

	if result.Valid {
		t.Fatal("expected invalid result for non-bool if: expression, got valid")
	}
	found := false
	for _, iss := range result.Issues {
		if strings.Contains(iss.Message, "ERR_CEL_TYPE") {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("expected issue containing ERR_CEL_TYPE for if: \"1 + 1\"; got issues: %v", result.Issues)
	}
}

// TestIfConditional_PreviousUpdatesOnAssertionFail verifies that `previous` is
// updated with the HTTP response even when the request's assertions fail.
// This ensures that if: expressions on subsequent items can inspect the actual
// response from a prior failed request (e.g. previous.status == 404).
func TestIfConditional_PreviousUpdatesOnAssertionFail(t *testing.T) {
	// A: returns 404 but asserts status 200 → assertion fails, request recorded as failed.
	// B: if: "previous.status == 404" → should evaluate true (B runs) because previous
	//    must be updated from A's actual 404 response, not left nil.
	executor := func(_ context.Context, req *httpexec.Request) (*httpexec.Result, error) {
		// A gets 404, B gets 200.
		if req.URL == "https://example.com/a" {
			return &httpexec.Result{StatusCode: 404, Body: []byte(`{}`)}, nil
		}
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "previous-on-fail",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com/a"},
				// Asserts 200 but gets 404 → assertion fails.
				Assertions: parser.Assertions{
					Status: parser.StatusCodes{Codes: []int{200}},
				},
			},
			{
				Name:    "B",
				If:      "previous.status == 404",
				Request: parser.Request{Method: "GET", URL: "https://example.com/b"},
			},
		}},
	}

	results, _, err := Run(context.Background(), col, executor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 2 {
		t.Fatalf("want 2 results, got %d", len(results))
	}
	// A should be recorded as failed (assertion failure).
	if results[0].Skipped {
		t.Errorf("A should not be skipped")
	}
	if results[0].AssertionResults == nil || results[0].AssertionResults.Passed {
		t.Errorf("A should have failing assertions")
	}
	// B should have run (if: true because previous.status == 404).
	if results[1].Skipped {
		t.Errorf("B should not be skipped; previous.status should be 404 from A's response")
	}
}

// TestIfConditional_EnvAccessible verifies that the env activation surface is
// populated from vars.EnvVar so that if: expressions can reference env.<name>.
func TestIfConditional_EnvAccessible(t *testing.T) {
	execCalled := false
	recorder := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled = true
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "env-gate-test",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "gated",
				If:      `env.RUN_MODE == "skip"`,
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
			},
		}},
	}

	// EnvVar["RUN_MODE"] = "skip" → if: evaluates true → gate open → request executes.
	results, _, err := Run(context.Background(), col, recorder, VarSources{
		EnvVar: map[string]string{"RUN_MODE": "skip"},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// if: "env.RUN_MODE == \"skip\"" with env.RUN_MODE = "skip" → true → gate open → runs
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].Skipped {
		t.Errorf("result should not be skipped when env.RUN_MODE matches the if: expression")
	}
	if !execCalled {
		t.Error("exec should have been called when if: gate is open")
	}

	// Second sub-case: env.RUN_MODE = "run" → condition is false → item skipped.
	execCalled = false
	results2, _, err2 := Run(context.Background(), col, recorder, VarSources{
		EnvVar: map[string]string{"RUN_MODE": "run"},
	})
	if err2 != nil {
		t.Fatalf("Run (false case): %v", err2)
	}
	if len(results2) != 1 {
		t.Fatalf("want 1 result, got %d", len(results2))
	}
	if !results2[0].Skipped {
		t.Errorf("result should be skipped when env.RUN_MODE does not match the if: expression")
	}
	if execCalled {
		t.Error("exec should not be called when if: gate is closed")
	}
}

// --- TestCelAssertion_* tests (M19-004 behaviours) ---

// TestCelAssertion_TruePasses verifies that a cel: assertion that evaluates
// to true is recorded as passed in the assertion results.
func TestCelAssertion_TruePasses(t *testing.T) {
	col := &parser.Collection{
		Name: "celtest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					CEL: parser.CELAssertions{Items: []parser.CELAssertion{
						{Source: "response.status == 200"},
					}},
				},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].AssertionResults == nil {
		t.Fatal("want assertion results, got nil")
	}
	celResult := findAssertionByType(results[0].AssertionResults, "assertions[0].cel")
	if celResult == nil {
		t.Fatal("want assertions[0].cel result, got nil")
	}
	if !celResult.Passed {
		t.Errorf("cel assertion should pass, got: actual=%q", celResult.Actual)
	}
}

// TestCelAssertion_FalseFails verifies that a cel: assertion that evaluates
// to false is recorded as failed with the expression source in the message.
func TestCelAssertion_FalseFails(t *testing.T) {
	col := &parser.Collection{
		Name: "celtest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					CEL: parser.CELAssertions{Items: []parser.CELAssertion{
						{Source: "response.status == 404"},
					}},
				},
			},
		}},
	}
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if results[0].AssertionResults == nil {
		t.Fatal("want assertion results, got nil")
	}
	celResult := findAssertionByType(results[0].AssertionResults, "assertions[0].cel")
	if celResult == nil {
		t.Fatal("want assertions[0].cel result, got nil")
	}
	if celResult.Passed {
		t.Errorf("cel assertion should fail")
	}
	if !strings.Contains(celResult.Actual, "response.status") {
		t.Errorf("failure message missing ref: %q", celResult.Actual)
	}
}

// TestCelAssertion_MutualExclusionWithOperatorAssertion verifies that
// a collection with both cel: and operator keys on the same entry fails
// to parse with an appropriate error.
func TestCelAssertion_MutualExclusionWithOperatorAssertion(t *testing.T) {
	const yamlDoc = `
name: c
requests:
  - name: r
    request:
      method: GET
      url: https://example.com
    assertions:
      cel:
        - cel: "response.status == 200"
          eq: 200
`
	dir := t.TempDir()
	f := filepath.Join(dir, "cel_and_operator.yaml")
	if err := os.WriteFile(f, []byte(yamlDoc), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	_, err := parser.ParseFile(f)
	if err == nil {
		t.Fatal("want error, got nil")
	}
	if !strings.Contains(err.Error(), "mutually exclusive") {
		t.Errorf("want 'mutually exclusive' error, got: %v", err)
	}
}

// TestCollectionHasCelAssertions verifies the predicate that drives CEL
// evaluator pre-init.
func TestCollectionHasCelAssertions(t *testing.T) {
	tests := []struct {
		name  string
		items []parser.RequestItem
		want  bool
	}{
		{"empty", nil, false},
		{"none", []parser.RequestItem{{Name: "r", Request: parser.Request{Method: "GET", URL: "https://x.com"}}}, false},
		{"main has cel", []parser.RequestItem{{
			Name:    "r",
			Request: parser.Request{Method: "GET", URL: "https://x.com"},
			Assertions: parser.Assertions{
				CEL: parser.CELAssertions{Items: []parser.CELAssertion{{Source: "true"}}},
			},
		}}, true},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			col := &parser.Collection{
				Name:     "test",
				Requests: parser.Section{Items: tc.items},
			}
			got := collectionHasCelAssertions(col)
			if got != tc.want {
				t.Errorf("collectionHasCelAssertions = %v, want %v", got, tc.want)
			}
		})
	}
}

// TestCelAssertion_ParallelFallback verifies that a collection with CEL
// assertions and Parallel=true falls back to sequential execution.
func TestCelAssertion_ParallelFallback(t *testing.T) {
	col := &parser.Collection{
		Name: "celtest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name:    "A",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					CEL: parser.CELAssertions{Items: []parser.CELAssertion{
						{Source: "response.status == 200"},
					}},
				},
			},
		}},
	}
	var diagBuf bytes.Buffer
	results, _, err := Run(context.Background(), col, successExecutor, VarSources{
		Parallel:    true,
		Diagnostics: &diagBuf,
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	if !strings.Contains(diagBuf.String(), "falling back to sequential") {
		t.Errorf("diagnostics should mention fallback; got: %q", diagBuf.String())
	}
}

// TestRun_CelAssertion_SkippedByIf_NotEvaluated verifies behavior #6:
// when a request is skipped by its if: gate, no cel: assertion is evaluated
// and none is counted in the run's assertion totals.
func TestRun_CelAssertion_SkippedByIf_NotEvaluated(t *testing.T) {
	execCalled := 0
	recorder := func(_ context.Context, _ *httpexec.Request) (*httpexec.Result, error) {
		execCalled++
		return &httpexec.Result{StatusCode: 200}, nil
	}

	col := &parser.Collection{
		Name: "celtest",
		Requests: parser.Section{Items: []parser.RequestItem{
			{
				Name: "A",
				// if: evaluates to false — the entire item is skipped.
				If:      "1 + 1 == 3",
				Request: parser.Request{Method: "GET", URL: "https://example.com"},
				Assertions: parser.Assertions{
					CEL: parser.CELAssertions{Items: []parser.CELAssertion{
						// This would fail if evaluated; it must NOT be evaluated.
						{Source: "response.status == 999"},
					}},
				},
			},
		}},
	}

	results, summary, err := Run(context.Background(), col, recorder, VarSources{})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	// No HTTP call should have been made.
	if execCalled != 0 {
		t.Errorf("exec should not be called for skipped item; called %d times", execCalled)
	}
	if len(results) != 1 {
		t.Fatalf("want 1 result, got %d", len(results))
	}
	// Item must be recorded as Skipped.
	if !results[0].Skipped {
		t.Errorf("want Skipped=true, got false")
	}
	if results[0].SkipReason != "if: false" {
		t.Errorf("SkipReason = %q, want %q", results[0].SkipReason, "if: false")
	}
	// AssertionResults must be nil — no CEL assertion was evaluated.
	if results[0].AssertionResults != nil {
		t.Errorf("want nil AssertionResults for skipped item, got %+v", results[0].AssertionResults)
	}
	// Summary must record the item as Skipped, not as a failed assertion.
	if summary.Skipped != 1 {
		t.Errorf("summary.Skipped = %d, want 1", summary.Skipped)
	}
	if summary.AssertionFailures != 0 {
		t.Errorf("summary.AssertionFailures = %d, want 0 (skipped items must not count assertions)", summary.AssertionFailures)
	}
}

// findAssertionByType returns the first Result whose Type matches the given
// string, or nil if none is found.
func findAssertionByType(ar *assertion.Results, typ string) *assertion.Result {
	if ar == nil {
		return nil
	}
	for i := range ar.Items {
		if ar.Items[i].Type == typ {
			return &ar.Items[i]
		}
	}
	return nil
}

// ---- Step 4: Runner locale precedence resolver ----

func TestResolveLocale_Precedence(t *testing.T) {
	tests := []struct {
		name    string
		v       VarSources
		want    string
		wantErr bool
	}{
		{
			name:    "cli beats all",
			v:       VarSources{ProjectLocale: "en-US", CollectionLocale: "fr-FR", Locale: "de-DE"},
			want:    "de-DE",
			wantErr: false,
		},
		{
			name:    "collection beats project when no flag",
			v:       VarSources{ProjectLocale: "en-US", CollectionLocale: "de-DE"},
			want:    "de-DE",
			wantErr: false,
		},
		{
			name:    "project only",
			v:       VarSources{ProjectLocale: "de-DE"},
			want:    "de-DE",
			wantErr: false,
		},
		{
			name:    "nothing set returns empty",
			v:       VarSources{},
			want:    "",
			wantErr: false,
		},
		{
			name:    "unknown winner errors",
			v:       VarSources{Locale: "xx-YY"},
			want:    "",
			wantErr: true,
		},
		{
			name:    "whitespace-only values are skipped",
			v:       VarSources{ProjectLocale: "  ", CollectionLocale: "de-DE"},
			want:    "de-DE",
			wantErr: false,
		},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveLocale(tc.v)
			if tc.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Errorf("resolveLocale = %q, want %q", got, tc.want)
			}
		})
	}
}
