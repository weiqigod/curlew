# Implementation Plan: M2-010

## Overview
Extends `RequestItem` with an `auth:` field that references an auth profile by name, and wires the runner to inject the resolved credentials as an `Authorization` header before each request executes.

## Task Details
- **ID:** M2-010
- **Title:** Per-request auth profile reference
- **Phase:** M2: Dynamic Auth
- **Priority:** 3
- **Complexity:** medium

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M2-008 | Auth profile configuration and execution | done |

---

## Implementation Steps

### Step 1: Add `Auth` field to `RequestItem`
**Rationale:** Smallest possible change — adds one YAML-parseable field to a struct. No behavior changes; `omitempty` means existing YAML without `auth:` is unaffected and zero-values in tests remain compatible.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/collection.go` | modify | Add `Auth string` to `RequestItem` |
| `internal/parser/parser_test.go` | modify | Tests for parsing `auth:` field |
| `internal/parser/testdata/with_auth.yaml` | create | YAML fixture with `auth:` on a request |

#### Current Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Path       string            `yaml:"path,omitempty"`
	Required   *bool             `yaml:"required,omitempty"` // nil = false (default)
	Request    Request           `yaml:"request"`
	Variables  SensitiveVars     `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### New Code
```go
// RequestItem is a single test entry in a collection.
type RequestItem struct {
	Name       string            `yaml:"name"`
	Path       string            `yaml:"path,omitempty"`
	Auth       string            `yaml:"auth,omitempty"` // references an auth profile by name
	Required   *bool             `yaml:"required,omitempty"` // nil = false (default)
	Request    Request           `yaml:"request"`
	Variables  SensitiveVars     `yaml:"variables,omitempty"`
	Assertions Assertions        `yaml:"assertions,omitempty"`
	Extract    map[string]string `yaml:"extract,omitempty"`
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_AuthField(t *testing.T) {
    tests := []struct {
        name     string
        file     string
        wantAuth string
    }{
        {"auth field parsed on request", "testdata/with_auth.yaml", "admin_token"},
        {"auth field empty when absent", "testdata/simple.yaml", ""},
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            col, err := ParseFile(tc.file)
            require.NoError(t, err)
            require.NotEmpty(t, col.Requests)
            assert.Equal(t, tc.wantAuth, col.Requests[0].Auth)
        })
    }
}
```

Fixture `testdata/with_auth.yaml`:
```yaml
name: auth-test
requests:
  - name: Get User
    auth: admin_token
    request:
      method: GET
      url: https://example.com/user
    assertions:
      status: [200]
```

#### Impact on Existing Tests
- No existing tests affected — `Auth: ""` is the zero value, matching all existing expected structs.

---

### Step 2: Propagate `Auth` in external reference resolution
**Rationale:** When a collection uses `path:` to reference an external file, reference-site overrides (name, variables) are already propagated. `auth:` must follow the same pattern. Comes after Step 1 because it depends on `RequestItem.Auth` existing.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/parser/external.go` | modify | Copy `item.Auth` into resolved `RequestItem` |
| `internal/parser/external_test.go` | modify | Test auth propagation from reference site |
| `internal/parser/testdata/ref_with_auth.yaml` | create | Collection that references an external file and sets `auth:` |

#### Current Code
```go
ri := RequestItem{
    Name:       ext.Name,
    Request:    ext.Request,
    Assertions: ext.Assertions,
    Extract:    ext.Extract,
}

// Reference site variable overrides
if len(item.Variables.Values) > 0 {
    ri.Variables = item.Variables
}

// Reference site name override
if item.Name != "" {
    ri.Name = item.Name
}
```

#### New Code
```go
ri := RequestItem{
    Name:       ext.Name,
    Request:    ext.Request,
    Assertions: ext.Assertions,
    Extract:    ext.Extract,
}

// Reference site variable overrides
if len(item.Variables.Values) > 0 {
    ri.Variables = item.Variables
}

// Reference site name override
if item.Name != "" {
    ri.Name = item.Name
}

