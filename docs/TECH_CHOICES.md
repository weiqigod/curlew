# Tech Choices: API Testing Tool

## Overview

This document records the technology decisions, tooling conventions, and development process for the API Testing Tool. It sits alongside the [Development Philosophy](./development-philosophy.md) (always-runnable, vertical slices) and the [Specification](./api-testing-tool-specification-v4.md). A new developer should be able to read these three documents and start contributing.

The project has two codebases: a CLI tool and a backend service. They share a development process but differ in language and ecosystem.

---

## CLI — Go

### Language

Go, latest stable release. The CLI is distributed as a single statically-linked binary with no runtime dependencies. This aligns with the spec's emphasis on "build the binary, run it, no setup."

### Project Structure

Follow the standard Go project layout:

```
cmd/
    apitest/          # main package — entry point only
internal/
    parser/           # collection file parsing
    http/             # HTTP client, request execution
    variable/         # variable system, interpolation engine
    assertion/        # assertion evaluation
    output/           # terminal, JSON, XML, HTML formatters
    auth/             # authentication handlers
    config/           # configuration loading, .env, precedence
pkg/                  # public library code (if any surfaces)
testdata/             # sample project, fixtures
go.mod
go.sum
```

`internal/` enforces encapsulation at the compiler level — nothing outside the module can import these packages. Each package owns its domain and exposes a narrow interface.

### Tooling

**Build:** `go build ./cmd/apitest` — one command, one binary. Cross-compilation via `GOOS` and `GOARCH` environment variables. No Makefiles unless task complexity justifies one.

**Testing:** The standard `testing` package. No third-party test frameworks. Use `testify/assert` only if the team finds raw `if` checks too noisy — but prefer the standard library first. Table-driven tests are the default pattern for any function with more than two interesting inputs.

**Integration tests:** Following the development philosophy's completeness contract, every slice includes at least one test that builds the binary and invokes it as a subprocess with real input files, asserting on stdout, stderr, and exit codes. Use `os/exec` and `testdata/` fixtures for this.

**Linting:** `golangci-lint` with a checked-in `.golangci.yml`. Enable at minimum: `govet`, `staticcheck`, `errcheck`, `gosimple`, `ineffassign`, `unused`. Add `gofumpt` for formatting stricter than `gofmt`.

**Code coverage:** `go test -coverprofile` piped into `go tool cover`. 80% line coverage enforced in CI; see the Test Coverage section under Development Process for the full policy.

**Documentation:** `godoc` conventions. Every exported symbol gets a comment. Package-level doc comments explain purpose and usage.

### Conventions

- **Error handling:** Return errors, don't panic. Wrap errors with `fmt.Errorf("context: %w", err)` to build traceable chains. Sentinel errors for well-known failure modes (e.g., `ErrCircularReference`, `ErrUnsupportedFeature`).
- **Naming:** Follow Effective Go. Short variable names in tight scopes, descriptive names for package-level identifiers. No stuttering (`parser.Parser` is fine, `parser.ParserConfig` is not — use `parser.Config`).
- **Dependencies:** Minimise external dependencies. The standard library covers HTTP, JSON, YAML (with one library), CLI argument parsing, and testing. Evaluate every `go get` against the cost of the dependency.
- **Concurrency:** Use goroutines and channels where the spec calls for parallelism (e.g., independent request execution). Always use `context.Context` for cancellation and timeouts. No shared mutable state — pass data through channels or protect with clearly scoped mutexes.

---

## Backend — C# / .NET

### Language and Runtime

C# on the latest stable .NET release (currently .NET 9). The backend handles authentication and dashboards as described in Phase 3+ of the spec.

### Project Structure

Follow the standard .NET solution layout:

```
ApiTestBackend.sln
src/
    ApiTestBackend.Api/           # ASP.NET Core web host, controllers/endpoints
    ApiTestBackend.Core/          # domain logic, interfaces, no infrastructure deps
    ApiTestBackend.Infrastructure/ # database, external services, auth providers
    ApiTestBackend.Contracts/     # shared DTOs, API contracts (consumed by CLI too)
tests/
    ApiTestBackend.Api.Tests/
    ApiTestBackend.Core.Tests/
    ApiTestBackend.Infrastructure.Tests/
    ApiTestBackend.Integration.Tests/  # end-to-end against running server
```

