Write tests for: $ARGUMENTS

---

## Step 1: Identify Target

Determine the test target from the argument:
- **Package**: `internal/parser` → test the entire package
- **Function**: `ParseCollection` → test that specific function
- **Feature area**: `variable interpolation` → test all related functions
- **File**: `internal/parser/collection.go` → test the source file

---

## Step 2: Review Standards

Read CLAUDE.md testing section now. Do NOT work from memory.

Key conventions:
- Framework: Go `testing` package only — no third-party test frameworks
- Subtests: `t.Run()` for all cases
- Naming: `TestFunctionName_Scenario_ExpectedResult`
- Pattern: table-driven tests as default
- Fixtures: `testdata/` directory for file-based input
- Integration: `os/exec` to exercise the real binary

---

## Step 3: Read Source

Read the source file(s) being tested in full. For each file, identify:
- All public functions and methods
- All error return paths
- All edge cases and boundary conditions
- All sentinel errors
- All branch conditions

---

## Step 4: Basic Unit Test Pattern

Every test follows Arrange-Act-Assert:

```go
func TestFunctionName_Scenario(t *testing.T) {
    // Arrange
    input := "test input"
    expected := "expected output"

    // Act
    result, err := pkg.FunctionName(input)

    // Assert
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if result != expected {
        t.Errorf("got %q, want %q", result, expected)
    }
}
```

Use `t.Fatalf` for errors that make further assertions meaningless. Use `t.Errorf` for assertions where the test can continue.

---

## Step 5: Table-driven Tests

**Default pattern for all tests with multiple cases.** Use this for Curlew:

```go
func TestParseCollection_Formats(t *testing.T) {
    tests := []struct {
        name    string
        input   string
        want    *Collection
        wantErr bool
    }{
        {
            name:  "valid single request",
            input: "testdata/single_request.yaml",
            want:  &Collection{Name: "test", Requests: []Request{{Method: "GET"}}},
        },
        {
            name:  "valid multi request",
            input: "testdata/multi_request.yaml",
            want:  &Collection{Name: "test", Requests: []Request{{Method: "GET"}, {Method: "POST"}}},
        },
        {
            name:    "empty file",
            input:   "testdata/empty.yaml",
            wantErr: true,
        },
        {
            name:    "invalid yaml",
            input:   "testdata/invalid.yaml",
            wantErr: true,
        },
        {
            name:    "file not found",
            input:   "testdata/nonexistent.yaml",
            wantErr: true,
        },
    }

    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            got, err := ParseCollection(tt.input)
            if (err != nil) != tt.wantErr {
                t.Fatalf("error = %v, wantErr %v", err, tt.wantErr)
            }
            if !tt.wantErr && !reflect.DeepEqual(got, tt.want) {
                t.Errorf("got %+v, want %+v", got, tt.want)
            }
        })
    }
}
```

---

## Step 6: Subtests with Hierarchy

Group related tests with nested `t.Run()`:

```go
func TestVariableResolver(t *testing.T) {
    t.Run("interpolation", func(t *testing.T) {
        t.Run("simple variable", func(t *testing.T) { /* ... */ })
        t.Run("nested variable", func(t *testing.T) { /* ... */ })
        t.Run("undefined variable", func(t *testing.T) { /* ... */ })
    })

    t.Run("resolution order", func(t *testing.T) {
        t.Run("cli overrides env", func(t *testing.T) { /* ... */ })
        t.Run("env overrides file", func(t *testing.T) { /* ... */ })
    })
}
```

---

## Step 7: Error Assertion Patterns

### Sentinel errors (use `errors.Is`)
```go
func TestParseCollection_CircularRef(t *testing.T) {
    _, err := ParseCollection("testdata/circular.yaml")
    if !errors.Is(err, ErrCircularReference) {
        t.Errorf("got %v, want ErrCircularReference", err)
    }
}
```

### Error context (use `strings.Contains` or error message check)
```go
func TestResolveVariable_Undefined(t *testing.T) {
    _, err := Resolve("${undefined_var}", vars)
    if err == nil {
        t.Fatal("expected error for undefined variable")
    }
    if !strings.Contains(err.Error(), "undefined_var") {
        t.Errorf("error should mention variable name, got: %v", err)
    }
}
```