// Reference site auth override
if item.Auth != "" {
    ri.Auth = item.Auth
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestParseFile_ExternalRef_AuthPropagated(t *testing.T) {
    col, err := ParseFile("testdata/ref_with_auth.yaml")
    require.NoError(t, err)
    require.Len(t, col.Requests, 1)
    assert.Equal(t, "admin_token", col.Requests[0].Auth)
}

func TestParseFile_ExternalRef_NoAuth_EmptyWhenAbsent(t *testing.T) {
    col, err := ParseFile("testdata/ref_without_auth.yaml")
    require.NoError(t, err)
    require.Len(t, col.Requests, 1)
    assert.Equal(t, "", col.Requests[0].Auth)
}
```

#### Impact on Existing Tests
- No existing tests affected — the new block only runs when `item.Auth != ""`.

---

### Step 3: Add auth profile resolution function to runner
**Rationale:** Pure function with no side effects — easiest to unit-test in isolation. Must come before the injection wiring in Step 4.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Add `resolveAuthProfile`, `hasHeaderCaseInsensitive`, and `ErrAuthProfileNotFound` |
| `internal/runner/runner_test.go` | modify | Unit tests for the two new functions |

#### New Code

```go
// ErrAuthProfileNotFound is returned when a request references an auth profile
// that does not exist in the configured auth profiles.
var ErrAuthProfileNotFound = errors.New("auth profile not found")

// resolveAuthProfile looks up authName in profiles and returns the header name
// and value to inject. Returns ("", "", nil) when authName is empty.
// Returns a descriptive ErrAuthProfileNotFound when the profile is not found,
// listing all available profile names.
func resolveAuthProfile(authName string, profiles []auth.Profile, scope *variable.Scope) (headerName, headerValue string, err error) {
	if authName == "" {
		return "", "", nil
	}

	var found *auth.Profile
	available := make([]string, 0, len(profiles))
	for i := range profiles {
		available = append(available, profiles[i].Name)
		if profiles[i].Name == authName {
			found = &profiles[i]
		}
	}

	if found == nil {
		if len(available) == 0 {
			return "", "", fmt.Errorf("%w: %q; no auth profiles configured (add auth_profiles: to curlew.yaml)",
				ErrAuthProfileNotFound, authName)
		}
		sort.Strings(available)
		return "", "", fmt.Errorf("%w: %q; available profiles: %s",
			ErrAuthProfileNotFound, authName, strings.Join(available, ", "))
	}

	varName := found.Extract
	if varName == "" {
		varName = found.Name
	}

	val, ok := scope.Get(varName)
	if !ok {
		return "", "", fmt.Errorf("auth profile %q: variable %q not found in scope (did the profile execute successfully?)",
			authName, varName)
	}

	return "Authorization", "Bearer " + val, nil
}

// hasHeaderCaseInsensitive reports whether headers contains name (case-insensitive).
func hasHeaderCaseInsensitive(headers map[string]string, name string) bool {
	lower := strings.ToLower(name)
	for k := range headers {
		if strings.ToLower(k) == lower {
			return true
		}
	}
	return false
}
```

#### Tests to Write FIRST (RED phase)

```go
func TestResolveAuthProfile(t *testing.T) {
    makeScope := func(kvs ...string) *variable.Scope {
        s := variable.NewScope()
        for i := 0; i < len(kvs); i += 2 {
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
            name:     "empty auth name returns nothing",
            authName: "",
            scope:    makeScope(),
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
            name: "profile without extract uses name as variable",
            authName: "myprofile",
            profiles: []auth.Profile{{Name: "myprofile"}},
            scope:    makeScope("myprofile", "secret"),
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
            scope:       makeScope(), // empty scope
            wantErr:     true,
            errContains: "admin_token",
        },
    }
    for _, tc := range tests {
        t.Run(tc.name, func(t *testing.T) {
            profs := tc.profiles
            if profs == nil && !tc.wantErr {
                profs = profiles
            }
            hdr, val, err := resolveAuthProfile(tc.authName, profs, tc.scope)
            if tc.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tc.errContains)
                return
            }
            require.NoError(t, err)
            assert.Equal(t, tc.wantHeader, hdr)
            assert.Equal(t, tc.wantValue, val)
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
            assert.Equal(t, tc.want, hasHeaderCaseInsensitive(tc.headers, tc.key))
        })
    }
}
```

#### Impact on Existing Tests
- No existing tests affected — new unexported functions with no call sites yet.

---

### Step 4: Inject auth header in `executePhase`
**Rationale:** Final wiring step. Depends on Steps 1 (Auth field on item), 2 (external refs), and 3 (resolution function). Placed after interpolation so the explicit-header precedence check works on the final interpolated header map.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/runner/runner.go` | modify | Inject auth header between interpolation and `exec` call in `executePhase` |
| `internal/runner/runner_test.go` | modify | Integration-level tests for auth injection in full `Run` |
| `smoke/run.sh` | modify | Add scenario verifying per-request auth injection |

