# curlew — Variables reference

Interpolate strings using `{{name}}`. Use collection/project/request `variables:`
blocks, `environments/<name>.yaml` selected with `--env`, or CLI overrides.
`--var name=value` has highest priority; `--env-var NAME` imports a process variable
(and `--env-var 'name=$OS_NAME'` renames one). There is no `env_import:` YAML key.

The precedence order, low to high, is dynamic functions, project variables,
environment file, `.env`, command-backed variables, vault secrets, collection
variables, request variables, `--env-var`, `--var`. Extracted values are propagated
as execution proceeds; avoid reusing a name for unrelated meanings.

Use `extract: {name: "$.path"}` to capture JSON into later requests. Extraction is
JSONPath-only. XML XPath extraction is not supported. `validate` checks collection
structure but cannot prove that future response fields or dynamic values exist.
Undefined/circular runtime variables exit with code 5; inspect stderr and current events.

## Run the example

Start Mudflat using the repository's `site/README.md` setup. Save the complete
collection below as `example.yaml` in a scratch directory. `MUDFLAT_URL` defaults
to that setup's local port; override it if your fixture uses another port.

```bash
export MUDFLAT_URL="${MUDFLAT_URL:-http://127.0.0.1:18080}"
export RUN_ID="agent-$(date +%s)-$$"
curlew validate example.yaml --format json
curlew run example.yaml --var mud="$MUDFLAT_URL" --var run="$RUN_ID" --format json
```

## Complete collection

<!-- agent-source: examples/agent/variables.yaml -->
```yaml
name: Extraction example
requests:
- name: create a resource
  request:
    method: POST
    url: '{{mud}}/s/{{run}}-chain/resources'
    body:
      name: first
      kind: widget
  assertions:
    status: 201
    headers:
      Location:
        matches: /resources/res_
      ETag:
        matches: ^"[0-9a-f]{32}"$
    body:
      $.id:
        matches: ^res_.+_1$
      $.seq:
        equals: 1
      $.body.name:
        equals: first
  extract:
    first_id: $.id
    first_etag: $.etag
- name: fetch it by the extracted id
  request:
    method: GET
    url: '{{mud}}/s/{{run}}-chain/resources/{{first_id}}'
  assertions:
    status: 200
    body:
      $.body.name:
        equals: first
      $.body.kind:
        equals: widget
      $.seq:
        equals: 1
      $.id:
        equals: '{{first_id}}'
      $.etag:
        equals: '{{first_etag}}'
```
