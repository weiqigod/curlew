# Improvement Report: M2-010

**Task:** Per-request auth profile reference
**Date:** 2026-03-29
**Review:** management/reviews/M2-010-review.md

## Resolved Findings

| # | Severity | Finding | Fix Applied | Verified |
|---|----------|---------|------------|----------|
| 1 | High | Smoke test not updated. Plan listed `smoke/run.sh` as a file to modify; Definition of Done requires "Smoke test updated (if new capability)". | Added scenario to `smoke/run.sh` that creates a collection with `auth: admin_token` on a request (no auth_profiles configured) and verifies the output contains "auth profile" — confirming the `auth:` field is parsed and the runner produces a clear error when no profiles are available. | ✓ tests pass, lint pass |
| 2 | Medium | CHANGELOG not updated. Quality gates in CLAUDE.md require `CHANGELOG.md updated` for user-visible features. | Added entry under `[Unreleased] → Added` describing the `auth: <profile_name>` field, Bearer token injection, explicit header precedence, and error messaging. | ✓ tests pass, lint pass |

## Out of Scope (Deferred)

No findings deferred. All findings resolved.

## Quality Gate

| Check | Result |
|-------|--------|
| `go build ./cmd/apitest` | PASS |
| `go test ./...` | PASS |
| `golangci-lint run` | PASS |
| Coverage (`internal/parser`) | 91.0% |
| Coverage (`internal/runner`) | 91.3% |
| Coverage (total) | 91.6% |

## Fix Commits

| Commit | Message | Findings Resolved |
|--------|---------|-------------------|
| 70ee2be | fix(smoke,docs): add smoke test and CHANGELOG entry for M2-010 | #1, #2 |

## Summary

2/2 findings resolved. 0 deferred.
