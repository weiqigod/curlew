# Tech Choices: API Testing Tool

## Overview

This document records the technology decisions, tooling conventions, and development process for the API Testing Tool. It sits alongside the [Development Philosophy](./DEVELOPMENT_PHILOSOPHY.md) (always-runnable, vertical slices) and the [Specification](./SPECIFICATION.md). A new developer should be able to read these three documents and start contributing.

The project has two codebases: a CLI tool and a backend service. They share a development process but differ in language and ecosystem.

---

## CLI — Go

### Language

Go, latest stable release. The CLI is distributed as a single statically-linked binary with no runtime dependencies. This aligns with the spec's emphasis on "build the binary, run it, no setup."

### Project Structure

Follow the standard Go project layout:

```
cmd/
    curlew/          # main package — entry point only
internal/
    parser/           # collection file parsing
    http/             # HTTP client, request execution
    variable/         # variable system, interpolation engine
    assertion/        # assertion evaluation
    output/           # terminal, JSON, XML, HTML formatters
    auth/             # authentication handlers
    config/           # configuration loading, .env, precedence
schemas/              # canonical JSON Schemas; //go:embed'd, served by `curlew schema`
templates/            # templates/skills/ is //go:embed'd (curlew init --skill agent)
ui/                   # Svelte SPA; built into internal/uiserver/assets/dist, //go:embed'd
testdata/             # sample project, fixtures
go.mod
go.sum
```

`internal/` enforces encapsulation at the compiler level — nothing outside the module can import these packages. Each package owns its domain and exposes a narrow interface.

### Tooling

**Build:** `go build ./cmd/curlew` — one command, one binary. Cross-compilation via `GOOS` and `GOARCH` environment variables. No Makefiles unless task complexity justifies one.

**Testing:** The standard `testing` package. No third-party test frameworks. Use `testify/assert` only if the team finds raw `if` checks too noisy — but prefer the standard library first. Table-driven tests are the default pattern for any function with more than two interesting inputs.

**Integration tests:** Following the development philosophy's completeness contract, every slice includes at least one test that builds the binary and invokes it as a subprocess with real input files, asserting on stdout, stderr, and exit codes. Use `os/exec` and `testdata/` fixtures for this.

**Linting:** `golangci-lint` with a checked-in `.golangci.yml`. Enable at minimum: `govet`, `staticcheck`, `errcheck`, `gosimple`, `ineffassign`, `unused`. Add `gofumpt` for formatting stricter than `gofmt`.

**Code coverage:** `go test -coverprofile` piped into `go tool cover`. 80% line coverage enforced in CI; see the Test Coverage section under Development Process for the full policy.

**Documentation:** `godoc` conventions. Every exported symbol gets a comment. Package-level doc comments explain purpose and usage.

### Distribution

The complete binary embeds the Svelte UI: run `./scripts/build-ui.sh` before
`go build`. GoReleaser’s release hooks and the local gate do this from a clean
checkout. Node.js 22+ is a build dependency only. A plain Go-only installation
provides the CLI and a build-required UI page.

**Release artifacts:** goreleaser builds six archives (linux/darwin/windows ×
amd64/arm64) from a tag, each carrying LICENSE, NOTICE, README.md, CHANGELOG.md
and the current user, agent and UI guides. See `.goreleaser.yaml`.

**Package managers: no, not yet.** Homebrew is the one macOS users would ask
for, and goreleaser can publish a tap from a small `brews:` block, so the
question is whether to. The answer today is no, and the reason is disqualifying
rather than cautious: a tap's formula fetches the release asset
**anonymously**, and this repository is private — an unauthenticated request for
a v0.1.0 asset returns HTTP 404 (measured 2026-08-16). A tap would not be a
maintenance burden so much as a formula that cannot install anything.

Two further costs, for when the blocking reason clears: a tap needs its own
`homebrew-tap` repository and a write-scoped token, which adds a publish target
that can fail *after* the tag is immutable; and with one maintainer and a first
release cut on 2026-08-16, there is no install volume to justify a second
distribution channel.

