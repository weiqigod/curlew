# testdata/enterprise/

Test fixtures for M5-020 (E2E enterprise full-pipeline spec).

## Contents

| File | Purpose |
|------|---------|
| `fake-idp-cert.pem` | X.509 self-signed certificate for the fake SAML IdP |
| `fake-idp-key.pem` | Matching RSA-2048 private key |
| `e2e-collection.yaml` | API test collection run by the qa-lead service token |

## Security notice

**The RSA private key (`fake-idp-key.pem`) is committed intentionally.**

This keypair is used exclusively by the local test stack (`docker-compose.test.yml`) to
sign fake SAML assertions for the fake IdP sidecar. It has no production use and is not a
secret — anyone with this repo already has both halves of the keypair.

**Do NOT use this keypair in any production environment.**

The corresponding certificate is registered with the backend's SAML configuration via
`scripts/seed-enterprise.sh` at stack startup time.