### Wrapped errors (use `errors.As`)
```go
func TestHTTPExecute_Timeout(t *testing.T) {
    _, err := Execute(ctx, req)
    var netErr *net.OpError
    if !errors.As(err, &netErr) {
        t.Errorf("expected net.OpError, got %T: %v", err, err)
    }
}
```

---

## Step 8: Integration Tests

Build the real binary and exercise it end-to-end:

```go
func TestIntegration_RunCommand(t *testing.T) {
    if testing.Short() {
        t.Skip("skipping integration test in short mode")
    }

    // Build binary
    binary := filepath.Join(t.TempDir(), "curlew")
    build := exec.Command("go", "build", "-o", binary, "./cmd/curlew")
    if out, err := build.CombinedOutput(); err != nil {
        t.Fatalf("build failed: %v\n%s", err, out)
    }

    // Run with test fixture
    cmd := exec.Command(binary, "run", "testdata/simple.yaml")
    cmd.Env = append(os.Environ(), "API_BASE_URL=http://localhost:8080")
    out, err := cmd.CombinedOutput()

    // Assert
    if err != nil {
        t.Fatalf("command failed: %v\n%s", err, out)
    }
    if !strings.Contains(string(out), "PASS") {
        t.Errorf("expected PASS in output, got: %s", out)
    }
}
```

---

## Step 9: testdata Management

Conventions for test fixtures:
- Place in `testdata/` directory within the package being tested
- Go tooling automatically ignores `testdata/` directories
- Name files descriptively: `valid_single_request.yaml`, `invalid_missing_url.yaml`
- For golden file tests, use `.golden` extension
- Keep fixtures minimal — only the data needed for the test case

```
internal/parser/testdata/
├── valid_single_request.yaml
├── valid_multi_request.yaml
├── valid_with_variables.yaml
├── invalid_empty.yaml
├── invalid_malformed.yaml
└── expected_output.golden
```

---

## Step 10: Parallel Tests

Use `t.Parallel()` when safe:

```go
func TestParseCollection(t *testing.T) {
    t.Parallel() // Mark parent as parallel

    tests := []struct{ /* ... */ }{}
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            t.Parallel() // Mark each subtest as parallel
            // ... test body (must not share mutable state)
        })
    }
}
```

**When `t.Parallel()` is safe:**
- Test reads only from `testdata/` (immutable)
- Test uses `t.TempDir()` for writable files
- Test does not depend on package-level mutable state
- Test does not depend on execution order

**When `t.Parallel()` is NOT safe:**
- Test modifies package-level state
- Test uses a shared resource (port, file) without isolation
- Test depends on sequential ordering

---

## Step 11: Run and Verify

Run all tests:
```bash
go test ./...
```

Run with race detector:
```bash
go test -race ./...
```

Run specific test:
```bash
go test -v -run TestFunctionName ./internal/package/
```

Run with coverage:
```bash
go test -coverprofile=coverage.out ./...
go tool cover -func=coverage.out
```

Generate HTML coverage report:
```bash
go tool cover -html=coverage.out -o coverage.html
```

---

## Step 12: Identify Test Cases

For the target under test, ensure coverage of:

1. **Happy path** — normal, expected operation
2. **Edge cases** — empty input, single element, maximum size, boundary values
3. **Error cases** — invalid input, missing files, malformed data, permission denied
4. **State variations** — different configurations, flag combinations
5. **Nil handling** — nil pointers, nil slices, nil maps
6. **Concurrency** — if the code is used concurrently, test with `-race`

---

## Test Checklist

- [ ] Every public function has at least one test
- [ ] Error paths tested (not just happy path)
- [ ] Edge cases covered (empty, nil, boundary)
- [ ] Table-driven tests used for multiple similar cases
- [ ] Subtests use `t.Run()` with descriptive names
- [ ] `t.Fatalf` for fatal setup errors, `t.Errorf` for assertion failures
- [ ] `testdata/` used for file-based fixtures
- [ ] Integration test exercises real binary (if applicable)
- [ ] `t.Parallel()` used where safe
- [ ] No test depends on execution order
- [ ] Tests run cleanly with `go test -race ./...`