Clean Architecture layering: Core has zero dependencies on Infrastructure or Api. Dependencies point inward. Infrastructure implements interfaces defined in Core.

### Tooling

**Build:** `dotnet build` / `dotnet publish`. Single-file deployment where feasible. Target Linux containers for production.

**Testing:** xUnit as the test framework. FluentAssertions for readable assertion syntax. Use `WebApplicationFactory<T>` for integration tests that spin up the API in-process — fast, no port conflicts, no external process management.

**Linting and analysis:** Enable the .NET analyzers that ship with the SDK. Add `StyleCop.Analyzers` or `Meziantou.Analyzers` for additional code style enforcement. Treat warnings as errors in CI.

**Code coverage:** Coverlet integrated with `dotnet test`. 80% line coverage enforced in CI; see the Test Coverage section under Development Process for the full policy.

**API documentation:** Use minimal APIs or controllers with XML doc comments. Generate OpenAPI specs automatically.

### Conventions

- **Nullable reference types:** Enabled project-wide. No `null` where the type says non-null.
- **Async all the way:** All I/O-bound operations are async. No `.Result` or `.Wait()` blocking calls.
- **Dependency injection:** Use the built-in DI container. Register services with appropriate lifetimes. Constructor injection only — no service locator pattern.
- **Configuration:** Use the Options pattern (`IOptions<T>`) for strongly-typed configuration. Secrets via user-secrets in development, environment variables or vault in production.
- **Error handling:** Use Result types or problem details (RFC 9457) for API error responses. Exceptions for truly exceptional conditions, not control flow.

---

## Development Process — Both Codebases

### TDD: Red-Green-Refactor

Every feature, bug fix, and behavioural change follows the red-green-refactor cycle:

**Red.** Write a failing test that describes the desired behaviour. The test must fail for the right reason — it should compile, run, and produce a clear assertion failure that demonstrates the behaviour doesn't exist yet. If you can't write the test, you don't understand the requirement well enough to write the code.

**Green.** Write the minimum code to make the test pass. No abstractions, no cleverness, no "while I'm here" additions. The goal is a passing test as fast as possible. Ugly is fine. Duplication is fine. You're proving the behaviour works.

**Refactor.** With green tests as your safety net, improve the code. Extract duplication, introduce abstractions, rename for clarity, simplify control flow. The tests must stay green throughout. If a refactoring breaks a test, you've changed behaviour — back up and rethink.

### The Cycle in Practice

A single slice from the development philosophy maps to multiple red-green-refactor cycles. A slice like "parse a minimal collection file and execute one GET request" might involve:

1. Red: test that the parser rejects an empty file with a clear error → Green → Refactor
2. Red: test that the parser extracts a single request from a minimal collection → Green → Refactor
3. Red: test that the HTTP executor sends a GET and returns status code → Green → Refactor
4. Red: test that the CLI wires parser to executor and prints the result → Green → Refactor
5. Red: integration test that builds the binary, runs it against a fixture file, asserts on output → Green → Refactor

Each cycle is small — minutes, not hours. The integration test at the end satisfies the development philosophy's completeness contract.

### What TDD Does Not Mean

- It does not mean writing tests for internal implementation details. Test behaviour through public interfaces.
- It does not mean 100% code coverage. Coverage is a side effect of good TDD, not the goal.
- It does not mean you can't spike. When exploring an unfamiliar API or library, spike freely — then throw the spike away and TDD the real implementation.
- It does not mean slow. Small cycles with fast tests are faster than writing code first and debugging later.

### Commit Discipline

Each completed red-green-refactor cycle (or small group of related cycles) is a commit. The always-runnable constraint from the development philosophy applies: every commit on main builds, runs, and passes all tests. Commits are small, focused, and frequent.

### Code Review

All changes go through review. Reviewers check:

