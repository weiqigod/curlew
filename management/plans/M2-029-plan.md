# Implementation Plan: M2-029

## Overview
Create the `internal/graphql/` package implementing a GraphQL protocol adapter that transforms `protocol: graphql` requests into HTTP POST requests with JSON bodies containing `query` and `variables`. This is the foundational GraphQL task -- external query files, fragments, and advanced error handling modes are deferred to M2-030 and M2-031.

## Task Details
- **ID:** M2-029
- **Title:** GraphQL protocol adapter (queries and mutations)
- **Phase:** M2: GraphQL
- **Priority:** 3
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M1-001 | CLI scaffold and basic `run` command | done |
| M1-005 | JSONPath body assertions | done |
| M1-028 | Feature gate framework | done |

## Architecture Decision: Adapter vs. Pre-processing

The specification describes a `ProtocolAdapter` interface pattern. However, for M2-029 (the first protocol beyond HTTP), the simplest and most aligned approach is a **pre-processing transformation** that:

1. Adds a `Protocol` field and `GraphQL` config block to `parser.Request`
2. Creates `internal/graphql/` with a `BuildRequest` function that transforms GraphQL config into an HTTP-compatible request (POST with JSON body `{"query":"...","variables":{...}}`)
3. Has the runner detect `protocol: graphql`, gate-check it, run the transformer, and then use the existing HTTP execution path

This avoids prematurely abstracting a protocol adapter interface that only has one non-default implementation. A full `ProtocolAdapter` interface can be refactored when WebSocket support (M2-032) requires a genuinely different execution model.

**Error Handling Scope for M2-029:** The task behaviors specify two modes: `fail` (default -- fail if `$.errors` present) and `warn` (log warning, exit 0). The `ignore` mode and global config are deferred to M2-031. For M2-029, the per-request `error_handling` field on the `graphql:` block controls behavior. Default is `fail`.

## Implementation Steps

### Step 1: Add Protocol and GraphQL fields to parser types
**Rationale:** The parser types must be extended first since everything else depends on the YAML structure being parseable. Smallest blast radius -- only adds new optional fields.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Protocol` field to `Request`, add `GraphQL` config struct |
| `internal/parser/parser.go` | modify | Add validation for `protocol` field values |
| `internal/parser/parser_test.go` | modify | Add tests for new protocol/graphql parsing |
| `internal/parser/testdata/graphql_basic.yaml` | create | Test fixture for GraphQL collection |
| `internal/parser/testdata/graphql_invalid_protocol.yaml` | create | Test fixture for invalid protocol value |

#### Current Code
```go
// Request defines the HTTP request to execute.
type Request struct {
	Method  string            `yaml:"method"`
	URL     string            `yaml:"url"`
	Headers map[string]string `yaml:"headers,omitempty"`
	Body any `yaml:"body,omitempty"`
	QueryParams map[string]string `yaml:"query,omitempty"`
}
```

#### New Code
```go
// GraphQLConfig holds the GraphQL-specific request configuration.
type GraphQLConfig struct {
	Query         string         `yaml:"query"`
	Variables     map[string]any `yaml:"variables,omitempty"`
	ErrorHandling string         `yaml:"error_handling,omitempty"` // "fail" (default), "warn"
}