#### Current Code (lines 412–420 of `runner.go`)
```go
// Interpolate request fields (on a copy, not mutating parsed collection)
req := item.Request
interpolated, interpErr := interpolateRequest(reqScope, &req)
if interpErr != nil {
    return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, interpErr)
}
req = *interpolated

result, execErr := exec(ctx, &req)
```

#### New Code
```go
// Interpolate request fields (on a copy, not mutating parsed collection)
req := item.Request
interpolated, interpErr := interpolateRequest(reqScope, &req)
if interpErr != nil {
    return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, interpErr)
}
req = *interpolated

// Per-request auth profile injection
if item.Auth != "" {
    headerName, headerValue, authErr := resolveAuthProfile(item.Auth, vars.AuthProfiles, reqScope)
    if authErr != nil {
        return results, requiredFailed, fmt.Errorf("request %q: %w", item.Name, authErr)
    }
    // Explicit header takes precedence (Behavior 4)
    if headerName != "" && !hasHeaderCaseInsensitive(req.Headers, headerName) {
        if req.Headers == nil {
            req.Headers = make(map[string]string)
        }
        req.Headers[headerName] = headerValue
    }
}

result, execErr := exec(ctx, &req)
```

#### Tests to Write FIRST (RED phase)

```go
func TestRun_PerRequestAuth(t *testing.T) {
    // captureExec records headers of each call; returns 200 OK
    captureExec := func(captured *[]map[string]string) ExecuteFunc {
        return func(ctx context.Context, req *parser.Request) (*parser.Response, error) {
            headers := make(map[string]string, len(req.Headers))
            for k, v := range req.Headers {
                headers[k] = v
            }
            *captured = append(*captured, headers)
            return &parser.Response{StatusCode: 200}, nil
        }
    }

    makeProfile := func(name, extract string) auth.Profile {
        return auth.Profile{Name: name, Extract: extract}
    }

    tests := []struct {
        name        string
        requests    []parser.RequestItem
        profiles    []auth.Profile
        scopeVars   map[string]string // pre-seeded as CLI vars
        wantHeaders []map[string]string
        wantErr     bool
        errContains string
    }{
        {
            name: "auth injects bearer header",
            requests: []parser.RequestItem{
                {Name: "r1", Auth: "admin_token",
                    Request: parser.Request{Method: "GET", URL: "https://example.com"}},
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
                {Name: "r1", Auth: "admin_token",
                    Request: parser.Request{Method: "GET", URL: "https://example.com"}},
                {Name: "r2", Auth: "user_token",
                    Request: parser.Request{Method: "GET", URL: "https://example.com"}},
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
                {Name: "r1", Auth: "admin_token",
                    Request: parser.Request{
                        Method:  "GET",
                        URL:     "https://example.com",
                        Headers: map[string]string{"Authorization": "Custom explicit"},
                    }},
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
                {Name: "r1", Auth: "admin_token",
                    Request: parser.Request{
                        Method:  "GET",
                        URL:     "https://example.com",
                        Headers: map[string]string{"authorization": "Custom lowercase"},
                    }},
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
                {Name: "r1", Auth: "missing",
                    Request: parser.Request{Method: "GET", URL: "https://example.com"}},
            },
            profiles:    []auth.Profile{makeProfile("admin_token", "admin_token")},
            scopeVars:   map[string]string{"admin_token": "tok"},
            wantErr:     true,
            errContains: "admin_token",
        },
        {
            name: "auth with no profiles configured returns clear error",
            requests: []parser.RequestItem{
                {Name: "r1", Auth: "foo",
                    Request: parser.Request{Method: "GET", URL: "https://example.com"}},
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
                Requests: tc.requests,
            }
            vars := VarSources{
                AuthProfiles:    tc.profiles,
                CLI:             tc.scopeVars,
                Tier:            auth.TierSolo,
                AuthExecuteFunc: func(context.Context, string) (map[string]string, error) {
                    return tc.scopeVars, nil
                },
            }
            _, _, err := Run(context.Background(), col, captureExec(&captured), vars)
            if tc.wantErr {
                require.Error(t, err)
                assert.Contains(t, err.Error(), tc.errContains)
                return
            }
            require.NoError(t, err)
            require.Len(t, captured, len(tc.wantHeaders))
            for i, want := range tc.wantHeaders {
                for k, v := range want {
                    assert.Equal(t, v, captured[i][k], "request %d header %q", i, k)
                }
                // Ensure no unexpected auth headers added
                if _, authExpected := want["Authorization"]; !authExpected {
                    if _, authExpectedLower := want["authorization"]; !authExpectedLower {
                        _, hasAuth := captured[i]["Authorization"]
                        assert.False(t, hasAuth, "unexpected Authorization header injected for request %d", i)
                    }
                }
            }
        })
    }
}
```