- Does the test actually describe the stated behaviour?
- Does the implementation satisfy the test without overbuilding?
- Is the code idiomatic for the language (Go or C#)?
- Does the refactoring step show up in the diff? (If there's no refactoring, was it actually not needed, or was it skipped?)

### CI Pipeline

Both codebases run the same logical pipeline on every push:

1. **Lint** — catch style and correctness issues before compilation.
2. **Build** — compile the binary / publish the application.
3. **Unit tests** — fast, isolated, run in seconds.
4. **Integration tests** — build the real artefact, run it against fixtures or spin up the server.
5. **Coverage gate** — 80% minimum enforced; build fails below this threshold.

The pipeline must pass before merging to main. This is non-negotiable — it's how the always-runnable constraint is enforced mechanically.

### Test Coverage

TDD naturally produces high coverage, but the CI pipeline enforces a floor to catch lapses in discipline.

**Hard gate:** 80% line coverage. The build fails below this. This is the safety net — it catches situations where someone writes code without going through the red-green-refactor cycle.

**Expectation:** Real coverage should sit in the 85–95% range as a natural consequence of disciplined TDD. If coverage is consistently at 80%, that's a signal the process is slipping, not that the threshold needs lowering.

**What counts:** Line coverage across the entire codebase, measured by `go test -coverprofile` (Go) and Coverlet (C#/.NET). Generated code, test helpers, and main entry points may be excluded with standard tooling conventions.

**What doesn't count as coverage:** Tests that exercise lines without asserting on behaviour are noise. Code review is the mechanism for catching this — reviewers should flag tests that exist only to inflate the number.

The specific CI platform (GitHub Actions, GitLab CI, etc.) is not prescribed here. Pick one and keep the configuration simple.

### Manual Smoke Test — The "Run It and See" Step

Automated tests prove correctness. The smoke test proves the experience. After completing a slice, you build the binary, run it against a real scenario, and watch it behave. This is not optional and not something that gets added later — it exists from the first commit that produces a runnable binary.

#### CLI

A `smoke/` directory lives in the repo root. It contains real collection files, environment configs, and test data that exercise whatever the current build supports. A single script runs the full scenario:

```
./smoke/run.sh
```

This script builds the binary from source, runs it against the collection files in `smoke/`, and lets the output print to the terminal unfiltered. No assertions, no pass/fail — you read the output yourself. You see the HTTP requests go out, the responses come back, the assertions evaluate, the errors format. You watch the tool do its job.

The smoke directory grows with the tool, exactly like the sample project described in the development philosophy. When variable interpolation lands, the smoke files start using variables. When authentication lands, there's a smoke scenario that authenticates. The smoke directory is always an accurate picture of what the tool can do right now, exercised end to end.

**The rule:** if you finish a slice and `./smoke/run.sh` doesn't demonstrate the new behaviour working in a realistic context, the slice isn't done.

#### Backend

Same concept, different shape. A `smoke/` directory in the backend repo contains a script that:

```
./smoke/run.sh
```

This starts the server, waits for the health check to pass, runs a series of real HTTP requests against it (using `curl` or the CLI tool itself once it can talk to the backend), prints the responses, and shuts the server down. You see the API respond to real requests with real data.

#### Why This Is Separate From Tests

Automated tests run in CI and produce a binary pass/fail. They're essential but they're not the same as using the tool. The smoke test is the developer putting themselves in the user's seat for thirty seconds after every slice. It catches things tests don't: awkward output formatting, confusing error messages, slow startup, a workflow that technically works but feels wrong. It's the feedback loop the development philosophy talks about — running the actual artefact you're shipping, not a simulation of it.

---

## Decisions Not Yet Made

The following will be decided when they become relevant, not before:

- **Database** for the backend (PostgreSQL is the likely default, but Phase 3 will clarify requirements)
- **Container orchestration** (single container may suffice initially)
- **Monitoring and observability** stack
- **Release and versioning** strategy (semantic versioning is assumed but cadence is TBD)
- **CI platform** selection

These are recorded here so they aren't forgotten, but premature decisions on infrastructure create constraints without evidence. Decide when the first slice that needs them arrives.