// Request defines the HTTP request to execute.
type Request struct {
	Method      string            `yaml:"method"`
	URL         string            `yaml:"url"`
	Headers     map[string]string `yaml:"headers,omitempty"`
	Body        any               `yaml:"body,omitempty"`
	QueryParams map[string]string `yaml:"query,omitempty"`
	Protocol    string            `yaml:"protocol,omitempty"` // "http" (default), "graphql"
	GraphQL     *GraphQLConfig    `yaml:"graphql,omitempty"`
}
```

Parser validation additions:
```go
// In ParseFile, after method normalization:
// Validate protocol field
for _, section := range []*[]RequestItem{&col.Setup.Items, &col.Requests.Items, &col.Teardown.Items} {
    for i := range *section {
        protocol := (*section)[i].Request.Protocol
        if protocol != "" && protocol != "http" && protocol != "graphql" {
            return nil, &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: path,
                Message:  fmt.Sprintf("unsupported protocol %q in request %q", protocol, (*section)[i].Name),
                Hint:     "Allowed protocols: http, graphql",
                Inner:    ErrUnsupportedProtocol,
            }
        }
        // GraphQL requests must have a query
        if protocol == "graphql" && ((*section)[i].Request.GraphQL == nil || (*section)[i].Request.GraphQL.Query == "") {
            return nil, &apierrors.Structured{
                Category: apierrors.CategoryParse,
                FilePath: path,
                Message:  fmt.Sprintf("graphql request %q must have a graphql.query field", (*section)[i].Name),
                Hint:     "Add graphql: query: |... to your request",
                Inner:    ErrMissingRequiredField,
            }
        }
    }
}
```

#### Tests to Write FIRST (RED phase)

```go
// In parser_test.go
func TestParseFile_graphql(t *testing.T) {
    tests := []struct {
        name    string
        file    string
        wantErr error
        check   func(*testing.T, *Collection)
    }{
        {"basic graphql query parses", "testdata/graphql_basic.yaml", nil, func(t *testing.T, col *Collection) {
            if col.Requests.Items[0].Request.Protocol != "graphql" { t.Error("expected protocol graphql") }
            if col.Requests.Items[0].Request.GraphQL == nil { t.Fatal("expected graphql config") }
            if col.Requests.Items[0].Request.GraphQL.Query == "" { t.Error("expected query") }
        }},
        {"graphql with variables parses", "testdata/graphql_with_vars.yaml", nil, func(t *testing.T, col *Collection) {
            gql := col.Requests.Items[0].Request.GraphQL
            if gql.Variables == nil { t.Fatal("expected variables") }
            if gql.Variables["id"] != "123" { t.Errorf("expected id=123, got %v", gql.Variables["id"]) }
        }},
        {"invalid protocol value", "testdata/graphql_invalid_protocol.yaml", ErrUnsupportedProtocol, nil},
        {"graphql without query field", "testdata/graphql_no_query.yaml", ErrMissingRequiredField, nil},
        {"default protocol is empty (http)", "testdata/minimal.yaml", nil, func(t *testing.T, col *Collection) {
            if col.Requests.Items[0].Request.Protocol != "" { t.Errorf("expected empty protocol, got %q", col.Requests.Items[0].Request.Protocol) }
        }},
        {"graphql error_handling parses", "testdata/graphql_error_handling.yaml", nil, func(t *testing.T, col *Collection) {
            gql := col.Requests.Items[0].Request.GraphQL
            if gql.ErrorHandling != "warn" { t.Errorf("expected warn, got %q", gql.ErrorHandling) }
        }},
    }
    // ...
}
```

#### Impact on Existing Tests
- No existing parser tests are affected -- `Protocol` field defaults to empty string (treated as `http`), `GraphQL` field defaults to `nil`
- All existing `Request` structs in test fixtures continue to work unchanged

### Step 2: Create `internal/graphql/` package with request builder
**Rationale:** The core adapter logic must exist before the runner can use it. This step is purely functional -- no side effects, easy to test in isolation.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/graphql/graphql.go` | create | GraphQL request builder and response checker |
| `internal/graphql/graphql_test.go` | create | Table-driven tests for all behaviors |
| `internal/graphql/errors.go` | create | Sentinel errors for GraphQL-specific failures |

