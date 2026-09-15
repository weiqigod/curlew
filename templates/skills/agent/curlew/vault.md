# curlew — Vault reference

Use a `secrets:` block in `curlew.yaml`, with `provider`, provider settings, and a
`keys:` mapping. Registered providers are `aws-secrets-manager`, `azure-key-vault`,
`hashicorp-vault`, `gcp-secret-manager`, and `1password`; their CLIs must be installed
and authenticated. Discover configured names with `curlew vault list`.
Do not use the obsolete `vault.provider`/`vault://` shape or assume Bitwarden is a
built-in provider. For a command-backed secret, a variable can use `from_command`
and `sensitive: true`; inspect the command before executing it.

Provider configuration example (requires your own account and item):

```yaml
# curlew.yaml fragment; not part of the local example below
secrets:
  provider: 1password
  keys:
    api_key: "op://Employee/demo-api/api-key#credential"
  cache_ttl: 300
```

Use `{{api_key}}` in a request for this project-level configuration.
The `{{secrets.name}}` syntax belongs to shared team templates, which require
the corresponding team configuration and selected environment. Vault-resolved values are sensitive.
Keep redaction enabled and review artifacts before sharing; arbitrary response
content may still contain secrets. Provider access is not tested by a local fixture.
The complete collection below demonstrates sensitivity using a public dummy value.

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

<!-- agent-source: examples/agent/vault.yaml -->
```yaml
name: Local secret redaction
variables:
  demo_token:
    value: local-example-token
    sensitive: true
requests:
- name: Echo a sensitive header
  request:
    method: GET
    url: '{{mud}}/echo'
    headers:
      Authorization: Bearer {{demo_token}}
  assertions:
    status: 200
```
