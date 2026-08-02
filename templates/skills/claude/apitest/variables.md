# apitest — Variables reference

Load this file when the user asks about variable interpolation, environment
files, variable precedence, or extracting values from responses.

## Variable syntax

Wrap a variable name in double braces: `{{name}}`. ApiTool interpolates every
string value in a collection before the HTTP request is sent.

```yaml
requests:
  - name: Get user
    request:
      method: GET
      url: "{{base_url}}/users/{{user_id}}"
```

## Variable types

| Type | Where defined | Example |
|---|---|---|
| **Literal** | `apitest.yaml` `variables:` block | `base_url: "https://api.example.com"` |
| **Environment** | `environments/<name>.yaml` `variables:` block | per-env overrides |
| **OS env import** | `env_import:` list in collection or root config | `env_import: [API_KEY]` |
| **CLI override** | `--var name=value` flag | ephemeral, highest priority |
| **Extract** | `extract:` on a request | captured from a previous response |

## Precedence ladder (highest wins)

1. `--var` CLI override
2. OS env import (`env_import:`)
3. Active environment (`environments/<name>.yaml`)
4. Root project (`apitest.yaml` `variables:`)
5. Collection-level `variables:` block
6. Literal fallback defaults

## Environments

Select an environment with `--env <name>`. ApiTool loads
`environments/<name>.yaml` and merges its `variables:` block onto the root
layer.

```yaml
# environments/staging.yaml
variables:
  base_url: "https://staging.api.example.com"
  timeout_ms: 5000
```

```bash
apitest run collections/users.yaml --env staging
```

## OS env import

Pull secrets from the OS environment without committing them:

```yaml
# apitest.yaml
env_import:
  - API_KEY
  - DB_PASSWORD
```

At runtime, ApiTool reads `$API_KEY` and `$DB_PASSWORD` from the process
environment and makes them available as `{{API_KEY}}` and `{{DB_PASSWORD}}`.
Store real values in `.env` (git-ignored); put placeholders in `.env.example`
(committed).

## Extracting from responses

Capture a value from one response and use it in the next request:

```yaml
requests:
  - name: Login
    request:
      method: POST
      url: "{{base_url}}/auth/login"
      body:
        username: alice
        password: "{{PASSWORD}}"
    extract:
      token: "$.access_token"   # JSONPath

  - name: Get profile
    request:
      method: GET
      url: "{{base_url}}/me"
      headers:
        Authorization: "Bearer {{token}}"
```

`extract:` values use JSONPath (`$.<path>`) for JSON responses and XPath for
XML. Extracted variables are available to all subsequent requests in the same
run.

## Notes

- Undefined variables cause exit code 5. Run `apitest validate` to catch them
  before hitting the network.
- Circular references (`a: "{{b}}"`, `b: "{{a}}"`) are also exit code 5.
- For CEL expressions that reference variables at runtime, see `expressions.md`.
