# curlew — Vault and secret providers reference

Load this file when the user asks about secret management, 1Password, Bitwarden,
vault providers, or secret redaction.

## Secret providers

Curlew integrates with secret managers so credentials never appear in
collection YAML. Configure providers in `curlew.yaml`:

```yaml
vault:
  provider: 1password
  account: my.1password.com
```

Then reference vault items with the `vault://` scheme:

```yaml
variables:
  api_key: "vault://Personal/API Key/credential"
```

Curlew resolves the vault reference at run time and substitutes the secret
value into `{{api_key}}` throughout the collection.

## Supported providers

### 1Password

```yaml
vault:
  provider: 1password
  account: my.1password.com       # optional if op CLI has a default account
```

Requires the `op` CLI to be installed and authenticated. The reference format
is `vault://<vault-name>/<item-title>/<field-name>`.

### Bitwarden

```yaml
vault:
  provider: bitwarden
```

Requires the `bw` CLI to be installed and unlocked (`bw unlock`). The
reference format is `vault://<collection-or-folder>/<item-name>/<field-name>`.

### Custom provider

```yaml
vault:
  provider: custom
  command: ["./scripts/get-secret.sh"]
```

Curlew calls the command with the vault path as the first argument and expects
the secret on stdout. Use this for HashiCorp Vault, AWS Secrets Manager,
Azure Key Vault, or any other secret store.

## Redaction contract

Any value resolved from a vault provider is added to the sensitive-value set.
Curlew redacts sensitive values in:

- All output formats (terminal, markdown, JSON, TAP, JUnit, HTML)
- The NDJSON event stream
- Log lines and error messages

Redacted values appear as `[REDACTED]`. The original bytes never appear in any
artifact.

If you extract a vault-resolved secret into a variable with `extract:`, the
extracted value inherits the redaction flag and is also redacted.

## Notes

- Vault lookups add latency. Curlew caches resolved secrets for the lifetime
  of a single run to avoid repeated round-trips.
- If a vault reference cannot be resolved (missing item, auth failure), the
  run exits with code 3 (configuration error) and stderr names the unresolved
  reference.
- For OS environment secrets (not vault), use `env_import:` — see
  `variables.md`.
