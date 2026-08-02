# Milestone Mapping

## Phase 1 — Core CLI (M1)

### Task ID Scheme

All M1 tasks use the `M1-` prefix with three-digit numbering (`M1-001` through `M1-029`). Numbers indicate rough execution order. Each task is a vertical slice cutting through multiple packages.

| Prefix | Journey Stage | Description | Tasks |
|--------|--------------|-------------|-------|
| `M1` | `first_test` | From zero to a running test | 3 |
| `M1` | `test_with_confidence` | Assertions and error clarity | 5 |
| `M1` | `organize_and_reuse` | Variables, environments, composition | 10 |
| `M1` | `output_and_ci` | Output formats and redaction | 5 |
| `M1` | `project_commands` | Init and validate | 2 |
| `M1` | `ai_and_gating` | AI commands and feature gates | 4 |
| | | **Total** | **29** |

### M1 Dependency Graph (DAG)

```
M1-001 ─┬─ M1-002
        ├─ M1-003 ─┬─ M1-009 ─┬─ M1-010 ─── M1-015 ── M1-016 ── M1-025
        │          │          ├─ M1-011 ── M1-012 ─┬─ M1-013 ── M1-014 ── M1-018
        │          │          │                    └─ M1-017
        │          ├─ M1-024  │
        │          └─ M1-029  │
        │                     │
        └─ M1-004 ─┬─ M1-005 ── M1-007
                   ├─ M1-006 ── M1-019 ─┬─ M1-020 ─┬─ M1-026
                   │                    │          ├─ M1-028
                   │                    ├─ M1-021  │
                   │                    └─ M1-022 ── M1-023
                   └─ M1-008

M1-025 ── M1-027
```

Tasks form a DAG — multiple independent tasks can be worked in parallel. For example, after M1-001, both M1-002 (HTTP methods) and M1-004 (assertions) can proceed simultaneously.

### Parallel Work Opportunities

After M1-001 is done, two independent tracks open:
- **Track A** (assertions): M1-004 → M1-005/M1-006/M1-008 → ...
- **Track B** (sequences): M1-003 → M1-009/M1-024/M1-029 → ...

These tracks converge later (e.g., M1-019 needs M1-006, M1-025 needs M1-016).

## Phase 2 — Advanced CLI (M2) — COMPLETE

| Prefix | Capability Key | Phase Name | Slices | Status |
|--------|---------------|------------|--------|--------|
| `M2` | `vault_commands` | M2: Vault Commands | 3 | done |
| `M2` | `vault_storage` | M2: Vault Storage | 4 | done |
| `M2` | `dynamic_auth` | M2: Dynamic Auth | 3 | done |
| `M2` | `watch_mode` | M2: Watch Mode | 2 | done |
| `M2` | `retries` | M2: Retries | 2 | done |
| `M2` | `parallel` | M2: Parallel Execution | 4 | done |
| `M2` | `data_driven` | M2: Data-Driven Testing | 4 | done |
| `M2` | `adv_retries` | M2: Advanced Retries | 3 | done |
| `M2` | `reporting` | M2: Reporting | 3 | done |
| `M2` | `graphql` | M2: GraphQL | 3 | done |
| `M2` | `websocket` | M2: WebSocket | 3 | done |
| | | **Total** | **34** | |

## Phase 3 — Professional Tier (M3) — COMPLETE

M3 started after M1 and M2 were complete. All six slices are merged.

| Prefix | Capability Key | Phase Name | Slices | Status |
|--------|---------------|------------|--------|--------|
| `M3` | `rate_limiting` | M3: Professional Tier | 1 | done |
| `M3` | `test_discovery` | M3: Professional Tier | 1 | done |
| `M3` | `composition` | M3: Professional Tier | 1 | done |
| `M3` | `schema_validation` | M3: Professional Tier | 1 | done |
| `M3` | `openapi_import` | M3: Professional Tier | 2 | done |
| | | **Total** | **6** | |

### M3 Dependency Graph (DAG)

```
(all M2 done)
│
├─ M3-001  global rate_limit_rps       [depends on M2-013]
├─ M3-002  glob discovery              [no M3 deps]
├─ M3-003  include directive           [no M3 deps]
├─ M3-004  JSON Schema assertion       [no M3 deps]
└─ M3-005  openapi import (core)       [no M3 deps]
               │
               └─ M3-006  openapi import (bodies + assertions)
```

### M3 Parallel Work Opportunities

M3-001, M3-002, M3-003, M3-004, and M3-005 are fully independent — all can
be worked simultaneously after M2 completion. M3-006 is the only task with
an M3 internal dependency (requires M3-005).

**Suggested execution order** (by ascending complexity, to unblock CI wins first):
1. M3-002 (glob discovery) — medium; unblocks `curlew run "**/*_test.yaml"` in CI
2. M3-001 (global rate limit) — medium; reuses existing rateLimiter code
3. M3-004 (JSON Schema) — medium; self-contained new assertion kind
4. M3-003 (include directive) — high; touches parser core
5. M3-005 (openapi import core) — high; new subcommand + new dep
6. M3-006 (openapi bodies/assertions) — high; extends M3-005

## Phase 4 — Team Tier (M4)

M4 shifts focus from the Go CLI to the C# .NET backend and web portal.
Many slices in M4 will NOT follow the single-binary vertical-slice pattern
that M1–M3 used; see "Track Notes" below.

| Prefix | Capability Key    | Phase Name    | Slices | Status  |
|--------|-------------------|---------------|--------|---------|
| `M4`   | `vault_templates` | M4: Team Tier | 2      | backlog |
| `M4`   | `team_dashboards` | M4: Team Tier | 3      | backlog |
| `M4`   | `scheduled_runs`  | M4: Team Tier | 2      | backlog |
| `M4`   | `notifications`   | M4: Team Tier | 2      | backlog |
| `M4`   | `web_portal`      | M4: Team Tier | 3      | backlog |
|        |                   | **Total**     | **12** |         |

Slice counts are estimates. The Plan agent may refine by ±20%.

### M4 Dependency Graph (DAG)

```
(all M3 done)
│
├─ M4-001  shared vault template format           [track: go-cli]
├─ M4-002  CLI consumes shared template           [depends on M4-001]
│
├─ M4-003  backend: seat/RBAC data model          [track: backend]
├─ M4-004  backend: results ingestion API         [depends on M4-003]
├─ M4-005  web: results dashboard                 [depends on M4-004, track: web]
│
├─ M4-006  backend: scheduled run trigger         [depends on M4-003]
├─ M4-007  CLI: PR status check subcommand        [depends on M4-004, track: go-cli]
│
├─ M4-008  backend: notification dispatcher       [depends on M4-004]
├─ M4-009  web: notification config UI            [depends on M4-008, track: web]
│
├─ M4-010  backend: billing + seat mgmt API       [depends on M4-003]
├─ M4-011  web: billing portal                    [depends on M4-010, track: web]
└─ M4-012  e2e: CLI → backend → dashboard         [depends on M4-005, M4-007]
```

Tracks are labels, not separate DAGs. The DAG is one graph spanning all tracks.

### M4 Parallel Work Opportunities

Three tracks run in parallel once M4-003 (the backend data model) is in place:
- **go-cli**: M4-001 → M4-002, M4-007
- **backend**: M4-003 → M4-004 / M4-006 / M4-008 / M4-010
- **web**: M4-005, M4-009, M4-011 (each after its backend dependency)

M4-012 is the convergence point — a full end-to-end slice that proves all
three tracks work together.

### Track Notes

M4 is heterogeneous. Slices belong to one of three tracks:

| Track   | Stack                  | Observable shape                                    |
|---------|------------------------|-----------------------------------------------------|
| go-cli  | Go CLI (`cmd/curlew`) | `go build && ./curlew <cmd>` — same as M1–M3       |
| backend | C# .NET backend        | `dotnet test` + `curl` against running service      |
| web     | Web portal             | e2e test against deployed or locally-started portal |

Each generated M4 task YAML MUST include a `track:` field with one of those
three values. The Plan agent uses it to pick the right observable template:

- **go-cli** tasks: use `task-template.md`'s examples unchanged
- **backend** tasks: observable is `dotnet test` plus an HTTP probe against
  a service started from the task's fixture
- **web** tasks: observable is an end-to-end test run that hits a deployed
  or locally-started portal

Do NOT force backend/web slices into Go-package boundaries. Vertical slices
for those tracks cut across C# projects or web components. The completeness
contract still applies: help text (for CLI), API docs (for backend), user-
visible UI (for web), plus integration tests that exercise a running system.

## Phase 5 — Enterprise Tier (M5)

M5 is the Enterprise tier. Like M4 it spans all three tracks (`go-cli`,
`backend`, `web`). Roadmap source: `docs/SPECIFICATION.md:9001-9008`.

| Prefix | Capability Key      | Phase Name          | Slices | Status  |
|--------|---------------------|---------------------|--------|---------|
| `M5`   | `sso`               | M5: Enterprise Tier | 3      | backlog |
| `M5`   | `audit_logging`     | M5: Enterprise Tier | 2      | backlog |
| `M5`   | `rbac_granular`     | M5: Enterprise Tier | 2      | backlog |
| `M5`   | `distributed_exec`  | M5: Enterprise Tier | 3      | backlog |
| `M5`   | `perf_testing`      | M5: Enterprise Tier | 2      | backlog |
| `M5`   | `offline_license`   | M5: Enterprise Tier | 2      | backlog |
| `M5`   | `self_hosted`       | M5: Enterprise Tier | 2      | backlog |
| `M5`   | `plugin_system`     | M5: Enterprise Tier | 3      | backlog |
| `M5`   | `enterprise_e2e`    | M5: Enterprise Tier | 1      | backlog |
|        |                     | **Total**           | **20** |         |

Slice counts are estimates. The Plan agent may refine by ±20%.

### M5 Capability Descriptions

- **sso** — SAML 2.0 and OIDC integration for org-level single sign-on
  (`Organization.settings.sso_*` fields in `docs/SPECIFICATION.md:7033-7035`
  already exist). Web UI to configure identity providers per organization.
- **audit_logging** — Backend capture middleware writing to the existing
  `user_audit_log`, `auth_audit_log`, `organization_audit_log`, and
  `subscription_audit_log` tables (see `docs/SPECIFICATION.md:8498-8873`).
  Web viewer with filter/export.
- **rbac_granular** — Custom role definitions beyond the fixed
  owner/admin/member trio. Per-resource permissions map
  (`docs/SPECIFICATION.md:7085-7116`). Web role editor.
- **distributed_exec** — Multi-worker coordination so long test suites can
  run across multiple machines. Backend coordinator, CLI worker agent,
  `--workers` execution flag. **Spec is thin here** — Plan agent should
  flag ambiguity (worker discovery? auth? result aggregation protocol?).
- **perf_testing** — Load-generation mode (concurrent virtual users, ramp
  profiles) and performance metrics reports. **Spec is thin** — infer
  reasonable k6/Vegeta-style shape.
- **offline_license** — Embedded JWKS + grace period for air-gapped
  environments (`docs/SPECIFICATION.md:7411-7478`). `license export`
  subcommand to bundle tokens for offline use.
- **self_hosted** — Packaging the .NET backend for on-prem deployment:
  docker-compose bundle with Postgres/Redis, migration runner, admin
  bootstrap. **Spec is thin** — infer from existing `Dockerfile` and
  `Migrations/` conventions.
- **plugin_system** — Plugin interface + loader for custom integrations.
  Hook registry for request/response/result lifecycle. Example plugin +
  developer docs. **Spec is thin** — Plan agent should propose concrete
  plugin shape (Go plugin package vs external process vs WASM).
- **enterprise_e2e** — Convergence slice: SSO login → test upload with
  audit entry → dashboard view with custom role. Mirrors M4-012's role.

### M5 Dependency Graph (DAG)

```
(all M4 done)
│
├─ M5-001  backend: SAML 2.0 auth flow          [depends on M4-003]
├─ M5-002  backend: OIDC auth flow              [depends on M4-003]
├─ M5-003  web: SSO config UI                   [depends on M5-001, M5-002]
│
├─ M5-004  backend: audit log capture           [depends on M4-003]
├─ M5-005  web: audit log viewer                [depends on M5-004]
│
├─ M5-006  backend: custom role + permission    [depends on M4-003]
├─ M5-007  web: role editor UI                  [depends on M5-006]
│
├─ M5-008  backend: worker coordinator service  [depends on M4-004]
├─ M5-009  go-cli: worker agent protocol        [depends on M5-008]
├─ M5-010  go-cli: --workers execution flag     [depends on M5-009]
│
├─ M5-011  go-cli: load generation mode         [no M5 deps]
├─ M5-012  go-cli: perf metrics + report        [depends on M5-011]
│
├─ M5-013  go-cli: offline JWT verify + grace   [no M5 deps]
├─ M5-014  go-cli: license export bundle        [depends on M5-013]
│
├─ M5-015  backend: self-hosted bundle          [no M5 deps]
├─ M5-016  backend: admin bootstrap + migrate   [depends on M5-015]
│
├─ M5-017  go-cli: plugin interface + loader    [no M5 deps]
├─ M5-018  go-cli: hook registry                [depends on M5-017]
├─ M5-019  go-cli: example plugin + docs        [depends on M5-018]
│
└─ M5-020  e2e: SSO → audit → dashboard         [depends on M5-003, M5-005, M5-007]
```

### M5 Parallel Work Opportunities

Four backend tracks open the milestone in parallel (all depend on M4-003
or M4-004, which are `done`): SSO (M5-001, M5-002), audit (M5-004), RBAC
(M5-006), distributed-exec coordinator (M5-008).

Fully independent go-cli tracks that can run from day one: perf (M5-011),
offline license (M5-013), plugins (M5-017). Self-hosted (M5-015) is also
independent.

The web track waits for its backend dependency in each capability.

### M5 Track Assignments

Uses the same three tracks defined under M4 — `go-cli`, `backend`, `web`.
No new track. Self-hosted deployment artifacts live under `backend`
because they package the existing .NET service; no separate `ops` track
is worth introducing for two slices.

### M5 Under-Specification Warning

Five of the nine capabilities have only a single roadmap bullet in the
spec (`distributed_exec`, `perf_testing`, `self_hosted`, `plugin_system`,
and parts of `offline_license` beyond the License Validation section).
When `/backlog M5` dispatches the Plan agent:

1. For well-specified capabilities (`sso`, `audit_logging`, `rbac_granular`,
   `offline_license`), the agent should anchor on the specific spec line
   ranges noted above.
2. For thin capabilities, the agent MUST emit explicit open questions in
   its ambiguities section rather than inventing design details. We will
   decide whether to expand the spec or defer those slices before
   approving the generated tasks.

## Phase 12 — Foundational dynamic-function helpers (M12) — COMPLETE

M12 ships the foundational dynamic-function helpers from Phases 2 and 3 of the dynamic-function rollout (`docs/SPECIFICATION.md:746–747`). 13 functions across encoding, hashing, date arithmetic, and advanced strings — but the milestone is fronted by an arg-parsing foundation slice because the existing dynPattern (`internal/variable/variable.go:82`) only matches no-arg `{{$fn}}` syntax. The manual already documents `{{$base64('user:pass')}}` and `{{$hmacSha256('data', 'secret')}}` as if available, so this milestone closes a real documentation/implementation drift.

| Prefix | Capability Key             | Phase Name                                | Slices | Status  |
|--------|----------------------------|-------------------------------------------|--------|---------|
| `M12`  | `dynamic_function_helpers` | M12: Foundational dynamic-function helpers | 8      | done    |
|        |                            | **Total**                                 | **8**  |         |

### M12 Capability Description

- **dynamic_function_helpers** — argument syntax for the dynamic-function evaluator (slice 1, foundation), then 13 helper functions across four families: encoding (`$base64`, `$base64Decode`, `$urlEncode`, `$jsonEncode`), hashing (`$sha256`, `$md5`, `$hmacSha256`), date arithmetic and formatting (`$dateAdd`, `$dateSubtract`, `$formatDate`, `$parseDate`), advanced strings (`$randomPassword`, `$randomBase64`). The hashing family includes sensitive-key propagation: when `$hmacSha256`'s key argument resolves from a sensitive variable, the resolved key is added to the run's SensitiveSet so it is redacted in serialised output. M12 unblocks first-party signing plugins (Stripe / GitHub / Slack webhook signatures) and tightens the no-scripting argument from `docs/ASSESSMENT.md` and `docs/SCRIPTING.md`.

### M12 Dependency Graph (DAG)

```
(all M11 done)
│
└─ M12-001  argument-parsing foundation
            │
            ├─ M12-002  $base64, $base64Decode
            ├─ M12-003  $urlEncode, $jsonEncode
            ├─ M12-004  $sha256, $md5
            ├─ M12-005  $hmacSha256 + sensitive-key propagation
            ├─ M12-006  $dateAdd, $dateSubtract
            ├─ M12-007  $formatDate, $parseDate
            └─ M12-008  $randomPassword, $randomBase64
```

M12-001 is a strict prerequisite for M12-002 through M12-008. The seven function-implementation tasks are mutually independent — they can land in parallel after M12-001 ships.

### M12 Track Assignments