#### New Code
```go
// Package graphql provides GraphQL protocol support for the API testing tool.
package graphql

import (
	"encoding/json"
	"fmt"

	"github.com/peterlindqvist/apitest/internal/parser"
)

// ErrorHandlingMode controls how GraphQL errors in responses are treated.
type ErrorHandlingMode string

const (
	ErrorHandlingFail ErrorHandlingMode = "fail"
	ErrorHandlingWarn ErrorHandlingMode = "warn"
)

// RequestBody is the JSON structure sent as the GraphQL HTTP POST body.
type RequestBody struct {
	Query     string         `json:"query"`
	Variables map[string]any `json:"variables,omitempty"`
}

// BuildRequest transforms a GraphQL request config into HTTP-compatible request fields.
// It sets Method to POST, serializes query+variables as JSON body, and sets Content-Type.
// Returns the modified request (does not mutate the input).
func BuildRequest(req *parser.Request) (*parser.Request, error) {
	if req.GraphQL == nil {
		return nil, ErrMissingConfig
	}
	if req.GraphQL.Query == "" {
		return nil, ErrEmptyQuery
	}

	body := RequestBody{
		Query:     req.GraphQL.Query,
		Variables: req.GraphQL.Variables,
	}
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("serializing graphql body: %w", err)
	}

	out := *req // shallow copy
	out.Method = "POST"
	out.Body = string(jsonBytes)
	if out.Headers == nil {
		out.Headers = make(map[string]string)
	}
	if _, ok := out.Headers["Content-Type"]; !ok {
		out.Headers["Content-Type"] = "application/json"
	}
	return &out, nil
}

// ParseErrorHandling returns the error handling mode from the config string.
// Defaults to ErrorHandlingFail if empty.
func ParseErrorHandling(mode string) (ErrorHandlingMode, error) {
	switch mode {
	case "", "fail":
		return ErrorHandlingFail, nil
	case "warn":
		return ErrorHandlingWarn, nil
	default:
		return "", fmt.Errorf("%w: %q (allowed: fail, warn)", ErrInvalidErrorHandling, mode)
	}
}

// GraphQLError represents a single error from a GraphQL response's errors array.
type GraphQLError struct {
	Message string `json:"message"`
}

// ResponseCheck holds the result of checking a GraphQL response for errors.
type ResponseCheck struct {
	HasErrors bool           // true if $.errors array is non-empty
	HasData   bool           // true if $.data is non-null
	Errors    []GraphQLError // parsed error objects (if any)
}

// CheckResponse examines a response body for GraphQL-level errors.
// It parses the JSON and checks for the presence of "errors" and "data" fields.
func CheckResponse(body []byte) (*ResponseCheck, error) {
	if len(body) == 0 {
		return &ResponseCheck{}, nil
	}
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parsing graphql response: %w", err)
	}

	check := &ResponseCheck{}

	// Check for errors array
	if errRaw, ok := parsed["errors"]; ok {
		var errors []GraphQLError
		if err := json.Unmarshal(errRaw, &errors); err == nil && len(errors) > 0 {
			check.HasErrors = true
			check.Errors = errors
		}
	}

	// Check for data field
	if dataRaw, ok := parsed["data"]; ok {
		// data: null is not "has data"
		if string(dataRaw) != "null" {
			check.HasData = true
		}
	}

	return check, nil
}
```

```go
// errors.go
package graphql

import "errors"

var (
	ErrMissingConfig        = errors.New("graphql config is required for protocol: graphql")
	ErrEmptyQuery           = errors.New("graphql query must not be empty")
	ErrInvalidErrorHandling = errors.New("invalid graphql error_handling mode")
	ErrGraphQLErrors        = errors.New("graphql response contains errors")
)
```

#### Tests to Write FIRST (RED phase)

```go
func TestBuildRequest(t *testing.T) {
    tests := []struct {
        name    string
        req     *parser.Request
        wantErr error
        check   func(*testing.T, *parser.Request)
    }{
        {"basic query sets POST method", ...},
        {"basic query sets Content-Type application/json", ...},
        {"query with variables includes variables in body", ...},
        {"preserves existing headers", ...},
        {"does not override existing Content-Type", ...},
        {"nil graphql config returns ErrMissingConfig", ...},
        {"empty query returns ErrEmptyQuery", ...},
        {"preserves URL from input", ...},
    }
}

func TestCheckResponse(t *testing.T) {
    tests := []struct {
        name      string
        body      []byte
        wantCheck *ResponseCheck
        wantErr   bool
    }{
        {"success response - no errors", ...},
        {"full failure - errors only, data null", ...},
        {"partial success - data and errors", ...},
        {"empty body", ...},
        {"invalid JSON returns error", ...},
        {"null errors array treated as no errors", ...},
        {"empty errors array treated as no errors", ...},
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
        {"invalid mode returns error", "invalid", "", true},
    }
}
```

#### Impact on Existing Tests
- No existing tests affected -- new package entirely

