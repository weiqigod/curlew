# Implementation Plan: M1-002

## Overview

Extend the parser and HTTP executor to support all standard HTTP methods (POST, PUT, DELETE, PATCH, HEAD, OPTIONS), request headers, JSON body serialization, string body passthrough, and query parameter handling — making the CLI capable of testing real-world API endpoints.

## Task Details
- **ID:** M1-002
- **Title:** All HTTP methods, headers, query params, JSON body
- **Phase:** M1: Core CLI
- **Priority:** 2
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | Run a single GET request from a collection file | done |

## Current Code State

### `internal/parser/collection.go`
```go
type Request struct {
    Method  string            `yaml:"method"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers,omitempty"`
}
```

### `internal/httpexec/executor.go`
```go
func Execute(ctx context.Context, req *parser.Request) (*Result, error) {
    httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, nil)
    // ... sets headers, sends request, returns status + duration
}
```

**Key observations:**
- `Request.Headers` already exists and is applied in `Execute()` via `Header.Set`
- `Request.Method` is used but has no validation — any string is passed through
- No `Body` or `QueryParams` fields exist yet
- Executor passes `nil` as body to `http.NewRequestWithContext`
- No default method logic — empty method would cause `net/http` error

## Implementation Steps

### Step 1: Method Validation and Normalization in Parser
**Rationale:** Purely additive to parser, touches nothing else. Smallest blast radius — establishes the sentinel error and validation logic that all subsequent steps rely on.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/errors.go` | modify | Add `ErrUnsupportedMethod` sentinel |
| `internal/parser/parser.go` | modify | Add `normalizeAndValidateMethod` helper; call in `ParseFile` post-unmarshal loop |
| `internal/parser/parser_test.go` | modify | Add test cases for method validation, normalization, and defaulting |
| `internal/parser/testdata/no_method.yaml` | create | Fixture: request with no method field |
| `internal/parser/testdata/unsupported_method.yaml` | create | Fixture: request with `method: FOOBAR` |
| `internal/parser/testdata/post_lowercase.yaml` | create | Fixture: request with `method: post` |

#### Current Code (`parser.go` — end of `ParseFile`)
```go
    if col.Name == "" {
        return nil, ErrEmptyCollection
    }

    return &col, nil
```

#### New Code (`parser.go`)
```go
    if col.Name == "" {
        return nil, ErrEmptyCollection
    }

    for i := range col.Requests {
        method, err := normalizeAndValidateMethod(col.Requests[i].Request.Method)
        if err != nil {
            return nil, err
        }
        col.Requests[i].Request.Method = method
    }

    return &col, nil
```

#### New Helper (`parser.go`)
```go
var allowedMethods = map[string]bool{
    "GET": true, "POST": true, "PUT": true, "DELETE": true,
    "PATCH": true, "HEAD": true, "OPTIONS": true,
}

func normalizeAndValidateMethod(method string) (string, error) {
    if method == "" {
        return "GET", nil
    }
    upper := strings.ToUpper(method)
    if !allowedMethods[upper] {
        return "", fmt.Errorf("%w: %s", ErrUnsupportedMethod, method)
    }
    return upper, nil
}
```

#### New Sentinel (`errors.go`)
```go
ErrUnsupportedMethod = errors.New("unsupported HTTP method")
```

#### Tests to Write FIRST (RED phase)

```go
// Add to TestParseFile table in parser_test.go:
{
    name: "method defaults to GET when empty",
    file: "testdata/no_method.yaml",
    wantCol: &Collection{
        Name: "No Method Test",
        Requests: []RequestItem{{
            Name:    "Default Method",
            Request: Request{Method: "GET", URL: "https://example.com"},
        }},
    },
},
{
    name: "method is normalized to uppercase",
    file: "testdata/post_lowercase.yaml",
    wantCol: &Collection{
        Name: "Lowercase Method Test",
        Requests: []RequestItem{{
            Name:    "Post Lowercase",
            Request: Request{Method: "POST", URL: "https://example.com"},
        }},
    },
},
{
    name:    "unsupported method returns error",
    file:    "testdata/unsupported_method.yaml",
    wantErr: ErrUnsupportedMethod,
},
```

#### Impact on Existing Tests
- `"valid minimal collection"` — fixture has `method: GET`, normalized to `"GET"`. **No breakage.**
- `"collection with headers"` — fixture has `method: GET`. **No breakage.**

---

