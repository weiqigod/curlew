package graphql

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"github.com/peterlindqvist/apitest/internal/parser"
)

func TestBuildRequest(t *testing.T) {
	tests := []struct {
		name    string
		req     *parser.Request
		wantErr error
		check   func(*testing.T, *parser.Request)
	}{
		{
			name: "basic query sets POST method",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL:  &parser.GraphQLConfig{Query: "{ users { id } }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				if req.Method != "POST" {
					t.Errorf("Method = %q, want POST", req.Method)
				}
			},
		},
		{
			name: "basic query sets Content-Type application/json",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL:  &parser.GraphQLConfig{Query: "{ users { id } }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				if req.Headers["Content-Type"] != "application/json" {
					t.Errorf("Content-Type = %q, want application/json", req.Headers["Content-Type"])
				}
			},
		},
		{
			name: "query with variables includes variables in body",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL: &parser.GraphQLConfig{
					Query:     "query GetUser($id: ID!) { user(id: $id) { name } }",
					Variables: map[string]any{"id": "123"},
				},
			},
			check: func(t *testing.T, req *parser.Request) {
				bodyStr, ok := req.Body.(string)
				if !ok {
					t.Fatalf("Body type = %T, want string", req.Body)
				}
				var body RequestBody
				if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				if body.Variables["id"] != "123" {
					t.Errorf("Variables[id] = %v, want 123", body.Variables["id"])
				}
			},
		},
		{
			name: "preserves existing headers",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				Headers:  map[string]string{"Authorization": "Bearer token123"},
				GraphQL:  &parser.GraphQLConfig{Query: "{ me { id } }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				if req.Headers["Authorization"] != "Bearer token123" {
					t.Errorf("Authorization = %q, want Bearer token123", req.Headers["Authorization"])
				}
			},
		},
		{
			name: "does not override existing Content-Type",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				Headers:  map[string]string{"Content-Type": "application/graphql+json"},
				GraphQL:  &parser.GraphQLConfig{Query: "{ me { id } }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				if req.Headers["Content-Type"] != "application/graphql+json" {
					t.Errorf("Content-Type = %q, want application/graphql+json", req.Headers["Content-Type"])
				}
			},
		},
		{
			name:    "nil graphql config returns ErrMissingConfig",
			req:     &parser.Request{URL: "https://api.example.com/graphql", Protocol: "graphql"},
			wantErr: ErrMissingConfig,
		},
		{
			name: "empty query returns ErrEmptyQuery",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL:  &parser.GraphQLConfig{Query: ""},
			},
			wantErr: ErrEmptyQuery,
		},
		{
			name: "preserves URL from input",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL:  &parser.GraphQLConfig{Query: "{ hello }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				if req.URL != "https://api.example.com/graphql" {
					t.Errorf("URL = %q, want https://api.example.com/graphql", req.URL)
				}
			},
		},
		{
			name: "body contains valid JSON with query field",
			req: &parser.Request{
				URL:      "https://api.example.com/graphql",
				Protocol: "graphql",
				GraphQL:  &parser.GraphQLConfig{Query: "{ users { id } }"},
			},
			check: func(t *testing.T, req *parser.Request) {
				bodyStr, ok := req.Body.(string)
				if !ok {
					t.Fatalf("Body type = %T, want string", req.Body)
				}
				var body RequestBody
				if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
					t.Fatalf("unmarshal body: %v", err)
				}
				if body.Query != "{ users { id } }" {
					t.Errorf("Query = %q, want { users { id } }", body.Query)
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := BuildRequest(tt.req)
			if tt.wantErr != nil {
				if !errors.Is(err, tt.wantErr) {
					t.Errorf("BuildRequest() error = %v, wantErr %v", err, tt.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("BuildRequest() unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, got)
			}
		})
	}
}

func TestCheckResponse(t *testing.T) {
	tests := []struct {
		name      string
		body      []byte
		wantCheck *ResponseCheck
		wantErr   bool
	}{
		{
			name: "success response - no errors",
			body: []byte(`{"data":{"user":{"name":"Alice"}}}`),
			wantCheck: &ResponseCheck{
				HasErrors: false,
				HasData:   true,
			},
		},
		{
			name: "full failure - errors only, data null",
			body: []byte(`{"errors":[{"message":"not found"}],"data":null}`),
			wantCheck: &ResponseCheck{
				HasErrors: true,
				HasData:   false,
				Errors:    []GraphQLError{{Message: "not found"}},
			},
		},
		{
			name: "partial success - data and errors",
			body: []byte(`{"data":{"user":{"name":"Alice"}},"errors":[{"message":"deprecated field"}]}`),
			wantCheck: &ResponseCheck{
				HasErrors: true,
				HasData:   true,
				Errors:    []GraphQLError{{Message: "deprecated field"}},
			},
		},
		{
			name:      "empty body",
			body:      []byte{},
			wantCheck: &ResponseCheck{},
		},
		{
			name:    "invalid JSON returns error",
			body:    []byte(`{not json`),
			wantErr: true,
		},
		{
			name:      "null errors array treated as no errors",
			body:      []byte(`{"data":{"ok":true},"errors":null}`),
			wantCheck: &ResponseCheck{HasData: true},
		},
		{
			name:      "empty errors array treated as no errors",
			body:      []byte(`{"data":{"ok":true},"errors":[]}`),
			wantCheck: &ResponseCheck{HasData: true},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := CheckResponse(tt.body)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("CheckResponse() unexpected error: %v", err)
			}
			if got.HasErrors != tt.wantCheck.HasErrors {
				t.Errorf("HasErrors = %v, want %v", got.HasErrors, tt.wantCheck.HasErrors)
			}
			if got.HasData != tt.wantCheck.HasData {
				t.Errorf("HasData = %v, want %v", got.HasData, tt.wantCheck.HasData)
			}
			if len(tt.wantCheck.Errors) > 0 {
				if len(got.Errors) != len(tt.wantCheck.Errors) {
					t.Fatalf("Errors count = %d, want %d", len(got.Errors), len(tt.wantCheck.Errors))
				}
				for i, e := range tt.wantCheck.Errors {
					if got.Errors[i].Message != e.Message {
						t.Errorf("Errors[%d].Message = %q, want %q", i, got.Errors[i].Message, e.Message)
					}
				}
			}
		})
	}
}

