# Agent Harness Fixtures

This directory contains curated, deliberately-broken fixture collections used by
the agent validation harness (`cmd/curlew-agent-harness/`). Each subdirectory is
a self-contained scenario with a fixture collection and an `expect.yaml` contract.

## Contract format (`expect.yaml`)

```yaml
description: |
  Human-readable description of what the scenario tests.
exit_code: 5           # expected process exit code
events:
  - kind: run.error    # required event kind in the NDJSON stream
    must_have:
      error.category: input          # exact match
      error.code: VAR_UNDEFINED      # exact match
      error.file_nonempty: true      # field must be non-empty string
      error.line_nonzero: true       # field must be non-zero integer
      error.code_pattern: "^VAR_"   # field must match the regex
    hint_contains_any:
      - Set             # error.hint must contain at least one of these
      - Define
```

## Scenarios

| Scenario | Expected failure | exit code |
|----------|-----------------|-----------|
| `missing-variable` | VAR_UNDEFINED on undefined variable reference | 5 |
| `bad-yaml` | PARSE_INVALID_YAML on malformed YAML | 3 |
| `failing-assertion` | ASSERTION_FAILED on status mismatch | 1 |
| `unreachable-host` | NETWORK_CONNECTION_REFUSED on port 1 | 4 |
| `auth-missing` | RUNNER_AUTH_PROFILE_NOT_FOUND on missing auth profile | 5 |
| `circular-include` | PARSE_CIRCULAR_INCLUDE for a→b→a include loop | 3 |

## Template collections

Scenarios using `collection.template.yaml` (instead of `collection.yaml`) have
`{{SERVER_URL}}` substituted at test time with a real `httptest.Server` URL.
This allows the scenario to make real HTTP requests without hard-coding ports.

## Optional files

- `args.txt` — extra CLI args passed to `curlew run`, one per line (`#` = comment)
- `env.txt` — environment variables `KEY=VALUE` per line (`#` = comment)