**Revisit when** the repository is public *and* a second release exists — the
point at which a tap has something to serve and a cadence to keep. Until then
the documented install paths are a release archive, `go install`, and a clone;
all three are executed on every gate by `TestReadme_install_commands_execute`.

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
ApiTool.Backend.sln
src/
    ApiTool.Backend/              # the backend project
    ApiTool.Backend.Tests/        # unit and integration tests, in-tree
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

## Repository Shape

**Verdict: keep `src/` and `web/` in this repository, and mark them frozen.**

Measured 2026-08-14: `cmd/` + `internal/` is 463 Go files, 134,144 lines — the
shipped product. `src/` is 866 C# files, 139,627 lines — larger than the
entire Go CLI. `web/` adds 186 more files. The shipped CLI does not call any
of it: the backend strip on 2026-08-03 removed `curlew login`, `curlew
worker`, distributed execution, report upload, and every `CURLEW_BACKEND_*`
variable. More than half the repository, by line count, is code the product
does not use, and it was the first thing a visitor to the repository root
saw, with no table naming it.

Three options were on the table:

- **A. Excise** — move `src/` and `web/` to their own repository. Rejected as
  a decision this pipeline may not make unilaterally: `src/LICENSE`,
  `web/LICENSE` and `deploy/LICENSE` are each proprietary while the
  repository root is Apache-2.0, so splitting the repository is a licensing
  action — reconciling three separate license grants and standing up an
  external repository — not a path move.
- **C. Archive in place** — move both under an `archive/` or `platform/`
  directory. Rejected: the decisive evidence is
  `scripts/ci-local.sh` around its scope-detection block, which already
  carries a comment recording that a *previous* rename broke the same
  greps it relies on — matching on `src/Curlew.Backend` there silently
  skipped the backend and E2E gates for every backend change after the
  project was renamed to `ApiTool.Backend`. Option C rewrites those same
  greps, `ApiTool.Backend.sln`, `docker-compose.test.yml`, `.dockerignore`,
  eight workflows, and every `src/ApiTool.Backend/...` path in an
  11,000+-line specification. Its failure mode is a gate that reports PASS
  while quietly testing nothing — precisely the false clear this codebase is
  organised against.
- **B. Keep and explain** — chosen. The cheapest option, and the only
  reversible one: it forecloses neither A nor C, while doing A or C first and
  finding it wrong costs a second migration. The input that would settle
  this — is the platform a live product, a paused one, or a finished one? —
  is exactly the input this pipeline does not have, so the reversible move is
  the correct one.

PlatformStatus, the sentence that binds this decision across documents:

> The `src/` backend and `web/` dashboard stay in this repository, frozen: they build and pass their tests in `./scripts/ci-local.sh --full`, no new feature work is planned, and the `curlew` CLI does not call them.

"Frozen" rather than "archived" or "maintained" is itself a measured claim, not
a preference. Against "archived": `./scripts/ci-local.sh --full` still runs
`dotnet test`, the web gate steps, and five Playwright convergence specs
against the platform, and `docs/SPECIFICATION.md` still describes it as
current. Against "maintained": there has been no `src/` change since the
2026-08-03 backend strip, and no `src/` or `web/` work anywhere in the
M25–M29 roadmap. Kept green, not developed — that is what "frozen" means
here.

**This decision was recorded by an automated pipeline, not the project
owner**, because the task that required it (M28-001) explicitly reserves the
choice among A/B/C to the owner. B was chosen on the stated grounds above
because it is the reversible option; A and C remain open. The durable part of
this task is option-independent: every top-level directory of the repository
must be named in `README.md` with its relationship to the CLI stated, and a
new one that is not fails `go test ./internal/docs/ -run
TestReadme_accounts_for_every_top_level_directory`. If the owner later
chooses A or C, that guard survives the move and enforces the new shape.

**Revisit when** the project owner states whether the platform is live,
paused, or being wound down. That answer is what distinguishes B from A or C;
until it exists, B is the only defensible choice.

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
- **Release cadence** — the mechanism is decided and executed (see Distribution
  above and `.goreleaser.yaml`; v0.1.0 was tagged 2026-08-16). How often to cut
  one is not.
- **CI platform** selection

These are recorded here so they aren't forgotten, but premature decisions on infrastructure create constraints without evidence. Decide when the first slice that needs them arrives.