func TestBuildRequest_does_not_mutate_caller_headers(t *testing.T) {
	originalHeaders := map[string]string{"Authorization": "Bearer token123"}
	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Protocol: "graphql",
		Headers:  originalHeaders,
		GraphQL:  &parser.GraphQLConfig{Query: "{ me { id } }"},
	}

	result, err := BuildRequest(req)
	if err != nil {
		t.Fatalf("BuildRequest() error: %v", err)
	}

	// The returned request should have Content-Type
	if result.Headers["Content-Type"] != "application/json" {
		t.Errorf("result Content-Type = %q, want application/json", result.Headers["Content-Type"])
	}

	// The original headers map must NOT have been modified
	if _, ok := originalHeaders["Content-Type"]; ok {
		t.Error("BuildRequest mutated the caller's Headers map by adding Content-Type")
	}
	if len(originalHeaders) != 1 {
		t.Errorf("original headers length = %d, want 1", len(originalHeaders))
	}
}

func TestParseErrorHandling(t *testing.T) {
	tests := []struct {
		name    string
		mode    string
		want    ErrorHandlingMode
		wantErr bool
	}{
		{"empty defaults to fail", "", ErrorHandlingFail, false},
		{"explicit fail", "fail", ErrorHandlingFail, false},
		{"warn mode", "warn", ErrorHandlingWarn, false},
		{"ignore mode", "ignore", ErrorHandlingIgnore, false},
		{"invalid mode returns error", "invalid", "", true},
		{"error message mentions ignore", "bogus", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseErrorHandling(tt.mode)
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				if !errors.Is(err, ErrInvalidErrorHandling) {
					t.Errorf("expected ErrInvalidErrorHandling, got %v", err)
				}
				// error message must mention "ignore" since it is now a valid option
				if tt.name == "error message mentions ignore" && !strings.Contains(err.Error(), "ignore") {
					t.Errorf("error message should mention 'ignore', got: %v", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("ParseErrorHandling(%q) unexpected error: %v", tt.mode, err)
			}
			if got != tt.want {
				t.Errorf("ParseErrorHandling(%q) = %q, want %q", tt.mode, got, tt.want)
			}
		})
	}
}

func TestClassifyOutcome(t *testing.T) {
	tests := []struct {
		name  string
		check *ResponseCheck
		want  Outcome
	}{
		{"nil check is empty", nil, OutcomeEmpty},
		{"both absent is empty", &ResponseCheck{}, OutcomeEmpty},
		{"data only is success", &ResponseCheck{HasData: true}, OutcomeSuccess},
		{"errors only (data null) is full failure", &ResponseCheck{HasErrors: true}, OutcomeFullFailure},
		{"data + errors is partial success", &ResponseCheck{HasData: true, HasErrors: true}, OutcomePartialSuccess},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := ClassifyOutcome(tt.check); got != tt.want {
				t.Errorf("ClassifyOutcome(%+v) = %v, want %v", tt.check, got, tt.want)
			}
		})
	}
}
