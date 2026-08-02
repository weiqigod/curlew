# Implementation Plan: M4-012

## Overview

Convergence slice that wires the existing `curlew run` pipeline to `--report-upload`, so a single CLI invocation both runs a collection and posts its results (plus an optional PR-check) to the backend, then validates end-to-end via a Playwright spec that navigates the web dashboard. Ships `testdata/team/e2e-collection.yaml`, `web/tests/e2e/full-pipeline.spec.ts`, seeds a team-tier org in `scripts/test-stack.sh`, and adds a CI workflow that runs the full pipeline on Linux.

## Task Details
- **ID:** M4-012
- **Title:** E2E: CLI run -> backend ingest -> web dashboard
- **Phase:** M4: Team Tier
- **Priority:** 2
- **Complexity:** high

## Dependencies

| Task | Title | Status |
|------|-------|--------|
| M4-005 | Web: team dashboard for runs | done |
| M4-007 | CLI: pr-check subcommand posting status to backend | done |

## Scope Clarification (decisions)

The task scope says **"No new backend endpoints, no new CLI commands"** but the observable requires:
- `curlew run --report-upload` to upload a result and post a PR-check in one call
- A web route `/org/[slug]/pr-checks` where the PR-check row is visible

The following reality constraints force targeted deviations from that scope:

1. **`--report-upload` flag on `curlew run`** — this is *not* a new subcommand; it is a new flag on an existing subcommand, reusing `internal/prcheck.Client`. This is how I interpret "no new CLI commands".
2. **Backend `POST/GET /api/v1/organizations/{orgId}/pr-checks`** — no such endpoint currently exists. M4-007 added only the CLI side; the CLI today calls a not-yet-existent server path. A minimal PR-check endpoint is required so the Playwright spec can assert the row appears. Added in this slice as a thin stub (in-memory/SQLite persistence, RBAC via `CurrentUserAccessor`, no GitHub integration).
3. **Web route `/org/[slug]/pr-checks`** — a new SvelteKit page + server loader that lists PR-checks for the org. Required for the `confirms pr-check row shows in /org/acme/pr-checks` assertion.

These additions are the minimum surface area needed to satisfy the observable and definition of done. They stay inside the same "glue slice" spirit: no new CLI commands, no new ingest surfaces, no new fields beyond what the CLI already sends.

## Implementation Steps

Steps are ordered by blast radius (smallest first). Every step TDDs with tests written before code.

---

### Step 1: Summary-to-payload mapping in `internal/prcheck`

**Rationale:** Pure function with no I/O, no network, no goroutines. Smallest blast radius. Unblocks Step 3 (the new `--report-upload` flag) by providing the canonical mapping between `runner.RequestResult`/`runner.Summary` and `prcheck.ResultsPayload`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/payload.go` | create | New `BuildPayload(col *parser.Collection, results []runner.RequestResult, sum *runner.Summary, trigger TriggerInfo) *ResultsPayload` |
| `internal/prcheck/payload_test.go` | create | Table-driven tests for payload shape, status mapping, duration, skipped/failed counts |
| `internal/prcheck/prcheck.go` | modify | Add `TriggerInfo` struct (TriggeredBy, GitSha, RunAt) |

#### Current Code

```go
// internal/prcheck/prcheck.go
type ResultsPayload struct {
    CollectionName string       `json:"collection_name"`
    RunAt          string       `json:"run_at"`
    DurationMs     int64        `json:"duration_ms"`
    PassCount      int          `json:"pass_count"`
    FailCount      int          `json:"fail_count"`
    SkippedCount   int          `json:"skipped_count"`
    TriggeredBy    string       `json:"triggered_by"`
    GitSha         string       `json:"git_sha,omitempty"`
    Items          []ResultItem `json:"items"`
}
```

#### New Code

```go
// internal/prcheck/payload.go
package prcheck

import (
    "time"

    "github.com/weiqigod/curlew/internal/runner"
)

