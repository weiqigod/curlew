package mudflat

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// GraphQL (§9.K).
//
// This family exists for one trap: a partial success arrives as HTTP 200 with
// both `data` and `errors` populated. A client that reads the status code and
// stops has just passed a test that failed. curlew's `error_handling:
// fail|warn|ignore` is the setting that decides, and until now nothing had put
// the case in front of it.
//
// mudflat does not implement GraphQL, and should not: §16 deletes any endpoint
// that cannot say which curlew behaviour it exercises, and a query engine would
// exercise the engine. What it does instead is reflect the request back inside
// `data`. That turns `query_file` and `fragments` — whose whole output is the
// assembled query text, which the client can otherwise never see — into
// something a collection can assert on.

// graphqlRequest is the standard POST body: {"query", "variables",
// "operationName"}.
type graphqlRequest struct {
	Query         string         `json:"query"`
	Variables     map[string]any `json:"variables,omitempty"`
	OperationName string         `json:"operationName,omitempty"`
}

// graphqlResponse is the response document. Data is a pointer so a full failure
// can serialise `data: null` rather than omitting the key: the two are different
// documents, and `$.data` is addressable only in the first.
type graphqlResponse struct {
	Data   *graphqlData   `json:"data"`
	Errors []graphqlError `json:"errors,omitempty"`
}

type graphqlData struct {
	Echo  graphqlEcho   `json:"echo"`
	Users []graphqlUser `json:"users"`
}

// graphqlEcho is what makes the family assertable from a collection.
type graphqlEcho struct {
	Query         string         `json:"query"`
	OperationName string         `json:"operation_name"`
	Variables     map[string]any `json:"variables"`
	FragmentNames []string       `json:"fragment_names"`
	QueryBytes    int            `json:"query_bytes"`
}

type graphqlUser struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

type graphqlError struct {
	Message   string         `json:"message"`
	Path      []string       `json:"path,omitempty"`
	Locations []graphqlLoc   `json:"locations,omitempty"`
	Ext       map[string]any `json:"extensions,omitempty"`
}

type graphqlLoc struct {
	Line   int `json:"line"`
	Column int `json:"column"`
}

// operationNamePattern and fragmentPattern read the query text. Deliberately
// lexical rather than a parser: mudflat is not a GraphQL implementation, and a
// regex that fails on an exotic query is a better outcome than a half-parser
// that silently disagrees with a real server.
var (
	operationNamePattern = regexp.MustCompile(`(?m)^\s*(?:query|mutation|subscription)\s+([A-Za-z_][A-Za-z0-9_]*)`)
	fragmentPattern      = regexp.MustCompile(`(?m)^\s*fragment\s+([A-Za-z_][A-Za-z0-9_]*)\s+on\s`)
)

// mudflatUsers is the canned resolver result. Fixed, because §6.1 makes
// determinism an invariant: two runs must produce byte-identical output.
var mudflatUsers = []graphqlUser{
	{ID: "usr_1", Name: "Ada Lovelace", Email: "ada@mudflat.test"},
	{ID: "usr_2", Name: "Grace Hopper", Email: "grace@mudflat.test"},
	{ID: "usr_3", Name: "Karen Spärck Jones", Email: "karen@mudflat.test"},
	{ID: "usr_4", Name: "Barbara Liskov", Email: "barbara@mudflat.test"},
	{ID: "usr_5", Name: "Radia Perlman", Email: "radia@mudflat.test"},
}

func (s *Server) registerGraphQL() {
	s.register(Endpoint{
		Pattern:   "/graphql",
		Methods:   []string{http.MethodPost},
		Family:    "K",
		Summary:   "Reflects the GraphQL request inside data, and resolves users from the variables.",
		Exercises: "The graphql protocol adapter: that query, variables and operationName reach the wire as sent, that a variable stays a number, and that fragments were concatenated — the assembled query text is the only evidence of query_file and fragments, and only the server can see it.",
		Handler:   s.handleGraphQL,
	})

	s.register(Endpoint{
		Pattern:   "/graphql/partial",
		Methods:   []string{http.MethodPost},
		Family:    "K",
		Summary:   "HTTP 200 with both data and errors populated.",
		Exercises: "error_handling: fail|warn|ignore against a partial success. This is the trap the family exists for: a client that reads the status code and stops passes a test that failed.",
		Handler:   s.handleGraphQLPartial,
	})

	s.register(Endpoint{
		Pattern:   "/graphql/errors",
		Methods:   []string{http.MethodPost},
		Family:    "K",
		Summary:   "HTTP 200 with data: null and errors populated.",
		Exercises: "The full-failure row of the error_handling table, and whether $.data is addressable as an explicit null rather than absent.",
		Handler:   s.handleGraphQLErrors,
	})

	s.register(Endpoint{
		Pattern:   "/graphql/introspection",
		Methods:   []string{http.MethodPost},
		Family:    "K",
		Summary:   "A small but well-formed introspection response.",
		Exercises: "Deep JSONPath assertions over a large nested document, and body assertions against __schema — a field name whose leading underscores make it awkward to address.",
		Handler:   s.handleGraphQLIntrospection,
	})

	s.register(Endpoint{
		Pattern:   "/graphql/http-error",
		Methods:   []string{http.MethodPost},
		Family:    "K",
		Summary:   "500 with an HTML body — not a GraphQL document at all.",
		Exercises: "Whether the adapter assumes every response parses as {data, errors}. A gateway returning its own error page is the common real-world case.",
		Handler:   s.handleGraphQLHTTPError,
	})
}

