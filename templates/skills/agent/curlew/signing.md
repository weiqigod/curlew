# curlew — Request signing reference

Load this file when the user asks about AWS SigV4 signing, OAuth 1.0a,
HMAC signing, or dynamic signing functions.

## Signing block

Add `signing:` to a request to apply a request-signing scheme before the
HTTP call is sent:

```yaml
requests:
  - name: List S3 objects
    request:
      method: GET
      url: "https://s3.amazonaws.com/{{bucket}}/"
    signing:
      type: aws-sigv4
      region: us-east-1
      service: s3
      access_key: "{{AWS_ACCESS_KEY_ID}}"
      secret_key: "{{AWS_SECRET_ACCESS_KEY}}"
```

## Supported signers

### `aws-sigv4` — AWS Signature Version 4

Signs requests for any AWS service. Required fields:

| Field | Meaning |
|---|---|
| `region` | AWS region (e.g. `us-east-1`) |
| `service` | AWS service name (e.g. `s3`, `execute-api`, `sts`) |
| `access_key` | AWS access key ID (use a variable or vault reference) |
| `secret_key` | AWS secret access key (use a variable or vault reference) |
| `session_token` | Optional session token for temporary credentials |

```yaml
signing:
  type: aws-sigv4
  region: eu-west-1
  service: execute-api
  access_key: "{{AWS_ACCESS_KEY_ID}}"
  secret_key: "{{AWS_SECRET_ACCESS_KEY}}"
  session_token: "{{AWS_SESSION_TOKEN}}"
```

### `oauth1` — OAuth 1.0a

Signs requests using the OAuth 1.0a HMAC-SHA1 scheme. Required fields:

| Field | Meaning |
|---|---|
| `consumer_key` | OAuth consumer key |
| `consumer_secret` | OAuth consumer secret |
| `token` | OAuth access token |
| `token_secret` | OAuth access token secret |

```yaml
signing:
  type: oauth1
  consumer_key: "{{OAUTH_CONSUMER_KEY}}"
  consumer_secret: "{{OAUTH_CONSUMER_SECRET}}"
  token: "{{OAUTH_TOKEN}}"
  token_secret: "{{OAUTH_TOKEN_SECRET}}"
```

## Dynamic signing functions

When the signing parameters depend on runtime values (e.g. a timestamp
embedded in a signature), use `signing_fn:` to call a registered dynamic
function:

```yaml
signing:
  type: custom
  signing_fn: "my_hmac_fn"
  params:
    key: "{{SIGNING_KEY}}"
    algorithm: sha256
```

Dynamic functions are registered as plugins. See the plugin documentation
(`docs/MANUAL.md` §10) for the plugin API.

## Notes

- All signing secrets (access keys, consumer secrets, token secrets) should
  come from `env_import:` or a vault provider — never hardcode them in YAML.
- AWS SigV4 signs the `Authorization` header, the `x-amz-date` header, and
  optionally a `x-amz-security-token` header. Curlew sets these automatically.
- OAuth 1.0a signs the `Authorization: OAuth ...` header. The nonce and
  timestamp are generated fresh per request.
- Signing happens after variable interpolation but before the HTTP call.