### Step 3: Register GraphQL feature gate
**Rationale:** The feature gate must be registered before the runner can enforce it. Small change to `internal/auth/registry.go`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/auth/registry.go` | modify | Add `protocol_graphql` feature definition |
| `internal/auth/gate_test.go` | modify | Add tests for the new feature gate |

#### Current Code
```go
// In DefaultRegistry():
r.Register(FeatureDefinition{
    Name:         "html_report",
    RequiredTier: TierProfessional,
    Description:  "HTML report generation requires Professional tier ($19/month)",
    Workaround:   "Use --format json for machine-readable output, or --format junit for CI integration",
})
return r
```

#### New Code
```go
r.Register(FeatureDefinition{
    Name:         "html_report",
    RequiredTier: TierProfessional,
    Description:  "HTML report generation requires Professional tier ($19/month)",
    Workaround:   "Use --format json for machine-readable output, or --format junit for CI integration",
})
r.Register(FeatureDefinition{
    Name:         "protocol_graphql",
    RequiredTier: TierProfessional,
    Description:  "GraphQL protocol support requires Professional tier ($19/month)",
    Workaround:   "Use protocol: http with manual POST requests and JSON body for basic GraphQL",
})
return r
```

#### Tests to Write FIRST (RED phase)

```go
func TestCheckFeature_graphql(t *testing.T) {
    registry := DefaultRegistry()
    tests := []struct {
        name    string
        tier    Tier
        wantErr bool
    }{
        {"free tier gated", TierFree, true},
        {"solo tier gated", TierSolo, true},
        {"professional tier allowed", TierProfessional, false},
        {"team tier allowed", TierTeam, false},
        {"enterprise tier allowed", TierEnterprise, false},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := CheckFeature(registry, "protocol_graphql", tt.tier)
            if (err != nil) != tt.wantErr {
                t.Errorf("CheckFeature(protocol_graphql, %q) error = %v, wantErr %v", tt.tier, err, tt.wantErr)
            }
            if tt.wantErr {
                var gateErr *GateError
                if !errors.As(err, &gateErr) {
                    t.Errorf("expected *GateError, got %T", err)
                }
            }
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected -- adding a new feature definition does not change existing lookups

### Step 4: Integrate GraphQL into the runner
**Rationale:** The runner is the integration point where protocol detection, feature gating, and request transformation come together. This is the largest change and depends on steps 1-3.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add GraphQL detection, gate check, request transformation, and response error checking in `executePhase` |
| `internal/runner/runner_test.go` | modify | Add tests for GraphQL execution path |

#### Current Code (executePhase, lines ~688-693)
```go
// Interpolate request fields (on a copy, not mutating parsed collection)
req := item.Request
interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
if interpErr != nil {
    return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, interpErr)
}
req = *interpolated
```

#### New Code
```go
// Interpolate request fields (on a copy, not mutating parsed collection)
req := item.Request
interpolated, interpErr := requtil.InterpolateRequest(reqScope, &req)
if interpErr != nil {
    return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, interpErr)
}
req = *interpolated

// GraphQL protocol handling: transform graphql request into HTTP POST
if req.Protocol == "graphql" {
    // Feature gate check
    reg := vars.Registry
    if reg == nil {
        reg = auth.DefaultRegistry()
    }
    tier := vars.Tier
    if tier == "" {
        tier = auth.TierFree
    }
    if gateErr := auth.CheckFeature(reg, "protocol_graphql", tier); gateErr != nil {
        return results, requiredFailed, gateErr
    }

    // Interpolate graphql variables
    if req.GraphQL != nil {
        gqlReq, buildErr := graphql.BuildRequest(&req)
        if buildErr != nil {
            return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, buildErr)
        }
        req = *gqlReq
    }
}
```

After execution and assertion evaluation, add GraphQL error checking:
```go
// After existing assertion evaluation (ar := assertion.Evaluate(...)):
// GraphQL error checking (after assertions, so user assertions on $.errors still work)
if item.Request.Protocol == "graphql" && execErr == nil && result != nil {
    mode, _ := graphql.ParseErrorHandling("")
    if item.Request.GraphQL != nil && item.Request.GraphQL.ErrorHandling != "" {
        mode, _ = graphql.ParseErrorHandling(item.Request.GraphQL.ErrorHandling)
    }
    check, checkErr := graphql.CheckResponse(result.Body)
    if checkErr == nil && check.HasErrors && mode == graphql.ErrorHandlingFail {
        // Fail: treat GraphQL errors as assertion failure
        if ar == nil {
            ar = &assertion.Results{Passed: false}
        }
        ar.Passed = false
        errMsg := "GraphQL response contains errors"
        if len(check.Errors) > 0 {
            errMsg = fmt.Sprintf("GraphQL error: %s", check.Errors[0].Message)
        }
        ar.Details = append(ar.Details, assertion.Detail{
            Type:     "graphql_error",
            Message:  errMsg,
            Passed:   false,
        })
    }
    // warn mode: errors are logged but don't fail the test
    // (handled by output layer when rendering results)
}
```

Also need to add `graphql` import to runner.go, and handle GraphQL variable interpolation (the `graphql.variables` values need interpolation via scope).

#### Variable Interpolation for GraphQL

The `graphql.variables` field contains `map[string]any` which may contain `{{variable}}` references as string values. We need to interpolate these. The `BuildRequest` function should receive the already-interpolated request (interpolation happens on the `Request` fields). However, `GraphQL.Variables` are typed as `map[string]any` and are not interpolated by `requtil.InterpolateRequest` (which only handles `Request.URL`, `Request.Headers`, `Request.QueryParams`, `Request.Body`).

**Decision:** Add interpolation of `GraphQL.Query` and string values in `GraphQL.Variables` to `requtil.InterpolateRequest` (or add a new function in graphql package). Since requtil already handles request interpolation, it's most natural to extend it.

#### Files to Modify (additional)

| File | Action | Description |
|------|--------|-------------|
| `internal/requtil/requtil.go` | modify | Add GraphQL field interpolation to `InterpolateRequest` |

```go
// After existing interpolation in InterpolateRequest:
if out.GraphQL != nil {
    gql := *out.GraphQL // shallow copy
    if gql.Query, err = scope.Interpolate(gql.Query); err != nil {
        return nil, fmt.Errorf("graphql query: %w", err)
    }
    if gql.Variables != nil {
        interpolatedVars := make(map[string]any, len(gql.Variables))
        for k, v := range gql.Variables {
            if sv, ok := v.(string); ok {
                iv, intErr := scope.Interpolate(sv)
                if intErr != nil {
                    return nil, fmt.Errorf("graphql variable %q: %w", k, intErr)
                }
                interpolatedVars[k] = iv
            } else {
                interpolatedVars[k] = v
            }
        }
        gql.Variables = interpolatedVars
    }
    out.GraphQL = &gql
}
```

#### Tests to Write FIRST (RED phase)

```go
// In runner_test.go
func TestRun_graphql_basic_query(t *testing.T) {
    // Given a request with protocol: graphql and a graphql.query
    // When executed
    // Then the query is sent as POST with Content-Type application/json
}

func TestRun_graphql_with_variables(t *testing.T) {
    // Given a GraphQL query with graphql.variables
    // When executed
    // Then variables are included in the request body
}

func TestRun_graphql_mutation(t *testing.T) {
    // Given a GraphQL mutation
    // When executed
    // Then the mutation is sent and assertions validate $.data fields
}

func TestRun_graphql_errors_fail_default(t *testing.T) {
    // Given a GraphQL response with $.errors array
    // When error_handling is empty (default = fail)
    // Then the test fails
}

func TestRun_graphql_errors_warn(t *testing.T) {
    // Given a GraphQL response with $.errors and error_handling: warn
    // When executed
    // Then exit code is 0 (warning logged but not failure)
}

func TestRun_graphql_feature_gate_free_tier(t *testing.T) {
    // Given protocol: graphql at Free tier
    // When the collection runs
    // Then a GateError is returned
}

func TestRun_graphql_jsonpath_assertions(t *testing.T) {
    // Given assertions on $.data.user.name and $.errors
    // When evaluated
    // Then JSONPath assertions work identically to HTTP
}

// In requtil test or graphql test
func TestInterpolateRequest_graphql_variables(t *testing.T) {
    // Given graphql.variables with {{var}} references
    // When interpolated
    // Then variables are resolved
}
```

#### Impact on Existing Tests
- `runner_test.go`: No breakage -- existing tests use `Request.Protocol == ""` which means HTTP (default)
- `requtil_test.go`: No breakage -- existing InterpolateRequest tests don't set GraphQL fields

### Step 5: Add parser error sentinel and validation
**Rationale:** Clean error reporting requires a new sentinel error for unsupported protocols. Added as part of Step 1 implementation but documented separately for clarity.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/errors.go` | modify | Add `ErrUnsupportedProtocol` sentinel |

#### Current Code
```go
// (existing errors in errors.go)
```

#### New Code
```go
var ErrUnsupportedProtocol = errors.New("unsupported protocol")
```

#### Impact on Existing Tests
- None -- new error, not changing existing ones

### Step 6: Add GraphQL-aware assertion detail type
**Rationale:** The assertion package needs a "graphql_error" detail type so output formatters can display GraphQL errors properly. This is a minor extension.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/assertion/assertion.go` | verify | Confirm `Detail` struct already supports arbitrary `Type` strings (it does) |

Looking at the existing `assertion.Detail` struct -- it already has a `Type` field and `Message` field that are generic enough. No changes needed to `assertion.go` itself. The runner will create `Detail{Type: "graphql_error", Message: "...", Passed: false}` directly.

### Step 7: Update smoke test
**Rationale:** The smoke test should verify the GraphQL feature gate (Free tier) returns exit code 6. This is the observable verification from the task.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `smoke/run.sh` | modify | Add GraphQL protocol feature gate test |
| `smoke/graphql_gate.yaml` | create | Test collection with protocol: graphql |

#### New Code
```bash
echo "--- Running GraphQL collection (expect exit 6 - feature gate) ---"
GQL_FILE=$(mktemp /tmp/apitest_gql_XXXXXX.yaml)
cat > "$GQL_FILE" << 'YAML'
name: GraphQL Gate Test
requests:
  - name: GraphQL Query
    request:
      protocol: graphql
      url: "https://example.com/graphql"
      graphql:
        query: "query { hello }"
YAML
./apitest run "$GQL_FILE" 2>&1 && echo "ERROR: should have failed" || echo "Exit code: $?"
rm -f "$GQL_FILE"
echo
```

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | all existing | none | no change |
| `internal/runner/runner_test.go` | all existing | none | no change |
| `internal/auth/gate_test.go` | all existing | none | no change |
| `internal/requtil/requtil.go` | all existing | none | no change |

No existing tests break. All changes are additive.

## Risks and Edge Cases

- **Risk:** GraphQL variables contain nested objects (maps within maps) that need deep interpolation -> **Mitigation:** For M2-029, only interpolate top-level string values in `graphql.variables`. Deep interpolation of nested objects is an edge case that can be added if needed. The spec examples show flat variable maps with string values.

- **Risk:** The `graphql.variables` field accepts `map[string]any` but YAML may deserialize integers, booleans, etc. -> **Mitigation:** `json.Marshal` handles all YAML-native types correctly. Only string values get interpolated; numeric/boolean values pass through as-is.

- **Edge case:** User sets `protocol: graphql` but also sets `method: GET` -> **Handling:** `BuildRequest` always overrides to POST, which is correct per GraphQL spec (queries are sent via POST in our implementation).

- **Edge case:** User sets `protocol: graphql` but provides no `graphql:` block -> **Handling:** Parser validation catches this and returns `ErrMissingRequiredField` with a helpful hint.

- **Edge case:** User sets explicit `Content-Type` header on a GraphQL request -> **Handling:** `BuildRequest` does not override existing `Content-Type` headers, respecting user intent.

- **Edge case:** GraphQL response is not valid JSON (server error) -> **Handling:** `CheckResponse` returns an error, which the runner logs as a warning but does not crash -- the response is still available for assertions.

- **Risk:** Adding `Protocol` field to `Request` struct could affect YAML serialization/deserialization of existing collections -> **Mitigation:** The field has `yaml:"protocol,omitempty"`, so it's only present when explicitly set. Existing collections without `protocol:` parse identically.

- **Edge case:** `error_handling: warn` with GraphQL errors should NOT cause assertion failure -> **Handling:** The runner only adds the GraphQL error assertion detail when mode is `fail`. In `warn` mode, the errors are noted in the response check but the assertion results are not modified.

## Verification

```bash
go build ./cmd/apitest
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create a collection with protocol: graphql
cat > /tmp/graphql_test.yaml << 'YAML'
name: GraphQL Test
requests:
  - name: GraphQL Query
    request:
      protocol: graphql
      url: "https://example.com/graphql"
      graphql:
        query: "query { hello }"
YAML

# Run at Free tier (expect exit code 6 - feature gate)
apitest run /tmp/graphql_test.yaml
echo "Exit code: $?"

# Run unit tests for GraphQL adapter
go test ./internal/graphql/...
```
