# curlew — Signing reference

Place `signing:` beside `request:`. It contains `type` and a nested `params`
map. Built-in types are `aws-sigv4` and `oauth1`.

SigV4 parameters: `region`, `service`, `access_key`, `secret_key`, optional
`session_token`. OAuth parameters: `consumer_key`, `consumer_secret`, optional
`token`, `token_secret`, `method`, `realm`; HMAC-SHA1 is the default and HMAC-SHA256
is supported. There is no `signing_fn` YAML key.

For real services, obtain credentials through `--env-var`, sensitive variables,
or the `secrets:` project configuration. The values below are published Mudflat
fixture credentials, not an AWS or OAuth account. The local server recomputes both
signatures; this proves signing compatibility with that fixture, not account access.

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

<!-- agent-source: examples/agent/signing.yaml -->
```yaml
name: Locally verified signatures
variables:
  aws_access_key: AKIAMUDFLATTEST0000
  aws_secret_key: wJalrMudflatEXAMPLEKEY/K7MDENG/bPxRfi
  oauth_consumer_key: mud_consumer_0001
  oauth_consumer_secret: mud_consumer_secret_0001
  oauth_token_secret: mud_token_secret_0001
requests:
- name: aws sigv4 signature is accepted
  request:
    method: GET
    url: '{{mud}}/verify/sigv4'
  signing:
    type: aws-sigv4
    params:
      region: us-east-1
      service: execute-api
      access_key: '{{aws_access_key}}'
      secret_key: '{{aws_secret_key}}'
  assertions:
    status: 200
    body:
      $.ok:
        equals: true
      $.algorithm:
        equals: AWS4-HMAC-SHA256
      $.checks:
        type: array
    cel:
    - response.body.checks.all(c, c.ok)
- name: oauth 1.0a signature is accepted
  request:
    method: POST
    url: '{{mud}}/verify/oauth1'
  signing:
    type: oauth1
    params:
      consumer_key: '{{oauth_consumer_key}}'
      consumer_secret: '{{oauth_consumer_secret}}'
      token_secret: '{{oauth_token_secret}}'
      nonce: fixednonce
      timestamp: '1700000000'
  assertions:
    status: 200
    body:
      $.ok:
        equals: true
      $.algorithm:
        contains: OAuth
      $.base_string:
        contains: oauth_consumer_key
```