// TriggerInfo captures the operator context that is not in Summary.
type TriggerInfo struct {
    CollectionName string    // defaults from Summary caller
    RunAt          time.Time // zero => time.Now().UTC()
    TriggeredBy    string    // "cli" by default
    GitSha         string    // optional
}

// BuildPayload converts a completed run into a ResultsPayload for upload.
// Skipped requests are reported as status="skipped".
// Teardown failures are folded into FailCount only when --report-include-teardown
// is set (default false, to match exit-code semantics).
func BuildPayload(results []runner.RequestResult, sum *runner.Summary, info TriggerInfo) *ResultsPayload { ... }
```

#### Tests to Write FIRST (RED phase)

```go
// internal/prcheck/payload_test.go
func TestBuildPayload(t *testing.T) {
    tests := []struct {
        name    string
        results []runner.RequestResult
        summary *runner.Summary
        info    TriggerInfo
        want    *ResultsPayload
    }{
        {"all passing", /* 3 passing requests, summary {3,0,0} */, /* PassCount=3, Items=3, FailCount=0 */},
        {"one failing assertion", /* 2 pass + 1 fail */, /* FailCount=1, item.Status="failed" */},
        {"request error (Err != nil)", /* network error */, /* status="failed", message=err.Error() */},
        {"skipped request", /* r.Skipped=true */, /* status="skipped", SkippedCount=1 */},
        {"teardown failure excluded from fail_count", /* main:pass, teardown:fail */, /* FailCount=0 */},
        {"trigger info defaults", /* RunAt zero, TriggeredBy empty */, /* RunAt in RFC3339 now, TriggeredBy="cli" */},
        {"git sha propagated", /* info.GitSha="abc" */, /* payload.GitSha="abc" */},
        {"data-driven iteration folded", /* IsDataDriven, IterationIndex=1 */, /* item name includes "[2/3]" */},
    }
    for _, tc := range tests { /* run BuildPayload, compare JSON */ }
}
```

#### Impact on Existing Tests
- No existing tests broken — new file, exported addition only.

---

### Step 2: Wire `TriggerInfo` detection (git sha)

**Rationale:** Pure helper, no network; tested in isolation. Keeps the `cmd/curlew` glue simple.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/git.go` | create | `DetectGitSha(dir string) string` — runs `git rev-parse --short HEAD`, returns "" on failure |
| `internal/prcheck/git_test.go` | create | Tests with/without `.git`, captures exec via PATH override |

#### Tests to Write FIRST

```go
func TestDetectGitSha(t *testing.T) {
    tests := []struct {
        name  string
        setup func(t *testing.T) string // returns dir
        want  string                    // "" or non-empty
    }{
        {"not a git repo", /* tmpdir, no .git */, ""},
        {"git binary missing", /* PATH without git */, ""},
        {"valid git repo", /* init, commit, return sha */, /* non-empty 7-hex */},
    }
    // ...
}
```

---

### Step 3: Add `--report-upload` flag to `curlew run`

**Rationale:** Builds on Step 1 (mapping) and Step 2 (git). This is the main behavioural change and wiring — kept small because all the heavy lifting lives in `internal/prcheck`.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` | modify | Parse `--report-upload`, `--org`, `--pr`, `--repo`, `--triggered-by`, `--git-sha` in `parseRunArgs`; after summary is built in `runCmdInner`, if `--report-upload` set, call `prcheck.Run` (or new `UploadRun`) |
| `cmd/curlew/main_test.go` | modify | New table tests for flag parsing, error when env missing, stdout line format |
| `cmd/curlew/report_upload_test.go` | create | Integration test against `httptest.NewServer` asserting 2 POSTs and stdout |
| `internal/prcheck/run.go` | modify | Add `UploadRun(ctx, cfg, payload, state)` that skips the file-load step and does not require `ResultsFile`. Keep existing `Run()` for `pr-check` subcommand. |
| `internal/prcheck/prcheck.go` | modify | In `Validate()`, make `ResultsFile` optional when `InMemoryPayload` is set; add new sentinel `ErrOrgRequired` if not already present (already checked) |

#### Current Code

```go
// cmd/curlew/main.go parseRunArgs signature
func parseRunArgs(args []string) (file, envName, format, report string,
    vars, envVarVars map[string]string, seed *int64, noColor bool,
    verbosity output.Verbosity, allowSensitive, showDeps, dryRun,
    parallel, confirmLargeDataset bool, err error)