### Step 2: Add `Body` and `QueryParams` Fields to Request
**Rationale:** Structural change to the shared type. Must come before executor changes that consume these fields. Adding both at once since they are independent fields on the same struct.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Body any` and `QueryParams map[string]string` fields |
| `internal/parser/parser_test.go` | modify | Add test cases for body and query parsing |
| `internal/parser/testdata/with_body_map.yaml` | create | POST request with YAML map body |
| `internal/parser/testdata/with_body_string.yaml` | create | POST request with string body |
| `internal/parser/testdata/with_query.yaml` | create | Request with query params |

#### Current Code (`collection.go`)
```go
type Request struct {
    Method  string            `yaml:"method"`
    URL     string            `yaml:"url"`
    Headers map[string]string `yaml:"headers,omitempty"`
}
```

#### New Code (`collection.go`)
```go
type Request struct {
    Method      string            `yaml:"method"`
    URL         string            `yaml:"url"`
    Headers     map[string]string `yaml:"headers,omitempty"`
    Body        any               `yaml:"body,omitempty"`
    QueryParams map[string]string `yaml:"query,omitempty"`
}
```

**Design decisions:**
- `Body` is `any` because YAML can unmarshal to `map[string]interface{}` (map body) or `string` (raw body). The executor type-switches at runtime.
- `QueryParams` uses YAML tag `query` (shorter, matches task description). Go field named `QueryParams` for clarity.
- Both use `omitempty` — zero values (nil) mean "not present".

#### Tests to Write FIRST (RED phase)

```go
// Add to TestParseFile table:
{
    name: "body as YAML map",
    file: "testdata/with_body_map.yaml",
    // wantCol checks Body is map[string]interface{}{"email": "test@example.com", "name": "Test"}
},
{
    name: "body as string",
    file: "testdata/with_body_string.yaml",
    // wantCol checks Body is "raw string body"
},
{
    name: "query params parsed",
    file: "testdata/with_query.yaml",
    // wantCol checks QueryParams == map[string]string{"page": "1", "limit": "10"}
},
```

Note: Body comparison with `reflect.DeepEqual` works for `any` types since YAML consistently unmarshals maps to `map[string]interface{}`.

#### Impact on Existing Tests
- All existing `Request` literals use `{Method: "GET", URL: "..."}` or `{Method: "GET", URL: "...", Headers: {...}}`. New fields zero-value to `nil`, which is correct. **No breakage.**

---

### Step 3: Body Serialization in Executor
**Rationale:** Implements the core body-handling logic. Depends on Step 2 for the `Body` field. Isolated to `internal/httpexec/` — doesn't change the parser.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor.go` | modify | Add `prepareBody` helper; wire into `Execute` |
| `internal/httpexec/executor_test.go` | modify | Add body-specific test cases |

#### New Helper (`executor.go`)
```go
func prepareBody(body any) (io.Reader, string, error) {
    if body == nil {
        return nil, "", nil
    }
    switch b := body.(type) {
    case string:
        return strings.NewReader(b), "", nil
    default:
        data, err := json.Marshal(b)
        if err != nil {
            return nil, "", fmt.Errorf("serializing request body to JSON: %w", err)
        }
        return bytes.NewReader(data), "application/json", nil
    }
}
```

#### Updated `Execute` (body portion)
```go
func Execute(ctx context.Context, req *parser.Request) (*Result, error) {
    // ... (query params handled in Step 4)

    bodyReader, contentType, err := prepareBody(req.Body)
    if err != nil {
        return nil, fmt.Errorf("preparing request body: %w", err)
    }

    httpReq, err := http.NewRequestWithContext(ctx, req.Method, req.URL, bodyReader)
    // ...

    for k, v := range req.Headers {
        httpReq.Header.Set(k, v)
    }

    // Auto-set Content-Type for JSON bodies if not already set by user
    if contentType != "" && httpReq.Header.Get("Content-Type") == "" {
        httpReq.Header.Set("Content-Type", contentType)
    }
    // ... rest unchanged
```

**Content-Type behavior:**
- Map body + no user Content-Type → auto-set `application/json`
- Map body + user-set Content-Type → respect user's (still JSON-serialize)
- String body → no auto Content-Type
- Nil body → nothing

#### Tests to Write FIRST (RED phase)

