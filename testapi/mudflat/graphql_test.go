package mudflat

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"
)

// GraphQL (§9.K).
//
// The family exists for one trap: a partial success arrives as HTTP 200 with
// both `data` and `errors` populated. A client that reads the status code and
// stops has passed a test that failed. Everything else here supports proving
// that curlew's `graphql:` block put on the wire what it claimed to.
//
// mudflat does not implement GraphQL. It reflects the request back inside
// `data`, which is what makes `query_file` and `fragments` — features whose
// output is otherwise invisible from the client side — assertable.

func postGraphQL(t *testing.T, client *http.Client, url string, payload any) (*http.Response, []byte) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	resp, err := client.Post(url, "application/json", bytes.NewReader(raw))
	if err != nil {
		t.Fatalf("POST %s: %v", url, err)
	}
	t.Cleanup(func() { _ = resp.Body.Close() })
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body of %s: %v", url, err)
	}
	return resp, body
}

func decodeGraphQL(t *testing.T, body []byte) graphqlResponse {
	t.Helper()
	var out graphqlResponse
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	return out
}

func TestGraphQL_ReflectsTheQueryItWasSent(t *testing.T) {
	base, client := startServer(t)

	query := "query ListUsers($limit: Int!) { users(first: $limit) { id name } }"
	_, body := postGraphQL(t, client, base+"/graphql", map[string]any{
		"query":     query,
		"variables": map[string]any{"limit": 2},
	})

	out := decodeGraphQL(t, body)
	if out.Data == nil {
		t.Fatalf("no data in response: %s", body)
	}
	if out.Data.Echo.Query != query {
		t.Errorf("echoed query = %q, want %q", out.Data.Echo.Query, query)
	}
	if out.Data.Echo.OperationName != "ListUsers" {
		t.Errorf("operation_name = %q, want ListUsers", out.Data.Echo.OperationName)
	}
	// A variable that arrives as the string "2" rather than the number 2 is a
	// serialisation defect the reflection makes visible.
	if got, ok := out.Data.Echo.Variables["limit"].(float64); !ok || got != 2 {
		t.Errorf("variables.limit = %#v, want the number 2", out.Data.Echo.Variables["limit"])
	}
	if len(out.Data.Users) != 2 {
		t.Errorf("users = %d, want 2 — the resolver is driven by the variable", len(out.Data.Users))
	}
	if len(out.Errors) != 0 {
		t.Errorf("unexpected errors: %v", out.Errors)
	}
}

func TestGraphQL_NamesTheFragmentsItReceived(t *testing.T) {
	// Fragment concatenation happens entirely inside curlew; the assembled
	// query text is the only evidence it happened, and the server is the only
	// place that text can be observed.
	base, client := startServer(t)

	query := `query ListUsers { users { ...UserFields ...Timestamps } }
fragment UserFields on User { id name }
fragment Timestamps on Node { createdAt }`

	_, body := postGraphQL(t, client, base+"/graphql", map[string]any{"query": query})
	out := decodeGraphQL(t, body)

	want := []string{"UserFields", "Timestamps"}
	if len(out.Data.Echo.FragmentNames) != len(want) {
		t.Fatalf("fragment_names = %v, want %v", out.Data.Echo.FragmentNames, want)
	}
	for i, name := range want {
		if out.Data.Echo.FragmentNames[i] != name {
			t.Errorf("fragment_names[%d] = %q, want %q", i, out.Data.Echo.FragmentNames[i], name)
		}
	}
}

func TestGraphQL_PartialSuccessIs200WithBoth(t *testing.T) {
	// The trap. HTTP 200, data present, errors present.
	base, client := startServer(t)

	resp, body := postGraphQL(t, client, base+"/graphql/partial", map[string]any{"query": "{ users { id } }"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200 — a partial success is not an HTTP error", resp.StatusCode)
	}

	out := decodeGraphQL(t, body)
	if out.Data == nil {
		t.Error("data is null; a partial success has both")
	}
	if len(out.Errors) == 0 {
		t.Error("no errors; a partial success has both")
	}
	if len(out.Errors) > 0 && out.Errors[0].Message == "" {
		t.Error("error carries no message")
	}
	if len(out.Errors) > 0 && len(out.Errors[0].Path) == 0 {
		t.Error("error carries no path; a client cannot say which field failed")
	}
}

func TestGraphQL_FullFailureIs200WithNullData(t *testing.T) {
	base, client := startServer(t)

	resp, body := postGraphQL(t, client, base+"/graphql/errors", map[string]any{"query": "{ users { id } }"})
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// data must be present as an explicit null, not absent: the two are
	// different documents and $.data is addressable only in the first.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(body, &raw); err != nil {
		t.Fatalf("decode: %v", err)
	}
	data, ok := raw["data"]
	if !ok {
		t.Fatal("no data key at all; a full failure carries data: null")
	}
	if string(data) != "null" {
		t.Errorf("data = %s, want null", data)
	}
	out := decodeGraphQL(t, body)
	if len(out.Errors) == 0 {
		t.Error("no errors on a full failure")
	}
}

func TestGraphQL_IntrospectionAnswersTheStandardQuery(t *testing.T) {
	base, client := startServer(t)

	_, body := postGraphQL(t, client, base+"/graphql/introspection",
		map[string]any{"query": "query IntrospectionQuery { __schema { queryType { name } } }"})

	var out struct {
		Data struct {
			Schema struct {
				QueryType struct {
					Name string `json:"name"`
				} `json:"queryType"`
				Types []struct {
					Name   string `json:"name"`
					Kind   string `json:"kind"`
					Fields []struct {
						Name string `json:"name"`
					} `json:"fields"`
				} `json:"types"`
			} `json:"__schema"`
		} `json:"data"`
	}
	if err := json.Unmarshal(body, &out); err != nil {
		t.Fatalf("decode: %v (%s)", err, body)
	}
	if out.Data.Schema.QueryType.Name != "Query" {
		t.Errorf("queryType.name = %q, want Query", out.Data.Schema.QueryType.Name)
	}
	if len(out.Data.Schema.Types) == 0 {
		t.Error("__schema.types is empty; nothing to introspect")
	}
}

func TestGraphQL_HTTPErrorIsNotAGraphQLDocument(t *testing.T) {
	// A 500 whose body is not GraphQL at all — the case where a client that
	// assumes every response parses as {data, errors} falls over.
	base, client := startServer(t)

	resp, body := postGraphQL(t, client, base+"/graphql/http-error", map[string]any{"query": "{ users { id } }"})
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", resp.StatusCode)
	}
	if ct := resp.Header.Get("Content-Type"); !strings.HasPrefix(ct, "text/html") {
		t.Errorf("Content-Type = %q, want text/html — the point is that it is not JSON", ct)
	}
	if json.Valid(body) {
		t.Errorf("body parses as JSON (%s); it must not", body)
	}
}

func TestGraphQL_RejectsANonPOSTMethod(t *testing.T) {
	base, client := startServer(t)
	resp, _ := get(t, client, base+"/graphql")
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

func TestGraphQL_RejectsAMissingQuery(t *testing.T) {
	base, client := startServer(t)
	resp, body := postGraphQL(t, client, base+"/graphql", map[string]any{"variables": map[string]any{}})
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (%s)", resp.StatusCode, body)
	}
}