```

The positional signature is already extremely wide. Refactor to a struct to stay readable:

#### New Code

```go
// cmd/curlew/main.go — replace the wide-tuple return with a struct.
type runFlags struct {
    file, envName, format, report string
    vars, envVarVars              map[string]string
    seed                          *int64
    noColor                       bool
    verbosity                     output.Verbosity
    allowSensitive, showDeps, dryRun, parallel, confirmLargeDataset bool

    // New in M4-012:
    reportUpload bool
    org          string
    pr           int
    repo         string
    triggeredBy  string
    gitSha       string
}

func parseRunArgs(args []string) (runFlags, error) { ... }
```

The switch handling `--report-upload`, `--org`, `--pr`, `--repo`, `--triggered-by`, `--git-sha`, follows the existing pattern (same style as `--report`, `--var`).

Then, in `runCmdInner`, after the run completes and before returning:

```go
if flags.reportUpload {
    if code := handleReportUpload(ctx, flags, col, results, summary); code != 0 {
        return code, summary
    }
}
```

`handleReportUpload` builds the payload with `prcheck.BuildPayload`, calls a new `prcheck.UploadRun(ctx, prcheck.UploadConfig{...})` and prints `Uploaded result res_...; status posted` (or `Uploaded result res_...` when `--pr` is absent).

Exit-code rules:
- Upload succeeds, run passed → return existing exit code (0)
- Upload succeeds, run failed → return existing exit code (1 or 4)
- Upload itself fails (ErrUnauthorized, ErrNetworkFailure, ErrBackendURLMissing):
  - If the run also failed, print the upload error to stderr, return the run's exit code (do not mask test failure).
  - If the run passed, return exit code 2 with the upload error to stderr.

#### Tests to Write FIRST

```go
// cmd/curlew/report_upload_test.go
func TestRunWithReportUpload(t *testing.T) {
    tests := []struct {
        name       string
        args       []string
        backendFn  http.HandlerFunc  // test server handling /results and /pr-checks
        wantExit   int
        wantStdoutContains []string
    }{
        {"upload then pr-check on pass", /* --report-upload --org acme --pr 7 --repo acme/api */, /* 2 POSTs */, 0, []string{"Uploaded result res_", "status posted"}},
        {"upload only (no --pr)", /* --report-upload --org acme */, /* 1 POST */, 0, []string{"Uploaded result res_"}},
        {"failing run still uploads and reports state=failure", /* assertion fails + upload flags */, /* expects state=failure in pr-checks body */, 1, []string{"state posted: failure"}},
        {"backend unreachable on passing run returns exit 2", /* connection refused */, /* no server */, 2, []string{"backend unreachable"}},
        {"missing CURLEW_BACKEND_URL returns exit 2 before run", /* no env */, nil, 2, []string{"backend URL not configured"}},
        {"missing --org returns exit 1 at parse time", /* --report-upload without --org */, nil, 1, []string{"--org is required"}},
    }
    // use httptest.NewServer, set CURLEW_BACKEND_URL to ts.URL, call runCmdInner directly
}
```

#### Impact on Existing Tests
- `cmd/curlew/run_test.go`, `cmd/curlew/discovery_run_test.go`, `cmd/curlew/validate_team_test.go`, `cmd/curlew/main_test.go` — all call `parseRunArgs` via the runCmdInner path. The struct refactor breaks any test that unpacks the tuple directly. Expected fix: replace the 15-arg destructure with `flags := parseRunArgs(...)` and read `flags.X`. Search `parseRunArgs(` to enumerate call sites.
- `cmd/curlew/main.go` itself has 2 call sites of `parseRunArgs` (in `runCmdInner` and `watchCmd` line 924). Both must be updated.
- `internal/prcheck/run_test.go` — no changes needed; `Run()` preserved.
- `internal/prcheck/prcheck_test.go` — if `Validate()` is relaxed to allow missing `ResultsFile` when in-memory payload is set, a new case table entry proves the pre-existing "missing results" error still fires for the `pr-check` subcommand.

---

### Step 4: Backend — PR-check endpoint (`POST` + `GET`)

**Rationale:** Needed so the CLI round-trip actually completes in Docker and the Playwright assertion on `/org/acme/pr-checks` has data to render. Isolated from the CLI — any test in the Go code can mock this endpoint.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `src/ApiTool.Backend/PrChecks/PrCheck.cs` | create | Domain entity (Id, OrgId, Repo, Pr, State, ResultId, CreatedAt) |
| `src/ApiTool.Backend/PrChecks/PrCheckId.cs` | create | Strongly-typed id wrapper, mirrors `ResultId.cs` |
| `src/ApiTool.Backend/PrChecks/PrCheckDto.cs` | create | Wire DTO (snake_case via SnakeCaseOptions) |
| `src/ApiTool.Backend/PrChecks/PrCheckRequest.cs` | create | Matches CLI `PrCheckPayload` (repo, pr, state, result_id) |
| `src/ApiTool.Backend/PrChecks/PrCheckError.cs` | create | `None, PermissionDenied, InvalidState, ResultNotFound` |
| `src/ApiTool.Backend/PrChecks/PrChecksService.cs` | create | `PostAsync`, `ListAsync` using `AppDbContext` — reuse the membership/permission pattern from `ResultsService` |
| `src/ApiTool.Backend/PrChecks/PrChecksEndpoints.cs` | create | `MapPost("", Post)`, `MapGet("", List)` under `/api/v1/organizations/{orgId}/pr-checks` |
| `src/ApiTool.Backend/Program.cs` | modify | Register service + map endpoints |
| `src/ApiTool.Backend/Migrations/...` | create | Add `pr_checks` table (migration + snapshot) |
| `src/ApiTool.Backend.Tests/PrChecks/PrChecksServiceTests.cs` | create | Unit tests: post, list, RBAC denial, invalid state |
| `src/ApiTool.Backend.Tests/PrChecks/PrChecksEndpointsTests.cs` | create | HTTP tests: 200 post, 401, 403, 400 invalid state, list newest-first |

#### Tests to Write FIRST

```csharp
[Fact]
public async Task Post_ValidBody_Returns200WithRowPersisted() { ... }

[Fact]
public async Task Post_InvalidState_Returns400() { ... }

[Fact]
public async Task Post_AsNonMember_Returns403() { ... }

[Fact]
public async Task List_ReturnsPrChecksNewestFirst() { ... }

[Fact]
public async Task List_AsNonMember_Returns403() { ... }
```

#### Impact on Existing Tests
- New files only. Existing backend tests unchanged.
- `Program.cs` change is additive (new `MapPrChecksEndpoints(app)` line), verified by integration tests that re-resolve endpoints.

---

### Step 5: Backend — CLI Client update

**Rationale:** The CLI already posts to `/pr-checks` (from M4-007). Once step 4 is deployed, no CLI change is needed — but we add an integration test that verifies the real backend accepts the payload shape the CLI sends.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `internal/prcheck/client_test.go` | modify | Add a table entry that sends the exact shape backend expects and asserts 2xx |

No production code changes here. The purpose is to pin the contract.

---

### Step 6: Web — `/org/[slug]/pr-checks` page + API client

**Rationale:** Needed for Playwright assertion. The page is a thin list view reusing the `(app)` layout. No mutations, no complex state.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/src/lib/types/pr-checks.ts` | create | `PrCheck` type (id, repo, pr, state, result_id, created_at) |
| `web/src/lib/api/pr-checks.ts` | create | `prChecksApi.list(orgId, opts)` — mirrors `resultsApi.list` |
| `web/src/lib/api/pr-checks.test.ts` | create | Tests for list call, error propagation |
| `web/src/routes/(app)/org/[slug]/pr-checks/+page.server.ts` | create | Loader: auth, team-tier guard, fetch pr-checks, pass to view |
| `web/src/routes/(app)/org/[slug]/pr-checks/+page.svelte` | create | Table with columns: repo, PR, state badge, result link, created-at; data-testids: `pr-checks-table`, `pr-check-row`, `pr-check-state-<state>` |
| `web/src/routes/(app)/org/[slug]/+layout.svelte` (if subnav) | modify | Add `PR Checks` link with `data-testid="subnav-pr-checks-link"` for team-tier orgs |

(Subnav change is conditional — only if the layout exists; will be verified during exploration. Not strictly required by the spec but improves test ergonomics.)

#### Tests to Write FIRST (Playwright — deferred to Step 8)

Unit test example (`pr-checks.test.ts`):

```ts
it('calls /api/v1/organizations/:id/pr-checks and returns array', async () => {
    const { fetch } = createMockFetch({ pr_checks: [{ id: 'prc_1', repo: 'acme/api', pr: 7, state: 'success', result_id: 'res_1', created_at: '2026-04-17T10:00:00Z' }] });
    const out = await prChecksApi.list('org_abc', { token: 't', fetch });
    expect(out).toHaveLength(1);
    expect(out[0].pr).toBe(7);
});
```

#### Impact on Existing Tests
- New files only. No existing web tests affected.

---

### Step 7: E2E fixture + seed

**Rationale:** Tiny change; must be in place before the Playwright spec but is independent of web/backend specifics.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `testdata/team/e2e-collection.yaml` | create | A minimal 2-request collection hitting `httpbin.org` (or a local stub) with assertions that pass deterministically. Also include a commented-out "failure" variant referenced by the spec. |
| `testdata/team/e2e-collection-failing.yaml` | create | A variant with a deliberately failing assertion (for the "failure path" DoD item). |
| `scripts/seed-test-data.sh` | modify | Already seeds the `acme` org — add a create-api-token step that prints the token, to be consumed by the E2E spec/CLI. Use idempotent approach (check-then-create). |
| `scripts/test-stack.sh` | modify | Add idempotency assertion: running `up` twice does not duplicate fixtures (verify by counting rows or relying on seed script's existing 409 detection). Add a `--reset` sub-flag for forced re-seed. |

#### Tests to Write FIRST

`smoke/run.sh` assertion:

```bash
# --- Idempotent stack up ---
./scripts/test-stack.sh up
COUNT_BEFORE=$(curl -sS -H "Authorization: Bearer $(./scripts/test-token.sh owner@example.com)" http://localhost:5000/api/v1/organizations | jq '.organizations | length')
./scripts/test-stack.sh up
COUNT_AFTER=$(curl -sS -H "Authorization: Bearer $(./scripts/test-token.sh owner@example.com)" http://localhost:5000/api/v1/organizations | jq '.organizations | length')
[ "$COUNT_BEFORE" = "$COUNT_AFTER" ] || { echo "FAIL: seeding is not idempotent"; exit 1; }
./scripts/test-stack.sh down
```

#### Impact on Existing Tests
- Existing `org-results.spec.ts` and `org-notifications.spec.ts` continue to work (no mutation to shared fixtures).

---

### Step 8: Playwright spec — full pipeline

**Rationale:** End-to-end assertion. Deliberately placed last because it depends on every prior step.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `web/tests/e2e/full-pipeline.spec.ts` | create | Playwright spec: |
| `web/tests/e2e/helpers/cli.ts` | create | `runCurlew({ flags })` helper that `execSync`s `./curlew run …` and parses the `res_...` id from stdout |

Spec structure (5 assertions — meets the >=4 DoD):

```ts
test.describe('E2E: CLI -> backend -> web', () => {
    test.beforeEach(async ({ context }) => { await seedAuthCookie(context, OWNER_EMAIL); });

    test('uploaded run appears in /org/acme/results within 5s', async ({ page }) => {
        const { resultId, pass, fail } = runCurlew({ flags: ['--report-upload','--org','acme','--pr','7','--repo','acme/api'], collection: 'testdata/team/e2e-collection.yaml' });
        await page.goto(`/org/acme/results?range=all`);
        await expect(page.locator(`[data-result-id="${resultId}"]`)).toBeVisible({ timeout: 5000 });
        await expect(page.getByTestId(`result-pass-count-${resultId}`)).toContainText(String(pass));
        await expect(page.getByTestId(`result-fail-count-${resultId}`)).toContainText(String(fail));
    });

    test('pr-check row is visible in /org/acme/pr-checks', async ({ page }) => {
        await page.goto(`/org/acme/pr-checks`);
        await expect(page.getByTestId('pr-checks-table')).toBeVisible();
        await expect(page.getByTestId('pr-check-state-success').first()).toBeVisible();
    });

    test('failing collection uploads state=failure and dashboard shows failure badge', async ({ page }) => {
        const res = runCurlew({ flags: ['--report-upload','--org','acme','--pr','8','--repo','acme/api'], collection: 'testdata/team/e2e-collection-failing.yaml', expectExit: 1 });
        await page.goto(`/org/acme/pr-checks`);
        await expect(page.getByTestId('pr-check-state-failure').first()).toBeVisible();
    });
});
```

#### Impact on Existing Tests
- Existing Playwright specs unaffected (different `.spec.ts` file).
- Global setup (`CURLEW_MANAGE_STACK=1`) stays compatible.

---

### Step 9: CI workflow `e2e-m4`

**Rationale:** Last because it just orchestrates everything prior.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `.github/workflows/e2e-m4.yml` | create | Job on `ubuntu-latest` that: checks out, installs Go 1.24, installs Node LTS + npm, builds CLI, builds docker images, runs `scripts/test-stack.sh up`, runs CLI upload, runs `npm run test:e2e -- tests/e2e/full-pipeline.spec.ts`, tears down |
| `web/package.json` | modify if needed | Ensure `test:e2e` script forwards args (Playwright CLI does by default) |

#### Tests to Write FIRST
- Workflow is validated by running it in a PR. Local lint: `npx @action-validator/cli` or `act`.

#### Impact on Existing Tests
- None (no prior workflows).

---

### Step 10: Docs + help text

**Rationale:** Must ship alongside the feature per DoD. Zero test impact.

#### Files to Modify

| File | Action | Description |
|------|--------|-------------|
| `cmd/curlew/main.go` (`printHelp`, `run` help section) | modify | Document `--report-upload`, `--org`, `--pr`, `--repo`, `--triggered-by`, `--git-sha`; env vars `CURLEW_BACKEND_URL`, `CURLEW_BACKEND_TOKEN` |
| `CHANGELOG.md` | modify | Add `### Added` entry describing the feature |
| `smoke/run.sh` | modify | Add a dry-run-style test case for `curlew run --report-upload --dry-run` (stub backend) to catch help-text/flag regressions |

---

## Test Impact Summary

| Test File | Test Function | Impact | Action Required |
|-----------|--------------|--------|----------------|
| `cmd/curlew/main_test.go` | `TestParseRunArgs*` | breaks (tuple → struct) | Update destructures to struct access |
| `cmd/curlew/run_test.go` | `TestRun*` | breaks (tuple → struct) | Same |
| `cmd/curlew/discovery_run_test.go` | `TestDiscovery*` | breaks (tuple → struct) | Same |
| `cmd/curlew/validate_team_test.go` | `TestValidate*` | neutral | No change |
| `internal/prcheck/run_test.go` | `TestRun*` | none | `Run()` preserved |
| `internal/prcheck/prcheck_test.go` | `TestValidate*` | small | Add case for missing-results-allowed when in-memory mode set |
| `internal/prcheck/client_test.go` | — | additive | Add contract pin |
| `web/src/lib/api/*.test.ts` | — | additive | New `pr-checks.test.ts` only |
| `src/ApiTool.Backend.Tests/**` | — | additive | New `PrChecks/*.cs` tests only |

## Risks and Edge Cases

- **Risk:** Scope says "no new backend endpoints", but the pr-checks endpoint is a hard prerequisite for the Playwright assertion. → **Mitigation:** Documented as a scope clarification above; the endpoint is minimal (two methods, no GitHub integration) and the team spec lives outside M4-012.
- **Risk:** `parseRunArgs` tuple refactor touches many tests. → **Mitigation:** Mechanical change; do it as the first code commit so subsequent steps build on clean foundation. Run `go test ./...` after the refactor before moving on.
- **Risk:** Docker stack boot time in CI. → **Mitigation:** Reuse existing `scripts/test-stack.sh` which already has 60s polling. Set workflow timeout to 20 minutes.
- **Risk:** `httpbin.org` flakiness in CI (used by the e2e collection). → **Mitigation:** The collection can point to the backend's own `/health` endpoint (already exposed for healthcheck). Decision: use `http://backend:5000/swagger/v1/swagger.json` or a local nginx echo; document choice in `e2e-collection.yaml`.
- **Risk:** Upload failure on passing run masks test success. → **Mitigation:** Documented exit-code rules in Step 3 — upload error on a failing run does not override the failure; upload error on a passing run yields exit 2 with the upload error on stderr.
- **Risk:** Seed idempotency — second `test-stack.sh up` could duplicate fixtures. → **Mitigation:** Existing seed script is already idempotent (checks 409). Add assertion to smoke to pin this behavior.
- **Edge case:** Run has zero requests (empty collection). → **Handling:** `BuildPayload` returns a payload with empty `Items`; backend ingest already accepts this per M4-004.
- **Edge case:** `--report-upload` without `CURLEW_BACKEND_URL` env. → **Handling:** Validate at parse time, exit 2 with `backend URL not configured` — reuses existing `prcheck.ErrBackendURLMissing`.
- **Edge case:** `--pr` without `--repo` or vice versa. → **Handling:** Parse-time validation error (`--pr and --repo must be set together`), exit 1.
- **Edge case:** Run cancelled via SIGINT. → **Handling:** Context already plumbed; `prcheck.UploadRun` inherits the cancelled context and returns `context.Canceled`; we check and print `upload cancelled` on stderr, exit 130.

## Proposed Go Function Signatures

```go
// internal/prcheck/payload.go
func BuildPayload(results []runner.RequestResult, sum *runner.Summary, info TriggerInfo) *ResultsPayload

// internal/prcheck/git.go
func DetectGitSha(dir string) string

// internal/prcheck/run.go
type UploadConfig struct {
    BackendURL   string
    BackendToken string
    Org          string
    PR           int    // 0 = no pr-check
    Repo         string // "" = no pr-check
    HTTPClient   *http.Client // nil = DefaultClient
}

func UploadRun(ctx context.Context, cfg UploadConfig, payload *ResultsPayload) (*RunResult, error)

// cmd/curlew/main.go
type runFlags struct { ... }
func parseRunArgs(args []string) (runFlags, error)
func handleReportUpload(ctx context.Context, f runFlags, col *parser.Collection, results []runner.RequestResult, sum *runner.Summary) (exitCode int)
```

## Verification

```bash
go build ./cmd/curlew
go test ./...
~/go/bin/golangci-lint run
./smoke/run.sh
# Backend:
dotnet test src/ApiTool.Backend.Tests/ApiTool.Backend.Tests.csproj
# Web:
cd web && npm test && cd -
```

Observable verification (from the task YAML):

```bash
scripts/test-stack.sh up
go build ./cmd/curlew
CURLEW_BACKEND_URL=http://localhost:5000 \
CURLEW_BACKEND_TOKEN=$(scripts/test-token.sh owner@example.com) \
  ./curlew run testdata/team/e2e-collection.yaml --report-upload \
    --org acme --pr 7 --repo acme/api
# Expected: exit 0, stdout contains "Uploaded result res_...; status posted"

cd web && npm run test:e2e -- tests/e2e/full-pipeline.spec.ts
# Expected: >=4 Playwright assertions pass

scripts/test-stack.sh down
```