```go
// Add to TestExecute table in executor_test.go:
{
    name: "POST with JSON body sends correct Content-Type",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Content-Type") != "application/json" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req:        &parser.Request{Method: "POST", Body: map[string]interface{}{"key": "value"}},
    wantStatus: 200,
},
{
    name: "POST with JSON body sends correct JSON",
    handler: func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        var m map[string]interface{}
        if json.Unmarshal(body, &m) != nil || m["key"] != "value" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req:        &parser.Request{Method: "POST", Body: map[string]interface{}{"key": "value"}},
    wantStatus: 200,
},
{
    name: "POST with string body sends raw string",
    handler: func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        if string(body) != "raw body content" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req:        &parser.Request{Method: "POST", Body: "raw body content"},
    wantStatus: 200,
},
{
    name: "POST with string body does not auto-set Content-Type",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Content-Type") != "" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req:        &parser.Request{Method: "POST", Body: "raw body"},
    wantStatus: 200,
},
{
    name: "POST with map body and explicit Content-Type preserves user header",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.Header.Get("Content-Type") != "application/vnd.api+json" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req: &parser.Request{
        Method:  "POST",
        Body:    map[string]interface{}{"key": "value"},
        Headers: map[string]string{"Content-Type": "application/vnd.api+json"},
    },
    wantStatus: 200,
},
{
    name: "nested JSON body is correctly serialized",
    handler: func(w http.ResponseWriter, r *http.Request) {
        body, _ := io.ReadAll(r.Body)
        var m map[string]interface{}
        if json.Unmarshal(body, &m) != nil {
            w.WriteHeader(400)
            return
        }
        user, ok := m["user"].(map[string]interface{})
        if !ok || user["name"] != "Test" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req: &parser.Request{
        Method: "POST",
        Body:   map[string]interface{}{"user": map[string]interface{}{"name": "Test"}},
    },
    wantStatus: 200,
},
```

#### Impact on Existing Tests
- Existing tests pass `nil` Body (zero value of `any`). `prepareBody(nil)` returns `nil, "", nil` — same as the current `nil` hardcode. **No breakage.**

---

### Step 4: Query Parameter Handling in Executor
**Rationale:** Independent of body handling. Depends on Step 2 for the `QueryParams` field. Affects URL construction in `Execute`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor.go` | modify | Add `applyQueryParams` helper; wire into `Execute` before request creation |
| `internal/httpexec/executor_test.go` | modify | Add query param test cases |

#### New Helper (`executor.go`)
```go
func applyQueryParams(rawURL string, params map[string]string) (string, error) {
    if len(params) == 0 {
        return rawURL, nil
    }
    u, err := url.Parse(rawURL)
    if err != nil {
        return "", fmt.Errorf("parsing URL: %w", err)
    }
    q := u.Query()
    for k, v := range params {
        q.Set(k, v)
    }
    u.RawQuery = q.Encode()
    return u.String(), nil
}
```

**Design:** Uses `url.Values.Encode()` for automatic URL-encoding. `q.Set` overwrites if a key exists in both the URL and `query:` map. `u.Query()` preserves existing URL-inline params.

#### Updated `Execute` (query portion — added before `http.NewRequestWithContext`)
```go
    reqURL, err := applyQueryParams(req.URL, req.QueryParams)
    if err != nil {
        return nil, fmt.Errorf("%w: %w", ErrNetwork, err)
    }

    // ... then use reqURL instead of req.URL
    httpReq, err := http.NewRequestWithContext(ctx, req.Method, reqURL, bodyReader)
```

#### Tests to Write FIRST (RED phase)

```go
// Add to TestExecute table:
{
    name: "query params appended to URL",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Query().Get("page") != "1" || r.URL.Query().Get("limit") != "10" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req: &parser.Request{
        Method:      "GET",
        QueryParams: map[string]string{"page": "1", "limit": "10"},
    },
    wantStatus: 200,
},
{
    name: "query params with special characters are URL-encoded",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.URL.Query().Get("q") != "hello world" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req: &parser.Request{
        Method:      "GET",
        QueryParams: map[string]string{"q": "hello world"},
    },
    wantStatus: 200,
},
{
    name: "empty query params map does not modify URL",
    handler: func(w http.ResponseWriter, r *http.Request) {
        if r.URL.RawQuery != "" {
            w.WriteHeader(400)
            return
        }
        w.WriteHeader(200)
    },
    req:        &parser.Request{Method: "GET", QueryParams: map[string]string{}},
    wantStatus: 200,
},
```

#### Impact on Existing Tests
- Existing tests pass `nil` QueryParams. `applyQueryParams(url, nil)` returns the URL unchanged due to `len(nil) == 0`. **No breakage.**

---

### Step 5: All HTTP Methods Integration Tests
**Rationale:** Validates that the method normalization in the parser correctly flows through to actual HTTP requests sent by the executor.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/httpexec/executor_test.go` | modify | Add table-driven test for all HTTP methods |

