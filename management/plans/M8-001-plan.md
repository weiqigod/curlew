# M8-001 — Implementation Plan

**Title:** Publish collection JSON Schema + document VS Code yaml.schemas snippet
**Phase:** M8 — Developer & Agent Exploration Experience
**Driver doc:** `IMPROVEMENT.md` §5 W1 + §8.6 (versioned path decision)
**Analysis doc:** `~/.claude/plans/lets-analyze-w1-deeply-synthetic-sutherland.md`

## Goal

Move the collection JSON Schema from `internal/schema/collection.json` to a stable, versioned in-repo path at `schemas/collection-v1.json`, and document a `.vscode/settings.json` snippet so developers editing `collections/*.yaml` get autocomplete, hover docs, and inline validation via `redhat.vscode-yaml`. Preserve the `schema.CollectionSchema` public symbol.

The §5 "Risks" audit found schema gaps (`auth`, `retry` at 3 levels, `data_driven`, section object form, variables object form, loosely-typed `status`). Those are tracked separately as M8-002 and **out of scope** for this task.

## Design decisions (locked)

| Decision | Choice |
|---|---|
| Published path | `schemas/collection-v1.json` (versioned per IMPROVEMENT.md §8.6) |
| Embed strategy | New repo-root package `schemas/` owns the `//go:embed`; `internal/schema/schema.go` aliases `schemas.CollectionV1`. `//go:embed` forbids `..`, so a sibling package is the only clean re-point. |
| `$id` | `https://raw.githubusercontent.com/peterlindqvist/curlew/main/schemas/collection-v1.json` |
| `title` | `Curlew Collection v1` (was: `Curlew Collection`) |
| `.vscode/settings.json` | Documented in MANUAL.md, NOT checked in (`.vscode/` already in `.gitignore:28`) |
| Docs placement | New `docs/MANUAL.md` §1.5 "Editor setup (VS Code)" between §1.4 and Part 2 |

## File-by-file changes

| Path | Action |
|---|---|
| `schemas/collection-v1.json` | **create** — content = current `internal/schema/collection.json` + `$id` + new `title` |
| `schemas/schemas.go` | **create** — `package schemas`; `//go:embed collection-v1.json`; `var CollectionV1 []byte` |
| `internal/schema/collection.json` | **delete** |
| `internal/schema/schema.go` | **modify** — `var CollectionSchema = schemas.CollectionV1` (import `github.com/weiqigod/curlew/schemas`) |
| `internal/schema/validate_test.go` | **create** — DoD test + negative control + drift guard |
| `internal/schema/schema_test.go` | **modify (additive)** — add `TestSchema_file_exists_at_published_path` |
| `docs/MANUAL.md` | **modify** — insert §1.5 |
| `CHANGELOG.md` | **modify** — one bullet under `[Unreleased]` → `### Added` |

## Existing utilities to reuse

- `github.com/santhosh-tekuri/jsonschema/v6` v6.0.2 (`go.mod:9`) — already a dep.
- Prior-art pattern for "walk `runtime.Caller` to repo root and compile a versioned schema": `internal/output/events/schema_test.go:24-60` (`schemaPath`, `compileEventSchema`). Mirror shape; do not extract shared helper (one other use-site = premature).
- `internal/scaffold.Init(scaffold.Options{Dir: tmpDir})` — call directly; no binary shell-out.
- `cmd/curlew/main.go:2578` (`schemaCmdOut`) — unchanged caller of `schema.CollectionSchema`.

## Test plan

`internal/schema/validate_test.go` (new):

1. `TestSchema_validates_scaffolded_sample` — DoD test. Temp dir → `scaffold.Init` → read `collections/sample.yaml` → YAML → `any` → compile from `schema.CollectionSchema` → `schema.Validate(doc)` → assert nil error.
2. `TestSchema_rejects_sample_missing_required_name` — negative control. Delete top-level `name:` from the YAML tree before validating. Assert validation error names the missing required field.
3. `TestSchema_published_path_matches_embed` — drift guard. Walk `runtime.Caller` to repo root, read `schemas/collection-v1.json`, assert `bytes.Equal(fileBytes, schema.CollectionSchema)`.

`internal/schema/schema_test.go` (additive):

4. `TestSchema_file_exists_at_published_path` — cheap reachability check. `os.Stat`, non-empty.

## TDD sequence

Branch: `feature/M8-001-publish-collection-schema`.

- **Commit 0** — `chore(M8-001): create task files and management plan`
  - `management/tasks/M8-001.yaml` (status `in_progress`)
  - `management/tasks/M8-002.yaml` (status `backlog`, dep `[M8-001]`)
  - `management/backlog.yaml` — add `editor_integration` capability block
  - `management/plans/M8-001-plan.md` — this file
- **Commit 1 — RED** — `test(schema): add failing tests for published schema path`
  - `internal/schema/validate_test.go`
  - `internal/schema/schema_test.go` (additive)
  - `go test ./internal/schema/...` fails: path/file tests fail, drift guard fails.
- **Commit 2 — GREEN** — `feat(schema): publish collection JSON Schema at schemas/collection-v1.json`
  - Create `schemas/collection-v1.json`, `schemas/schemas.go`
  - Modify `internal/schema/schema.go`
  - Delete `internal/schema/collection.json`
  - `go test ./...` passes; `go build ./cmd/curlew` clean.
- **Commit 3 — Docs** — `docs(M8-001): add §1.5 Editor setup and CHANGELOG entry`
  - `docs/MANUAL.md` §1.5
  - `CHANGELOG.md` `[Unreleased]` → Added bullet

No REFACTOR commit. Alias file is slim; further consolidation (caller migration) = separate task.

## Verification

1. `go test ./...` — all green.
2. `go test -cover ./internal/schema/...` — ≥80%.
3. `go build ./cmd/curlew` — clean.
4. `golangci-lint run` — clean.
5. `./smoke/run.sh` — passes.
6. `./scripts/ci-local.sh` — passes (authoritative gate).
7. `./curlew schema | jq -r .title` → `Curlew Collection v1`.
8. `./curlew schema | jq -r '.["$id"]'` → raw.githubusercontent URL.
9. Manual VS Code smoke: autocomplete + hover + inline validation against `collections/sample.yaml`.

## Definition of Done

Full checklist is in `management/tasks/M8-001.yaml`.

## Out of scope (explicit deferrals)

Deferred to M8-002: `auth`, `retry` (3 levels), `data_driven`, section object form, variables object form, `status` oneOf.

Deferred indefinitely (per IMPROVEMENT.md §7): auto-writing `.vscode/settings.json`, VS Code extension / LSP, `request.method` enum restriction, schemastore.org submission (gated on tagged release per §8.6).