All eight tasks are `track: go-cli`. No backend or web work in this milestone.

### M12 Spec Anchors

- `docs/SPECIFICATION.md:740–850` — dynamic-function rollout phases and the faker function reference
- `docs/MANUAL.md:694–699` — current (forward-dated) examples using `$base64`, `$hmacSha256`, `$urlEncode`
- `docs/MANUAL.md:1025–1090` — current dynamic-functions manual section (15 no-arg entries; the M12 entries extend this table)
- `internal/variable/dynamic.go` — current registration pattern (16 no-arg DynFuncs)
- `internal/variable/variable.go:80–230` — current regex (`dynPattern`), Pass 1 dynamic resolution
- `internal/variable/sensitive.go` — SensitiveSet, AddValue (used by M12-005)

### Notes carried from `/backlog M12` decomposition

The Plan agent flagged five spec ambiguities resolved in-task (defaults documented in MANUAL.md per-task) — see the closing section of each task's `scope:`. Notable defaults:

- `$dateAdd` is 2-arg (amount, unit) with implicit base of `now`. A 3-arg form with explicit base is deferred.
- `$formatDate` accepts digits-only inputs as Unix seconds, otherwise `time.RFC3339`. Other ISO variants error and direct the user to `$parseDate`.
- `$randomPassword` symbol set is `!@#$%^&*()-_=+[]{}<>?,.` — a one-line change if product wants different.
- `$hmacSha256` propagates only the key's sensitivity, not the payload's. Literal keys are documented as a footgun (no back-traceable variable means no sensitivity propagation possible).
- Layout-as-template (Go's `time.Format` silently passes unrecognised layouts as literal output) is documented in MANUAL.md rather than guarded with a layout-validation step.

## Phase 13 — Faker depth (M13) — COMPLETE

M13 ships Phase 4 of the dynamic-function rollout (`docs/SPECIFICATION.md:748`): the `$faker.*` realistic-data family — 53 functions across personal, location, company, internet, content, financial, and file categories (`docs/SPECIFICATION.md:751–847`). The milestone is fronted by a dotted-namespace syntax slice because the existing `dynPattern` (`internal/variable/variable.go:96`) matches function names as `[a-zA-Z][a-zA-Z0-9_]*` only — it does not admit the dot in `$faker.firstName`, so without that foundation no faker function reaches the registry. M13 closes REVIEW.md gap **1c** and the Phase 4 line of the spec.

| Prefix | Capability Key | Phase Name        | Slices | Status  |
|--------|----------------|-------------------|--------|---------|
| `M13`  | `faker_depth`  | M13: Faker depth  | 8      | done    |
|        |                | **Total**         | **8**  |         |

### M13 Capability Description

- **faker_depth** — dotted-namespace syntax foundation (slice 1), then 53 realistic-data helper functions across seven categories: personal (10 fns including auto-sensitive `$faker.ssn`), location (12 fns), company (5 fns), internet (9 fns), content (5 fns; argument-bearing — `$faker.words(count)`, `$faker.sentence(wordCount)`, `$faker.paragraph(sentenceCount)`, `$faker.text(charCount)` — uses M12-001 arg parsing), financial (8 fns including auto-sensitive `$faker.creditCard`, `$faker.creditCardCVV`, `$faker.iban`), and file (4 fns; `$faker.imageUrl(width, height)` is argument-bearing). Auto-sensitive functions register their generated value with `SensitiveSet` at generation time so it is redacted in serialised output, reusing the propagation pattern established in M12-005. M13 closes the test-data-quality gap from REVIEW.md:75 and tightens the no-scripting argument by giving free-tier users plausible fixtures without external dependencies.

### M13 Dependency Graph (DAG)

```
(all M12 done)
│
└─ M13-001  dotted-namespace syntax foundation (dynPattern + registry lookup)
            │
            ├─ M13-002  $faker personal data        (10 fns; $faker.ssn auto-sensitive)
            ├─ M13-003  $faker location data        (12 fns)
            ├─ M13-004  $faker company data         (5 fns)
            ├─ M13-005  $faker internet data        (9 fns)
            ├─ M13-006  $faker content data         (5 fns; uses M12-001 arg parsing)
            ├─ M13-007  $faker financial data       (8 fns; 3 auto-sensitive)
            └─ M13-008  $faker file data            (4 fns; imageUrl uses M12-001 arg parsing)
```

M13-001 is a strict prerequisite for M13-002 through M13-008. The seven category slices are mutually independent and can land in any order or in parallel after M13-001 ships.

### M13 Track Assignments

All eight tasks are `track: go-cli`. No backend or web work in this milestone.

### M13 Spec Anchors

- `docs/SPECIFICATION.md:743–748` — dynamic-function rollout phasing (Phase 4 = M13)
- `docs/SPECIFICATION.md:751–847` — full faker function reference (seven category tables, 53 functions total)
- `docs/SPECIFICATION.md:849–857` — auto-sensitive classification (`$faker.ssn`, `$faker.creditCard`, `$faker.creditCardCVV`, `$faker.iban`)
- `docs/SPECIFICATION.md:859–904` — locale support (**deferred from M13** — see Open Decisions below)
- `internal/variable/dynamic.go:163–222` — current registration pattern; faker entries land alongside the existing legacy `$randomEmail`, `$randomName`, `$randomFirstName`, `$randomLastName`, `$randomColor` registrations
- `internal/variable/variable.go:82–96` — current `dynPattern` regex; M13-001 extends the function-name character class to admit dotted namespaces
- `internal/variable/sensitive.go` — `SensitiveSet`, `AddValue` (used by M13-002 for SSN and M13-007 for creditCard / CVV / iban; mirrors M12-005's pattern)

### Open Decisions for `/backlog M13`

These ambiguities should be resolved by the milestone author before generating tasks; defaults are proposed.

1. **Locale support is deferred from M13.** SPECIFICATION.md:859–904 specifies 15 supported locales, per-locale name/phone/address variation, a precedence chain (`default < project < env < collection < CLI flag`), and locale-fallback logic. That is a milestone of its own — out of scope for M13. **Default:** M13 ships en-US for every function; the `--locale` flag is recognised-but-warns-and-ignores until a follow-up milestone (suggested name: `M14-locale` or `faker_locale`, sequenced after the revenue-plumbing M14). Each task's `scope:` should state this explicitly so it does not silently sneak back in.
2. **Spec count typo: 52 vs 53.** Line 748 says "Phase 4: Faker integration (52 functions)"; line 847 says "Total: 53 faker functions". Counting the tables (10 + 12 + 5 + 9 + 5 + 8 + 4) confirms 53. **Default:** M13-001 includes a doc-fix in SPECIFICATION.md:748 changing 52 → 53.
3. **Legacy non-namespaced helpers retained, not aliased.** `$randomEmail`, `$randomName`, `$randomFirstName`, `$randomLastName`, `$randomColor` exist today and overlap by name with `$faker.email`, `$faker.fullName`, `$faker.firstName`, `$faker.lastName`, `$faker.color`. **Default:** keep both as distinct registrations. Their contracts differ — e.g. legacy `$randomEmail` returns `"first.NNNN@example.com"` (firstName-prefixed); spec `$faker.email` is "valid format, example.com domain" without prescribing the local-part shape. Aliasing would either lock the spec contract to the legacy implementation or break existing callers. No deprecation in M13.
4. **Auto-sensitive functions register at generation time, no separate slice.** `$faker.ssn` (M13-002), `$faker.creditCard`, `$faker.creditCardCVV`, `$faker.iban` (M13-007) call `SensitiveSet.AddValue` on each generated value. This reuses the M12-005 propagation pattern — no new infrastructure needed. **Default:** test in each slice that the generated value is redacted in JSON/markdown output. No dedicated sensitive-propagation slice.
5. **`--seed` reproducibility per SPECIFICATION.md:755.** All faker functions must produce identical output for identical `--seed` values. **Default:** every category slice's test suite includes a seeded reproducibility check (run twice with the same seed, assert byte-equal output) and a no-seed entropy check (run twice with no seed, assert different output).
6. **Argument-bearing functions inherit M12-001 parsing.** `$faker.words(count)`, `$faker.sentence(wordCount)`, `$faker.paragraph(sentenceCount)`, `$faker.text(charCount)` (all M13-006), `$faker.price(min, max)` (M13-007), `$faker.imageUrl(width, height)` (M13-008). M12-001 already supports comma-separated string-and-int arguments; no further parsing work needed.

## Phase 14 — Revenue plumbing (M14)

M14 is the largest pending milestone, the only T1 (revenue-critical) milestone in the gap inventory, and the gate for ~half of M15 and most of M16. The design pass is captured in two artefacts:

1. `docs/M14_INVESTIGATION.md` — the complete pre-work investigation and eleven settled load-bearing decisions (2026-05-03).
2. `docs/SPECIFICATION.md` v4.2 (lines 4–37; License Validation, CLI ↔ Backend Integration, Stripe Webhook Idempotency, SendGrid Templates) and v4.2.1 (the GitHub Checks API Integration supplement).

Everything below is grounded in those two artefacts; do NOT reopen settled decisions during `/backlog M14`. The eleven decisions in the investigation's "Decisions resolved" table (lines 605–625) and the ten v4.2.1 decisions (`docs/SPECIFICATION.md` "Decisions Resolved (v4.2.1)") are commitments, not proposals.

M14 spans all three tracks (`backend`, `go-cli`, `web`) plus a final end-to-end convergence slice. It is heterogeneous in the same sense as M4/M5 — see "Track Notes" below.

| Prefix | Capability Key         | Phase Name                       | Slices | Status  |
|--------|------------------------|----------------------------------|--------|---------|
| `M14`  | `license_issuance`     | M14: Revenue plumbing            | 3      | backlog |
| `M14`  | `cli_backend`          | M14: Revenue plumbing            | 4      | backlog |
| `M14`  | `stripe_gateway`       | M14: Revenue plumbing            | 3      | backlog |
| `M14`  | `stripe_webhooks`      | M14: Revenue plumbing            | 3      | backlog |
| `M14`  | `transactional_email`  | M14: Revenue plumbing            | 2      | backlog |
| `M14`  | `github_checks_api`    | M14: Revenue plumbing            | 4      | backlog |
| `M14`  | `web_integrations`     | M14: Revenue plumbing            | 1      | backlog |
| `M14`  | `revenue_e2e`          | M14: Revenue plumbing            | 1      | backlog |
|        |                        | **Total**                        | **21** |         |

Slice counts are bounded estimates per the investigation (~19–21 range). The Plan agent may refine by ±10%; significant deviation should surface as an explicit ambiguity rather than a silent rescope.

### M14 Capability Descriptions

- **license_issuance** — backend signing infrastructure for the v4.2 ES256 token model. Three slices: (1) `signing_keys` table + `IKeyProvider` interface + `FileKeyProvider` for `deploy/self-hosted/` + `GoogleKmsKeyProvider` for SaaS (asymmetric `EC_SIGN_P256_SHA256` HSM-tier key, FIPS 140-2 Level 3, 90-day rotation cadence with 60-day verification window); (2) `POST /api/v1/auth/refresh` unified mint endpoint that returns all three tokens (License JWT, Access token, rotated refresh token) in one round-trip per v4.2 §3 — there is no separate `/api/v1/license/issue`; (3) `GET /api/v1/.well-known/jwks.json` (RFC 8615 well-known URI; replaces the v4.1 bespoke `/api/v1/public-key`) returning current + verifying keys with `Cache-Control: max-age=3600`. The claim shapes are pinned in `docs/SPECIFICATION.md` "Token Claim Shapes" (License JWT 17 claims, Access token 9 claims) including the trial fields that default to `none`/`null` so M16's trial slices populate without claim-shape churn.

- **cli_backend** — CLI side of the network boundary. Four slices: (1) HTTP client foundation in a new `internal/backend/` package (Bearer auth, RFC 7807 error mapping, single-flight `flock` on `~/.config/curlew/refresh.lock`, hybrid OS-keychain + AES-256-GCM encrypted-file fallback for refresh-token storage); (2) `curlew login` device-code flow (RFC 8628) with browser auto-open when interactive TTY + default browser detected; (3) `curlew license --refresh` (calls `/auth/refresh`, persists all three tokens) + `--debug` (prints decoded License JWT and cache state); (4) JWKS fetch + cache at `~/.config/curlew/jwks_cache.json` for offline verification fallback. The full exit-code taxonomy for `--refresh` (codes 0–7) is documented in `docs/SPECIFICATION.md` "CLI Exit-Code Taxonomy" — exit codes 3 and 6 are non-fatal for `curlew run` because the License JWT is offline-verified within its 30-day + 14-day grace window.

- **stripe_gateway** — live Stripe SDK integration replacing the placeholder `StripeGateway.cs`. Three slices: (1) SDK wiring + `IStripeGateway.CreateCheckoutAsync` against `Stripe.Net 43.x`; (2) `CreateBillingPortalAsync`; (3) `ComputeProrationAsync` via Stripe's upcoming-invoice preview (the interface shifts to async because Stripe computes proration server-side; the existing `FakeStripeGateway`'s local-math diverges from real Stripe and is preserved only for unit tests). All three slices use the existing `IdempotencyKey` per call to defend against network-retry double-charges. Test strategy is three-layer per v4.2 §6: unit (`FakeStripeGateway`), integration (`stripe-mock` Docker container in CI on port `12111`, `[Trait("Category", "stripe-integration")]`), smoke (Stripe test mode + `stripe listen` pre-release).

- **stripe_webhooks** — Stripe → backend event ingestion. Three slices: (1) signature-verify middleware via `Stripe.Net.EventUtility.ConstructEvent` (default 5-min tolerance — never set to 0; multi-secret rotation via `STRIPE__WEBHOOK_SECRETS` comma-separated) + `stripe_webhook_events` table migration + the `INSERT ... ON CONFLICT (event_id) DO NOTHING RETURNING received_at` idempotency pattern + 5-failure quarantine retry budget; (2) `customer.subscription.{created,updated,deleted}` and `customer.updated` handlers — handlers re-fetch from Stripe API rather than applying event-payload deltas (event-ordering defense; Stripe does not guarantee order); (3) `invoice.{payment_succeeded,payment_failed,finalized}` and `payment_method.{attached,detached}` handlers — payment-succeeded queues a `billing_receipt` email, payment-failed queues `billing_payment_failed`. Total events handled: 9.

- **transactional_email** — SendGrid integration with in-repo MJML + manifest source-of-truth. Two slices: (1) `ISmtpSender` + `SendGridSmtpSender` + `EmailQueueProcessor` (consumes from existing `System.Threading.Channels`) + the MJML compile pipeline (`curlew-backend dev email-preview <slug>` renderer; CI upload job that posts compiled HTML to SendGrid on tagged release and writes `SENDGRID__TEMPLATES__<SLUG>` to secrets); (2) the six M14 transactional templates with manifests: `email_verification`, `auth_device_code`, `billing_receipt`, `billing_payment_failed`, `billing_subscription_canceled`, `account_security_alert`. Templates `password_reset` and `trial_expiring` are explicitly NOT in M14 (they ship with M16's password-reset and trial cron slices). Template-injection prevention: at send time, `SendGridSmtpSender` rejects any variable name not in the template's manifest.

- **github_checks_api** — outbound integration for posting PR-check results to GitHub's Checks API; folded into M14 from M16 per investigation Decision #11. Four slices: (1) `IGitHubAppKeyProvider` + `FileGitHubAppKeyProvider` (`deploy/self-hosted/`) + `GoogleKmsGitHubAppKeyProvider` (SaaS, `RSA_SIGN_PKCS1_2048_SHA256` HSM-tier key — note RS256 is mandated by GitHub for App JWTs; this is the one place RS256 lives in the system per v4.2.1's Decision #13) + JWT signing for installation-token requests (`iss = app_id`, `iat = now - 60s`, `exp = iat + 540s`); (2) `github_installations` table + dashboard-initiated and webhook-first install/claim flows + `installation_repositories.added/removed` reconciliation + daily reconciliation job + the schema-level `UNIQUE (org_id) WHERE deleted_at IS NULL AND org_id IS NOT NULL` cross-tenant guardrail; (3) outbound Checks API (`POST /repos/{owner}/{repo}/check-runs`) + `pr_checks` schema expansion (`installation_id`, `check_run_id`, `external_id`, `conclusion`, `details_url`, `output_*`, `annotations`, retry/state fields) + 6-state mapping (`success/failure/cancelled/timed_out/neutral/skipped`) + always-post-on-green per v4.2.1 Decision #19 + idempotency-on-retry via `(head_sha, app_id)` lookup + token-bucket rate-limit handling per `installation_id` + RFC 7807 error taxonomy with new `PRCHECK_*` codes; (4) `POST /webhooks/github` inbound webhook handler with `X-Hub-Signature-256` HMAC-SHA256 verification (constant-time compare; raw body before JSON parse; legacy SHA-1 header ignored) + `github_webhook_events` idempotency table keyed on `X-GitHub-Delivery` + 5-failure quarantine + multi-secret rotation via `GITHUB__WEBHOOK_SECRETS` (despite GitHub itself supporting only one secret at a time — operator-side overlap window) + handlers for `installation.{created,deleted,suspend,unsuspend}`, `installation_repositories.{added,removed}`, `check_run.rerequested`. Permission scope is pinned to `metadata: read` + `checks: write` only; adding any other permission requires security review (v4.2.1 Decision #14). GHES support is explicitly out of M14 scope (Decision #21). GitLab parity is explicitly NOT in M14.

- **web_integrations** — minimal web-track work for M14: a single dashboard slice (1) for the "Connect GitHub" install entry point (`/api/v1/integrations/github/install-url` callback flow with signed `state` token containing `org_id`) and the install-state view (renders `installed`, `repos covered`, `suspended`, `pending claim`). Other web work — billing portal UI surfaces, organization SSO, dashboards beyond M4-005's existing results dashboard — is explicitly NOT in M14; those belong to later milestones.

- **revenue_e2e** — single convergence slice that proves the full revenue + integration loop end-to-end: `curlew login` (device code) → backend mints License JWT + Access token → `curlew run --report-upload` against a fixture API → backend persists results + posts a check run to a test-mode GitHub App on a real PR → SendGrid receipt email is queued (verified via `stripe-mock`'s `invoice.payment_succeeded` simulation, not real charge). Mirrors M4-012's role.

### M14 Dependency Graph (DAG)

```
(all M5/M11/M12/M13/M17 done; M11's results-ingestion API and M4-007's pr-checks endpoint already exist)
│
├─ backend foundation (license issuance) ─────────────────────────────
│  M14-001  signing_keys + IKeyProvider + File/KMS providers   [track: backend]
│           │
│           ├─ M14-002  POST /auth/refresh unified mint        [track: backend]
│           └─ M14-003  GET /.well-known/jwks.json             [track: backend]
│
├─ CLI ↔ backend (depends on M14-002, M14-003) ──────────────────────
│  M14-004  internal/backend/ HTTP client + flock + keychain   [track: go-cli]
│           │
│           ├─ M14-005  curlew login (device code, RFC 8628)  [depends on M14-002, M14-004, track: go-cli]
│           │           │
│           │           └─ M14-006  license --refresh + --debug + exit codes  [depends on M14-005, track: go-cli]
│           │
│           └─ M14-007  JWKS fetch + cache (CLI side)          [depends on M14-003, M14-004, track: go-cli]
│
├─ Stripe gateway (depends on M14-002 for org auth context) ─────────
│  M14-008  SDK wiring + checkout session                      [depends on M14-002, track: backend]
│           │
│           ├─ M14-009  billing portal session                 [depends on M14-008, track: backend]
│           └─ M14-010  proration via upcoming-invoice preview [depends on M14-008, track: backend]
│
├─ Stripe webhooks (independent track) ──────────────────────────────
│  M14-011  signature verify + stripe_webhook_events table     [no M14 deps, track: backend]
│           │
│           ├─ M14-012  subscription + customer event handlers [depends on M14-011, M14-014 for receipt email queueing, track: backend]
│           └─ M14-013  invoice + payment-method handlers      [depends on M14-011, M14-014, track: backend]
│
├─ SMTP / email (independent track) ─────────────────────────────────
│  M14-014  SendGrid sender + EmailQueueProcessor + MJML pipe  [no M14 deps, track: backend]
│           │
│           └─ M14-015  6-template inventory + manifests + CI  [depends on M14-014, track: backend]
│
├─ GitHub Checks API (independent track from M14-001/002/003) ───────
│  M14-016  IGitHubAppKeyProvider + RS256 App JWT signing      [no M14 deps, track: backend]
│           │
│           └─ M14-017  github_installations + claim flow      [depends on M14-016, track: backend]
│                       │
│                       ├─ M14-018  Checks API outbound + pr_checks expansion  [depends on M14-017, track: backend]
│                       ├─ M14-019  POST /webhooks/github + github_webhook_events  [depends on M14-017, track: backend]
│                       └─ M14-020  web: Connect GitHub install page  [depends on M14-017, track: web]
│
└─ Convergence
   M14-021  e2e: login → run → report-upload → check-run → receipt email  [depends on M14-006, M14-013, M14-015, M14-018, track: e2e]
```

Tracks are labels, not separate DAGs. The DAG is one graph spanning all four. M14-012 and M14-013's dependency on M14-014 is for the email-queueing call site — the handlers compose `EmailMessage`s and push them onto the channel; without `EmailQueueProcessor` the channel is unread and the handlers block. Plan agent may treat this as a soft dependency (handlers can land first with the queue mocked) but the convergence slice (M14-021) requires both.

### M14 Parallel Work Opportunities

Five independent track heads open the milestone in parallel:

1. **License-issuance backend** (M14-001) — gates M14-002, M14-003.
2. **CLI HTTP client** (M14-004) — can land before backend endpoints exist; tested against `httptest`-style fakes initially, then wired against M14-002/003 once those land.
3. **Stripe webhooks foundation** (M14-011) — independent of license issuance; signature verification + idempotency table are infrastructure work.
4. **SendGrid pipeline** (M14-014) — fully independent; can land first since it's a pure infrastructure addition with no auth surface.
5. **GitHub App keys** (M14-016) — fully independent; the App credentials story is parallel to (not dependent on) our own ES256 key infrastructure.

Mid-milestone, three more parallel arms open:

- After M14-008: M14-009 and M14-010 (Stripe portal + proration) can run concurrently.
- After M14-011: M14-012 and M14-013 can run concurrently (only mild coupling to M14-014).
- After M14-017: M14-018, M14-019, and M14-020 are mutually independent and can run in parallel across `backend` and `web` tracks.

The convergence slice M14-021 is the latest possible slice; it requires the full happy-path graph to be green.

### M14 Track Assignments

M14 uses the same three tracks defined under M4 (`go-cli`, `backend`, `web`) plus an `e2e` label for the convergence slice. Slice → track mapping summary:

| Track    | Slices | Count |
|----------|--------|-------|
| backend  | M14-001, M14-002, M14-003, M14-008, M14-009, M14-010, M14-011, M14-012, M14-013, M14-014, M14-015, M14-016, M14-017, M14-018, M14-019 | 15 |
| go-cli   | M14-004, M14-005, M14-006, M14-007 | 4 |
| web      | M14-020 | 1 |
| e2e      | M14-021 | 1 |
|          | **Total** | **21** |

Each generated M14 task YAML MUST include a `track:` field with one of `go-cli`, `backend`, `web`, `e2e`. The `e2e` track's observable shape is "the docker-compose stack started by `./scripts/ci-local.sh --full` accepts a CLI invocation, persists results, posts a check run to GitHub test mode, and queues a receipt email" — see M4-012 / M5-020 for prior `e2e` slice patterns.

### M14 Spec Anchors

The v4.2 + v4.2.1 spec edition is the authoritative reference for every M14 slice. Specific anchors:

- `docs/SPECIFICATION.md:7733–7990` — License Validation & Enforcement: Design Decisions, Token Model, Token Claim Shapes, Refresh-Token Rotation and Revocation, Daily Validation Logic, Offline JWT Verification, Signing-Key Storage, Embedded Public Key Management, Key Rotation Strategy. **Primary anchor for M14-001, M14-002, M14-003, M14-007.**
- `docs/SPECIFICATION.md:8171–8270` — CLI ↔ Backend Integration: Auth Scheme, Login Flow (RFC 8628 device code), Endpoint Reference, RFC 7807 Error Model, Exit-Code Taxonomy. **Primary anchor for M14-004, M14-005, M14-006.**
- `docs/SPECIFICATION.md:6771–6857` — Stripe Webhook Events + Stripe Test Strategy. **Primary anchor for M14-011, M14-012, M14-013.**
- `docs/SPECIFICATION.md:8443–8546` — Email Service Integration (SendGrid pipeline + 6-template inventory). **Primary anchor for M14-014, M14-015.**
- `docs/SPECIFICATION.md:8275–8643` — GitHub Checks API Integration (the v4.2.1 supplement, all 12 subsections). **Primary anchor for M14-016, M14-017, M14-018, M14-019, M14-020.**
- `docs/SPECIFICATION.md:9268–10106` — Database Schema appendix: `signing_keys`, `refresh_tokens` (replaced shape), `stripe_webhook_events`, `github_installations`, `github_webhook_events`, `pr_checks` (expanded). **Migration sources for the DDL slices.**
- `docs/M14_INVESTIGATION.md` — full design-pass record. The eleven decisions in lines 605–625 and the slice-count breakdown in lines 152–164 are the authoritative scope reference.

Implementation surface and existing code touched:

- `src/ApiTool.Backend/Subscriptions/StripeGateway.cs` — current placeholder (`throw new NotImplementedException`); M14-008/009/010 replace.
- `src/ApiTool.Backend/Subscriptions/FakeStripeGateway.cs` — preserved for unit tests; the proration math diverges intentionally from real Stripe.
- `src/ApiTool.Backend/Notifications/{ISmtpSender,NoopSmtpSender}.cs` — interface in place; M14-014 adds `SendGridSmtpSender` + `SendTemplateAsync`.
- `src/ApiTool.Backend/PrChecks/PrChecksEndpoints.cs` — M4-007's storage-only endpoint; M14-018 wires the outbound GitHub Checks call into the existing controller.
- `cmd/curlew/license.go:28–30` — `--refresh` and `--debug` currently return `"Not yet implemented"`; M14-006 replaces.
- `cmd/curlew/main.go` — adds the `curlew login` subcommand wired through `internal/backend/`.
- `internal/license/jwt.go` — current `VerifyRS256`; the algorithm-agnostic rename to `VerifyJWT(token, jwks)` lands as part of M14-007 (per v4.2 "Offline JWT Verification" subsection).
- `deploy/self-hosted/` — M14-001's `FileKeyProvider` and M14-016's `FileGitHubAppKeyProvider` operator-facing key generation; the bundle's install runbook gains both procedures.

### M14 Open Decisions for `/backlog M14`

The eleven cross-cutting design questions identified in the investigation are settled — do NOT re-litigate them. The remaining open decisions are scoping decisions the Plan agent must resolve before generating tasks; defaults are proposed.

1. **Slice 14-021 e2e scope.** The convergence slice could verify the happy path only, or also exercise one failure mode per cluster (revoked refresh family → re-auth; webhook quarantine → 200 returned to Stripe; GitHub App suspended → degraded UI). **Default:** happy-path only in M14-021; failure-mode coverage lives in each cluster's own slice tests. The e2e slice's role is "all parts wire together," not "all failure modes covered."

2. **Stripe portal redirect URL.** `M14-009` (billing portal session) needs a `return_url` for Stripe to redirect back to. **Default:** read from `App:WebAppUrl` config (already present per backend env config) and append `/billing`. Document in the task `scope:`.

3. **Device-code `verification_uri`.** `M14-005` needs to render `verification_uri` in CLI output. **Default:** the backend returns both `verification_uri` (`https://app.apitool.dev/device`) and `verification_uri_complete` (with the user code pre-filled per RFC 8628 §3.3.1); the CLI prints the bare `verification_uri` plus the user code as the primary path and uses `verification_uri_complete` for the auto-open browser invocation. Matches `gh auth login` pattern.

4. **GitHub App slug for the SaaS product.** M14-016 needs an App registered on GitHub under a chosen slug. **Default:** the slug is operator-chosen at the SaaS deployment level (e.g., `curlew-checks`); the slug is configured via `GITHUB_APP__SLUG` env var. The `deploy/self-hosted/` bundle includes the runbook for the operator to register their own App with their preferred slug.

5. **Initial refresh-token cleanup migration.** When the v4.2 `refresh_tokens` schema replaces v4.1's, any existing rows must either migrate or be dropped. **Default:** there are no production refresh-token rows to preserve (M5-013/M5-014 shipped offline-grace machinery; refresh-token issuance has not yet shipped to users), so M14-001's migration is a `DROP TABLE refresh_tokens; CREATE TABLE refresh_tokens (...)` rather than an ALTER. Verify the assumption against `management/backlog.yaml` before generating M14-001's `scope:`.

6. **`pr_checks` migration.** v4.2.1's expansion adds many new columns to the M4-007 `pr_checks` table. **Default:** the migration is additive (`ALTER TABLE pr_checks ADD COLUMN ...` for each new column). Existing rows backfill `external_id` from `gen_random_uuid()`, `status` from a CASE expression on `state`, and leave the other v4.2.1 columns NULL. M14-018 carries this migration.

7. **GHES seam.** v4.2.1 Decision #21 explicitly excludes GHES from M14, but documents the future seam (`IGitHubApiUrlProvider`, `github_installations.api_base_url`). **Default:** the M14 implementation hardcodes `https://api.github.com` directly (no abstraction layer); the seam is added in the future GHES milestone. Premature abstraction is forbidden.

8. **SendGrid template authoring.** M14-015 ships six templates as MJML files. The visual design / copy is a separate work item — engineering ships syntactically-valid MJML with placeholder copy + correct variable substitution; product/design polishes copy in a later non-M14 commit. **Default:** M14-015's `scope:` notes this explicitly so reviewers do not block on copy aesthetics.

9. **Stripe-mock CI runtime cost.** `stripe-mock` adds ~30s container startup to backend CI runs. **Default:** acceptable as part of the existing backend gate detection in `./scripts/ci-local.sh`; no need for further optimisation. If M14-011's CI run pushes total CI time above 10 minutes, revisit by gating stripe-mock on `[Trait]`-filtered tests only.

10. **Web-track coverage.** M14-020 is the single web slice; broader dashboard work for billing/account is NOT in M14. **Default:** M14-020's `scope:` is bounded to the GitHub install entry-point and install-state view only. Any web work that touches billing UI, license display, or account settings is rejected from M14 and routed to a later milestone.

## Phase 15 — Tier-matrix corrections (M15)

M15 is a small, surgical milestone that closes three documented "revenue leak" or "no-scripting story" line items in the tier matrix. The originally-proposed M15 in `docs/REVIEW.md:189` was three slices (`license --refresh`, `license --debug`, plugin tier-gate); the first two were absorbed by M14-006 ([cmd/curlew/license.go:289](cmd/curlew/license.go:289), :450), so M15 here is rescoped to a coherent "tier-matrix corrections" theme with three independent slices. Two of the slices (SSO Enterprise gate, `from_command` retiering) were originally listed in REVIEW.md's M16 scope at REVIEW.md:191 as "trivial subset (SSO tier gate, `from_command` retiering) is ready" — pulling them forward into M15 leaves M16 lighter.

| Prefix | Capability Key         | Phase Name                       | Slices | Status  |
|--------|------------------------|----------------------------------|--------|---------|
| `M15`  | `tier_matrix`          | M15: Tier-matrix corrections     | 3      | backlog |
|        |                        | **Total**                        | **3**  |         |

### M15 Capability Description

- **tier_matrix** — three independent registry/enforcement corrections. (1) `plugin_loading` Enterprise feature gate: add `plugin_loading: TierEnterprise` to `internal/auth/registry.go` (currently no entry exists, so `CURLEW_PLUGINS` loads at any tier — direct revenue leak per REVIEW.md:79); wire the gate check in `cmd/curlew/plugins.go` (`buildHookDispatcher` at line 32 and `pluginsListCmdOut` at line 91) before either `host.Load*` call so non-Enterprise tiers get the standard `auth.Gate` error and exit code rather than silently loading. (2) SSO Enterprise tier gate: the backend's `Subscription.Tier` field exists (`src/ApiTool.Backend/Data/Entities/Subscription.cs:13`, enum at `SubscriptionTier.cs`) but the SSO endpoints in `src/ApiTool.Backend/Sso/` do no tier check, so any subscription level can configure and use SSO — direct revenue leak per REVIEW.md:107. The gate runs in `SsoService` (or a new `SsoTierGate` service) and rejects non-Enterprise orgs with HTTP 402 Payment Required for the authenticated config endpoints (`/api/v1/organizations/{id}/sso` — SAML and OIDC) and HTTP 404 for the unauthenticated flow endpoints (`/api/v1/sso/saml/{orgId}/{login,acs}`, `/api/v1/sso/oidc/{orgId}/{login,callback}`) so existence isn't leaked to non-customers. (3) `from_command` Free-tier retiering: change `from_command` registry entry from `TierSolo` to `TierFree` at `internal/auth/registry.go:43`, and update its `Description` and `Workaround` strings; update seven MANUAL.md sites (`docs/MANUAL.md:65`, `:817`, `:2701`, `:2724`, `:2756`, `:3688`, plus the description at `:3581`) to remove the `[Solo]` badge or replace with `[Free]`. The variable system already supports `from_command` at any tier — this is a registry-level retiering, not a feature implementation.

### M15 Dependency Graph (DAG)

```
(M14 done)
│
├─ M15-001  plugin_loading Enterprise gate         [track: go-cli]
├─ M15-002  SSO Enterprise tier gate               [track: backend]
└─ M15-003  from_command to Free tier              [track: go-cli]
```

All three slices are mutually independent and can land in any order. None depend on each other. None depend on M16's pending design pass.

### M15 Track Assignments

| Track    | Slices                | Count |
|----------|-----------------------|-------|
| go-cli   | M15-001, M15-003      | 2     |
| backend  | M15-002               | 1     |
|          | **Total**             | **3** |

Each generated M15 task YAML MUST include a `track:` field. M15-001 and M15-003 use `track: go-cli`; M15-002 uses `track: backend`.

### M15 Spec / Source Anchors

There is no SPECIFICATION.md anchor for the M15 corrections — they're tier-matrix bookkeeping, not new spec surface. Authoritative references:

- `docs/REVIEW.md:79` — gap 3 (plugin tier gate as Enterprise-only per `MANUAL.md:3066`; not enforced)
- `docs/REVIEW.md:107` — gap 16 (SSO not tier-gated; revenue leak)
- `docs/REVIEW.md:129` — gap 23 (`from_command` to free tier; pricing/no-scripting story)
- `docs/REVIEW.md:189` — original M15 scope (rescoped here after M14-006 absorbed two of the three items)
- `docs/REVIEW.md:191` — M16 scope explicitly calls out the "trivial subset (SSO tier gate, `from_command` retiering) is ready" — i.e. unblocked
- `docs/MANUAL.md:3066` — current plugin tier-gate doc claim ("Enterprise-only")
- `docs/MANUAL.md:65, 817, 2701, 2724, 2756, 3581, 3688` — `from_command` `[Solo]` references that flip to `[Free]`

Implementation surface and existing code touched:

- `internal/auth/registry.go:35–143` — feature registry, `DefaultRegistry()`. M15-001 adds a new entry; M15-003 changes `from_command`'s `RequiredTier` and copy.
- `internal/auth/tier.go:7–20` — `Tier` constants and ordering. M15 reads-only; no modifications expected.
- `internal/auth/gate.go` — gate enforcement entry point (used by callers to check whether the active license supports a feature; M15-001 wires its check through this).
- `cmd/curlew/plugins.go:32, 91` — the two plugin-host call sites that need the gate check before `host.LoadForRun` / `host.Load`.
- `internal/variable/command.go` — `from_command` execution; behavior unchanged. M15-003 verifies any tier-gate code path here is updated (or removed if the registry is the sole gate site).
- `src/ApiTool.Backend/Sso/SamlEndpoints.cs:17–46`, `OidcEndpoints.cs:17–40` — endpoint registration. M15-002 inserts the tier-gate check (probably as a small filter/middleware or inline `SsoService` precondition) for all SAML and OIDC routes, both authenticated and public.
- `src/ApiTool.Backend/Sso/SsoService.cs` — central SSO service; the most natural site for the tier-gate check (lookup org's active subscription, reject if non-Enterprise).
- `src/ApiTool.Backend/Data/Entities/Subscription.cs:13` — `Tier` field used by the gate. The Organization entity itself has no tier column; tier reads via the org's active Subscription row.
- `docs/MANUAL.md:65, 817, 2701, 2724, 2756, 3066, 3581, 3688` — documentation updates carried per slice.

### M15 Open Decisions for `/backlog M15`

1. **SSO tier-gate enforcement layer.** Three options: (a) ASP.NET Core endpoint filter / authorization handler that runs before the route, returning 402/404; (b) inline check inside `SsoService` methods, throwing a typed exception that the endpoint translates; (c) middleware on `MapGroup`. **Default:** option (b) — inline service-layer check. Keeps the gate in one place that can be unit-tested with the existing `SsoService` test surface, and avoids spreading auth-policy types into the SSO endpoint files. Authenticated endpoints translate the typed exception to 402; public endpoints translate to 404.
2. **Existing seeded SSO configs on non-Enterprise orgs.** Dev fixtures may have SSO configured on Free/Professional/Team orgs; the tier gate would lock them out at next deploy. **Default:** the gate is deny-by-default at the active-subscription read; if dev-seed orgs lack a subscription row, treat as Free → SSO denied. Update dev seed (if any) to seed an Enterprise subscription on whichever org has SSO configured. Surface this as a one-line note in M15-002's `scope:`.
3. **Public flow endpoint 404 vs 403.** Per REVIEW.md the recommendation is 404 to avoid leaking existence. **Default:** 404 with no body; leak nothing. The `Cache-Control: no-store` header should be set on these 404s so non-Enterprise org IDs aren't cached as "non-existent" by intermediaries.
4. **Plugin gate exit code.** The existing `auth.Gate` returns a structured error that `cmd/curlew/main.go` maps to exit code 6. **Default:** plug into the same path. M15-001 reuses `auth.Gate` rather than introducing a new error shape; the user-facing message comes from the registered `Description` and `Workaround` strings.
5. **`from_command` retiering migration impact.** Existing collections that worked under Solo continue working under Free (more permissive — backwards compatible). No tests need new fixtures; existing Solo-tier from_command tests should now also pass at TierFree. **Default:** M15-003 updates the test matrix in `internal/auth/registry_test.go` and any tier-gate matrix tests in the variable package; no new fixtures.
6. **MANUAL.md tier-badge sweep.** Seven sites referencing `[Solo]` for `from_command`. **Default:** M15-003 updates all seven in a single commit; no scope creep into other tier-badge corrections, even if the sweep surfaces other inconsistencies (those become a separate issue).
7. **Plugin tier gate vs built-in signers (M17).** M17-001's MANUAL.md clarification distinguished built-in signers/dynamic functions (universally available) from user-supplied plugins (Enterprise gate). M15-001 closes the user-supplied side. **Default:** verify the M17-001 clarification text at `docs/MANUAL.md:3066` is unchanged by M15-001's enforcement work; if the clarification needs sharpening (e.g. explicit "user-supplied plugins via `CURLEW_PLUGINS` are Enterprise-only"), include the doc-fix in M15-001.

## Phase 16 — Workflow completion (M16)

M16 is the "Team tier actually works" milestone. Through M14, scheduling, PR-checks, and shared-vault all had partial scaffolding (entities and endpoints) but no end-to-end workflow. M16 closes the workflow gaps for six themes plus a small refactor:

- **Password reset + email verification** — endpoints, state, web pages, and the two SendGrid templates deferred from M14 (`password_reset`, `trial_expiring`).
- **Trial JWT issuance + on-demand re-trials** — `ITrialStateResolver`, `trials` table, activation endpoint, CLI claims-struct extension, and the `trial_expiring` cron.
- **Schedule executor** — self-hosted runner via `curlew worker --schedule-pull`; no backend-resident execution.
- **GitLab Commit Status API parity** — PAT-based, parallel to M14's GitHub Checks integration.
- **Shared vault for Team tier** — backend storage of `team_vaults`, CLI propagation with 5-min TTL cache, enforcement at the existing `CURLEW_TEAM_CONFIG` load site (closes the M15-style revenue leak).
- **Health metrics dashboard** — pinned `/results/stats` and `/results/failures` schemas; on-demand SQL aggregation.
- **Tier-gate generic abstraction** — `ITierGate.EnsureAsync` lifting `SsoTierGate`'s pattern so the three new gates above don't copy-paste the EF query.

The design pass is captured in two artefacts:

1. `docs/M16_INVESTIGATION.md` — the complete pre-work investigation (2026-05-07) including the residual-scope correction (M14/M15 absorbed REVIEW.md gaps 12-GitHub, 16, 23, plus parts of 9 and 10) and the nine cross-cutting design questions REVIEW.md missed.
2. `docs/SPECIFICATION.md` v4.3 (header + Changelog at lines 4–43; new top-level sections "Schedule Execution Model", "Password Reset & Email Verification Flow", "GitLab Commit Status API Integration"; rewritten "Layer 4: Shared Vault Configuration Templates"; extended "Trial Persistence and Activation" subsection; new "Tier-Gate Generic Abstraction" subsection under Backend Architecture; expanded "Test Results Dashboard" with response schemas; nine new tables in the schema appendix). The fifteen v4.3 cross-cutting decisions (`v3-1` through `v3-15`) are the authoritative scope reference.

Everything below is grounded in those two artefacts; do NOT reopen settled decisions during `/backlog M16`.

M16 spans all three tracks (`backend`, `go-cli`, `web`) plus a final convergence slice. Heterogeneous in the same sense as M4/M5/M14.

| Prefix | Capability Key             | Phase Name                       | Slices | Status  |
|--------|----------------------------|----------------------------------|--------|---------|
| `M16`  | `tier_gate_generic`        | M16: Workflow completion         | 1      | backlog |
| `M16`  | `password_reset_verification` | M16: Workflow completion      | 3      | backlog |
| `M16`  | `trial_resolver`           | M16: Workflow completion         | 4      | backlog |
| `M16`  | `schedule_executor`        | M16: Workflow completion         | 4      | backlog |
| `M16`  | `gitlab_pr_checks`         | M16: Workflow completion         | 4      | backlog |
| `M16`  | `shared_vault_propagation` | M16: Workflow completion         | 2      | backlog |
| `M16`  | `health_dashboard`         | M16: Workflow completion         | 2      | backlog |
| `M16`  | `m16_e2e`                  | M16: Workflow completion         | 1      | backlog |
|        |                            | **Total**                        | **21** |         |

Slice counts are bounded estimates per the investigation (16–22 range). The Plan agent may refine by ±10%; significant deviation should surface as an explicit ambiguity rather than a silent rescope.

### M16 Capability Descriptions

- **tier_gate_generic** — single foundation slice that lifts `SsoTierGate.EnsureEnterpriseAsync` ([src/ApiTool.Backend/Sso/SsoTierGate.cs:24](src/ApiTool.Backend/Sso/SsoTierGate.cs)) into a generic `ITierGate.EnsureAsync(orgId, requiredTier, ct)` in `src/ApiTool.Backend/Internal/TierGates/` per `docs/SPECIFICATION.md` "Tier-Gate Generic Abstraction (v4.3)". Existing `SsoTierGate` becomes a thin call-site adapter; new `ScheduleExecutorTierGate`, `VaultConfigTierGate`, `DashboardTierGate` adapters land alongside. RFC 7807 mapping centralized: `402 Payment Required` for authenticated org-scoped endpoints with the org's tier in the problem detail; `404 Not Found` with `Cache-Control: no-store` for unauthenticated public-flow endpoints (mirrors M15-002). Test surface adds an injectable `ITierGate` (the existing static `SsoTierGate` is replaced by the interface; existing call sites at `SsoService.cs:37, 141, 186, 221`, `OidcService.cs:33, 142, 193` keep their signatures).

- **password_reset_verification** — three slices closing REVIEW.md gap 9 with the auth-model resolution from v4.3 (Argon2id + token-based reset, magic-link retired). (1) Migrations + entities for `password_reset_tokens` and `email_verification_tokens` tables (per "Password Reset & Email Verification Flow → Token Tables"); add `users.email_verified` boolean column; add `password_reset` and `trial_expiring` SendGrid templates to `EmailTemplateInventory` + MJML sources + manifests (these are the two templates explicitly deferred from M14-015's six-template inventory). (2) Endpoints `POST /api/v1/auth/password-reset/{request,confirm}` and `POST /api/v1/auth/email-verification/{resend,confirm}` + service layer (`IPasswordResetService`, `IEmailVerificationService`) + per-email and per-IP rate limits via existing `Internal/RateLimit` (3 reset/24h, 5 verify/24h per email; 10/24h per IP) + identical 200 responses regardless of email existence (enumeration defense). On confirm-success: invalidate ALL of the user's refresh-token families per RFC 9700 §4.14. The `[RequireVerifiedEmail]` filter wires verification gating onto Stripe checkout and org-invite-acceptance endpoints only — free use is unblocked (per "Verification Gating Policy"). (3) Web pages at `/auth/password-reset/{request,confirm}` and `/auth/email-verification/{request,confirm}` in `web/src/routes/auth/`; password-strength validator uses zxcvbn-server-side `score >= 3`; email-verification confirmation auto-redirects to dashboard.

- **trial_resolver** — four slices closing REVIEW.md gap 10. (1) `trials` table migration + entity per "Trial Persistence and Activation"; `UNIQUE (user_id, feature)` enforces one-trial-per-feature-per-user globally. (2) `ITrialStateResolver` interface + `DatabaseTrialStateResolver` implementation; `LicenseTokenIssuer` ([LicenseTokenIssuer.cs:54-55](src/ApiTool.Backend/Licensing/Tokens/LicenseTokenIssuer.cs)) replaces hardcoded `"none"`/`null` defaults with resolver calls; the v4.3 trial-state transition rules (active / expired / preempted_by_subscription) are enforced at `LicenseTokenIssuer` issuance time; tier-upgrade preemption fires on `customer.subscription.created` Stripe webhook handler. (3) `POST /api/v1/trials/{feature}` endpoint with `409 Conflict` on already-consumed feature; re-mints all three tokens in the response so the CLI sees the new feature on next invocation; CLI `curlew license trial start <feature>` subcommand calls it (extends `cmd/curlew/license.go:41` switch); `backend.Client.StartTrial(ctx, feature)` method. **Includes the `Claims` struct extension in [internal/license/jwt.go](internal/license/jwt.go)** — add `TrialState string \`json:"trial_state,omitempty"\`` and `TrialExpiry int64 \`json:"trial_expiry,omitempty"\`` fields and the `IsTrialActiveFor(feature string) bool` accessor; integrate at every premium-feature gate in `internal/auth/registry.go`. (4) `TrialExpiryNotifier` daily cron at 09:00 UTC: queues `trial_expiring` SendGrid emails at 3-day and 1-day expiry marks; uses `notified_*_at` columns to prevent duplicates.

- **schedule_executor** — four slices closing REVIEW.md gap 11 with the v4.3 self-hosted runner architecture. (1) Backend endpoints `GET /api/v1/schedules/next-run`, `POST /api/v1/schedules/runs/{run_id}/heartbeat`, `POST /api/v1/schedules/runs/{run_id}/result` per "Schedule Execution Model → Endpoint Reference"; `claim_token` mechanism prevents two workers running the same `scheduled_run`; `ShardReaper` extended to reap stale schedule runs after 5-minute heartbeat timeout. (2) `scheduled_runs.result_id` FK migration linking to `results` table; result-ingest path sets the FK on completion. (3) `curlew worker --schedule-pull` mode in `cmd/curlew/worker.go` (or a new flag if the file is structured differently); reuses M11's distributed-perf job/shard polling pattern; resolves `collection_ref` per "Collection Source-of-Truth" (`file:` and `git:` schemes); fetches Layer 4 shared vault config at startup with 5-min TTL refresh; on result-post failure, queues to `~/.config/curlew/pending-uploads/`. (4) `schedules.timezone` migration + IANA TZ-aware Cronos invocation; web dashboard schedules page at `/org/[slug]/schedules` per "Web Dashboard → Team Features" with timezone picker, cron live-preview, run-now button, per-schedule run history page; tier-gated by `ScheduleExecutorTierGate`.

- **gitlab_pr_checks** — four slices closing REVIEW.md gap 12 GitLab side, with the v4.3 PAT-based architecture. (1) `gitlab_installations` table migration + `IGitLabKeyProvider` interface with `FileGitLabKeyProvider` (`deploy/self-hosted/`) and `GoogleKmsGitLabKeyProvider` (SaaS HSM tier); per-row AES-256-GCM ciphertext for the PAT under a wrapped DEK; `gitlab_webhook_events` table + idempotency on `X-Gitlab-Event-UUID`. (2) Outbound `IGitLabCheckPoster` against `POST /projects/{id}/statuses/{commit_sha}` with the lossy state mapping from v3-GL-4 (timed_out → failed, neutral/skipped → success+marker); `pr_checks.provider` discriminator + nullable `gitlab_installation_id`/`gitlab_status_id` columns added. (3) Inbound `POST /webhooks/gitlab` with `X-Gitlab-Token` constant-time verification + 5-failure quarantine + multi-secret rotation via `GITLAB__WEBHOOK_SECRETS_<installation_id>`; `Pipeline Hook` handler (the only event the M16 dispatcher acts on; `Push Hook` and `Merge Request Hook` stored for debugging only). (4) Web dashboard `/org/[slug]/integrations/gitlab` with PAT submission form, project-path lookup, optional `gitlab_base_url` and `gitlab_ca_bundle` for self-managed GitLab; connection-health table for existing installations.

- **shared_vault_propagation** — two slices closing REVIEW.md gap 14 with the v4.3 plaintext-with-coordinate-validation model. (1) `team_vaults` table migration + entity + `VaultConfigService` + manifest validator (rejects literal-secret-shaped values via the regex heuristic in "Layer 4 → Storage Model"); `GET/PUT/DELETE /api/v1/organizations/{orgId}/vault-config` endpoints with `vault_config.{view,manage}` RBAC + `VaultConfigTierGate.EnsureTeamOrAboveAsync` from the foundation slice; web dashboard `/org/[slug]/vault-config` YAML editor page with manifest-validator-on-save and "generate CLI snippet" button. (2) CLI side: `backend.Client.GetTeamVault(ctx, orgId)` method; cache file at `~/.config/curlew/team_vault.json` with 5-min TTL + stale-while-revalidate fetch + `--refresh-vault` flag; integration in `internal/vault/teamtemplate/` so the backend-fetched template is the base and `CURLEW_TEAM_CONFIG` (if set) overlays per-key; **enforcement at the load site** — when the CLI sees a team-vault config request and the active JWT's tier is below `team` AND `IsTrialActiveFor("shared_vault_templates")` is false, the standard feature-gated error fires with exit code 5. Closes the registered-but-not-enforced revenue leak.

- **health_dashboard** — two slices closing REVIEW.md gap 15. (1) `GET /api/v1/organizations/{orgId}/results/stats` and `/results/failures` endpoints per "Test Results Dashboard" response schemas; on-demand SQL aggregation against `(org_id, created_at)` index — no rollup table; window selector `7d|30d|90d` (default 30d); failure grouping by `(method, path_template)` with the same path-template extraction the result-ingest pipeline already uses; `DashboardTierGate.EnsureTeamOrAboveAsync`. (2) Web dashboard `/org/[slug]/dashboard` Svelte page with overview cards (total runs, pass-rate %, avg duration, p50/p95), pass-rate trend line chart (daily aggregation), recent-runs table (existing data shape), frequently-failing-endpoints list (top 10).

- **m16_e2e** — single convergence slice that proves the full workflow end-to-end: registration with Argon2id password → email verification flow → 14-day full-trial JWT issued → schedule created via dashboard → `curlew worker --schedule-pull` claims and executes → result ingested + `scheduled_runs.result_id` linked → `/results/stats` reflects the new run → trial expiry notification queued at the 3-day mark → on-demand trial activated for one feature → password reset flow → all token families revoked. Mirrors M14-021's role.

### M16 Dependency Graph (DAG)

```
(M14 done; M15 done; M11/M12/M13/M17 done)
│
├─ tier-gate foundation (must land first; gates 4 downstream slices) ─
│  M16-001  ITierGate.EnsureAsync + adapters refactor                [track: backend]
│
├─ password reset + email verification (independent of trial) ────────
│  M16-002  password_reset_tokens + email_verification_tokens
│           + email_verified column + password_reset/trial_expiring
│           templates                                                  [track: backend]
│           │
│           └─ M16-003  endpoints + service + rate limits +
│                       [RequireVerifiedEmail] filter                  [depends on M16-002, track: backend]
│                       │
│                       └─ M16-004  web pages                          [depends on M16-003, track: web]
│
├─ trial resolver (depends on M16-001 for tier-aware gating) ─────────
│  M16-005  trials table + entity                                     [track: backend]
│           │
│           └─ M16-006  ITrialStateResolver + LicenseTokenIssuer      [depends on M16-005, track: backend]
│                       │
│                       ├─ M16-007  POST /trials/{feature} endpoint
│                       │           + Claims struct + IsTrialActiveFor
│                       │           + curlew license trial start     [depends on M16-006, track: go-cli + backend (split slice)]
│                       │
│                       └─ M16-008  TrialExpiryNotifier daily cron     [depends on M16-006, track: backend]
│
├─ schedule executor (depends on M16-001 for tier gate) ──────────────
│  M16-009  /schedules/next-run + /heartbeat + /result endpoints
│           + claim_token + ShardReaper extension                      [depends on M16-001, track: backend]
│           │
│           ├─ M16-010  scheduled_runs.result_id migration + linkage   [depends on M16-009, track: backend]
│           ├─ M16-011  curlew worker --schedule-pull mode            [depends on M16-009, track: go-cli]
│           └─ M16-012  schedules.timezone migration + web schedules  [depends on M16-001, M16-009, track: backend + web]
│
├─ GitLab PR-check parity (independent of other clusters) ────────────
│  M16-013  gitlab_installations + IGitLabKeyProvider + KMS/file
│           + gitlab_webhook_events                                    [track: backend]
│           │
│           ├─ M16-014  outbound IGitLabCheckPoster + provider
│           │           discriminator on pr_checks                     [depends on M16-013, track: backend]
│           ├─ M16-015  inbound /webhooks/gitlab                       [depends on M16-013, track: backend]
│           └─ M16-016  web /integrations/gitlab dashboard page        [depends on M16-013, track: web]
│
├─ shared vault (depends on M16-001 for tier gate) ───────────────────
│  M16-017  team_vaults + endpoints + manifest validator
│           + web vault-config page                                    [depends on M16-001, track: backend + web]
│           │
│           └─ M16-018  CLI GetTeamVault + cache + load-site
│                       enforcement                                    [depends on M16-017, track: go-cli]
│
├─ health metrics dashboard (depends on M16-001 for tier gate) ───────
│  M16-019  /results/stats + /results/failures endpoints              [depends on M16-001, track: backend]
│           │
│           └─ M16-020  web /dashboard page                            [depends on M16-019, track: web]
│
└─ Convergence
   M16-021  e2e: registration → verification → trial → schedule run
            → vault → dashboard → trial expiry → on-demand trial
            → password reset → token revocation                        [depends on M16-004, M16-008, M16-011, M16-018, M16-020, M16-014, track: e2e]
```

Tracks are labels, not separate DAGs. The DAG is one graph spanning all four tracks. The tier-gate foundation slice (M16-001) is a strict prerequisite for the trial / schedule / vault / dashboard clusters; it could be folded into one of those slices but extracting it cleanly is worth one dedicated slice.

### M16 Parallel Work Opportunities

Five independent clusters open mid-milestone after M16-001 lands:

1. **Auth flow** (M16-002) — independent of tier-gate; can begin immediately.
2. **Trial resolver** (M16-005) — depends on M16-001; opens once foundation is in.
3. **Schedule executor** (M16-009) — depends on M16-001.
4. **GitLab parity** (M16-013) — fully independent of M16-001; can run parallel from start.
5. **Shared vault + dashboard** (M16-017, M16-019) — depend on M16-001.

Within each cluster, the dashboard / web slices are the latest possible (require backend endpoints first). The convergence slice M16-021 requires the full happy-path graph green.

### M16 Track Assignments

| Track    | Slices | Count |
|----------|--------|-------|
| backend  | M16-001, M16-002, M16-003, M16-005, M16-006, M16-008, M16-009, M16-010, M16-013, M16-014, M16-015, M16-019 | 12 |
| go-cli   | M16-011, M16-018 | 2 |
| web      | M16-004, M16-016, M16-020 | 3 |
| split (backend + go-cli or backend + web) | M16-007, M16-012, M16-017 | 3 |
| e2e      | M16-021 | 1 |
|          | **Total** | **21** |

Each generated M16 task YAML MUST include a `track:` field. Split slices include both tracks they touch in the value (e.g., `track: backend+go-cli`).

### M16 Spec Anchors

The v4.3 spec edition is the authoritative reference for every M16 slice. Specific anchors:

- `docs/SPECIFICATION.md` "Layer 4: Shared Vault Configuration Templates (Team Tier, $39/month)" — backend storage model, propagation, tier-gate enforcement, override precedence. **Primary anchor for M16-017, M16-018.**
- `docs/SPECIFICATION.md` "Trial Persistence and Activation (v4.3)" — uniqueness scope, JWT claim transitions, activation endpoint, tier-upgrade preemption, expiry cron. **Primary anchor for M16-005 through M16-008.**
- `docs/SPECIFICATION.md` "Schedule Execution Model" — self-hosted runner architecture, worker modes, endpoint reference, collection source-of-truth, secrets propagation, timezone, failure handling. **Primary anchor for M16-009 through M16-012.**
- `docs/SPECIFICATION.md` "Password Reset & Email Verification Flow" — auth-model resolution, threat model, token tables, endpoints, verification gating policy, M16 SendGrid templates, CLI surface. **Primary anchor for M16-002 through M16-004.**
- `docs/SPECIFICATION.md` "GitLab Commit Status API Integration" — PAT auth, threat model, architecture, encryption, outbound API, inbound webhook, self-managed support, GL-1 through GL-7 decisions. **Primary anchor for M16-013 through M16-016.**
- `docs/SPECIFICATION.md` "Tier-Gate Generic Abstraction (v4.3)" — `ITierGate.EnsureAsync` interface, per-feature adapters, RFC 7807 mapping. **Primary anchor for M16-001.**
- `docs/SPECIFICATION.md` "Web Dashboard → Team Features → Test Results Dashboard" — `/results/stats` and `/results/failures` response schemas, daily aggregation, failure taxonomy. **Primary anchor for M16-019, M16-020.**
- `docs/SPECIFICATION.md` "Database Schema Reference (Appendix)" — `team_vaults`, `password_reset_tokens`, `email_verification_tokens`, `trials`, `gitlab_installations`, `gitlab_webhook_events`, expanded `schedules` and `scheduled_runs`. **Migration sources for the DDL slices.**
- `docs/SPECIFICATION.md` "Decisions Resolved (v4.3, all M16 themes)" — the fifteen v3-* decisions are commitments, not proposals.
- `docs/M16_INVESTIGATION.md` — full design-pass record with the residual-scope correction and cross-cutting-questions table.

Implementation surface and existing code touched:

- `src/ApiTool.Backend/Sso/SsoTierGate.cs` — refactored to call-site adapter over `ITierGate` (M16-001).
- `src/ApiTool.Backend/Licensing/Tokens/LicenseTokenIssuer.cs:54–55` — hardcoded trial defaults replaced by `ITrialStateResolver` calls (M16-006).
- `src/ApiTool.Backend/Schedules/SchedulerHost.cs`, `SchedulesService.cs`, `ISchedulerEnqueuer.cs` — extended for the worker-pull model; `ScheduledRun` entity gains `claim_token`, heartbeat columns, `result_id` (M16-009, M16-010).
- `src/ApiTool.Backend/Notifications/Email/EmailTemplateInventory.cs:18-24` — adds `password_reset` and `trial_expiring` slugs (M16-002, M16-008).
- `src/ApiTool.Backend/Auth/AuthLoginEndpoints.cs` — kept (Argon2id model preserved); password-reset and email-verification endpoints land in new files alongside (M16-003).
- `src/ApiTool.Backend/PrChecks/` — `PrCheck` entity gains `provider` discriminator + nullable `gitlab_*` columns (M16-014); existing GitHub paths unchanged.
- `internal/license/jwt.go` — `Claims` struct gains `TrialState` and `TrialExpiry` fields + `IsTrialActiveFor(feature)` accessor (M16-007).
- `internal/auth/registry.go` — feature gates consult `IsTrialActiveFor` before rejecting on `RequiredTier` (M16-007).
- `internal/vault/teamtemplate/` — backend-fetched template integration; load-site enforcement (M16-018).
- `internal/backend/client.go` — new methods `GetTeamVault`, `StartTrial`, schedule worker poll/post (M16-007, M16-011, M16-018).
- `cmd/curlew/license.go` — new `trial start <feature>` subcommand (M16-007).
- `cmd/curlew/worker.go` (or equivalent) — new `--schedule-pull` flag (M16-011).
- `web/src/routes/auth/password-reset/`, `web/src/routes/auth/email-verification/` — new pages (M16-004).
- `web/src/routes/(app)/org/[slug]/schedules/`, `dashboard/`, `vault-config/`, `integrations/gitlab/` — new pages (M16-012, M16-016, M16-017, M16-020).

### M16 Open Decisions for `/backlog M16`

The fifteen v4.3 cross-cutting decisions (`v3-1` through `v3-15`) are settled. The remaining open decisions are scoping decisions the Plan agent must resolve before generating tasks; defaults are proposed.

1. **M16-007 split shape.** This slice spans `backend` (endpoint) and `go-cli` (Claims struct + subcommand + client method). **Default:** keep as one logical slice with `track: backend+go-cli`; the slice's observable is "user runs `curlew license trial start vault_provider_profiles` and the next `curlew run` succeeds for that feature." Splitting in two creates ordering problems (the CLI needs the endpoint to exist; the endpoint is useless without the CLI consumption). Plan agent may split into two if it judges the seam is clean.

2. **`gitlab_base_url` validation.** Self-managed GitLab instances may use IP addresses, internal hostnames, or HTTP-not-HTTPS in dev. **Default:** require HTTPS in prod via config flag (`GITLAB__ALLOW_HTTP=true` to bypass for dev); validate the URL parses but don't reject by hostname (allow IP and `.local` and similar).

3. **GitLab webhook `X-Gitlab-Token` ad-hoc rotation.** The constant-time compare against a single secret is the minimum; multi-secret rotation is documented in v3-GL-5. **Default:** M16-015 ships single-secret support via `GITLAB__WEBHOOK_SECRETS_<installation_id>`; multi-secret rotation lands in a follow-up. State explicitly in M16-015's `scope:`.

4. **Schedule executor — `git:` collection ref scope.** v4.3 names two schemes (`file:`, `git:`). **Default:** M16-011 ships `file:` only; `git:` is documented as a future extension. The customer's worker startup script can `git clone` ahead of time. Adds simplicity without losing real customers — anyone who wants `git:` runs `git pull` in their startup.

5. **Vault template manifest validator strictness.** The literal-secret-detection regex (`^[A-Za-z0-9+/=._-]{16,}$` not matching a recognized provider coordinate pattern) is heuristic. **Default:** M16-017 validator is **warn-on-suspicious** in dev mode (logs a warning, accepts), **reject-on-suspicious** in prod mode (returns 422). Ship in dev-warn mode initially; flip to reject mode after a 30-day soak. Document in M16-017's `scope:`.

6. **Stripe checkout email verification gate UX.** The `[RequireVerifiedEmail]` filter on `POST /api/v1/subscriptions/checkout` returns 403. **Default:** the dashboard intercepts the 403 client-side and shows a "verify your email" modal with resend button rather than letting the raw 403 propagate. The backend behaviour is unconditional 403; the UX is dashboard-only.

7. **Trial preemption on cancellation.** v3-6 settles preemption on subscription start. The mirror question — when a paid customer cancels and reverts to free, do their original 14-day trials remain `preempted_by_subscription`? **Default:** yes, they remain preempted. Re-issuing trials on cancellation creates a churn-loop incentive (cancel → re-trial → re-cancel). The "Trial Persistence and Activation → Subscription downgrade / cancellation" subsection states this; the migration on M16-005 must not reset trial rows.

8. **`password_reset_tokens` retention.** v3-2 says daily cleanup of dead rows older than 7 days. **Default:** the cleanup job is one of M16's hosted services (similar to `TrialExpiryNotifier`). Implement as a single `IHostedService` (`AuthTokenCleanupService`) that handles both `password_reset_tokens` and `email_verification_tokens` cleanup. Keeps the operational surface small.

9. **M16-021 e2e scope.** Same shape as M14-021. **Default:** happy-path only; failure modes covered in each cluster's own slice tests. The e2e slice's role is "all parts wire together," not "all failure modes covered."

10. **Dashboard `?window=` outside the menu.** v3-15 fixes `7d/30d/90d`. **Default:** if a user passes `?window=14d`, return 400 "Unsupported window. Allowed values: 7d, 30d, 90d." Don't silently accept arbitrary values.

11. **M16 web track scope.** Three new pages (auth flows, dashboard, vault-config, schedules, GitLab integration). **Default:** these are bounded as "minimum-viable working pages with the documented form fields" — visual polish is a separate work item. M16 web slices state explicitly that copy/visual aesthetics are not blocking review. Same convention as M14-020.

12. **M16-001 enforcement-completeness verification.** The four new tier-gate adapters (Schedule, Vault, Dashboard) plus the existing SSO adapter must all route through `ITierGate.EnsureAsync`. **Default:** M16-001 ships with a unit test that verifies all four adapters delegate to a mocked `ITierGate`; static analysis (or grep) verifies no remaining direct `db.Subscriptions.Where(...)` tier-checks exist outside `Internal/TierGates/`.


## Phase 17 — First-party signers and signing helpers (M17) — COMPLETE

M17 closes REVIEW.md gap #4 (`docs/REVIEW.md:81`). The originally-proposed framing was "ship first-party plugins" (`curlew-sigv4`, `curlew-oauth1`, `curlew-webhook-sig`, `curlew-jwt-decode`); after a verification pass against the codebase that framing was rescoped. Two structural facts forced the rescope:

1. **The existing `auth_profile` machinery cannot host request-mutating signing.** `internal/auth/profile.go:23` defines exactly one `ProfileType` (`"dynamic"`); the entire model is sub-collection-token-fetcher → extract-variables-into-outer-scope. SigV4/OAuth1 need access to the templated outer request (method, URL, headers, body) to compute a body hash and inject `Authorization` — fundamentally a different shape than profile dispatch.
2. **The right hook exists in-process but is exposed only via JSON-RPC.** `internal/plugin/hooks/hooks.go:36–42` defines a mutable `RequestPayload` (method, URL, headers, body, query params); `internal/runner/runner.go:38` defines `HooksDispatcher` as an interface with `OnRequest(ctx, RequestPayload) (RequestPayload, error)`; the runner already wraps `exec` with this hook (`runner.go:421–471`). The hook is exactly the surface SigV4 needs — but `*hooks.Dispatcher` is the only implementation today and it goes out-of-process to JSON-RPC plugins.

The cleanest fit is a built-in signer registry invoked through a new request-level `signing:` field, sitting alongside the runner's existing exec-wrap seam. Webhook signing is a different shape (pure body+secret → string, no request mutation) and is best served by dynamic functions building on M12-005's `$hmacSha256`. JWT decoding is pure standard-library work and is also best served by dynamic functions. Net: M17 ships zero plugin binaries, two built-in signer types, and five dynamic functions.

| Prefix | Capability Key         | Phase Name                                  | Slices | Status  |
|--------|------------------------|---------------------------------------------|--------|---------|
| `M17`  | `first_party_signing`  | M17: First-party signers and signing helpers | 5      | done    |
|        |                        | **Total**                                   | **5**  |         |

### M17 Capability Description

- **first_party_signing** — five slices closing the no-scripting story for the four canonical signing scenarios named in REVIEW.md:81 (AWS SigV4, OAuth1, webhook signatures, JWT decode):
  - **M17-001 (foundation):** new `internal/signer/` package with a `Signer` interface, named registry, and `signing:` request-level YAML field (also collection-level default, mirroring M2-008/M2-010's auth_profile precedence). The runner gains a signer-invocation step between templating and transport, sitting alongside the existing `OnRequest` plugin-hook seam (`internal/runner/runner.go:421–471`). M17-001 is foundation-only — no concrete signer ships in this slice.
  - **M17-002:** AWS Signature Version 4 signer (`type: aws-sigv4`). Signs the canonical request per AWS specification, injects `x-amz-content-sha256` before signature computation, injects `Authorization: AWS4-HMAC-SHA256 ...` after.
  - **M17-003:** OAuth 1.0a signer (`type: oauth1`). HMAC-SHA1 default, HMAC-SHA256 via explicit `method:` field. Computes OAuth1 base string, injects `Authorization: OAuth oauth_signature=...`.
  - **M17-004:** webhook-sig dynamic functions — `$webhookSign.stripe(body, secret, [timestamp])`, `$webhookSign.github(body, secret)`, `$webhookSign.slack(body, secret, [timestamp])`. Each returns the provider's signature header value (e.g. `t=...,v1=...` for Stripe). Built on `$hmacSha256` from M12-005.
  - **M17-005:** JWT decode dynamic functions — `$jwtDecodeHeader('token')`, `$jwtDecodeClaims('token')`. Pure decode-only (no signature verification); returns the JSON-string of the decoded segment for downstream JSONPath assertion.

  Sensitive-secret propagation: when any signer or webhook-sig function resolves a secret/key from a sensitive variable, the resolved value is added to the run's SensitiveSet at signing time so it is redacted in serialised output. Mirrors the M12-005 pattern for `$hmacSha256`.

  Documentation cleanup carried in M17-001: `docs/MANUAL.md:2761` currently cites `curlew-sigv4` as the canonical plugin example; this is replaced with the built-in `signing: {type: aws-sigv4, ...}` example. `docs/MANUAL.md:3066` (plugin tier gate as Enterprise-only) is clarified to apply only to user-supplied plugins loaded via `CURLEW_PLUGINS`; built-in signers and dynamic functions are universally available across tiers.

### M17 Dependency Graph (DAG)

```
(M12 done; M13-001 recommended — see Open Decision #4)
│
├─ M17-001  signer registry + `signing:` request field foundation
│           ├─ M17-002  AWS SigV4 signer
│           └─ M17-003  OAuth1 1.0a signer
│
├─ M17-004  webhook-sig dynamic functions ($webhookSign.stripe / .github / .slack)
│           [depends on M12-005 (done) for $hmacSha256; recommended on M13-001 for dotted-namespace]
│
└─ M17-005  JWT decode dynamic functions ($jwtDecodeHeader, $jwtDecodeClaims)
            [depends only on M12 (done)]
```

M17-001 is a strict prerequisite for M17-002 and M17-003. M17-004 and M17-005 are independent of M17-001 — they're pure dynamic functions and can ship in any order. After M17-001 lands, M17-002 and M17-003 are mutually independent and parallelisable.

### M17 Track Assignments

All five tasks are `track: go-cli`. No backend or web work in this milestone.

### M17 Spec Anchors

There is no SPECIFICATION.md anchor for these features today — the spec stops short of mandating SigV4/OAuth1/webhook-sig/JWT-decode (REVIEW.md:81 calls this out). M17 ships ahead of a v4.2 spec edition; the milestone-mapping section is the authoritative scope until the spec catches up. Supporting references:

- `docs/REVIEW.md:81` — gap #4 framing ("first-party plugins not shipped")
- `docs/REVIEW.md:193` — original "M17 — first-party plugins" milestone sketch (rescoped here)
- `docs/REVIEW.md:240` — readiness ("ready after M12 lands"; M12 is done)
- `docs/SCRIPTING.md` — the no-scripting argument that M17 closes by giving free-tier users genuine escape hatches
- `docs/MANUAL.md:2761` — canonical `curlew-sigv4` plugin reference (rescope target — replaced with built-in example in M17-001)
- `docs/MANUAL.md:3066` — Enterprise plugin tier gate (clarification target — applies to user-supplied plugins only)

Implementation surface:

- `internal/runner/runner.go:38` — `HooksDispatcher` interface (architectural model for the signer integration; signer is a *separate* seam, not an additional dispatcher)
- `internal/runner/runner.go:421–471` — `exec` wrapping pattern; M17-001 adds a parallel signer-invocation step
- `internal/plugin/hooks/hooks.go:36–42` — `RequestPayload` shape (the data the signer interface mirrors: method, URL, headers, body, query params)
- `internal/auth/profile.go:23` — `ProfileType "dynamic"` (explicitly NOT extended; M17 does not modify the auth_profile machinery)
- `internal/variable/dynamic.go` — registration site for M17-004 and M17-005
- `internal/variable/sensitive.go` — `SensitiveSet.AddValue` (M17-001 wires the signer-side propagation; M17-004 propagates webhook secrets the same way)

### Open Decisions for `/backlog M17`

These should be resolved by the milestone author before generating tasks; defaults are proposed.

1. **`signing:` field scope (per-request vs collection-default).** Mirror the M2-008/M2-010 auth_profile precedence: `signing:` is allowed at the collection-level (applies to every request) and at the per-request level (overrides). Per-request `signing: null` disables for that request. **Default:** support both, with per-request overriding collection. M17-001's `scope:` should state this explicitly so future verification can confirm.
2. **AWS credential sourcing.** SigV4 requires `access_key`, `secret_key`, optional `session_token`, `region`, `service`. **Default:** every field accepts the standard variable-interpolation form (`access_key: "{{aws_key}}"`); resolution flows through the existing precedence chain (CLI > env > collection > project > built-in). No new credential-resolution logic. Recommend documenting `AWS_ACCESS_KEY_ID` / `AWS_SECRET_ACCESS_KEY` / `AWS_SESSION_TOKEN` as conventional environment-variable names users can `from_command:` or simply env-load — but no special parsing.
3. **OAuth1 signature method.** OAuth1 spec defines four methods (HMAC-SHA1, HMAC-SHA256, RSA-SHA1, PLAINTEXT). **Default:** ship HMAC-SHA1 as the implicit default (most-common in the wild) with an explicit `method: HMAC-SHA256` opt-in. Skip RSA-SHA1 (requires RSA key handling — separate concern; revisit if customer demand emerges). Skip PLAINTEXT (security footgun; never the right answer).
4. **Webhook-sig naming — depend on M13-001 or not?** Three signing variants exist (Stripe / GitHub / Slack) with different argument shapes. Three options: (a) generic `$webhookSign(provider, body, secret, ...)` with variadic args — ugly; (b) three dedicated `$webhookSignStripe(body, secret)` etc. — flat namespace pollution; (c) dotted `$webhookSign.stripe(body, secret)` — clean but requires the M13-001 dotted-namespace foundation. **Default:** option (c). Add `M17-004` dependency on `M13-001`; if M13 isn't ready when M17 begins, pull M13-001 forward first (it's a 3–5 hour foundation slice). Naming hygiene is worth the small ordering coupling.
5. **Sensitive-secret propagation.** Three propagation points: SigV4 `secret_key`, OAuth1 `consumer_secret` and `token_secret`, webhook-sig `secret` argument. Each, when resolved from a sensitive variable, must be added to the run's `SensitiveSet` at signing time. **Default:** M17-001's `Signer` interface includes a `Sign(ctx, req, sensitives *SensitiveSet) error` shape that propagates as part of signing. M17-004 propagates inside the dynamic-function body, mirroring `$hmacSha256` (M12-005). State explicitly in each task's `scope:`.
6. **Tier gating.** Currently `MANUAL.md:3066` says "plugin loading is Enterprise-only," and REVIEW.md gap #3 (slated for M15) is about adding the missing tier-gate enforcement for user-supplied plugins. **Default:** built-in signers (M17-002, M17-003) and built-in dynamic functions (M17-004, M17-005) are universally available — no tier check. The Enterprise plugin gate continues to apply to user-supplied code loaded via `CURLEW_PLUGINS`; that path is unchanged. M17-001's `scope:` carries a one-line MANUAL.md clarification disambiguating the two.
7. **JWT decode error shape.** Three failure modes: (a) input lacks three dot-separated segments; (b) base64-url-decoding fails on header or claims segment; (c) decoded segment is not valid JSON. **Default:** all three return structured `CategoryInput` errors with codes mirroring M12-002's `$base64Decode` shape (`DYNFN_JWT_DECODE_BAD_FORMAT`, `DYNFN_JWT_DECODE_BAD_BASE64`, `DYNFN_JWT_DECODE_BAD_JSON`). Truncate offending input to 32 chars in error messages to avoid leaking long tokens.
8. **MANUAL.md:2761 doc-fix scope.** The current canonical-plugin-example replacement is concrete (drop `curlew-sigv4` plugin example, insert built-in `signing: {type: aws-sigv4, ...}` example). **Default:** M17-001 carries the doc-fix; downstream signer slices (M17-002, M17-003) update only their own MANUAL.md table rows. A SPECIFICATION.md v4.2 update describing the `signing:` field is recommended as a separate follow-up commit, not blocking M17.

## Phase 19 — Strategic expressiveness (M19)

M19 closes REVIEW.md gaps #24 (`docs/REVIEW.md:131`) and #25 (`docs/REVIEW.md:133`) — the two residual cells in `docs/SCRIPTING.md`'s use-case audit that no helper or signer can reach: response-content branching and cross-field aggregation. The audit identified these as roughly 5% of real test cases — bounded, but the only cases where "scripting" would otherwise be the right answer. M19 ships the two smallest declarative moves that close them.

The design rationale is `docs/SCRIPTING.md` (Rungs 1 and 2 of the ladder, §"The middle ground"). The spec is silent on both features — like M17, M19 ships ahead of a SPECIFICATION.md edition; this milestone-mapping section is the authoritative scope until the spec catches up. SPECIFICATION.md v4.4 describing `if:` and the CEL surface is recommended as a follow-up commit, not blocking M19.

**Track note — M18 independence.** M19 does not depend on M18 (compliance and post-launch). M18 covers GDPR data export, telemetry, audit-log bulk export, and SOC 2 / ISO 27001 prep — none of which the CEL surface, `if:` field, or skill expansion touch. M19 may be generated and shipped before M18 in either order; the `/backlog M19` Step 2 check should treat M18 as not-blocking-M19 (same shape as M17's independence from M16, which depended on M12 only).

Two structural facts shape the slicing:

1. **`if:` is a CEL boolean — one mental model.** SCRIPTING.md §Recommendation step 4 originally specified Go-template interpolation for `if:` on the assumption CEL was deferred (step 5). M19 collapses both steps into one milestone, so `if:` ships as a single CEL boolean expression from day one. One syntax, one evaluator, one set of activation rules across `if:`, `extract:`, and `assertions:`. Cost: M19-001 gains a hard dependency on M19-002 (the CEL foundation). Benefit: users never see two syntaxes for the same job.
2. **CEL needs three integration points, not one.** A CEL evaluator integrated only in `extract:` is half-shipped. The minimum coherent surface is foundation + `if:` + `extract:` + `assertions:` + `curlew validate` type-check. Splitting these across slices keeps each slice testable in isolation; bundling them creates a slice too large to review cleanly.

| Prefix | Capability Key      | Phase Name                          | Slices | Status  |
|--------|---------------------|-------------------------------------|--------|---------|
| `M19`  | `if_conditional`    | M19: Strategic expressiveness       | 1      | backlog |
| `M19`  | `cel_expressions`   | M19: Strategic expressiveness       | 3      | backlog |
| `M19`  | `manual_as_skill`   | M19: Strategic expressiveness       | 1      | backlog |
|        |                     | **Total**                           | **5**  |         |

Slice counts are bounded estimates. The Plan agent may refine by ±20%; significant deviation should surface as an explicit ambiguity rather than a silent rescope.

### M19 Capability Descriptions

- **if_conditional** — one slice closing REVIEW.md gap #24 (SCRIPTING.md Rung 1). New `if:` field on request items accepting a single CEL expression that must evaluate to a boolean (CEL's native `bool` type — no string truthiness coercion). Evaluated against the standard CEL activation defined by M19-002, scoped to `if:`'s surface: `previous` (last response in the dependency chain), `vars` (the full variable namespace), `env` (environment variables). `response` is intentionally not exposed in `if:` (the current request hasn't run yet). Falsy expressions skip the request: it appears in output formats with status `skipped`, does not contribute to assertion totals, does not increment retry counters, and dependents (`depends_on:`) treat a skipped parent as also-skipped (skip-on-skipped-parent — see Open Decision #4). `curlew validate` runs CEL parse + type-check on every `if:` expression, surfacing both expression errors and `not a boolean` type errors at validate time before any HTTP request fires. The `if:` gate runs before request templating so skipped requests never trigger templating side effects (`from_command:` sub-shells, vault provider fetches, faker seeding). Sensitive-value redaction propagates through CEL evaluation per the existing `internal/variable/sensitive.go` machinery. Universally available — no tier gate.

  Depends on M19-002 for the CEL foundation. Touch points: `internal/parser/` for the new field; `internal/runner/runner.go` for the pre-templating gate; `internal/output/` for `skipped` status in all six formatters; `cmd/curlew/validate.go` for parse + boolean type-check; MANUAL.md for the new field reference.

- **cel_expressions** — three slices closing REVIEW.md gap #25 (SCRIPTING.md Rung 2), narrowed from the original four-slice scope per the 2026-05-16 "no duplicate surfaces" rule. New `internal/cel/` package wrapping `github.com/google/cel-go`. CEL is chosen over Starlark: both are deterministic and I/O-free, but CEL's design centre is "predicates and projections over JSON" (the residual gap exactly), CEL has stronger type-checking at parse time, and Kubernetes admission policies have made it the LLM-familiar choice for declarative expression evaluation.

  CEL is surfaced *only* at sites where it closes a real gap with no clean existing solution:
  - `if:` (M19-001) — branching has no existing equivalent.
  - `assertions: - cel:` (M19-004) — cross-field invariants and aggregations have no existing equivalent (operator assertions cannot express "total == sum of items").

  CEL is *not* surfaced at `extract:`. JSONPath already handles single-value extraction cleanly, and the only new-functionality slice CEL would add there (aggregated-value extraction) is rare per SCRIPTING.md's audit and has no compelling demo value. Users who need an aggregated quantity can express it as a CEL assertion. Revisit only if customer pull emerges.

  - **M19-002 (foundation):** `internal/cel/` package with `Evaluator` interface, standard activation (`response`, `previous`, `vars`, `env`), Go-side type registration for `Response` (status, headers, body as `dyn`), parse-time type checking, deterministic-only function set (no time, no random, no I/O). The standard activation matches what `if:` and `assertions:` need without leaking any additional power. M19-002 is foundation-only — no `if:` or `assertions:` integration in this slice.
  - **M19-004:** CEL in `assertions:`. New assertion shape `- cel: "<expression>"` evaluating to boolean — true passes, false fails with a generated failure message that includes the expression source and the resolved sub-values for each named reference. Coexists with existing operator-based assertions; the two are mutually exclusive within a single assertion entry. **Discipline (docs and skill, not enforced at parse time):** operator assertions remain the recommended form for simple comparisons (`status_code: 200`, `body.x: { eq: y }`). `cel:` is the right answer for cross-field invariants, aggregations, and predicates that span multiple JSON paths. MANUAL.md (M19-005) and the bundled skill (M19-006) carry a "when to use what" decision table that makes this explicit.
  - **M19-005:** `curlew validate` CEL type-check pass + MANUAL.md. Validate walks every CEL site in the parsed collection (`if:` and `assertions: - cel:`) and runs CEL's parse + type-check against the standard activation, surfacing parse and type errors at validate time (before any HTTP request). MANUAL.md gains a new "Expression Language (CEL)" section under "Variable System and Precedence" with the standard activation reference, the type-check examples, and the "when to reach for CEL vs operators" decision table.

  Universally available — no tier gate. CEL's evaluation cost is bounded (parse-once / evaluate-many), so there is no resource argument for paywall placement. Sensitive-secret redaction: when CEL evaluation references a sensitive variable, the resolved value is added to the run's SensitiveSet at evaluation time so it is redacted in serialised output. Mirrors the M17-001 / M12-005 propagation pattern.

- **manual_as_skill** — one slice closing REVIEW.md gap #21 (`docs/REVIEW.md:119–121`) and discharging the "Claude skill as operator manual" direction (project memory `project_dev_ux_direction.md`, agreed 2026-04-22 item 5). The existing bundled skill at `templates/skills/claude/curlew/SKILL.md` is 134 lines — sufficient as trigger-phrase scaffolding but not as an operator manual. M19-006 expands it into a comprehensive skill payload that encodes the operator-level content of `docs/MANUAL.md` so an agent invoking the skill has the full reference available without re-reading the manual file. Scope:
  - Restructure `templates/skills/claude/curlew/` to a multi-file skill following Claude skill conventions (one root `SKILL.md` that loads, plus per-topic reference files under the same directory): variable system, output formats, assertions, retry, parallel execution, vault providers, signing (M17), CEL expressions and `if:` (this milestone's new surface), exit codes, and the failure-playbook.
  - The skill content is *derived from* `docs/MANUAL.md` but is not a verbatim copy — it is rewritten in the second-person operator voice the existing SKILL.md uses, with examples lifted from MANUAL.md's reference sections. Out of scope: any build/sync mechanism that auto-generates skill content from MANUAL.md. Doc drift between the two is acceptable for v1 and is tracked as a manual maintenance task.
  - `curlew init --skill claude` continues to deliver the full skill directory (the existing init logic at `cmd/curlew/init.go` already copies the directory tree; no change needed if the embedded-FS walker is recursive — verify).
  - The skill's failure-playbook section gains entries for each new CEL error code introduced in M19-005 (`ERR_CEL_PARSE`, `ERR_CEL_TYPE`).

  Depends on M19-005 so the CEL surface is documented in MANUAL.md before being skill-encoded. Universally available — the skill ships with every binary regardless of tier.

  Note on the smaller skill update in M19-005: M19-005's "short 'CEL escape hatch' entry" is superseded by M19-006's full expansion. M19-005 still updates MANUAL.md; M19-006 carries the skill side. State explicitly in both slice scopes to avoid double-edit collisions.

### M19 Dependency Graph (DAG)

```
(M12 done; M13 done; M17 done — preconditions per REVIEW.md:217)
│
└─ M19-002  CEL evaluator foundation (internal/cel/ package)      [track: go-cli]
            │
            ├─ M19-001  if: field (CEL boolean)                    [depends on M19-002, track: go-cli]
            │
            ├─ M19-004  CEL in assertions:                         [depends on M19-002, track: go-cli]
            │
            ├─ M19-005  curlew validate CEL type-check + MANUAL   [depends on M19-002 (parser); land after M19-001, M19-004 for full doc coverage, track: go-cli]
            │
            └─ M19-006  manual-as-skill expansion                  [depends on M19-005, track: go-cli]
```

M19-002 is a strict prerequisite for all four downstream slices. M19-001 (`if:`) and M19-004 (`assertions:`) are mutually independent and parallelisable after M19-002 lands. M19-005's parse-pass depends only on M19-002, but its documentation surface depends on M19-001 and M19-004 having shipped (so the MANUAL.md section can describe both CEL integration sites). M19-006 depends on M19-005 so the CEL surface is documented in MANUAL.md before being skill-encoded.

Numbering note: M19-001 retains its position as the `if_conditional` capability's only slice for backlog-index readability, even though the DAG places it downstream of M19-002. M19-003 is intentionally unused (the original `extract: cel:` slice was dropped per the 2026-05-16 no-duplicate-surfaces resolution). The Plan agent should not renumber or re-use M19-003.

### M19 Parallel Work Opportunities

After M19-002 lands, two slices open in parallel:

- **M19-001** — `if:` field wiring.
- **M19-004** — CEL in `assertions:`.

M19-005 lands after those two (its docs depend on them). M19-006 lands last (depends on M19-005's MANUAL.md updates). There is no convergence slice. M19 is small enough (and CLI-only) that a dedicated e2e slice is not necessary — the existing smoke test gets a new fixture that exercises both `if:` and `- cel: ...` assertions; the skill expansion in M19-006 is verified by `curlew init --skill claude` in a fresh directory and inspecting the materialised file tree.

### M19 Track Assignments

All five slices are `track: go-cli`. No backend or web work in this milestone. CEL evaluation is purely client-side; the backend never sees CEL expressions (they are not in any request/result payload). The skill expansion in M19-006 is bundled-asset work, also CLI-side.

### M19 Spec Anchors

There is no SPECIFICATION.md anchor for these features today (`grep -nE "CEL|Starlark|expression evaluator|conditional.*execution" docs/SPECIFICATION.md` returns zero hits matching this surface). M19 ships ahead of a v4.4 spec edition; the milestone-mapping section is the authoritative scope until the spec catches up. Supporting references:

- `docs/SCRIPTING.md` §"The middle ground" Rung 1 (lines 156–169) — `if:` field design rationale.
- `docs/SCRIPTING.md` §"The middle ground" Rung 2 (lines 171–192) — CEL vs Starlark design rationale and recommended integration shape.
- `docs/SCRIPTING.md` §"The genuine residual" (lines 81–106) — frequency and shape of the two unsolved cells these features close.
- `docs/SCRIPTING.md` §Recommendation steps 4 and 5 (lines 253–254) — the decision to ship `if:` first with existing interpolation and to defer CEL until M12 + M17 land (now satisfied).
- `docs/REVIEW.md:119–121` — gap #21 framing (manual-as-skill).
- `docs/REVIEW.md:131` — gap #24 framing.
- `docs/REVIEW.md:133` — gap #25 framing; CEL named as "second-step move."
- `docs/REVIEW.md:197` — original M19 milestone sketch (notes T5 items #21 and #22 "sit here too if ever pursued"; gap #21 now in scope per 2026-05-16 resolution).
- `docs/REVIEW.md:217` — readiness criteria ("only after M12 has shipped and at least M17's first plugins are in production"); both satisfied.
- Project memory `project_dev_ux_direction.md` (agreed 2026-04-22 item 5) — "Default Claude skill at `templates/skills/claude/curlew/SKILL.md` … encodes trigger phrases, artifact paths, failure playbook per exit code, narration rules"; M19-006 discharges the unfulfilled portion of this direction.

Implementation surface:

- `internal/parser/` — `if:` field on request items; `- cel: <expr>` assertion shape. (`extract:` is unchanged; CEL is not surfaced there per the 2026-05-16 no-duplicate-surfaces resolution.)
- `internal/runner/runner.go` — pre-templating `if:` gate; skipped-request status threading through the exec wrap.
- `internal/output/` — six formatters (terminal, JSON, TAP, JUnit, HTML, Markdown) gain `skipped` rendering.
- `internal/cel/` — new package wrapping `github.com/google/cel-go`; `Evaluator` interface; standard activation.
- `internal/variable/sensitive.go` — `SensitiveSet.AddValue` extension points for CEL-resolved sensitive values.
- `internal/variable/` — existing Go-template interpolation reused by `if:`; no changes.
- `cmd/curlew/validate.go` — CEL parse + type-check pass over the parsed collection.
- `docs/MANUAL.md` — new "Expression Language (CEL)" section; `if:` field reference; JSONPath / CEL migration table.
- `templates/skills/claude/curlew/` — directory expanded from single 134-line `SKILL.md` to a multi-file skill payload per Claude skill conventions (M19-006). The existing single-file form is the M19-005 update site; the multi-file expansion is the M19-006 site.
- `cmd/curlew/init.go` — `--skill claude` delivery path; verify the embedded-FS walker handles the new directory tree (M19-006).

### M19 Open Decisions for `/backlog M19`

These should be resolved by the milestone author before generating tasks; defaults are proposed. The five SCRIPTING.md §"Open questions for the team" items (lines 260–266) collapse into points 1, 2, 6, and 7 below; the remaining points are scope decisions surfaced by the slice breakdown above.

1. **`if:` syntax — interpolation vs CEL.** SCRIPTING.md §Recommendation step 4 originally named "existing interpolation syntax" on the assumption CEL was deferred (step 5). M19 collapses both steps into one milestone. **Resolved (2026-05-16):** `if:` ships as a CEL boolean from day one. One syntax across `if:`, `extract:`, `assertions:`; users never see two ways to do the same job. Cost: M19-001 has a hard dependency on M19-002, and the slice can only ship after the CEL foundation lands.

2. **CEL vs Starlark.** SCRIPTING.md leans toward CEL ("CEL especially is a good match"); both are deterministic and I/O-free. **Default:** CEL via `github.com/google/cel-go`. Three structural reasons: (a) CEL's design centre is "predicates and projections over JSON," exactly the residual gap; (b) CEL's compile-time type checker is stronger than Starlark's runtime errors, which lets `curlew validate` catch more before any HTTP request fires; (c) Kubernetes admission policies have made CEL the LLM-familiar choice for declarative expression evaluation. Starlark's only edge is "more general" — and "more general" is exactly what M19 is trying not to ship.

3. **`if:` falsy-mapping table.** ~~Truth set for a resolved string.~~ **Moot under Decision #1's resolution:** `if:` accepts a CEL boolean expression, not a string. CEL's native `bool` type carries no truthiness ambiguity. `curlew validate` rejects any `if:` expression whose CEL type-check resolves to a non-`bool` type (`ERR_CEL_TYPE` with field path).

4. **`if:` interaction with `depends_on:`.** When request B `depends_on:` request A and A is skipped via `if:`, three options: (a) B also skips ("skip-on-skipped-parent"); (b) B runs as if A had passed (predicate skip ≠ failure); (c) B fails. **Default:** (a) skip-on-skipped-parent. A skipped parent means the user's branching predicate said "don't run this branch"; running the dependent makes no sense. Document explicitly; M19-001's behaviour tests cover this case.

5. **`if:` evaluation order vs templating.** The expression references variables; the request body also references variables. Three orderings: (a) evaluate `if:` first against the pre-template variable scope, skip templating if false; (b) template the request, then evaluate `if:` against the templated request; (c) template both in parallel. **Default:** (a). Skipped requests should never trigger templating side effects (`from_command:` sub-shells, vault provider fetches, faker seeding). M19-001's `scope:` states this explicitly; the runner must wire the `if:` gate before the templating step.

6. **CEL standard activation surface.** Four variables to expose: `response` (current request's response — only available in `assertions:`), `previous` (parent response in the `depends_on:` chain — available in `if:` and `assertions:`), `vars` (the full variable namespace), `env` (environment variables). **Resolved (2026-05-16):** `if:` sees `previous`, `vars`, `env`. `assertions:` sees all four. CEL never sees `request` (the in-flight request body) — needing that suggests the user wants pre-request scripting, which M19 explicitly does not ship; that surface is owned by M17 signers. `previous` semantics: the parent in the `depends_on:` chain (matches how the dependency edge reads), not "the most recently executed request in the run" (which would be order-dependent and surprising under parallel execution).

7. **CEL function set.** `cel-go` ships with a standard library (math, string, list ops, time-as-data). **Resolved (2026-05-16):** enable the standard library minus the time-of-day functions (`now()`, `timestamp()` with no argument). Time-of-day breaks determinism; faker seeding stops working. Curlew already has `$timestamp` and `$dateAdd` dynamic functions for the legitimate time use cases; CEL does not duplicate them.

8. **`extract: { cel: ... }` and JSONPath coexistence.** ~~Original proposal: exactly one of `jsonpath:` and `cel:` per entry.~~ **Moot under the 2026-05-16 no-duplicate-surfaces resolution:** CEL is not surfaced at `extract:` at all. `jsonpath:` remains the only extraction language. Aggregated-value extraction (the only new-functionality slice CEL would have added) is rare per SCRIPTING.md and can be expressed as a CEL assertion. Revisit only if customer pull emerges.

9. **`curlew validate` CEL error shape.** CEL parse errors and type errors are distinct (`ERR_CEL_PARSE`, `ERR_CEL_TYPE`). **Resolved (2026-05-16):** structured errors in the existing validate error catalogue with the field path (`if` or `assertions[3].cel`) and the CEL library's error message. `ERR_CEL_TYPE` also fires when an `if:` expression resolves to a non-`bool` type. Truncate CEL source in error messages to 200 chars to avoid validate output explosions on long expressions.

10. **Tier gating.** `if:` and CEL are both expressiveness fixes for the no-scripting story (REVIEW.md frames them as ladder rungs *replacing* scripting, not gated additions). **Resolved (2026-05-16):** both universally available across tiers — no tier check. Consistent with M17's "built-in helpers are universally available" precedent. Revenue-counter-argument (gate CEL to Solo+) explicitly rejected: M19 is a closer for the no-scripting promise, not a paid power-user feature.

11. **MANUAL.md / SPECIFICATION.md split.** SPECIFICATION.md currently has no `if:` or CEL entry. **Resolved (2026-05-16):** M19-005 ships only the MANUAL.md surface (the user-facing reference is the manual, not the spec). A SPECIFICATION.md v4.4 update describing `if:` and the CEL surface is recommended as a follow-up commit, not blocking M19. Mirrors the M17 / SPECIFICATION.md v4.2 pattern.

12. **Manual-as-skill (REVIEW.md gap #21) and MCP server (gap #22) in M19?** REVIEW.md:197 names these as "remaining T5 items that sit here too if ever pursued." **Resolved (2026-05-16):** manual-as-skill (gap #21) is in scope as M19-006. Rationale: project memory `project_dev_ux_direction.md` (agreed 2026-04-22 item 5) names the Claude skill as operator manual a stated direction; the existing 134-line `templates/skills/claude/curlew/SKILL.md` is sufficient scaffolding but not an operator manual; the unfulfilled portion belongs in M19. MCP server (gap #22) remains out of scope — no spec requirement and no customer pull; revisit only if customer demand emerges.

## Phase 18 — Compliance and launch readiness (M18)

M18 is the only remaining pre-launch milestone. It closes REVIEW.md gaps 17 (audit-log bulk export), 18 (GDPR per-user export + deletion), 19 (telemetry / conversion tracking), 20 (SOC 2 + ISO 27001 evidence), plus the two encryption-at-rest deferrals carried forward from v4.3 (`team_vaults.template_jsonb` per v3-10; `schedules.env_vars` sensitive values per spec `:11057`). Per project memory `project_milestone_release_model.md`, no public launch happens until M18 ships.

The design pass is captured in two artefacts:

1. `docs/M18_INVESTIGATION.md` — the complete pre-work investigation (skeleton 2026-05-17, verification pass + decisions same day) including the residual-scope correction (audit-log CSV scaffolding already exists; `IKmsClient` envelope pattern already exists; telemetry spec at `:6091–6809` is extensive, not loose), the fifteen cross-cutting decisions, and the explicit out-of-scope list (faker locale → own follow-up; GHES → future Enterprise milestone).
2. `docs/SPECIFICATION.md` v4.4 (header + Changelog entry; new top-level sections "Audit Log Export & Retention", "GDPR Data Subject Rights (Full)", "Telemetry Phase 3 Implementation Pipeline", "Encryption-at-Rest Extension (v2)", "Compliance Artifact Inventory"; new tables in the schema appendix). The fifteen v4.4 cross-cutting decisions (`v4-1` through `v4-15`) are the authoritative scope reference.

Everything below is grounded in those two artefacts; do NOT reopen settled decisions during `/backlog M18`.

M18 spans `backend`, `go-cli`, `web`, and a new `compliance` track for the non-code policy + vendor-inventory work.

| Prefix | Capability Key             | Phase Name                       | Slices | Status  |
|--------|----------------------------|----------------------------------|--------|---------|
| `M18`  | `audit_log_export`         | M18: Compliance & launch         | 1      | backlog |
| `M18`  | `audit_log_retention_rbac` | M18: Compliance & launch         | 1      | backlog |
| `M18`  | `gdpr_export`              | M18: Compliance & launch         | 2      | backlog |
| `M18`  | `gdpr_deletion`            | M18: Compliance & launch         | 2      | backlog |
| `M18`  | `telemetry_phase3_ingest`  | M18: Compliance & launch         | 2      | backlog |
| `M18`  | `encryption_at_rest_v2`    | M18: Compliance & launch         | 1      | backlog |
| `M18`  | `compliance_artifacts`     | M18: Compliance & launch         | 2      | backlog |
| `M18`  | `m18_e2e`                  | M18: Compliance & launch         | 1      | backlog |
|        |                            | **Total**                        | **12** |         |

Slice counts are bounded estimates per the investigation (11–13 range). The Plan agent may refine by ±10%; significant deviation should surface as an explicit ambiguity rather than a silent rescope.

### M18 Capability Descriptions

- **audit_log_export** — single slice closing REVIEW.md gap 17. Lifts the existing `?format=csv` handler at [src/ApiTool.Backend/Audit/AuditLogEndpoints.cs:76-82](src/ApiTool.Backend/Audit/AuditLogEndpoints.cs) into a bulk-export shape: removes the `MaxLimit=200` cap when `format ∈ {csv, jsonl}`, switches to `Transfer-Encoding: chunked` streaming, adds JSONL alongside CSV, applies `AuditLogExportTierGate.EnsureEnterpriseAsync` (using the M16 `ITierGate` foundation). Adds Enterprise-only `audit_log.export` permission check at endpoint entry per v4-3. Streaming covers ~1M rows comfortably; async-job model deferred per v4-1. Tests cover row-cap behaviour, format negotiation, tier-gate response shape, and chunked-output integrity.

- **audit_log_retention_rbac** — single slice per v4-2 and v4-3. (a) `audit_log.view` and `audit_log.export` permissions added to [src/ApiTool.Backend/Rbac/Permissions.cs](src/ApiTool.Backend/Rbac/Permissions.cs); hardcoded Owner/Admin gate at [AuditLogQueryService.cs:24](src/ApiTool.Backend/Audit/AuditLogQueryService.cs) lifted to permission check; built-in roles updated to grant both to Owner+Admin; new "Security Auditor" custom-role template carries `audit_log.view` only (separation-of-duties evidence for SOC 2). (b) `organizations.audit_log_retention_days` column migration (default 365, capped at 365 for non-Enterprise per v4-2); `AuditLogCleanupHost : BackgroundService` runs daily at 02:00 UTC, hard-deletes rows older than `now() - retention_days` per org. Mirrors the `GithubWebhookCleanupHost` pattern.

- **gdpr_export** — two slices closing the export half of REVIEW.md gap 18 per v4-4. (1) **Data inventory pass.** Concrete decision matrix for each of the 13 user-attributable tables enumerated in the investigation (RefreshToken, OrganizationMember, EmailVerificationToken, PasswordResetToken, NotificationRule, CustomRole, Schedule, CoordinatorJob, TeamVault, OrganizationAuditLogEntry, GithubInstallation, GitLabInstallation, plus User itself): for each, document whether it is in-export and in-deletion-hard or in-deletion-anonymise. Output: `docs/security/data-inventory.md` + EF Core entity attributes (`[GdprIncluded]`, `[GdprAnonymise]`) wired into the assembly scanner that builds the export bundle. (2) **Export endpoint.** `POST /api/v1/users/me/export-requests` (queues the build) + `GET /api/v1/users/me/export-requests/{id}` (status + signed URL when ready). Signed URL backed by `IObjectStore` (S3-compatible self-hosted, GCS for SaaS) with 24h expiry. Rate-limited 1 request per user per 24h. New user-level account-settings page at `web/src/routes/(app)/account/data/` (the existing `/settings/` is org-scoped — new user-level surface required) with "Request data export" button + status list.

- **gdpr_deletion** — two slices closing the deletion half of REVIEW.md gap 18 per v4-5, v4-6, v4-7. (1) **State machine + finalizer.** `User.PendingDeletionAt` + `User.AnonymisedAt` columns + migration; `POST /api/v1/users/me/deletion-requests` requires re-authentication (password re-entry token issued in last 5 minutes, single-use); response is the cancellation window. `UserDeletionFinalizerHost : BackgroundService` runs daily at 03:00 UTC, finalizes deletions past their 30-day cooldown. Email notifications on initiation AND on completion via existing SendGrid pipeline (two new templates: `account_deletion_initiated`, `account_deletion_completed`). (2) **Anonymisation + last-admin protection + UX.** `IUserAnonymiser` service: replaces `ActorId` with NULL, `ActorEmail` with `deleted-user-{first8(sha256(user_id+org_id))}` across all audit-log entries; hard-deletes everything in the in-deletion-hard set from the inventory; emits `user.anonymised` audit event preserving the audit-of-audit trail. Last-admin protection extends the existing `MembersService.cs:183-184` `OwnerCannotLeave` check to the user-delete trigger site; response body lists every blocking org. Web UX adds "Delete account" panel under `web/src/routes/(app)/account/data/` with re-auth prompt, blocking-orgs preview, and cancellation page reachable for 30 days.

- **telemetry_phase3_ingest** — two slices implementing Phase 3 of the spec's three-phase telemetry rollout (lines 6308–6809) per v4-8, v4-9, v4-10, v4-11. (1) **Backend ingest.** `telemetry_events` table migration (per-event row: `install_id`, `event_type`, `event_payload_jsonb`, `received_at`, `idempotency_key`); `POST /api/v1/telemetry/events` endpoint, anonymous (no Bearer), idempotency-key dedup, body size cap 64KB per event, rate-limited per `install_id` via existing `Internal/RateLimit` (60/min); `telemetry_daily_aggregates` rollup table built by `TelemetryAggregatorHost` daily at 04:00 UTC; raw events purged after 90 days per spec `:6604`. (2) **CLI emitter.** New `internal/telemetry/` package per v4-11 — dedicated HTTP client (no Bearer, idempotency key per event), `curlew telemetry {enable, disable, status, reset-id, export, delete-request}` subcommand per v4-8. First-run UUID generated at `~/.config/curlew/install_id` (mode 0600); persistent across runs; regeneratable. Opt-in only (consent under GDPR Art 6(1)(a) per v4-9). Events fire from the runner's existing event-emission points but only if `~/.config/curlew/telemetry.json` declares `enabled: true`. Spec update: v4-8 reverses the prior per-session-UUID model; the SPECIFICATION.md v4.4 entry for telemetry must reflect the persistent install-ID design.

- **encryption_at_rest_v2** — single slice per v4-12 closing the two open encryption-at-rest deferrals (`team_vaults.template_jsonb` from v3-10 and `schedules.env_vars` sensitive values from spec `:11057`). Reuses the existing `IKmsClient` envelope pattern at [src/ApiTool.Backend/Licensing/Keys/IKmsClient.cs:8-43](src/ApiTool.Backend/Licensing/Keys/IKmsClient.cs); new `TeamVaultKeyProvider` and `ScheduleEnvKeyProvider` clone `GoogleKmsGitLabKeyProvider`'s per-row DEK + KMS-wrapped KEK pattern. Single migration adds `team_vaults.template_jsonb_ciphertext` + `team_vaults.dek_ciphertext` columns (and the matching `schedules.env_vars_ciphertext` + `schedules.env_vars_dek_ciphertext`), backfills from existing plaintext, drops plaintext columns after backfill verification. Manifest-validator from v3-10 stays — encryption is added in addition. For self-hosted without KMS, `FileTeamVaultKeyProvider` and `FileScheduleEnvKeyProvider` mirror the GitLab file-provider fallback. v4-13's signing-key CI lint extension lands here too (`scripts/ci-local.sh` checks `signing_keys.kms_key_id IS NOT NULL` against the production DB for SaaS builds).

- **compliance_artifacts** — two slices per v4-14. (1) **Policy + docs production.** New `docs/COMPLIANCE.md` umbrella + sub-documents in `docs/security/`: information-security policy template (Vanta/Drata-compatible), access-review policy (quarterly cadence per v4-15-adjacent), incident-response runbook (severity taxonomy, on-call rota expectation, comms templates), data-classification matrix (Public/Internal/Confidential/Restricted per-table), `docs/security/data-inventory.md` from the gdpr_export slice referenced here. (2) **Vendor inventory + DFD + pen-test orchestration.** `docs/security/vendor-inventory.md` listing every external dependency (Stripe, SendGrid, Google KMS, AWS S3/SES if used, GitHub Apps, GitLab) with: data shared, retention, breach-notification SLA, SOC 2 status of the vendor. Two data-flow diagrams (`docs/security/data-flow-customer.md`, `docs/security/data-flow-internal.md`) covering customer-PII paths and internal-secrets paths. First vendor pen-test engagement orchestrated (scope statement, NDA, kickoff); remediation log committed back to repo as `docs/security/pentest-YYYY-Q.md`.

- **m18_e2e** — single convergence slice that proves the full compliance workflow end-to-end: user registers → consents to Phase 3 telemetry → install-ID UUID created and persisted → events flow to backend → user exports their data (full bundle including telemetry events scoped to their install_id) → user requests account deletion → re-auth required and provided → 30-day cooldown countdown begins → cancellation tested → deletion finalizer runs → audit-log rows for that user are anonymised (`ActorEmail` shows the deterministic token) → `user.anonymised` audit event lands → Enterprise admin exports the org's audit log as JSONL via streaming chunked transfer → encrypted columns in `team_vaults` and `schedules` round-trip cleanly under the new envelope. Mirrors M14-021's and M16-021's role.

### M18 Dependency Graph (DAG)

```
(M14 done; M15 done; M16 done; M17 done; M19 done — all pre-M18 milestones complete)
│
├─ audit-log refactors (independent foundation; no upstream deps within M18) ─────
│  M18-001  audit_log_export                                         [track: backend]
│  M18-002  audit_log_retention_rbac                                 [track: backend]
│
├─ GDPR cluster (gdpr_export → gdpr_deletion; deletion consumes inventory) ───────
│  M18-003  gdpr_export — data inventory + entity attributes        [track: backend+docs]
│  M18-004  gdpr_export — endpoint + signed-URL + web UX             [track: split]
│  M18-005  gdpr_deletion — state machine + finalizer + emails      [track: backend]
│  M18-006  gdpr_deletion — anonymiser + last-admin + web UX         [track: split]
│
├─ telemetry cluster (ingest before emitter; spec update is part of emitter slice) ─
│  M18-007  telemetry_phase3_ingest — backend + table + rollup      [track: backend]
│  M18-008  telemetry_phase3_ingest — CLI emitter + subcommand      [track: go-cli]
│
├─ encryption + compliance (parallel-OK; compliance docs reference encryption work) ─
│  M18-009  encryption_at_rest_v2                                    [track: backend]
│  M18-010  compliance_artifacts — policies + data-inventory        [track: compliance]
│  M18-011  compliance_artifacts — vendor inventory + DFD + pen-test [track: compliance]
│
└─ convergence ─────────────────────────────────────────────────────────────────
   M18-012  m18_e2e                                                  [track: e2e]
```

### M18 Parallel Work Opportunities

- M18-001 / M18-002 / M18-003 / M18-007 / M18-009 / M18-010 are mutually independent and can start in parallel as soon as `/backlog M18` lands.
- M18-004 depends only on M18-003 (data inventory must define the bundle shape).
- M18-005 / M18-006 (gdpr_deletion) can start in parallel with M18-003/M18-004 (gdpr_export) — different entities, different endpoints.
- M18-008 depends on M18-007 (emitter needs the endpoint contract).
- M18-011 builds on M18-010 (policies frame the inventory) and benefits from M18-009 landed (encryption work informs the data-flow diagrams).
- M18-012 (convergence) depends on M18-001 through M18-011 all done.

### M18 Track Assignments

- `backend` track: M18-001, M18-002, M18-003 (partial), M18-005, M18-007, M18-009 (~6 slices).
- `go-cli` track: M18-008 (1 slice).
- `web` track: M18-004 (partial), M18-006 (partial) (~2 partial slices).
- `compliance` track (new): M18-010, M18-011 (~2 slices).
- `e2e` track: M18-012 (1 slice).
- Split slices (M18-004, M18-006) carry both backend + web work; the Plan agent should split if scope warrants.

### M18 Spec Anchors

- `docs/SPECIFICATION.md` v4.4 Changelog + Decisions Resolved (v4.4, all M18 themes) table — fifteen `v4-N` decisions.
- New top-level sections: "Audit Log Export & Retention", "GDPR Data Subject Rights (Full)", "Telemetry Phase 3 Implementation Pipeline", "Encryption-at-Rest Extension (v2)", "Compliance Artifact Inventory".
- New schema appendix entries: `telemetry_events`, `telemetry_daily_aggregates`, `team_vaults.template_jsonb_ciphertext` columns, `schedules.env_vars_ciphertext` columns, `organizations.audit_log_retention_days` column, `users.pending_deletion_at` + `users.anonymised_at` columns.

### M18 Open Decisions for `/backlog M18`

All fifteen cross-cutting questions are closed by `docs/M18_INVESTIGATION.md` "Decisions Resolved (v4.4, all M18 themes)" — do NOT reopen during `/backlog M18`. The Plan agent should treat v4-1 through v4-15 as commitments. Any new ambiguity surfaced during decomposition should be flagged explicitly as a `v4.4.1` supplement candidate, not resolved silently.

## Phase 20 — Faker locale completion (M20)

M20 closes the documented-but-not-built locale drift deliberately deferred from M13 (see M13 Open Decision 1) and re-confirmed as carry-along triage during the M18 design pass (`docs/SPECIFICATION.md:19`, "filed as a separate small follow-up milestone (`faker_locale_completion`, provisional M20)"). The spec is already complete: `SPECIFICATION.md:961–1026` defines a 15-locale table, the `--locale` CLI flag, a config precedence chain, and a fallback chain. The implementation is en-US only. Verified 2026-06-12: **no `--locale` parsing exists anywhere in the CLI** — the flag is silently ignored by the arg parser, so REVIEW.md §1c's "recognised-but-warns-and-ignores" overstates the current state; M20 builds the flag from zero. Not pre-launch — sequenced after the M14–M19 launch set per the M18 triage.

| Prefix | Capability Key              | Phase Name                    | Slices | Status        |
|--------|-----------------------------|-------------------------------|--------|---------------|
| `M20`  | `faker_locale_completion`   | M20: Faker locale completion  | 4      | not generated |
|        |                             | **Total**                     | **4**  |               |

### M20 Capability Description

- **faker_locale_completion** — 14 additional locale data pools (15 total including the en-US default) across the seven faker categories from M13; `--locale` CLI flag; precedence chain Default (en-US) < Project < Environment < Collection < CLI flag (`SPECIFICATION.md:1006`); fallback chain `en-GB` → `en` → `en-US` with verbose-mode warnings (`:989`); `ERR_LOCALE_UNKNOWN` for unresolvable locales; seed/locale independence — the same `--seed` selects the same pool position in every locale (`:1023–1026`); lazy per-locale data pools, ~100KB per locale (`:1110`); MANUAL.md locale reference table.

### M20 Dependency Graph (DAG)

```
(all M13 done — satisfied)
│
└─ M20-001  --locale flag + precedence chain + fallback chain + ERR_LOCALE_UNKNOWN,
            shipped end-to-end with one non-default locale (de-DE)
            │
            ├─ M20-002  Latin-script European locale pools (en-GB, fr-FR, es-ES,
            │           it-IT, pt-BR, nl-NL, pl-PL, sv-SE, tr-TR)
            ├─ M20-003  CJK + Cyrillic locale pools (ja-JP, zh-CN, ko-KR, ru-RU)
            └─ M20-004  MANUAL.md locale reference + cross-locale seed
                        reproducibility matrix
```

M20-001 is the strict prerequisite (no locale pool is reachable without the flag and resolution chain). M20-002 and M20-003 are mutually independent. The suggested slice shape is a starting point — the Plan agent decomposing this milestone may re-batch the pools if a slice violates vertical-slice sizing.

### M20 Track Assignments

All tasks are `track: go-cli`. No backend or web work in this milestone.

### M20 Spec Anchors (verified 2026-06-12)

- `docs/SPECIFICATION.md:858` — faker design principle ("Support localization via `--locale` for region-specific data")
- `docs/SPECIFICATION.md:961–1026` — Localization Support: CLI examples (`:964–967`), 15-locale table (`:971–988`), fallback chain (`:989`), config placement for project/environment/collection (`:991–1006`), precedence chain (`:1006`), seed/locale independence (`:1023–1026`)
- `docs/SPECIFICATION.md:1110` — lazy locale data pools, ~100KB memory per locale
- `internal/variable/dynamic.go:647`, `:713`, `:847`, `:1565`, `:1634`, `:1678` — "en-US only; locale deferred" markers at the M13 faker registration sites where locale-aware data selection must hook in
- `docs/REVIEW.md:75`, `:156` — §1c closeout recording the M13 deferral

### M20 Open Decisions for `/backlog M20`

These ambiguities should be resolved by the milestone author before generating tasks; defaults are proposed.

1. **Locale count: 15, not 16.** `management/backlog.yaml`'s M20 entry says "16 locales" but lists 15 names; the spec table (`:973–988`) has exactly 15 rows including the en-US default. **Default:** 15 is authoritative; correct the backlog entry count during `/backlog M20`.
2. **Flag baseline is zero, not warn-stub.** REVIEW.md §1c claims `--locale` is "recognised-but-warns-and-ignores"; in fact no parsing exists and the flag is silently ignored (verified against the built binary 2026-06-12). **Default:** M20-001 implements full parsing; correct the REVIEW.md §1c wording in passing.
3. **Pool batching: by script family, not by faker category.** Each locale slice should produce a coherent observable (`curlew run --locale X` shows localized output across all seven categories) rather than category slices that localize fragments of every locale at once. **Default:** the script-family batching in the DAG above.
4. **Data storage.** M13 ships en-US data as in-source Go tables; the spec's lazy-load note (`:1110`) implies per-locale pools are not all resident. **Default:** per-locale in-source tables initialised behind `sync.Once` — satisfies the lazy contract without new `go:embed` plumbing. Revisit only if binary-size measurement says otherwise.
5. **Telemetry `locale` field is out of scope.** `SPECIFICATION.md:6406` lists a `locale` data point in the telemetry execution payload ("System locale for i18n prioritization") — that is the M18 telemetry subsystem reporting the *system* locale, not the faker `--locale` knob. **Default:** no M20 work touches telemetry; do not conflate the two when wiring precedence.

## Task Numbering

- Three-digit zero-padded: `001`, `002`, ..., `999`
- Within a milestone, tasks are numbered in rough execution order
- Future milestones continue the same scheme (M4 starts at M4-001)
- When a milestone uses tracks (see M4), IDs are still unique per milestone — tracks do not get their own numbering.