// decodeGraphQLRequest reads and validates the POST body.
func decodeGraphQLRequest(w http.ResponseWriter, r *http.Request) (graphqlRequest, bool) {
	var req graphqlRequest
	body := bodyFromContext(r.Context())
	if err := json.Unmarshal(body, &req); err != nil {
		writeProblem(w, http.StatusBadRequest,
			"request body is not valid JSON: a GraphQL request is {\"query\": ..., \"variables\": ...}")
		return req, false
	}
	if strings.TrimSpace(req.Query) == "" {
		writeProblem(w, http.StatusBadRequest,
			"no query in the request body; the graphql: block must produce {\"query\": ...}")
		return req, false
	}
	return req, true
}

// echoOf builds the reflection block.
func echoOf(req graphqlRequest) graphqlEcho {
	name := req.OperationName
	if name == "" {
		if m := operationNamePattern.FindStringSubmatch(req.Query); m != nil {
			name = m[1]
		}
	}
	fragments := []string{}
	for _, m := range fragmentPattern.FindAllStringSubmatch(req.Query, -1) {
		fragments = append(fragments, m[1])
	}
	vars := req.Variables
	if vars == nil {
		vars = map[string]any{}
	}
	return graphqlEcho{
		Query:         req.Query,
		OperationName: name,
		Variables:     vars,
		FragmentNames: fragments,
		QueryBytes:    len(req.Query),
	}
}

// usersFor resolves the canned list, bounded by a `limit` variable when one is
// present. A limit that arrived as the string "2" does not match float64 and
// yields the full list, which is what makes the defect visible.
func usersFor(req graphqlRequest) []graphqlUser {
	limit := len(mudflatUsers)
	if raw, ok := req.Variables["limit"]; ok {
		if n, isNumber := raw.(float64); isNumber && n >= 0 && int(n) < limit {
			limit = int(n)
		}
	}
	return mudflatUsers[:limit]
}

func (s *Server) handleGraphQL(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGraphQLRequest(w, r)
	if !ok {
		return
	}
	writeJSON(w, http.StatusOK, graphqlResponse{
		Data: &graphqlData{Echo: echoOf(req), Users: usersFor(req)},
	})
}

func (s *Server) handleGraphQLPartial(w http.ResponseWriter, r *http.Request) {
	req, ok := decodeGraphQLRequest(w, r)
	if !ok {
		return
	}
	// 200. Data resolved. One field failed. Every one of those three is true at
	// once, which is the whole point.
	writeJSON(w, http.StatusOK, graphqlResponse{
		Data: &graphqlData{Echo: echoOf(req), Users: usersFor(req)},
		Errors: []graphqlError{{
			Message:   "Cannot resolve field 'email' for user usr_3: upstream directory timed out",
			Path:      []string{"users", "2", "email"},
			Locations: []graphqlLoc{{Line: 1, Column: 24}},
			Ext:       map[string]any{"code": "UPSTREAM_TIMEOUT", "classification": "DataFetchingException"},
		}},
	})
}

func (s *Server) handleGraphQLErrors(w http.ResponseWriter, r *http.Request) {
	if _, ok := decodeGraphQLRequest(w, r); !ok {
		return
	}
	// Data is an explicit null, not an omitted key.
	writeJSON(w, http.StatusOK, graphqlResponse{
		Data: nil,
		Errors: []graphqlError{{
			Message:   "Variable '$limit' of required type 'Int!' was not provided",
			Locations: []graphqlLoc{{Line: 1, Column: 7}},
			Ext:       map[string]any{"code": "GRAPHQL_VALIDATION_FAILED"},
		}},
	})
}

func (s *Server) handleGraphQLIntrospection(w http.ResponseWriter, r *http.Request) {
	if _, ok := decodeGraphQLRequest(w, r); !ok {
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"data": map[string]any{
			"__schema": map[string]any{
				"queryType":        map[string]any{"name": "Query"},
				"mutationType":     map[string]any{"name": "Mutation"},
				"subscriptionType": nil,
				"types": []any{
					map[string]any{
						"kind":        "OBJECT",
						"name":        "Query",
						"description": "The root query type.",
						"fields": []any{
							map[string]any{
								"name":        "users",
								"description": "Every user, optionally bounded by first.",
								"args": []any{
									map[string]any{
										"name": "first",
										"type": map[string]any{"kind": "SCALAR", "name": "Int"},
									},
								},
								"type": map[string]any{
									"kind": "LIST",
									"name": nil,
									"ofType": map[string]any{
										"kind": "OBJECT", "name": "User",
									},
								},
							},
						},
					},
					map[string]any{
						"kind":        "OBJECT",
						"name":        "User",
						"description": "A person.",
						"fields": []any{
							map[string]any{"name": "id", "type": map[string]any{"kind": "SCALAR", "name": "ID"}},
							map[string]any{"name": "name", "type": map[string]any{"kind": "SCALAR", "name": "String"}},
							map[string]any{"name": "email", "type": map[string]any{"kind": "SCALAR", "name": "String"}},
						},
					},
					map[string]any{"kind": "SCALAR", "name": "Int", "fields": nil},
					map[string]any{"kind": "SCALAR", "name": "String", "fields": nil},
					map[string]any{"kind": "SCALAR", "name": "ID", "fields": nil},
				},
			},
		},
	})
}

func (s *Server) handleGraphQLHTTPError(w http.ResponseWriter, _ *http.Request) {
	// A gateway's own error page. Not JSON, not GraphQL, and the Content-Type
	// says so — a client that unmarshals every response into {data, errors}
	// must produce a legible error rather than a nil-pointer walk.
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(http.StatusInternalServerError)
	_, _ = w.Write([]byte("<html><head><title>502 Bad Gateway</title></head>\n" +
		"<body><h1>502 Bad Gateway</h1>\n" +
		"<p>The upstream GraphQL service did not respond. " +
		"This body is HTML on purpose: not every 500 is a GraphQL document.</p>\n" +
		"</body></html>\n"))
}
