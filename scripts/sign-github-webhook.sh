#!/usr/bin/env bash
# Computes the X-Hub-Signature-256 header value for a GitHub webhook delivery.
# Usage: bash scripts/sign-github-webhook.sh <secret> <body-file>
# Or pipe body: echo '{"action":"created"}' | bash scripts/sign-github-webhook.sh <secret>
set -euo pipefail

SECRET="${1:?Usage: $0 <secret> [body-file]}"
BODY_FILE="${2:-/dev/stdin}"

SIG="sha256=$(cat "$BODY_FILE" | openssl dgst -sha256 -hmac "$SECRET" -binary | xxd -p -c 256)"
echo "X-Hub-Signature-256: $SIG"