#### Impact on Existing Tests
- All existing `TestRun_*` tests: unaffected because no existing `RequestItem` sets `Auth`, so the new `if item.Auth != ""` block is never entered.
- `ErrAuthProfileNotFound` is a new sentinel error — no conflicts.

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `internal/parser/parser_test.go` | all existing | none | — |
| `internal/parser/external_test.go` | all existing | none | — |
| `internal/runner/runner_test.go` | all existing | none | — |
| `internal/parser/parser_test.go` | `TestParseFile_AuthField` | new | write in Step 1 |
| `internal/parser/external_test.go` | `TestParseFile_ExternalRef_Auth*` | new | write in Step 2 |
| `internal/runner/runner_test.go` | `TestResolveAuthProfile` | new | write in Step 3 |
| `internal/runner/runner_test.go` | `TestHasHeaderCaseInsensitive` | new | write in Step 3 |
| `internal/runner/runner_test.go` | `TestRun_PerRequestAuth` | new | write in Step 4 |

---

## Risks and Edge Cases

- **Risk: Case-sensitive header key lookup** → **Mitigation:** `hasHeaderCaseInsensitive` performs a case-insensitive scan of the headers map before injecting, so `authorization:` and `Authorization:` both signal an explicit header.

- **Risk: Profile has no `Extract` field, produces multiple variables** → **Mitigation:** Fall back to using `profile.Name` as the variable key. If that variable is also not in scope, return a clear error message.

- **Risk: Auth profile variable overwritten by higher-precedence source** → **Mitigation:** `scope.Get()` returns the current effective value; this is correct behavior (CLI `--var` should win over profile-extracted tokens). Document as expected.

- **Edge case: `auth:` on external request files themselves** → **Handling:** The `externalRequest` struct intentionally has no `Auth` field; auth is set at the reference site in the collection, not in the external file. This mirrors the `variables:` override pattern.

- **Edge case: `auth:` field with interpolated value (`auth: "{{profile_name}}"`)** → **Handling:** Not supported — `auth:` is a static reference resolved at YAML parse time, not runtime. The profile name must be a literal string.

- **Edge case: Auth and setup/teardown phases** → **Handling:** `executePhase` is called for all three phases with the same item-iteration logic. Auth injection automatically applies to setup and teardown items that set `auth:`.

---

## Deviations

### Deviation 1: No testify dependency
**Plan assumed:** test code uses `require.NoError`, `assert.Equal`, `assert.Contains` from testify.
**Reality:** The project has no testify dependency (`go.mod` only has `gopkg.in/yaml.v3`). All tests use stdlib `t.Fatalf`/`t.Errorf`/`strings.Contains`.
**Fix:** Rewrite all test assertions using standard `testing.T` methods.

### Deviation 2: Wrong return type in captureExec
**Plan assumed:** `captureExec` returns `&parser.Response{StatusCode: 200}`.
**Reality:** `ExecuteFunc` returns `*httpexec.Result`, not `*parser.Response`.
**Fix:** Return `&httpexec.Result{StatusCode: 200}` and import `httpexec` package.

---

## Verification

```bash
go build ./cmd/curlew
go test ./internal/parser/...
go test ./internal/runner/...
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
```

Observable verification:
```bash
# Create collection with auth: on individual requests
cat > /tmp/test_auth_per_req.yaml <<'EOF'
name: per-request-auth-test
requests:
  - name: Authenticated Request
    auth: admin_token
    request:
      method: GET
      url: https://httpbin.org/headers
    assertions:
      status: [200]
EOF

# Run with a mock auth profile (requires curlew.yaml with auth_profiles:)
curlew run /tmp/test_auth_per_req.yaml

# Unit test verification
go test ./internal/runner/... -run TestRun_PerRequestAuth -v
go test ./internal/runner/... -run TestResolveAuthProfile -v
go test ./internal/parser/... -run TestParseFile_AuthField -v
```
