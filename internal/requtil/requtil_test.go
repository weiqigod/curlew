package requtil

import (
	"strings"
	"testing"

	"github.com/weiqigod/curlew/internal/parser"
	"github.com/weiqigod/curlew/internal/variable"
)

func TestInterpolateRequest_graphql_nil(t *testing.T) {
	scope := variable.NewScope(map[string]string{"host": "example.com"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:    "https://{{host}}/api",
		Method: "GET",
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.GraphQL != nil {
		t.Error("expected GraphQL to remain nil")
	}
	if result.URL != "https://example.com/api" {
		t.Errorf("URL = %q, want %q", result.URL, "https://example.com/api")
	}
}

func TestInterpolateRequest_graphql_query_interpolation(t *testing.T) {
	scope := variable.NewScope(map[string]string{"type_name": "User"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query: "query { {{type_name}} { id name } }",
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.GraphQL == nil {
		t.Fatal("expected GraphQL to be non-nil")
	}
	expected := "query { User { id name } }"
	if result.GraphQL.Query != expected {
		t.Errorf("Query = %q, want %q", result.GraphQL.Query, expected)
	}
}

func TestInterpolateRequest_graphql_variables_interpolation(t *testing.T) {
	scope := variable.NewScope(map[string]string{"user_id": "42"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query:     "query GetUser($id: ID!) { user(id: $id) { name } }",
			Variables: map[string]any{"id": "{{user_id}}", "limit": 10},
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.GraphQL == nil {
		t.Fatal("expected GraphQL to be non-nil")
	}
	if result.GraphQL.Variables["id"] != "42" {
		t.Errorf("Variables[id] = %v, want %q", result.GraphQL.Variables["id"], "42")
	}
	// Non-string values pass through unchanged
	if result.GraphQL.Variables["limit"] != 10 {
		t.Errorf("Variables[limit] = %v, want 10", result.GraphQL.Variables["limit"])
	}
}

func TestInterpolateRequest_graphql_empty_variables(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query: "{ hello }",
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.GraphQL == nil {
		t.Fatal("expected GraphQL to be non-nil")
	}
	if result.GraphQL.Variables != nil {
		t.Errorf("expected nil Variables, got %v", result.GraphQL.Variables)
	}
}

func TestInterpolateRequest_graphql_query_interpolation_error(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query: "query { {{undefined_var}} { id } }",
		},
	}

	_, err := InterpolateRequest(scope, req)
	if err == nil {
		t.Fatal("expected error for undefined variable in graphql query")
	}
}

func TestInterpolateRequest_graphql_variable_interpolation_error(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query:     "query GetUser($id: ID!) { user(id: $id) { name } }",
			Variables: map[string]any{"id": "{{undefined_var}}"},
		},
	}

	_, err := InterpolateRequest(scope, req)
	if err == nil {
		t.Fatal("expected error for undefined variable in graphql variables")
	}
}

func TestInterpolateRequest_graphql_does_not_mutate_input(t *testing.T) {
	scope := variable.NewScope(map[string]string{"val": "resolved"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	originalVars := map[string]any{"key": "{{val}}"}
	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query:     "{ hello }",
			Variables: originalVars,
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}

	// Result should have resolved value
	if result.GraphQL.Variables["key"] != "resolved" {
		t.Errorf("result Variables[key] = %v, want %q", result.GraphQL.Variables["key"], "resolved")
	}

	// Original should be unchanged
	if originalVars["key"] != "{{val}}" {
		t.Errorf("original Variables[key] = %v, want %q", originalVars["key"], "{{val}}")
	}
}

func TestInterpolateRequest_websocket_steps_interpolate_message_raw(t *testing.T) {
	scope := variable.NewScope(map[string]string{"token": "abc123"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "ws://example.com/ws",
		Method:   "WS",
		Protocol: "websocket",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{Action: "send", MessageRaw: "auth {{token}}"},
			},
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.WebSocket == nil {
		t.Fatal("expected WebSocket to be non-nil")
	}
	got := result.WebSocket.Steps[0].MessageRaw
	if got != "auth abc123" {
		t.Errorf("MessageRaw = %q, want %q", got, "auth abc123")
	}
	// Original must remain untouched
	if req.WebSocket.Steps[0].MessageRaw != "auth {{token}}" {
		t.Errorf("input mutated: got %q", req.WebSocket.Steps[0].MessageRaw)
	}
}

func TestInterpolateRequest_websocket_steps_interpolate_message_map(t *testing.T) {
	scope := variable.NewScope(map[string]string{"channel": "chat", "user": "alice"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "ws://example.com/ws",
		Method:   "WS",
		Protocol: "websocket",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{
					Action: "send",
					Message: map[string]any{
						"type":    "subscribe",
						"channel": "{{channel}}",
						"meta": map[string]any{
							"user": "{{user}}",
						},
						"tags": []any{"{{channel}}-a", "{{channel}}-b"},
					},
				},
			},
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	msg := result.WebSocket.Steps[0].Message
	if msg["channel"] != "chat" {
		t.Errorf("Message[channel] = %v, want chat", msg["channel"])
	}
	meta, ok := msg["meta"].(map[string]any)
	if !ok {
		t.Fatalf("meta not a map: %T", msg["meta"])
	}
	if meta["user"] != "alice" {
		t.Errorf("meta[user] = %v, want alice", meta["user"])
	}
	tags, ok := msg["tags"].([]any)
	if !ok {
		t.Fatalf("tags not a slice: %T", msg["tags"])
	}
	if tags[0] != "chat-a" || tags[1] != "chat-b" {
		t.Errorf("tags = %v, want [chat-a chat-b]", tags)
	}
	// Input must not be mutated
	if req.WebSocket.Steps[0].Message["channel"] != "{{channel}}" {
		t.Errorf("input mutated: %v", req.WebSocket.Steps[0].Message["channel"])
	}
}

func TestInterpolateRequest_websocket_steps_interpolate_reason_and_extract(t *testing.T) {
	scope := variable.NewScope(map[string]string{"why": "bye", "path": "token"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "ws://example.com/ws",
		Method:   "WS",
		Protocol: "websocket",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{
					Action:  "expect",
					Extract: map[string]string{"tok": "$.{{path}}"},
				},
				{Action: "close", Reason: "{{why}}"},
			},
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if got := result.WebSocket.Steps[0].Extract["tok"]; got != "$.token" {
		t.Errorf("Extract[tok] = %q, want $.token", got)
	}
	if got := result.WebSocket.Steps[1].Reason; got != "bye" {
		t.Errorf("Reason = %q, want bye", got)
	}
	// Input must not be mutated
	if req.WebSocket.Steps[0].Extract["tok"] != "$.{{path}}" {
		t.Errorf("input mutated: %v", req.WebSocket.Steps[0].Extract["tok"])
	}
}

func TestInterpolateRequest_websocket_undefined_variable_errors(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "ws://example.com/ws",
		Method:   "WS",
		Protocol: "websocket",
		WebSocket: &parser.WebSocketConfig{
			Steps: []parser.WebSocketStep{
				{Action: "send", MessageRaw: "{{missing}}"},
			},
		},
	}

	if _, err := InterpolateRequest(scope, req); err == nil {
		t.Fatal("expected error for undefined variable in websocket step")
	}
}

func TestInterpolateRequest_graphql_preserves_error_handling(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			Query:         "{ hello }",
			ErrorHandling: "warn",
		},
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if result.GraphQL.ErrorHandling != "warn" {
		t.Errorf("ErrorHandling = %q, want %q", result.GraphQL.ErrorHandling, "warn")
	}
}

// TestInterpolateRequest_graphql_query_from_file is a regression guard for
// M2-030: when a GraphQL query is loaded from an external .graphql file by
// the parser, {{var}} placeholders inside the loaded content must still be
// interpolated by the runner pipeline.
func TestInterpolateRequest_graphql_query_from_file(t *testing.T) {
	scope := variable.NewScope(map[string]string{"user_id": "abc-123"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:      "https://api.example.com/graphql",
		Method:   "POST",
		Protocol: "graphql",
		GraphQL: &parser.GraphQLConfig{
			// Simulates a query loaded from an external file that contains
			// a {{var}} placeholder.
			Query: "query { user(id: \"{{user_id}}\") { id } }",
		},
	}

	out, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest error: %v", err)
	}
	if strings.Contains(out.GraphQL.Query, "{{user_id}}") {
		t.Errorf("placeholder not interpolated: %q", out.GraphQL.Query)
	}
	if !strings.Contains(out.GraphQL.Query, "abc-123") {
		t.Errorf("value not substituted: %q", out.GraphQL.Query)
	}
}

func TestInterpolateRequest_body_file_text_interpolates(t *testing.T) {
	scope := variable.NewScope(map[string]string{"user_id": "u-42"})
	if err := scope.Resolve(); err != nil {
		t.Fatalf("Resolve: %v", err)
	}

	req := &parser.Request{
		URL:                 "https://example.com/api",
		Method:              "POST",
		BodyFile:            "payload.json",
		BodyFileContent:     `{"user":"{{user_id}}"}`,
		BodyFileContentType: "application/json",
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	got, ok := result.Body.(string)
	if !ok {
		t.Fatalf("Body = %T, want string", result.Body)
	}
	if got != `{"user":"u-42"}` {
		t.Errorf("Body = %q, want %q", got, `{"user":"u-42"}`)
	}
	if result.Headers["Content-Type"] != "application/json" {
		t.Errorf("Content-Type header = %q, want application/json", result.Headers["Content-Type"])
	}
}

func TestInterpolateRequest_body_file_explicit_header_wins(t *testing.T) {
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}

	req := &parser.Request{
		URL:                 "https://example.com/api",
		Method:              "POST",
		Headers:             map[string]string{"content-type": "application/vnd.custom+json"},
		BodyFile:            "payload.json",
		BodyFileContent:     `{"ok":true}`,
		BodyFileContentType: "application/json",
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	// Explicit header wins (case-insensitive match); no duplicate added.
	if _, ok := result.Headers["Content-Type"]; ok {
		t.Errorf("should not add canonical Content-Type when explicit case-insensitive variant is present; headers=%v", result.Headers)
	}
	if result.Headers["content-type"] != "application/vnd.custom+json" {
		t.Errorf("explicit Content-Type clobbered, got %q", result.Headers["content-type"])
	}
}

func TestInterpolateRequest_body_binary_file_passes_through(t *testing.T) {
	scope := variable.NewScope(map[string]string{"user_id": "u-42"})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}

	// Placeholders in the payload must NOT be interpolated for the binary variant.
	rawBytes := []byte{0x00, 0xFF, '{', '{', 'u', 's', 'e', 'r', '_', 'i', 'd', '}', '}', 0x01}
	req := &parser.Request{
		URL:                   "https://example.com/upload",
		Method:                "POST",
		BodyBinaryFile:        "payload.bin",
		BodyBinaryFileContent: rawBytes,
		BodyFileContentType:   "application/octet-stream",
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	got, ok := result.Body.([]byte)
	if !ok {
		t.Fatalf("Body = %T, want []byte", result.Body)
	}
	if string(got) != string(rawBytes) {
		t.Errorf("Body = %x, want %x (no interpolation for binary)", got, rawBytes)
	}
	if result.Headers["Content-Type"] != "application/octet-stream" {
		t.Errorf("Content-Type = %q, want application/octet-stream", result.Headers["Content-Type"])
	}
}

func TestInterpolateRequest_body_file_does_not_mutate_input(t *testing.T) {
	scope := variable.NewScope(map[string]string{"id": "x"})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}

	req := &parser.Request{
		URL:                 "https://example.com/api",
		Method:              "POST",
		Headers:             map[string]string{"X-Custom": "v"},
		BodyFile:            "p.json",
		BodyFileContent:     `{"id":"{{id}}"}`,
		BodyFileContentType: "application/json",
	}

	_, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatalf("InterpolateRequest: %v", err)
	}
	if _, injected := req.Headers["Content-Type"]; injected {
		t.Errorf("input headers mutated: %v", req.Headers)
	}
	if req.Body != nil {
		t.Errorf("input Body mutated: %#v", req.Body)
	}
}

func TestInterpolateRequest_body_binary_buffer_isolation(t *testing.T) {
	// Mutating the returned body must not reach back into the parser's source.
	scope := variable.NewScope(map[string]string{})
	if err := scope.Resolve(); err != nil {
		t.Fatal(err)
	}
	original := []byte{1, 2, 3, 4, 5}
	req := &parser.Request{
		URL:                   "https://example.com/",
		Method:                "POST",
		BodyBinaryFile:        "p.bin",
		BodyBinaryFileContent: original,
		BodyFileContentType:   "application/octet-stream",
	}

	result, err := InterpolateRequest(scope, req)
	if err != nil {
		t.Fatal(err)
	}
	out := result.Body.([]byte)
	out[0] = 0xFF
	if original[0] != 1 {
		t.Errorf("parser source mutated through shared slice: original[0]=%d", original[0])
	}
}