#### Tests to Write FIRST (RED phase)

```go
func TestExecute_all_methods(t *testing.T) {
    methods := []string{"GET", "POST", "PUT", "DELETE", "PATCH", "HEAD", "OPTIONS"}

    for _, method := range methods {
        t.Run(method+" request sends correct method", func(t *testing.T) {
            handler := func(w http.ResponseWriter, r *http.Request) {
                if r.Method != method {
                    w.WriteHeader(400)
                    return
                }
                w.WriteHeader(200)
            }
            srv := httptest.NewServer(http.HandlerFunc(handler))
            defer srv.Close()

            req := &parser.Request{Method: method, URL: srv.URL}
            result, err := Execute(context.Background(), req)
            if err != nil {
                t.Fatalf("unexpected error: %v", err)
            }
            if result.StatusCode != 200 {
                t.Errorf("got status %d, want 200", result.StatusCode)
            }
        })
    }
}
```

#### Impact on Existing Tests
- None. This is a new test function.

---

### Step 6: Update Sample Collection and Smoke Test
**Rationale:** Final step — makes the new capabilities visible in the project's sample and smoke test. Depends on all prior steps being complete.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `sample/hello.yaml` | modify | Add POST example with body, headers, and query params |
| `smoke/run.sh` | modify | Verify the updated sample runs successfully |

#### Current `sample/hello.yaml`
```yaml
name: Hello API
description: A simple example collection

requests:
  - name: Get httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"
```

#### New `sample/hello.yaml`
```yaml
name: Hello API
description: A simple example collection

requests:
  - name: Get httpbin
    request:
      method: GET
      url: "https://httpbin.org/get"

  - name: Post with JSON body
    request:
      method: POST
      url: "https://httpbin.org/post"
      headers:
        Accept: application/json
      query:
        source: curlew
      body:
        message: "Hello from Curlew"
        timestamp: "2026-01-01T00:00:00Z"
```

#### Impact on Existing Tests
- Smoke test already runs `sample/hello.yaml`. The updated file adds a second request — the smoke test will now exercise POST, body, headers, and query params. **No breakage** (smoke test just checks exit code 0).

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | `TestParseFile` | none | add new table entries |
| `internal/httpexec/executor_test.go` | `TestExecute` | none | add new table entries |
| `internal/httpexec/executor_test.go` | `TestExecute_network_error_preserves_inner_error` | none | — |
| `internal/httpexec/executor_test.go` | `TestExecute_cancelled_context` | none | — |
| `cmd/curlew/main_test.go` | (all) | none | — |

No existing tests break. All changes are additive.

## Risks and Edge Cases

- **Risk:** `any` type for Body requires runtime type-switch → **Mitigation:** Exhaustive test cases for map, string, nil, nested map. `json.Marshal` handles all YAML-parseable types.
- **Risk:** YAML tag `query` vs spec's `query_params` → **Mitigation:** Using `query` per task description. Easy rename later; no users yet.
- **Edge case:** Body on GET/HEAD/DELETE requests → **Handling:** No restriction enforced. HTTP allows it; `net/http` handles it. User knows what they're doing.
- **Edge case:** Body as YAML integer/boolean/array → **Handling:** `json.Marshal` serializes all Go types correctly. No special handling needed.
- **Edge case:** Empty body field (`body:` with no value) → **Handling:** YAML unmarshals as `nil`. `prepareBody(nil)` returns nil reader.
- **Edge case:** Query params with special chars → **Handling:** `url.Values.Encode()` URL-encodes automatically.
- **Edge case:** Duplicate header keys in YAML → **Handling:** YAML `map[string]string` keeps last value. `Header.Set` overwrites. Consistent behavior.
- **Edge case:** Case sensitivity in method names → **Handling:** `strings.ToUpper` normalization in parser.
- **Edge case:** Query params merge with URL-inline params → **Handling:** `u.Query()` parses existing, `q.Set` adds/overwrites. Both preserved.
- **Edge case:** Content-Type header casing → **Handling:** `http.Header.Get` is case-insensitive per Go stdlib.

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
./curlew run sample/hello.yaml
# Expected: Both GET and POST requests execute successfully
# POST request sends JSON body, headers, and query params to httpbin.org/post
```
